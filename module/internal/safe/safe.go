// Package safe picks the point farthest from hostile monsters around a center (the idea of
// mod-playerbots' FleeManager): candidate points on rings, scored by their clearance (distance to
// the nearest threat), best first. Checking that a point is reachable is left to the caller.
package safe

import (
	"math"
	"sort"
)

// AggroRange approximates Creature::GetAggroRange with the default detection range (20 yards).
func AggroRange(player, creature uint32) float32 {
	return AggroRangeOf(DefaultDetection, player, creature)
}

// DefaultDetection is creature_template.detection_range when unknown (its default; 18 in Elwynn).
const DefaultDetection = 20

// AggroRangeOf is Creature::GetAggroRange (read 2026-10-24): the creature's detection range, one yard
// less per level the player is above it (more when below, at most 25 levels), between 5 and 45, times
// Rate.Creature.Aggro (1 on our server). The distance is 3D, and a creature more than ZAggro yards
// above or below does not aggro (CanStartAttack).
func AggroRangeOf(detection float32, player, creature uint32) float32 {
	diff := max(float32(player)-float32(creature), -25)
	return min(max(detection-diff, 5), 45)
}

// ZAggro is CREATURE_Z_ATTACK_RANGE: no aggro beyond this height difference.
const ZAggro = 3

// AssistRange and AssistDelay: a creature entering combat calls its friends within this 3D distance
// (in sight) after this delay (CreatureFamilyAssistanceRadius, CreatureFamilyAssistanceDelay).
const (
	AssistRange   = 10
	AssistDelayMs = 2000
)

// Point is a candidate with its clearance.
type Point struct {
	X, Y      float32
	Clearance float32 // distance to the nearest threat (large when there is none)
}

const noThreat = 1000

// Clearance is the distance from (x, y) to the nearest threat.
func Clearance(x, y float32, threats [][2]float32) float32 {
	best := float32(noThreat)
	for _, t := range threats {
		if d := float32(math.Hypot(float64(x-t[0]), float64(y-t[1]))); d < best {
			best = d
		}
	}
	return best
}

// Candidates returns points on rings from minDist to maxDist around (cx, cy) (every 5 yards, 16
// directions; the center itself when minDist is 0), sorted by clearance, best first; equal
// clearances keep the point closer to the center.
func Candidates(cx, cy, minDist, maxDist float32, threats [][2]float32) []Point {
	var out []Point
	if minDist <= 0 {
		out = append(out, Point{cx, cy, Clearance(cx, cy, threats)})
		minDist = 5
	}
	for d := minDist; d <= maxDist+0.01; d += 5 {
		for i := 0; i < 16; i++ {
			a := float64(i) * math.Pi / 8
			x, y := cx+d*float32(math.Cos(a)), cy+d*float32(math.Sin(a))
			out = append(out, Point{x, y, Clearance(x, y, threats)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Clearance > out[j].Clearance })
	return out
}
