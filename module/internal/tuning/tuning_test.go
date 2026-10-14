package tuning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverrideAndDecide(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tuning.json"), []byte(`{"chase_margin": 5, "avoid": false}`), 0o644)
	tu := NewSource(dir).Get()
	if tu.ChaseMargin != 5 || tu.Avoid || tu.FleeMargin != 15 {
		t.Fatalf("override: %+v", tu)
	}
	f := &Flee{}
	log := `time=2026-10-14T10:00:00Z level=INFO msg="flee outcome" key=2:10 stayed=true how=explore hp=55 slack=14 result=won
time=2026-10-14T10:01:00Z level=INFO msg="flee outcome" key=2:10 stayed=true how=explore hp=50 slack=12 result=died
time=2026-10-14T10:02:00Z level=INFO msg="flee outcome" key=2:10 stayed=false how=rule hp=50 slack=12 result=fled
time=2026-10-14T10:03:00Z level=INFO msg="flee outcome" key=2:10 stayed=true how=learned hp=50 slack=12 result=fled
`
	if n := f.Learn(strings.NewReader(log)); n != 2 || f.Cells["2:10"].N != 2 || f.Cells["2:10"].Deaths != 1 {
		t.Fatalf("learned %d: %+v", n, f.Cells["2:10"])
	}
	if n := f.Learn(strings.NewReader(log)); n != 0 {
		t.Fatalf("learned twice: %d", n)
	}
	d := Defaults()
	if flee, learned := Decide(f, "2:10", d); !flee || learned {
		t.Errorf("2 fights: rule (flee), not learned")
	}
	f.Cells["2:10"] = &Cell{N: 20, Deaths: 1} // p = 2/22: 16 s expected vs 90 s flight: stay
	if flee, learned := Decide(f, "2:10", d); flee || !learned {
		t.Errorf("low death rate: stay")
	}
	f.Cells["1:-10"] = &Cell{N: 10, Deaths: 6} // p = 7/12: 105 s vs 90 s: flee
	if flee, _ := Decide(f, "1:-10", d); !flee {
		t.Errorf("high death rate: flee")
	}
	if k := Key(5, -45); k != "3:-30" {
		t.Errorf("key %s", k)
	}
}
