package auto

import (
	"math"
	"time"

	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/safe"
	"wowagent/internal/xpscore"
)

// Monsters on the way, engaged first (tuning engage_on_way): speedrunners kill the green and yellow
// monsters next to their route instead of being hit in the back by them; a warrior who charges
// starts with rage and a stunned target. Only a lone monster close ahead of us, close to the line to
// the destination, that the learned fights say is cheap, with enough health.
const (
	engageReach    = 25 // yards from us
	engageOffLine  = 12 // yards from the line to the destination
	engageMinHP    = 70 // % of our health
	engageLevelLow = 2  // levels below ours still worth it (lower: little XP)
	engageEvery    = 2 * time.Second
	engageMaxLoss  = 30 // % of health the learned fight may cost
	// engageMaxPerWalk: monsters engaged during one walk at most (a long walk through a busy zone
	// would turn into a grind).
	engageMaxPerWalk = 4
)

// engageOnTheWay returns the monster to engage now, or nil.
func (r *run) engageOnTheWay(x, y float32) *statepb.Obj {
	if !r.s.tune.Get().EngageOnWay || time.Since(r.lastEngageCheck) < engageEvery {
		return nil
	}
	r.lastEngageCheck = time.Now()
	b, err := r.s.api.Brief(r.ctx)
	if err != nil || b.Self == nil || b.Self.MaxHp == 0 || b.Self.Hp*100 < engageMinHP*b.Self.MaxHp {
		return nil
	}
	me := b.Self
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 60, Max: 40, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return nil
	}
	rangeOf := r.s.aggroOf(r.ctx, me.Level)
	var best *statepb.Obj
	for _, o := range vis.Objs {
		if o.Target != 0 || o.Dist > engageReach || o.Level+engageLevelLow < me.Level || o.Level > me.Level+1 {
			continue
		}
		if !ahead(me, o, x, y) || offLine(me, o, x, y) > engageOffLine {
			continue
		}
		if len(safe.Adds(o, vis.Objs, rangeOf)) > 0 { // not alone
			continue
		}
		sp := &worldpb.Spawn{Entry: o.Entry, MinLevel: o.Level, MaxLevel: o.Level}
		if r.s.tooDangerous(sp, me.Level, me.ClassId) != "" {
			continue
		}
		if loss, _ := xpscore.Single(spawnMob(sp), o.Level, me.Level, me.ClassId, r.s.fights.Get()); loss > engageMaxLoss {
			continue
		}
		if best == nil || o.Dist < best.Dist {
			best = o
		}
	}
	return best
}

// ahead: o is on our side of the destination's way (not behind us).
func ahead(me *statepb.SelfInfo, o *statepb.Obj, x, y float32) bool {
	dx, dy := float64(x-me.X), float64(y-me.Y)
	return dx*float64(o.X-me.X)+dy*float64(o.Y-me.Y) > 0
}

// offLine is o's distance to the segment from us to the destination (2D).
func offLine(me *statepb.SelfInfo, o *statepb.Obj, x, y float32) float32 {
	ax, ay, bx, by := float64(me.X), float64(me.Y), float64(x), float64(y)
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	t := 0.0
	if l2 > 1e-9 {
		t = math.Max(0, math.Min(1, ((float64(o.X)-ax)*dx+(float64(o.Y)-ay)*dy)/l2))
	}
	return float32(math.Hypot(ax+t*dx-float64(o.X), ay+t*dy-float64(o.Y)))
}
