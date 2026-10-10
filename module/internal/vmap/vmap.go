// Package vmap reads the server's collision geometry (vmaps: buildings, bridges, rocks, trees) and
// answers the two questions the server asks it: the floor height under a point and the line of sight
// between two points. It is a port of AzerothCore's StaticMapTree / ModelInstance / WorldModel /
// BIH (src/common/Collision), reading the same files (assets/vmaps).
package vmap

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
)

const (
	magic = "VMAP_4.8"
	// mid converts world coordinates to the internal ones: internal = (mid - x, mid - y, z).
	mid      = 0.5 * 64 * 533.33333
	gridSize = 533.33333

	modM2       = 1
	modHasBound = 1 << 2
)

// Tree is the static collision tree of one map; model tiles are loaded on demand.
type Tree struct {
	dir   string
	mapID uint32
	tiled bool

	mu        sync.Mutex
	bih       bih
	instances []*instance // by tree object index; nil until its tile is loaded
	tiles     map[[2]int]bool
	models    map[string]*worldModel
}

type instance struct {
	flags    uint32
	pos      [3]float32
	scale    float32
	lo, hi   [3]float32 // bounds, internal coordinates
	invRot   mat3
	invScale float32
	model    *worldModel
	name     string
}

type worldModel struct {
	flags     uint32
	groups    []groupModel
	groupTree bih
}

type groupModel struct {
	verts    [][3]float32
	tris     [][3]uint32
	meshTree bih
}

// Open reads the map tree (<map>.vmtree).
func Open(dir string, mapID uint32) (*Tree, error) {
	f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%03d.vmtree", mapID)))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := &reader{r: f}
	if r.chunk(8) != magic {
		return nil, fmt.Errorf("%03d.vmtree: bad magic", mapID)
	}
	t := &Tree{dir: dir, mapID: mapID, tiles: map[[2]int]bool{}, models: map[string]*worldModel{}}
	t.tiled = r.u8() != 0
	if r.chunk(4) != "NODE" {
		return nil, fmt.Errorf("%03d.vmtree: no NODE chunk", mapID)
	}
	t.bih = r.bih()
	if r.chunk(4) != "GOBJ" || r.err != nil {
		return nil, fmt.Errorf("%03d.vmtree: truncated", mapID)
	}
	t.instances = make([]*instance, len(t.bih.objects))
	if !t.tiled { // a single global model (instances, dungeons)
		if sp, ok := r.spawn(); ok {
			if m, err := t.model(sp.name, sp.flags); err == nil && len(t.instances) > 0 {
				t.instances[0] = newInstance(sp, m)
			}
		}
	}
	return t, r.err
}

// Height is the floor under (x, y) (world coordinates) searching down from z+2 for up to 50 yards,
// as the server does for a walking unit (WorldObject::GetMapHeight). ok is false when no model is hit.
func (t *Tree) Height(x, y, z float32) (float32, bool) {
	t.loadAround(x, y)
	org := [3]float32{mid - x, mid - y, z + 2}
	dist := float32(50)
	t.mu.Lock()
	hit := t.intersect(org, [3]float32{0, 0, -1}, &dist, false, false)
	t.mu.Unlock()
	if !hit {
		return 0, false
	}
	return org[2] - dist, true
}

// LineOfSight tells whether nothing of the static geometry stands between the two points (world
// coordinates); doodads (M2: trees, crates) are ignored like the server's spell checks do.
func (t *Tree) LineOfSight(a, b [3]float32) bool {
	t.loadAround(a[0], a[1])
	t.loadAround(b[0], b[1])
	p1 := [3]float32{mid - a[0], mid - a[1], a[2]}
	p2 := [3]float32{mid - b[0], mid - b[1], b[2]}
	d := [3]float32{p2[0] - p1[0], p2[1] - p1[1], p2[2] - p1[2]}
	l := float32(math.Sqrt(float64(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])))
	if l < 1e-6 {
		return true
	}
	dir := [3]float32{d[0] / l, d[1] / l, d[2] / l}
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.intersect(p1, dir, &l, true, true)
}

