package auto

import (
	"testing"

	worldpb "wowagent/gen/worldv1"
)

func TestTooDangerous(t *testing.T) {
	cases := []struct {
		sp    *worldpb.Spawn
		level uint32
		bad   bool
	}{
		{&worldpb.Spawn{Name: "Rockhide Boar", MinLevel: 5, MaxLevel: 6}, 6, false},
		{&worldpb.Spawn{Name: "Kobold Miner", MinLevel: 6, MaxLevel: 7}, 5, false},
		{&worldpb.Spawn{Name: "Goldtooth", MinLevel: 8, MaxLevel: 8}, 5, true},
		{&worldpb.Spawn{Name: "Hogger", MinLevel: 11, MaxLevel: 11, Rank: 1}, 15, true},
	}
	for _, c := range cases {
		if got := tooDangerous(c.sp, c.level) != ""; got != c.bad {
			t.Errorf("%s at level %d: dangerous=%v, want %v", c.sp.Name, c.level, got, c.bad)
		}
	}
}
