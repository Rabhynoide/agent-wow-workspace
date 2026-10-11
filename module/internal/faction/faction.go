// Package faction reads FactionTemplate.dbc and tells how two faction templates react to each other
// (FactionTemplateEntry::IsFriendlyTo / IsHostileTo of the server).
package faction

import (
	"path/filepath"

	"wowagent/internal/dbc"
)

// Template is one FactionTemplate.dbc row.
type Template struct {
	ID, Faction, Group, FriendGroup, EnemyGroup uint32
	Enemies, Friends                            [4]uint32
}

// Table maps a template id to its row.
type Table map[uint32]Template

// Load reads <dir>/FactionTemplate.dbc.
func Load(dir string) (Table, error) {
	f, err := dbc.Open(filepath.Join(dir, "FactionTemplate.dbc"))
	if err != nil {
		return nil, err
	}
	out := make(Table, f.Records)
	for r := 0; r < f.Records; r++ {
		t := Template{ID: f.U32(r, 0), Faction: f.U32(r, 1), Group: f.U32(r, 3), FriendGroup: f.U32(r, 4), EnemyGroup: f.U32(r, 5)}
		for i := 0; i < 4; i++ {
			t.Enemies[i], t.Friends[i] = f.U32(r, 6+i), f.U32(r, 10+i)
		}
		out[t.ID] = t
	}
	return out, nil
}

// Friendly mirrors IsFriendlyTo: such units cannot be attacked.
func Friendly(a, p Template) bool {
	if a.ID == p.ID {
		return true
	}
	for _, e := range a.Enemies {
		if e != 0 && e == p.Faction {
			return false
		}
	}
	for _, f := range a.Friends {
		if f != 0 && f == p.Faction {
			return true
		}
	}
	return a.FriendGroup&p.Group != 0 || a.Group&p.FriendGroup != 0
}

// Hostile mirrors IsHostileTo: such units attack on sight.
func Hostile(a, p Template) bool {
	if a.ID == p.ID {
		return false
	}
	for _, e := range a.Enemies {
		if e != 0 && e == p.Faction {
			return true
		}
	}
	for _, f := range a.Friends {
		if f != 0 && f == p.Faction {
			return false
		}
	}
	return a.EnemyGroup&p.Group != 0
}

// Reaction is "hostile", "friendly" or "neutral" (attackable, does not attack first); "" when a
// template is unknown.
func (t Table) Reaction(unit, player uint32) string {
	a, ok1 := t[unit]
	p, ok2 := t[player]
	switch {
	case !ok1 || !ok2:
		return ""
	case Hostile(a, p):
		return "hostile"
	case Friendly(a, p):
		return "friendly"
	}
	return "neutral"
}
