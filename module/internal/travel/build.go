package travel

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/dbc"
	"wowagent/internal/faction"
	"wowagent/internal/move"
	"wowagent/internal/world"
)

// BuildOpts: assets directory (dbc, mmaps, maps, vmaps), maps to cover, parallel planners.
type BuildOpts struct {
	Assets  string
	Maps    []uint32
	Workers int
}

// Graph building constants: special nodes (flight masters, portals, piers, inns) are linked to the
// nearest specialGrid grid nodes and to every special node within specialReach yards.
const (
	specialReach = 900
	specialGrid  = 6
	linkEnd      = 3 // a link must end this close to its node
	// A link longer than linkDetour × the straight line (+ linkSlack) is a detour through other
	// tiles (9 000 yards between two Elwynn points 700 yards apart): other links cover it.
	linkDetour = 3
	linkSlack  = 150
)

type builder struct {
	o      BuildOpts
	tab    *dbc.Travel
	fac    faction.Table
	mobs   map[[3]int32][]mob // hostile non-elite spawns by map and mobCell cell
	w      *world.Service
	pl     *move.Planner
	g      *TravelGraph
	maps   map[uint32]bool
	grid   map[[3]int32]int // map, tile x, tile y -> grid node
	pairs  map[[2]int]bool
	jobs   [][2]int
	ctx    context.Context
	errors int
}

// Build computes the travel graph (cmd/travelgraph).
func Build(ctx context.Context, o BuildOpts) (*TravelGraph, error) {
	tab, err := dbc.LoadTravel(o.Assets + "/dbc")
	if err != nil {
		return nil, err
	}
	if o.Workers <= 0 {
		o.Workers = 1
	}
	fac, err := faction.Load(o.Assets + "/dbc")
	if err != nil {
		return nil, err
	}
	b := &builder{o: o, tab: tab, fac: fac, w: &world.Service{}, ctx: ctx,
		pl: move.NewPlanner(o.Assets+"/mmaps", o.Assets+"/maps", o.Assets+"/vmaps", o.Assets+"/roads"),
		g: &TravelGraph{Version: GraphVersion, Built: time.Now().UTC().Format(time.RFC3339), Maps: o.Maps,
			Areas: map[uint32]int32{}},
		mobs: map[[3]int32][]mob{}, maps: map[uint32]bool{}, grid: map[[3]int32]int{}, pairs: map[[2]int]bool{}}
	for _, m := range o.Maps {
		b.maps[m] = true
	}
	for _, m := range o.Maps {
		if err := b.mobLevels(m); err != nil {
			return nil, err
		}
		if err := b.gridNodes(m); err != nil {
			return nil, err
		}
		if err := b.npcNodes(m); err != nil {
			return nil, err
		}
	}
	if err := b.portalNodes(); err != nil {
		return nil, err
	}
	if err := b.pierNodes(); err != nil {
		return nil, err
	}
	b.planLinks()
	b.runLinks()
	slog.Info("travel graph", "nodes", len(b.g.Nodes), "links", len(b.g.Links), "portals", len(b.g.Portals), "failed", b.errors)
	return b.g, nil
}

// unattackable: UNIT_FLAG_NON_ATTACKABLE, IMMUNE_TO_PC, NOT_SELECTABLE.
const unattackable = 0x2 | 0x100 | 0x2000000

// humanFaction: the faction template of a human player (hostility of the monsters).
const humanFaction = 1

// mobAreaMin: an area needs this many hostile spawns for its own level (else its zone's).
const mobAreaMin = 8

// Danger of a computed walk: the level of the hostile monsters within mobNear yards of the path (plus
// their wander radius; mobDZ in height), at the mobPct percentile (a single strong spawn by the road
// does not close it; a zone full of them does), elites and rares counted eliteBonus levels higher,
// bosses (rank 3, instances) left out. 0 when there are none. Scripted spawns and patrols on waypoints
// are not seen (Stitches on the Duskwood road).
const (
	mobCell     = 50
	mobNear     = 30
	mobDZ       = 15
	mobPct      = 0.8
	mobOutliers = 2
	// eliteBonus: an elite counts as a monster this many levels higher.
	eliteBonus = 3
)

type mob struct {
	x, y, z, wander float32
	lv              int32
}

func cellOf(m uint32, x, y float32) [3]int32 {
	return [3]int32{int32(m), int32(math.Floor(float64(x / mobCell))), int32(math.Floor(float64(y / mobCell)))}
}

