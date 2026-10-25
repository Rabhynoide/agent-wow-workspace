// Package move walks the character along navmesh paths.
package move

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/movev1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/api"
	"wowagent/internal/jobs"
	"wowagent/internal/moveio"
	"wowagent/internal/nav"
	"wowagent/internal/roads"
	"wowagent/internal/terrain"
	"wowagent/internal/vmap"
)

const (
	flagForward = 0x1
	flagWalk    = 0x100
	tick        = 100 * time.Millisecond
	heartbeat   = 250 * time.Millisecond
	maxWait     = 25
)

type job = jobs.Job[*pb.MoveStatus]

// route is a path handed to the running walk (re-targeting without stop/start packets).
type route struct {
	path   [][3]float32
	target [3]float32
	radius float32
	plen   float32
	mapID  uint32
	known  map[uint64]bool // monsters avoided when planning (avoid.go)
}

type Service struct {
	pb.UnimplementedMoveServer
	session modv1.SessionClient
	api     api.C
	dir     string
	guid    uint64
	jobs    *jobs.Runner[*pb.MoveStatus]

	mu     sync.Mutex
	meshes map[uint32]*nav.Mesh
	terr   map[uint32]*terrain.Maps
	vmaps  map[uint32]*vmap.Tree // nil value: no vmaps for that map
	maps   string
	roads  map[uint32]*roads.Map // nil value: no road grid for that map
	// roadsDir: where the road grids are ("" = ROADS_DIR, else /roads; "-" = none)
	roadsDir string

	rmu       sync.Mutex
	walking   bool   // a walk loop is running and accepts new routes
	walkFlags uint32 // movement flags of that walk
	next      *route
}

func Register(s grpc.ServiceRegistrar, session modv1.SessionClient) {
	dir := os.Getenv("MMAPS_DIR")
	if dir == "" {
		dir = "/mmaps"
	}
	guid, _ := strconv.ParseUint(os.Getenv("AGENT_WOW_CHARACTER_GUID"), 10, 64)
	runner := jobs.NewRunner(&pb.MoveStatus{State: "idle"})
	runner.Decorate = func(j *job, st *pb.MoveStatus) { st.ElapsedS = float32(time.Since(j.Start).Seconds()) }
	pb.RegisterMoveServer(s, &Service{session: session, api: api.C{S: session}, dir: dir, guid: guid, jobs: runner,
		meshes: map[uint32]*nav.Mesh{}, terr: map[uint32]*terrain.Maps{}, vmaps: map[uint32]*vmap.Tree{}, maps: mapsDir(),
		roads: map[uint32]*roads.Map{}})
}

func mapsDir() string {
	if d := os.Getenv("MAPS_DIR"); d != "" {
		return d
	}
	return "/maps"
}

// vmapFor opens the collision geometry of a map once (nil when absent).
func (s *Service) vmapFor(id uint32) *vmap.Tree {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.vmaps[id]; ok {
		return t
	}
	dir := os.Getenv("VMAPS_DIR")
	if dir == "" {
		dir = "/vmaps"
	}
	t, err := vmap.Open(dir, id)
	if err != nil {
		slog.Warn("no vmaps: heights from terrain and navmesh only", "map", id, "err", err)
		t = nil
	}
	s.vmaps[id] = t
	return t
}

// roadFor gives the road test of a map for nav.Filter.Road (nil without a road grid: cmd/roads).
func (s *Service) roadFor(id uint32) func(nav.V3) bool {
	s.mu.Lock()
	r, ok := s.roads[id]
	if !ok {
		dir := s.roadsDir
		if dir == "" {
			dir = os.Getenv("ROADS_DIR")
		}
		if dir == "" {
			dir = "/roads"
		}
		var err error
		if dir == "-" {
			r = nil
		} else if r, err = roads.Load(dir, id); err != nil {
			slog.Info("no road grid: paths do not prefer roads", "map", id, "err", err)
			r = nil
		}
		s.roads[id] = r
	}
	s.mu.Unlock()
	if r == nil {
		return nil
	}
	return func(p nav.V3) bool {
		x, y, _ := p.ToWoW()
		return r.On(x, y)
	}
}

func (s *Service) terrainFor(id uint32) *terrain.Maps {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.terr[id]; t != nil {
		return t
	}
	t := terrain.New(s.maps, id)
	s.terr[id] = t
	return t
}

