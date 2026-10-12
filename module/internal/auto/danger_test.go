package auto

import (
	"testing"

	worldpb "wowagent/gen/worldv1"
)

func TestTooDangerous(t *testing.T) {
	cases := []struct {
		sp           *worldpb.Spawn
		level, class uint32
		bad          bool
	}{
		{&worldpb.Spawn{Name: "Stonetusk Boar", MinLevel: 5, MaxLevel: 6}, 6, 1, false},
		{&worldpb.Spawn{Name: "Kobold Miner", MinLevel: 6, MaxLevel: 7}, 5, 1, false}, // warrior: +1 at most
		{&worldpb.Spawn{Name: "Level 7 monster", MinLevel: 7, MaxLevel: 7}, 5, 1, true},
		{&worldpb.Spawn{Name: "Level 7 monster", MinLevel: 7, MaxLevel: 7}, 5, 5, false}, // priest: +2
		{&worldpb.Spawn{Name: "Rockhide Boar", MinLevel: 7, MaxLevel: 8}, 5, 1, true},    // killed a level-5 warrior twice
		{&worldpb.Spawn{Name: "Rockhide Boar", MinLevel: 7, MaxLevel: 8}, 5, 5, false},
		{&worldpb.Spawn{Name: "Goldtooth", MinLevel: 8, MaxLevel: 8}, 5, 5, true},
		{&worldpb.Spawn{Name: "Hogger", MinLevel: 11, MaxLevel: 11, Rank: 1}, 15, 1, true},
	}
	for _, c := range cases {
		if got := tooDangerous(c.sp, c.level, c.class) != ""; got != c.bad {
			t.Errorf("%s at level %d class %d: dangerous=%v, want %v", c.sp.Name, c.level, c.class, got, c.bad)
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
