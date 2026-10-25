package move

import (
	"os"
	"testing"

	"wowagent/internal/nav"
)

func TestGhostPathDunMorogh(t *testing.T) {
	if os.Getenv("GHOST") == "" {
		t.Skip()
	}
	a := "../../../assets/"
	pl := NewPlanner(a+"mmaps", a+"maps", a+"vmaps", a+"roads")
	from := [3]float32{-5475.18, -1845.84, 399.786}
	h, _ := pl.Terrain(0).Height(-5733, -1671)
	to := [3]float32{-5733, -1671, h}
	pts, l, err := pl.Path(0, from, to, 0)
	t.Logf("to z=%.1f: %d pts %.0f yards err=%v", h, len(pts), l, err)
	m, _ := pl.Mesh(0)
	for _, p := range [][3]float32{from, to} {
		t.Logf("island at %v: %d polys", p, m.IslandSize(nav.ToRecast(p[0], p[1], p[2]), nav.GroundOnly, 5000))
	}
	if pts, l, err := pl.Path(0, from, to, 35); err == nil && len(pts) > 0 {
		e := pts[len(pts)-1]
		t.Logf("snap 35: %.0f yards, end (%.0f,%.0f,%.0f), %.1f yards from the corpse", l, e[0], e[1], e[2], dist2(e, to))
	} else {
		t.Logf("snap 35: %v", err)
	}
	// Without the road preference.
	pl2 := NewPlanner(a+"mmaps", a+"maps", a+"vmaps", "")
	_, l2, err2 := pl2.Path(0, from, to, 0)
	t.Logf("no roads: %.0f yards err=%v", l2, err2)
}
