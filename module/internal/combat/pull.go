package combat

import (
	"context"
	"log/slog"
	"math"
	"time"

	pb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/safe"
)

// Pulling a single monster. A monster attacks on sight within its aggro range; the ones around our
// target join the fight (aggro, or called for help within ~10 yards). Before engaging, the target
// is chosen so that it and its adds fit in our capacity; failing that we body pull it: stand where
// only it sees us, wait until it comes, back off to a safe spot, then fight.

func aggroRange(player, creature uint32) float32 { return safe.AggroRange(player, creature) }

// assistRange: a monster attacked calls for help within this distance (CreatureFamilyAssistanceRadius).
const assistRange = 10

// pullWeight is how much a monster counts against the capacity: weaker monsters less, stronger more.
func pullWeight(player, creature uint32) float32 {
	d := int(creature) - int(player)
	switch {
	case d <= -5:
		return 0.25
	case d <= -3:
		return 0.5
	case d >= 3:
		return 2
	case d >= 1:
		return 1.25
	}
	return 1
}

// pullCapacity is how many monsters (weighted) we accept at once by default. It grows with the level
// (more health, area spells); see BACKLOG for making it learn from the fights.
func pullCapacity(level uint32) float32 {
	switch {
	case level >= 40:
		return 2
	case level >= 20:
		return 1.5
	}
	return 1
}

func dist2D(ax, ay, bx, by float32) float32 {
	return float32(math.Hypot(float64(ax-bx), float64(ay-by)))
}

// addsOf lists the hostile monsters that would join a fight at the target's spot: not engaged with
// anyone, and within their aggro range (or the assistance range) of the target.
func addsOf(t *statepb.Obj, hostiles []*statepb.Obj, level uint32) []*statepb.Obj {
	var out []*statepb.Obj
	for _, h := range hostiles {
		if h.Guid == t.Guid || h.Target != 0 {
			continue
		}
		if dist2D(h.X, h.Y, t.X, t.Y) <= max(aggroRange(level, h.Level), assistRange)+2 {
			out = append(out, h)
		}
	}
	return out
}

// pullLoad is the weighted count of the target and its adds. The target alone counts at most 1: a
// monster a level or two above us is still fine alone (too strong ones are refused before, by the
// danger margin); only the adds can push the load over the capacity.
func pullLoad(t *statepb.Obj, adds []*statepb.Obj, level uint32) float32 {
	w := min(pullWeight(level, t.Level), 1)
	for _, a := range adds {
		w += pullWeight(level, a.Level)
	}
	return w
}

// pullSpot finds where to stand so that only the target sees us: within its aggro range, out of the
// others' (keeping margin yards). It prefers the largest margin, then the spot nearest to us.
func pullSpot(me, t *statepb.Obj, others []*statepb.Obj, level uint32) (x, y float32, ok bool) {
	reach := aggroRange(level, t.Level)
	bestScore := float32(-1e9)
	for _, d := range []float32{reach - 2, reach - 4, reach - 6} {
		if d < 4 {
			continue
		}
		for i := 0; i < 16; i++ {
			a := float64(i) * math.Pi / 8
			px, py := t.X+d*float32(math.Cos(a)), t.Y+d*float32(math.Sin(a))
			margin := float32(100)
			for _, o := range others {
				margin = min(margin, dist2D(px, py, o.X, o.Y)-max(aggroRange(level, o.Level), assistRange))
			}
			if margin < 3 {
				continue
			}
			score := min(margin, 15) - 0.05*dist2D(px, py, me.X, me.Y)
			if score > bestScore {
				bestScore, x, y, ok = score, px, py, true
			}
		}
	}
	return x, y, ok
}

