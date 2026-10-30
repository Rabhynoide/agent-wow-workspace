package tuning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverrideAndDecide(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tuning.json"), []byte(`{"chase_margin": 5, "avoid": true}`), 0o644)
	tu := NewSource(dir).Get()
	if tu.ChaseMargin != 5 || !tu.Avoid || tu.FleeMargin != 15 {
		t.Fatalf("override: %+v", tu)
	}
	f := &Flee{}
	log := `time=2026-10-14T10:00:00Z level=INFO msg="flee outcome" key=2:10 stayed=true how=explore hp=55 slack=14 result=won
time=2026-10-14T10:01:00Z level=INFO msg="flee outcome" key=2:10 stayed=true how=explore hp=50 slack=12 result=died
time=2026-10-14T10:02:00Z level=INFO msg="flee outcome" key=2:10 stayed=false how=rule hp=50 slack=12 result=fled
time=2026-10-14T10:03:00Z level=INFO msg="flee outcome" key=2:10 stayed=true how=learned hp=50 slack=12 result=fled
`
	if n := f.Learn(strings.NewReader(log)); n != 3 || f.Cells["2:10"].N != 2 || f.Cells["2:10"].Deaths != 1 || f.Cells["2:10"].FleeN != 1 {
		t.Fatalf("learned %d: %+v", n, f.Cells["2:10"])
	}
	if n := f.Learn(strings.NewReader(log)); n != 0 {
		t.Fatalf("learned twice: %d", n)
	}
	d := Defaults()
	// Nothing learned: computed from the slack. 20 % of health left over a loss of 40 % (spread 20):
	// P(death) 16 %, 29 s, against a flight of 90 s: stay. 20 % short: P 84 %, 151 s against 90 + 18: flee.
	f = &Flee{Cells: map[string]*Cell{}}
	if flee, p, _ := Decide(f, "2:20", 20, 40, d); flee || p < 0.15 || p > 0.17 {
		t.Errorf("slack 20: stay, p %v", p)
	}
	if flee, _, _ := Decide(f, "2:-20", -20, 40, d); !flee {
		t.Errorf("slack -20: flee")
	}
	// Stays learned in the case correct it: 8 deaths in 10 at slack 20, (8 + 4 × 0.16) / 14 = 62 %: flee.
	f.Cells["2:20"] = &Cell{N: 10, Deaths: 8}
	if flee, _, _ := Decide(f, "2:20", 20, 40, d); !flee {
		t.Errorf("deadly stays: flee")
	}
	// Flights that die (from the same number of attackers too): stay at slack -20.
	f.Cells["2:-20"] = &Cell{FleeN: 10, FleeDeaths: 8}
	f.Cells["2:0"] = &Cell{FleeN: 10, FleeDeaths: 7}
	if flee, _, _ := Decide(f, "2:-20", -20, 40, d); flee {
		t.Errorf("deadly flights: stay")
	}
	if p := FleeDeath(f, "9:9", d); p != d.FleeDeathPrior {
		t.Errorf("unknown attackers: prior, got %v", p)
	}
	if k := Key(5, -45); k != "4:-30" {
		t.Errorf("key %s", k)
	}
}
