// Package warrior holds the warrior spell ranks used by combat (highest first), checked against
// Spell.dbc in tests. The leveling build is in journal/classes/warrior.yaml.
package warrior

const ClassID = 1

// Spell ranks, highest first.
// Proc auras (buffs on the warrior) and aura states (Spell.dbc CasterAuraState/TargetAuraState).
const (
	TasteForBlood      = 60503 // Overpower usable (talent proc)
	SuddenDeath        = 52437 // Execute usable at any health (talent proc)
	StateVictoryRush   = 10    // on us: Victory Rush usable
	StateHealthBelow20 = 2     // on the target: Execute usable
)

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
	Cleave         = []uint32{47520, 47519, 25231, 20569, 11609, 11608, 7369, 845}
	Bladestorm     = []uint32{46924}
	IntimShout     = []uint32{5246}
	Retaliation    = []uint32{20230}
	EnragedRegen   = []uint32{55694}
	BerserkerRage  = []uint32{18499}

	SpellGroups = map[string][]uint32{
		"Battle Shout": BattleShout, "Charge": Charge, "Rend": Rend, "Heroic Strike": HeroicStrike, "Thunder Clap": ThunderClap,
		"Victory Rush": VictoryRush, "Overpower": Overpower, "Bloodrage": Bloodrage, "Execute": Execute, "Mortal Strike": MortalStrike,
		"Slam": Slam, "Sweeping Strikes": SweepingStrike, "Bladestorm": Bladestorm, "Intimidating Shout": IntimShout,
		"Retaliation": Retaliation, "Enraged Regeneration": EnragedRegen, "Berserker Rage": BerserkerRage,
		"Cleave": Cleave,
	}

	// Auras by name for the priority table (journal/classes/warrior.yaml), when the aura is not the
	// spell itself: procs and the buff Bloodrage leaves.
	Auras = map[string][]uint32{
		"Taste for Blood": {TasteForBlood}, "Sudden Death": {SuddenDeath}, "Victorious": {32216}, "Bloodrage": {29131},
	}
)