func (s *Service) mesh(id uint32) (*nav.Mesh, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.meshes[id]; m != nil {
		return m, nil
	}
	m, err := nav.LoadMap(s.dir, id)
	if err != nil {
		return nil, fmt.Errorf("no mmaps for map %d: %w", id, err)
	}
	s.meshes[id] = m
	return m, nil
}

func (s *Service) self(ctx context.Context) (*statepb.SelfInfo, error) {
	b, err := s.api.Brief(ctx)
	if err != nil {
		return nil, err
	}
	if b.Self == nil {
		return nil, fmt.Errorf("self state not available yet")
	}
	return b.Self, nil
}

func (s *Service) setPos(ctx context.Context, x, y, z, o float32) {
	if err := s.api.SetPos(ctx, x, y, z, o); err != nil {
		slog.Warn("state.set_pos failed", "err", err)
	}
}

func (s *Service) send(ctx context.Context, op uint32, flags uint32, x, y, z, o float32) error {
	return moveio.Send(ctx, s.session, s.guid, op, flags, x, y, z, o)
}

func orient(dx, dy float32) float32 {
	o := math.Atan2(float64(dy), float64(dx))
	if o < 0 {
		o += 2 * math.Pi
	}
	return float32(o)
}

// densify converts the funnel path to WoW-space points about 1.5 yards apart, snapped to the real terrain
// (the navmesh surface can be off by a yard or more); the navmesh height is kept under bridges and roofs.
func densify(m *nav.Mesh, tm *terrain.Maps, pts []nav.V3, corridor []nav.PolyRef) [][3]float32 {
	var out [][3]float32
	add := func(v nav.V3) {
		x, y, z := v.ToWoW()
		if tm != nil {
			if th, ok := tm.Height(x, y); ok && math.Abs(float64(th-z)) < 0.8 {
				z = th
			}
		}
		out = append(out, [3]float32{x, y, z})
	}
	add(pts[0])
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		d := float32(math.Hypot(float64(b[0]-a[0]), float64(b[2]-a[2])))
		n := int(d / 1.5)
		for k := 1; k <= n; k++ {
			t := float32(k) / float32(n+1)
			v := nav.V3{a[0] + t*(b[0]-a[0]), a[1] + t*(b[1]-a[1]), a[2] + t*(b[2]-a[2])}
			if h, ok := m.HeightInCorridor(v, corridor); ok {
				v[1] = h
			}
			add(v)
		}
		add(b)
	}
	return smoothZ(out)
}

// smoothZ low-passes heights so stairs and bumps become ramps; real cliffs (big steps) are kept.
func smoothZ(p [][3]float32) [][3]float32 {
	if len(p) < 3 {
		return p
	}
	out := make([][3]float32, len(p))
	copy(out, p)
	const w = 3
	for i := 1; i < len(p)-1; i++ { // the end points keep their exact floor height
		var sum, n float32
		for j := max(0, i-w); j <= min(len(p)-1, i+w); j++ {
			if math.Abs(float64(p[j][2]-p[i][2])) > 2.5 {
				continue
			}
			sum += p[j][2]
			n++
		}
		if n > 0 {
			out[i][2] = sum / n
		}
	}
	return out
}

func (s *Service) plan(mapID uint32, from [3]float32, x, y, z float32, avoid []nav.Danger) ([][3]float32, float32, error) {
	return s.planReach(mapID, from, x, y, z, avoid, 0)
}

// planReach is plan with the reach of the closest walkable point to an unreachable destination.
func (s *Service) planReach(mapID uint32, from [3]float32, x, y, z float32, avoid []nav.Danger, reach float32) ([][3]float32, float32, error) {
	m, err := s.mesh(mapID)
	if err != nil {
		return nil, 0, err
	}
	tm, vm := s.terrainFor(mapID), s.vmapFor(mapID)
	to := [3]float32{x, y, z}
	f := nav.GroundOnly
	f.Avoid = avoid
	f.EndReach, f.Partial = reach, reach > 0
	f.Road = s.roadFor(mapID)
	dense, err := navPlan(m, tm, vm, from, to, f)
	// Only when the START is the problem: an unreachable destination would make every escape try
	// search the whole area (seconds each).
	if errors.Is(err, nav.ErrNoStartPoly) || (errors.Is(err, nav.ErrNoPath) &&
		m.IslandSize(nav.ToRecast(from[0], from[1], from[2]), nav.GroundOnly, islandPolys) < islandPolys) {
		// Pushed onto a scrap of navmesh cut from the rest (ledge, rock) or off it: walk a short
		// straight line on the ground to a point that has a path, then follow it.
		if out, ok := escape(m, tm, vm, from, to, f); ok {
			slog.Info("unstuck: straight walk to the navmesh", "from", from, "via", out[0])
			dense, err = out[1:], nil
		}
	}
	if err != nil {
		return nil, 0, err
	}
	var l float32
	for i := 1; i < len(dense); i++ {
		l += dist(dense[i-1], dense[i])
	}
	return dense, l, nil
}

