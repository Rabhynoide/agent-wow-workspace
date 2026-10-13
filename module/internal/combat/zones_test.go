package combat

import (
	"testing"
	"time"

	statepb "wowagent/gen/statev1"
)

func TestFledZone(t *testing.T) {
	now := time.Now()
	// The pack of 3 Mangy Wolves of the 2026-10-13 bench, around (-9530,-150).
	att := []*statepb.Obj{{X: -9525, Y: -150}, {X: -9535, Y: -145}, {X: -9530, Y: -155}}
	z := zoneOf(0, att, now)
	if z.x != -9530 || z.y != -150 {
		t.Fatalf("centre = %v,%v, want -9530,-150", z.x, z.y)
	}
	zs := []fledZone{z}
	for _, c := range []struct {
		name string
		m    uint32
		x, y float32
		at   time.Time
		want bool
	}{
		{"wolf of the pack", 0, -9530, -150, now, true},
		{"spawn point 30 yards away", 0, -9500, -150, now, true},
		{"spawn point 60 yards away", 0, -9470, -150, now, false},
		{"other map", 1, -9530, -150, now, false},
		{"4 minutes later", 0, -9530, -150, now.Add(4 * time.Minute), true},
		{"6 minutes later", 0, -9530, -150, now.Add(6 * time.Minute), false},
	} {
		if got := inFledZone(zs, c.m, c.x, c.y, c.at); got != c.want {
			t.Errorf("%s: in zone = %v, want %v", c.name, got, c.want)
		}
	}
}
