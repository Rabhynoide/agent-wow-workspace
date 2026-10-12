// Package priest holds the priest spell ranks used by combat (highest first), checked against
// Spell.dbc in tests. The talent build is in journal/classes/priest.yaml.
package priest

const ClassID = 5

// Spell ranks, highest first.
// WeakenedSoul is the debuff Power Word: Shield leaves (no new shield while it lasts).
const WeakenedSoul = 6788

var (
	Smite       = []uint32{48123, 48122, 25364, 25363, 10934, 10933, 6060, 1004, 984, 598, 591, 585}
	LesserHeal  = []uint32{2053, 2052, 2050}
	Heal        = []uint32{6064, 6063, 2055, 2054}
	Shield      = []uint32{48066, 48065, 25218, 25217, 10901, 10900, 10899, 10898, 6066, 6065, 3747, 600, 592, 17}
	Renew       = []uint32{48068, 48067, 25222, 25221, 25315, 10929, 10928, 10927, 6078, 6077, 6076, 6075, 6074, 139}
	Pain        = []uint32{48125, 48124, 25368, 25367, 10894, 10893, 10892, 2767, 992, 970, 594, 589}
	MindBlast   = []uint32{48127, 48126, 25375, 25372, 10947, 10946, 10945, 8106, 8105, 8104, 8103, 8102, 8092}
	Fortitude   = []uint32{48161, 25389, 10938, 10937, 2791, 1245, 1244, 1243}
	InnerFire   = []uint32{48168, 48040, 25431, 10952, 10951, 1006, 602, 7128, 588}
	Shoot       = []uint32{5019}
	Scream      = []uint32{10890, 10888, 8124, 8122}
	Desperate   = []uint32{48173, 48172, 25437, 19243, 19242, 19241, 19240, 19238, 19236}
	Fade        = []uint32{586}
	SpellGroups = map[string][]uint32{
		"Smite": Smite, "Lesser Heal": LesserHeal, "Heal": Heal, "Power Word: Shield": Shield, "Renew": Renew,
		"Shadow Word: Pain": Pain, "Mind Blast": MindBlast, "Power Word: Fortitude": Fortitude, "Inner Fire": InnerFire, "Shoot": Shoot,
		"Psychic Scream": Scream, "Desperate Prayer": Desperate, "Fade": Fade,
	}

	// Auras by name for the priority table (journal/classes/priest.yaml) that are not spells we cast.
	Auras = map[string][]uint32{"Weakened Soul": {WeakenedSoul}, "Spirit Tap": {15271}}
)

// Best returns the highest known rank, or 0.
func Best(known map[uint32]bool, ranks []uint32) uint32 {
	for _, id := range ranks {
		if known[id] {
			return id
		}
	}
	return 0
}
