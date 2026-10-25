package travel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	statepb "wowagent/gen/statev1"
	pb "wowagent/gen/travelv1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/api"
	"wowagent/internal/dbc"
	"wowagent/internal/moveio"
	"wowagent/internal/wire"
)

const (
	npcFlightMaster = 0x2000
	npcInnkeeper    = 0x10000
	unitTaxiFlight  = 0x00100000
	hearthItem      = 6948
	hearthSpell     = 8690
	hearthCooldown  = 30 * time.Minute // Spell.dbc 8690 CategoryRecoveryTime
	taxiMaskSize    = 14
)

type Service struct {
	pb.UnimplementedTravelServer
	s   modv1.SessionClient
	api api.C
	dir string // where the known nodes are kept (/logs)

	tab       *dbc.Travel
	tabErr    error
	zoneLevel map[uint32]int32 // zone -> lowest exploration level of its areas

	mu       sync.Mutex
	guid     uint64
	known    map[uint32]bool
	loaded   bool
	bind     *Point
	hearthAt time.Time
	masters  map[uint32]*worldpb.Spawn // node -> its flight master (nil: none found)
	ports    []*worldpb.Teleport
	// enters: area trigger of each portal entry point; fixed: the ground point inside it we can walk
	// to (the trigger centre floats above the floor: the Stormwind tram entrance is 8 yards up)
	enters map[Point]*dbc.AreaTrigger
	fixed  map[Point]Point
	// boatsCache: transports of our faction with docks and piers (boatsvc.go)
	boatsCache []*boat
	boatsDone  bool
	walks      map[walkCacheKey]walkCacheRes // walks checked this session

	shows   chan []byte // SMSG_SHOWTAXINODES
	learned chan struct{}
	replies chan uint32 // SMSG_ACTIVATETAXIREPLY
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	sv := &Service{s: s, api: api.C{S: s}, dir: "/logs", known: map[uint32]bool{}, masters: map[uint32]*worldpb.Spawn{},
		enters: map[Point]*dbc.AreaTrigger{}, fixed: map[Point]Point{}, walks: map[walkCacheKey]walkCacheRes{},
		shows: make(chan []byte, 4), learned: make(chan struct{}, 4), replies: make(chan uint32, 4)}
	dir := os.Getenv("DBC_DIR")
	if dir == "" {
		dir = "/dbc"
	}
	sv.tab, sv.tabErr = dbc.LoadTravel(dir)
	if sv.tabErr != nil {
		slog.Error("travel tables", "err", sv.tabErr)
	} else {
		sv.zoneLevel = zoneLevels(sv.tab)
	}
	pb.RegisterTravelServer(g, sv)
}

func zoneLevels(t *dbc.Travel) map[uint32]int32 {
	out := map[uint32]int32{}
	for _, a := range t.Areas {
		if a.Zone != 0 && a.Level > 0 && (out[a.Zone] == 0 || a.Level < out[a.Zone]) {
			out[a.Zone] = a.Level
		}
	}
	return out
}

// areaLevel is the exploration level of an area, or of its zone's easiest area.
func (s *Service) areaLevel(id uint32) int32 {
	a := s.tab.Areas[id]
	if a == nil {
		return 0
	}
	if a.Level > 0 {
		return a.Level
	}
	if a.Zone == 0 {
		return s.zoneLevel[a.ID]
	}
	return s.zoneLevel[a.Zone]
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	b := p.GetPayload()
	switch p.GetOpcode() {
	case uint32(opcode.SMSGShowtaxinodes):
		s.readMask(b)
		select {
		case s.shows <- b:
		default:
		}
	case uint32(opcode.SMSGNewTaxiPath):
		select {
		case s.learned <- struct{}{}:
		default:
		}
	case uint32(opcode.SMSGActivatetaxireply):
		r := wire.NewReader(b)
		code := r.U32()
		select {
		case s.replies <- code:
		default:
		}
	case uint32(opcode.SMSGBindpointupdate):
		r := wire.NewReader(b)
		x, y, z, m := r.F32(), r.F32(), r.F32(), r.U32()
		if !r.Err {
			s.mu.Lock()
			s.bind = &Point{Map: m, X: x, Y: y, Z: z}
			s.mu.Unlock()
			slog.Info("bind point", "map", m, "x", x, "y", y)
			s.save()
		}
	}
	return &emptypb.Empty{}, nil
}