// pathDanger: see mobPct.
func (b *builder) pathDanger(m uint32, pts [][3]float32) int32 {
	seen := map[*mob]bool{}
	var lvs []int32
	for i, p := range pts {
		if i%5 != 0 && i != len(pts)-1 { // points every 2 yards: check every 10
			continue
		}
		c := cellOf(m, p[0], p[1])
		for dx := int32(-1); dx <= 1; dx++ {
			for dy := int32(-1); dy <= 1; dy++ {
				list := b.mobs[[3]int32{c[0], c[1] + dx, c[2] + dy}]
				for k := range list {
					o := &list[k]
					if seen[o] || math.Abs(float64(o.z-p[2])) > mobDZ || math.Hypot(float64(o.x-p[0]), float64(o.y-p[1])) > float64(mobNear+o.wander) {
						continue
					}
					seen[o] = true
					lvs = append(lvs, o.lv)
				}
			}
		}
	}
	// The two strongest are left out (the Sewer Beast, a level 50 rare of the Stormwind canals,
	// closed the tram).
	if len(lvs) < mobOutliers+1 {
		return 0
	}
	sort.Slice(lvs, func(i, j int) bool { return lvs[i] < lvs[j] })
	return lvs[min(int(mobPct*float64(len(lvs)-1)), len(lvs)-1-mobOutliers)]
}

// mobLevels sets the median level of the hostile non-elite monsters of each area and zone of a map,
// and lists the hostile monsters for pathDanger.
func (b *builder) mobLevels(m uint32) error {
	spawns, err := b.w.SpawnLevels(b.ctx, m)
	if err != nil {
		return err
	}
	terr := b.pl.Terrain(m)
	byArea := map[uint32][]int32{}
	for _, sp := range spawns {
		if sp.Rank == 3 || sp.MaxLevel > 83 || sp.UnitFlags&unattackable != 0 || b.fac.Reaction(sp.Faction, humanFaction) != "hostile" {
			continue
		}
		lv := int32(sp.MinLevel+sp.MaxLevel) / 2
		c := cellOf(m, sp.X, sp.Y)
		if sp.Rank != 0 {
			b.mobs[c] = append(b.mobs[c], mob{sp.X, sp.Y, sp.Z, min(sp.Wander, 40), lv + eliteBonus})
			continue
		}
		b.mobs[c] = append(b.mobs[c], mob{sp.X, sp.Y, sp.Z, min(sp.Wander, 40), lv})
		area := terr.Area(sp.X, sp.Y)
		if area == 0 {
			continue
		}
		byArea[area] = append(byArea[area], lv)
		if a := b.tab.Areas[area]; a != nil && a.Zone != 0 {
			byArea[a.Zone] = append(byArea[a.Zone], lv)
		}
	}
	for area, lvs := range byArea {
		if len(lvs) < mobAreaMin {
			continue
		}
		sort.Slice(lvs, func(i, j int) bool { return lvs[i] < lvs[j] })
		b.g.Areas[area] = lvs[len(lvs)/2]
	}
	slog.Info("monster levels", "map", m, "areas", len(byArea))
	return nil
}

func (b *builder) add(n GNode) int {
	b.g.Nodes = append(b.g.Nodes, n)
	return len(b.g.Nodes) - 1
}

// gridNodes: one walkable point per navmesh tile, near its centre (or a quarter of a tile away).
func (b *builder) gridNodes(m uint32) error {
	mesh, err := b.pl.Mesh(m)
	if err != nil {
		return err
	}
	terr := b.pl.Terrain(m)
	tiles := mesh.Tiles()
	sort.Slice(tiles, func(i, j int) bool {
		return tiles[i][0] < tiles[j][0] || (tiles[i][0] == tiles[j][0] && tiles[i][1] < tiles[j][1])
	})
	n := 0
	for _, t := range tiles {
		c := mesh.TileCenter(t[0], t[1])
		cx, cy, _ := c.ToWoW()
		for _, off := range [][2]float32{{0, 0}, {1, 1}, {-1, -1}, {1, -1}, {-1, 1}} {
			x, y := cx+off[0]*133, cy+off[1]*133
			z, vert := float32(0), float32(2000)
			if h, ok := terr.Height(x, y); ok {
				z, vert = h, 60
			}
			p, ok := b.pl.Snap(m, [3]float32{x, y, z}, vert, 120)
			if !ok {
				continue
			}
			b.grid[[3]int32{int32(m), t[0], t[1]}] = b.add(GNode{Kind: KindGrid, Map: m, X: p[0], Y: p[1], Z: p[2]})
			n++
			break
		}
	}
	slog.Info("grid nodes", "map", m, "tiles", len(tiles), "nodes", n)
	return nil
}