// islandPolys: a start reaching fewer polygons than this is on an island (cut from the rest).
const islandPolys = 300

func navPlan(m *nav.Mesh, tm *terrain.Maps, vm *vmap.Tree, from, to [3]float32, f nav.Filter) ([][3]float32, error) {
	src, dst := nav.ToRecast(from[0], from[1], from[2]), nav.ToRecast(to[0], to[1], to[2])
	// Points every 2 yards that slide along the walkable polygons (the server's own smoothing), heights
	// from the polygon reached; the old funnel + interpolation is the fallback.
	if pts, err := m.SmoothPath(src, dst, f, nav.SmoothStep, nav.SmoothSlop); err == nil {
		return onGround(pts, tm, vm), nil
	} else {
		slog.Info("smooth path failed, using the funnel path", "err", err)
	}
	pts, corridor, err := m.PathCorridor(src, dst, f)
	if err != nil {
		return nil, err
	}
	return densify(m, tm, pts, corridor), nil
}

// escape looks for a point 4 to 24 yards away, towards the destination first, then in other
// directions, reachable in a straight line on the ground (no step over 1.5 yards, nothing in the
// way) and from which the navmesh has a path to the destination. It returns that point followed by
// the straight walk and the path (the first element is the escape point, for the log).
func escape(m *nav.Mesh, tm *terrain.Maps, vm *vmap.Tree, from, to [3]float32, f nav.Filter) ([][3]float32, bool) {
	base := math.Atan2(float64(to[1]-from[1]), float64(to[0]-from[0]))
	for _, turn := range []float64{0, 0.5, -0.5, 1, -1, 1.6, -1.6, 2.2, -2.2, math.Pi} {
		a := base + turn
		dx, dy := float32(math.Cos(a)), float32(math.Sin(a))
		line := [][3]float32{from}
		for d := float32(1); d <= 24; d++ {
			p := [3]float32{from[0] + dx*d, from[1] + dy*d, line[len(line)-1][2]}
			h, ok := floor(p[0], p[1], p[2]+1.5, tm, vm)
			if !ok || math.Abs(float64(h-line[len(line)-1][2])) > 1.5 {
				break // cliff or wall
			}
			p[2] = h
			if vm != nil && !vm.LineOfSight([3]float32{line[len(line)-1][0], line[len(line)-1][1], line[len(line)-1][2] + 1},
				[3]float32{p[0], p[1], p[2] + 1}) {
				break
			}
			line = append(line, p)
			if d < 4 || int(d)%4 != 0 {
				continue
			}
			if rest, err := navPlan(m, tm, vm, p, to, f); err == nil && len(rest) > 0 {
				return append([][3]float32{p}, append(line, rest...)...), true
			}
		}
	}
	return nil, false
}

// onGround converts navmesh points to WoW space and puts them on the real floor the way the server
// does (WorldObject::GetMapHeight + Map::GetHeight): search 2 yards above the point; the terrain
// height counts when we are not under it, the vmap floor (buildings, bridges, rocks) when one is
// found; between both, the vmap one wins when it is above the terrain or closer. A floor more than
// 3 yards from the navmesh is ignored (another layer). Heights are then low-passed (stairs → ramps).
func onGround(pts []nav.V3, tm *terrain.Maps, vm *vmap.Tree) [][3]float32 {
	out := make([][3]float32, 0, len(pts))
	for _, v := range pts {
		x, y, z := v.ToWoW()
		if h, ok := floor(x, y, z, tm, vm); ok && math.Abs(float64(h-z)) < 3 {
			z = h
		}
		out = append(out, [3]float32{x, y, z})
	}
	return smoothZ(out)
}