// readMask stores the known nodes of SMSG_SHOWTAXINODES (u32 1, guid, current node, 14 u32 mask).
func (s *Service) readMask(b []byte) {
	r := wire.NewReader(b)
	r.U32()
	r.U64()
	r.U32()
	known := map[uint32]bool{}
	for i := 0; i < taxiMaskSize; i++ {
		m := r.U32()
		for bit := 0; bit < 32; bit++ {
			if m&(1<<bit) != 0 {
				known[uint32(i*32+bit+1)] = true
			}
		}
	}
	if r.Err {
		return
	}
	s.mu.Lock()
	s.known, s.loaded = known, true
	s.mu.Unlock()
	s.save()
}

type saved struct {
	Nodes    []uint32  `json:"nodes"`
	Bind     *Point    `json:"bind,omitempty"`
	HearthAt time.Time `json:"hearth_at"`
}

func (s *Service) file() string { return filepath.Join(s.dir, fmt.Sprintf("travel_%x.json", s.guid)) }

func (s *Service) save() {
	s.mu.Lock()
	if s.guid == 0 {
		s.mu.Unlock()
		return
	}
	sv := saved{Bind: s.bind, HearthAt: s.hearthAt}
	for n := range s.known {
		sv.Nodes = append(sv.Nodes, n)
	}
	f := s.file()
	s.mu.Unlock()
	b, _ := json.Marshal(sv)
	if err := os.WriteFile(f, b, 0o644); err != nil {
		slog.Warn("travel: save", "err", err)
	}
}

// Race capitals known from the start (PlayerTaxi::InitTaxiNodesForLevel).
var raceNodes = map[uint32][]uint32{1: {2}, 2: {23}, 3: {6}, 4: {26, 27}, 5: {11}, 6: {22}, 7: {6}, 8: {23}, 10: {82}, 11: {94}}

// self loads the character, and the saved nodes and bind point the first time.
func (s *Service) self(ctx context.Context) (*statepb.SelfInfo, error) {
	b, err := s.api.Brief(ctx)
	if err != nil {
		return nil, err
	}
	if b.Self == nil {
		return nil, fmt.Errorf("no character")
	}
	me := b.Self
	s.mu.Lock()
	first := s.guid != me.Guid
	s.guid = me.Guid
	s.mu.Unlock()
	if first {
		var sv saved
		data, err := os.ReadFile(s.file())
		s.mu.Lock()
		if err == nil && json.Unmarshal(data, &sv) == nil {
			if !s.loaded {
				for _, n := range sv.Nodes {
					s.known[n] = true
				}
			}
			if s.bind == nil {
				s.bind = sv.Bind
			}
			s.hearthAt = sv.HearthAt
		}
		for _, n := range raceNodes[me.Race] {
			s.known[n] = true
		}
		s.mu.Unlock()
	}
	return me, nil
}

func alliance(race uint32) bool {
	switch race {
	case 1, 3, 4, 7, 11:
		return true
	}
	return false
}

// usable tells whether our faction has a mount at a node.
func usable(n *dbc.TaxiNode, ally bool) bool {
	if ally {
		return n.AllianceMount != 0
	}
	return n.HordeMount != 0
}

// master finds the flight master of a node (spawn with the flight master flag within 60 yards).
func (s *Service) master(ctx context.Context, n *dbc.TaxiNode) *worldpb.Spawn {
	s.mu.Lock()
	m, ok := s.masters[n.ID]
	s.mu.Unlock()
	if ok {
		return m
	}
	var r worldpb.FindNpcResponse
	if err := s.api.Call(ctx, "world", "find_npc", &worldpb.FindNpcRequest{MapFilter: true, Map: n.Map, X: n.X, Y: n.Y, Radius: 60, Max: 1, NpcFlags: npcFlightMaster}, &r); err != nil {
		return nil
	}
	if len(r.Spawns) > 0 {
		m = r.Spawns[0]
	}
	s.mu.Lock()
	s.masters[n.ID] = m
	s.mu.Unlock()
	return m
}

// npcNear returns the nearest unit with these NPC flags within radius.
func (s *Service) npcNear(ctx context.Context, flags uint32, radius float32) *statepb.Obj {
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", NpcFlags: flags, Radius: radius, Max: 1, AliveOnly: true})
	if err != nil || len(vis.Objs) == 0 {
		return nil
	}
	return vis.Objs[0]
}

