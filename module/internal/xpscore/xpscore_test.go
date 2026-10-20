package xpscore

import (
	"math"
	"strings"
	"testing"
)

func TestBaseGain(t *testing.T) {
	for _, c := range []struct{ pl, mob, want uint32 }{{5, 5, 70}, {5, 6, 74}, {5, 4, 56}, {5, 12, 84}, {10, 4, 0}, {6, 1, 0}} {
		if got := BaseGain(c.pl, c.mob); got != c.want {
			t.Errorf("BaseGain(%d, %d) = %d, want %d", c.pl, c.mob, got, c.want)
		}
	}
}

const lines = `time=2026-10-12T15:00:00Z level=INFO msg=fight result=won target=1 target_level=4 level=5 class=1 hp_start=100 hp_end=96 attackers_max=1 s=11
time=2026-10-12T15:00:01Z level=INFO msg=fight result=won target=1 target_level=4 level=5 class=1 hp_start=95 hp_end=91 attackers_max=1 s=11
time=2026-10-12T15:00:02Z level=INFO msg=fight result=won target=1 target_level=4 level=5 class=1 hp_start=100 hp_end=96 attackers_max=1 s=11
time=2026-10-12T15:00:03Z level=INFO msg=fight result=won target=1 target_level=5 level=5 class=1 hp_start=100 hp_end=86 attackers_max=1 s=12
time=2026-10-12T15:00:04Z level=INFO msg=fight result=won target=1 target_level=5 level=5 class=1 hp_start=100 hp_end=86 attackers_max=1 s=12
time=2026-10-12T15:00:05Z level=INFO msg=fight result=won target=1 target_level=5 level=5 class=1 hp_start=100 hp_end=86 attackers_max=1 s=12
time=2026-10-12T15:00:06Z level=INFO msg=fight result=won target=1 target_level=5 level=5 class=1 hp_start=100 hp_end=40 attackers_max=2 s=12
time=2026-10-12T15:00:07Z level=INFO msg=fight result=died target=1 target_level=6 level=5 class=1 hp_start=30 hp_end=0 attackers_max=1 s=12
time=2026-10-12T15:00:08Z level=INFO msg=fight result=died target=2 target_level=5 level=5 class=1 hp_start=100 hp_end=0 attackers_max=3 s=12
time=2026-10-12T15:00:09Z level=INFO msg=fight result=died target=2 target_level=5 level=5 class=1 hp_start=35 hp_end=0 attackers_max=1 s=12
`

func TestLearnAndScore(t *testing.T) {
	tb := &Table{Classes: map[string]map[string]*Cell{}}
	if n := tb.Learn(strings.NewReader(lines)); n != 10 {
		t.Fatalf("learned %d fights, want 10", n)
	}
	if c := tb.Measured(1, 1, 1); c != (MobCell{N: 1, Deaths: 1, Chained: 1, ChainedDeaths: 1}) {
		t.Fatalf("monster 1 at +1: %+v", c)
	}
	if n := tb.Learn(strings.NewReader(lines)); n != 0 {
		t.Fatalf("lines learned twice: %d", n)
	}
	if loss, secs := tb.Fight(1, 0); loss != 14 || secs != 12 {
		t.Fatalf("gap 0 = %.1f %% %.1f s, want 14 %% 12 s", loss, secs)
	}
	if loss, _ := tb.Fight(1, 1); loss < 25 || loss > 27 { // extrapolated from gap 0
		t.Fatalf("gap +1 extrapolated to %.1f %%", loss)
	}
	base := Mob{Entry: 1, MinLevel: 4, MaxLevel: 4, Spawns: 20, AreaRadius: 150, RespawnS: 300}
	easy := Of(base, 5, 1, tb, AddCost)
	hard := base
	hard.MinLevel, hard.MaxLevel = 6, 6
	camp := base
	camp.Neighbours = 3
	if h := Of(hard, 5, 1, tb, AddCost); h.XPMin >= easy.XPMin || h.Loss <= easy.Loss {
		t.Errorf("level 6 (%.0f xp/min, %.0f %%) should score below level 4 (%.0f xp/min, %.0f %%)", h.XPMin, h.Loss, easy.XPMin, easy.Loss)
	}
	if c := Of(camp, 5, 1, tb, AddCost); c.XPMin >= easy.XPMin || c.PDeath <= easy.PDeath {
		t.Errorf("a camp (%.0f xp/min, death %.2f) should score below lone monsters (%.0f, %.2f)", c.XPMin, c.PDeath, easy.XPMin, easy.PDeath)
	}
	// Two deaths in two fights against monster 2 (same level): the measured risk wins over the model.
	killer := base
	killer.Entry, killer.MinLevel, killer.MaxLevel = 2, 5, 5
	if k := Of(killer, 5, 1, tb, AddCost); k.PDeath < 2.0/12 || k.Deaths != 2 || k.Chained != 1 {
		t.Errorf("monster 2: death %.2f, measured %+v", k.PDeath, k)
	}
}

// A table saved before the death counts reads the old lines again for them, once.
func TestLearnMobsAfterUpgrade(t *testing.T) {
	tb := &Table{Classes: map[string]map[string]*Cell{}, Until: "2026-10-12T15:00:09Z"}
	if n := tb.Learn(strings.NewReader(lines)); n != 10 {
		t.Fatalf("learned %d lines, want 10 (monster counts only)", n)
	}
	if tb.Classes["1"] != nil {
		t.Fatal("single fights counted twice")
	}
	if c := tb.Measured(1, 2, 0); c.N != 2 || c.Deaths != 2 {
		t.Fatalf("monster 2: %+v", c)
	}
}

// Past keepFights, the old fights are scaled down: 100 fights with 10 deaths, then one won fight,
// keep the death rate but weigh 40 fights.
func TestForget(t *testing.T) {
	c := &MobCell{N: 101, Deaths: 10, Chained: 20, ChainedDeaths: 5, Fled: 4}
	c.forget()
	if c.N != keepFights || math.Abs(c.Deaths-10*40.0/101) > 1e-9 || math.Abs(c.Fled-4*40.0/101) > 1e-9 {
		t.Errorf("after forget: %+v", c)
	}
	f := &Cell{N: 30, LossSum: 300, SecSum: 360}
	f.forget()
	if f.N != 30 || f.LossSum != 300 {
		t.Errorf("under keepFights, unchanged: %+v", f)
	}
	g := &Cell{N: 80, LossSum: 1200, SecSum: 960}
	g.forget()
	if g.N != 40 || g.LossSum/g.N != 15 || g.SecSum/g.N != 12 {
		t.Errorf("averages kept: %+v", g)
	}
}
