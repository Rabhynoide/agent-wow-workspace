// Package priest holds the priest class data used by combat and progress: spell ranks
// (highest first) and the leveling talent build. Both are checked against Spell.dbc/Talent.dbc in tests.
package priest

const ClassID = 5

// Spell ranks, highest first.
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
	SpellGroups = map[string][]uint32{
		"Smite": Smite, "Lesser Heal": LesserHeal, "Heal": Heal, "Power Word: Shield": Shield, "Renew": Renew,
		"Shadow Word: Pain": Pain, "Mind Blast": MindBlast, "Power Word: Fortitude": Fortitude, "Inner Fire": InnerFire, "Shoot": Shoot,
	}
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

// TalentStep raises a talent (by its spell name) to Rank.
type TalentStep struct {
	Name string
	Rank int
}

// Build is the Shadow leveling build in learning order (one point per level from level 10, 71 points
// at level 80). Order from the Highground Gaming / Icy Veins WotLK Classic leveling guides; the first
// 30 points (levels 10-40) are the ones that matter for solo questing. Replayed against Talent.dbc
// by TestBuild (tier requirements, prerequisites, maximum ranks).
var Build = []TalentStep{
	{"Spirit Tap", 3}, // 10-12
	{"Darkness", 2},   // 13-14
	{"Shadow Focus", 3},
	{"Darkness", 4},
	{"Mind Flay", 1}, // 20
	{"Darkness", 5},
	{"Improved Shadow Word: Pain", 2},
	{"Improved Mind Blast", 1},
	{"Shadow Weaving", 3}, // 25-27
	{"Shadow Reach", 2},
	{"Improved Mind Blast", 5}, // 30-33
	{"Vampiric Embrace", 1},
	{"Mind Melt", 2},
	{"Improved Devouring Plague", 3},
	{"Shadowform", 1}, // 40
	{"Improved Spirit Tap", 2},
	{"Shadow Power", 2},
	{"Misery", 3},
	{"Improved Shadowform", 2},
	{"Vampiric Touch", 1}, // 50
	{"Pain and Suffering", 3},
	{"Psychic Horror", 1},
	{"Twisted Faith", 5},
	{"Dispersion", 1}, // 60
	// Discipline for the last points.
	{"Twin Disciplines", 5},
	{"Improved Inner Fire", 3},
	{"Improved Power Word: Fortitude", 2},
	{"Meditation", 3},
	{"Focused Mind", 3},
	{"Improved Vampiric Embrace", 2},
	{"Shadow Power", 4}, // 79-80
}
