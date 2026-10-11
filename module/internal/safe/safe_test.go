package safe

import "testing"

func TestCandidatesAwayFromThreats(t *testing.T) {
	// Monsters east of the corpse: the best point is on the west edge of the ring.
	threats := [][2]float32{{20, 0}, {25, 5}, {22, -6}}
	c := Candidates(0, 0, 0, 35, threats)
	if len(c) != 1+7*16 {
		t.Fatalf("%d candidates", len(c))
	}
	best := c[0]
	if best.X > -30 || best.Clearance < 50 {
		t.Fatalf("best = %+v, want the west edge, far from the monsters", best)
	}
	for i := 1; i < len(c); i++ {
		if c[i].Clearance > c[i-1].Clearance {
			t.Fatal("not sorted by clearance")
		}
	}
	// No threat: the center comes first (closest among equals).
	if c := Candidates(3, 4, 0, 10, nil); c[0].X != 3 || c[0].Y != 4 {
		t.Fatalf("without threats the center must win, got %+v", c[0])
	}
}
