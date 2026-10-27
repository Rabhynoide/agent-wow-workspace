package combat

import (
	"context"
	"log/slog"
	"math"

	statepb "wowagent/gen/statev1"
)

// Resting among respawning monsters (Fargodeep mine, RB75 series of 2026-10-23): kobolds came back and
// attacked before the character sat down, it chained fights started at 10-49 % health and three of
// the four deaths of the series were such fights (one right after a resurrection at the corpse).

// restSpot walks to a quieter spot before a rest when we stand in the danger of an idle monster (its
// aggro disc or the halo of its friends' call for help, internal/danger), whatever our health: the
// series 045436f lost a run resting at 54 % right after a flight, attacked while resting
// (2026-10-27). The spots tried: our trail back and a ring of real walks around us (15 and 30 yards);
// the one with the least danger at its end and on the way wins, our trail slightly preferred.
// tuning SafeRestBelow 0 turns it off. Callers rest with no attacker on us (the combat flag lingers
// a few seconds after a fight, so it is not checked).
func (s *Service) restSpot(ctx context.Context, me *statepb.SelfInfo) {
	if s.tuneGet().SafeRestBelow == 0 || me.MaxHp == 0 {
		return
	}
	f := s.dangerField(ctx, me, nil)
	here := f.At(me.X, me.Y, me.Z)
	if here <= 0 {
		return
	}
	type spot struct {
		end   [3]float32
		score float32
	}
	score := func(path [][3]float32, trail bool) spot {
		end := path[len(path)-1]
		sc := 4*f.At(end[0], end[1], end[2]) + f.Along(path)/20 + polyLen2(path)/40
		if trail {
			sc *= fleeTrailBonus
		}
		return spot{end, sc}
	}
	var best *spot
	consider := func(sp spot) {
		if best == nil || sp.score < best.score {
			best = &sp
		}
	}
	if legs, _ := s.trail.back(me.Map, me.X, me.Y); len(legs) > 0 {
		path := [][3]float32{{me.X, me.Y, me.Z}}
		for _, l := range legs {
			path = append(path, [3]float32{l.x, l.y, l.z})
			if polyLen2(path) >= safeRestDist {
				break
			}
		}
		consider(score(path, true))
	}
	for _, d := range []float32{15, 30} {
		for i := 0; i < 12; i++ {
			a := float64(i) * math.Pi / 6
			x, y := me.X+d*float32(math.Cos(a)), me.Y+d*float32(math.Sin(a))
			res, err := s.api.PathFromSnapPoints(ctx, me.Map, me.X, me.Y, me.Z, x, y, me.Z, 8)
			if err != nil || res.Error != "" || len(res.Pts) < 2 || res.Length > 2*d+10 {
				continue
			}
			path := make([][3]float32, len(res.Pts))
			for k, p := range res.Pts {
				path[k] = [3]float32{p.X, p.Y, p.Z}
			}
			consider(score(path, false))
		}
	}
	if best == nil || best.score >= 4*here-0.5 {
		slog.Info("no quieter spot to rest", "hp", pct(me.Hp, me.MaxHp), "danger", here, "best", best)
		return
	}
	slog.Info("walking to a quieter spot to rest", "hp", pct(me.Hp, me.MaxHp), "danger", here,
		"spot_danger", f.At(best.end[0], best.end[1], best.end[2]), "monsters", len(f.Src))
	_, _ = s.api.GotoWait(ctx, best.end[0], best.end[1], best.end[2], 2, 15)
}

// safeRestDist bounds the walk back along our trail to a resting spot.
const safeRestDist = 40

func polyLen2(p [][3]float32) float32 {
	var l float32
	for i := 1; i < len(p); i++ {
		l += dist2(p[i-1][0], p[i-1][1], p[i][0], p[i][1])
	}
	return l
}

// restNear: a hostile monster closer than this (on our level) forbids sitting down to eat.
const restNear = 8

// hostileNear tells whether a hostile monster stands within restNear on our level, and whether one of
// those is in combat (fighting us, most likely).
func (s *Service) hostileNear(ctx context.Context, me *statepb.SelfInfo) (near, fighting bool) {
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: restNear, Max: 12, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return false, false
	}
	for _, o := range vis.Objs {
		if o.Guid == me.Guid || math.Abs(float64(o.Z-me.Z)) > 4 || math.Hypot(float64(o.X-me.X), float64(o.Y-me.Y)) > restNear {
			continue
		}
		near = true
		if o.UnitFlags&unitInCombat != 0 {
			slog.Info("monster next to us, not resting", "entry", o.Entry, "dist", int(math.Hypot(float64(o.X-me.X), float64(o.Y-me.Y))), "on_me", o.Target == me.Guid)
			return true, true
		}
	}
	return near, false
}
