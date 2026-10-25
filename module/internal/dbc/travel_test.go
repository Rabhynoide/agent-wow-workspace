package dbc

import "testing"

// Needs the files copied in assets/dbc (skipped without them).
func TestLoadTravel(t *testing.T) {
	tr, err := LoadTravel("../../../assets/dbc")
	if err != nil {
		t.Skip(err)
	}
	if n := tr.Nodes[2]; n == nil || n.Map != 0 || n.AllianceMount == 0 {
		t.Fatalf("Stormwind node: %+v", n)
	}
	if a := tr.Areas[12]; a == nil || a.Name != "Elwynn Forest" {
		t.Fatalf("Elwynn: %+v", a)
	}
	var sw2wf *TaxiPath
	for _, p := range tr.Paths {
		if p.From == 2 && p.To == 4 { // Stormwind -> Sentinel Hill
			sw2wf = p
		}
	}
	if sw2wf == nil || sw2wf.Length < 500 {
		t.Fatalf("Stormwind -> Sentinel Hill: %+v", sw2wf)
	}
	if tr.Triggers[2173] == nil || tr.Triggers[2173].Map != 0 {
		t.Fatalf("tram trigger: %+v", tr.Triggers[2173])
	}
	t.Logf("Burning Steppes level %d, Redridge %d, Loch Modan %d, Dun Morogh %d, SW->WF %.0f yards, tram trigger %+v",
		tr.Areas[46].Level, tr.Areas[44].Level, tr.Areas[38].Level, tr.Areas[1].Level, sw2wf.Length, *tr.Triggers[2173])
}
