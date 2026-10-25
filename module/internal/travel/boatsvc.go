package travel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/protobuf/types/known/emptypb"

	statepb "wowagent/gen/statev1"
	pb "wowagent/gen/travelv1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/moveio"
)

// boat is a transport with its docks and the graph pier next to each (-1 when none was found).
type boat struct {
	entry  uint32
	name   string
	period uint32
	docks  []Dock
	piers  []int
}

// boatList loads the transports of our faction (and neutral ones) once, with their docks and piers.
func (s *Service) boatList(ctx context.Context, ally bool) []*boat {
	s.mu.Lock()
	bs, done := s.boatsCache, s.boatsDone
	s.mu.Unlock()
	if done {
		return bs
	}
	var r worldpb.TransportsResponse
	if err := s.api.Call(ctx, "world", "transports", &emptypb.Empty{}, &r); err != nil {
		slog.Warn("travel: transports", "err", err)
		return nil
	}
	bs = nil
	for _, t := range r.Transports {
		if (ally && strings.Contains(t.Name, "Horde")) || (!ally && strings.Contains(t.Name, "Alliance")) {
			continue
		}
		kf, period := keyFrames(s.tab.PathNodes[t.Path], float64(t.Speed), float64(t.Accel))
		ds := docks(kf)
		if period == 0 || len(ds) < 2 {
			continue
		}
		b := &boat{entry: t.Entry, name: t.Name, period: period, docks: ds}
		for i := range ds {
			b.piers = append(b.piers, s.pierNode(t.Entry, i))
		}
		bs = append(bs, b)
	}
	s.mu.Lock()
	s.boatsCache, s.boatsDone = bs, true
	s.mu.Unlock()
	return bs
}

// pierNode is the graph node of the pier of dock i of a transport (-1: none).
func (s *Service) pierNode(entry uint32, i int) int {
	for j, n := range s.graph.Nodes {
		if n.Kind == KindPier && n.Transport == entry && n.Dock == i+1 {
			return j
		}
	}
	return -1
}

// transportObj finds a transport game object in sight (they are sent for the whole map).
func (s *Service) transportObj(ctx context.Context, entry uint32) *statepb.Obj {
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "gameobject", Entry: entry, Radius: 20000, Max: 1})
	if err != nil || len(vis.Objs) == 0 || vis.Objs[0].Period == 0 {
		return nil
	}
	return vis.Objs[0]
}

// boatEdges turns each boat into edges between its docks' piers (graph nodes), by starting node;
// their cost waits for the boat (from its position in its period when it is in sight, else half a
// period).
func (s *Service) boatEdges(ctx context.Context, ally bool) map[int][]Edge {
	out := map[int][]Edge{}
	read := time.Now()
	for _, b := range s.boatList(ctx, ally) {
		b := b
		timer, period := int64(-1), b.period
		if o := s.transportObj(ctx, b.entry); o != nil {
			timer, period = int64(o.PathProgress), o.Period
		}
		for i, di := range b.docks {
			for j, dj := range b.docks {
				if i == j || b.piers[i] < 0 || b.piers[j] < 0 {
					continue
				}
				di, dj := di, dj
				from, to := s.graph.Nodes[b.piers[i]].At(), s.graph.Nodes[b.piers[j]].At()
				out[b.piers[i]] = append(out[b.piers[i]], Edge{To: b.piers[j],
					Leg: Leg{Kind: "boat", From: from, To: to, Transport: b.entry, Name: b.name,
						Dock: Point{di.Map, di.X, di.Y, di.Z}, LandDock: Point{dj.Map, dj.X, dj.Y, dj.Z}},
					Cost: func(at float32) float32 {
						wait := float32(period) / 2000
						if timer >= 0 {
							t := uint32((timer + time.Since(read).Milliseconds() + int64(at*1000)) % int64(period))
							wait = float32(waitFor(di, t, period, boardMargin)) / 1000
						}
						return wait + float32(rideTime(di, dj, period))/1000 + 10
					}})
			}
		}
	}
	return out
}

// boardMargin: board only with this much time left before the boat leaves (ms).
const boardMargin = 10000

func (s *Service) boatDock(entry uint32, at *pb.Point) (*boat, Dock, error) {
	s.mu.Lock()
	bs := s.boatsCache
	s.mu.Unlock()
	for _, b := range bs {
		if b.entry != entry || at == nil {
			continue
		}
		best := -1
		for i, d := range b.docks {
			if d.Map == at.Map && (best < 0 || dist(Point{d.Map, d.X, d.Y, d.Z}, Point{at.Map, at.X, at.Y, at.Z}) <
				dist(Point{b.docks[best].Map, b.docks[best].X, b.docks[best].Y, b.docks[best].Z}, Point{at.Map, at.X, at.Y, at.Z})) {
				best = i
			}
		}
		if best >= 0 {
			return b, b.docks[best], nil
		}
	}
	return nil, Dock{}, fmt.Errorf("transport %d has no dock near %v", entry, at)
}

