// Package terrain reads ground heights from AzerothCore .map files.
package terrain

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
)

const (
	gridSize = 533.33333
	res      = 128
	flagNone = 0x1
	flagI16  = 0x2
	flagI8   = 0x4
)

type grid struct {
	v9 [129 * 129]float32
	v8 [128 * 128]float32
	// area: AreaTable id per 16x16 cell (areaAll for the whole grid when the file has no cells).
	area    []uint16
	areaAll uint16
	noH     bool // no height data (area only)
}

// Maps lazily loads the height grids of one map.
type Maps struct {
	dir   string
	mapID uint32
	mu    sync.Mutex
	grids map[[2]int]*grid
}

func New(dir string, mapID uint32) *Maps {
	return &Maps{dir: dir, mapID: mapID, grids: map[[2]int]*grid{}}
}

func (m *Maps) load(gx, gy int) *grid {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := [2]int{gx, gy}
	if g, ok := m.grids[k]; ok {
		return g
	}
	g, _ := readGrid(filepath.Join(m.dir, fmt.Sprintf("%03d%02d%02d.map", m.mapID, gx, gy)))
	m.grids[k] = g
	return g
}

func readGrid(path string) (*grid, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	u32 := func(o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
	if len(b) < 44 {
		return nil, fmt.Errorf("short file")
	}
	g := &grid{}
	// Area header (GridMap::loadAreaData): fourcc, u16 flags (1 = one area for the grid), u16 area,
	// then 16x16 u16 unless flag 1.
	if ao := int(u32(12)); ao > 0 && len(b) >= ao+8 {
		flags := binary.LittleEndian.Uint16(b[ao+4:])
		g.areaAll = binary.LittleEndian.Uint16(b[ao+6:])
		if flags&1 == 0 && len(b) >= ao+8+2*256 {
			g.area = make([]uint16, 256)
			for i := range g.area {
				g.area[i] = binary.LittleEndian.Uint16(b[ao+8+2*i:])
			}
		}
	}
	if u32(20) == 0 {
		g.noH = true
		return g, nil
	}
	off := int(u32(20))
	flags := u32(off + 4)
	base := math.Float32frombits(u32(off + 8))
	max := math.Float32frombits(u32(off + 12))
	if flags&flagNone != 0 {
		for i := range g.v9 {
			g.v9[i] = base
		}
		for i := range g.v8 {
			g.v8[i] = base
		}
		return g, nil
	}
	p := off + 16
	switch {
	case flags&flagI16 != 0:
		mul := (max - base) / 65535
		if len(b) < p+2*(len(g.v9)+len(g.v8)) {
			return nil, fmt.Errorf("short file")
		}
		for i := range g.v9 {
			g.v9[i] = float32(binary.LittleEndian.Uint16(b[p+2*i:]))*mul + base
		}
		p += 2 * len(g.v9)
		for i := range g.v8 {
			g.v8[i] = float32(binary.LittleEndian.Uint16(b[p+2*i:]))*mul + base
		}
	case flags&flagI8 != 0:
		mul := (max - base) / 255
		if len(b) < p+len(g.v9)+len(g.v8) {
			return nil, fmt.Errorf("short file")
		}
		for i := range g.v9 {
			g.v9[i] = float32(b[p+i])*mul + base
		}
		p += len(g.v9)
		for i := range g.v8 {
			g.v8[i] = float32(b[p+i])*mul + base
		}
	default:
		if len(b) < p+4*(len(g.v9)+len(g.v8)) {
			return nil, fmt.Errorf("short file")
		}
		for i := range g.v9 {
			g.v9[i] = math.Float32frombits(u32(p + 4*i))
		}
		p += 4 * len(g.v9)
		for i := range g.v8 {
			g.v8[i] = math.Float32frombits(u32(p + 4*i))
		}
	}
	return g, nil
}

// Height returns the terrain height at WoW (x, y); ok is false without data.
func (m *Maps) Height(x, y float32) (float32, bool) {
	fx := float64(32 - float64(x)/gridSize)
	fy := float64(32 - float64(y)/gridSize)
	gx, gy := int(math.Floor(fx)), int(math.Floor(fy))
	if gx < 0 || gy < 0 || gx > 63 || gy > 63 {
		return 0, false
	}
	g := m.load(gx, gy)
	if g == nil || g.noH {
		return 0, false
	}
	cx, cy := fx*res, fy*res
	xi, yi := int(cx), int(cy)
	fxr, fyr := float32(cx-float64(xi)), float32(cy-float64(yi))
	xi &= res - 1
	yi &= res - 1
	h5 := 2 * g.v8[xi*128+yi]
	var a, b, c float32
	if fxr+fyr < 1 {
		if fxr > fyr {
			h1, h2 := g.v9[xi*129+yi], g.v9[(xi+1)*129+yi]
			a, b, c = h2-h1, h5-h1-h2, h1
		} else {
			h1, h3 := g.v9[xi*129+yi], g.v9[xi*129+yi+1]
			a, b, c = h5-h1-h3, h3-h1, h1
		}
	} else {
		if fxr > fyr {
			h2, h4 := g.v9[(xi+1)*129+yi], g.v9[(xi+1)*129+yi+1]
			a, b, c = h2+h4-h5, h4-h2, h5-h4
		} else {
			h3, h4 := g.v9[xi*129+yi+1], g.v9[(xi+1)*129+yi+1]
			a, b, c = h4-h3, h3+h4-h5, h5-h4
		}
	}
	return a*fxr + b*fyr + c, true
}

// Area returns the AreaTable id at WoW (x, y) (GridMap::getArea), 0 without data.
func (m *Maps) Area(x, y float32) uint32 {
	fx := float64(32 - float64(x)/gridSize)
	fy := float64(32 - float64(y)/gridSize)
	gx, gy := int(math.Floor(fx)), int(math.Floor(fy))
	if gx < 0 || gy < 0 || gx > 63 || gy > 63 {
		return 0
	}
	g := m.load(gx, gy)
	if g == nil {
		return 0
	}
	if g.area == nil {
		return uint32(g.areaAll)
	}
	lx, ly := int(fx*16)&15, int(fy*16)&15
	return uint32(g.area[lx*16+ly])
}
