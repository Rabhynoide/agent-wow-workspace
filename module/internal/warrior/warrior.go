// Package warrior holds the warrior spell ranks used by combat (highest first), checked against
// Spell.dbc in tests. The leveling build is in journal/classes/warrior.yaml.
package warrior

const ClassID = 1

// Spell ranks, highest first.
var (
	BattleShout    = []uint32{47436, 2048, 25289, 11551, 11550, 11549, 6192, 5242, 6673}
	Charge         = []uint32{11578, 6178, 100}
	Rend           = []uint32{47465, 46845, 25208, 11574, 11573, 11572, 6548, 6547, 6546, 772}
	HeroicStrike   = []uint32{47450, 47449, 30324, 29707, 25286, 11567, 11566, 11565, 11564, 1608, 285, 284, 78}
	ThunderClap    = []uint32{47502, 47501, 25264, 11581, 11580, 8205, 8204, 8198, 6343}
	VictoryRush    = []uint32{34428}
	Overpower      = []uint32{7384}
	Bloodrage      = []uint32{2687}
	Execute        = []uint32{47471, 47470, 25236, 25234, 20662, 20661, 20660, 20658, 5308}
	MortalStrike   = []uint32{47486, 47485, 30330, 25248, 21553, 21552, 21551, 12294}
	Slam           = []uint32{47475, 47474, 25242, 25241, 11605, 11604, 8820, 1464}
	SweepingStrike = []uint32{12328}
	Bladestorm     = []uint32{46924}

	SpellGroups = map[string][]uint32{
		"Battle Shout": BattleShout, "Charge": Charge, "Rend": Rend, "Heroic Strike": HeroicStrike, "Thunder Clap": ThunderClap,
		"Victory Rush": VictoryRush, "Overpower": Overpower, "Bloodrage": Bloodrage, "Execute": Execute, "Mortal Strike": MortalStrike,
		"Slam": Slam, "Sweeping Strikes": SweepingStrike, "Bladestorm": Bladestorm,
	}
)
