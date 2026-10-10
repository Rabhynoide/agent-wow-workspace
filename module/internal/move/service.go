// Package move walks the character along navmesh paths.
package move

import (
	"context"
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
	"wowagent/internal/terrain"
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
	maps   string

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
		meshes: map[uint32]*nav.Mesh{}, terr: map[uint32]*terrain.Maps{}, maps: mapsDir()})
}

func mapsDir() string {
	if d := os.Getenv("MAPS_DIR"); d != "" {
		return d
	}
	return "/maps"
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
	for i := range p {
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

func (s *Service) plan(mapID uint32, from [3]float32, x, y, z float32) ([][3]float32, float32, error) {
	m, err := s.mesh(mapID)
	if err != nil {
		return nil, 0, err
	}
	pts, corridor, err := m.PathCorridor(nav.ToRecast(from[0], from[1], from[2]), nav.ToRecast(x, y, z), nav.GroundOnly)
	if err != nil {
		return nil, 0, err
	}
	dense := densify(m, s.terrainFor(mapID), pts, corridor)
	var l float32
	for i := 1; i < len(dense); i++ {
		l += dist(dense[i-1], dense[i])
	}
	return dense, l, nil
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
	pts, l, err := s.plan(self.Map, [3]float32{self.X, self.Y, self.Z}, r.X, r.Y, r.Z)
	if err != nil {
		return &pb.PathResponse{Error: err.Error()}, nil
	}
	return &pb.PathResponse{Points: uint32(len(pts)), Length: l}, nil
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
	// Already walking with the same gait: plan from the live position and swap the route in place,
	// so chasing a moving target does not stutter with stop/start packets.
	if j := s.jobs.Cur(); j != nil {
		cur := j.Snap()
		path, plen, err := s.plan(self.Map, [3]float32{cur.X, cur.Y, cur.Z}, r.X, r.Y, r.Z)
		if err == nil && len(path) >= 2 {
			s.rmu.Lock()
			ok := s.walking && s.walkFlags == flags
			if ok {
				s.next = &route{path: path, target: target, radius: radius, plen: plen}
			}
			s.rmu.Unlock()
			if ok {
				return nil, nil
			}
		}
	}
	path, plen, err := s.plan(self.Map, [3]float32{self.X, self.Y, self.Z}, r.X, r.Y, r.Z)
	if err != nil {
		s.jobs.StartLocked(&pb.MoveStatus{State: "failed", Reason: err.Error(), X: self.X, Y: self.Y, Z: self.Z}, func(*job) {})
		return &pb.MoveStatus{State: "failed", Reason: err.Error(), X: self.X, Y: self.Y, Z: self.Z}, nil
	}
	s.jobs.StartLocked(&pb.MoveStatus{State: "moving", X: self.X, Y: self.Y, Z: self.Z, PathLen: plen, DistLeft: plen}, func(j *job) {
		s.run(j, &route{path: path, target: target, radius: radius, plen: plen}, speed, flags)
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
