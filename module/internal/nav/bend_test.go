package nav

import (
	"os"
	"testing"
)

// A monster in the middle of the Goldshire road: the bent path passes just outside its aggro range,
// a few yards longer than the road, where the weighted A* went far around or through.
func TestBendAroundMonster(t *testing.T) {
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
	from, to := long[10], long[70]
	p, err := m.Path(from, to, GroundOnly)
	if err != nil {
		t.Fatal(err)
	}
	sp, _ := m.SmoothPath(from, to, GroundOnly, SmoothStep, SmoothSlop)
	c := sp[len(sp)/2]
	f := GroundOnly
	f.Avoid = []Danger{{P: c, R: 15}}
	q, bent, err := m.Bend(from, to, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("plain %.0f yards, bent %.0f yards (%d discs), passes %.1f yards from the monster", length(p), length(q), bent, segMin(q, c))
	if bent != 1 || segMin(q, c) < 15 {
		t.Errorf("not around the monster: %d bent, %.1f yards", bent, segMin(q, c))
	}
	if length(q) > length(p)+30 {
		t.Errorf("detour too long: %.0f for %.0f", length(q), length(p))
	}
	// A camp of three overlapping discs is walked around as one.
	f.Avoid = []Danger{{P: c, R: 12}, {P: V3{c[0] + 10, c[1], c[2]}, R: 12}, {P: V3{c[0], c[1], c[2] + 10}, R: 12}}
	q, bent, err = m.Bend(from, to, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Avoid {
		if segMin(q, d.P) < d.R-0.5 {
			t.Errorf("camp: passes %.1f yards from a monster (range %.0f)", segMin(q, d.P), d.R)
		}
	}
	t.Logf("camp: bent %.0f yards (%d)", length(q), bent)
}

// segMin is the closest the corners' segments come to c (2D).
func segMin(p []V3, c V3) float32 {
	cc := circle{float64(c[0]), float64(c[2]), 0}
	best := float32(1e9)
	for i := 1; i < len(p); i++ {
		for r := float32(0); r < 60; r += 0.25 {
			cc.r = float64(r)
			if cc.segHits(p[i-1], p[i], 0) {
				best = min(best, r)
				break
			}
		}
	}
	return best
}
