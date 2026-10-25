// Package roads tells where the roads and paths are: the client paints them on the terrain as texture
// layers (ADT MTEX/MCLY/MCAL). cmd/roads reads them from the client archives once and writes one grid per
// map (assets/roads); move and travel prefer walking on them (fewer monsters, like a player).
package roads

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	gridSize = 533.33333
	// Res cells per grid side (4.2 yards each).
	Res = 128
)

// Map is the road grid of one map: per grid (gx, gy as in the .map files: 32 - x/533, 32 - y/533) a
// Res × Res bitset indexed [lx*Res+ly] like terrain.
type Map struct {
	Grids map[[2]int][]byte // Res*Res/8 bytes
}

func New() *Map { return &Map{Grids: map[[2]int][]byte{}} }

// Set marks the cell at grid (gx, gy), cell (lx, ly) as road.
func (m *Map) Set(gx, gy, lx, ly int) {
	k := [2]int{gx, gy}
	g := m.Grids[k]
	if g == nil {
		g = make([]byte, Res*Res/8)
		m.Grids[k] = g
	}
	i := lx*Res + ly
	g[i/8] |= 1 << (i % 8)
}

// On tells whether WoW (x, y) is on a road.
func (m *Map) On(x, y float32) bool {
	if m == nil {
		return false
	}
	fx := 32 - float64(x)/gridSize
	fy := 32 - float64(y)/gridSize
	gx, gy := int(math.Floor(fx)), int(math.Floor(fy))
	g := m.Grids[[2]int{gx, gy}]
	if g == nil {
		return false
	}
	lx, ly := int((fx-float64(gx))*Res), int((fy-float64(gy))*Res)
	if lx < 0 || ly < 0 || lx >= Res || ly >= Res {
		return false
	}
	i := lx*Res + ly
	return g[i/8]&(1<<(i%8)) != 0
}

// File is <dir>/<map>.roads (gzip: "ROAD", grid count, then gx, gy (u8) and the bitset per grid).
func File(dir string, mapID uint32) string {
	return filepath.Join(dir, fmt.Sprintf("%03d.roads", mapID))
}

func (m *Map) Write(path string) error {
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	z.Write([]byte("ROAD"))
	binary.Write(z, binary.LittleEndian, uint32(len(m.Grids)))
	for k, g := range m.Grids {
		z.Write([]byte{byte(k[0]), byte(k[1])})
		z.Write(g)
	}
	if err := z.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Load reads a road grid; a missing file gives an empty map (no road anywhere).
func Load(dir string, mapID uint32) (*Map, error) {
	f, err := os.Open(File(dir, mapID))
	if err != nil {
		return New(), err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return New(), err
	}
	b, err := io.ReadAll(z)
	if err != nil || len(b) < 8 || string(b[:4]) != "ROAD" {
		return New(), fmt.Errorf("bad road file %s", File(dir, mapID))
	}
	n := int(binary.LittleEndian.Uint32(b[4:]))
	m := New()
	o := 8
	for i := 0; i < n && o+2+Res*Res/8 <= len(b); i++ {
		m.Grids[[2]int{int(b[o]), int(b[o+1])}] = b[o+2 : o+2+Res*Res/8]
		o += 2 + Res*Res/8
	}
	return m, nil
}

// IsRoad tells whether a terrain texture is a road or a path (by its file name).
func IsRoad(tex string) bool {
	t := strings.ToLower(tex)
	for _, w := range roadWords {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

// roadWords: texture names of roads and paths (cmd/roads -list shows them all with their coverage).
var roadWords = []string{"road", "path", "cobble"}