func drain[T any](c chan T) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

// Discover talks to the flight master in reach: learns its node if new, and reads the known nodes.
func (s *Service) Discover(ctx context.Context, _ *emptypb.Empty) (*pb.TravelResult, error) {
	if _, err := s.self(ctx); err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	npc := s.npcNear(ctx, npcFlightMaster, 8)
	if npc == nil {
		return &pb.TravelResult{Reason: "no flight master within 8 yards"}, nil
	}
	if err := s.queryNodes(ctx, npc.Guid); err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	return &pb.TravelResult{Ok: true}, nil
}

// queryNodes sends CMSG_TAXIQUERYAVAILABLENODES until the server answers with the node list (a new
// node is learned first: SMSG_NEW_TAXI_PATH, then a second query shows the list).
func (s *Service) queryNodes(ctx context.Context, guid uint64) error {
	w := &wire.Writer{}
	w.U64(guid)
	for try := 0; try < 3; try++ {
		drain(s.shows)
		drain(s.learned)
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGTaxiqueryavailablenodes), w.B); err != nil {
			return err
		}
		select {
		case <-s.shows:
			return nil
		case <-s.learned:
			slog.Info("new flight path learned")
			continue
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("the flight master did not answer")
}

// Fly takes the flight through nodes from the flight master in reach (nodes[0] is its node).
func (s *Service) Fly(ctx context.Context, r *pb.FlyRequest) (*pb.TravelResult, error) {
	me, err := s.self(ctx)
	if err != nil || s.tabErr != nil {
		return &pb.TravelResult{Reason: fmt.Sprint(err, s.tabErr)}, nil
	}
	if len(r.Nodes) < 2 {
		return &pb.TravelResult{Reason: "need at least two nodes"}, nil
	}
	npc := s.npcNear(ctx, npcFlightMaster, 8)
	if npc == nil {
		return &pb.TravelResult{Reason: "no flight master within 8 yards"}, nil
	}
	if err := s.queryNodes(ctx, npc.Guid); err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	w := &wire.Writer{}
	w.U64(npc.Guid)
	w.U32(uint32(len(r.Nodes)))
	for _, n := range r.Nodes {
		w.U32(n)
	}
	drain(s.replies)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGActivatetaxiexpress), w.B); err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	// The server answers only on a refusal (ERR_TAXIOK is not sent for an express flight in AC):
	// wait for the taxi flag instead.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case code := <-s.replies:
			if code != 0 {
				return &pb.TravelResult{Reason: fmt.Sprintf("flight refused (code %d: %s)", code, taxiError(code))}, nil
			}
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if b, err := s.api.Brief(ctx); err == nil && b.Self != nil && b.Self.UnitFlags&unitTaxiFlight != 0 {
			var secs float32
			for i := 1; i < len(r.Nodes); i++ {
				secs += s.pathLength(r.Nodes[i-1], r.Nodes[i]) / FlightSpeed
			}
			last := s.tab.Nodes[r.Nodes[len(r.Nodes)-1]]
			slog.Info("flying", "nodes", r.Nodes, "to", last.Name, "s", int(secs))
			return &pb.TravelResult{Ok: true, Seconds: secs, At: &pb.Point{Map: last.Map, X: last.X, Y: last.Y, Z: last.Z}}, nil
		}
	}
	slog.Info("flight did not start", "nodes", r.Nodes, "level", me.Level)
	return &pb.TravelResult{Reason: "the flight did not start"}, nil
}

func taxiError(code uint32) string {
	switch code {
	case 1:
		return "unspecified server error"
	case 2:
		return "no such path"
	case 3:
		return "not enough money"
	case 4:
		return "too far away"
	case 5:
		return "no vendor nearby"
	case 6:
		return "node not visited"
	case 7:
		return "player busy"
	case 8:
		return "already mounted"
	case 9:
		return "shapeshifted"
	case 10:
		return "moving"
	case 11:
		return "same node"
	case 12:
		return "not standing"
	}
	return "?"
}

func (s *Service) pathLength(a, b uint32) float32 {
	for _, p := range s.tab.Paths {
		if p.From == a && p.To == b {
			return p.Length
		}
	}
	return 0
}

