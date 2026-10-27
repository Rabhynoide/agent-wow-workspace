package combat

import "testing"

func TestFlightPathHelpers(t *testing.T) {
	p := [][3]float32{{0, 0, 0}, {30, 0, 0}, {30, 40, 0}}
	c := cutPath(p, 50)
	if end := c[len(c)-1]; end[0] != 30 || end[1] != 20 {
		t.Fatalf("cut at 50 yards: %v", end)
	}
	if d := firstDir(p, 10); d == nil || d[0] < 0.99 {
		t.Fatalf("first direction %v", d)
	}
	legs := legsOf(p, 15)
	if len(legs) != 2 || legs[len(legs)-1].y != 40 { // at the path's own points (every 2 yards in real paths)
		t.Fatalf("legs %v", legs)
	}
}