// npcNodes: flight masters of every node with a mount, and innkeepers.
func (b *builder) npcNodes(m uint32) error {
	var ids []uint32
	for id, n := range b.tab.Nodes {
		if n.Map == m && (n.AllianceMount != 0 || n.HordeMount != 0) && !questNode(n.Name) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	flights := 0
	for _, id := range ids {
		n := b.tab.Nodes[id]
		r, err := b.w.FindNpc(b.ctx, &worldpb.FindNpcRequest{MapFilter: true, Map: m, X: n.X, Y: n.Y, Radius: 60, Max: 1, NpcFlags: npcFlightMaster})
		if err != nil {
			return err
		}
		if len(r.Spawns) == 0 {
			continue
		}
		sp := r.Spawns[0]
		b.add(GNode{Kind: KindFlight, Map: m, X: sp.X, Y: sp.Y, Z: sp.Z, Name: n.Name, Taxi: id, Npc: sp.Entry})
		flights++
	}
	r, err := b.w.FindNpc(b.ctx, &worldpb.FindNpcRequest{MapFilter: true, Map: m, Max: 100, NpcFlags: npcInnkeeper})
	if err != nil {
		return err
	}
	for _, sp := range r.Spawns {
		b.add(GNode{Kind: KindInn, Map: m, X: sp.X, Y: sp.Y, Z: sp.Z, Name: sp.Name, Npc: sp.Entry})
	}
	slog.Info("npc nodes", "map", m, "flight", flights, "inns", len(r.Spawns))
	return nil
}

// portalNodes: area triggers between the covered maps, entered at a walkable point inside (the
// trigger centre floats above the floor: 8 yards at the Stormwind tram entrance).
func (b *builder) portalNodes() error {
	r, err := b.w.Teleports(b.ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	for _, t := range r.Teleports {
		tr := b.tab.Triggers[t.Id]
		if tr == nil || !b.maps[tr.Map] || !b.maps[t.Map] {
			continue
		}
		var enter *[3]float32
		for _, spot := range triggerSpots(tr) {
			for _, reach := range []float32{2, 6} {
				q, ok := b.pl.Snap(tr.Map, [3]float32{spot.X, spot.Y, spot.Z}, 12, reach)
				if ok && inTrigger(tr, Point{tr.Map, q[0], q[1], q[2]}) {
					enter = &q
					break
				}
			}
			if enter != nil {
				break
			}
		}
		if enter == nil {
			slog.Info("portal without a walkable point inside", "trigger", t.Id, "name", t.Name)
			continue
		}
		exit := [3]float32{t.X, t.Y, t.Z}
		if q, ok := b.pl.Snap(t.Map, exit, 10, 10); ok {
			exit = q
		}
		in := b.add(GNode{Kind: KindEnter, Map: tr.Map, X: enter[0], Y: enter[1], Z: enter[2], Name: t.Name, Trigger: t.Id})
		out := b.add(GNode{Kind: KindExit, Map: t.Map, X: exit[0], Y: exit[1], Z: exit[2], Name: t.Name, Trigger: t.Id})
		b.g.Portals = append(b.g.Portals, GPortal{Trigger: t.Id, Name: t.Name, Enter: in, Exit: out})
	}
	slog.Info("portals", "n", len(b.g.Portals))
	return nil
}

// pierNodes: next to each dock of each boat and zeppelin, the end of a walk from the nearest flight
// point of the map to the dock (the navmesh stops on the pier).
func (b *builder) pierNodes() error {
	r, err := b.w.Transports(b.ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	piers := 0
	for _, t := range r.Transports {
		kf, period := keyFrames(b.tab.PathNodes[t.Path], float64(t.Speed), float64(t.Accel))
		if period == 0 {
			continue
		}
		for i, d := range docks(kf) {
			if !b.maps[d.Map] {
				continue
			}
			dock := Point{d.Map, d.X, d.Y, d.Z}
			var anchor *Point
			for _, n := range b.tab.Nodes {
				p := Point{n.Map, n.X, n.Y, n.Z}
				if n.Map == d.Map && dist(p, dock) < 2500 && (anchor == nil || dist(p, dock) < dist(*anchor, dock)) {
					anchor = &p
				}
			}
			if anchor == nil {
				continue
			}
			pts, _, err := b.pl.Path(d.Map, [3]float32{anchor.X, anchor.Y, anchor.Z}, [3]float32{d.X, d.Y, d.Z}, 60)
			if err != nil || len(pts) == 0 {
				slog.Info("no pier", "transport", t.Name, "dock", i, "err", err)
				continue
			}
			e := pts[len(pts)-1]
			if dist(Point{d.Map, e[0], e[1], e[2]}, dock) > 70 {
				slog.Info("pier too far from the dock", "transport", t.Name, "dock", i)
				continue
			}
			b.add(GNode{Kind: KindPier, Map: d.Map, X: e[0], Y: e[1], Z: e[2], Name: t.Name, Transport: t.Entry, Dock: i + 1})
			piers++
		}
	}
	slog.Info("piers", "n", piers)
	return nil
}

func (b *builder) pair(a, c int) {
	if a == c {
		return
	}
	if a > c {
		a, c = c, a
	}
	if !b.pairs[[2]int{a, c}] {
		b.pairs[[2]int{a, c}] = true
		b.jobs = append(b.jobs, [2]int{a, c})
	}
}

// planLinks lists the walks to compute: grid nodes with their 8 neighbours, special nodes with the
// nearest grid nodes and the special nodes around.
func (b *builder) planLinks() {
	for k, a := range b.grid {
		for _, d := range [][2]int32{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
			if c, ok := b.grid[[3]int32{k[0], k[1] + d[0], k[2] + d[1]}]; ok {
				b.pair(a, c)
			}
		}
	}
	for i, n := range b.g.Nodes {
		if n.Kind == KindGrid {
			continue
		}
		type cand struct {
			j int
			d float32
		}
		var grid []cand
		for j, o := range b.g.Nodes {
			if j == i || o.Map != n.Map {
				continue
			}
			d := dist2D(n.At(), o.At())
			if o.Kind == KindGrid {
				grid = append(grid, cand{j, d})
			} else if d <= specialReach {
				b.pair(i, j)
			}
		}
		sort.Slice(grid, func(x, y int) bool { return grid[x].d < grid[y].d })
		for x := 0; x < len(grid) && x < specialGrid; x++ {
			b.pair(i, grid[x].j)
		}
	}
	// Neighbouring jobs together: the planners keep the tiles they read.
	sort.Slice(b.jobs, func(x, y int) bool {
		na, nb := b.g.Nodes[b.jobs[x][0]], b.g.Nodes[b.jobs[y][0]]
		if na.Map != nb.Map {
			return na.Map < nb.Map
		}
		if ta, tb := int(na.X/533), int(nb.X/533); ta != tb {
			return ta < tb
		}
		return na.Y < nb.Y
	})
	slog.Info("walks to compute", "n", len(b.jobs))
}

func dist2D(a, b Point) float32 {
	return dist(Point{a.Map, a.X, a.Y, 0}, Point{b.Map, b.X, b.Y, 0})
}

// runLinks computes the walks with Workers planners, by chunks of neighbouring jobs.
func (b *builder) runLinks() {
	const chunk = 64
	ch := make(chan [][2]int)
	go func() {
		for i := 0; i < len(b.jobs); i += chunk {
			ch <- b.jobs[i:min(i+chunk, len(b.jobs))]
		}
		close(ch)
	}()
	var mu sync.Mutex
	var done, failed atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < b.o.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pl := move.NewPlanner(b.o.Assets+"/mmaps", b.o.Assets+"/maps", b.o.Assets+"/vmaps", b.o.Assets+"/roads")
			for jobs := range ch {
				for _, j := range jobs {
					l, ok := b.walk(pl, j[0], j[1])
					if n := done.Add(1); n%500 == 0 {
						slog.Info("walks", "done", n, "of", len(b.jobs), "failed", failed.Load(), "s", int(time.Since(start).Seconds()))
					}
					if !ok {
						failed.Add(1)
						continue
					}
					mu.Lock()
					b.g.Links = append(b.g.Links, l)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	b.errors = int(failed.Load())
	sort.Slice(b.g.Links, func(x, y int) bool {
		return b.g.Links[x].A < b.g.Links[y].A || (b.g.Links[x].A == b.g.Links[y].A && b.g.Links[x].B < b.g.Links[y].B)
	})
}

func (b *builder) walk(pl *move.Planner, a, c int) (GLink, bool) {
	na, nc := b.g.Nodes[a], b.g.Nodes[c]
	pts, l, err := pl.Path(na.Map, [3]float32{na.X, na.Y, na.Z}, [3]float32{nc.X, nc.Y, nc.Z}, 0)
	if err != nil || len(pts) == 0 {
		return GLink{}, false
	}
	e := pts[len(pts)-1]
	if dist(Point{nc.Map, e[0], e[1], e[2]}, nc.At()) > linkEnd {
		return GLink{}, false
	}
	if l > linkDetour*dist(na.At(), nc.At())+linkSlack {
		return GLink{}, false
	}
	return GLink{A: a, B: c, Len: l, Lv: b.pathDanger(na.Map, pts)}, true
}

func (n GNode) String() string { return fmt.Sprintf("%s %s %v", n.Kind, n.Name, n.At()) }