// flightRoutes computes, from each known usable node, the fastest flight to every other known node
// (Dijkstra on TaxiPath through known nodes only: the server refuses unvisited ones).
func (s *Service) flightRoutes(known map[uint32]bool, ally bool) func(a, b uint32) (float32, []uint32, bool) {
	adj := map[uint32][]*dbc.TaxiPath{}
	for _, p := range s.tab.Paths {
		na, nb := s.tab.Nodes[p.From], s.tab.Nodes[p.To]
		if na == nil || nb == nil || !known[p.From] || !known[p.To] || !usable(na, ally) || !usable(nb, ally) || p.Length == 0 {
			continue
		}
		adj[p.From] = append(adj[p.From], p)
	}
	type res struct {
		cost map[uint32]float32
		prev map[uint32]uint32
	}
	cache := map[uint32]*res{}
	run := func(src uint32) *res {
		if r, ok := cache[src]; ok {
			return r
		}
		r := &res{cost: map[uint32]float32{src: 0}, prev: map[uint32]uint32{}}
		done := map[uint32]bool{}
		for {
			var u uint32
			best := float32(-1)
			for n, c := range r.cost {
				if !done[n] && (best < 0 || c < best) {
					u, best = n, c
				}
			}
			if best < 0 {
				break
			}
			done[u] = true
			for _, p := range adj[u] {
				c := best + p.Length/FlightSpeed
				if old, ok := r.cost[p.To]; !ok || c < old {
					r.cost[p.To], r.prev[p.To] = c, u
				}
			}
		}
		cache[src] = r
		return r
	}
	return func(a, b uint32) (float32, []uint32, bool) {
		r := run(a)
		c, ok := r.cost[b]
		if !ok || a == b {
			return 0, nil, false
		}
		path := []uint32{b}
		for n := b; n != a; {
			n = r.prev[n]
			path = append([]uint32{n}, path...)
		}
		return c, path, true
	}
}

// Ways between the open-world maps: Eastern Kingdoms, Kalimdor, Outland, Northrend, Deeprun Tram.
var openMaps = map[uint32]bool{0: true, 1: true, 530: true, 571: true, 369: true}

func (s *Service) portals(ctx context.Context) []Portal {
	s.mu.Lock()
	ports := s.ports
	s.mu.Unlock()
	if ports == nil {
		var r worldpb.TeleportsResponse
		if err := s.api.Call(ctx, "world", "teleports", &emptypb.Empty{}, &r); err != nil {
			slog.Warn("travel: teleports", "err", err)
			return nil
		}
		ports = r.Teleports
		s.mu.Lock()
		s.ports = ports
		s.mu.Unlock()
	}
	var out []Portal
	for _, t := range ports {
		tr := s.tab.Triggers[t.Id]
		if tr == nil || !openMaps[tr.Map] || !openMaps[t.Map] {
			continue
		}
		enter := Point{tr.Map, tr.X, tr.Y, tr.Z}
		s.mu.Lock()
		s.enters[enter] = tr
		s.mu.Unlock()
		out = append(out, Portal{Trigger: t.Id, Name: t.Name, Enter: enter, Exit: Point{t.Map, t.X, t.Y, t.Z}})
	}
	return out
}

// inTrigger mirrors Player::IsInAreaTriggerRadius with no tolerance (CMSG_AREATRIGGER), with a
// margin of a yard.
func inTrigger(tr *dbc.AreaTrigger, p Point) bool {
	if p.Map != tr.Map {
		return false
	}
	if tr.Radius > 0 {
		return dist(p, Point{tr.Map, tr.X, tr.Y, tr.Z}) <= tr.Radius-1
	}
	// Box: rotate into the box frame.
	dx, dy := float64(p.X-tr.X), float64(p.Y-tr.Y)
	c, sn := math.Cos(float64(-tr.BoxO)), math.Sin(float64(-tr.BoxO))
	lx, ly := dx*c-dy*sn, dx*sn+dy*c
	return math.Abs(lx) <= float64(tr.BoxX/2-1) && math.Abs(ly) <= float64(tr.BoxY/2-1) && math.Abs(float64(p.Z-tr.Z)) <= float64(tr.BoxZ/2-1)
}

// triggerSpots: points inside a trigger to try walking to, the centre first, then lower and around.
func triggerSpots(tr *dbc.AreaTrigger) []Point {
	var out []Point
	h := tr.Radius
	if h == 0 {
		h = min(tr.BoxX, tr.BoxY) / 2
	}
	for _, dz := range []float32{0, -3, -6, -9} {
		for _, off := range [][2]float32{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			out = append(out, Point{tr.Map, tr.X + off[0]*h*0.4, tr.Y + off[1]*h*0.4, tr.Z + dz})
		}
	}
	return out
}

