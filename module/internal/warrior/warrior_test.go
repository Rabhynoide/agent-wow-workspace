package warrior

import (
	"os"
	"testing"

	"wowagent/internal/dbc"
)

func TestSpellRanks(t *testing.T) {
	const dir = "../../../assets/dbc"
	if _, err := os.Stat(dir + "/Spell.dbc"); err != nil {
		t.Skip("assets/dbc not available")
	}
	d, err := dbc.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, ranks := range SpellGroups {
		prev := uint32(1000)
		for _, id := range ranks {
			s := d.Spells[id]
			if s == nil || s.Name != name {
				t.Errorf("%s: spell %d is %+v", name, id, s)
				continue
			}
			if len(d.ClassSpells(ClassID, name)) == 0 {
				t.Errorf("%s is not a warrior spell", name)
			}
			if s.Level > prev {
				t.Errorf("%s: %d listed after a lower level rank", name, id)
			}
			prev = s.Level
		}
	}
}
