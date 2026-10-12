package dbc

import (
	"math"
	"path/filepath"
)

// SpellInfo is what a client needs to know before casting (Spell.dbc + SpellRange.dbc, 3.3.5a).
type SpellInfo struct {
	ID               uint32
	Category         uint32 // shared cooldown category
	CasterAuraState  uint32 // aura state required on the caster (0 = none)
	TargetAuraState  uint32 // aura state required on the target
	CastTimeIndex    uint32 // 1 = instant
	Recovery         uint32 // cooldown, ms
	CategoryRecovery uint32 // category cooldown, ms
	PowerType        int32  // 0 mana, 1 rage, 3 energy, -2 health
	Cost             uint32 // in the power's stored unit (rage x10)
	CostPct          uint32 // percent of base mana
	MinRange         float32
	MaxRange         float32
	Melee            bool   // range type "combat range": reach + target size
	GCD              uint32 // StartRecoveryTime, ms (1500 for most spells)
	NeedsWeapon      bool   // EquippedItemClass = weapon (Rend, Heroic Strike...)
}

// Spell.dbc field indexes (verified on Charge, Rend, Heroic Strike, Mind Blast, Shadow Word: Pain).
const (
	siCategory         = 1
	siCasterAuraState  = 20
	siTargetAuraState  = 21
	siCastTimeIndex    = 28
	siRecovery         = 29
	siCategoryRecovery = 30
	siPowerType        = 41
	siManaCost         = 42
	siRangeIndex       = 46
	siEquippedClass    = 68
	siManaCostPct      = 204
	siStartRecovery    = 206
)

// LoadSpellInfo reads the casting data of every spell.
func LoadSpellInfo(dir string) (map[uint32]SpellInfo, error) {
	ranges := map[uint32][3]float32{} // min, max, type
	if r, err := Open(filepath.Join(dir, "SpellRange.dbc")); err == nil {
		for i := 0; i < r.Records; i++ {
			ranges[r.U32(i, 0)] = [3]float32{math.Float32frombits(r.U32(i, 1)), math.Float32frombits(r.U32(i, 3)), float32(r.U32(i, 5))}
		}
	} else {
		return nil, err
	}
	f, err := Open(filepath.Join(dir, "Spell.dbc"))
	if err != nil {
		return nil, err
	}
	out := make(map[uint32]SpellInfo, f.Records)
	for r := 0; r < f.Records; r++ {
		s := SpellInfo{
			ID: f.U32(r, 0), Category: f.U32(r, siCategory),
			CasterAuraState: f.U32(r, siCasterAuraState), TargetAuraState: f.U32(r, siTargetAuraState),
			CastTimeIndex: f.U32(r, siCastTimeIndex), Recovery: f.U32(r, siRecovery), CategoryRecovery: f.U32(r, siCategoryRecovery),
			PowerType: int32(f.U32(r, siPowerType)), Cost: f.U32(r, siManaCost), CostPct: f.U32(r, siManaCostPct),
			GCD: f.U32(r, siStartRecovery), NeedsWeapon: f.U32(r, siEquippedClass) == 2,
		}
		if rg, ok := ranges[f.U32(r, siRangeIndex)]; ok {
			s.MinRange, s.MaxRange, s.Melee = rg[0], rg[1], rg[2] == 1
		}
		out[s.ID] = s
	}
	return out, nil
}
