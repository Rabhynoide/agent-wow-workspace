package main

import (
	"reflect"
	"testing"
	"time"
)

func TestNextBatchKeepsRouteOrder(t *testing.T) {
	const human, warrior = 1, 1
	cache := map[uint32]*quest{
		1: {ID: 1, MinLevel: 1, Level: 5},
		2: {ID: 2, MinLevel: 1, Level: 6},
		3: {ID: 3, MinLevel: 7, Level: 9}, // needs level 7: everything after waits
		4: {ID: 4, MinLevel: 1, Level: 5},
		5: {ID: 5, MinLevel: 1, Level: 5, Prev: 9}, // chain broken (9 skipped)
		9: {ID: 9, MinLevel: 1, Level: 5},
	}
	entries := []routeQuest{{id: 9}, {id: 1}, {id: 5}, {id: 2}, {id: 3}, {id: 4}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{9: "failed"}, retryAt: map[uint32]uint32{}, waitUntil: map[uint32]time.Time{}}
	batch, need, _ := nextBatch(entries, cache, nil, ps, human, warrior, "human", "warrior", 6, 2, 5)
	if !reflect.DeepEqual(batch, []uint32{1, 2}) || need != 0 {
		t.Fatalf("batch %v need %d, want [1 2] and 0 (stop before quest 3, keep the order)", batch, need)
	}
	ps.finished[1], ps.finished[2] = true, true
	batch, need, blocker := nextBatch(entries, cache, nil, ps, human, warrior, "human", "warrior", 6, 2, 5)
	if batch != nil || need != 7 || blocker != 3 {
		t.Fatalf("batch %v need %d blocker %d, want grind to 7 for quest 3", batch, need, blocker)
	}
	// A quest found too dangerous waits for the next level even when its minimum is reached.
	ps.retryAt[3] = 8
	if _, need, _ := nextBatch(entries, cache, nil, ps, human, warrior, "human", "warrior", 7, 2, 5); need != 8 {
		t.Fatalf("need %d, want 8 after a postponement", need)
	}
}

func TestLevelGateStopsTheRoute(t *testing.T) {
	cache := map[uint32]*quest{1: {ID: 1, MinLevel: 1, Level: 2}, 2: {ID: 2, MinLevel: 1, Level: 3}}
	entries := []routeQuest{{id: 1}, {gate: 5}, {id: 2}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{}, waitUntil: map[uint32]time.Time{}}
	if batch, need, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 4, 2, 5); len(batch) != 1 || need != 0 {
		t.Fatalf("batch %v need %d: quest 1, then stop at the gate", batch, need)
	}
	ps.finished[1] = true
	if batch, need, blocker := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 4, 2, 5); batch != nil || need != 5 || blocker != 0 {
		t.Fatalf("batch %v need %d blocker %d: grind to 5 here", batch, need, blocker)
	}
	if batch, _, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 5, 2, 5); len(batch) != 1 || batch[0] != 2 {
		t.Fatalf("at level 5 the route goes on: %v", batch)
	}
}

func TestPostponedQuestLetsTheNextOnesGo(t *testing.T) {
	cache := map[uint32]*quest{1: {ID: 1, MinLevel: 1, Level: 2}, 2: {ID: 2, MinLevel: 1, Level: 2}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{},
		waitUntil: map[uint32]time.Time{1: time.Now().Add(time.Minute)}}
	if batch, _, _ := nextBatch([]routeQuest{{id: 1}, {id: 2}}, cache, nil, ps, 1, 1, "human", "warrior", 3, 2, 5); len(batch) != 1 || batch[0] != 2 {
		t.Fatalf("batch %v: quest 1 waits, quest 2 goes", batch)
	}
	ps.waitUntil[1] = time.Now().Add(-time.Second)
	if batch, _, _ := nextBatch([]routeQuest{{id: 1}, {id: 2}}, cache, nil, ps, 1, 1, "human", "warrior", 3, 2, 5); len(batch) != 2 || batch[0] != 1 {
		t.Fatalf("batch %v: quest 1 is back first", batch)
	}
}

func TestGrayLevel(t *testing.T) {
	for l, want := range map[uint32]uint32{5: 0, 8: 3, 20: 13, 45: 35, 70: 61} {
		if g := grayLevel(l); g != want {
			t.Errorf("grayLevel(%d) = %d, want %d", l, g, want)
		}
	}
}

func TestJobDeaths(t *testing.T) {
	for reason, want := range map[string]int{"stopping: died 4 times": 4, "combat: dead": 1, "time limit": 0} {
		if got := jobDeaths(reason); got != want {
			t.Errorf("jobDeaths(%q) = %d, want %d", reason, got, want)
		}
	}
}
