package nav

import (
	"container/heap"
	"errors"
	"math"
)

// partialReach is how far (yards) the closest reachable point may be from an unreachable destination
// for the path to end there instead of failing.
const partialReach = 40

var (
	ErrNoStartPoly = errors.New("start position is not on the navmesh")
	ErrNoEndPoly   = errors.New("destination is not on the navmesh")
	ErrNoPath      = errors.New("no walkable path")
)

// Filter selects walkable polygons.
// Partial lets a path end at the closest reachable point when the destination is in water, on a
// cliff or on an island; without it such a destination is an error.
type Filter struct {
	Include uint16
	Partial bool
}

var GroundOnly = Filter{Include: NavGround}

// GroundPartial is GroundOnly with partial paths.
var GroundPartial = Filter{Include: NavGround, Partial: true}

func (f Filter) ok(p *poly) bool { return p.typ == 0 && p.flags&f.Include != 0 }

func dist2D(a, b V3) float32 {
	dx, dz := a[0]-b[0], a[2]-b[2]
	return float32(math.Sqrt(float64(dx*dx + dz*dz)))
}

func dist3D(a, b V3) float32 {
	dx, dy, dz := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
}

func (t *Tile) polyVerts(p *poly) []V3 {
	out := make([]V3, p.n)
	for i := range out {
		out[i] = t.vert(p.verts[i])
	}
	return out
}

func inPoly2D(pt V3, vs []V3) bool {
	in := false
	for i, j := 0, len(vs)-1; i < len(vs); j, i = i, i+1 {
		if (vs[i][2] > pt[2]) != (vs[j][2] > pt[2]) &&
			pt[0] < (vs[j][0]-vs[i][0])*(pt[2]-vs[i][2])/(vs[j][2]-vs[i][2])+vs[i][0] {
			in = !in
		}
	}
	return in
}

func distToSeg2D(p, a, b V3) float32 {
	dx, dz := b[0]-a[0], b[2]-a[2]
	l := dx*dx + dz*dz
	var t float32
	if l > 0 {
		t = ((p[0]-a[0])*dx + (p[2]-a[2])*dz) / l
		t = max(0, min(1, t))
	}
	return dist2D(p, V3{a[0] + t*dx, 0, a[2] + t*dz})
}

// heightIn returns the surface height of the polygon at the point's x/z, if inside.
func (t *Tile) heightIn(idx int, pt V3) (float32, bool) {
	p := &t.polys[idx]
	vs := t.polyVerts(p)
	if !inPoly2D(pt, vs) {
		return 0, false
	}
	dm := t.dmeshes[idx]
	for i := 0; i < int(dm.triCount); i++ {
		tri := t.dtris[(int(dm.triBase)+i)*4:]
		var v [3]V3
		for k := 0; k < 3; k++ {
			if int(tri[k]) < int(p.n) {
				v[k] = vs[tri[k]]
			} else {
				o := (int(dm.vertBase) + int(tri[k]) - int(p.n)) * 3
				v[k] = V3{t.dverts[o], t.dverts[o+1], t.dverts[o+2]}
			}
		}
		if h, ok := triHeight(pt, v[0], v[1], v[2]); ok {
			return h, true
		}
	}
	// Fall back to the polygon plane approximation.
	var s float32
	for _, v := range vs {
		s += v[1]
	}
	return s / float32(len(vs)), true
}

func triHeight(p, a, b, c V3) (float32, bool) {
	const eps = 1e-4
	v0x, v0z := c[0]-a[0], c[2]-a[2]
	v1x, v1z := b[0]-a[0], b[2]-a[2]
	v2x, v2z := p[0]-a[0], p[2]-a[2]
	den := v0x*v1z - v0z*v1x
	if math.Abs(float64(den)) < 1e-9 {
		return 0, false
	}
	u := (v1z*v2x - v1x*v2z) / den
	v := (v0x*v2z - v0z*v2x) / den
	if u < -eps || v < -eps || u+v > 1+eps {
		return 0, false
	}
	return a[1] + (c[1]-a[1])*u + (b[1]-a[1])*v, true
}

