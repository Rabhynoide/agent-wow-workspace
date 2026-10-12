package xpscore

import (
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
`

func TestLearnAndScore(t *testing.T) {
	tb := &Table{Classes: map[string]map[string]*Cell{}}
	if n := tb.Learn(strings.NewReader(lines)); n != 6 {
		t.Fatalf("learned %d fights, want 6", n)
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
	easy := Of(base, 5, 1, tb)
	hard := base
	hard.MinLevel, hard.MaxLevel = 6, 6
	camp := base
	camp.Neighbours = 3
	if h := Of(hard, 5, 1, tb); h.XPMin >= easy.XPMin || h.Loss <= easy.Loss {
		t.Errorf("level 6 (%.0f xp/min, %.0f %%) should score below level 4 (%.0f xp/min, %.0f %%)", h.XPMin, h.Loss, easy.XPMin, easy.Loss)
	}
	if c := Of(camp, 5, 1, tb); c.XPMin >= easy.XPMin || c.PDeath <= easy.PDeath {
		t.Errorf("a camp (%.0f xp/min, death %.2f) should score below lone monsters (%.0f, %.2f)", c.XPMin, c.PDeath, easy.XPMin, easy.PDeath)
	}
}