// Danger rule of the walks: no point farther than dangerEnds from both ends in an area more than
// dangerAbove levels above ours (the walk from Elwynn to Loch Modan went through the Burning Steppes,
// level 50+, and died: bench 8 -> 12 of 2026-10-24).
const (
	dangerAbove = 5
	dangerEnds  = 150
)

// errBusy: planning ran out of its time budget; the walks checked so far are kept and the next
// call goes on (the core cancels module calls after 30 s: a plan from Dun Morogh to Elwynn timed out
// and the walk that followed went through the Searing Gorge, 3 deaths, 2026-10-25).
var errBusy = errors.New("busy")

type walkCacheKey struct {
	a, b  [4]int32
	level uint32
}

type walkCacheRes struct {
	secs float32
	ok   bool
}

func quant(p Point) [4]int32 {
	return [4]int32{int32(p.Map), int32(math.Round(float64(p.X) / 2)), int32(math.Round(float64(p.Y) / 2)), int32(math.Round(float64(p.Z) / 4))}
}

// planBudget: a plan stops checking walks after this long (errBusy), under the 30 s call limit.
const planBudget = 20 * time.Second

func (s *Service) checkWalk(level uint32, deadline time.Time) func(a, b Point) (float32, bool, error) {
	return func(a, b Point) (float32, bool, error) {
		s.mu.Lock()
		tr := s.enters[b]
		fix, fixed := s.fixed[b]
		if fixed {
			b = fix
		}
		key := walkCacheKey{quant(a), quant(b), level}
		cached, hit := s.walks[key]
		s.mu.Unlock()
		if hit {
			return cached.secs, cached.ok, nil
		}
		if dist(a, b) < 30 && tr == nil {
			return dist(a, b) / RunSpeed, true, nil
		}
		if time.Now().After(deadline) {
			return 0, false, errBusy
		}
		secs, ok, err := s.walkFor(a, b, tr, fixed, level)
		if err != nil {
			return 0, false, err
		}
		s.mu.Lock()
		s.walks[key] = walkCacheRes{secs, ok}
		s.mu.Unlock()
		return secs, ok, nil
	}
}

// walkFor checks one walk with move.path (its own context: a canceled plan call must not turn the
// walks it was checking into refusals).
func (s *Service) walkFor(a, b Point, tr *dbc.AreaTrigger, fixed bool, level uint32) (float32, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	res, err := s.api.PathFrom(ctx, a.Map, a.X, a.Y, a.Z, b.X, b.Y, b.Z)
	if err != nil {
		return 0, false, err
	}
	if tr != nil && !fixed {
		// A portal: the walk must end inside its trigger.
		ok := res.Error == "" && res.End != nil && inTrigger(tr, Point{b.Map, res.End.X, res.End.Y, res.End.Z})
		for _, spot := range triggerSpots(tr) {
			if ok {
				break
			}
			if res, err = s.api.PathFrom(ctx, a.Map, a.X, a.Y, a.Z, spot.X, spot.Y, spot.Z); err != nil {
				return 0, false, err
			}
			ok = res.Error == "" && res.End != nil && inTrigger(tr, Point{b.Map, res.End.X, res.End.Y, res.End.Z})
		}
		if !ok {
			slog.Info("travel: no walk into the trigger", "from", a, "trigger", tr.ID)
			return 0, false, nil
		}
		end := Point{b.Map, res.End.X, res.End.Y, res.End.Z}
		s.mu.Lock()
		s.fixed[b] = end
		s.mu.Unlock()
		slog.Info("travel: trigger spot", "trigger", tr.ID, "spot", fmt.Sprintf("(%.1f,%.1f,%.1f)", end.X, end.Y, end.Z))
	}
	if res.Error != "" {
		slog.Info("travel: no walk", "from", a, "to", b, "err", res.Error)
		return 0, false, nil
	}
	for _, sp := range res.Areas {
		if sp.End < dangerEnds || sp.Start > res.Length-dangerEnds {
			continue
		}
		if lv := s.areaLevel(sp.Area); lv > int32(level)+dangerAbove {
			name := ""
			if ar := s.tab.Areas[sp.Area]; ar != nil {
				name = ar.Name
			}
			slog.Info("travel: walk too dangerous", "from", a, "to", b, "area", name, "level", lv)
			return 0, false, nil
		}
	}
	return res.Length / RunSpeed, true, nil
}