// NearestPoly finds the polygon under pt (within vertical reach), falling back to the closest within reach2D.
func (m *Mesh) NearestPoly(pt V3, f Filter, vert, reach2D float32) (polyRef, V3, bool) {
	tx, ty := m.TileCoord(pt)
	var best polyRef
	var bestPt V3
	bestD := float32(math.MaxFloat32)
	found := false
	for dx := int32(-1); dx <= 1; dx++ {
		for dy := int32(-1); dy <= 1; dy++ {
			t := m.Tile(tx+dx, ty+dy)
			if t == nil {
				continue
			}
			for i := range t.polys {
				p := &t.polys[i]
				if !f.ok(p) || pt[0] < t.pmin[i][0]-reach2D || pt[0] > t.pmax[i][0]+reach2D ||
					pt[2] < t.pmin[i][2]-reach2D || pt[2] > t.pmax[i][2]+reach2D {
					continue
				}
				if h, ok := t.heightIn(i, pt); ok {
					d := float32(math.Abs(float64(h - pt[1])))
					if d <= vert && d < bestD {
						best, bestPt, bestD, found = polyRef{tx + dx, ty + dy, int32(i)}, V3{pt[0], h, pt[2]}, d, true
					}
				}
			}
		}
	}
	if found {
		return best, bestPt, true
	}
	// Nothing directly below: take the closest polygon edge within reach2D.
	bestD = reach2D
	for dx := int32(-1); dx <= 1; dx++ {
		for dy := int32(-1); dy <= 1; dy++ {
			t := m.Tile(tx+dx, ty+dy)
			if t == nil {
				continue
			}
			for i := range t.polys {
				p := &t.polys[i]
				if !f.ok(p) || pt[1] < t.pmin[i][1]-vert || pt[1] > t.pmax[i][1]+vert {
					continue
				}
				vs := t.polyVerts(p)
				for a, b := len(vs)-1, 0; b < len(vs); a, b = b, b+1 {
					if d := distToSeg2D(pt, vs[a], vs[b]); d < bestD {
						bestD = d
						best = polyRef{tx + dx, ty + dy, int32(i)}
						bestPt = closestOnSeg(pt, vs[a], vs[b])
						found = true
					}
				}
			}
		}
	}
	return best, bestPt, found
}

func closestOnSeg(p, a, b V3) V3 {
	dx, dz := b[0]-a[0], b[2]-a[2]
	l := dx*dx + dz*dz
	var t float32
	if l > 0 {
		t = max(0, min(1, ((p[0]-a[0])*dx+(p[2]-a[2])*dz)/l))
	}
	return V3{a[0] + t*dx, a[1] + t*(b[1]-a[1]), a[2] + t*dz}
}

type link struct {
	to     polyRef
	pa, pb V3  // portal segment (left, right as seen leaving the source polygon)
	edge   int // edge of the source polygon (vertex edge -> edge+1)
}

func oppositeSide(s uint16) uint16 { return (s + 4) & 7 }

var sideOffset = [8][2]int32{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}

// links lists the polygons reachable from r through one edge.
func (m *Mesh) links(r polyRef, f Filter) []link {
	t, p := m.poly(r)
	if t == nil {
		return nil
	}
	var out []link
	n := int(p.n)
	for i := 0; i < n; i++ {
		nei := p.neis[i]
		if nei == 0 {
			continue
		}
		va, vb := t.vert(p.verts[i]), t.vert(p.verts[(i+1)%n])
		if nei&extLink == 0 {
			idx := int32(nei) - 1
			if f.ok(&t.polys[idx]) {
				out = append(out, link{polyRef{r.tx, r.ty, idx}, va, vb, i})
			}
			continue
		}
		side := nei & 0xff
		if side >= 8 {
			continue
		}
		nt := m.Tile(r.tx+sideOffset[side][0], r.ty+sideOffset[side][1])
		if nt == nil {
			continue
		}
		opp := oppositeSide(side)
		for j := range nt.polys {
			np := &nt.polys[j]
			if !f.ok(np) {
				continue
			}
			nn := int(np.n)
			for k := 0; k < nn; k++ {
				if np.neis[k] != extLink|opp {
					continue
				}
				ua, ub := nt.vert(np.verts[k]), nt.vert(np.verts[(k+1)%nn])
				if lo, hi, ok := overlapSlab(side, va, vb, ua, ub, t.climb); ok {
					out = append(out, link{polyRef{r.tx + sideOffset[side][0], r.ty + sideOffset[side][1], int32(j)}, lo, hi, i})
				}
			}
		}
	}
	return out
}

