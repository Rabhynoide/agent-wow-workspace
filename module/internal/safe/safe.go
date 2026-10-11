// Package safe picks the point farthest from hostile monsters around a center (the idea of
// mod-playerbots' FleeManager): candidate points on rings, scored by their clearance (distance to
// the nearest threat), best first. Checking that a point is reachable is left to the caller.
package safe

import (
	"math"
	"sort"
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
