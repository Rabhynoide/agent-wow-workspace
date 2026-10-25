package travel

import (
	"encoding/json"
	"os"
	"strings"

	"wowagent/internal/dbc"
)

// The travel graph is computed offline (cmd/travelgraph) from the navmesh, the area grid and the
// world database, like the TravelNodeMap of mod-playerbots: planning then only checks the walks from
// the start to the graph and from the graph to the destination (checking every long walk in game
// took more than the 30 s of a module call, 2026-10-25).

// Node kinds.
const (
	KindGrid   = "grid"   // a walkable point near the centre of a navmesh tile
	KindFlight = "flight" // a flight master (Taxi = node)
	KindEnter  = "enter"  // a walkable point inside a teleporting area trigger
	KindExit   = "exit"   // where such a trigger leads
	KindPier   = "pier"   // where to wait for a boat (Transport, Dock = dock index + 1)
	KindInn    = "inn"    // an innkeeper (Npc)
)

type GNode struct {
	Kind      string  `json:"k"`
	Map       uint32  `json:"m"`
	X         float32 `json:"x"`
	Y         float32 `json:"y"`
	Z         float32 `json:"z"`
	Name      string  `json:"n,omitempty"`
	Taxi      uint32  `json:"taxi,omitempty"`
	Npc       uint32  `json:"npc,omitempty"`
	Trigger   uint32  `json:"trig,omitempty"`
	Transport uint32  `json:"tr,omitempty"`
	Dock      int     `json:"dock,omitempty"`
}

func (n GNode) At() Point { return Point{n.Map, n.X, n.Y, n.Z} }

// questNode: TaxiNodes used only by quest flights ("Quest - Wintergarde -> Stars' Rest (Start)") sit
// on ordinary flight masters (Stormwind's) and cannot be learned: the bot went to "learn" them each time
// it passed by (2026-10-25).
func questNode(name string) bool { return strings.HasPrefix(name, "Quest") }

// GLink is a walk between two nodes (both ways): navmesh length and its danger, the level of the
// hostile monsters along it (builder.pathDanger).
type GLink struct {
	A   int     `json:"a"`
	B   int     `json:"b"`
	Len float32 `json:"l"`
	Lv  int32   `json:"lv"`
}

// GPortal: walking into the trigger at node Enter leads to node Exit.
type GPortal struct {
	Trigger uint32 `json:"trig"`
	Name    string `json:"n"`
	Enter   int    `json:"in"`
	Exit    int    `json:"out"`
}

type TravelGraph struct {
	Version int       `json:"version"`
	Built   string    `json:"built"`
	Maps    []uint32  `json:"maps"`
	Nodes   []GNode   `json:"nodes"`
	Links   []GLink   `json:"links"`
	Portals []GPortal `json:"portals"`
	// Areas: median level of the hostile non-elite monsters (for a human) by area and by zone.
	Areas map[uint32]int32 `json:"areas"`
}

const GraphVersion = 3

func LoadGraph(path string) (*TravelGraph, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g TravelGraph
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// MobLevel is the typical level of the hostile monsters of an area (its own when it has enough
// spawns, else its zone's; 0 when none): the danger of walking through it. AreaTable exploration
// levels were 5-8 above the monsters (Duskwood 24-30) and closed zones the guide sends us to.
func (g *TravelGraph) MobLevel(tab *dbc.Travel, area uint32) int32 {
	if lv, ok := g.Areas[area]; ok {
		return lv
	}
	if a := tab.Areas[area]; a != nil && a.Zone != 0 {
		return g.Areas[a.Zone]
	}
	return 0
}
