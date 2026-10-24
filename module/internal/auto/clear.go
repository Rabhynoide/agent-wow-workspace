package auto

import (
	"context"
	"fmt"
	"math"
	"strings"

	combatpb "wowagent/gen/combatv1"
	movepb "wowagent/gen/movev1"
	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/safe"
	"wowagent/internal/xpscore"
)

// Clearing a path step by step (user's idea for the mine explorations): look at the next stretch of
// the path, fight the idle hostile monsters whose aggro reaches it one at a time (nearest first),
// rest, then walk that stretch, and so on. A walk straight in brings the whole mine on us.

const (
	clearStretch = 25 // yards of path looked at and walked per step
	clearMargin  = 3  // yards added to the monsters' aggro range
)

// stretch returns the path points within the first `length` yards.
func stretch(pts []*movepb.PathPoint, length float32) []*movepb.PathPoint {
	var out []*movepb.PathPoint
	var walked float32
	for i, p := range pts {
		if i > 0 {
			q := pts[i-1]
			walked += float32(math.Hypot(float64(p.X-q.X), float64(p.Y-q.Y)))
		}
		if walked > length {
			break
		}
		out = append(out, p)
	}
	return out
}

// blockers lists the idle hostile monsters whose aggro range reaches the stretch, nearest to us first.
// rangeOf gives a monster's aggro radius (its detection range, safe.AggroRangeOf); one more than
// safe.ZAggro yards above or below the path (plus a yard: path heights are approximate) cannot aggro.
func blockers(seg []*movepb.PathPoint, hostiles []*statepb.Obj, rangeOf func(*statepb.Obj) float32) []*statepb.Obj {
	var out []*statepb.Obj
	for _, h := range hostiles {
		reach := rangeOf(h) + clearMargin
		for _, p := range seg {
			if math.Hypot(float64(h.X-p.X), float64(h.Y-p.Y)) <= float64(reach) && math.Abs(float64(h.Z-p.Z)) <= safe.ZAggro+1 {
				out = append(out, h)
				break
			}
		}
	}
	for i := 1; i < len(out); i++ { // by distance to us (Dist from state.nearby)
		for j := i; j > 0 && out[j].Dist < out[j-1].Dist; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// clearWalk walks to (x, y, z) clearing the monsters on the way.
func (r *run) clearWalk(x, y, z, radius float32) error {
	for step := 0; step < 60; step++ {
		snap, err := r.snapshot()
		if err != nil {
			return err
		}
		me := snap.Self
		if math.Hypot(float64(me.X-x), float64(me.Y-y)) <= float64(radius) {
			return nil
		}
		path, err := r.s.api.PathPoints(r.ctx, x, y, z)
		if err != nil || path.Error != "" || len(path.Pts) < 2 {
			return r.walk(x, y, z, radius) // no path: the plain walk reports why
		}
		seg := stretch(path.Pts, clearStretch)
		vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 70, Max: 40, AliveOnly: true, HostileOnly: true})
		if err != nil {
			return err
		}
		if bl := blockers(seg, vis.Objs, r.s.aggroOf(r.ctx, me.Level)); len(bl) > 0 {
			t := bl[0]
			sp := &worldpb.Spawn{Entry: t.Entry, Name: fmt.Sprintf("monster %d", t.Entry), MinLevel: t.Level, MaxLevel: t.Level}
			if why := r.s.tooDangerous(sp, me.Level, me.ClassId); why != "" {
				return fmt.Errorf("path blocked: %s", why)
			}
			// One fight at a time, but against a whole passage: each guard must cost at most
			// passageLoss (bench c8edda2: three deaths clearing the Fargodeep mine at level 5 against
			// level-6 kobolds, about 28 % each). "too dangerous" makes level.sh postpone the quest to
			// the next level.
			if loss, _ := xpscore.Single(spawnMob(sp), t.Level, me.Level, me.ClassId, r.s.fights.Get()); loss > passageLoss {
				return fmt.Errorf("too dangerous for now: path guarded by monster %d (level %d, character %d, expected loss %.0f %%)", t.Entry, t.Level, me.Level, loss)
			}
			r.step("clearing the way: monster %d (level %d) %.0f yards away", t.Entry, t.Level, t.Dist)
			if _, err := r.fightJob(&combatpb.KillRequest{Guid: t.Guid, Radius: 80, MaxS: 120, WaitS: 20}); err != nil &&
				!strings.Contains(err.Error(), "no live entry") {
				return err
			}
			r.restIfHurt()
			continue
		}
		end := seg[len(seg)-1]
		if err := r.walk(end.X, end.Y, end.Z, 2); err != nil {
			return err
		}
	}
	return fmt.Errorf("clearing the way to (%.0f,%.0f): too many steps", x, y)
}

// passageLoss: the most health (percent) one guard of a passage may cost on average; the fights
// follow each other with a rest only below 70 %.
const passageLoss = 0.6 * xpscore.MaxLoss

// restIfHurt rests (and eats) below 70 % health before the next monster.
func (r *run) restIfHurt() { r.restBelow(70) }

// walkRestBelow: a walk starts (or goes on after a fight) only above this health (percent).
const walkRestBelow = 40

// restBelow rests (and eats) when alive with less than pct % health.
func (r *run) restBelow(pct uint32) {
	b, err := r.s.api.Brief(r.ctx)
	if err != nil || b.Self == nil || b.Self.Dead || b.Self.Ghost || b.Self.MaxHp == 0 || b.Self.Hp*100/b.Self.MaxHp >= pct {
		return
	}
	r.step("resting first (%d %% health)", b.Self.Hp*100/b.Self.MaxHp)
	st, err := r.s.api.CombatRest(r.ctx)
	for err == nil && st.State == "running" {
		st, err = r.s.api.CombatStatus(r.ctx, 20)
	}
}

// aggroOf gives a monster's aggro radius against us from its detection range (creature template, read
// once per entry; 18 in Elwynn instead of the default 20).
func (s *Service) aggroOf(ctx context.Context, level uint32) func(*statepb.Obj) float32 {
	return func(o *statepb.Obj) float32 {
		det := float32(safe.DefaultDetection)
		if v, ok := s.detection.Load(o.Entry); ok {
			det = v.(float32)
		} else {
			if c, err := s.api.Creature(ctx, o.Entry); err == nil && c.DetectionRange > 0 {
				det = c.DetectionRange
			}
			s.detection.Store(o.Entry, det)
		}
		return safe.AggroRangeOf(det, level, o.Level)
	}
}