// floor is Map::GetHeight for a walker standing at z (the search starts 2 yards higher).
func floor(x, y, z float32, tm *terrain.Maps, vm *vmap.Tree) (float32, bool) {
	const tolerance = 0.05 // GROUND_HEIGHT_TOLERANCE
	zq := z + 2
	mapH, mapOK := float32(0), false
	if tm != nil {
		if th, ok := tm.Height(x, y); ok && zq >= th-tolerance {
			mapH, mapOK = th, true
		}
	}
	if vm != nil {
		if vh, ok := vm.Height(x, y, z); ok {
			if !mapOK || vh > mapH || math.Abs(float64(mapH-zq)) > math.Abs(float64(vh-zq)) {
				return vh, true
			}
		}
	}
	return mapH, mapOK
}

// LineOfSight checks the static geometry between our eyes and the target's (2 yards above each).
func (s *Service) LineOfSight(ctx context.Context, r *pb.LosRequest) (*pb.LosResult, error) {
	self, err := s.self(ctx)
	if err != nil {
		return &pb.LosResult{Reason: err.Error()}, nil
	}
	vm := s.vmapFor(self.Map)
	if vm == nil {
		return &pb.LosResult{Visible: true, Reason: "no vmaps for this map"}, nil
	}
	ok := vm.LineOfSight([3]float32{self.X, self.Y, self.Z + 2}, [3]float32{r.X, r.Y, r.Z + 2})
	return &pb.LosResult{Visible: ok}, nil
}

func dist(a, b [3]float32) float32 {
	return float32(math.Sqrt(float64((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))))
}

func dist2(a, b [3]float32) float32 {
	return float32(math.Hypot(float64(a[0]-b[0]), float64(a[1]-b[1])))
}

func (s *Service) Path(ctx context.Context, r *pb.PathRequest) (*pb.PathResponse, error) {
	self, err := s.self(ctx)
	if err != nil {
		return &pb.PathResponse{Error: err.Error()}, nil
	}
	mapID, from, avoid := self.Map, [3]float32{self.X, self.Y, self.Z}, []nav.Danger(nil)
	if r.FromSet {
		mapID, from = r.Map, [3]float32{r.FromX, r.FromY, r.FromZ}
	} else {
		avoid = navDangers(s.dangers(ctx, self, r.X, r.Y, r.Z))
	}
	pts, l, err := s.planReach(mapID, from, r.X, r.Y, r.Z, avoid, r.Snap)
	if err != nil {
		return &pb.PathResponse{Error: err.Error()}, nil
	}
	out := &pb.PathResponse{Points: uint32(len(pts)), Length: l}
	if n := len(pts); n > 0 {
		out.End = &pb.PathPoint{X: pts[n-1][0], Y: pts[n-1][1], Z: pts[n-1][2]}
	}
	if r.WithAreas {
		out.Areas = areaSpans(s.terrainFor(mapID), pts)
	}
	if r.WithPoints {
		for _, p := range pts {
			out.Pts = append(out.Pts, &pb.PathPoint{X: p[0], Y: p[1], Z: p[2]})
		}
	}
	return out, nil
}

func (s *Service) Stop(ctx context.Context, _ *emptypb.Empty) (*pb.MoveStatus, error) {
	s.jobs.Stop()
	return s.jobs.Current(), nil
}

func (s *Service) BeforeLogout(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	s.jobs.Stop()
	return &emptypb.Empty{}, nil
}

func (s *Service) Status(ctx context.Context, r *pb.StatusRequest) (*pb.MoveStatus, error) {
	return s.jobs.Wait(ctx, r.WaitS, 0, maxWait), nil
}

func (s *Service) Goto(ctx context.Context, r *pb.GotoRequest) (*pb.MoveStatus, error) {
	wait := r.WaitS
	if wait == 0 {
		wait = 20
	}
	st, err := s.startGoto(ctx, r)
	if st != nil || err != nil {
		return st, err
	}
	return s.jobs.Wait(ctx, wait, wait, maxWait), nil
}

