package nav

import "math"

// Bending a path around monsters, the way a player walks past a camp: take the shortest path, and
// where a stretch of it crosses a monster's aggro disc, replace that stretch by the shorter of the
// two ways around the disc, just outside its edge. The weighted A* (stepCost) could only detour by
// whole navmesh polygons: a meadow polygon is tens of yards wide, so the path either crossed the
// disc or went far around it (walking 22-26 % → 38 % of the run, the avoidance was switched off on
// 2026-10-17).

const (
	bendClear   = 1.5 // yards kept between the bent path and a disc's edge
	bendArcStep = math.Pi / 9
	bendMaxIter = 40
	// BendMaxDetour: a way around longer than the stretch it replaces by more than this (yards) is not
	// taken: the stretch is walked through (a fight costs ~15-30 s, 80 yards ~11 s).
	BendMaxDetour = 80
	bendMergeGap  = 3 // yards: discs closer than this are one camp (no way between them)
)

// circle is a disc in the (x, z) plane of Recast space.
type circle struct {
	x, z, r float64
}

func (c circle) has(p V3, slack float64) bool {
	return math.Hypot(float64(p[0])-c.x, float64(p[2])-c.z) < c.r-slack
}

// segHits tells whether the segment a-b passes inside the disc (by more than slack yards).
func (c circle) segHits(a, b V3, slack float64) bool {
	ax, az, bx, bz := float64(a[0]), float64(a[2]), float64(b[0]), float64(b[2])
	dx, dz := bx-ax, bz-az
	l2 := dx*dx + dz*dz
	t := 0.0
	if l2 > 1e-9 {
		t = math.Max(0, math.Min(1, ((c.x-ax)*dx+(c.z-az)*dz)/l2))
	}
	return math.Hypot(ax+t*dx-c.x, az+t*dz-c.z) < c.r-slack
}

// mergeDiscs joins discs that overlap or leave less than bendMergeGap between them into enclosing
// discs: there is no way between two monsters of a camp.
func mergeDiscs(ds []Danger) []circle {
	cs := make([]circle, len(ds))
	for i, d := range ds {
		cs[i] = circle{float64(d.P[0]), float64(d.P[2]), float64(d.R)}
	}
	for merged := true; merged; {
		merged = false
		for i := 0; i < len(cs) && !merged; i++ {
			for j := i + 1; j < len(cs); j++ {
				a, b := cs[i], cs[j]
				d := math.Hypot(a.x-b.x, a.z-b.z)
				if d >= a.r+b.r+bendMergeGap {
					continue
				}
				var c circle
				switch {
				case d+b.r <= a.r:
					c = a
				case d+a.r <= b.r:
					c = b
				default:
					c.r = (d + a.r + b.r) / 2
					t := (c.r - a.r) / d
					c.x, c.z = a.x+(b.x-a.x)*t, a.z+(b.z-a.z)*t
				}
				cs[i] = c
				cs = append(cs[:j], cs[j+1:]...)
				merged = true
				break
			}
		}
	}
	return cs
}

// arcAround returns the corners of the way from a to b around the disc on one side (ccw: counter-
// clockwise in the (x, z) plane), at radius r: the tangent points and corners every bendArcStep,
// pushed out so that the chords stay outside r. Heights are a's (fixed later on the mesh).
func arcAround(c circle, a, b V3, r float64, ccw bool) []V3 {
	tan := func(p V3) (phi, alpha float64) {
		dx, dz := float64(p[0])-c.x, float64(p[2])-c.z
		d := math.Hypot(dx, dz)
		phi = math.Atan2(dz, dx)
		if d > r {
			alpha = math.Acos(r / d)
		}
		return
	}
	pa, aa := tan(a)
	pb, ab := tan(b)
	var start, end float64
	if ccw {
		start, end = pa+aa, pb-ab
		for end < start {
			end += 2 * math.Pi
		}
	} else {
		start, end = pa-aa, pb+ab
		for end > start {
			end -= 2 * math.Pi
		}
	}
	span := end - start
	n := int(math.Ceil(math.Abs(span) / bendArcStep))
	if n < 1 {
		n = 1
	}
	step := span / float64(n)
	ro := r / math.Cos(math.Abs(step)/2) // chords between corners stay outside r
	out := make([]V3, 0, n+1)
	for i := 0; i <= n; i++ {
		th := start + step*float64(i)
		rr := ro
		if i == 0 || i == n {
			rr = r // the tangent points themselves
		}
		out = append(out, V3{float32(c.x + rr*math.Cos(th)), a[1], float32(c.z + rr*math.Sin(th))})
	}
	return out
}

