package combat

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"time"

	pb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/safe"
	"wowagent/internal/xpscore"
)

// Pulling a single monster. A monster attacks on sight within its aggro range; the ones around our
// target join the fight (aggro, or called for help within ~10 yards). Before engaging, the target
// is chosen so that it and its adds fit in our capacity; failing that we body pull it: stand where
// only it sees us, wait until it comes, back off to a safe spot, then fight.

func aggroRange(player, creature uint32) float32 { return safe.AggroRange(player, creature) }

// aggroFn gives a monster's aggro radius against us (its detection range from the creature
// template, our levels: safe.AggroRangeOf).
type aggroFn func(o *statepb.Obj) float32

// levelAggro uses the default detection range (tests, and when the template is unknown).
func levelAggro(level uint32) aggroFn {
	return func(o *statepb.Obj) float32 { return safe.AggroRangeOf(safe.DefaultDetection, level, o.Level) }
}

// aggroOf reads each monster's detection range (18 for every monster of Elwynn, not the default 20:
// the pull choice saw adds 2 yards too far) once per entry.
func (s *Service) aggroOf(ctx context.Context, level uint32) aggroFn {
	return func(o *statepb.Obj) float32 {
		s.mu.Lock()
		det, ok := s.detection[o.Entry]
		s.mu.Unlock()
		if !ok {
			det = safe.DefaultDetection
			if c, err := s.api.Creature(ctx, o.Entry); err == nil && c.DetectionRange > 0 {
				det = c.DetectionRange
			}
			s.mu.Lock()
			s.detection[o.Entry] = det
			s.mu.Unlock()
		}
		return safe.AggroRangeOf(det, level, o.Level)
	}
}

func dist3D(a, b *statepb.Obj) float32 {
	return float32(math.Sqrt(float64((a.X-b.X)*(a.X-b.X) + (a.Y-b.Y)*(a.Y-b.Y) + (a.Z-b.Z)*(a.Z-b.Z))))
}

// addMargin: yards added to an aggro radius for an add (we fight around the target, not on it).
const addMargin = 2

// groupYards: with the adaptive pull size, one more monster in a pull is worth this many yards of
// walk (2-3 s per kill saved against about 3 s of walk).
const groupYards = 20

// assistRange: a monster attacked calls for help within this distance (CreatureFamilyAssistanceRadius).
const assistRange = safe.AssistRange

// pullWeight is how much a monster counts against the capacity: weaker monsters less, stronger more.
func pullWeight(player, creature uint32) float32 {
	d := int(creature) - int(player)
	switch {
	case d <= -5:
		return 0.25
	case d <= -3:
		return 0.5
	case d >= 3:
		return 2
	case d >= 1:
		return 1.25
	}
	return 1
}

func dist2D(ax, ay, bx, by float32) float32 {
	return float32(math.Hypot(float64(ax-bx), float64(ay-by)))
}

// addsOf lists the hostile monsters that would join a fight at the target's spot: not engaged with
// anyone, and either within their aggro radius of it (3D, at most safe.ZAggro yards above or below:
// Creature::CanStartAttack) or called by the target or by another add (within the assistance range,
// 3D, repeated: the whole chain of a camp). A kobold on
// the floor above in Fargodeep mine is no add (it counted as one, 2D, and crowded the pulls).
func addsOf(t *statepb.Obj, hostiles []*statepb.Obj, rangeOf aggroFn) []*statepb.Obj {
	var out []*statepb.Obj
	for _, h := range hostiles {
		if h.Guid == t.Guid || h.Target != 0 {
			continue
		}
		d := dist3D(h, t)
		aggro := math.Abs(float64(h.Z-t.Z)) <= safe.ZAggro && d <= rangeOf(h)+addMargin
		if aggro || d <= assistRange+1 {
			out = append(out, h)
		}
	}
	// Each monster in the fight calls for help again every 3 s (Creature::CallAssistance,
	// CreatureFamilyAssistancePeriod), within the assistance range of where it stands: a camp joins
	// link by link (the wolf pack of 7 and the Fargodeep kobolds of the runs of 2026-10-26).
	in := map[uint64]bool{t.Guid: true}
	for _, o := range out {
		in[o.Guid] = true
	}
	for grew := true; grew; {
		grew = false
		for _, h := range hostiles {
			// On the fight's floor only: the server wants a line of sight between them, which floors
			// block (the stacked galleries of Fargodeep).
			if in[h.Guid] || h.Target != 0 || math.Abs(float64(h.Z-t.Z)) > safe.ZAggro {
				continue
			}
			for _, m := range out {
				if dist3D(h, m) <= assistRange+1 {
					out = append(out, h)
					in[h.Guid], grew = true, true
					break
				}
			}
		}
	}
	return out
}

