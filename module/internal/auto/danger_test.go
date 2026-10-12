package auto

import (
	"path/filepath"
	"strings"
	"testing"

	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/xpscore"
)

// Danger from the learned fights (warrior table like the 2026-10-12 benches: -1/0/+1 cost 5/15/28 %).
func TestTooDangerous(t *testing.T) {
	dir := t.TempDir()
	tb := &xpscore.Table{Classes: map[string]map[string]*xpscore.Cell{"1": {
		"-1": {N: 30, LossSum: 150, SecSum: 300}, "0": {N: 40, LossSum: 600, SecSum: 520}, "1": {N: 40, LossSum: 1120, SecSum: 640},
	}}}
	tb.Learn(strings.NewReader(strings.Repeat("time=2026-10-12T15:00:00Z msg=fight result=died target=525 target_level=5 level=5 class=1 hp_start=40 hp_end=0 attackers_max=2 s=8\n", 5)))
	if err := tb.Save(filepath.Join(dir, "fights.json")); err != nil {
		t.Fatal(err)
	}
	s := &Service{fights: xpscore.NewFile(filepath.Join(dir, "fights.json"))}
	cases := []struct {
		sp           *worldpb.Spawn
		level, class uint32
		bad          bool
	}{
		{&worldpb.Spawn{Name: "Stonetusk Boar", MinLevel: 5, MaxLevel: 6}, 6, 1, false},
		{&worldpb.Spawn{Name: "Kobold Vermin", MinLevel: 5, MaxLevel: 6}, 5, 1, false},  // +1: 28 %
		{&worldpb.Spawn{Name: "Level 7 monster", MinLevel: 7, MaxLevel: 7}, 5, 1, true}, // +2: 52 %
		{&worldpb.Spawn{Name: "Rockhide Boar", MinLevel: 7, MaxLevel: 8}, 5, 1, true},   // killed a level-5 warrior twice
		{&worldpb.Spawn{Name: "Weak hitter", MinLevel: 7, MaxLevel: 7, DamageMod: 0.5}, 5, 1, false},
		{&worldpb.Spawn{Name: "Level 6 monster", MinLevel: 6, MaxLevel: 6}, 5, 5, false}, // priest: no fights, default
		{&worldpb.Spawn{Name: "Goldtooth", MinLevel: 8, MaxLevel: 8}, 5, 5, true},
		{&worldpb.Spawn{Name: "Hogger", MinLevel: 11, MaxLevel: 11, Rank: 1}, 15, 1, true},
		{&worldpb.Spawn{Entry: 525, Name: "Mangy Wolf", MinLevel: 5, MaxLevel: 5}, 5, 1, true}, // 5 deaths measured
	}
	for _, c := range cases {
		if got := s.tooDangerous(c.sp, c.level, c.class) != ""; got != c.bad {
			t.Errorf("%s at level %d class %d: dangerous=%v (%s), want %v", c.sp.Name, c.level, c.class, got, s.tooDangerous(c.sp, c.level, c.class), c.bad)
		}
	}
}

func TestByIsolation(t *testing.T) {
	camp := []*worldpb.Spawn{{X: 0, Y: 0, Dist: 30}, {X: 5, Y: 0, Dist: 32}, {X: 0, Y: 5, Dist: 33}, {X: 5, Y: 5, Dist: 35}}
	lone := &worldpb.Spawn{X: 200, Y: 0, Dist: 120}
	sp := byIsolation(append(camp, lone))
	if sp[0] != lone {
		t.Fatalf("an isolated spawn at 120 yards should beat a camp at 30: got %+v", sp[0])
	}
	if n := neighbours(camp[0], camp); n != 3 {
		t.Fatalf("neighbours = %d, want 3", n)
	}
}
