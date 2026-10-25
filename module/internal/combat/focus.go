package combat

import (
	statepb "wowagent/gen/statev1"
)

// Several attackers. Each one alive keeps hitting us, so the fastest way to lower the damage taken
// is to kill the one with the least health left first (Honorbuddy's Singular does the same); the
// area spells (Thunder Clap, Cleave, Sweeping Strikes) hit the others meanwhile, through the
// near_attackers variable of the priority tables.

const (
	areaRange = 8 // yards: Thunder Clap and the cleaving strikes
	// focusRatio: switch only to an attacker whose score is above the current target's divided by
	// this (with equal damage: less than this share of its health).
	focusRatio = 0.6
	// executeZone: a target this low (health %) is finished first.
	executeZone = 20
)

// Kill order (2026-10-25, idea of the user): the damage we take in all is the sum, over the attackers,
// of their damage per second times the time they stay alive; it is the smallest when they die in the
// order of their damage per second divided by the time to kill them (Smith's rule). The time to kill is
// their health divided by our damage per second, the same for all, so the order is by damage per
// second divided by health. The damage of each attacker is measured (dps.go); one not measured yet
// counts as the mean of the others (1 when none is), which gives the old rule: the weakest first.

// focusScore is the kill-order score of an attacker: its damage per second over its health.
func focusScore(a *statepb.Obj, dps map[uint64]float64) float64 {
	return attackerDPS(a.Guid, dps) / float64(max(a.Hp, 1))
}

func attackerDPS(guid uint64, dps map[uint64]float64) float64 {
	if d, ok := dps[guid]; ok && d > 0 {
		return d
	}
	if len(dps) == 0 {
		return 1
	}
	var sum float64
	for _, d := range dps {
		sum += d
	}
	return max(sum/float64(len(dps)), 1e-3)
}

// pickAttacker chooses whom to fight among the attackers: the best kill-order score within reach,
// else the nearest (the list comes from Nearby, nearest first). nil when the list is empty.
func pickAttacker(att []*statepb.Obj, dps map[uint64]float64) *statepb.Obj {
	var best *statepb.Obj
	for _, a := range att {
		if a.Dist <= areaRange && (best == nil || focusScore(a, dps) > focusScore(best, dps)) {
			best = a
		}
	}
	if best == nil && len(att) > 0 {
		best = att[0]
	}
	return best
}

// focusTarget returns the attacker to hit instead of t, nil to keep on t: one within reach whose
// score beats t's by 1/focusRatio (a near tie is not worth the lost swings and debuffs).
func focusTarget(t *statepb.Obj, att []*statepb.Obj, dps map[uint64]float64) *statepb.Obj {
	if t.MaxHp > 0 && pct(t.Hp, t.MaxHp) <= executeZone {
		return nil
	}
	var others []*statepb.Obj
	for _, a := range att {
		if a.Guid != t.Guid {
			others = append(others, a)
		}
	}
	b := pickAttacker(others, dps)
	if b == nil || b.Dist > areaRange || focusScore(b, dps)*focusRatio <= focusScore(t, dps) {
		return nil
	}
	return b
}
