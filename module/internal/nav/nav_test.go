package nav

import (
	"os"
	"testing"
	"time"
)

const mmapDir = "../../../assets/mmaps"

func TestNorthshirePath(t *testing.T) {
	if _, err := os.Stat(mmapDir); err != nil {
		t.Skip("no mmaps")
	}
	m, err := LoadMap(mmapDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	from := ToRecast(-8949.95, -132.49, 83.53)
	to := ToRecast(-8902.59, -162.6, 82.0) // Marshal McBride
	tx, ty := m.TileCoord(from)
	tl := m.Tile(tx, ty)
	if tl == nil {
		t.Fatalf("no tile at %d,%d", tx, ty)
	}
	t.Logf("tile %d,%d header %d,%d polys=%d bounds %v %v", tx, ty, tl.X, tl.Y, len(tl.polys), tl.Min, tl.Max)
	p, err := m.Path(from, to, GroundOnly)
	if err != nil {
		t.Fatal(err)
	}
	var l float32
	for i := 1; i < len(p); i++ {
		l += dist3D(p[i-1], p[i])
	}
	t.Logf("%d points, length %.1f, straight %.1f", len(p), l, dist3D(from, to))
	for _, v := range p {
		x, y, z := v.ToWoW()
		t.Logf("  %.1f %.1f %.1f", x, y, z)
	}
}

func TestLongPaths(t *testing.T) {
	if _, err := os.Stat(mmapDir); err != nil {
		t.Skip("no mmaps")
	}
	m, _ := LoadMap(mmapDir, 0)
	from := ToRecast(-8949.95, -132.49, 83.53)
	for name, to := range map[string]V3{
		"Goldshire inn":          ToRecast(-9462, 16, 58),
		"Stormwind gate":         ToRecast(-8913, 554, 94),
		"Westfall Sentinel Hill": ToRecast(-10515, 1030, 62),
	} {
		start := time.Now()
		p, err := m.Path(from, to, GroundOnly)
		if err != nil {
			t.Logf("%s: %v (%v)", name, err, time.Since(start))
			continue
		}
		var l float32
		for i := 1; i < len(p); i++ {
			l += dist3D(p[i-1], p[i])
		}
		t.Logf("%s: %d pts, len %.0f, straight %.0f, %v", name, len(p), l, dist3D(from, to), time.Since(start))
	}
}