// overlapSlab intersects two portal edges that lie on opposite sides of a tile boundary.
func overlapSlab(side uint16, a0, a1, b0, b1 V3, climb float32) (V3, V3, bool) {
	axis := 2 // crossing +-x: edges run along z
	if side == 2 || side == 6 {
		axis = 0
	}
	if math.Abs(float64(a0[2-axis]-b0[2-axis])) > 0.5 && math.Abs(float64(a0[2-axis]-b1[2-axis])) > 0.5 {
		return a0, a1, false
	}
	lo := max(min(a0[axis], a1[axis]), min(b0[axis], b1[axis]))
	hi := min(max(a0[axis], a1[axis]), max(b0[axis], b1[axis]))
	if hi-lo < 0.05 {
		return a0, a1, false
	}
	at := func(p, q V3, c float32) V3 {
		if q[axis] == p[axis] {
			return p
		}
		s := (c - p[axis]) / (q[axis] - p[axis])
		return V3{p[0] + s*(q[0]-p[0]), p[1] + s*(q[1]-p[1]), p[2] + s*(q[2]-p[2])}
	}
	pa, pb := at(a0, a1, lo), at(a0, a1, hi)
	qa, qb := at(b0, b1, lo), at(b0, b1, hi)
	if abs32(pa[1]-qa[1]) > climb*2+1 || abs32(pb[1]-qb[1]) > climb*2+1 {
		return a0, a1, false
	}
	// Keep the source polygon's orientation (a0 -> a1).
	if a0[axis] > a1[axis] {
		pa, pb = pb, pa
	}
	return pa, pb, true
}

func abs32(f float32) float32 { return float32(math.Abs(float64(f))) }

type node struct {
	ref    polyRef
	pos    V3
	g, f   float32
	parent *node
	portal [2]V3 // portal used to enter this node (from parent)
	idx    int
}

type pq []*node

func (q pq) Len() int           { return len(q) }
func (q pq) Less(i, j int) bool { return q[i].f < q[j].f }
func (q pq) Swap(i, j int)      { q[i], q[j] = q[j], q[i]; q[i].idx = i; q[j].idx = j }
func (q *pq) Push(x any)        { n := x.(*node); n.idx = len(*q); *q = append(*q, n) }
func (q *pq) Pop() any          { o := *q; n := o[len(o)-1]; *q = o[:len(o)-1]; return n }

// Path finds a smoothed path (Recast space) between two Recast-space points.
func (m *Mesh) Path(from, to V3, f Filter) ([]V3, error) {
	pts, _, err := m.PathCorridor(from, to, f)
	return pts, err
}

// PathCorridor is Path plus the polygons the path runs through (used to resolve heights between layers).
func (m *Mesh) PathCorridor(from, to V3, f Filter) ([]V3, []polyRef, error) {
	sPt, ePt, chain, err := m.corridor(from, to, f)
	if err != nil {
		return nil, nil, err
	}
	refs := make([]polyRef, len(chain))
	for i, n := range chain {
		refs[i] = n.ref
	}
	if len(chain) == 1 {
		return []V3{sPt, ePt}, refs, nil
	}
	return funnel(sPt, ePt, chain), refs, nil
}

