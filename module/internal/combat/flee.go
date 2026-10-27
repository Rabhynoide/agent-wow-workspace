package combat

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"math"
	"math/rand"
	"time"

	pb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/tuning"
	"wowagent/internal/xpscore"
)

// Fleeing. The pull choice cannot stop a monster that joins a fight already engaged (wandering
// wolves: the deaths of the 2026-10-12 benches). During a fight, once a second, the expected loss
// still to come (xpscore.LossLeft: each attacker's learned single fight times the health it has
// left, plus the add cost) is compared with our health; above it minus fleeMargin, we run to a safe
// spot. Monsters run at 0.86 times our speed (creature_template.speed_run of the Elwynn monsters)
// and give up 30 yards from where they were pulled (CreatureLeashRadius), so running away works as
// long as no spell slows us.

// The flight margins are in journal/learn/tuning.json (internal/tuning, defaults and their reasons
// there); borderline cases are explored and learned (fleeDecision).
const (
	// fleeEvery: no new flight this soon after one (no flight loop). 90 s at first: both deaths of the
	// 2026-10-13 bench came 40-60 s after a successful flight, when the same wolves caught up again.
	fleeEvery = 15 * time.Second
	fleeMaxS  = 40 // running time before fighting again
	fleeDist  = 35 // yards per leg of the run (safe point search radius)
)

// onMe lists the units in combat that target us within 30 yards.
func (s *Service) onMe(ctx context.Context, me *statepb.SelfInfo) []*statepb.Obj {
	return s.onMeWithin(ctx, me, 30)
}

// chaseRadius: a flight is over when nothing targets us within it. At 30 yards, kobolds a few yards
// behind were not seen: "fled" after 5 s and 36 yards, caught resting 4 s later, dead (bench of
// 2026-10-26 13:19).
const chaseRadius = 60

func (s *Service) onMeWithin(ctx context.Context, me *statepb.SelfInfo, radius float32) []*statepb.Obj {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: radius, Max: 12, AliveOnly: true})
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
	return xpscore.LossLeft(levels, left, me.Level, me.ClassId, s.fights.Get(), s.tuneGet().AddCost)
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
	if !ok {
		s.setHolding(false, "")
		return false, loss, att
	}
	if hold, why := s.lateHold(me, att); hold {
		s.setHolding(true, why)
		return false, loss, att
	}
	s.setHolding(false, "")
	if hopeless, why := s.hopelessFlight(me, att, loss); hopeless {
		slog.Info("flight hopeless: staying", "why", why, "hp", pct(me.Hp, me.MaxHp), "loss", int(loss))
		return false, loss, att
	}
	return s.decide(me, att, loss), loss, att
}

// hopelessBelow: health (%) under which a hopeless flight is not tried (hopelessFlight).
const hopelessBelow = 75

// hopelessFlight: the flight would cost more health than we have (the damage we take times the time
// they keep hitting us, dpsMargin without its reserve): we stay, even when the fight is expected to
// kill us too (a hopeless stay at 41 % became a flight at 32 % once the fight's loss passed our
// health, dead 6 s later, bench of 2026-10-26 14:19). The 3 deaths of the bench of 2026-10-26 13:19
// were flights from kobolds and Prowlers that kept up (4 kobolds at 60 % with 57 % to lose, 2 at 35 %
// with 26 %); of all the flights logged, those started under 45 % of health against 4 monsters or
// more died 4 times in 5.
func (s *Service) hopelessFlight(me *statepb.SelfInfo, att []*statepb.Obj, loss float64) (bool, string) {
	t := s.tuneGet()
	hp := float64(pct(me.Hp, me.MaxHp))
	// Above hopelessBelow the flights still mostly escape (against 4 monsters or more: 34 in 41 from
	// 75 % and up, 12 in 19 below): the expected losses of crowds were too low before.
	if hp >= hopelessBelow {
		return false, ""
	}
	fresh := 0
	for _, a := range att {
		if a.MaxHp > 0 && 2*a.Hp > a.MaxHp {
			fresh++
		}
	}
	m, ok := s.dpsMargin(me, t, fresh, att)
	if !ok {
		return false, ""
	}
	cost := m - t.FleeReserve
	return cost >= hp, fmt.Sprintf("flight costs %.0f %%, the fight %.0f %%", cost, loss)
}

