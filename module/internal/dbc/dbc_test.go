package dbc

import (
	"os"
	"sort"
	"testing"
)

const dir = "../../../assets/dbc"

func load(t *testing.T) *Data {
	if _, err := os.Stat(dir + "/Spell.dbc"); err != nil {
		t.Skip("assets/dbc not available")
	}
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSpellFields(t *testing.T) {
	d := load(t)
	s := d.Spells[585]
	if s == nil || s.Name != "Smite" || s.Rank != "Rank 1" {
		t.Fatalf("585 = %+v", s)
	}
	if s := d.Spells[17]; s == nil || s.Name != "Power Word: Shield" {
		t.Fatalf("17 = %+v", s)
	}
}

func TestPriestTalentTree(t *testing.T) {
	d := load(t)
	type row struct {
		page, row, col uint32
		name           string
		ranks          int
	}
	var rows []row
	for _, tal := range d.Talents {
		tab := d.Tabs[tal.Tab]
		if tab == nil || tab.ClassMask != 1<<4 {
			continue
		}
		rows = append(rows, row{tab.Page, tal.Row, tal.Col, d.Spells[tal.Ranks[0]].Name, tal.MaxRank()})
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.page != b.page {
			return a.page < b.page
		}
		if a.row != b.row {
			return a.row < b.row
		}
		return a.col < b.col
	})
	for _, r := range rows {
		t.Logf("tab %d row %d col %d %-32s %d", r.page, r.row, r.col, r.name, r.ranks)
	}
	if len(rows) < 80 {
		t.Fatalf("only %d priest talents", len(rows))
	}
}

func TestTabNamesAndClassSpells(t *testing.T) {
	d := load(t)
	names := map[string]bool{}
	for _, tab := range d.Tabs {
		if tab.ClassMask == 1 { // warrior
			names[tab.Name] = true
		}
	}
	for _, n := range []string{"Arms", "Fury", "Protection"} {
		if !names[n] {
			t.Errorf("warrior tree %q missing (have %v)", n, names)
		}
	}
	if len(d.ClassSpells(1, "Rend")) == 0 || len(d.ClassSpells(5, "Rend")) != 0 {
		t.Error("Rend should be a warrior spell only")
	}
}