func (t *Tree) intersect(org, dir [3]float32, maxDist *float32, stopAtFirst, ignoreM2 bool) bool {
	hit := false
	t.bih.intersectRay(org, dir, maxDist, stopAtFirst, func(obj uint32, dist *float32) bool {
		if int(obj) >= len(t.instances) || t.instances[obj] == nil {
			return false
		}
		in := t.instances[obj]
		if ignoreM2 && in.flags&modM2 != 0 {
			return false
		}
		if in.intersectRay(org, dir, dist, stopAtFirst) {
			hit = true
			return true
		}
		return false
	})
	return hit
}

// loadAround loads the vmap tile under (x, y) and its neighbours (models spanning tiles are listed in each).
func (t *Tree) loadAround(x, y float32) {
	gx := int(math.Floor(32 - float64(x)/gridSize))
	gy := int(math.Floor(32 - float64(y)/gridSize))
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			t.loadTile(gx+dx, gy+dy)
		}
	}
}

func (t *Tree) loadTile(gx, gy int) {
	if !t.tiled || gx < 0 || gy < 0 || gx > 63 || gy > 63 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := [2]int{gx, gy}
	if _, done := t.tiles[k]; done {
		return
	}
	t.tiles[k] = true
	// File names put the y grid index first (StaticMapTree::getTileFileName).
	f, err := os.Open(filepath.Join(t.dir, fmt.Sprintf("%03d_%02d_%02d.vmtile", t.mapID, gy, gx)))
	if err != nil {
		return
	}
	defer f.Close()
	r := &reader{r: f}
	if r.chunk(8) != magic {
		return
	}
	n := r.u32()
	for i := uint32(0); i < n && r.err == nil; i++ {
		sp, ok := r.spawn()
		if !ok {
			return
		}
		ref := r.u32()
		if int(ref) >= len(t.instances) || t.instances[ref] != nil {
			continue
		}
		m, err := t.model(sp.name, sp.flags)
		if err != nil {
			continue
		}
		t.instances[ref] = newInstance(sp, m)
	}
}

// model loads (once) a <name>.vmo world model.
func (t *Tree) model(name string, flags uint32) (*worldModel, error) {
	if m, ok := t.models[name]; ok {
		return m, nil
	}
	f, err := os.Open(filepath.Join(t.dir, name+".vmo"))
	if err != nil {
		t.models[name] = nil
		return nil, err
	}
	defer f.Close()
	r := &reader{r: f}
	m := &worldModel{flags: flags}
	if r.chunk(8) != magic || r.chunk(4) != "WMOD" {
		return nil, fmt.Errorf("%s: bad header", name)
	}
	r.u32() // chunk size
	r.u32() // root WMO id
	if r.chunk(4) == "GMOD" {
		count := r.u32()
		for i := uint32(0); i < count && r.err == nil; i++ {
			m.groups = append(m.groups, r.group())
		}
		if r.chunk(4) == "GBIH" {
			m.groupTree = r.bih()
		}
	}
	if r.err != nil {
		return nil, fmt.Errorf("%s: %w", name, r.err)
	}
	t.models[name] = m
	return m, nil
}

type spawn struct {
	flags    uint32
	pos, rot [3]float32
	scale    float32
	lo, hi   [3]float32
	name     string
}

func newInstance(sp spawn, m *worldModel) *instance {
	// G3D Matrix3::fromEulerAnglesZYX(pi*rot.y/180, pi*rot.x/180, pi*rot.z/180) = Rz * Ry * Rx; inverse = transpose.
	rad := func(d float32) float64 { return math.Pi * float64(d) / 180 }
	cz, sz := math.Cos(rad(sp.rot[1])), math.Sin(rad(sp.rot[1]))
	cy, sy := math.Cos(rad(sp.rot[0])), math.Sin(rad(sp.rot[0]))
	cx, sx := math.Cos(rad(sp.rot[2])), math.Sin(rad(sp.rot[2]))
	rz := [3][3]float64{{cz, -sz, 0}, {sz, cz, 0}, {0, 0, 1}}
	ry := [3][3]float64{{cy, 0, sy}, {0, 1, 0}, {-sy, 0, cy}}
	rx := [3][3]float64{{1, 0, 0}, {0, cx, -sx}, {0, sx, cx}}
	rot := mul(rz, mul(ry, rx))
	in := &instance{flags: sp.flags, pos: sp.pos, scale: sp.scale, lo: sp.lo, hi: sp.hi, invScale: 1 / sp.scale, model: m, name: sp.name}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			in.invRot[i][j] = float32(rot[j][i])
		}
	}
	return in
}

