package move

import (
	"testing"

	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/nav"
)

// A Cutpurse that comes into sight next to the road ahead makes the walk plan again; one already
// avoided, one behind us or far from the road does not.
func TestNewOnPath(t *testing.T) {
	path := [][3]float32{{0, 0, 0}, {20, 0, 0}, {40, 0, 0}, {60, 0, 0}, {200, 0, 0}}
	at := func(g uint64, x, y float32) danger { return danger{g, nav.Danger{P: nav.ToRecast(x, y, 0), R: 25}} }
	for _, c := range []struct {
		name  string
		d     danger
		known bool
		want  bool
	}{
		{"new, next to the road ahead", at(1, 50, 10), false, true},
		{"already avoided", at(1, 50, 10), true, false},
		{"far from the road", at(2, 50, 60), false, false},
		{"beyond the checked stretch", at(3, 200, 5), false, false},
	} {
		known := map[uint64]bool{1: c.known}
		if got := newOnPath([]danger{c.d}, known, path, 1); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// A wandering wolf is matched to its spawn point; an idle one and a far spawn are not.
func TestHomeOf(t *testing.T) {
	homes := []*worldpb.Spawn{
		{Entry: 299, X: 100, Y: 100, MovementType: 1, Wander: 10},
		{Entry: 299, X: 300, Y: 100, MovementType: 1, Wander: 10},
		{Entry: 40, X: 100, Y: 100, MovementType: 0},
	}
	if h := homeOf(homes, &statepb.Obj{Entry: 299, X: 108, Y: 104}); h == nil || h.X != 100 {
		t.Fatalf("wolf home: %v", h)
	}
	if h := homeOf(homes, &statepb.Obj{Entry: 40, X: 101, Y: 100}); h != nil {
		t.Fatal("an idle kobold has no wander home")
	}
	if h := homeOf(homes, &statepb.Obj{Entry: 299, X: 200, Y: 100}); h != nil {
		t.Fatal("too far from any spawn")
	}
}
