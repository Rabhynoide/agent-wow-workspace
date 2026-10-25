package move

import (
	"os"
	"testing"
	"time"
)

// Share of path points on a road, without and with the road preference (ROADS=<dir>, assets needed).
func TestRoadPaths(t *testing.T) {
	dir := os.Getenv("ROADS")
	if dir == "" {
		t.Skip("ROADS not set")
	}
	a := "../../../assets/"
	plain := NewPlanner(a+"mmaps", a+"maps", a+"vmaps", "")
	road := NewPlanner(a+"mmaps", a+"maps", a+"vmaps", dir)
	ref := NewPlanner(a+"mmaps", a+"maps", a+"vmaps", dir)
	goldshire := [3]float32{-9460, 60, 56}
	for _, c := range []struct {
		name string
		to   [3]float32
	}{
		{"Stormwind gate", [3]float32{-9050, 440, 93}},
		{"Sentinel Hill", [3]float32{-10630, 1060, 34}},
		{"Lakeshire", [3]float32{-9220, -2150, 64}},
		{"Darkshire", [3]float32{-10560, -1150, 28}},
		{"Eastvale", [3]float32{-9450, -1330, 47}},
	} {
		for _, pl := range []*Planner{plain, road} {
			start := time.Now()
			pts, l, err := pl.Path(0, goldshire, c.to, 0)
			took := time.Since(start)
			if err != nil {
				t.Logf("%s: %v", c.name, err)
				continue
			}
			on := 0
			for _, p := range pts {
				if ref.OnRoad(0, p[0], p[1]) {
					on++
				}
			}
			kind := "plain"
			if pl == road {
				kind = "roads"
			}
			t.Logf("%-15s %s: %5.0f yards, %3.0f%% on road, %v", c.name, kind, l, 100*float64(on)/float64(len(pts)), took.Round(time.Millisecond))
		}
	}
}
