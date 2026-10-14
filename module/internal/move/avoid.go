package move

import (
	"context"
	"math"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/nav"
	"wowagent/internal/safe"
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
	var out []danger
	if near, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: avoidSeen, Max: 60, AliveOnly: true, HostileOnly: true}); err == nil {
		for _, o := range near.Objs {
			if o.Target != 0 || math.Abs(float64(o.Z-me.Z)) > avoidFloor {
				continue // in a fight already (with us: running away from it is the flight's job)
			}
			r := safe.AggroRange(me.Level, o.Level) + avoidMargin
			if math.Hypot(float64(o.X-x), float64(o.Y-y)) < float64(r) {
				continue
			}
			out = append(out, danger{o.Guid, nav.Danger{P: nav.ToRecast(o.X, o.Y, o.Z), R: r}})
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
