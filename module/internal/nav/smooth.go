package nav

import (
	"errors"
	"math"
)

// Smooth paths, ported from Detour's sample "findSmoothPath" as used by the server (AzerothCore
// PathGenerator::FindSmoothPath): from the current point, steer towards the next corner of the
// straight path, advance at most one step with moveAlongSurface (which slides along the walkable
// polygons instead of crossing walls or gaps), fix the polygon corridor up, and take the height of
// the polygon reached. Every point therefore lies on the navmesh surface.

const (
	// SmoothStep is the distance between two points of a smooth path (the server uses 4 yards; a
	// shorter step follows stairs and slopes more closely).
	SmoothStep = 2.0
	// SmoothSlop is how close a corner must be to count as reached.
	SmoothSlop = 0.3

	maxSmoothPoints = 8192
	maxSurfaceNodes = 48 // polygons explored by one moveAlongSurface (Detour MAX_STACK)
)

// ErrSmoothStuck means the smooth path could not reach the destination along the surface.
var ErrSmoothStuck = errors.New("smooth path stuck before the destination")

// SmoothPath returns points about step apart (Recast space) on the walkable surface from `from` to `to`.
func (m *Mesh) SmoothPath(from, to V3, f Filter, step, slop float32) ([]V3, error) {
	sPt, ePt, chain, err := m.corridor(from, to, f)
	if err != nil {
		return nil, err
	}
	corr := make([]polyRef, len(chain))
	for i, n := range chain {
		corr[i] = n.ref
	}
	pc := &portalCache{m: m, f: f, links: map[polyRef][]link{}}
	iter := sPt
	out := []V3{iter}
	stuck := 0
	for len(corr) > 0 && len(out) < maxSmoothPoints {
		steer, isEnd := pc.steerTarget(iter, ePt, corr, slop)
		delta := V3{steer[0] - iter[0], steer[1] - iter[1], steer[2] - iter[2]}
		l := float32(math.Sqrt(float64(delta[0]*delta[0] + delta[1]*delta[1] + delta[2]*delta[2])))
		frac := float32(1)
		if l > 1e-6 && !(isEnd && l < step) {
			frac = min(1, step/l)
		}
		target := V3{iter[0] + delta[0]*frac, iter[1] + delta[1]*frac, iter[2] + delta[2]*frac}
		res, visited := pc.moveAlongSurface(corr[0], iter, target)
		corr = fixupCorridor(corr, visited)
		if h, ok := m.polyHeight(corr[0], res); ok {
			res[1] = h
		}
		if dist2D(res, iter) < 0.01 {
			if stuck++; stuck > 8 {
				break
			}
		} else {
			stuck = 0
		}
		iter = res
		if isEnd && dist2D(iter, steer) < slop && abs32(iter[1]-steer[1]) < 1 {
			out = append(out, iter)
			return out, nil
		}
		out = append(out, iter)
	}
	if dist2D(iter, ePt) > 1 {
		return out, ErrSmoothStuck
	}
	return out, nil
}

// portalCache memoizes polygon links during one path computation.
type portalCache struct {
	m     *Mesh
	f     Filter
	links map[polyRef][]link
}

func (pc *portalCache) linksOf(r polyRef) []link {
	if l, ok := pc.links[r]; ok {
		return l
	}
	l := pc.m.links(r, pc.f)
	pc.links[r] = l
	return l
}

// portal returns the shared edge going from polygon a to polygon b.
func (pc *portalCache) portal(a, b polyRef) (V3, V3, bool) {
	for _, l := range pc.linksOf(a) {
		if l.to == b {
			return l.pa, l.pb, true
		}
	}
	return V3{}, V3{}, false
}

// steerTarget is the first corner of the straight path that is not already reached (Detour getSteerTarget).
func (pc *portalCache) steerTarget(start, end V3, corr []polyRef, slop float32) (V3, bool) {
	corners := pc.straightCorners(start, end, corr, 3)
	for i, c := range corners {
		last := i == len(corners)-1
		if dist2D(c, start) > slop || last {
			return c, last && vEq(c, end)
		}
	}
	return end, true
}

