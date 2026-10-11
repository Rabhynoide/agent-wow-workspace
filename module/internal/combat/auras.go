package combat

import (
	"math"

	statepb "wowagent/gen/statev1"
)

// auraLeft tells whether one of the spells (any rank) is among the auras, cast by us when mine is set,
// and its time left in milliseconds (MaxUint32 for an aura without duration).
func auraLeft(auras []*statepb.Aura, spells []uint32, mine bool) (uint32, bool) {
	for _, a := range auras {
		if mine && !a.Mine {
			continue
		}
		for _, id := range spells {
			if a.Spell == id {
				if a.LeftMs == 0 {
					return math.MaxUint32, true
				}
				return a.LeftMs, true
			}
		}
	}
	return 0, false
}

// hasAura is auraLeft without the time.
func hasAura(auras []*statepb.Aura, spells ...uint32) bool {
	_, ok := auraLeft(auras, spells, false)
	return ok
}

// auraState tells whether the aura state n (1-based, Spell.dbc numbering) is set in UNIT_FIELD_AURASTATE.
func auraState(field, n uint32) bool { return n > 0 && field&(1<<(n-1)) != 0 }

// needsRefresh: the aura is missing, or ours expires within ms.
func needsRefresh(auras []*statepb.Aura, spells []uint32, ms uint32) bool {
	left, ok := auraLeft(auras, spells, true)
	return !ok || left < ms
}
