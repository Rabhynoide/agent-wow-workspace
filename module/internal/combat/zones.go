package combat

import (
	"math"
	"time"

	pb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
)

// Fled zones. After a flight the monsters go back home (leash) and the next pull or the next walk of
// the grind used to bring us right back to them: 5 flights in 3 minutes from the same pack of wolves
// (bench of 2026-10-13). The place they attacked from is avoided for a while: pull choice here, spawn
// points and walks in auto (the zones travel in every CombatStatus).

const (
	// fledRadius: around the attackers' centre; monsters there are not pulled. Aggro range at our
	// level (about 20 yards) plus the pack's spread.
	fledRadius = 40
	fledFor    = 5 * time.Minute
)

type fledZone struct {
	m    uint32
	x, y float32
	at   time.Time
}

// zoneOf is the centre of the attackers (where they stand when we start running: their home is close).
func zoneOf(m uint32, att []*statepb.Obj, now time.Time) fledZone {
	var x, y float32
	for _, a := range att {
		x += a.X
		y += a.Y
	}
	n := float32(len(att))
	return fledZone{m: m, x: x / n, y: y / n, at: now}
}

// addZone records a fled zone and forgets the old ones.
func (s *Service) addZone(z fledZone) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones = append(liveZones(s.zones, z.at), z)
}

func liveZones(zs []fledZone, now time.Time) []fledZone {
	var out []fledZone
	for _, z := range zs {
		if now.Sub(z.at) < fledFor {
			out = append(out, z)
		}
	}
	return out
}

// inFledZone tells whether a point of this map is in a zone we fled from.
func inFledZone(zs []fledZone, m uint32, x, y float32, now time.Time) bool {
	for _, z := range liveZones(zs, now) {
		if z.m == m && math.Hypot(float64(x-z.x), float64(y-z.y)) < fledRadius {
			return true
		}
	}
	return false
}

func (s *Service) fledZones() []fledZone {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fledZone(nil), s.zones...)
}

// withZones adds the live fled zones to a status sent to the caller.
func (s *Service) withZones(st *pb.CombatStatus) *pb.CombatStatus {
	now := time.Now()
	st.Avoid = nil
	for _, z := range liveZones(s.fledZones(), now) {
		st.Avoid = append(st.Avoid, &pb.Zone{Map: z.m, X: z.x, Y: z.y, Radius: fledRadius,
			LeftS: uint32((fledFor - now.Sub(z.at)).Seconds())})
	}
	return st
}
