package combat

import (
	"context"
	"log/slog"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/safe"
)

// Resting low among respawning monsters (Fargodeep mine, RB75 series of 2026-10-23): kobolds came
// back and attacked before the character sat down, it chained fights started at 10-49 % health and
// three of the four deaths of the series were such fights (one right after a resurrection at the
// corpse). Below tuning SafeRestBelow, a rest first walks away from them.

// safeRestClear: a hostile monster closer than this makes us move before resting (aggro range of
// a monster of our level, 20 yards, plus a margin for its wandering and respawns).
const safeRestClear = 25

// safeRestDist bounds the walk to the resting spot.
const safeRestDist = 40

// safeRestGain: the spot found must be this much farther from the nearest monster than where we
// stand, or the walk is not worth it.
const safeRestGain = 8

// restSpot walks to a quieter spot before a rest when our health is below tuning SafeRestBelow and
// a hostile monster stands within safeRestClear. Callers rest with no attacker on us (the combat flag
// lingers a few seconds after a fight, so it is not checked).
func (s *Service) restSpot(ctx context.Context, me *statepb.SelfInfo) {
	below := s.tuneGet().SafeRestBelow
	if below == 0 || me.MaxHp == 0 || pct(me.Hp, me.MaxHp) >= below {
		return
	}
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 60, Max: 60, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return
	}
	var threats [][2]float32
	for _, o := range vis.Objs {
		threats = append(threats, [2]float32{o.X, o.Y})
	}
	here := safe.Clearance(me.X, me.Y, threats)
	if here >= safeRestClear {
		return
	}
	sp, err := s.api.SafePoint(ctx, me.X, me.Y, me.Z, safeRestDist, true)
	if err != nil || !sp.Ok || sp.Clearance < here+safeRestGain {
		slog.Info("no quieter spot to rest", "hp", pct(me.Hp, me.MaxHp), "nearest", int(here), "best", int(sp.GetClearance()), "err", err)
		return
	}
	slog.Info("walking to a quieter spot to rest", "hp", pct(me.Hp, me.MaxHp), "nearest", int(here), "spot", int(sp.Clearance))
	_, _ = s.api.GotoWait(ctx, sp.X, sp.Y, sp.Z, 2, 15)
}
