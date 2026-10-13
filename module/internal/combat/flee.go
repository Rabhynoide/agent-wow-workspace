package combat

import (
	"context"
	"log/slog"
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
	return loss > float64(pct(me.Hp, me.MaxHp))-fleeMargin, loss
}

// flee runs from safe spot to safe spot until nothing targets us (or fleeMaxS), and reports whether
// we died on the way. The attackers are skipped by the pull choice for a while, not given up: we
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
	slog.Info("fleeing", "hp", pct(me.Hp, me.MaxHp), "loss_left", int(loss), "attackers", entries, "zone", [2]int{int(z.x), int(z.y)})
	j.Update(func(st *pb.CombatStatus) { st.Doing = "fleeing" })
	started := time.Now()
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
		if len(s.onMe(ctx, me)) == 0 {
			slog.Info("fled", "hp", pct(me.Hp, me.MaxHp), "s", int(time.Since(started).Seconds()))
			return false
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
