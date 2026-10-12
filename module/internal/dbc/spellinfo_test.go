package dbc

import (
	"os"
	"testing"
)

func TestSpellInfo(t *testing.T) {
	m, err := LoadSpellInfo(dir)
	if os.IsNotExist(err) || (err != nil && m == nil) {
		t.Skip("no Spell.dbc / SpellRange.dbc")
	}
	charge, rend, hs, vr, blast := m[100], m[772], m[78], m[34428], m[8092]
	if charge.MinRange != 8 || charge.MaxRange != 25 || charge.CategoryRecovery != 15000 || charge.PowerType != 1 {
		t.Errorf("Charge %+v", charge)
	}
	if !rend.Melee || rend.Cost != 100 || rend.GCD != 1500 {
		t.Errorf("Rend %+v", rend)
	}
	if hs.Cost != 150 || hs.GCD != 0 {
		t.Errorf("Heroic Strike (no global cooldown) %+v", hs)
	}
	if !rend.NeedsWeapon || !hs.NeedsWeapon || charge.NeedsWeapon {
		t.Errorf("weapon requirement: rend %v, heroic strike %v, charge %v", rend.NeedsWeapon, hs.NeedsWeapon, charge.NeedsWeapon)
	}
	if vr.CasterAuraState != 10 {
		t.Errorf("Victory Rush %+v", vr)
	}
	if blast.CategoryRecovery != 8000 || blast.CostPct != 17 || blast.MaxRange != 30 {
		t.Errorf("Mind Blast %+v", blast)
	}
}
