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
	// Quest monsters (a kill objective of a quest in the log, or a creature that drops one of its
	// items, still missing): worth a longer detour than XP alone, and down to any level (they count
	// whatever their level); an objective done on the way is a hunt less (walking analysis of
	// 2026-10-31: walking ~40 % of the time). The same safety rules (alone, cheap, health).
	engageQuestReach   = 40
	engageQuestOffLine = 25
	// questTargetsEvery: the quest targets are worked out again this often (and at each new log).
	questTargetsEvery = 20 * time.Second
	// questDropMin: item sources with a drop chance below this (%) are not worth a detour.
	questDropMin = 10
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
	// Neutral monsters too: they do not attack us, but answer the call for help of their friends (a
	// Defias Bandit engaged at (-9974,-151) brought a Dockmaster, a Bodyguard and a Dockworker, dead;
	// series 793817f).
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 60, Max: 60, AliveOnly: true})
	if err != nil {
		return nil
	}
	var others []*statepb.Obj
	for _, o := range vis.Objs {
		if o.Reaction != "friendly" && o.Guid != me.Guid && o.Level+5 >= me.Level { // not critters
			others = append(others, o)
		}
	}
	rangeOf := r.s.aggroOf(r.ctx, me.Level)
	wanted := r.questTargets()
	var best *statepb.Obj
	bestQuest := false
	for _, o := range others {
		quest := wanted[o.Entry]
		reach, off := float32(engageReach), float32(engageOffLine)
		if quest {
			reach, off = engageQuestReach, engageQuestOffLine
		}
		if o.Reaction != "hostile" || o.Target != 0 || o.Dist > reach || (!quest && o.Level+engageLevelLow < me.Level) || o.Level > me.Level+1 {
			continue
		}
		if !ahead(me, o, x, y) || offLine(me, o, x, y) > off {
			continue
		}
		if len(safe.Adds(o, others, rangeOf)) > 0 { // not alone
			continue
		}
		sp := &worldpb.Spawn{Entry: o.Entry, MinLevel: o.Level, MaxLevel: o.Level}
		if r.s.tooDangerous(sp, me.Level, me.ClassId) != "" {
			continue
		}
		if loss, _ := xpscore.Single(spawnMob(sp), o.Level, me.Level, me.ClassId, r.s.fights.Get()); loss > engageMaxLoss {
			continue
		}
		// Quest monsters first, then the nearest.
		if best == nil || (quest && !bestQuest) || (quest == bestQuest && o.Dist < best.Dist) {
			best, bestQuest = o, quest
		}
	}
	return best
}

// questTargets is the set of creatures still useful to the quests in the log: kill objectives not
// done (or the creatures that give their credit) and creatures dropping a missing quest item (at
// least questDropMin %).
func (r *run) questTargets() map[uint32]bool {
	if r.questWanted != nil && time.Since(r.questWantedAt) < questTargetsEvery {
		return r.questWanted
	}
	r.questWantedAt = time.Now()
	snap, err := r.s.api.Snapshot(r.ctx)
	if err != nil {
		return r.questWanted
	}
	out := map[uint32]bool{}
	for _, q := range snap.Quests {
		info, err := r.questInfo(q.Id)
		if err != nil {
			continue
		}
		for _, o := range info.Objectives {
			switch o.Kind {
			case "kill":
				if int(o.Counter) < len(q.Counts) && q.Counts[o.Counter] >= o.Count {
					continue
				}
				out[o.Entry] = true
				for _, e := range o.CreditFrom {
					out[e] = true
				}
			case "item":
				if itemCount(snap.Items, o.Entry) >= o.Count {
					continue
				}
				srcs, err := r.itemSources(o.Entry)
				if err != nil {
					continue
				}
				for _, src := range srcs {
					if src.Kind == "creature" && src.Chance >= questDropMin {
						out[src.Entry] = true
					}
				}
			}
		}
	}
	r.questWanted = out
	return out
}

// questInfo is a quest's template, cached for the service.
func (r *run) questInfo(id uint32) (*worldpb.QuestInfo, error) {
	if v, ok := r.s.questInfos.Load(id); ok {
		return v.(*worldpb.QuestInfo), nil
	}
	info, err := r.s.api.QuestInfo(r.ctx, id)
	if err == nil {
		r.s.questInfos.Store(id, info)
	}
	return info, err
}

// itemSources lists who drops an item, cached for the service.
func (r *run) itemSources(item uint32) ([]*worldpb.ItemSource, error) {
	if v, ok := r.s.itemSrcs.Load(item); ok {
		return v.([]*worldpb.ItemSource), nil
	}
	srcs, err := r.s.api.ItemSources(r.ctx, item)
	if err == nil {
		r.s.itemSrcs.Store(item, srcs)
	}
	return srcs, err
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