// choosePull returns the target to fight, after a body pull when needed; nil when no monster of the
// entry can be engaged safely for now (they are marked as given up for a while).
func (s *Service) choosePull(ctx context.Context, j *job, me *statepb.SelfInfo, r *pb.KillRequest, radius float32) (*statepb.Obj, error) {
	if r.Guid != 0 { // a given unit (clearing a path)
		o, err := s.api.Obj(ctx, r.Guid)
		if err != nil || (o.Hp == 0 && o.MaxHp > 0) {
			return nil, nil
		}
		return o, nil
	}
	cands, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Entry: r.Entry, Radius: radius, Max: 8, AliveOnly: true})
	if err != nil {
		return nil, err
	}
	hostile, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: radius + 40, Max: 60, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return nil, err
	}
	capacity := r.MaxPull
	if capacity <= 0 {
		capacity = pullCapacity(me.Level)
	}
	self := &statepb.Obj{Guid: me.Guid, X: me.X, Y: me.Y, Z: me.Z}
	var crowded, taken []*statepb.Obj
	for _, c := range cands.Objs {
		if s.wasAbandoned(c.Guid) {
			continue
		}
		if c.Target == me.Guid {
			return c, nil // already on us
		}
		if c.Target != 0 {
			taken = append(taken, c)
			continue
		}
		adds := addsOf(c, hostile.Objs, me.Level)
		load := pullLoad(c, adds, me.Level)
		if load <= capacity {
			return c, nil
		}
		slog.Info("crowded target", "entry", c.Entry, "level", c.Level, "dist", int(c.Dist), "adds", len(adds), "load", load, "capacity", capacity)
		crowded = append(crowded, c)
	}
	for i, c := range crowded {
		if r.BodyPull && i < 2 && s.bodyPull(ctx, j, self, c, hostile.Objs, me.Level) {
			return c, nil
		}
		s.abandon(c.Guid, "too many monsters around it", c.Entry)
	}
	if len(crowded) > 0 {
		slog.Info("pull choice", "candidates", len(cands.Objs), "crowded", len(crowded), "taken", len(taken))
	}
	if len(crowded) == 0 && len(taken) > 0 {
		return taken[0], nil // the old behaviour: share a monster someone else fights
	}
	return nil, nil
}

// bodyPull brings the target alone: go where only it sees us, wait for it to come, then back off to a
// spot away from the others. It returns whether the target is now coming at us.
func (s *Service) bodyPull(ctx context.Context, j *job, me, t *statepb.Obj, hostiles []*statepb.Obj, level uint32) bool {
	if t.Reaction != "hostile" {
		return false // a neutral monster never comes by itself
	}
	var others []*statepb.Obj
	for _, h := range hostiles {
		if h.Guid != t.Guid && h.Target == 0 {
			others = append(others, h)
		}
	}
	x, y, ok := pullSpot(me, t, others, level)
	if !ok {
		return false
	}
	j.Update(func(st *pb.CombatStatus) { st.Doing = "body pulling" })
	slog.Info("body pull", "entry", t.Entry, "others", len(others), "spot", [2]float32{x, y})
	if st, err := s.api.GotoWait(ctx, x, y, t.Z, 1.5, 60); err != nil || st.State != "arrived" {
		return s.onUs(ctx, t.Guid)
	}
	for i := 0; i < 8; i++ {
		if s.onUs(ctx, t.Guid) {
			s.backOff(ctx)
			return true
		}
		if !sleep(ctx, time.Second) {
			return false
		}
	}
	return false
}

// onUs tells whether the unit targets us.
func (s *Service) onUs(ctx context.Context, guid uint64) bool {
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 80, Max: 40, AliveOnly: true})
	if err != nil {
		return false
	}
	for _, o := range vis.Objs {
		if o.Guid == guid {
			return o.Target == s.guid
		}
	}
	return false
}

// backOff moves to a spot near us away from the hostile monsters (path included), so the fight with
// the pulled monster happens far from its friends.
func (s *Service) backOff(ctx context.Context) {
	b, err := s.api.Brief(ctx)
	if err != nil || b.Self == nil {
		return
	}
	me := b.Self
	sp, err := s.api.SafePoint(ctx, me.X, me.Y, me.Z, 15, true)
	if err != nil || !sp.Ok {
		return
	}
	_, _ = s.api.GotoWait(ctx, sp.X, sp.Y, sp.Z, 2, 15)
}