// startGoto starts a walk, or re-targets the running one; it returns a status only on failure.
func (s *Service) startGoto(ctx context.Context, r *pb.GotoRequest) (*pb.MoveStatus, error) {
	s.jobs.Lock()
	defer s.jobs.Unlock()
	self, err := s.self(ctx)
	if err != nil {
		return &pb.MoveStatus{State: "failed", Reason: err.Error()}, nil
	}
	if self.Dead && !self.Ghost {
		return &pb.MoveStatus{State: "failed", Reason: "character is dead", X: self.X, Y: self.Y, Z: self.Z}, nil
	}
	speed := self.RunSpeed
	if speed <= 0 {
		speed = 7
	}
	flags := uint32(flagForward)
	if r.Walk {
		speed = 2.5
		flags |= flagWalk
	}
	radius := r.Radius
	if radius <= 0 {
		radius = 1.5
	}
	target := [3]float32{r.X, r.Y, r.Z}
	ds := s.dangers(ctx, self, r.X, r.Y, r.Z)
	avoid, known := navDangers(ds), knownOf(ds)
	// Already walking with the same gait: plan from the live position and swap the route in place,
	// so chasing a moving target does not stutter with stop/start packets.
	if j := s.jobs.Cur(); j != nil {
		cur := j.Snap()
		path, plen, err := s.plan(self.Map, [3]float32{cur.X, cur.Y, cur.Z}, r.X, r.Y, r.Z, avoid)
		if err == nil && len(path) >= 2 {
			s.rmu.Lock()
			ok := s.walking && s.walkFlags == flags
			if ok {
				s.next = &route{path: path, target: target, radius: radius, plen: plen, mapID: self.Map, known: known}
			}
			s.rmu.Unlock()
			if ok {
				return nil, nil
			}
		}
	}
	path, plen, err := s.plan(self.Map, [3]float32{self.X, self.Y, self.Z}, r.X, r.Y, r.Z, avoid)
	if err != nil {
		s.jobs.StartLocked(&pb.MoveStatus{State: "failed", Reason: err.Error(), X: self.X, Y: self.Y, Z: self.Z}, func(*job) {})
		return &pb.MoveStatus{State: "failed", Reason: err.Error(), X: self.X, Y: self.Y, Z: self.Z}, nil
	}
	s.jobs.StartLocked(&pb.MoveStatus{State: "moving", X: self.X, Y: self.Y, Z: self.Z, PathLen: plen, DistLeft: plen}, func(j *job) {
		s.run(j, &route{path: path, target: target, radius: radius, plen: plen, mapID: self.Map, known: known}, speed, flags)
	})
	return nil, nil
}

