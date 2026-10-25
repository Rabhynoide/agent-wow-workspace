package dbc

import (
	"encoding/binary"
	"math"
	"path/filepath"
	"sort"
)

// F32 reads a float field.
func (f *File) F32(rec, field int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(f.data[rec*f.RecordSize+4*field:]))
}

// Area is an AreaTable.dbc row: its map, parent zone (0 for a zone), exploration level and name.
type Area struct {
	ID, Map, Zone uint32
	Level         int32
	Name          string
}

// TaxiNode is a TaxiNodes.dbc row; mounts (creature entries) are 0 when the faction cannot use it.
type TaxiNode struct {
	ID, Map                   uint32
	X, Y, Z                   float32
	Name                      string
	HordeMount, AllianceMount uint32
}

// TaxiPath is a TaxiPath.dbc row with the flight length from TaxiPathNode.dbc (yards, same map
// legs only).
type TaxiPath struct {
	ID, From, To, Price uint32
	Length              float32
}

// AreaTrigger is an AreaTrigger.dbc row: a sphere (Radius > 0) or a box around (X, Y, Z).
type AreaTrigger struct {
	ID, Map                uint32
	X, Y, Z, Radius        float32
	BoxX, BoxY, BoxZ, BoxO float32
}

// PathNode is a TaxiPathNode.dbc row (transports follow these too): Flags 1 = teleport, 2 = stop for
// Delay seconds.
type PathNode struct {
	Index, Map   uint32
	X, Y, Z      float32
	Flags, Delay uint32
}

// Travel holds the client tables the travel planner reads.
type Travel struct {
	Areas     map[uint32]*Area
	Nodes     map[uint32]*TaxiNode
	Paths     []*TaxiPath
	Triggers  map[uint32]*AreaTrigger
	PathNodes map[uint32][]PathNode // by path id, in index order
}

// LoadTravel reads AreaTable, TaxiNodes, TaxiPath, TaxiPathNode and AreaTrigger from dir.
func LoadTravel(dir string) (*Travel, error) {
	t := &Travel{Areas: map[uint32]*Area{}, Nodes: map[uint32]*TaxiNode{}, Triggers: map[uint32]*AreaTrigger{}, PathNodes: map[uint32][]PathNode{}}
	f, err := Open(filepath.Join(dir, "AreaTable.dbc"))
	if err != nil {
		return nil, err
	}
	for r := 0; r < f.Records; r++ {
		a := &Area{ID: f.U32(r, 0), Map: f.U32(r, 1), Zone: f.U32(r, 2), Level: int32(f.U32(r, 10)), Name: f.Str(r, 11)}
		t.Areas[a.ID] = a
	}
	if f, err = Open(filepath.Join(dir, "TaxiNodes.dbc")); err != nil {
		return nil, err
	}
	for r := 0; r < f.Records; r++ {
		n := &TaxiNode{ID: f.U32(r, 0), Map: f.U32(r, 1), X: f.F32(r, 2), Y: f.F32(r, 3), Z: f.F32(r, 4), Name: f.Str(r, 5),
			HordeMount: f.U32(r, 22), AllianceMount: f.U32(r, 23)}
		t.Nodes[n.ID] = n
	}
	if f, err = Open(filepath.Join(dir, "TaxiPath.dbc")); err != nil {
		return nil, err
	}
	byID := map[uint32]*TaxiPath{}
	for r := 0; r < f.Records; r++ {
		p := &TaxiPath{ID: f.U32(r, 0), From: f.U32(r, 1), To: f.U32(r, 2), Price: f.U32(r, 3)}
		t.Paths = append(t.Paths, p)
		byID[p.ID] = p
	}
	if f, err = Open(filepath.Join(dir, "TaxiPathNode.dbc")); err != nil {
		return nil, err
	}
	type pt struct {
		idx, m  uint32
		x, y, z float32
	}
	last := map[uint32]pt{}
	for r := 0; r < f.Records; r++ {
		p := pt{idx: f.U32(r, 2), m: f.U32(r, 3), x: f.F32(r, 4), y: f.F32(r, 5), z: f.F32(r, 6)}
		path := f.U32(r, 1)
		t.PathNodes[path] = append(t.PathNodes[path], PathNode{Index: p.idx, Map: p.m, X: p.x, Y: p.y, Z: p.z, Flags: f.U32(r, 7), Delay: f.U32(r, 8)})
		if q, ok := last[path]; ok && q.m == p.m && p.idx == q.idx+1 {
			if tp := byID[path]; tp != nil {
				tp.Length += float32(math.Sqrt(float64((p.x-q.x)*(p.x-q.x) + (p.y-q.y)*(p.y-q.y) + (p.z-q.z)*(p.z-q.z))))
			}
		}
		last[path] = p
	}
	for id, ns := range t.PathNodes {
		sort.Slice(ns, func(i, j int) bool { return ns[i].Index < ns[j].Index })
		t.PathNodes[id] = ns
	}
	if f, err = Open(filepath.Join(dir, "AreaTrigger.dbc")); err != nil {
		return nil, err
	}
	for r := 0; r < f.Records; r++ {
		a := &AreaTrigger{ID: f.U32(r, 0), Map: f.U32(r, 1), X: f.F32(r, 2), Y: f.F32(r, 3), Z: f.F32(r, 4), Radius: f.F32(r, 5),
			BoxX: f.F32(r, 6), BoxY: f.F32(r, 7), BoxZ: f.F32(r, 8), BoxO: f.F32(r, 9)}
		t.Triggers[a.ID] = a
	}
	return t, nil
}