func mul(a, b [3][3]float64) [3][3]float64 {
	var c [3][3]float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				c[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return c
}

// mat3 is a 3x3 rotation matrix (row major).
type mat3 [3][3]float32

func (m *mat3) apply(v [3]float32) [3]float32 {
	return [3]float32{
		m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2],
		m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2],
		m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2],
	}
}

// intersectRay tests the ray (internal coordinates) against the model in its own space (ModelInstance::intersectRay).
func (in *instance) intersectRay(org, dir [3]float32, maxDist *float32, stopAtFirst bool) bool {
	if in.model == nil || !rayHitsBox(org, dir, in.lo, in.hi, *maxDist) {
		return false
	}
	p := in.invRot.apply([3]float32{org[0] - in.pos[0], org[1] - in.pos[1], org[2] - in.pos[2]})
	p = [3]float32{p[0] * in.invScale, p[1] * in.invScale, p[2] * in.invScale}
	d := in.invRot.apply(dir)
	dist := *maxDist * in.invScale
	if !in.model.intersectRay(p, d, &dist, stopAtFirst) {
		return false
	}
	*maxDist = dist * in.scale
	return true
}

// rayHitsBox is G3D Ray::intersectionTime(AABox) < inf, limited to maxDist.
func rayHitsBox(org, dir, lo, hi [3]float32, maxDist float32) bool {
	tmin, tmax := float32(0), maxDist
	for i := 0; i < 3; i++ {
		if dir[i] == 0 {
			if org[i] < lo[i] || org[i] > hi[i] {
				return false
			}
			continue
		}
		t1, t2 := (lo[i]-org[i])/dir[i], (hi[i]-org[i])/dir[i]
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tmin, tmax = max(tmin, t1), min(tmax, t2)
		if tmin > tmax {
			return false
		}
	}
	return true
}

func (m *worldModel) intersectRay(org, dir [3]float32, dist *float32, stopAtFirst bool) bool {
	if len(m.groups) == 1 {
		return m.groups[0].intersectRay(org, dir, dist, stopAtFirst)
	}
	hit := false
	m.groupTree.intersectRay(org, dir, dist, stopAtFirst, func(obj uint32, d *float32) bool {
		if int(obj) < len(m.groups) && m.groups[obj].intersectRay(org, dir, d, stopAtFirst) {
			hit = true
		}
		return hit
	})
	return hit
}

func (g *groupModel) intersectRay(org, dir [3]float32, dist *float32, stopAtFirst bool) bool {
	if len(g.tris) == 0 {
		return false
	}
	hit := false
	g.meshTree.intersectRay(org, dir, dist, stopAtFirst, func(obj uint32, d *float32) bool {
		if int(obj) < len(g.tris) && intersectTriangle(g.tris[obj], g.verts, org, dir, d) {
			hit = true
		}
		return hit
	})
	return hit
}

