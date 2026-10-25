package travel

import (
	"os"
	"strings"
	"testing"
)

// Routes on the computed graph (assets/travel/graph.json, skipped without it); joining walks are
// straight lines.
func TestGraphRoutes(t *testing.T) {
	gr, err := LoadGraph("../../../assets/travel/graph.json")
	if err != nil {
		t.Skip(err)
	}
	goldshire := Point{0, -9460, 60, 56}
	cases := []struct {
		name  string
		level uint32
		to    Point
		want  string // legs kinds that must appear
	}{
		{"Ironforge by the tram", 10, Point{0, -4838, -1318, 501}, "portal"},
		{"Thelsamar by the tram", 10, Point{0, -5424, -2929, 347}, "portal"},
		{"Sentinel Hill", 10, Point{0, -10628, 1037, 51}, "walk"},
		{"Darkshire at 20", 20, Point{0, -10560, -1150, 28}, "walk"},
		{"Darkshire at 18", 18, Point{0, -10560, -1150, 28}, "walk"},
		{"Darkshire at 22", 22, Point{0, -10560, -1150, 28}, "walk"},
	}
	for _, c := range cases {
		g := baseGraph(gr, c.level)
		g.Walk = func(a, b Point) (float32, bool, error) { return dist(a, b) / RunSpeed, true, nil }
		legs, total, err := Plan(g, goldshire, c.to)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		k := kinds(legs)
		if !strings.Contains(k, c.want) {
			t.Errorf("%s: %s", c.name, k)
		}
		// Never through areas far above the level (Burning Steppes, Searing Gorge: 50+).
		var maxLv int32
		for _, l := range gr.Links {
			for _, leg := range legs {
				if leg.Kind == "walk" && (leg.From == gr.Nodes[l.A].At() && leg.To == gr.Nodes[l.B].At() ||
					leg.From == gr.Nodes[l.B].At() && leg.To == gr.Nodes[l.A].At()) {
					maxLv = max(maxLv, l.Lv)
				}
			}
		}
		if maxLv > int32(c.level)+dangerAbove {
			t.Errorf("%s: crosses level %d", c.name, maxLv)
		}
		if os.Getenv("V") != "" {
			t.Logf("%s: %.0f s, %d legs, max area level %d: %s", c.name, total, len(legs), maxLv, k)
		}
	}
}
