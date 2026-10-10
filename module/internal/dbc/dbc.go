// Package dbc reads the WotLK 3.3.5 client data files (WDBC) used by the modules.
package dbc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// File is a loaded WDBC table.
type File struct {
	Records, Fields, RecordSize int
	data, strings               []byte
}

func Open(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 20 || string(b[:4]) != "WDBC" {
		return nil, fmt.Errorf("%s: not a WDBC file", path)
	}
	u := func(o int) int { return int(binary.LittleEndian.Uint32(b[o:])) }
	f := &File{Records: u(4), Fields: u(8), RecordSize: u(12)}
	end := 20 + f.Records*f.RecordSize
	if end+u(16) > len(b) {
		return nil, fmt.Errorf("%s: truncated", path)
	}
	f.data, f.strings = b[20:end], b[end:end+u(16)]
	return f, nil
}

func (f *File) U32(rec, field int) uint32 {
	return binary.LittleEndian.Uint32(f.data[rec*f.RecordSize+4*field:])
}

func (f *File) Str(rec, field int) string {
	o := int(f.U32(rec, field))
	if o >= len(f.strings) {
		return ""
	}
	s := f.strings[o:]
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return string(s)
}

// Spell.dbc field indexes (3.3.5a).
const (
	spEffect     = 71  // 71-73
	spTrigger    = 116 // 116-118
	spName       = 136 // enUS
	spRank       = 153 // enUS
	spSpellLevel = 39

	EffectLearnSpell = 36
)

// Spell is the subset of Spell.dbc the modules need.
type Spell struct {
	ID      uint32
	Name    string
	Rank    string
	Level   uint32
	Effect  [3]uint32
	Trigger [3]uint32
}

// Learned lists the spells a trainer spell teaches (its LEARN_SPELL triggers), or itself.
func (s *Spell) Learned() []uint32 {
	var out []uint32
	for i, e := range s.Effect {
		if e == EffectLearnSpell && s.Trigger[i] != 0 {
			out = append(out, s.Trigger[i])
		}
	}
	if len(out) == 0 {
		out = []uint32{s.ID}
	}
	return out
}

// Talent is one Talent.dbc row: Ranks[r] is the spell for rank r+1.
type Talent struct {
	ID, Tab, Row, Col uint32
	Ranks             [5]uint32
	DependsOn         uint32
	DependsOnRank     uint32
}

// MaxRank is the number of ranks of the talent.
func (t *Talent) MaxRank() int {
	n := 0
	for _, r := range t.Ranks {
		if r != 0 {
			n++
		}
	}
	return n
}

// TalentTab is one TalentTab.dbc row.
type TalentTab struct {
	ID, ClassMask, Page uint32
	Name                string // enUS, e.g. "Arms"
}

// SkillAbility restricts a spell to races/classes (0 = any).
type SkillAbility struct {
	RaceMask, ClassMask uint32
}

// Data holds the tables, indexed by id.
type Data struct {
	Spells    map[uint32]*Spell
	Talents   map[uint32]*Talent
	Tabs      map[uint32]*TalentTab
	Abilities map[uint32][]SkillAbility // by spell id
}

// Load reads Spell, Talent, TalentTab and SkillLineAbility from dir.
func Load(dir string) (*Data, error) {
	d := &Data{Spells: map[uint32]*Spell{}, Talents: map[uint32]*Talent{}, Tabs: map[uint32]*TalentTab{}, Abilities: map[uint32][]SkillAbility{}}
	f, err := Open(filepath.Join(dir, "Spell.dbc"))
	if err != nil {
		return nil, err
	}
	for r := 0; r < f.Records; r++ {
		s := &Spell{ID: f.U32(r, 0), Name: f.Str(r, spName), Rank: f.Str(r, spRank), Level: f.U32(r, spSpellLevel)}
		for i := 0; i < 3; i++ {
			s.Effect[i], s.Trigger[i] = f.U32(r, spEffect+i), f.U32(r, spTrigger+i)
		}
		d.Spells[s.ID] = s
	}
	if f, err = Open(filepath.Join(dir, "Talent.dbc")); err != nil {
		return nil, err
	}
	for r := 0; r < f.Records; r++ {
		t := &Talent{ID: f.U32(r, 0), Tab: f.U32(r, 1), Row: f.U32(r, 2), Col: f.U32(r, 3), DependsOn: f.U32(r, 13), DependsOnRank: f.U32(r, 16)}
		for i := 0; i < 5; i++ {
			t.Ranks[i] = f.U32(r, 4+i)
		}
		d.Talents[t.ID] = t
	}
	if f, err = Open(filepath.Join(dir, "TalentTab.dbc")); err != nil {
		return nil, err
	}
	for r := 0; r < f.Records; r++ {
		d.Tabs[f.U32(r, 0)] = &TalentTab{ID: f.U32(r, 0), ClassMask: f.U32(r, 20), Page: f.U32(r, 22), Name: f.Str(r, 1)}
	}
	if f, err = Open(filepath.Join(dir, "SkillLineAbility.dbc")); err != nil {
		return nil, err
	}
	for r := 0; r < f.Records; r++ {
		id := f.U32(r, 2)
		d.Abilities[id] = append(d.Abilities[id], SkillAbility{RaceMask: f.U32(r, 3), ClassMask: f.U32(r, 4)})
	}
	return d, nil
}

// FitsRaceClass mirrors Player::IsSpellFitByClassAndRace: a spell listed in SkillLineAbility must
// have at least one entry matching the race and class masks.
func (d *Data) FitsRaceClass(spell, race, class uint32) bool {
	abs := d.Abilities[spell]
	if len(abs) == 0 {
		return true
	}
	for _, a := range abs {
		if (a.RaceMask == 0 || a.RaceMask&(1<<(race-1)) != 0) && (a.ClassMask == 0 || a.ClassMask&(1<<(class-1)) != 0) {
			return true
		}
	}
	return false
}

// TalentBySpellName finds the talent of a class whose rank-1 spell has this name.
func (d *Data) TalentBySpellName(class uint32, name string) *Talent {
	for _, t := range d.Talents {
		tab := d.Tabs[t.Tab]
		if tab == nil || tab.ClassMask&(1<<(class-1)) == 0 {
			continue
		}
		if s := d.Spells[t.Ranks[0]]; s != nil && s.Name == name {
			return t
		}
	}
	return nil
}

// ClassSpells lists the spell ids with this name that the class can learn (SkillLineAbility class mask).
func (d *Data) ClassSpells(class uint32, name string) []uint32 {
	var out []uint32
	for id, s := range d.Spells {
		if s.Name != name {
			continue
		}
		for _, a := range d.Abilities[id] {
			if a.ClassMask&(1<<(class-1)) != 0 {
				out = append(out, id)
				break
			}
		}
	}
	return out
}
