package combat

import (
	"context"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"math"
	"time"

	pb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/xpscore"
)

// Fleeing. The pull choice cannot stop a monster that joins a fight already engaged (wandering
// wolves: the deaths of the 2026-10-12 benches). During a fight, once a second, the expected loss
// still to come (xpscore.LossLeft: each attacker's learned single fight times the health it has
// left, plus the add cost) is compared with our health; above it minus fleeMargin, we run to a safe
// spot. Monsters run at 0.86 times our speed (creature_template.speed_run of the Elwynn monsters)
// and give up 30 yards from where they were pulled (CreatureLeashRadius), so running away works as
// long as no spell slows us.

const (
	// fleeMargin: health (percent) we want left when the fight is expected to end. A fight can cost
	// twice its average; a death (150 s, durability) costs far more than a flight (a kill lost).
	fleeMargin = 15.0
	// fleeEvery: no new flight this soon after one (no flight loop). 90 s at first: both deaths of the
	// 2026-10-13 bench came 40-60 s after a successful flight, when the same wolves caught up again.
	fleeEvery = 15 * time.Second
	fleeMaxS  = 40 // running time before fighting again
	fleeDist  = 35 // yards per leg of the run (safe point search radius)
	// crowdFlee: this many attackers with more than half their health and we run whatever our health.
	// The flight only works while few monsters chase us: with 5-6 kobolds of a pack (Fargodeep mine,
	// 6 → 10 bench of 2026-10-13) the ones still on us at the start of the run killed us twice.
	crowdFlee = 3
	// crowdLoss: ... as long as they are expected to cost this much (percent). Three kobolds two levels
	// below (19 %) made us flee at 92 % health and postponed the mine quest level after level (bench of
	// 2026-10-13: 0 quests in 100 minutes).
	crowdLoss = 30.0
	// highMargin: added to fleeMargin per attacker at least 2 levels above us: its hits in the first
	// seconds of the run cost more (two deaths to a level-10 Riverpaw Outrunner at level 8, flights
	// started at 44 and 20 %).
	highMargin = 10.0
	// chaseMargin: added per fresh attacker (more than half its health) beyond the first: each keeps
	// hitting us while we run (Prowler + Young Forest Bear at level 9: flight started at 34 %, dead 8 s
	// later, bench of 2026-10-13).
	chaseMargin = 10.0
)

// onMe lists the units in combat that target us within 30 yards.
func (s *Service) onMe(ctx context.Context, me *statepb.SelfInfo) []*statepb.Obj {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 30, Max: 12, AliveOnly: true})
	if err != nil {
		return nil
	}
	var out []*statepb.Obj
	for _, o := range r.Objs {
		if o.Target == me.Guid && o.UnitFlags&unitInCombat != 0 {
			out = append(out, o)
		}
	}
	return out
}

// lossLeft is the expected loss of the fight still to come against these attackers.
func (s *Service) lossLeft(me *statepb.SelfInfo, att []*statepb.Obj) float64 {
	levels, left := make([]uint32, len(att)), make([]float64, len(att))
	for i, a := range att {
		levels[i], left[i] = a.Level, 1
		if a.MaxHp > 0 {
			left[i] = float64(a.Hp) / float64(a.MaxHp)
		}
	}
	return xpscore.LossLeft(levels, left, me.Level, me.ClassId, s.fights.Get())
}

// mustFlee tells whether the fight is expected to cost more health than we have (with fleeMargin),
// with the expected loss and the attackers.
func (s *Service) mustFlee(ctx context.Context, me *statepb.SelfInfo) (bool, float64, []*statepb.Obj) {
	if s.since(&s.fledAt) < fleeEvery {
		return false, 0, nil
	}
	att := s.onMe(ctx, me)
	if len(att) == 0 {
		return false, 0, nil
	}
	// A potion ready in the bags is health too: running from two melee monsters costs hits all the
	// way (Benchtrhoxd died 11 s into a flight started at 55 %, with an expected loss of 41 %).
	if s.potionReady(ctx, me) {
		boosted := proto.Clone(me).(*statepb.SelfInfo)
		boosted.Hp = min(me.MaxHp, me.Hp+me.MaxHp*potionHeal/100)
		me = boosted
	}
	ok, loss := s.losing(me, att)
	return ok, loss, att
}

// losing tells whether the fight still to come against these attackers is expected to leave us less
// than fleeMargin of health, with its expected loss.
func (s *Service) losing(me *statepb.SelfInfo, att []*statepb.Obj) (bool, float64) {
	if len(att) == 0 {
		return false, 0
	}
	loss := s.lossLeft(me, att)
	margin := fleeMargin
	fresh := 0
	for _, a := range att {
		if a.Level >= me.Level+2 {
			margin += highMargin
		}
		if a.MaxHp > 0 && 2*a.Hp > a.MaxHp {
			fresh++
		}
	}
	if fresh > 1 {
		margin += chaseMargin * float64(fresh-1)
	}
	return (crowd(att) && loss > crowdLoss) || loss > float64(pct(me.Hp, me.MaxHp))-margin, loss
}