// straightCorners runs the funnel over the corridor portals and returns up to maxCorners corners
// (the last one is `end` when it is reached within them).
func (pc *portalCache) straightCorners(start, end V3, corr []polyRef, maxCorners int) []V3 {
	n := len(corr) // portals 1..n-1 between polygons, portal n is the end point
	portalAt := func(i int) (V3, V3) {
		if i >= n {
			return end, end
		}
		if l, r, ok := pc.portal(corr[i-1], corr[i]); ok {
			return l, r
		}
		return end, end
	}
	var out []V3
	push := func(v V3) bool {
		if len(out) == 0 || !vEq(out[len(out)-1], v) {
			out = append(out, v)
		}
		return len(out) >= maxCorners
	}
	apex, left, right := start, start, start
	apexI, leftI, rightI := 0, 0, 0
	for i := 1; i <= n; i++ {
		l, r := portalAt(i)
		if triArea2(apex, right, r) <= 0 {
			if vEq(apex, right) || triArea2(apex, left, r) > 0 {
				right, rightI = r, i
			} else {
				if push(left) {
					return out
				}
				apex, apexI = left, leftI
				left, right, leftI, rightI = apex, apex, apexI, apexI
				i = apexI
				continue
			}
		}
		if triArea2(apex, left, l) >= 0 {
			if vEq(apex, left) || triArea2(apex, right, l) < 0 {
				left, leftI = l, i
			} else {
				if push(right) {
					return out
				}
				apex, apexI = right, rightI
				left, right, leftI, rightI = apex, apex, apexI, apexI
				i = apexI
				continue
			}
		}
	}
	push(end)
	return out
}

// moveAlongSurface moves from startPos (in polygon startRef) towards endPos without leaving the
// walkable polygons: a small breadth-first search finds the polygon holding endPos, otherwise the
// closest point on a wall edge. It returns the reached position (height not resolved) and the
// polygons from startRef to the one reached (Detour dtNavMeshQuery::moveAlongSurface).
func (pc *portalCache) moveAlongSurface(startRef polyRef, startPos, endPos V3) (V3, []polyRef) {
	type bnode struct {
		ref    polyRef
		parent int
	}
	nodes := []bnode{{startRef, -1}}
	seen := map[polyRef]bool{startRef: true}
	bestPos, best := startPos, 0
	bestDist := float32(math.MaxFloat32)
	searchPos := V3{(startPos[0] + endPos[0]) / 2, (startPos[1] + endPos[1]) / 2, (startPos[2] + endPos[2]) / 2}
	searchRad := dist3D(startPos, endPos)/2 + 0.001
	for qi := 0; qi < len(nodes); qi++ {
		cur := nodes[qi]
		t, p := pc.m.poly(cur.ref)
		if t == nil {
			continue
		}
		vs := t.polyVerts(p)
		if inPoly2D(endPos, vs) {
			best, bestPos = qi, endPos
			break
		}
		links := pc.linksOf(cur.ref)
		for e := range vs {
			a, b := vs[e], vs[(e+1)%len(vs)]
			wall := true
			for _, l := range links {
				if l.edge != e {
					continue
				}
				wall = false
				if seen[l.to] || len(nodes) >= maxSurfaceNodes || distToSeg2D(searchPos, l.pa, l.pb) > searchRad {
					continue
				}
				seen[l.to] = true
				nodes = append(nodes, bnode{l.to, qi})
			}
			if wall {
				if d := distToSeg2D(endPos, a, b); d < bestDist {
					bestDist, bestPos, best = d, closestOnSeg(endPos, a, b), qi
				}
			}
		}
	}
	var visited []polyRef
	for i := best; i >= 0; i = nodes[i].parent {
		visited = append(visited, nodes[i].ref)
	}
	for i, j := 0, len(visited)-1; i < j; i, j = i+1, j-1 {
		visited[i], visited[j] = visited[j], visited[i]
	}
	return bestPos, visited
}

// fixupCorridor makes the corridor start at the polygon reached (visited runs from the old first
// polygon to the reached one): it keeps the rest of the corridor after the furthest polygon shared
// with the visited ones (Detour sample fixupCorridor).
func fixupCorridor(path, visited []polyRef) []polyRef {
	furthestPath, furthestVisited := -1, -1
	for i := len(path) - 1; i >= 0 && furthestPath < 0; i-- {
		for j := len(visited) - 1; j >= 0; j-- {
			if path[i] == visited[j] {
				furthestPath, furthestVisited = i, j
			}
		}
	}
	if furthestPath < 0 {
		return path
	}
	out := make([]polyRef, 0, len(visited)-furthestVisited+len(path)-furthestPath-1)
	for j := len(visited) - 1; j >= furthestVisited; j-- {
		out = append(out, visited[j])
	}
	return append(out, path[furthestPath+1:]...)
}

// polyHeight is the surface height of the polygon at pt (closest edge point when pt sits on a border).
func (m *Mesh) polyHeight(r polyRef, pt V3) (float32, bool) {
	t, p := m.poly(r)
	if t == nil {
		return 0, false
	}
	if h, ok := t.heightIn(int(r.idx), pt); ok {
		return h, true
	}
	vs := t.polyVerts(p)
	bd := float32(math.MaxFloat32)
	var h float32
	for a, b := len(vs)-1, 0; b < len(vs); a, b = b, b+1 {
		if d := distToSeg2D(pt, vs[a], vs[b]); d < bd {
			bd, h = d, closestOnSeg(pt, vs[a], vs[b])[1]
		}
	}
	return h, bd < math.MaxFloat32
}
