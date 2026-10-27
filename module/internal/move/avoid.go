package move

import (
	"context"
	"math"
	"time"

	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/nav"
	"wowagent/internal/safe"
	"wowagent/internal/tuning"
)

// Paths keep away from the idle hostile monsters we can see and from the places combat fled from:
// walking through their aggro range was the main cost of the runs (43 fights while walking, quests
// postponed after flights on the way, the Fargodeep mine; bench of 2026-10-13). auto then clears only
// what is still on the path. A long walk is planned before the monsters ahead are visible: while
// walking, a monster newly seen close to the rest of the path makes us plan again (recheckEvery).

const (
	avoidMargin  = 4  // yards added to a monster's aggro range
	avoidSeen    = 90 // yards: monsters farther away are not sent (the server shows ~100)
	avoidFloor   = 12 // yards of height: a monster on another floor or ledge is ignored
	recheckEvery = 3  // seconds between two looks for new monsters on the way
	recheckAhead = 80 // yards of the remaining path checked
)

var tuneSrc = tuning.NewSource("")

// danger is a disc to keep away from, with the monster's guid (0 for a fled zone).
type danger struct {
	guid uint64
	nav.Danger
}

func navDangers(ds []danger) []nav.Danger {
	out := make([]nav.Danger, len(ds))
	for i, d := range ds {
		out[i] = d.Danger
	}
	return out
}

// dangers lists the discs around the idle hostile monsters near us and the fled zones, except the
// ones around the destination (the target of a pull, the quest giver's camp: we must go there).
func (s *Service) dangers(ctx context.Context, me *statepb.SelfInfo, x, y, z float32) []danger {
	if !tuneSrc.Get().Avoid { // tuning.json "avoid": false, to measure the walk without it
		return nil
	}
	var out []danger
	wander := tuneSrc.Get().AvoidWander
	if near, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: avoidSeen, Max: 60, AliveOnly: true, HostileOnly: true}); err == nil {
		homes := s.spawnsNear(ctx, me)
		for _, o := range near.Objs {
			if o.Target != 0 || math.Abs(float64(o.Z-me.Z)) > avoidFloor {
				continue // in a fight already (with us: running away from it is the flight's job)
			}
			r := safe.AggroRange(me.Level, o.Level) + avoidMargin
			cx, cy := o.X, o.Y
			// A wandering monster drifts while we pass (walk speed, random points within its wander
			// radius of its spawn point, 16 yards on average around Goldshire): a margin of a share of
			// that radius around where it stands. The whole radius around the spawn point made camps
			// too wide to walk around.
			if h := homeOf(homes, o); h != nil && wander > 0 {
				r += h.Wander * wander
			}
			if math.Hypot(float64(cx-x), float64(cy-y)) < float64(r) {
				continue
			}
			out = append(out, danger{o.Guid, nav.Danger{P: nav.ToRecast(cx, cy, o.Z), R: r}})
		}
	}
	// Places we fled from (combat/zones.go): their monsters may stand out of sight, or wander.
	if st, err := s.api.CombatStatus(ctx, 0); err == nil {
		for _, zn := range st.Avoid {
			if math.Hypot(float64(zn.X-x), float64(zn.Y-y)) < float64(zn.Radius) {
				continue
			}
			out = append(out, danger{0, nav.Danger{P: nav.ToRecast(zn.X, zn.Y, me.Z), R: zn.Radius}})
		}
	}
	return out
}

// newOnPath tells whether a danger not known when the route was planned touches the next
// recheckAhead yards of the path (from point seg on).
func newOnPath(ds []danger, known map[uint64]bool, path [][3]float32, seg int) bool {
	for _, d := range ds {
		if d.guid == 0 || known[d.guid] {
			continue
		}
		wx, wy, _ := d.P.ToWoW()
		ahead := float32(0)
		for i := seg; i < len(path); i++ {
			if i > seg {
				if ahead += dist2(path[i-1], path[i]); ahead > recheckAhead {
					break
				}
			}
			if math.Hypot(float64(path[i][0]-wx), float64(path[i][1]-wy)) < float64(d.R) {
				return true
			}
		}
	}
	return false
}

func knownOf(ds []danger) map[uint64]bool {
	k := map[uint64]bool{}
	for _, d := range ds {
		if d.guid != 0 {
			k[d.guid] = true
		}
	}
	return k
}

// Spawn points around us, cached: one query per walk area (the map has tens of thousands of spawns).
const (
	spawnsRadius = 200 // yards queried around us
	spawnsReuse  = 80  // yards we may move before querying again
)

type spawnCache struct {
	mapID uint32
	x, y  float32
	at    time.Time
	list  []*worldpb.Spawn
}

// spawnsNear returns the creature spawns within spawnsRadius of us (cached while we stay within
// spawnsReuse yards, 10 minutes at most).
func (s *Service) spawnsNear(ctx context.Context, me *statepb.SelfInfo) []*worldpb.Spawn {
	s.mu.Lock()
	c := s.spawns
	s.mu.Unlock()
	if c != nil && c.mapID == me.Map && time.Since(c.at) < 10*time.Minute &&
		math.Hypot(float64(c.x-me.X), float64(c.y-me.Y)) < spawnsReuse {
		return c.list
	}
	list, err := s.api.SpawnsAll(ctx, me.Map, me.X, me.Y, spawnsRadius)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	s.spawns = &spawnCache{mapID: me.Map, x: me.X, y: me.Y, at: time.Now(), list: list}
	s.mu.Unlock()
	return list
}

// homeOf is the spawn point of a wandering monster: the closest spawn of its entry that wanders,
// within its wander radius (+10 yards) of where it stands; nil for the others.
func homeOf(homes []*worldpb.Spawn, o *statepb.Obj) *worldpb.Spawn {
	var best *worldpb.Spawn
	bestD := math.Inf(1)
	for _, h := range homes {
		if h.Entry != o.Entry || h.MovementType != 1 || h.Wander <= 0 {
			continue
		}
		d := math.Hypot(float64(h.X-o.X), float64(h.Y-o.Y))
		if d <= float64(h.Wander)+10 && d < bestD {
			best, bestD = h, d
		}
	}
	return best
}
