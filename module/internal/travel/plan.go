// Package travel plans journeys across walking, flights, area-trigger portals (Deeprun Tram...) and
// the hearthstone, and runs the steps that are not walks (the auto module walks, fighting back).
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

// FlightNode is a known flight point we can use, with its flight master.
type FlightNode struct {
	ID     uint32
	Name   string
	At     Point // the flight master
	NpcEnt uint32
}

// Portal is a way from Enter to Exit that is not a walk: an area trigger that teleports (Trigger), or a
// boat (Transport: wait on the pier Enter for it to dock at Dock, ride to LandDock, step onto Exit).
// Cost gives its duration when we reach Enter at seconds from now (boats: waiting included).
type Portal struct {
	Trigger        uint32
	Transport      uint32
	Name           string
	Enter, Exit    Point
	Dock, LandDock Point
	Cost           func(at float32) float32
}

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

// Graph is what a plan is made from. Flight(a, b) gives the flight time and node path between two
// known nodes (ok false: no route). Walk(a, b) checks a walk for real (navmesh, dangerous areas):
// its time, or ok false when there is no safe path, or an error to stop planning (out of time); it
// is called only for the walks of a candidate plan, estimates (straight line × walkDetour) are used
// until then.
type Graph struct {
	Nodes   []FlightNode
	Flight  func(a, b uint32) (secs float32, path []uint32, ok bool)
	Portals []Portal
	Hearth  *Point // bind point when the hearthstone is ready
	Walk    func(a, b Point) (secs float32, ok bool, err error)
}

type vert struct {
	p    Point
	kind int // vStart, vGoal, vNode, vEnter, vExit, vBind
	idx  int
}

const (
	vStart = iota
	vGoal
	vNode
	vEnter
	vExit
	vBind
)

type walkKey struct{ a, b Point }

const maxRounds = 40

type walkRes struct {
	secs float32
	ok   bool
}

// Plan returns the fastest journey from start to goal. Walks are checked lazily: the best plan by
// estimates has its unchecked walks checked, and planning starts again until every walk of the best
// plan is checked (at most maxRounds rounds).
func Plan(g Graph, start, goal Point) ([]Leg, float32, error) {
	vs := []vert{{p: start, kind: vStart}, {p: goal, kind: vGoal}}
	for i, n := range g.Nodes {
		vs = append(vs, vert{p: n.At, kind: vNode, idx: i})
	}
	for i, p := range g.Portals {
		vs = append(vs, vert{p: p.Enter, kind: vEnter, idx: i}, vert{p: p.Exit, kind: vExit, idx: i})
	}
	if g.Hearth != nil {
		vs = append(vs, vert{p: *g.Hearth, kind: vBind})
	}
	checked := map[walkKey]walkRes{}
	for round := 0; round < maxRounds; round++ {
		legs, total, ok := shortest(g, vs, checked)
		if !ok {
			return nil, 0, fmt.Errorf("no way from %v to %v", start, goal)
		}
		fresh := false
		for _, l := range legs {
			if l.Kind != "walk" {
				continue
			}
			k := walkKey{l.From, l.To}
			if _, done := checked[k]; done {
				continue
			}
			s, ok, err := g.Walk(l.From, l.To)
			if err != nil {
				return nil, 0, err
			}
			checked[k] = walkRes{s, ok}
			fresh = true
		}
		if !fresh {
			return legs, total, nil
		}
	}
	return nil, 0, fmt.Errorf("no plan from %v to %v after %d rounds", start, goal, maxRounds)
}

// edge from vertex i to j (all pairs): kind and cost.
func edges(g Graph, vs []vert, checked map[walkKey]walkRes, i int, at float32, yield func(j int, l Leg)) {
	a := vs[i]
	if a.kind == vGoal {
		return
	}
	for j, b := range vs {
		if j == i || b.kind == vStart {
			continue
		}
		if a.kind == vEnter && b.kind != vExit {
			continue // a portal's entry leads only through it
		}
		// Walk on the same map (not into a portal's exit: those are reached through the portal).
		if a.p.Map == b.p.Map && b.kind != vExit && b.kind != vBind {
			k := walkKey{a.p, b.p}
			l := Leg{Kind: "walk", From: a.p, To: b.p}
			if r, ok := checked[k]; ok {
				if r.ok {
					l.Seconds = r.secs
					yield(j, l)
				}
			} else {
				l.Seconds = dist(a.p, b.p) * walkDetour / RunSpeed
				yield(j, l)
			}
		}
		switch {
		case a.kind == vNode && b.kind == vNode:
			na, nb := g.Nodes[a.idx], g.Nodes[b.idx]
			if s, path, ok := g.Flight(na.ID, nb.ID); ok {
				yield(j, Leg{Kind: "fly", From: a.p, To: b.p, Nodes: path, Npc: na.NpcEnt, Seconds: s + flightCost,
					Name: na.Name + " -> " + nb.Name})
			}
		case a.kind == vEnter && b.kind == vExit && a.idx == b.idx:
			p := g.Portals[a.idx]
			l := Leg{Kind: "portal", From: a.p, To: b.p, Trigger: p.Trigger, Seconds: portalCost, Name: p.Name}
			if p.Transport != 0 {
				l.Kind, l.Transport, l.Dock, l.LandDock = "boat", p.Transport, p.Dock, p.LandDock
			}
			if p.Cost != nil {
				l.Seconds = p.Cost(at)
			}
			yield(j, l)
		case a.kind == vStart && b.kind == vBind:
			yield(j, Leg{Kind: "hearth", From: a.p, To: b.p, Seconds: hearthCost, Name: "hearthstone"})
		}
	}
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

func shortest(g Graph, vs []vert, checked map[walkKey]walkRes) ([]Leg, float32, bool) {
	inf := float32(math.MaxFloat32)
	best := make([]float32, len(vs))
	prev := make([]int, len(vs))
	via := make([]Leg, len(vs))
	for i := range best {
		best[i], prev[i] = inf, -1
	}
	best[0] = 0
	q := &pq{{0, 0}}
	for q.Len() > 0 {
		it := heap.Pop(q).(item)
		if it.cost > best[it.v] {
			continue
		}
		if it.v == 1 {
			break
		}
		edges(g, vs, checked, it.v, it.cost, func(j int, l Leg) {
			if c := it.cost + l.Seconds; c < best[j] {
				best[j], prev[j], via[j] = c, it.v, l
				heap.Push(q, item{j, c})
			}
		})
	}
	if best[1] == inf {
		return nil, 0, false
	}
	var legs []Leg
	for v := 1; v != 0; v = prev[v] {
		legs = append([]Leg{via[v]}, legs...)
	}
	return legs, best[1], true
}
