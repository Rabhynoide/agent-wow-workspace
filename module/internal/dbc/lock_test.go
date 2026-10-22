package dbc

import "testing"

func TestLockOpenSpell(t *testing.T) {
	locks, err := LoadLocks(dir)
	if err != nil {
		t.Skip("no Lock.dbc")
	}
	for id, want := range map[uint32]uint32{
		57:   3365,  // Battered Chest: open or treasure
		43:   22810, // open kneeling
		29:   0,     // herbalism (Silverleaf)
		38:   0,     // mining (Copper Vein)
		1439: 0,     // a key
	} {
		if got := locks[id].OpenSpell(); got != want {
			t.Errorf("lock %d: open spell %d, want %d", id, got, want)
		}
	}
}
