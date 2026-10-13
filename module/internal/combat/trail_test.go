package combat

import "testing"

// A walk east along y = 0, then a flight from its end: back west, one leg per trailLeg yards.
func TestTrailBack(t *testing.T) {
	tr := &trail{}
	for x := float32(0); x <= 100; x += 5 {
		tr.add(trailPt{m: 0, x: x})
	}
	if n := len(tr.pts); n != 21 { // every trailStep yards
		t.Fatalf("%d points, want 21", n)
	}
	legs, idx := tr.back(0, 101, 1)
	if len(legs) == 0 || legs[0].x != 85 || idx[0] != 17 {
		t.Fatalf("first leg %v %v, want x=85 at 17", legs, idx)
	}
	if last := legs[len(legs)-1]; last.x != 10 { // trailRun yards at most
		t.Errorf("last leg at x=%v, want 10", last.x)
	}
	if l, _ := tr.back(1, 101, 1); l != nil {
		t.Errorf("other map: %v", l)
	}
	if l, _ := tr.back(0, 50, 40); l != nil {
		t.Errorf("far from the trail: %v", l)
	}
	// The run back is not recorded and the trail ends where the flight stopped.
	tr.pause(true)
	tr.add(trailPt{m: 0, x: 80, y: 3})
	tr.cut(idx[1])
	if n, x := len(tr.pts), tr.pts[len(tr.pts)-1].x; n != idx[1]+1 || x != 70 {
		t.Errorf("after the flight: %d points ending at x=%v", n, x)
	}
	// A teleport starts a new trail.
	tr.add(trailPt{m: 0, x: 500})
	if len(tr.pts) != 1 {
		t.Errorf("after a teleport: %d points", len(tr.pts))
	}
}
