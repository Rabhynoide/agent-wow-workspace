package travel

import "testing"

// Elwynn to Loch Modan: the direct walk is refused (Burning Steppes), the tram portal is taken, then
// the flight Ironforge -> Thelsamar once both nodes are known.
func TestPlanPortalAndFlight(t *testing.T) {
	elwynn := Point{0, -9460, 60, 56}
	loch := Point{0, -5400, -2900, 330}
	swTrig := Point{0, -8346, 514, 96}
	tramSW := Point{369, 68, 2490, -4}
	tramIFTrig := Point{369, 69, 20, -4}
	ifSide := Point{0, -4838, -1318, 501}
	ifNode := Point{0, -4821, -1155, 502}
	thelsamar := Point{0, -5424, -2929, 347}
	walks := 0
	g := Graph{
		Portals: []Portal{
			{Trigger: 2173, Name: "tram SW", Enter: swTrig, Exit: tramSW},
			{Trigger: 2166, Name: "tram IF", Enter: tramIFTrig, Exit: ifSide},
		},
		Flight: func(a, b uint32) (float32, []uint32, bool) { return 0, nil, false },
		Walk: func(a, b Point) (float32, bool, error) {
			walks++
			// Long walks across the continent go through the Burning Steppes.
			if dist(a, b) > 3000 && a.Map == 0 {
				return 0, false, nil
			}
			return dist(a, b) * 1.1 / RunSpeed, true, nil
		},
	}
	legs, _, err := Plan(g, elwynn, loch)
	if err != nil {
		t.Fatal(err)
	}
	kinds := ""
	for _, l := range legs {
		kinds += l.Kind + " "
	}
	if kinds != "walk portal walk portal walk " {
		t.Fatalf("legs without flights: %s", kinds)
	}
	g.Nodes = []FlightNode{{ID: 6, Name: "Ironforge", At: ifNode, NpcEnt: 1573}, {ID: 8, Name: "Thelsamar", At: thelsamar, NpcEnt: 1572}}
	g.Flight = func(a, b uint32) (float32, []uint32, bool) {
		if a == 6 && b == 8 {
			return 60, []uint32{6, 8}, true
		}
		return 0, nil, false
	}
	legs, _, err = Plan(g, elwynn, loch)
	if err != nil {
		t.Fatal(err)
	}
	kinds = ""
	for _, l := range legs {
		kinds += l.Kind + " "
	}
	if kinds != "walk portal walk portal walk fly walk " {
		t.Fatalf("legs with the flight: %s", kinds)
	}
	if legs[5].Npc != 1573 || len(legs[5].Nodes) != 2 {
		t.Fatalf("fly leg: %+v", legs[5])
	}
}

func TestPlanHearth(t *testing.T) {
	far := Point{0, -10500, 1000, 40}
	home := Point{0, -9460, 60, 56}
	g := Graph{
		Flight: func(a, b uint32) (float32, []uint32, bool) { return 0, nil, false },
		Walk:   func(a, b Point) (float32, bool, error) { return dist(a, b) / RunSpeed, true, nil },
		Hearth: &home,
	}
	legs, _, err := Plan(g, far, Point{0, -9450, 70, 56})
	if err != nil || len(legs) != 2 || legs[0].Kind != "hearth" {
		t.Fatalf("hearth: %v %+v", err, legs)
	}
}