// lateHold: the fight is lost, but we stay while the weakest attacker in reach can still die before
// our health reaches the safe flight threshold (dpsMargin): what we kill before running is ours (XP,
// quest, one monster fewer at the next try), damage done to the survivors is lost (they heal going
// home). Idea of the user, 2026-10-25: 8 flights in 11 were decided in the first 3 s, at 84-96 % of
// health, before a blow was measured. Before the measure, we hold above flee_late_hold.
func (s *Service) lateHold(me *statepb.SelfInfo, att []*statepb.Obj) (bool, string) {
	t := s.tuneGet()
	if !t.FleeLate || me.MaxHp == 0 {
		return false, ""
	}
	hp := float64(pct(me.Hp, me.MaxHp))
	fresh := 0
	for _, a := range att {
		if a.MaxHp > 0 && 2*a.Hp > a.MaxHp {
			fresh++
		}
	}
	threshold, measured := s.dpsMargin(me, t, fresh, att) // measured in the fight, or learned
	if !measured {
		// Not against a group: 4 Defias took 29 % of health in the 2 s held to measure them, and the
		// flight that followed died (run of 2026-10-26 00:09).
		return hp >= t.FleeLateHold && len(att) < lateGroup, "measuring"
	}
	in, _ := s.incoming(me, att)
	ours, ok := s.ours(me)
	if !ok || ours <= 0 || in <= 0 {
		return false, ""
	}
	var weakest *statepb.Obj
	for _, a := range att {
		if a.Dist <= areaRange && (weakest == nil || a.Hp < weakest.Hp) {
			weakest = a
		}
	}
	if weakest == nil {
		return false, ""
	}
	toKill := float64(weakest.Hp) / ours                      // s
	left := (hp - threshold) / (100 * in / float64(me.MaxHp)) // s before the threshold
	return left > lateMargin*toKill, fmt.Sprintf("finishing %d: %.0f s to kill, %.0f s before the threshold", weakest.Entry, toKill, left)
}

// exploreHopeless: no exploration when the expected loss is above our health by more than this (%).
const exploreHopeless = 20

// lateGroup: from this many attackers, no hold before the damage is measured.
const lateGroup = 3

// lateMargin: hold only when the time left is this much longer than the time to kill (estimates).
const lateMargin = 1.2

// setHolding logs when a lost fight starts or stops being held (lateHold).
func (s *Service) setHolding(on bool, why string) {
	s.mu.Lock()
	was := s.holding
	s.holding = on
	s.mu.Unlock()
	if on && !was {
		slog.Info("late flee: holding", "why", why)
	} else if !on && was {
		slog.Info("late flee: released")
	}
}

// fleeDecision: the first borderline decision of an encounter (the rule says flee), logged with the
// outcome of its encounter ("flee outcome", encounter.go) so that the death rate of staying is learned.
type fleeDecision struct {
	key    string
	stayed bool
	how    string // rule, learned, explore
	hp     uint32
	slack  float64
	fresh  int // fresh attackers when it was taken
}

