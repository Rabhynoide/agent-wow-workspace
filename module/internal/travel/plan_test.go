package travel

import (
	"sort"
	"testing"
)

// near: graph points of the same map, closest first, at most 3.
func nearOf(pts []Point) func(p Point) []int {
	return func(p Point) []int {
		var out []int
		for i, q := range pts {
			if q.Map == p.Map {
				out = append(out, i)
			}
		}
		sort.Slice(out, func(a, b int) bool { return dist(p, pts[out[a]]) < dist(p, pts[out[b]]) })
		if len(out) > 3 {
			out = out[:3]
		}
		return out
	}
}

func kinds(legs []Leg) string {
	k := ""
	for _, l := range legs {
		k += l.Kind + " "
	}
	return k
}

// Elwynn to Loch Modan: no walk across (the computed walks through the Burning Steppes are not in
// the graph at level 10), the tram portals are taken, then the flight once both nodes are known.
func TestPlanPortalAndFlight(t *testing.T) {
	elwynn := Point{0, -9460, 60, 56}
	loch := Point{0, -5400, -2900, 330}
	pts := []Point{
		{0, -8346, 514, 90},    // 0 tram entry, Stormwind
		{369, 68, 2490, -4},    // 1 tram arrival, Stormwind side
		{369, 69, 20, -4},      // 2 tram entry, Ironforge side
		{0, -4838, -1318, 501}, // 3 tram arrival, Ironforge
		{0, -4821, -1155, 502}, // 4 Ironforge flight master
		{0, -5424, -2929, 347}, // 5 Thelsamar flight master
		{0, -8800, 600, 95},    // 6 grid point in Stormwind
	}
	g := Graph{Pts: pts, Adj: make([][]Edge, len(pts)), Near: nearOf(pts), Direct: 1200}
	walk := func(a, b int, secs float32) {
		g.Adj[a] = append(g.Adj[a], Edge{To: b, Leg: Leg{Kind: "walk", From: pts[a], To: pts[b], Seconds: secs}})
		g.Adj[b] = append(g.Adj[b], Edge{To: a, Leg: Leg{Kind: "walk", From: pts[b], To: pts[a], Seconds: secs}})
	}
	walk(6, 0, 70)
	walk(1, 2, 350)
	walk(3, 4, 25)
	g.Adj[0] = append(g.Adj[0], Edge{To: 1, Leg: Leg{Kind: "portal", Seconds: portalCost}})
	g.Adj[2] = append(g.Adj[2], Edge{To: 3, Leg: Leg{Kind: "portal", Seconds: portalCost}})
	walks := 0
	g.Walk = func(a, b Point) (float32, bool, error) {
		walks++
		return dist(a, b) * 1.1 / RunSpeed, true, nil
	}
	// Loch Modan is joined only to its nearest graph points: from Ironforge it is a 2 000-yard walk.
	g.Near = func(p Point) []int {
		if p == loch {
			return []int{5}
		}
		return nearOf(pts)(p)
	}
	legs, _, err := Plan(g, elwynn, loch)
	if err == nil {
		t.Fatalf("Thelsamar has no way in without the flight: %s", kinds(legs))
	}
	g.Adj[4] = append(g.Adj[4], Edge{To: 5, Leg: Leg{Kind: "fly", From: pts[4], To: pts[5], Nodes: []uint32{6, 8}, Npc: 1573, Seconds: 75}})
	legs, _, err = Plan(g, elwynn, loch)
	if err != nil {
		t.Fatal(err)
	}
	// Elwynn is joined straight to the tram entry (one of its nearest graph points).
	if k := kinds(legs); k != "walk portal walk portal walk fly walk " {
		t.Fatalf("legs: %s", k)
	}
	if legs[5].Npc != 1573 || len(legs[5].Nodes) != 2 {
		t.Fatalf("fly leg: %+v", legs[5])
	}
	for _, l := range legs {
		if l.Name == unchecked {
			t.Fatalf("unchecked walk in the plan: %+v", l)
		}
	}
	if walks > 8 {
		t.Fatalf("%d walks checked", walks)
	}
}

// A refused joining walk is not used: the next graph point is.
func TestPlanRefusedJoin(t *testing.T) {
	pts := []Point{{0, 100, 0, 0}, {0, 0, 300, 0}, {0, 1000, 0, 0}}
	g := Graph{Pts: pts, Adj: make([][]Edge, len(pts)), Direct: 500}
	g.Near = func(p Point) []int { return nearOf(pts)(p)[:2] }
	g.Adj[0] = []Edge{{To: 2, Leg: Leg{Kind: "walk", From: pts[0], To: pts[2], Seconds: 130}}}
	g.Adj[1] = []Edge{{To: 2, Leg: Leg{Kind: "walk", From: pts[1], To: pts[2], Seconds: 150}}}
	g.Walk = func(a, b Point) (float32, bool, error) {
		if b == pts[0] {
			return 0, false, nil
		}
		return dist(a, b) / RunSpeed, true, nil
	}
	legs, _, err := Plan(g, Point{0, 0, 0, 0}, Point{0, 1010, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if legs[0].To != pts[1] {
		t.Fatalf("joined through %v", legs[0].To)
	}
}

func TestPlanHearth(t *testing.T) {
	far := Point{0, -10500, 1000, 40}
	home := Point{0, -9460, 60, 56}
	pts := []Point{{0, -10400, 1000, 40}, {0, -9470, 70, 56}}
	g := Graph{Pts: pts, Adj: make([][]Edge, len(pts)), Near: nearOf(pts), Direct: 1200,
		Walk:   func(a, b Point) (float32, bool, error) { return dist(a, b) / RunSpeed, true, nil },
		Hearth: &home}
	g.Adj[0] = []Edge{{To: 1, Leg: Leg{Kind: "walk", From: pts[0], To: pts[1], Seconds: 300}}}
	legs, _, err := Plan(g, far, Point{0, -9450, 70, 56})
	if err != nil || len(legs) != 2 || legs[0].Kind != "hearth" {
		t.Fatalf("hearth: %v %s", err, kinds(legs))
	}
}
