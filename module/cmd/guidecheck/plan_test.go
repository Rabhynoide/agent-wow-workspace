package main

import "testing"

func TestPlanRoute(t *testing.T) {
	const human, gnome, priest, warrior = 1, 7, 5, 1
	cache := map[uint32]*quest{
		1:  {ID: 1, MinLevel: 1, Level: 2},
		2:  {ID: 2, MinLevel: 7, Level: 8},                  // level 7 needed
		3:  {ID: 3, MinLevel: 3, Level: 5, Prev: 99},        // prerequisite never done
		4:  {ID: 4, MinLevel: 3, Level: 5, Prev: 1},         // prerequisite planned before it
		5:  {ID: 5, MinLevel: 1, Level: 4, Classes: 1 << 4}, // priest quest without "only"
		6:  {ID: 6, MinLevel: 1, Level: 4},                  // "only Gnome"
		7:  {ID: 7, MinLevel: 1, Level: 4, Classes: 1},      // warrior quest
		8:  {ID: 8, MinLevel: 5, Level: 9},                  // too far above level 6 (danger)
		9:  {ID: 9, MinLevel: 1, Level: 3},                  // manual
		10: {ID: 10, MinLevel: 1, Level: 3},                 // already rewarded
		11: {ID: 11, MinLevel: 1, Level: 3},                 // "only Priest" (lone class word)
	}
	entries := []routeQuest{{id: 1}, {id: 2}, {id: 3}, {id: 4}, {id: 5}, {id: 6, only: []only{{Race: "Gnome"}}},
		{id: 7, only: []only{{Race: "Human", Class: "Warrior"}}}, {id: 8}, {id: 9}, {id: 10}, {id: 11, only: []only{{Race: "Priest"}}}}
	manual := map[uint32]string{9: "escort quest"}
	rewarded := map[uint32]bool{10: true}
	got := planRoute(entries, cache, manual, rewarded, human, priest, "Human", "Priest", 6, 2, 20)
	want := []uint32{1, 4, 5, 11}
	if len(got) != len(want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("plan = %v, want %v", got, want)
		}
	}
	if !onlyMatches([]only{{Race: "Gnome"}}, gnome, warrior) || onlyMatches([]only{{Race: "Gnome"}}, human, priest) {
		t.Fatal("a lone race must only match that race")
	}
}