// crowd tells whether at least crowdFlee attackers have more than half their health.
func crowd(att []*statepb.Obj) bool {
	n := 0
	for _, a := range att {
		if a.MaxHp > 0 && 2*a.Hp > a.MaxHp {
			n++
		}
	}
	return n >= crowdFlee
}

// flee runs back along our trail (trail.go), then from safe spot to safe spot, until nothing targets
// us (or fleeMaxS), and reports whether we died on the way. The attackers are skipped by the pull choice for a while, not given up: we
// still fight them back if they catch us.
func (s *Service) flee(ctx context.Context, j *job, me *statepb.SelfInfo, att []*statepb.Obj, loss float64) bool {
	s.mark(&s.fledAt)
	z := zoneOf(me.Map, att, time.Now())
	s.addZone(z)
	entries := make([]uint32, len(att))
	for i, a := range att {
		entries[i] = a.Entry
		s.skipCrowded(a.Guid)
	}
	// Back along the trail unless its first leg leads towards the attackers (they came from behind).
	legs, idx := s.trail.back(me.Map, me.X, me.Y)
	if len(legs) > 0 && dist2(legs[0].x, legs[0].y, z.x, z.y) < dist2(me.X, me.Y, z.x, z.y) {
		legs = nil
	}
	way := "safe_point"
	if len(legs) > 0 {
		way = "trail"
	}
	slog.Info("fleeing", "hp", pct(me.Hp, me.MaxHp), "loss_left", int(loss), "attackers", entries, "crowd", crowd(att),
		"zone", [2]int{int(z.x), int(z.y)}, "way", way, "legs", len(legs))
	j.Update(func(st *pb.CombatStatus) { st.Doing = "fleeing" })
	s.trail.pause(true)
	end := -1
	defer func() { s.trail.cut(end) }()
	started := time.Now()
	leg, clear := 0, 0
	for time.Since(started) < fleeMaxS*time.Second {
		b, err := s.api.Brief(ctx)
		if err != nil || b.Self == nil {
			if !sleep(ctx, 300*time.Millisecond) {
				return false
			}
			continue
		}
		me = b.Self
		if me.Dead {
			s.forgetBuffs()
			return true
		}
		s.drinkPotion(ctx, me) // still hit while running
		// Twice in a row, running on in between: the chasers lost us for a moment and caught up again
		// right after "fled" (Fargodeep mine, 2026-10-13).
		if len(s.onMe(ctx, me)) > 0 {
			clear = 0
		} else if clear++; clear >= 2 {
			slog.Info("fled", "hp", pct(me.Hp, me.MaxHp), "s", int(time.Since(started).Seconds()), "legs", leg)
			return false
		}
		if leg < len(legs) {
			p := legs[leg]
			if _, err := s.api.GotoWait(ctx, p.x, p.y, p.z, 3, 8); err != nil && ctx.Err() != nil {
				return false
			}
			end = idx[leg]
			leg++
			continue
		}
		// The attackers are threats too: the point is away from them and from the other monsters
		// (not path_clear: our own spot, next to them, would rule out every path).
		sp, err := s.api.SafePoint(ctx, me.X, me.Y, me.Z, fleeDist, false)
		if err != nil || !sp.Ok {
			slog.Info("nowhere to flee", "err", err, "reason", sp.GetReason())
			return false
		}
		if _, err := s.api.GotoWait(ctx, sp.X, sp.Y, sp.Z, 2, 12); err != nil && ctx.Err() != nil {
			return false
		}
	}
	slog.Info("still chased after fleeing", "s", fleeMaxS)
	return false
}

// giveUp abandons a target we have not damaged for noProgress. When it keeps hitting us, staying
// meant dying: the attackers given up are not fought back (aggressor skips them) and no fight is
// running to flee from it (two deaths to a Defias Cutpurse, bench of 2026-10-13). So we run away;
// it gives up 30 yards from where it was pulled. It returns the fight's end reason ("" or "fled",
// "dead").
func (s *Service) giveUp(ctx context.Context, j *job, me *statepb.SelfInfo, t *statepb.Obj) string {
	s.abandon(t.Guid, "no damage dealt", t.Entry)
	if t.Target != me.Guid {
		return ""
	}
	tooFar, badFacing := s.melee.recent(5 * time.Second)
	slog.Info("unhittable attacker", "entry", t.Entry, "level", t.Level, "hp", pct(t.Hp, t.MaxHp), "dist", int(math.Hypot(float64(t.X-me.X), float64(t.Y-me.Y))),
		"dz", int(t.Z-me.Z), "not_in_range", tooFar, "bad_facing", badFacing, "flags", t.UnitFlags)
	att := s.onMe(ctx, me)
	if len(att) == 0 {
		att = []*statepb.Obj{t}
	}
	s.mark(&s.fledAt)
	if s.flee(ctx, j, me, att, 0) {
		return "dead"
	}
	if _, why := s.rest(ctx, j, 90, 70, 90); why == "dead" {
		return "dead"
	}
	return "fled"
}