// corridor runs A* over the polygons: start and end points snapped on the mesh, and the chain of
// nodes from the start polygon to the end polygon (each with the portal used to enter it).
func (m *Mesh) corridor(from, to V3, f Filter) (V3, V3, []*node, error) {
	sRef, sPt, ok := m.NearestPoly(from, f, 6, 3)
	if !ok {
		return V3{}, V3{}, nil, ErrNoStartPoly
	}
	eRef, ePt, ok := m.NearestPoly(to, f, 6, 3)
	if !ok {
		// Destination in water, on a cliff or inside a wall: with Partial, like the server, go to
		// the closest walkable point instead (within 30 yards); the path then ends short of the target.
		if !f.Partial {
			return V3{}, V3{}, nil, ErrNoEndPoly
		}
		if eRef, ePt, ok = m.NearestPoly(to, f, 12, 30); !ok {
			return V3{}, V3{}, nil, ErrNoEndPoly
		}
	}
	if sRef == eRef {
		return sPt, ePt, []*node{{ref: sRef, pos: sPt}}, nil
	}
	best := map[polyRef]*node{}
	open := &pq{}
	start := &node{ref: sRef, pos: sPt, f: dist3D(sPt, ePt)}
	best[sRef] = start
	heap.Push(open, start)
	closed := map[polyRef]bool{}
	var goal *node
	var expansions int
	closest, closestD := start, dist3D(sPt, ePt) // closest node explored, for an incomplete path
	for open.Len() > 0 {
		cur := heap.Pop(open).(*node)
		if cur.ref == eRef {
			goal = cur
			break
		}
		if d := dist3D(cur.pos, ePt); d < closestD {
			closest, closestD = cur, d
		}
		closed[cur.ref] = true
		if expansions++; expansions > 400000 {
			break
		}
		for _, l := range m.links(cur.ref, f) {
			if closed[l.to] {
				continue
			}
			mid := V3{(l.pa[0] + l.pb[0]) / 2, (l.pa[1] + l.pb[1]) / 2, (l.pa[2] + l.pb[2]) / 2}
			g := cur.g + dist3D(cur.pos, mid)
			if l.to == eRef {
				g += dist3D(mid, ePt)
				mid = ePt
			}
			if nb := best[l.to]; nb != nil && nb.g <= g {
				continue
			}
			nb := &node{ref: l.to, pos: mid, g: g, f: g + dist3D(mid, ePt), parent: cur, portal: [2]V3{l.pa, l.pb}}
			if old := best[l.to]; old != nil && old.idx < open.Len() && (*open)[old.idx] == old {
				heap.Remove(open, old.idx)
			}
			best[l.to] = nb
			heap.Push(open, nb)
		}
	}
	if goal == nil {
		// Unreachable destination (an island, a lake shore): like the server, end at the closest
		// explored point when it is near the destination.
		if !f.Partial || closest == start || closestD > partialReach {
			return V3{}, V3{}, nil, ErrNoPath
		}
		goal, ePt = closest, closest.pos
	}
	var chain []*node
	for n := goal; n != nil; n = n.parent {
		chain = append(chain, n)
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return sPt, ePt, chain, nil
}

func triArea2(a, b, c V3) float32 {
	acx, acz := c[0]-a[0], c[2]-a[2]
	abx, abz := b[0]-a[0], b[2]-a[2]
	return acx*abz - abx*acz
}

func vEq(a, b V3) bool {
	dx, dz := a[0]-b[0], a[2]-b[2]
	return dx*dx+dz*dz < 1e-6
}

// funnel applies the simple stupid funnel algorithm over the corridor portals.
func funnel(start, end V3, chain []*node) []V3 {
	type portal struct{ l, r V3 }
	ps := []portal{{start, start}}
	for _, n := range chain[1:] {
		ps = append(ps, portal{n.portal[0], n.portal[1]})
	}
	ps = append(ps, portal{end, end})
	out := []V3{start}
	apex, left, right := start, start, start
	apexI, leftI, rightI := 0, 0, 0
	for i := 1; i < len(ps); i++ {
		l, r := ps[i].l, ps[i].r
		if triArea2(apex, right, r) <= 0 {
			if vEq(apex, right) || triArea2(apex, left, r) > 0 {
				right, rightI = r, i
			} else {
				out = append(out, left)
				apex, apexI = left, leftI
				left, right = apex, apex
				leftI, rightI = apexI, apexI
				i = apexI
				continue
			}
		}
		if triArea2(apex, left, l) >= 0 {
			if vEq(apex, left) || triArea2(apex, right, l) < 0 {
				left, leftI = l, i
			} else {
				out = append(out, right)
				apex, apexI = right, rightI
				left, right = apex, apex
				leftI, rightI = apexI, apexI
				i = apexI
				continue
			}
		}
	}
	return append(out, end)
}

// Height returns the walkable surface height near pt (Recast space).
func (m *Mesh) Height(pt V3, f Filter) (float32, bool) {
	_, p, ok := m.NearestPoly(pt, f, 4, 1.5)
	return p[1], ok
}

// OnMesh reports whether the point lies inside a walkable polygon (2D) and returns the surface height there.
func (m *Mesh) OnMesh(pt V3, f Filter) (float32, bool) {
	tx, ty := m.TileCoord(pt)
	t := m.Tile(tx, ty)
	if t == nil {
		return 0, false
	}
	best := float32(math.MaxFloat32)
	var h float32
	found := false
	for i := range t.polys {
		p := &t.polys[i]
		if !f.ok(p) || pt[0] < t.pmin[i][0] || pt[0] > t.pmax[i][0] || pt[2] < t.pmin[i][2] || pt[2] > t.pmax[i][2] {
			continue
		}
		if hh, ok := t.heightIn(i, pt); ok {
			if d := abs32(hh - pt[1]); d < best {
				best, h, found = d, hh, true
			}
		}
	}
	return h, found
}

// HeightInCorridor returns the surface height at pt using only the corridor polygons.
func (m *Mesh) HeightInCorridor(pt V3, corridor []polyRef) (float32, bool) {
	best := float32(math.MaxFloat32)
	var h float32
	found := false
	for _, r := range corridor {
		t, p := m.poly(r)
		if t == nil {
			continue
		}
		i := int(r.idx)
		if pt[0] < t.pmin[i][0]-0.01 || pt[0] > t.pmax[i][0]+0.01 || pt[2] < t.pmin[i][2]-0.01 || pt[2] > t.pmax[i][2]+0.01 {
			continue
		}
		_ = p
		if hh, ok := t.heightIn(i, pt); ok {
			if d := abs32(hh - pt[1]); d < best {
				best, h, found = d, hh, true
			}
		}
	}
	if found {
		return h, true
	}
	// Slightly off the corridor (corner cutting): take the closest corridor polygon edge.
	bd := float32(2.0)
	for _, r := range corridor {
		t, p := m.poly(r)
		if t == nil {
			continue
		}
		vs := t.polyVerts(p)
		for a, b := len(vs)-1, 0; b < len(vs); a, b = b, b+1 {
			if d := distToSeg2D(pt, vs[a], vs[b]); d < bd {
				bd, h, found = d, closestOnSeg(pt, vs[a], vs[b])[1], true
			}
		}
	}
	return h, found
}

// IslandSize counts the polygons reachable from the one under pt, stopping at limit: a small number
// means the point sits on a scrap of navmesh cut from the rest (a rock, a ledge). It returns 0 when pt
// is not on the navmesh.
func (m *Mesh) IslandSize(pt V3, f Filter, limit int) int {
	ref, _, ok := m.NearestPoly(pt, f, 6, 3)
	if !ok {
		return 0
	}
	seen := map[polyRef]bool{ref: true}
	queue := []polyRef{ref}
	for len(queue) > 0 && len(seen) < limit {
		r := queue[0]
		queue = queue[1:]
		for _, l := range m.links(r, f) {
			if !seen[l.to] {
				seen[l.to] = true
				queue = append(queue, l.to)
			}
		}
	}
	return len(seen)
}
