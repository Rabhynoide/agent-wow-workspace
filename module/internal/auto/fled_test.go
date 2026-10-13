package auto

import (
	"testing"

	combatpb "wowagent/gen/combatv1"
)

func TestFledOn(t *testing.T) {
	zs := []*combatpb.Zone{{Map: 0, X: 0, Y: 0, Radius: 40}}
	for _, c := range []struct {
		name         string
		fx, fy, x, y float32
		want         bool
	}{
		{"spawn point in the zone", 100, 0, 20, 0, true},
		{"way through the zone", -100, 10, 100, 10, true},
		{"way beside the zone", -100, 60, 100, 60, false},
		{"from inside, leading out", 10, 0, 120, 0, false},
		{"from inside, through the centre", 10, 0, -120, 0, true},
		{"just after a flight, 45 yards off, going away", 45, 0, 150, 30, false},
	} {
		if got := fledOn(zs, 0, c.fx, c.fy, c.x, c.y) != nil; got != c.want {
			t.Errorf("%s: avoided = %v, want %v", c.name, got, c.want)
		}
	}
	if fledOn(zs, 1, -100, 10, 100, 10) != nil {
		t.Error("a zone of another map avoided")
	}
}