// decide: the rule says flee. The learned table may say stay (its death rate is low enough for the
// speedrun's costs); without enough fights, a borderline case where we are expected to survive is
// explored (stay) with probability Explore. An exploration holds for the rest of the encounter, a learned
// stay as long as the fight is expected to leave us some health (slack >= 0). Where fleeing itself
// kills often (tuning.FleeDeath), staying is explored without slack too.
func (s *Service) decide(me *statepb.SelfInfo, att []*statepb.Obj, loss float64) bool {
	t := s.tuneGet()
	s.mu.Lock()
	d := s.decision
	s.mu.Unlock()
	fresh := 0
	for _, a := range att {
		if a.MaxHp > 0 && 2*a.Hp > a.MaxHp {
			fresh++
		}
	}
	slack := float64(pct(me.Hp, me.MaxHp)) - loss
	// A learned stay holds too while we are still expected to survive: decided again each second, it
	// fell into a cell with few fights (a fourth kobold, health 75-85 %) and the rule fled. Of the
	// learned stays at 3 fresh attackers, all benches, 9 in 16 fled that way, while the explored stays
	// of those cells, held to the end, won 33 times in 37.
	// Held for the whole encounter now (encounter.go), an exploration stops holding when more fresh
	// monsters join than when it was taken (an encounter of series QR had 11 monsters).
	// Neither holds below the last safe moment to flee (dpsMargin, measured or learned): 3 Defias
	// Bandits, explored at 66 % with a loss of 80 %, killed us at 37 % with 2 left whose damage said
	// flee below 59 % (bench of 2026-10-26).
	unsafe := false
	if m, ok := s.dpsMargin(me, t, fresh, att); ok && float64(pct(me.Hp, me.MaxHp)) < m {
		unsafe = true
	}
	if d != nil && d.stayed && !unsafe && ((d.how == "explore" && fresh <= d.fresh) || slack >= 0) {
		return false
	}
	if d != nil && d.stayed && unsafe {
		slog.Info("flee decision released", "how", d.how, "hp", pct(me.Hp, me.MaxHp), "dps_pct", s.dpsPct(me))
	}
	key := tuning.Key(fresh, slack)
	flee, how := true, "rule"
	p := 0.0 // exploration probability, logged: 1 exploration in 8 eligible cases in game (series ES)
	if s.fleeTab != nil {
		if f, learned := tuning.Decide(s.fleeTab.Get(), key, t); learned {
			flee, how = f, "learned"
		} else if unsafe {
			p = 0
		} else if p = exploreProb(t, slack, t.ExploreFleeDeath > 0 && tuning.FleeDeath(s.fleeTab.Get(), key, t) >= t.ExploreFleeDeath); rand.Float64() < p {
			flee, how = false, "explore"
		}
	}
	if d == nil {
		s.mu.Lock()
		s.decision = &fleeDecision{key: key, stayed: !flee, how: how, hp: pct(me.Hp, me.MaxHp), slack: slack, fresh: fresh}
		nd := s.decision
		s.mu.Unlock()
		s.encounterDecision(nd) // judged when the encounter ends (encounter.go)
		slog.Info("flee decision", "key", key, "stay", !flee, "how", how, "hp", pct(me.Hp, me.MaxHp), "loss", int(loss), "fresh", fresh, "p", p,
			"dps_pct", s.dpsPct(me))
	}
	return flee
}

func (s *Service) tuneGet() tuning.Tuning {
	if s.tune == nil {
		return tuning.Defaults()
	}
	return s.tune.Get()
}

// losing tells whether the fight still to come against these attackers is expected to leave us less
// than fleeMargin of health, with its expected loss.
func (s *Service) losing(me *statepb.SelfInfo, att []*statepb.Obj) (bool, float64) {
	if len(att) == 0 {
		return false, 0
	}
	t := s.tuneGet()
	loss := s.lossLeft(me, att)
	margin := t.FleeMargin
	fresh := 0
	for _, a := range att {
		if a.Level >= me.Level+2 {
			margin += t.HighMargin
		}
		if a.MaxHp > 0 && 2*a.Hp > a.MaxHp {
			fresh++
		}
	}
	if fresh > 1 {
		margin += t.ChaseMargin * float64(fresh-1)
	}
	// Never later than the fixed margins: a monster that just joined has not hit us yet, so the
	// measured damage is low while it is fresh (staying in borderline cases is the learned table's job).
	if m, ok := s.dpsMargin(me, t, fresh, att); ok {
		margin = max(margin, m)
	}
	crowded := t.CrowdFlee > 0 && fresh >= max(t.CrowdFlee, s.pullSize.cap(t)+1)
	return (crowded && loss > t.CrowdLoss) || loss > float64(pct(me.Hp, me.MaxHp))-margin, loss
}

// dpsMargin is the flee margin from the damage we take (tuning flee_dps): what it costs during a
// flight (dps × (flee_exposure + flee_exposure_fresh per fresh attacker beyond the first) s, % of
// health) plus flee_reserve; ok false while not measured.
func (s *Service) dpsMargin(me *statepb.SelfInfo, t tuning.Tuning, fresh int, att []*statepb.Obj) (float64, bool) {
	if !t.FleeDPS || me.MaxHp == 0 {
		return 0, false
	}
	dps, how := s.incoming(me, att)
	if how == "" || dps <= 0 {
		return 0, false
	}
	exposure := t.FleeExposure + t.FleeExposureFresh*float64(max(fresh-1, 0))
	return 100*dps*exposure/float64(me.MaxHp) + t.FleeReserve, true
}

