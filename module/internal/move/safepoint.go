package move

import (
	"context"
	"fmt"

	pb "wowagent/gen/movev1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/safe"
)

// maxSafeChecks bounds the paths computed for one request (best candidates first).
const maxSafeChecks = 24

func (s *Service) SafePoint(ctx context.Context, r *pb.SafePointRequest) (*pb.SafePointResult, error) {
	self, err := s.self(ctx)
	if err != nil {
		return &pb.SafePointResult{Reason: err.Error()}, nil
	}
	maxDist := r.MaxDist
	if maxDist <= 0 {
		maxDist = 30
	}
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 150, Max: 60, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return &pb.SafePointResult{Reason: err.Error()}, nil
	}
	var threats [][2]float32
	for _, o := range vis.Objs {
		threats = append(threats, [2]float32{o.X, o.Y})
	}
	tm, vm := s.terrainFor(self.Map), s.vmapFor(self.Map)
	from := [3]float32{self.X, self.Y, self.Z}
	best := &pb.SafePointResult{Threats: uint32(len(threats)), Clearance: -1}
	checks := 0
	for _, c := range safe.Candidates(r.X, r.Y, r.MinDist, maxDist, threats) {
		if checks >= maxSafeChecks || (best.Ok && c.Clearance <= best.Clearance) {
			break // sorted: nothing better can follow
		}
		z, ok := floor(c.X, c.Y, r.Z+10, tm, vm)
		if !ok {
			continue
		}
		checks++
		path, plen, err := s.plan(self.Map, from, c.X, c.Y, z, nil)
		if err != nil || len(path) == 0 {
			continue
		}
		straight := dist(from, [3]float32{c.X, c.Y, z})
		if plen > 3*straight+40 {
			continue // long detour: probably another level or the other side of a wall
		}
		clear := c.Clearance
		if r.PathClear {
			for _, p := range path {
				clear = min(clear, safe.Clearance(p[0], p[1], threats))
			}
		}
		if clear > best.Clearance {
			end := path[len(path)-1]
			best = &pb.SafePointResult{Ok: true, X: end[0], Y: end[1], Z: end[2], Clearance: clear, Threats: uint32(len(threats))}
		}
	}
	if !best.Ok {
		best.Reason = fmt.Sprintf("no reachable point within %.0f yards (%d checked)", maxDist, checks)
	}
	return best, nil
}
