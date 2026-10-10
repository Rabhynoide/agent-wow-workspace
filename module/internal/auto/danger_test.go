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