// dpsPct is the damage we take per second in % of our health (0 when not measured), for the logs.
func (s *Service) dpsPct(me *statepb.SelfInfo) float64 {
	dps, ok := s.dmg.rate()
	if !ok || me.MaxHp == 0 {
		return 0
	}
	return math.Round(1000*dps/float64(me.MaxHp)) / 10
}

// crowd tells whether at least n attackers (tuning crowd_flee, 0 = no crowd rule) have more than half
// their health.
func crowd(att []*statepb.Obj, n int) bool {
	if n <= 0 {
		return false
	}
	fresh := 0
	for _, a := range att {
		if a.MaxHp > 0 && 2*a.Hp > a.MaxHp {
			fresh++
		}
	}
	return fresh >= n
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
	slog.Info("fleeing", "hp", pct(me.Hp, me.MaxHp), "loss_left", int(loss), "attackers", entries, "crowd", crowd(att, s.tuneGet().CrowdFlee),
		"zone", [2]int{int(z.x), int(z.y)}, "way", way, "legs", len(legs), "dps_pct", s.dpsPct(me))
	j.Update(func(st *pb.CombatStatus) { st.Doing = "fleeing" })
	s.trail.pause(true)
	end := -1
	defer func() { s.trail.cut(end) }()
	started := time.Now()
	// One direction for the whole flight, away from the attackers, and no stop: the target moves ahead
	// before we reach it (move.goto re-targets the running walk). Legs that stopped at each point and
	// safe points chosen again at each leg ran back towards the chasers: 50 yards in 15 s, 21 blows taken,
	// dead, against 3 Prowlers at 76 % and 3 boars at 91 % (run of 2026-10-26 07:56).
	dir, idle := s.fleeDirection(ctx, me, awayFrom(me, att))
	if len(legs) > 0 && !sameWay(me, legs[min(len(legs)-1, 1)], dir) {
		legs = nil // the trail turns back towards them
	}
	if len(legs) > 0 && trailNearIdle(me, legs, idle) {
		slog.Info("flight trail passes idle monsters: straight way")
		legs = nil
	}
	if s.tuneGet().FleeDanger { // the way scored on the danger map (fleemap.go) instead
		trailLegs, _ := s.trail.back(me.Map, me.X, me.Y)
		var how string
		legs, dir, how = s.chooseFlight(ctx, me, att, trailLegs)
		if how != "trail" {
			idx = nil
		} else {
			_, idx = s.trail.back(me.Map, me.X, me.Y)
		}
	}
	turn := 0 // index in fleeTurns
	var target *[3]float32
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
		if len(s.onMeWithin(ctx, me, chaseRadius)) > 0 {
			clear = 0
		} else if clear++; clear >= 2 {
			slog.Info("fled", "hp", pct(me.Hp, me.MaxHp), "s", int(time.Since(started).Seconds()), "legs", leg, "dist", int(dist2(me.X, me.Y, z.x, z.y)))
			return false
		}
		// The next point: along the trail while it goes our way, else straight ahead; a new one when
		// the current one is near (before we stop on it).
		if target == nil || dist2(me.X, me.Y, target[0], target[1]) < fleeAhead {
			target = nil
			for leg < len(legs) && dist2(me.X, me.Y, legs[leg].x, legs[leg].y) < fleeAhead {
				if leg < len(idx) {
					end = idx[leg]
				}
				leg++
			}
			if leg < len(legs) {
				target = &[3]float32{legs[leg].x, legs[leg].y, legs[leg].z}
			}
			for target == nil && turn < len(fleeTurns) {
				if p, ok := s.fleePoint(ctx, me, dir, fleeTurns[turn]); ok {
					target = &p
				} else {
					turn++ // a cliff, water or a wall that way
				}
			}
			if target == nil {
				slog.Info("nowhere to flee", "s", int(time.Since(started).Seconds()))
				return false
			}
		}
		if _, err := s.api.Goto(ctx, target[0], target[1], target[2], 2, 1); err != nil && ctx.Err() != nil {
			return false
		}
	}
	slog.Info("still chased after fleeing", "s", fleeMaxS)
	return false
}

