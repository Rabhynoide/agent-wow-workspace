package dbc

import "path/filepath"

// Lock is a Lock.dbc row (3.3.5a): ID, Type[8], Index[8], Skill[8], Action[8]. Type 1 = an item (key),
// 2 = a lock type (LockType.dbc: 1 lockpicking, 2 herbalism, 3 mining, 5 open, 6 treasure...).
type Lock struct {
	Type, Index, Skill [8]uint32
}

// Lock types any character opens without a skill, and the spell the client casts for each (an
// OPEN_LOCK effect whose misc value is the lock type).
var openSpells = map[uint32]uint32{
	5:  3365,  // Open: Battered Chest, Food Crate
	10: 6247,  // Quick Open
	13: 22810, // Open Kneeling ("Opening - No Text")
	17: 21651, // Slow Open
}

// OpenSpell is the spell that opens this lock without a key, profession or lockpicking (0: none).
func (l Lock) OpenSpell() uint32 {
	for i := range l.Type {
		if l.Type[i] == 2 && l.Skill[i] == 0 {
			if sp := openSpells[l.Index[i]]; sp != 0 {
				return sp
			}
		}
	}
	return 0
}

// LoadLocks reads Lock.dbc.
func LoadLocks(dir string) (map[uint32]Lock, error) {
	f, err := Open(filepath.Join(dir, "Lock.dbc"))
	if err != nil {
		return nil, err
	}
	out := make(map[uint32]Lock, f.Records)
	for r := 0; r < f.Records; r++ {
		var l Lock
		for i := 0; i < 8; i++ {
			l.Type[i], l.Index[i], l.Skill[i] = f.U32(r, 1+i), f.U32(r, 9+i), f.U32(r, 17+i)
		}
		out[f.U32(r, 0)] = l
	}
	return out, nil
}
