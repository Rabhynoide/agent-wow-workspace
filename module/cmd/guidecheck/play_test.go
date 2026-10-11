package main

import (
	"reflect"
	"testing"
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
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{9: "failed"}, retryAt: map[uint32]uint32{}}
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

func TestGrayLevel(t *testing.T) {
	for l, want := range map[uint32]uint32{5: 0, 8: 3, 20: 13, 45: 35, 70: 61} {
		if g := grayLevel(l); g != want {
			t.Errorf("grayLevel(%d) = %d, want %d", l, g, want)
		}
	}
}