// Flight geometry: a new point when the current one is fleeAhead yards away, points fleeStep yards
// ahead, turned by fleeTurns (degrees) when the straight way is blocked.
const (
	fleeAhead = 8
	fleeStep  = 30
)

var fleeTurns = []float64{0, 30, -30, 60, -60, 90, -90, 120, -120}

// awayFrom is the unit vector from the attackers' centre to us (2D); when they stand on us, the way
// we face turned around.
func awayFrom(me *statepb.SelfInfo, att []*statepb.Obj) [2]float64 {
	var cx, cy float64
	for _, a := range att {
		cx += float64(a.X)
		cy += float64(a.Y)
	}
	if len(att) > 0 {
		cx /= float64(len(att))
		cy /= float64(len(att))
	}
	dx, dy := float64(me.X)-cx, float64(me.Y)-cy
	if l := math.Hypot(dx, dy); len(att) > 0 && l > 1 {
		return [2]float64{dx / l, dy / l}
	}
	return [2]float64{-math.Cos(float64(me.O)), -math.Sin(float64(me.O))}
}

// fleeDirection turns the flight direction (within ±90°) to keep the run away from idle monsters:
// the chasers call for help every 3 s within 10 yards of where they are (Creature::CallAssistance), so
// a run past a camp brings it along (the pack of 7 wolves, 2026-10-26). Each idle monster closer than
// fleeClear yards to the first fleeLook yards of a way counts against it; the straightest way wins ties.
func (s *Service) fleeDirection(ctx context.Context, me *statepb.SelfInfo, base [2]float64) ([2]float64, []*statepb.Obj) {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: fleeLook + fleeClear, Max: 40, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return base, nil
	}
	var idle []*statepb.Obj
	for _, o := range r.Objs {
		if o.Target == 0 && o.UnitFlags&unitInCombat == 0 && math.Abs(float64(o.Z-me.Z)) <= 10 {
			idle = append(idle, o)
		}
	}
	return bestFleeWay(me, base, idle), idle
}

// trailNearIdle tells whether the first fleeLook yards of the trail pass within fleeClear of an idle
// monster: back the way we came past the Fargodeep mine entrance, 4 flights in 4 met the miners there
// and died (bench of 2026-10-26 14:19, up to 9 monsters).
func trailNearIdle(me *statepb.SelfInfo, legs []trailPt, idle []*statepb.Obj) bool {
	ax, ay, done := me.X, me.Y, float32(0)
	for _, l := range legs {
		bx, by := l.x, l.y
		d := dist2(ax, ay, bx, by)
		if left := fleeLook - done; d > left { // the leg cut at fleeLook
			bx, by = ax+(bx-ax)*left/d, ay+(by-ay)*left/d
		}
		for _, o := range idle {
			if segDist(o.X, o.Y, ax, ay, bx, by) < fleeClear {
				return true
			}
		}
		if done += d; done >= fleeLook {
			return false
		}
		ax, ay = bx, by
	}
	return false
}

// bestFleeWay: see fleeDirection.
func bestFleeWay(me *statepb.SelfInfo, base [2]float64, idle []*statepb.Obj) [2]float64 {
	best, bestScore := base, math.MaxFloat64
	for _, deg := range []float64{0, 30, -30, 60, -60, 90, -90} {
		d := turned(base, deg)
		ex, ey := me.X+float32(d[0]*fleeLook), me.Y+float32(d[1]*fleeLook)
		score := math.Abs(deg) / 90 // the straight way first
		for _, o := range idle {
			if segDist(o.X, o.Y, me.X, me.Y, ex, ey) < fleeClear {
				score += 3
			}
		}
		if score < bestScore {
			best, bestScore = d, score
		}
	}
	return best
}

// fleeLook: the part of a flight way checked for idle monsters; fleeClear: how far from it they must
// stand (the assistance range plus a margin).
const (
	fleeLook  = 45
	fleeClear = assistRange + 4
)

func turned(d [2]float64, deg float64) [2]float64 {
	a := deg * math.Pi / 180
	return [2]float64{d[0]*math.Cos(a) - d[1]*math.Sin(a), d[0]*math.Sin(a) + d[1]*math.Cos(a)}
}

