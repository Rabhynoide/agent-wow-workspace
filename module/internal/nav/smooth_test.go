package nav

import (
	"os"
	"testing"
	"time"
)

func ref(i int32) polyRef { return polyRef{0, 0, i} }

func TestFixupCorridor(t *testing.T) {
	path := []polyRef{ref(1), ref(2), ref(3), ref(4)}
	// Moved forward along the corridor into polygon 3.
	got := fixupCorridor(path, []polyRef{ref(1), ref(2), ref(3)})
	if len(got) != 2 || got[0] != ref(3) || got[1] != ref(4) {
		t.Fatalf("forward: %v", got)
	}
	// Slid sideways into polygon 9 (not in the corridor), reached from polygon 2.
	got = fixupCorridor(path, []polyRef{ref(1), ref(2), ref(9)})
	if len(got) != 4 || got[0] != ref(9) || got[1] != ref(2) || got[2] != ref(3) {
		t.Fatalf("sideways: %v", got)
	}
	// Nothing in common: keep the corridor.
	if got = fixupCorridor(path, []polyRef{ref(7)}); len(got) != 4 {
		t.Fatalf("unrelated: %v", got)
	}
}

func TestSmoothPathOnSurface(t *testing.T) {
	if _, err := os.Stat(mmapDir); err != nil {
		t.Skip("no mmaps")
	}
	m, _ := LoadMap(mmapDir, 0)
	from := ToRecast(-8949.95, -132.49, 83.53) // Northshire
	for name, to := range map[string]V3{
		"McBride (abbey)":  ToRecast(-8902.59, -162.6, 82.0),
		"tower floor":      ToRecast(-8900.0, -181.0, 113.5),
		"Goldshire inn":    ToRecast(-9462, 16, 58),
		"Maclure vineyard": ToRecast(-9924, 38, 33),
		"Defias camp":      ToRecast(-9059, -305, 73),
		"Stormwind gate":   ToRecast(-8913, 554, 94),
	} {
		start := time.Now()
		pts, err := m.SmoothPath(from, to, GroundOnly, SmoothStep, SmoothSlop)
		took := time.Since(start)
		if err != nil {
			t.Errorf("%s: %v after %d points", name, err, len(pts))
			continue
		}
		var l float32
		off := 0
		for i, p := range pts {
			if i > 0 {
				d := dist2D(pts[i-1], p)
				l += dist3D(pts[i-1], p)
				if d > SmoothStep+0.05 {
					t.Errorf("%s: step %d is %.2f yards", name, i, d)
				}
			}
			// On a walkable polygon of any tile (stairs cross tile borders), at its height.
			if _, q, ok := m.NearestPoly(p, GroundOnly, 0.5, 0.05); !ok || abs32(q[1]-p[1]) > 0.5 {
				off++
			}
		}
		straight, _ := m.Path(from, to, GroundOnly)
		var ls float32
		for i := 1; i < len(straight); i++ {
			ls += dist3D(straight[i-1], straight[i])
		}
		if off > 0 {
			t.Errorf("%s: %d of %d points off the walkable surface", name, off, len(pts))
		}
		if l > ls*1.2+5 {
			t.Errorf("%s: smooth path %.0f yards, funnel path %.0f", name, l, ls)
		}
		t.Logf("%s: %d points, %.0f yards (funnel %.0f), %d off-surface, %v", name, len(pts), l, ls, off, took)
	}
}

func TestDestinationInWaterGoesToTheShore(t *testing.T) {
	if _, err := os.Stat(mmapDir); err != nil {
		t.Skip("no mmaps")
	}
	m, _ := LoadMap(mmapDir, 0)
	from := ToRecast(-9462, 16, 58)       // Goldshire
	to := ToRecast(-9472.1, -220.7, 56.3) // murloc spawn at the lake (not on the walkable navmesh)
	if _, _, ok := m.NearestPoly(to, GroundOnly, 6, 3); ok {
		t.Skip("the spawn point is on the navmesh in this data")
	}
	if _, err := m.SmoothPath(from, to, GroundOnly, SmoothStep, SmoothSlop); err == nil {
		t.Fatal("without Partial a destination in the water must be an error")
	}
	pts, err := m.SmoothPath(from, to, GroundPartial, SmoothStep, SmoothSlop)
	if err != nil && err != ErrSmoothStuck {
		t.Fatalf("no path to the shore: %v", err)
	}
	last := pts[len(pts)-1]
	if d := dist2D(last, to); d > 32 {
		t.Fatalf("the path ends %.0f yards from the water spawn", d)
	}
	t.Logf("%d points, ends %.0f yards from the spawn", len(pts), dist2D(last, to))
}