func maxWait(r *pb.BoatRequest) time.Duration {
	if r.MaxWaitS == 0 {
		return 20 * time.Second
	}
	return time.Duration(r.MaxWaitS) * time.Second
}

// Board waits (at most max_wait_s) for the boat to be docked at r.dock, then steps aboard: a movement
// packet on the transport (MOVEMENTFLAG_ONTRANSPORT) puts us on its deck, at our pier's height.
func (s *Service) Board(ctx context.Context, r *pb.BoatRequest) (*pb.BoatStatus, error) {
	me, err := s.self(ctx)
	if err != nil {
		return &pb.BoatStatus{Reason: err.Error()}, nil
	}
	s.boatList(ctx, alliance(me.Race))
	_, dock, err := s.boatDock(r.Transport, r.Dock)
	if err != nil {
		return &pb.BoatStatus{Reason: err.Error()}, nil
	}
	deadline := time.Now().Add(maxWait(r))
	for {
		o := s.transportObj(ctx, r.Transport)
		if o == nil {
			return &pb.BoatStatus{Reason: "the boat is not in sight"}, nil
		}
		wait := waitFor(dock, o.PathProgress, o.Period, boardMargin)
		if wait == 0 {
			b, err := s.api.Brief(ctx)
			if err != nil || b.Self == nil {
				return &pb.BoatStatus{Reason: fmt.Sprint("brief: ", err)}, nil
			}
			me = b.Self
			dz := me.Z - dock.Z
			if dz < 2 || dz > 20 {
				dz = 6
			}
			x, y, z := dock.X, dock.Y, dock.Z+dz
			if err := moveio.SendOnTransport(ctx, s.s, me.Guid, uint32(opcode.MSGMoveHeartbeat), x, y, z, me.O, o.Guid, 0, 0, dz, 0); err != nil {
				return &pb.BoatStatus{Reason: err.Error()}, nil
			}
			_ = s.api.SetPos(ctx, x, y, z, me.O)
			slog.Info("boarded", "boat", r.Transport, "dock", fmt.Sprintf("%d:(%.0f,%.0f)", dock.Map, dock.X, dock.Y), "deck", dz,
				"progress", o.PathProgress, "period", o.Period)
			return &pb.BoatStatus{State: "aboard"}, nil
		}
		if time.Now().Add(time.Duration(wait) * time.Millisecond).After(deadline) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Until(deadline)):
			}
			return &pb.BoatStatus{State: "waiting", WaitS: wait / 1000}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(wait)*time.Millisecond + 500*time.Millisecond):
		}
	}
}

// Disembark waits (at most max_wait_s) for the boat to be docked at r.dock (after the crossing), then
// steps onto r.pier (a movement packet without the transport).
func (s *Service) Disembark(ctx context.Context, r *pb.BoatRequest) (*pb.BoatStatus, error) {
	me, err := s.self(ctx)
	if err != nil {
		return &pb.BoatStatus{Reason: err.Error()}, nil
	}
	s.boatList(ctx, alliance(me.Race))
	_, dock, err := s.boatDock(r.Transport, r.Dock)
	if err != nil {
		return &pb.BoatStatus{Reason: err.Error()}, nil
	}
	if r.Pier == nil {
		return &pb.BoatStatus{Reason: "no pier"}, nil
	}
	deadline := time.Now().Add(maxWait(r))
	for time.Now().Before(deadline) {
		b, err := s.api.Brief(ctx)
		if err == nil && b.Self != nil && b.Self.Map == dock.Map {
			if o := s.transportObj(ctx, r.Transport); o != nil {
				t := o.PathProgress
				if t >= dock.Arrive+1500 && t+3000 <= dock.Depart {
					me = b.Self
					if err := moveio.Send(ctx, s.s, me.Guid, uint32(opcode.MSGMoveHeartbeat), 0, r.Pier.X, r.Pier.Y, r.Pier.Z, me.O); err != nil {
						return &pb.BoatStatus{Reason: err.Error()}, nil
					}
					_ = s.api.SetPos(ctx, r.Pier.X, r.Pier.Y, r.Pier.Z, me.O)
					slog.Info("disembarked", "boat", r.Transport, "pier", fmt.Sprintf("%d:(%.0f,%.0f)", dock.Map, r.Pier.X, r.Pier.Y))
					return &pb.BoatStatus{State: "ashore"}, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return &pb.BoatStatus{State: "riding"}, nil
}
