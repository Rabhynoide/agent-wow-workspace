// Package travel plans journeys across walking, flights, area-trigger portals (Deeprun Tram...), boats
// and the hearthstone, and runs the steps that are not walks (the auto module walks, fighting back).
package travel

import (
	"container/heap"
	"fmt"
	"math"
)

// Point is a position on a map.
type Point struct {
	Map     uint32
	X, Y, Z float32
}

func (p Point) String() string { return fmt.Sprintf("%d:(%.0f,%.0f)", p.Map, p.X, p.Y) }

func dist(a, b Point) float32 {
	return float32(math.Sqrt(float64((a.X-b.X)*(a.X-b.X) + (a.Y-b.Y)*(a.Y-b.Y) + (a.Z-b.Z)*(a.Z-b.Z))))
}

// Speeds and fixed costs (seconds).
const (
	RunSpeed    = 7.0  // yards/s on foot
	FlightSpeed = 32.0 // PLAYER_FLIGHT_SPEED
	flightCost  = 15   // talk to the flight master, take off, land
	portalCost  = 8    // loading screen
	hearthCost  = 18   // 10 s cast + loading
	walkDetour  = 1.25 // navmesh path vs straight line, before a walk is checked
)

// Leg is one step of a journey (see travel.proto Leg).
type Leg struct {
	Kind           string // walk, fly, portal, boat, hearth
	From, To       Point
	Nodes          []uint32
	Trigger        uint32
	Transport      uint32
	Dock, LandDock Point
	Npc            uint32
	Seconds        float32
	Name           string
}

// Edge is a way from one graph point to another: a computed walk, a flight, a portal or a boat. Cost
// gives its duration when it starts at seconds from now (boats: waiting included); nil: Leg.Seconds.
type Edge struct {
	To   int
	Leg  Leg
	Cost func(at float32) float32
}

// Graph is what a plan is made from: the points of the travel graph usable now and their edges (Adj
// is indexed like Pts). The start, the destination and the bind point are joined to the graph by
// walks to or from the points Near gives (and start to destination directly when closer than
// Direct); those walks are checked for real by Walk, lazily: estimates (straight line × walkDetour)
// are used until a walk is part of the best plan. Walk returns its time, or ok false when there is
// no safe path, or an error to stop planning (out of time).
type Graph struct {
	Pts    []Point
	Adj    [][]Edge
	Near   func(p Point) []int
	Direct float32
	Hearth *Point // bind point when the hearthstone is ready
	Walk   func(a, b Point) (secs float32, ok bool, err error)
}

type walkKey struct{ a, b Point }

type walkRes struct {
	secs float32
	ok   bool
}

const maxRounds = 40

// unchecked marks (Leg.Name) a joining walk that is only estimated.
const unchecked = "unchecked"

// Plan returns the fastest journey from start to goal: the best plan by estimates has its unchecked
// walks checked, and planning starts again until every walk of the best plan is checked.
func Plan(g Graph, start, goal Point) ([]Leg, float32, error) {
	n := len(g.Pts)
	iS, iG, iB := n, n+1, n+2
	pts := append(append([]Point{}, g.Pts...), start, goal)
	dyn := map[int][]int{} // walks joining the graph: from -> to
	for _, j := range g.Near(start) {
		dyn[iS] = append(dyn[iS], j)
	}
	for _, j := range g.Near(goal) {
		dyn[j] = append(dyn[j], iG)
	}
	if start.Map == goal.Map && dist(start, goal) < g.Direct {
		dyn[iS] = append(dyn[iS], iG)
	}
	if g.Hearth != nil {
		pts = append(pts, *g.Hearth)
		for _, j := range g.Near(*g.Hearth) {
			dyn[iB] = append(dyn[iB], j)
		}
		if g.Hearth.Map == goal.Map && dist(*g.Hearth, goal) < g.Direct {
			dyn[iB] = append(dyn[iB], iG)
		}
	}
	checked := map[walkKey]walkRes{}
	for round := 0; round < maxRounds; round++ {
		legs, total, ok := shortest(g, pts, dyn, checked, iS, iG, iB)
		if !ok {
			return nil, 0, fmt.Errorf("no way from %v to %v", start, goal)
		}
		fresh := false
		for _, l := range legs {
			if l.Kind != "walk" || l.Name != unchecked {
				continue
			}
			s, ok, err := g.Walk(l.From, l.To)
			if err != nil {
				return nil, 0, err
			}
			checked[walkKey{l.From, l.To}] = walkRes{s, ok}
			fresh = true
		}
		if !fresh {
			return legs, total, nil
		}
	}
	return nil, 0, fmt.Errorf("no plan from %v to %v after %d rounds", start, goal, maxRounds)
}

type item struct {
	v    int
	cost float32
}
type pq []item

func (q pq) Len() int           { return len(q) }
func (q pq) Less(i, j int) bool { return q[i].cost < q[j].cost }
func (q pq) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *pq) Push(x any)        { *q = append(*q, x.(item)) }
func (q *pq) Pop() any          { o := *q; x := o[len(o)-1]; *q = o[:len(o)-1]; return x }

func shortest(g Graph, pts []Point, dyn map[int][]int, checked map[walkKey]walkRes, iS, iG, iB int) ([]Leg, float32, bool) {
	inf := float32(math.MaxFloat32)
	nv := len(pts)
	best := make([]float32, nv)
	prev := make([]int, nv)
	via := make([]Leg, nv)
	for i := range best {
		best[i], prev[i] = inf, -1
	}
	best[iS] = 0
	q := &pq{{iS, 0}}
	relax := func(from int, at float32, to int, l Leg) {
		if c := at + l.Seconds; c < best[to] {
			best[to], prev[to], via[to] = c, from, l
			heap.Push(q, item{to, c})
		}
	}
	for q.Len() > 0 {
		it := heap.Pop(q).(item)
		if it.cost > best[it.v] {
			continue
		}
		if it.v == iG {
			break
		}
		if it.v < len(g.Adj) {
			for _, e := range g.Adj[it.v] {
				l := e.Leg
				if e.Cost != nil {
					l.Seconds = e.Cost(it.cost)
				}
				relax(it.v, it.cost, e.To, l)
			}
		}
		for _, j := range dyn[it.v] {
			a, b := pts[it.v], pts[j]
			l := Leg{Kind: "walk", From: a, To: b}
			if r, ok := checked[walkKey{a, b}]; ok {
				if !r.ok {
					continue
				}
				l.Seconds = r.secs
			} else {
				l.Seconds, l.Name = dist(a, b)*walkDetour/RunSpeed, unchecked
			}
			relax(it.v, it.cost, j, l)
		}
		if it.v == iS && g.Hearth != nil {
			relax(iS, 0, iB, Leg{Kind: "hearth", From: pts[iS], To: pts[iB], Seconds: hearthCost, Name: "hearthstone"})
		}
	}
	if best[iG] == inf {
		return nil, 0, false
	}
	var legs []Leg
	for v := iG; v != iS; v = prev[v] {
		legs = append([]Leg{via[v]}, legs...)
	}
	return legs, best[iG], true
}
