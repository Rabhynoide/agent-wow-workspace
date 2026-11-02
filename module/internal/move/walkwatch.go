package move

import (
	"context"
	"log/slog"
	"math"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/safe"
)

// Walk watch (2026-11-02): 179 flights in 411 began with an attack during a walk although the path
// goes around the monsters seen. To learn the chance of an aggro by distance (a path cost in seconds:
// length / speed + sum of P(aggro) x the expected cost of the fight), each walk records, for the
// idle hostile monsters it passes, the closest distance, whether they attacked us and at what
// distance, the aggro radius we expected, whether the path avoided them, whether they wander.
// Logged when the walk ends: one "walk pass" line per monster that came within watchNear yards of
// its expected radius.
const (
	watchRadius = 45 // yards around us
	watchNear   = 15 // yards beyond the expected aggro radius
)

type passed struct {
	entry, level uint32
	minDist      float32 // closest distance while idle
	aggroDist    float32 // distance when it targeted us (0: never)
	expected     float32 // safe.AggroRange at the time
	avoided      bool    // a danger of the route
	wander       bool    // a wandering spawn
	dz           float32 // height difference at the closest point
	toDest       float32 // its distance to the walk's destination when first seen
	why          string  // why the path did not go around it (avoided false)
}

type walkWatch struct {
	seen map[uint64]*passed
}

func newWalkWatch() *walkWatch { return &walkWatch{seen: map[uint64]*passed{}} }

// sample looks at the hostile monsters around us once.
func (w *walkWatch) sample(ctx context.Context, s *Service, me *statepb.SelfInfo, rt *route) {
	near, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: watchRadius, Max: 40, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return
	}
	homes := s.spawnsNear(ctx, me)
	for _, o := range near.Objs {
		d := float32(math.Hypot(float64(o.X-me.X), float64(o.Y-me.Y)))
		p := w.seen[o.Guid]
		if p == nil {
			if o.Target != 0 { // already fighting someone (or us) when first seen
				continue
			}
			p = &passed{entry: o.Entry, level: o.Level, minDist: d, expected: safe.AggroRange(me.Level, o.Level),
				avoided: rt.known[o.Guid], wander: homeOf(homes, o) != nil, dz: float32(math.Abs(float64(o.Z - me.Z))),
				toDest: float32(math.Hypot(float64(o.X-rt.target[0]), float64(o.Y-rt.target[1])))}
			switch {
			case p.avoided:
			case !tuneSrc.Get().Avoid:
				p.why = "avoid off"
			case p.toDest < p.expected+avoidMargin+10:
				p.why = "near destination"
			case p.dz > avoidFloor:
				p.why = "other floor"
			case len(rt.known) == 0:
				p.why = "no dangers planned"
			default:
				p.why = "seen after planning"
			}
			w.seen[o.Guid] = p
		}
		if p.aggroDist > 0 {
			continue
		}
		if o.Target == me.Guid {
			p.aggroDist = d
			continue
		}
		if o.Target == 0 && d < p.minDist {
			p.minDist, p.dz = d, float32(math.Abs(float64(o.Z-me.Z)))
		}
	}
}

// log writes the monsters that came near.
func (w *walkWatch) log() {
	for _, p := range w.seen {
		if p.aggroDist == 0 && p.minDist > p.expected+watchNear {
			continue
		}
		slog.Info("walk pass", "entry", p.entry, "level", p.level, "min_dist", int(p.minDist), "aggro_dist", int(p.aggroDist),
			"expected", int(p.expected), "avoided", p.avoided, "wander", p.wander, "dz", int(p.dz), "to_dest", int(p.toDest), "why", p.why)
	}
}
