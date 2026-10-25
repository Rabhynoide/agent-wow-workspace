package move

import (
	"os"

	pb "wowagent/gen/movev1"
	"wowagent/internal/nav"
	"wowagent/internal/terrain"
	"wowagent/internal/vmap"
)

// Planner plans walks without a session, exactly as move.path does (offline tools: travelgraph).
// One per goroutine: meshes are not shared.
type Planner struct{ s *Service }

func NewPlanner(mmaps, maps, vmaps string) *Planner {
	os.Setenv("VMAPS_DIR", vmaps)
	return &Planner{&Service{dir: mmaps, maps: maps, meshes: map[uint32]*nav.Mesh{}, terr: map[uint32]*terrain.Maps{}, vmaps: map[uint32]*vmap.Tree{}}}
}

// Path is move.path from a point: the points and the length; reach > 0 ends at the closest walkable
// point within reach of an unreachable destination.
func (p *Planner) Path(mapID uint32, from, to [3]float32, reach float32) ([][3]float32, float32, error) {
	return p.s.planReach(mapID, from, to[0], to[1], to[2], nil, reach)
}

// Areas gives the areas a path crosses (move.path with_areas).
func (p *Planner) Areas(mapID uint32, pts [][3]float32) []*pb.AreaSpan {
	return areaSpans(p.s.terrainFor(mapID), pts)
}

func (p *Planner) Mesh(mapID uint32) (*nav.Mesh, error) { return p.s.mesh(mapID) }

func (p *Planner) Terrain(mapID uint32) *terrain.Maps { return p.s.terrainFor(mapID) }

// Snap finds the walkable point closest to pt (vert yards up or down, reach yards around), on a
// piece of navmesh of at least islandPolys polygons.
func (p *Planner) Snap(mapID uint32, pt [3]float32, vert, reach float32) ([3]float32, bool) {
	m, err := p.s.mesh(mapID)
	if err != nil {
		return pt, false
	}
	_, q, ok := m.NearestPoly(nav.ToRecast(pt[0], pt[1], pt[2]), nav.GroundOnly, vert, reach)
	if !ok || m.IslandSize(q, nav.GroundOnly, islandPolys) < islandPolys {
		return pt, false
	}
	x, y, z := q.ToWoW()
	return [3]float32{x, y, z}, true
}