// pullLoad is the weighted count of the target and its adds, against an explicit max_pull. The
// target alone counts at most 1: a monster a level or two above us is still fine alone (too strong
// ones are refused before, by auto's danger check); only the adds can push the load over it.
func pullLoad(t *statepb.Obj, adds []*statepb.Obj, level uint32) float32 {
	w := min(pullWeight(level, t.Level), 1)
	for _, a := range adds {
		w += pullWeight(level, a.Level)
	}
	return w
}

// pullSpot finds where to stand so that only the target sees us: within its aggro range, out of the
// others' (keeping margin yards). It prefers the largest margin, then the spot nearest to us.
func pullSpot(me, t *statepb.Obj, others []*statepb.Obj, rangeOf aggroFn) (x, y float32, ok bool) {
	reach := rangeOf(t)
	bestScore := float32(-1e9)
	for _, d := range []float32{reach - 2, reach - 4, reach - 6} {
		if d < 4 {
			continue
		}
		for i := 0; i < 16; i++ {
			a := float64(i) * math.Pi / 8
			px, py := t.X+d*float32(math.Cos(a)), t.Y+d*float32(math.Sin(a))
			margin := float32(100)
			for _, o := range others {
				r := float32(assistRange)
				if math.Abs(float64(o.Z-t.Z)) <= safe.ZAggro {
					r = max(rangeOf(o), r)
				}
				margin = min(margin, dist2D(px, py, o.X, o.Y)-r)
			}
			if margin < 3 {
				continue
			}
			score := min(margin, 15) - 0.05*dist2D(px, py, me.X, me.Y)
			if score > bestScore {
				bestScore, x, y, ok = score, px, py, true
			}
		}
	}
	return x, y, ok
}

// pullFits tells whether a target and its adds can be fought now, with the load and its limit: with
// maxPull > 0, the weighted count (pullLoad) against it; otherwise the expected health loss of the
// learned fights (xpscore.PullLoss) must leave xpscore.PullReserve of our health. A lone target
// always fits (auto refuses the too strong ones).
func (s *Service) pullFits(me *statepb.SelfInfo, maxPull float32) func(t *statepb.Obj, adds []*statepb.Obj) (bool, float32, float32) {
	if maxPull > 0 {
		return func(t *statepb.Obj, adds []*statepb.Obj) (bool, float32, float32) {
			load := pullLoad(t, adds, me.Level)
			return load <= maxPull, load, maxPull
		}
	}
	table, tune := s.fights.Get(), s.tuneGet()
	size := s.pullSize.cap(tune)
	limit := float32(pct(me.Hp, me.MaxHp)) - xpscore.PullReserve
	return func(t *statepb.Obj, adds []*statepb.Obj) (bool, float32, float32) {
		if len(adds) == 0 {
			return true, 0, limit
		}
		levels := []uint32{t.Level}
		for _, a := range adds {
			levels = append(levels, a.Level)
		}
		loss := float32(xpscore.PullLoss(levels, me.Level, me.ClassId, table, tune.AddCost))
		// The learned damage rates (dpslearn), when known: the 5 deaths of the runs of 2026-10-25/26
		// were groups of 2-4 that the learned losses let through.
		if dl, ok := s.groupLoss(me, append([]*statepb.Obj{t}, adds...)); ok {
			loss = max(loss, dl)
		}
		return loss <= limit && (size == 0 || len(levels) <= size), loss, limit
	}
}

// groupLoss is the health (% of the maximum) a group is expected to cost with the learned damage
// rates: killed in the kill order (focus.go), each monster hits us until it dies. ok false when our
// damage or no monster's damage is known yet.
func (s *Service) groupLoss(me *statepb.SelfInfo, group []*statepb.Obj) (float32, bool) {
	ours, ok := s.ours(me)
	if !ok || ours <= 0 || me.MaxHp == 0 {
		return 0, false
	}
	tab := s.dpsTab.Get()
	dps := map[uint64]float64{}
	for _, o := range group {
		if d, ok := tab.Taken(o.Entry, o.Level, me.Armor); ok {
			dps[o.Guid] = d
		}
	}
	if len(dps) == 0 {
		return 0, false
	}
	return float32(100 * orderedDamage(group, dps, ours) / float64(me.MaxHp)), true
}

// orderedDamage is the damage taken killing the group in the kill order at ours damage per second.
func orderedDamage(group []*statepb.Obj, dps map[uint64]float64, ours float64) float64 {
	left := append([]*statepb.Obj(nil), group...)
	sort.Slice(left, func(i, j int) bool { return focusScore(left[i], dps) > focusScore(left[j], dps) })
	var t, dmg float64
	for _, o := range left {
		t += float64(o.Hp) / ours
		dmg += attackerDPS(o.Guid, dps) * t
	}
	return dmg
}

