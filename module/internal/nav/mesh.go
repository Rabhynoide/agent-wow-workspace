// Package nav reads AzerothCore mmtiles (Detour navmesh) and finds walkable paths.
//
// All coordinates here are Recast space: x = WoW Y, y = WoW Z (up), z = WoW X.
package nav

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
)

const (
	NavGround = 0x01
	NavWater  = 0x08

	tileHeaderSize = 56 // MmapTileHeader (20) + MmapTileRecastConfig (36)
	dtMagic        = 0x444E4156
	extLink        = 0x8000
)

type V3 [3]float32

// ToRecast converts WoW (x, y, z) to Recast space.
func ToRecast(x, y, z float32) V3 { return V3{y, z, x} }

// ToWoW converts Recast space back to WoW (x, y, z).
func (v V3) ToWoW() (x, y, z float32) { return v[2], v[0], v[1] }

type poly struct {
	verts [6]uint16
	neis  [6]uint16
	flags uint16
	n     uint8
	area  uint8
	typ   uint8
}

type detailMesh struct {
	vertBase, triBase   uint32
	vertCount, triCount uint8
}

// Tile is one parsed Detour tile.
type Tile struct {
	X, Y       int32
	Min, Max   V3
	climb      float32
	verts      []float32
	polys      []poly
	dmeshes    []detailMesh
	dverts     []float32
	dtris      []byte
	pmin, pmax []V3 // per-poly bounds
}

func parseTile(data []byte) (*Tile, error) {
	if len(data) < tileHeaderSize+100 {
		return nil, fmt.Errorf("tile too short")
	}
	d := data[tileHeaderSize:]
	u32 := func(o int) uint32 { return binary.LittleEndian.Uint32(d[o:]) }
	f32 := func(o int) float32 { return math.Float32frombits(u32(o)) }
	if u32(0) != dtMagic || u32(4) != 7 {
		return nil, fmt.Errorf("bad detour header magic=%x version=%d", u32(0), u32(4))
	}
	t := &Tile{X: int32(u32(8)), Y: int32(u32(12))}
	polyCount, vertCount, maxLinks := int(u32(24)), int(u32(28)), int(u32(32))
	dmCount, dvCount, dtCount := int(u32(36)), int(u32(40)), int(u32(44))
	t.climb = f32(68)
	t.Min = V3{f32(72), f32(76), f32(80)}
	t.Max = V3{f32(84), f32(88), f32(92)}
	off := 100
	need := off + vertCount*12 + polyCount*32 + maxLinks*12 + dmCount*12 + dvCount*12 + dtCount*4
	if need > len(d) {
		return nil, fmt.Errorf("tile truncated: need %d have %d", need, len(d))
	}
	t.verts = make([]float32, vertCount*3)
	for i := range t.verts {
		t.verts[i] = f32(off + 4*i)
	}
	off += vertCount * 12
	t.polys = make([]poly, polyCount)
	for i := range t.polys {
		p := &t.polys[i]
		o := off + i*32
		for j := 0; j < 6; j++ {
			p.verts[j] = binary.LittleEndian.Uint16(d[o+4+2*j:])
			p.neis[j] = binary.LittleEndian.Uint16(d[o+16+2*j:])
		}
		p.flags = binary.LittleEndian.Uint16(d[o+28:])
		p.n = d[o+30]
		p.area = d[o+31] & 0x3f
		p.typ = d[o+31] >> 6
	}
	off += polyCount*32 + maxLinks*12
	t.dmeshes = make([]detailMesh, dmCount)
	for i := range t.dmeshes {
		o := off + i*12
		t.dmeshes[i] = detailMesh{u32(o), u32(o + 4), d[o+8], d[o+9]}
	}
	off += dmCount * 12
	t.dverts = make([]float32, dvCount*3)
	for i := range t.dverts {
		t.dverts[i] = f32(off + 4*i)
	}
	off += dvCount * 12
	t.dtris = d[off : off+dtCount*4]
	t.pmin = make([]V3, polyCount)
	t.pmax = make([]V3, polyCount)
	for i := range t.polys {
		p := &t.polys[i]
		lo := V3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
		hi := V3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
		for j := 0; j < int(p.n); j++ {
			v := t.vert(p.verts[j])
			for k := 0; k < 3; k++ {
				lo[k] = min(lo[k], v[k])
				hi[k] = max(hi[k], v[k])
			}
		}
		t.pmin[i], t.pmax[i] = lo, hi
	}
	return t, nil
}