// Plan finds the fastest journey to the point.
func (s *Service) Plan(ctx context.Context, r *pb.PlanRequest) (*pb.PlanResponse, error) {
	me, err := s.self(ctx)
	if err != nil {
		return &pb.PlanResponse{Reason: err.Error()}, nil
	}
	if s.tabErr != nil {
		return &pb.PlanResponse{Reason: s.tabErr.Error()}, nil
	}
	if r.To == nil {
		return &pb.PlanResponse{Reason: "no destination"}, nil
	}
	level := r.Level
	if level == 0 {
		level = me.Level
	}
	ally := alliance(me.Race)
	start, goal := Point{me.Map, me.X, me.Y, me.Z}, Point{r.To.Map, r.To.X, r.To.Y, r.To.Z}
	s.mu.Lock()
	known := map[uint32]bool{}
	for n := range s.known {
		known[n] = true
	}
	var hearth *Point
	if s.bind != nil && time.Since(s.hearthAt) > hearthCooldown {
		b := *s.bind
		hearth = &b
	}
	s.mu.Unlock()
	g := Graph{Flight: s.flightRoutes(known, ally), Portals: append(s.portals(ctx), s.boatPortals(ctx, ally)...), Walk: s.checkWalk(level, time.Now().Add(planBudget))}
	if hearth != nil && s.hasItem(ctx, hearthItem) {
		g.Hearth = hearth
	}
	for id := range known {
		n := s.tab.Nodes[id]
		if n == nil || !usable(n, ally) || !openMaps[n.Map] {
			continue
		}
		m := s.master(ctx, n)
		if m == nil {
			continue
		}
		g.Nodes = append(g.Nodes, FlightNode{ID: id, Name: n.Name, At: Point{m.Map, m.X, m.Y, m.Z}, NpcEnt: m.Entry})
	}
	legs, total, err := Plan(g, start, goal)
	out := &pb.PlanResponse{}
	if errors.Is(err, errBusy) {
		out.Busy, out.Reason = true, "planning not finished, call again"
		return out, nil
	}
	if direct, ok, _ := g.Walk(start, goal); ok {
		out.WalkSeconds = direct
	}
	if err != nil {
		out.Reason = err.Error()
		return out, nil
	}
	out.Ok, out.Seconds = true, total
	s.mu.Lock()
	for i := range legs {
		if f, ok := s.fixed[legs[i].To]; ok && legs[i].Kind == "walk" {
			legs[i].To = f
		}
		if f, ok := s.fixed[legs[i].From]; ok && legs[i].Kind == "portal" {
			legs[i].From = f
		}
	}
	s.mu.Unlock()
	for _, l := range legs {
		out.Legs = append(out.Legs, &pb.Leg{Kind: l.Kind, From: pt(l.From), To: pt(l.To), Nodes: l.Nodes, Trigger: l.Trigger,
			NpcEntry: l.Npc, Seconds: l.Seconds, Name: l.Name, Transport: l.Transport, Dock: pt(l.Dock), LandDock: pt(l.LandDock)})
	}
	return out, nil
}

func pt(p Point) *pb.Point { return &pb.Point{Map: p.Map, X: p.X, Y: p.Y, Z: p.Z} }

func (s *Service) hasItem(ctx context.Context, entry uint32) bool {
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return false
	}
	for _, it := range snap.Items {
		if it.Entry == entry {
			return true
		}
	}
	return false
}

// Trigger enters an area trigger we stand in (CMSG_AREATRIGGER) and waits for the teleport.
func (s *Service) Trigger(ctx context.Context, r *pb.TriggerRequest) (*pb.TravelResult, error) {
	me, err := s.self(ctx)
	if err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	w := &wire.Writer{}
	w.U32(r.Id)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGAreatrigger), w.B); err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	return s.waitArrival(ctx, me, r.Map, 20*time.Second)
}

