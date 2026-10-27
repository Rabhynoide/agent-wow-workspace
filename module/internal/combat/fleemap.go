package combat

import (
	"context"
	"log/slog"
	"math"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/danger"
	"wowagent/internal/safe"
)

// Flights chosen on the danger map (tuning flee_danger): the way back along our trail and a dozen
// ways around us, each one the real walk on the navmesh (the tunnels of a mine turn: a straight line
// said nothing of where the walk goes), scored by the danger it meets in its first fleeScan yards. The
// danger is the idle monsters in sight: their aggro disc plus the 10 yards in which our chasers will
// call them (fleeing 4 kobolds at 86 % in Fargodeep brought 10 of them in 14 s, 2026-10-27).

const (
	fleeScan       = 60  // yards of each way scored
	fleeWays       = 12  // directions tried around us
	fleeWayDist    = 40  // yards to the end of each way
	fleeTrailBonus = 0.8 // the way we came: walkable, cleared, known
	fleeTowardsAtt = 15  // cost of a way that starts straight at the attackers (scaled by the cosine)
)

// dangerField is the danger map around us: the idle hostile monsters in sight, the attackers left
// out (they follow us whatever the way).
func (s *Service) dangerField(ctx context.Context, me *statepb.SelfInfo, att []*statepb.Obj) *danger.Field {
	skip := map[uint64]bool{}
	for _, a := range att {
		skip[a.Guid] = true
	}
	f := &danger.Field{}
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 100, Max: 80, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return f
	}
	rangeOf := s.aggroOf(ctx, me.Level)
	for _, o := range r.Objs {
		if skip[o.Guid] || o.Target == me.Guid {
			continue
		}
		f.Src = append(f.Src, danger.Source{X: o.X, Y: o.Y, Z: o.Z, R: rangeOf(o), Halo: safe.AssistRange + 2,
			W: danger.Weight(int(o.Level)-int(me.Level), false)})
	}
	return f
}

// flightWay is one candidate: its points (from us), the danger met, whether it is our trail.
type flightWay struct {
	pts   [][3]float32
	cost  float32
	trail bool
	deg   int
}

// chooseFlight picks the way to run: it returns the legs to follow (every trailLeg yards), the
// direction to go on with after them, and the way's name for the log.
func (s *Service) chooseFlight(ctx context.Context, me *statepb.SelfInfo, att []*statepb.Obj, trailLegs []trailPt) ([]trailPt, [2]float64, string) {
	f := s.dangerField(ctx, me, att)
	away := awayFrom(me, att)
	var ways []flightWay
	if len(trailLegs) > 0 {
		pts := [][3]float32{{me.X, me.Y, me.Z}}
		for _, l := range trailLegs {
			pts = append(pts, [3]float32{l.x, l.y, l.z})
		}
		ways = append(ways, flightWay{pts: pts, trail: true, deg: -1})
	}
	base := math.Atan2(away[1], away[0])
	for i := 0; i < fleeWays; i++ {
		a := base + float64(i)*2*math.Pi/fleeWays
		x, y := me.X+float32(math.Cos(a)*fleeWayDist), me.Y+float32(math.Sin(a)*fleeWayDist)
		res, err := s.api.PathFromSnapPoints(ctx, me.Map, me.X, me.Y, me.Z, x, y, me.Z, 12)
		if err != nil || res.Error != "" || res.End == nil || len(res.Pts) < 2 || res.Length > 2*fleeWayDist ||
			dist2(me.X, me.Y, res.End.X, res.End.Y) < fleeWayDist/2 {
			continue
		}
		pts := make([][3]float32, len(res.Pts))
		for k, p := range res.Pts {
			pts[k] = [3]float32{p.X, p.Y, p.Z}
		}
		ways = append(ways, flightWay{pts: pts, deg: i * 360 / fleeWays})
	}
	if len(ways) == 0 {
		return nil, away, "safe_point"
	}
	best := -1
	for i := range ways {
		w := &ways[i]
		w.cost = f.Along(cutPath(w.pts, fleeScan))
		// Starting straight at the attackers means running through them.
		if d := firstDir(w.pts, 10); d != nil {
			if c := -(d[0]*away[0] + d[1]*away[1]); c > 0 {
				w.cost += float32(c) * fleeTowardsAtt
			}
		}
		if w.trail {
			w.cost *= fleeTrailBonus
		}
		if best < 0 || w.cost < ways[best].cost {
			best = i
		}
	}
	w := ways[best]
	slog.Info("flight way", "trail", w.trail, "deg", w.deg, "cost", int(w.cost), "ways", len(ways), "monsters", len(f.Src))
	legs := legsOf(w.pts, trailLeg)
	dir := away
	if n := len(w.pts); n >= 2 {
		if d := unit(w.pts[n-2], w.pts[n-1]); d != nil {
			dir = *d
		}
	}
	name := "map"
	if w.trail {
		name = "trail"
	}
	return legs, dir, name
}

// cutPath is the first n yards of a path.
func cutPath(p [][3]float32, n float32) [][3]float32 {
	out := [][3]float32{p[0]}
	var done float32
	for i := 1; i < len(p); i++ {
		d := dist2(p[i-1][0], p[i-1][1], p[i][0], p[i][1])
		if done+d >= n && d > 0 {
			t := (n - done) / d
			return append(out, [3]float32{p[i-1][0] + (p[i][0]-p[i-1][0])*t, p[i-1][1] + (p[i][1]-p[i-1][1])*t, p[i-1][2] + (p[i][2]-p[i-1][2])*t})
		}
		done += d
		out = append(out, p[i])
	}
	return out
}

// firstDir is the unit direction from the start of a path to its point n yards along.
func firstDir(p [][3]float32, n float32) *[2]float64 {
	c := cutPath(p, n)
	return unit(c[0], c[len(c)-1])
}

func unit(a, b [3]float32) *[2]float64 {
	dx, dy := float64(b[0]-a[0]), float64(b[1]-a[1])
	l := math.Hypot(dx, dy)
	if l < 0.5 {
		return nil
	}
	return &[2]float64{dx / l, dy / l}
}

// legsOf samples a path every step yards (the start excluded, the end included).
func legsOf(p [][3]float32, step float32) []trailPt {
	var out []trailPt
	var acc float32
	for i := 1; i < len(p); i++ {
		acc += dist2(p[i-1][0], p[i-1][1], p[i][0], p[i][1])
		if acc >= step || i == len(p)-1 {
			out = append(out, trailPt{x: p[i][0], y: p[i][1], z: p[i][2]})
			acc = 0
		}
	}
	return out
}