func (s *Service) run(j *job, rt *route, speed float32, flags uint32) {
	ctx := j.Ctx
	path := rt.path
	pos := path[0]
	o := float32(0)
	seg := 1
	s.rmu.Lock()
	s.walking, s.walkFlags, s.next = true, flags, nil
	s.rmu.Unlock()
	finish := func(state, reason string) {
		s.rmu.Lock()
		s.walking, s.next = false, nil
		s.rmu.Unlock()
		if state == "failed" && ctx.Err() != nil {
			state, reason = "stopped", "cancelled"
		}
		j.Update(func(st *pb.MoveStatus) {
			st.State, st.Reason = state, reason
			st.X, st.Y, st.Z, st.O = pos[0], pos[1], pos[2], o
			st.DistLeft, st.EtaS = dist2(pos, rt.target), 0
			st.ElapsedS = float32(time.Since(j.Start).Seconds())
		})
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.send(sctx, uint32(opcode.MSGMoveStop), 0, pos[0], pos[1], pos[2], o)
		s.setPos(sctx, pos[0], pos[1], pos[2], o)
	}
	if len(path) < 2 {
		finish("arrived", "already there")
		return
	}
	o = orient(path[1][0]-pos[0], path[1][1]-pos[1])
	if err := s.send(ctx, uint32(opcode.MSGMoveStartForward), flags, pos[0], pos[1], pos[2], o); err != nil {
		finish("failed", "send: "+err.Error())
		return
	}
	lastBeat := time.Now()
	lastTick := time.Now()
	lastAlive := time.Now()
	lastCheck := time.Now()
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			finish("stopped", "cancelled")
			return
		case <-t.C:
		}
		s.rmu.Lock()
		if s.next != nil {
			rt, s.next = s.next, nil
			path, seg = rt.path, 1
			// The route was planned from a slightly older position: skip nodes we already reached.
			for seg < len(path)-1 && dist2(pos, path[seg]) < 1 {
				seg++
			}
			j.Update(func(st *pb.MoveStatus) { st.PathLen = rt.plen })
		}
		s.rmu.Unlock()
		now := time.Now()
		// Monsters seen since the route was planned, close to what is left of it: plan again around them.
		if now.Sub(lastCheck) >= recheckEvery*time.Second && seg < len(path)-1 {
			lastCheck = now
			if me, err := s.self(ctx); err == nil {
				ds := s.dangers(ctx, me, rt.target[0], rt.target[1], rt.target[2])
				if newOnPath(ds, rt.known, path, seg) {
					np, plen, err := s.plan(rt.mapID, pos, rt.target[0], rt.target[1], rt.target[2], navDangers(ds))
					if err == nil && len(np) >= 2 {
						slog.Info("path planned again around monsters seen on the way", "dangers", len(ds), "len", int(plen))
						rt = &route{path: np, target: rt.target, radius: rt.radius, plen: plen, mapID: rt.mapID, known: knownOf(ds)}
						path, seg = np, 1
						j.Update(func(st *pb.MoveStatus) { st.PathLen = plen })
						continue
					}
					for g := range knownOf(ds) { // no other way: do not try again for them
						rt.known[g] = true
					}
				}
			}
		}
		// Stop walking when the character dies (otherwise the body "walks" on in our position model).
		if now.Sub(lastAlive) >= time.Second {
			lastAlive = now
			if b, err := s.api.Brief(ctx); err == nil && b.Self != nil && b.Self.Dead && !b.Self.Ghost {
				finish("failed", "character died")
				return
			}
		}
		step := speed * float32(now.Sub(lastTick).Seconds())
		lastTick = now
		for step > 0 && seg < len(path) {
			d := dist2(pos, path[seg])
			if d <= step || d < 1e-3 {
				pos = path[seg]
				step -= d
				seg++
				continue
			}
			f := step / d
			pos = [3]float32{pos[0] + f*(path[seg][0]-pos[0]), pos[1] + f*(path[seg][1]-pos[1]), pos[2] + f*(path[seg][2]-pos[2])}
			step = 0
		}
		turned := false
		if seg < len(path) {
			if no := orient(path[seg][0]-pos[0], path[seg][1]-pos[1]); angDiff(no, o) > 0.15 {
				o, turned = no, true
			}
		}
		var remaining float32
		prev := pos
		for i := seg; i < len(path); i++ {
			remaining += dist(prev, path[i])
			prev = path[i]
		}
		j.Update(func(st *pb.MoveStatus) {
			st.X, st.Y, st.Z, st.O = pos[0], pos[1], pos[2], o
			st.DistLeft = remaining
			st.EtaS = remaining / speed
		})
		if seg >= len(path) || (dist2(pos, rt.target) <= rt.radius && math.Abs(float64(pos[2]-rt.target[2])) <= 3) {
			s.rmu.Lock()
			pending := s.next != nil
			s.rmu.Unlock()
			if !pending {
				finish("arrived", "")
				return
			}
			continue
		}
		if turned {
			_ = s.send(ctx, uint32(opcode.MSGMoveSetFacing), flags, pos[0], pos[1], pos[2], o)
		}
		if now.Sub(lastBeat) >= heartbeat {
			lastBeat = now
			if err := s.send(ctx, uint32(opcode.MSGMoveHeartbeat), flags, pos[0], pos[1], pos[2], o); err != nil {
				finish("failed", "send: "+err.Error())
				return
			}
			s.setPos(ctx, pos[0], pos[1], pos[2], o)
		}
	}
}

func angDiff(a, b float32) float32 {
	d := math.Abs(float64(a - b))
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	return float32(d)
}

// areaSpans lists the areas a path crosses, sampled every 10 yards.
func areaSpans(tm *terrain.Maps, pts [][3]float32) []*pb.AreaSpan {
	var out []*pb.AreaSpan
	var l, next float32
	for i, p := range pts {
		if i > 0 {
			q := pts[i-1]
			l += float32(math.Sqrt(float64((p[0]-q[0])*(p[0]-q[0]) + (p[1]-q[1])*(p[1]-q[1]) + (p[2]-q[2])*(p[2]-q[2]))))
		}
		if l < next && i != len(pts)-1 {
			continue
		}
		next = l + 10
		a := tm.Area(p[0], p[1])
		if n := len(out); n > 0 && out[n-1].Area == a {
			out[n-1].End = l
			continue
		}
		out = append(out, &pb.AreaSpan{Area: a, Start: l, End: l})
	}
	return out
}