func polyLen(p []V3) float64 {
	l := 0.0
	for i := 1; i < len(p); i++ {
		l += float64(dist3D(p[i-1], p[i]))
	}
	return l
}

// legClear is the walkable path from a to b (plain filter) when it exists, is not much longer than
// the straight line (no way round a wall or a cliff) and stays out of the discs.
func (m *Mesh) legClear(a, b V3, f Filter, cs []circle) ([]V3, bool) {
	p, err := m.Path(a, b, f)
	if err != nil || len(p) < 2 || dist2D(p[len(p)-1], b) > 2 {
		return nil, false
	}
	if polyLen(p) > 1.3*float64(dist3D(a, b))+3 {
		return nil, false
	}
	for i := 1; i < len(p); i++ {
		for _, c := range cs {
			if c.segHits(p[i-1], p[i], 0.5) {
				return nil, false
			}
		}
	}
	return p, true
}

// Bend returns the corners of a path from `from` to `to` that keeps out of the dangers of f where a
// way around exists (see the top of this file), and how many discs it walks around; f.Avoid is not
// used by the A* itself. Discs holding the start or the end are ignored (we are in them already, or
// must go there).
func (m *Mesh) Bend(from, to V3, f Filter) ([]V3, int, error) {
	plain := f
	plain.Avoid = nil
	pts, err := m.Path(from, to, plain)
	if err != nil {
		return nil, 0, err
	}
	var cs []circle
	for _, c := range mergeDiscs(f.Avoid) {
		if !c.has(pts[0], 0) && !c.has(pts[len(pts)-1], 0) {
			cs = append(cs, c)
		}
	}
	legs := Filter{Include: f.Include}
	passed := map[int]bool{} // discs walked through (no way around)
	bent := 0
	for iter := 0; iter < bendMaxIter; iter++ {
		hit, seg := -1, -1
		for i := 1; i < len(pts) && hit < 0; i++ {
			for k, c := range cs {
				if !passed[k] && c.segHits(pts[i-1], pts[i], 0.5) {
					hit, seg = k, i
					break
				}
			}
		}
		if hit < 0 {
			break
		}
		c := cs[hit]
		// The stretch to replace: from the last corner before the disc to the first one after it.
		a, b := pts[seg-1], pts[seg]
		j := seg
		for j < len(pts)-1 && (c.has(pts[j], 0) || c.segHits(pts[j], pts[j+1], 0.5)) {
			j++
		}
		b = pts[j]
		var stretch float64
		for i := seg; i <= j; i++ {
			stretch += float64(dist3D(pts[i-1], pts[i]))
		}
		var others []circle
		for k, o := range cs {
			if k != hit && !passed[k] {
				others = append(others, o)
			}
		}
		var best []V3
		bestLen := math.Inf(1)
		for _, ccw := range []bool{true, false} {
			arc := arcAround(c, a, b, c.r+bendClear, ccw)
			way := []V3{a}
			ok := true
			for _, q := range arc {
				// The corner on the mesh (its height), close to where it was put.
				_, on, found := m.NearestPoly(q, legs, 8, 2)
				if !found {
					ok = false
					break
				}
				way = append(way, on)
			}
			if !ok {
				continue
			}
			way = append(way, b)
			var full []V3
			for i := 1; i < len(way) && ok; i++ {
				leg, clear := m.legClear(way[i-1], way[i], legs, append(others, circle{c.x, c.z, c.r}))
				if !clear {
					ok = false
					break
				}
				if len(full) > 0 {
					leg = leg[1:]
				}
				full = append(full, leg...)
			}
			if ok {
				if l := polyLen(full); l < bestLen {
					best, bestLen = way, l
				}
			}
		}
		if best == nil || bestLen-stretch > BendMaxDetour {
			passed[hit] = true
			continue
		}
		out := append([]V3{}, pts[:seg-1]...)
		out = append(out, best...)
		pts = append(out, pts[j+1:]...)
		bent++
	}
	return pts, bent, nil
}
