package faction

import (
	"os"
	"testing"
)

func TestReactionsForAHuman(t *testing.T) {
	tab, err := Load("../../../assets/dbc")
	if os.IsNotExist(err) || (err != nil && tab == nil) {
		t.Skip("no FactionTemplate.dbc in assets/dbc")
	}
	const human = 1
	for id, want := range map[uint32]string{12: "friendly", 14: "hostile", 7: "neutral", 18: "hostile", 85: "hostile", 1: "friendly"} {
		if got := tab.Reaction(id, human); got != want {
			t.Errorf("template %d -> %q, want %q", id, got, want)
		}
	}
	if tab.Reaction(999999, human) != "" {
		t.Error("unknown template must give no reaction")
	}
}