// choosePull returns the target to fight, after a body pull when needed; nil when no monster of the
// entry can be engaged safely for now (they are marked as given up for a while).
func (s *Service) choosePull(ctx context.Context, j *job, me *statepb.SelfInfo, r *pb.KillRequest, radius float32) (*statepb.Obj, error) {
	if r.Guid != 0 { // a given unit (clearing a path)
		o, err := s.api.Obj(ctx, r.Guid)
		if err != nil || (o.Hp == 0 && o.MaxHp > 0) {
			return nil, nil
		}
		return o, nil
	}
	cands, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Entry: r.Entry, Radius: radius, Max: 8, AliveOnly: true})
	if err != nil {
		return nil, err
	}
	hostile, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: radius + 40, Max: 60, AliveOnly: true, HostileOnly: true})
	if err != nil {
		return nil, err
	}
	fits := s.pullFits(me, r.MaxPull)
	rangeOf := s.aggroOf(ctx, me.Level)
	group := r.MaxPull == 0 && s.pullSize.cap(s.tuneGet()) > 1
	self := &statepb.Obj{Guid: me.Guid, X: me.X, Y: me.Y, Z: me.Z}
	var crowded, taken []*statepb.Obj
	var best *statepb.Obj
	bestScore, bestN := float32(-1e9), 0
	zones, now, fled := s.fledZones(), time.Now(), 0
	for _, c := range cands.Objs {
		if s.wasAbandoned(c.Guid) || s.wasCrowded(c.Guid) {
			continue
		}
		if c.Target == me.Guid {
			return c, nil // already on us
		}
		if inFledZone(zones, me.Map, c.X, c.Y, now) {
			fled++
			continue
		}
		if c.Target != 0 {
			taken = append(taken, c)
			continue
		}
		adds := addsOf(c, hostile.Objs, rangeOf)
		ok, load, limit := fits(c, adds)
		if ok && !group {
			return c, nil
		}
		if ok { // the most monsters the pull size allows, a little farther away if needed
			if score := float32(len(adds))*groupYards - c.Dist; score > bestScore {
				best, bestScore, bestN = c, score, 1+len(adds)
			}
			continue
		}
		slog.Info("crowded target", "entry", c.Entry, "level", c.Level, "dist", int(c.Dist), "adds", len(adds), "load", load, "limit", limit)
		crowded = append(crowded, c)
	}
	if best != nil {
		if bestN > 1 {
			slog.Info("group pull", "entry", best.Entry, "monsters", bestN, "dist", int(best.Dist), "size", s.pullSize.cap(s.tuneGet()))
		}
		return best, nil
	}
	for i, c := range crowded {
		if r.BodyPull && i < 2 && s.bodyPull(ctx, j, self, c, hostile.Objs, rangeOf) {
			return c, nil
		}
		s.skipCrowded(c.Guid)
	}
	if len(crowded) > 0 || fled > 0 {
		slog.Info("pull choice", "candidates", len(cands.Objs), "crowded", len(crowded), "taken", len(taken), "in_fled_zone", fled)
	}
	if len(crowded) == 0 && len(taken) > 0 {
		return taken[0], nil // the old behaviour: share a monster someone else fights
	}
	return nil, nil
}

// bodyPull brings the target alone: go where only it sees us, wait for it to come, then back off to a
// spot away from the others. It returns whether the target is now coming at us.
func (s *Service) bodyPull(ctx context.Context, j *job, me, t *statepb.Obj, hostiles []*statepb.Obj, rangeOf aggroFn) bool {
	if t.Reaction != "hostile" {
		return false // a neutral monster never comes by itself
	}
	var others []*statepb.Obj
	for _, h := range hostiles {
		if h.Guid != t.Guid && h.Target == 0 {
			others = append(others, h)
		}
	}
	x, y, ok := pullSpot(me, t, others, rangeOf)
	if !ok {
		return false
	}
	j.Update(func(st *pb.CombatStatus) { st.Doing = "body pulling" })
	slog.Info("body pull", "entry", t.Entry, "others", len(others), "spot", [2]float32{x, y})
	if st, err := s.api.GotoWait(ctx, x, y, t.Z, 1.5, 60); err != nil || st.State != "arrived" {
		return s.onUs(ctx, t.Guid)
	}
	for i := 0; i < 8; i++ {
		if s.onUs(ctx, t.Guid) {
			s.backOff(ctx)
			return true
		}
		if !sleep(ctx, time.Second) {
			return false
		}
	}
	return false
}

// onUs tells whether the unit targets us.
func (s *Service) onUs(ctx context.Context, guid uint64) bool {
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 80, Max: 40, AliveOnly: true})
	if err != nil {
		return false
	}
	for _, o := range vis.Objs {
		if o.Guid == guid {
			return o.Target == s.guid
		}
	}
	return false
}

// backOff moves to a spot near us away from the hostile monsters (path included), so the fight with
// the pulled monster happens far from its friends.
func (s *Service) backOff(ctx context.Context) {
	b, err := s.api.Brief(ctx)
	if err != nil || b.Self == nil {
		return
	}
	me := b.Self
	sp, err := s.api.SafePoint(ctx, me.X, me.Y, me.Z, 15, true)
	if err != nil || !sp.Ok {
		return
	}
	_, _ = s.api.GotoWait(ctx, sp.X, sp.Y, sp.Z, 2, 15)
}