func (t *Tile) vert(i uint16) V3 {
	return V3{t.verts[3*int(i)], t.verts[3*int(i)+1], t.verts[3*int(i)+2]}
}

// Mesh lazily loads the tiles of one map.
type Mesh struct {
	MapID uint32
	dir   string
	orig  V3
	tileW float32
	tileH float32
	mu    sync.Mutex
	files map[[2]int32]string
	tiles map[[2]int32]*Tile // nil value = no tile there
	used  map[[2]int32]uint64
	clock uint64
	// MaxTiles bounds the parsed tiles kept in memory (least recently used are dropped).
	MaxTiles int
}

// DefaultMaxTiles keeps roughly 100-200 MB of parsed tiles (a tile covers 533 yards).
const DefaultMaxTiles = 128

// LoadMap reads the map parameters file; tiles are loaded on demand.
func LoadMap(dir string, mapID uint32) (*Mesh, error) {
	b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%03d.mmap", mapID)))
	if err != nil {
		return nil, err
	}
	if len(b) < 28 {
		return nil, fmt.Errorf("short .mmap file")
	}
	f := func(i int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:])) }
	m := &Mesh{MapID: mapID, dir: dir, orig: V3{f(0), f(1), f(2)}, tileW: f(3), tileH: f(4),
		files: map[[2]int32]string{}, tiles: map[[2]int32]*Tile{}, used: map[[2]int32]uint64{}, MaxTiles: DefaultMaxTiles}
	// File names use ADT grid indices, not Detour tile indices, so index by header.
	names, _ := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%03d*.mmtile", mapID)))
	for _, name := range names {
		fh, err := os.Open(name)
		if err != nil {
			continue
		}
		hdr := make([]byte, tileHeaderSize+16)
		_, err = fh.Read(hdr)
		fh.Close()
		if err != nil || binary.LittleEndian.Uint32(hdr[tileHeaderSize:]) != dtMagic {
			continue
		}
		x := int32(binary.LittleEndian.Uint32(hdr[tileHeaderSize+8:]))
		y := int32(binary.LittleEndian.Uint32(hdr[tileHeaderSize+12:]))
		m.files[[2]int32{x, y}] = name
	}
	return m, nil
}

// TileCoord returns the tile indices containing the Recast-space point.
func (m *Mesh) TileCoord(p V3) (int32, int32) {
	return int32(math.Floor(float64((p[0] - m.orig[0]) / m.tileW))), int32(math.Floor(float64((p[2] - m.orig[2]) / m.tileH)))
}

// Tile returns the tile at (x, y) or nil when the map has none there.
func (m *Mesh) Tile(x, y int32) *Tile {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := [2]int32{x, y}
	m.clock++
	if t, ok := m.tiles[k]; ok {
		if t != nil {
			m.used[k] = m.clock
		}
		return t
	}
	var t *Tile
	if name, ok := m.files[k]; ok {
		if data, err := os.ReadFile(name); err == nil {
			if tt, err := parseTile(data); err == nil {
				t = tt
			}
		}
	}
	m.tiles[k] = t
	if t != nil {
		m.used[k] = m.clock
		m.evict()
	}
	return t
}

// evict drops the least recently used tiles above MaxTiles; they are re-read on demand.
func (m *Mesh) evict() {
	for m.MaxTiles > 0 && len(m.used) > m.MaxTiles {
		var oldest [2]int32
		first := true
		for k, c := range m.used {
			if first || c < m.used[oldest] {
				oldest, first = k, false
			}
		}
		delete(m.used, oldest)
		delete(m.tiles, oldest)
	}
}

// polyRef identifies a polygon: tile coordinates and index.
// PolyRef identifies a polygon.
type PolyRef = polyRef

type polyRef struct {
	tx, ty int32
	idx    int32
}

func (m *Mesh) poly(r polyRef) (*Tile, *poly) {
	t := m.Tile(r.tx, r.ty)
	if t == nil {
		return nil, nil
	}
	return t, &t.polys[r.idx]
}

// Tiles lists the tiles of the map (indices) and TileCenter gives the Recast-space centre of one
// (height 0): offline tools sample the map tile by tile.
func (m *Mesh) Tiles() [][2]int32 {
	var out [][2]int32
	for k := range m.files {
		out = append(out, k)
	}
	return out
}

func (m *Mesh) TileCenter(x, y int32) V3 {
	return V3{m.orig[0] + (float32(x)+0.5)*m.tileW, 0, m.orig[2] + (float32(y)+0.5)*m.tileH}
}