func sub(a, b [3]float32) [3]float32 { return [3]float32{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func dot(a, b [3]float32) float32    { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b [3]float32) [3]float32 {
	return [3]float32{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// intersectTriangle is the Möller–Trumbore test of WorldModel.cpp IntersectTriangle.
func intersectTriangle(tri [3]uint32, verts [][3]float32, org, dir [3]float32, dist *float32) bool {
	const eps = 1e-5
	if int(tri[0]) >= len(verts) || int(tri[1]) >= len(verts) || int(tri[2]) >= len(verts) {
		return false
	}
	v0 := verts[tri[0]]
	e1, e2 := sub(verts[tri[1]], v0), sub(verts[tri[2]], v0)
	p := cross(dir, e2)
	a := dot(e1, p)
	if a > -eps && a < eps {
		return false
	}
	f := 1 / a
	s := sub(org, v0)
	u := f * dot(s, p)
	if u < 0 || u > 1 {
		return false
	}
	q := cross(s, e1)
	v := f * dot(dir, q)
	if v < 0 || u+v > 1 {
		return false
	}
	if t := f * dot(e2, q); t > 0 && t < *dist {
		*dist = t
		return true
	}
	return false
}

// ---- file reading ----------------------------------------------------------

type reader struct {
	r   io.Reader
	err error
	buf [8]byte
}

func (r *reader) read(b []byte) {
	if r.err == nil {
		_, r.err = io.ReadFull(r.r, b)
	}
}

func (r *reader) chunk(n int) string {
	b := make([]byte, n)
	r.read(b)
	return string(b)
}

func (r *reader) u8() uint8 {
	r.read(r.buf[:1])
	return r.buf[0]
}

func (r *reader) u16() uint16 {
	r.read(r.buf[:2])
	return binary.LittleEndian.Uint16(r.buf[:2])
}

func (r *reader) u32() uint32 {
	r.read(r.buf[:4])
	return binary.LittleEndian.Uint32(r.buf[:4])
}

func (r *reader) f32() float32 { return math.Float32frombits(r.u32()) }

func (r *reader) vec() [3]float32 { return [3]float32{r.f32(), r.f32(), r.f32()} }

func (r *reader) u32s(n uint32) []uint32 {
	if n > 1<<28 {
		r.err = fmt.Errorf("bad count %d", n)
		return nil
	}
	b := make([]byte, 4*int(n))
	r.read(b)
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(b[4*i:])
	}
	return out
}

func (r *reader) bih() bih {
	var b bih
	b.lo, b.hi = r.vec(), r.vec()
	b.tree = r.u32s(r.u32())
	b.objects = r.u32s(r.u32())
	return b
}

// spawn reads a ModelSpawn (flags, adt id, id, position, rotation, scale, [bounds], name).
func (r *reader) spawn() (spawn, bool) {
	var sp spawn
	sp.flags = r.u32()
	r.u16() // adt id
	r.u32() // unique id
	sp.pos, sp.rot = r.vec(), r.vec()
	sp.scale = r.f32()
	if sp.flags&modHasBound != 0 {
		sp.lo, sp.hi = r.vec(), r.vec()
	}
	n := r.u32()
	if r.err != nil || n > 500 {
		return sp, false
	}
	sp.name = r.chunk(int(n))
	return sp, r.err == nil
}

// group reads a GroupModel; liquids are skipped (their size is given).
func (r *reader) group() groupModel {
	var g groupModel
	r.vec() // bound low
	r.vec() // bound high
	r.u32() // mogp flags
	r.u32() // group WMO id
	if r.chunk(4) != "VERT" {
		r.err = fmt.Errorf("no VERT chunk")
		return g
	}
	r.u32() // chunk size
	n := r.u32()
	if n == 0 { // no collision geometry: the group ends here
		return g
	}
	raw := r.u32s(3 * n)
	g.verts = make([][3]float32, n)
	for i := range g.verts {
		g.verts[i] = [3]float32{math.Float32frombits(raw[3*i]), math.Float32frombits(raw[3*i+1]), math.Float32frombits(raw[3*i+2])}
	}
	if r.chunk(4) != "TRIM" {
		r.err = fmt.Errorf("no TRIM chunk")
		return g
	}
	r.u32()
	n = r.u32()
	raw = r.u32s(3 * n)
	g.tris = make([][3]uint32, n)
	for i := range g.tris {
		g.tris[i] = [3]uint32{raw[3*i], raw[3*i+1], raw[3*i+2]}
	}
	if r.chunk(4) != "MBIH" {
		r.err = fmt.Errorf("no MBIH chunk")
		return g
	}
	g.meshTree = r.bih()
	if r.chunk(4) != "LIQU" {
		r.err = fmt.Errorf("no LIQU chunk")
		return g
	}
	if size := r.u32(); size > 0 && r.err == nil {
		_, r.err = io.CopyN(io.Discard, r.r, int64(size))
	}
	return g
}