// segDist is the 2D distance from (px, py) to the segment a-b.
func segDist(px, py, ax, ay, bx, by float32) float32 {
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	t := float32(0)
	if l2 > 0 {
		t = max(0, min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
	}
	return dist2(px, py, ax+t*dx, ay+t*dy)
}

// sameWay tells whether a trail point lies within 60 degrees of the flight direction.
func sameWay(me *statepb.SelfInfo, p trailPt, dir [2]float64) bool {
	dx, dy := float64(p.x-me.X), float64(p.y-me.Y)
	l := math.Hypot(dx, dy)
	return l < 1 || (dx*dir[0]+dy*dir[1])/l >= 0.5
}

// fleePoint is a walkable point about fleeStep yards from us in the flight direction turned by deg
// (move.path towards it, ending at the closest walkable point within 12 yards); ok false when the walk
// would not get us at least half of the way.
func (s *Service) fleePoint(ctx context.Context, me *statepb.SelfInfo, dir [2]float64, deg float64) ([3]float32, bool) {
	d := turned(dir, deg)
	x, y := me.X+float32(d[0]*fleeStep), me.Y+float32(d[1]*fleeStep)
	res, err := s.api.PathFromSnap(ctx, me.Map, me.X, me.Y, me.Z, x, y, me.Z, 12)
	if err != nil || res.Error != "" || res.End == nil || dist2(me.X, me.Y, res.End.X, res.End.Y) < fleeStep/2 ||
		res.Length > 2*fleeStep {
		return [3]float32{}, false
	}
	return [3]float32{res.End.X, res.End.Y, res.End.Z}, true
}

// giveUp abandons a target we have not damaged for noProgress. When it keeps hitting us, staying
// meant dying: the attackers given up are not fought back (aggressor skips them) and no fight is
// running to flee from it (two deaths to a Defias Cutpurse, bench of 2026-10-13). So we run away;
// it gives up 30 yards from where it was pulled. It returns the fight's end reason ("", "unhittable"
// or "dead"): not "fled", the quest goes on (13 mine quests postponed for such a run, 2026-10-21/23).
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
	s.encounterEnd("fled")
	if _, why := s.rest(ctx, j, 90, 70, 90); why == "dead" {
		return "dead"
	}
	return unhittable
}

// giveUpVertical abandons a target on another floor that we cannot reach, without fleeing: its blows
// reach us rarely (94 % health after 30 s against it, RB75 series run 3) and the flight is what killed
// us. We step out of its reach so that it follows us onto our floor (then aggressor fights it back:
// atMelee) or stays below.
func (s *Service) giveUpVertical(ctx context.Context, me *statepb.SelfInfo, t *statepb.Obj) string {
	s.abandon(t.Guid, "on another floor, not reached", t.Entry)
	if sp, err := s.api.SafePoint(ctx, me.X, me.Y, me.Z, 15, true); err == nil && sp.Ok {
		_, _ = s.api.GotoWait(ctx, sp.X, sp.Y, sp.Z, 2, 8)
	}
	return unhittable
}

// unhittable ends a fight against a target given up (giveUp): another one is picked, the job goes on.
const unhittable = "unhittable"

// exploreProb is the probability to explore a borderline case not learned yet: only when the fight is
// expected to leave us more than ExploreMinSlack % of health, or where fleeing kills often.
func exploreProb(t tuning.Tuning, slack float64, fleeKills bool) float64 {
	// Never a hopeless stay: even where fleeing often kills, a fight expected to cost 117 % of a 61 %
	// health (slack -56, 3 fresh attackers) was explored and died (run of 2026-10-26 00:09).
	if slack < -exploreHopeless {
		return 0
	}
	if slack > max(t.ExploreMinSlack, 0) || fleeKills {
		return exploreRate(t, slack)
	}
	return 0
}

// exploreRate is the probability to explore a borderline case: tuning ExploreSafe when the fight is
// expected to leave us ExploreSafeSlack % of health or more, else Explore.
func exploreRate(t tuning.Tuning, slack float64) float64 {
	if t.ExploreSafe > t.Explore && slack >= t.ExploreSafeSlack {
		return t.ExploreSafe
	}
	return t.Explore
}
