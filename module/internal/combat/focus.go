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
	// focusRatio: switch only to an attacker with less than this share of the current target's
	// health (a near tie is not worth the lost swings and debuffs).
	focusRatio = 0.6
	// executeZone: a target this low (health %) is finished first.
	executeZone = 20
)

// pickAttacker chooses whom to fight among the attackers: the one with the least health within
// reach, else the nearest (the list comes from Nearby, nearest first). nil when the list is empty.
func pickAttacker(att []*statepb.Obj) *statepb.Obj {
	var best *statepb.Obj
	for _, a := range att {
		if a.Dist <= areaRange && (best == nil || a.Hp < best.Hp) {
			best = a
		}
	}
	if best == nil && len(att) > 0 {
		best = att[0]
	}
	return best
}

// focusTarget returns the attacker to hit instead of t, nil to keep on t.
func focusTarget(t *statepb.Obj, att []*statepb.Obj) *statepb.Obj {
	if t.MaxHp > 0 && pct(t.Hp, t.MaxHp) <= executeZone {
		return nil
	}
	var others []*statepb.Obj
	for _, a := range att {
		if a.Guid != t.Guid {
			others = append(others, a)
		}
	}
	b := pickAttacker(others)
	if b == nil || b.Dist > areaRange || float64(b.Hp) >= focusRatio*float64(t.Hp) {
		return nil
	}
	return b
}
