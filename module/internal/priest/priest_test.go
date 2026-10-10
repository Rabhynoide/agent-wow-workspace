package priest

import (
	"os"
	"testing"

	"wowagent/internal/dbc"
	"wowagent/internal/gear"
)

func load(t *testing.T) *dbc.Data {
	const dir = "../../../assets/dbc"
	if _, err := os.Stat(dir + "/Spell.dbc"); err != nil {
		t.Skip("assets/dbc not available")
	}
	d, err := dbc.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSpellRanks(t *testing.T) {
	d := load(t)
	for name, ranks := range SpellGroups {
		prev := uint32(1000)
		for _, id := range ranks {
			s := d.Spells[id]
			if s == nil || s.Name != name {
				t.Errorf("%s: spell %d is %+v", name, id, s)
				continue
			}
			if s.Level > prev {
				t.Errorf("%s: %d (level %d) listed after a lower level rank", name, id, s.Level)
			}
			prev = s.Level
		}
	}
}

// TestBuild replays the build with the server rules: tier needs 5 points per row in the tree,
// prerequisites need their rank, ranks never exceed the maximum, total fits 71 points.
func TestBuild(t *testing.T) {
	d := load(t)
	rank := map[uint32]int{}
	spent := map[uint32]int{}
	total := 0
	for _, st := range Build {
		tal := d.TalentBySpellName(ClassID, st.Name)
		if tal == nil {
			t.Fatalf("unknown talent %q", st.Name)
		}
		if st.Rank > tal.MaxRank() {
			t.Fatalf("%s: rank %d > max %d", st.Name, st.Rank, tal.MaxRank())
		}
		for rank[tal.ID] < st.Rank {
			if spent[tal.Tab] < int(tal.Row)*5 {
				t.Fatalf("%s row %d: only %d points in tree", st.Name, tal.Row, spent[tal.Tab])
			}
			if tal.DependsOn != 0 && rank[tal.DependsOn] < int(tal.DependsOnRank)+1 {
				t.Fatalf("%s needs talent %d rank %d", st.Name, tal.DependsOn, tal.DependsOnRank+1)
			}
			rank[tal.ID]++
			spent[tal.Tab]++
			total++
		}
	}
	if total > 71 {
		t.Fatalf("build uses %d points (max 71)", total)
	}
	t.Logf("build uses %d points", total)
}

func TestProficiencySpells(t *testing.T) {
	d := load(t)
	want := map[uint32]string{196: "One-Handed Axes", 198: "One-Handed Maces", 227: "Staves", 1180: "Daggers", 5009: "Wands", 9078: "Cloth", 8737: "Mail", 9116: "Shield", 15590: "Fist Weapons", 201: "One-Handed Swords"}
	for _, sub := range gear.Proficiency {
		for _, id := range sub {
			s := d.Spells[id]
			if s != nil {
				t.Logf("%d %s", id, s.Name)
			}
			if s == nil {
				t.Errorf("proficiency spell %d missing", id)
			} else if w, ok := want[id]; ok && s.Name != w {
				t.Errorf("spell %d is %q, want %q", id, s.Name, w)
			}
		}
	}
}
