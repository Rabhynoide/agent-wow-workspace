package travel

import (
	"testing"

	"wowagent/internal/dbc"
)

// The Bravery (Stormwind Harbor - Auberdine): two docks on two maps, a period of a few minutes.
func TestBraveryDocks(t *testing.T) {
	tab, err := dbc.LoadTravel("../../../assets/dbc")
	if err != nil {
		t.Skip(err)
	}
	kf, period := keyFrames(tab.PathNodes[967], 30, 1)
	ds := docks(kf)
	if len(ds) != 2 || period < 120000 || period > 900000 || ds[0].Map == ds[1].Map {
		t.Fatalf("period %d docks %+v", period, ds)
	}
	for _, d := range ds {
		t.Logf("dock map %d (%.0f,%.0f,%.0f) %d-%d ms", d.Map, d.X, d.Y, d.Z, d.Arrive, d.Depart)
	}
	t.Logf("period %d ms, ride %d s", period, rideTime(ds[0], ds[1], period)/1000)
	if w := waitFor(ds[0], ds[0].Arrive+1000, period, 5000); w != 0 {
		t.Fatalf("docked: wait %d", w)
	}
	if w := waitFor(ds[0], ds[0].Depart-1000, period, 5000); w == 0 || w > period {
		t.Fatalf("leaving: wait %d", w)
	}
}

// Periods computed against the ones the server sent (GAMEOBJECT_LEVEL, 2026-10-25).
func TestPeriodsMatchServer(t *testing.T) {
	tab, err := dbc.LoadTravel("../../../assets/dbc")
	if err != nil {
		t.Skip(err)
	}
	for path, want := range map[uint32]uint32{285: 248842, 241: 231110, 301: 314929, 302: 239262, 737: 214543} {
		_, got := keyFrames(tab.PathNodes[path], 30, 1)
		d := int64(got) - int64(want)
		if d < -50 || d > 50 {
			t.Errorf("path %d: period %d, server %d", path, got, want)
		}
	}
}
