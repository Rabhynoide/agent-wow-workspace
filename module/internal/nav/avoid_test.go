package nav

import (
	"os"
	"testing"
)

// minDist is the closest a path comes to a point (2D, Recast space).
func minDist(p []V3, c V3) float32 {
	best := float32(1e9)
	for _, v := range p {
		best = min(best, dist2D(v, c))
	}
	return best
}

// A monster in the middle of the Goldshire road: the path walks around its aggro range.
func TestAvoidDanger(t *testing.T) {
	if _, err := os.Stat(mmapDir); err != nil {
		t.Skip("no mmaps")
	}
	m, err := LoadMap(mmapDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	long, err := m.SmoothPath(ToRecast(-9462, 16, 58), ToRecast(-8913, 554, 94), GroundOnly, SmoothStep, SmoothSlop)
	if err != nil || len(long) < 80 {
		t.Fatalf("road to Stormwind: %d points, %v", len(long), err)
	}
	from, to := long[10], long[70] // about 120 yards of road
	p, err := m.SmoothPath(from, to, GroundOnly, SmoothStep, SmoothSlop)
	if err != nil {
		t.Fatal(err)
	}
	c := p[len(p)/2]
	f := GroundOnly
	f.Avoid = []Danger{{P: c, R: 15}}
	q, err := m.SmoothPath(from, to, f, SmoothStep, SmoothSlop)
	if err != nil {
		t.Fatal(err)
	}
	pl, ql := length(p), length(q)
	t.Logf("plain %.0f yards (passes %.1f from the monster), avoiding %.0f yards (passes %.1f)", pl, minDist(p, c), ql, minDist(q, c))
	if minDist(q, c) < 10 {
		t.Errorf("the path still passes %.1f yards from the monster", minDist(q, c))
	}
}

func length(p []V3) float32 {
	var l float32
	for i := 1; i < len(p); i++ {
		l += dist3D(p[i-1], p[i])
	}
	return l
}
