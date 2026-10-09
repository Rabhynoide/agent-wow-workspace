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
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/movev1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/moveio"
	"wowagent/internal/nav"
	"wowagent/internal/terrain"
)

const (
	flagForward = 0x1
	flagWalk    = 0x100
	tick        = 100 * time.Millisecond
	heartbeat   = 500 * time.Millisecond
	maxWait     = 25
)

type job struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	st     pb.MoveStatus
	start  time.Time
}

func (j *job) snapshot() *pb.MoveStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := j.st
	s.ElapsedS = float32(time.Since(j.start).Seconds())
	if j.st.State != "moving" {
		s.ElapsedS = j.st.ElapsedS
	}
	return &s
}

type Service struct {
	pb.UnimplementedMoveServer
	session modv1.SessionClient
	dir     string
	guid    uint64

	mu     sync.Mutex
	meshes map[uint32]*nav.Mesh
	terr   map[uint32]*terrain.Maps
	maps   string
	cur    *job
	last   pb.MoveStatus
}

func Register(s grpc.ServiceRegistrar, session modv1.SessionClient) {
	dir := os.Getenv("MMAPS_DIR")
	if dir == "" {
		dir = "/mmaps"
	}
	guid, _ := strconv.ParseUint(os.Getenv("AGENT_WOW_CHARACTER_GUID"), 10, 64)
	pb.RegisterMoveServer(s, &Service{session: session, dir: dir, guid: guid, meshes: map[uint32]*nav.Mesh{}, terr: map[uint32]*terrain.Maps{}, maps: mapsDir(), last: pb.MoveStatus{State: "idle"}})
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
	req, _ := anypb.New(&emptypb.Empty{})
	resp, err := s.session.InvokeModule(ctx, &modv1.InvokeModuleRequest{Module: "state", Method: "snapshot", Request: req})
	if err != nil {
		return nil, err
	}
	var snap statepb.SnapshotResponse
	if err := resp.Result.UnmarshalTo(&snap); err != nil {
		return nil, err
	}
	if snap.Self == nil {
		return nil, fmt.Errorf("self state not available yet")
	}
	return snap.Self, nil
}

func (s *Service) setPos(ctx context.Context, x, y, z, o float32) {
	req, _ := anypb.New(&statepb.SetPosRequest{X: x, Y: y, Z: z, O: o})
	if _, err := s.session.InvokeModule(ctx, &modv1.InvokeModuleRequest{Module: "state", Method: "set_pos", Request: req}); err != nil {
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
	return out
}

func (s *Service) plan(ctx context.Context, self *statepb.SelfInfo, x, y, z float32) ([][3]float32, float32, error) {
	m, err := s.mesh(self.Map)
	if err != nil {
		return nil, 0, err
	}
	pts, corridor, err := m.PathCorridor(nav.ToRecast(self.X, self.Y, self.Z), nav.ToRecast(x, y, z), nav.GroundOnly)
	if err != nil {
		return nil, 0, err
	}
	dense := densify(m, s.terrainFor(self.Map), pts, corridor)
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
	pts, l, err := s.plan(ctx, self, r.X, r.Y, r.Z)
	if err != nil {
		return &pb.PathResponse{Error: err.Error()}, nil
	}
	return &pb.PathResponse{Points: uint32(len(pts)), Length: l}, nil
}

func (s *Service) stopCurrent() {
	s.mu.Lock()
	j := s.cur
	s.mu.Unlock()
	if j != nil {
		j.cancel()
		<-j.done
	}
}

func (s *Service) Stop(ctx context.Context, _ *emptypb.Empty) (*pb.MoveStatus, error) {
	s.stopCurrent()
	return s.current(), nil
}

func (s *Service) BeforeLogout(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	s.stopCurrent()
	return &emptypb.Empty{}, nil
}

func (s *Service) current() *pb.MoveStatus {
	s.mu.Lock()
	j, last := s.cur, s.last
	s.mu.Unlock()
	if j != nil {
		return j.snapshot()
	}
	return &last
}

func (s *Service) Status(ctx context.Context, r *pb.StatusRequest) (*pb.MoveStatus, error) {
	s.mu.Lock()
	j := s.cur
	s.mu.Unlock()
	if j != nil && r.WaitS > 0 {
		select {
		case <-j.done:
		case <-time.After(time.Duration(min(r.WaitS, maxWait)) * time.Second):
		case <-ctx.Done():
		}
	}
	return s.current(), nil
}

func (s *Service) Goto(ctx context.Context, r *pb.GotoRequest) (*pb.MoveStatus, error) {
	s.stopCurrent()
	self, err := s.self(ctx)
	if err != nil {
		return &pb.MoveStatus{State: "failed", Reason: err.Error()}, nil
	}
	if self.Dead && !self.Ghost {
		return &pb.MoveStatus{State: "failed", Reason: "character is dead", X: self.X, Y: self.Y, Z: self.Z}, nil
	}
	path, plen, err := s.plan(ctx, self, r.X, r.Y, r.Z)
	if err != nil {
		return &pb.MoveStatus{State: "failed", Reason: err.Error(), X: self.X, Y: self.Y, Z: self.Z}, nil
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
	jctx, cancel := context.WithCancel(context.Background())
	j := &job{cancel: cancel, done: make(chan struct{}), start: time.Now()}
	j.st = pb.MoveStatus{State: "moving", X: self.X, Y: self.Y, Z: self.Z, PathLen: plen, DistLeft: plen}
	s.mu.Lock()
	s.cur = j
	s.mu.Unlock()
	go s.run(jctx, j, path, [3]float32{r.X, r.Y, r.Z}, speed, flags, radius)
	wait := min(r.WaitS, maxWait)
	if r.WaitS == 0 {
		wait = 20
	}
	select {
	case <-j.done:
	case <-time.After(time.Duration(wait) * time.Second):
	case <-ctx.Done():
	}
	return s.current(), nil
}

func (s *Service) run(ctx context.Context, j *job, path [][3]float32, target [3]float32, speed float32, flags uint32, radius float32) {
	pos := path[0]
	o := float32(0)
	seg := 1
	finish := func(state, reason string) {
		j.mu.Lock()
		j.st.State, j.st.Reason = state, reason
		j.st.X, j.st.Y, j.st.Z, j.st.O = pos[0], pos[1], pos[2], o
		j.st.DistLeft, j.st.EtaS = dist2(pos, target), 0
		j.st.ElapsedS = float32(time.Since(j.start).Seconds())
		st := j.st
		j.mu.Unlock()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.send(sctx, uint32(opcode.MSGMoveStop), 0, pos[0], pos[1], pos[2], o)
		s.setPos(sctx, pos[0], pos[1], pos[2], o)
		s.mu.Lock()
		s.last = st
		s.cur = nil
		s.mu.Unlock()
		close(j.done)
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
	var remaining float32
	for i := 1; i < len(path); i++ {
		remaining += dist(path[i-1], path[i])
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
		now := time.Now()
		step := speed * float32(now.Sub(lastTick).Seconds())
		lastTick = now
		for step > 0 && seg < len(path) {
			d := dist(pos, path[seg])
			if d <= step {
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
		remaining = 0
		prev := pos
		for i := seg; i < len(path); i++ {
			remaining += dist(prev, path[i])
			prev = path[i]
		}
		j.mu.Lock()
		j.st.X, j.st.Y, j.st.Z, j.st.O = pos[0], pos[1], pos[2], o
		j.st.DistLeft = remaining
		j.st.EtaS = remaining / speed
		j.mu.Unlock()
		if seg >= len(path) || dist2(pos, target) <= radius {
			finish("arrived", "")
			return
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