// waitArrival waits until we are on map (or moved far on the same map) after a teleport.
func (s *Service) waitArrival(ctx context.Context, before *statepb.SelfInfo, m uint32, max time.Duration) (*pb.TravelResult, error) {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		b, err := s.api.Brief(ctx)
		if err != nil || b.Self == nil {
			continue
		}
		me := b.Self
		moved := dist(Point{me.Map, me.X, me.Y, me.Z}, Point{before.Map, before.X, before.Y, before.Z}) > 100
		if me.Map == m && (m != before.Map || moved) {
			time.Sleep(2 * time.Second) // let the new map's objects arrive
			return &pb.TravelResult{Ok: true, At: &pb.Point{Map: me.Map, X: me.X, Y: me.Y, Z: me.Z}}, nil
		}
	}
	return &pb.TravelResult{Reason: "no teleport"}, nil
}

// Bind makes the inn of the innkeeper in reach our home (CMSG_BINDER_ACTIVATE).
func (s *Service) Bind(ctx context.Context, _ *emptypb.Empty) (*pb.TravelResult, error) {
	if _, err := s.self(ctx); err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	npc := s.npcNear(ctx, npcInnkeeper, 8)
	if npc == nil {
		return &pb.TravelResult{Reason: "no innkeeper within 8 yards"}, nil
	}
	w := &wire.Writer{}
	w.U64(npc.Guid)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGBinderActivate), w.B); err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	// SMSG_BINDPOINTUPDATE updates s.bind (OnPacket).
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		s.mu.Lock()
		b := s.bind
		s.mu.Unlock()
		if b != nil && dist(*b, Point{b.Map, npc.X, npc.Y, npc.Z}) < 60 {
			return &pb.TravelResult{Ok: true, At: pt(*b)}, nil
		}
	}
	return &pb.TravelResult{Reason: "bind point not updated"}, nil
}

// Hearth uses the hearthstone and waits for the teleport home.
func (s *Service) Hearth(ctx context.Context, _ *emptypb.Empty) (*pb.TravelResult, error) {
	me, err := s.self(ctx)
	if err != nil {
		return &pb.TravelResult{Reason: err.Error()}, nil
	}
	s.mu.Lock()
	b := s.bind
	s.mu.Unlock()
	if b == nil {
		return &pb.TravelResult{Reason: "bind point unknown"}, nil
	}
	res, err := s.api.UseItem(ctx, hearthItem, hearthSpell, 0, "")
	if err != nil || !res.Ok {
		return &pb.TravelResult{Reason: fmt.Sprintf("hearthstone: %v %s", err, res.GetReason())}, nil
	}
	out, err := s.waitArrival(ctx, me, b.Map, 20*time.Second)
	if err == nil && out.Ok {
		s.mu.Lock()
		s.hearthAt = time.Now()
		s.mu.Unlock()
		s.save()
	}
	return out, err
}

// NearUnknown finds a flight master of our faction within radius whose node we do not know.
func (s *Service) NearUnknown(ctx context.Context, r *pb.NearRequest) (*pb.NearResponse, error) {
	me, err := s.self(ctx)
	if err != nil || s.tabErr != nil {
		return &pb.NearResponse{}, nil
	}
	ally := alliance(me.Race)
	here := Point{me.Map, me.X, me.Y, me.Z}
	s.mu.Lock()
	known := s.known
	s.mu.Unlock()
	for id, n := range s.tab.Nodes {
		if n.Map != me.Map || known[id] || !usable(n, ally) || dist(here, Point{n.Map, n.X, n.Y, n.Z}) > r.Radius+60 {
			continue
		}
		m := s.master(ctx, n)
		if m == nil || dist(here, Point{m.Map, m.X, m.Y, m.Z}) > r.Radius {
			continue
		}
		return &pb.NearResponse{Found: true, Node: id, Name: n.Name, NpcEntry: m.Entry, At: &pb.Point{Map: m.Map, X: m.X, Y: m.Y, Z: m.Z}}, nil
	}
	return &pb.NearResponse{}, nil
}

// Known lists the known nodes, the bind point and the hearthstone cooldown.
func (s *Service) Known(ctx context.Context, _ *emptypb.Empty) (*pb.KnownResponse, error) {
	if _, err := s.self(ctx); err != nil {
		return &pb.KnownResponse{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &pb.KnownResponse{}
	for n := range s.known {
		out.Nodes = append(out.Nodes, n)
	}
	if s.bind != nil {
		out.Bind, out.BindKnown = pt(*s.bind), true
	}
	if left := hearthCooldown - time.Since(s.hearthAt); left > 0 {
		out.HearthCooldownS = uint32(left.Seconds())
	}
	return out, nil
}
