package dpslearn

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestArmorReduction(t *testing.T) {
	// 400 armor against a level 10 monster: 400 / (400 + 400 + 850) = 24.2 %.
	if r := ArmorReduction(400, 10); math.Abs(r-0.2424) > 0.001 {
		t.Fatalf("reduction %v", r)
	}
	if ArmorReduction(100000, 10) != 0.75 {
		t.Fatal("cap")
	}
}

// A monster learned at one armor is predicted at another: more armor, less damage.
func TestTakenFollowsArmor(t *testing.T) {
	tb := New()
	tb.AddTaken(46, 9, 300, 6)
	low, _ := tb.Taken(46, 9, 300)
	high, ok := tb.Taken(46, 9, 900)
	if !ok || math.Abs(low-6) > 1e-9 || high >= low {
		t.Fatalf("low %v high %v", low, high)
	}
	// Nearest level when this one was never met.
	if v, ok := tb.Taken(46, 10, 300); !ok || v <= low {
		t.Fatalf("level 10 from level 9: %v %v", v, ok)
	}
	if _, ok := tb.Taken(99, 9, 300); ok {
		t.Fatal("unknown monster")
	}
}

func TestForgets(t *testing.T) {
	tb := New()
	for i := 0; i < 100; i++ {
		tb.AddDealt(1, 10, 10)
	}
	for i := 0; i < 20; i++ {
		tb.AddDealt(1, 10, 20) // a new weapon
	}
	if v, _ := tb.Dealt(1, 10); v < 17 {
		t.Fatalf("still at %v after 20 fights with the new weapon", v)
	}
	if v, ok := tb.Dealt(1, 12); !ok || v < 17 {
		t.Fatalf("level 12 from level 10: %v %v", v, ok)
	}
}

func TestLearnLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "combat.log")
	os.WriteFile(p, []byte(`time=2026-10-26T00:32:46Z level=INFO msg="damage taken" entry=116 level=9 armor=400 dps=8.5 hits=6 s=12
time=2026-10-26T00:32:46Z level=INFO msg="damage dealt" class=1 level=9 dps=14.2 s=12
time=2026-10-26T00:32:46Z level=INFO msg=fight result=won target=116
`), 0o644)
	tb := New()
	n, err := tb.Learn(p)
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if v, ok := tb.Taken(116, 9, 400); !ok || math.Abs(v-8.5) > 1e-9 {
		t.Fatalf("taken %v", v)
	}
	if v, _ := tb.Dealt(1, 9); v != 14.2 {
		t.Fatalf("dealt %v", v)
	}
}
