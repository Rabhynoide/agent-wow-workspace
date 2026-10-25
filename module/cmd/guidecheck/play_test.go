package main

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestNextBatchKeepsRouteOrder(t *testing.T) {
	const human, warrior = 1, 1
	cache := map[uint32]*quest{
		1: {ID: 1, MinLevel: 1, Level: 5},
		2: {ID: 2, MinLevel: 1, Level: 6},
		3: {ID: 3, MinLevel: 7, Level: 9}, // needs level 7: passed over, the next ones go
		4: {ID: 4, MinLevel: 1, Level: 5},
		5: {ID: 5, MinLevel: 1, Level: 5, Prev: 9}, // chain broken (9 skipped)
		9: {ID: 9, MinLevel: 1, Level: 5},
	}
	entries := []routeQuest{{id: 9}, {id: 1}, {id: 5}, {id: 2}, {id: 3}, {id: 4}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{9: "failed"}, retryAt: map[uint32]uint32{}, waitUntil: map[uint32]time.Time{}}
	batch, need, _ := nextBatch(entries, cache, nil, ps, human, warrior, "human", "warrior", 6, 2, 5, nil)
	if !reflect.DeepEqual(batch, []uint32{1, 2, 4}) || need != 0 {
		t.Fatalf("batch %v need %d, want [1 2 4] and 0 (quest 3 too high passed over, order kept)", batch, need)
	}
	ps.finished[1], ps.finished[2], ps.finished[4] = true, true, true
	batch, need, blocker := nextBatch(entries, cache, nil, ps, human, warrior, "human", "warrior", 6, 2, 5, nil)
	if batch != nil || need != 7 || blocker != 3 {
		t.Fatalf("batch %v need %d blocker %d, want grind to 7 for quest 3", batch, need, blocker)
	}
	// A quest found too dangerous waits for the next level even when its minimum is reached; the
	// quests after it go first, and it is the grind's blocker once they are done.
	ps.retryAt[3] = 8
	ps.finished[4] = false
	if batch, need, _ := nextBatch(entries, cache, nil, ps, human, warrior, "human", "warrior", 7, 2, 5, nil); !reflect.DeepEqual(batch, []uint32{4}) || need != 0 {
		t.Fatalf("batch %v need %d, want [4]: postponed quest 3 passed over", batch, need)
	}
	ps.finished[4] = true
	if batch, need, blocker := nextBatch(entries, cache, nil, ps, human, warrior, "human", "warrior", 7, 2, 5, nil); batch != nil || need != 8 || blocker != 3 {
		t.Fatalf("batch %v need %d blocker %d, want grind to 8 for postponed quest 3", batch, need, blocker)
	}
}

// Quest colors: at level 7, a level-10 quest (orange, 3 over) is done now; a level-12 one (red) waits.
func TestOnlyRedQuestsWait(t *testing.T) {
	cache := map[uint32]*quest{76: {ID: 76, MinLevel: 4, Level: 10}, 99: {ID: 99, MinLevel: 4, Level: 12}, 61: {ID: 61, MinLevel: 3, Level: 7}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{}, waitUntil: map[uint32]time.Time{}}
	batch, need, _ := nextBatch([]routeQuest{{id: 99}, {id: 76}, {id: 61}}, cache, nil, ps, 1, 1, "human", "warrior", 7, redAbove, 5, nil)
	if !reflect.DeepEqual(batch, []uint32{76, 61}) || need != 0 {
		t.Fatalf("batch %v need %d, want [76 61]: the red one passed over", batch, need)
	}
}

// A quest waiting for its retry is tried now when nothing else is left (not "route finished").
func TestWaitingQuestWhenNothingElse(t *testing.T) {
	cache := map[uint32]*quest{62: {ID: 62, MinLevel: 4, Level: 7}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{},
		waitUntil: map[uint32]time.Time{62: time.Now().Add(3 * time.Minute)}}
	if batch, need, _ := nextBatch([]routeQuest{{id: 62}}, cache, nil, ps, 1, 1, "human", "warrior", 6, redAbove, 5, nil); !reflect.DeepEqual(batch, []uint32{62}) || need != 0 {
		t.Fatalf("batch %v need %d, want [62]", batch, need)
	}
}

func TestLevelGateStopsTheRoute(t *testing.T) {
	cache := map[uint32]*quest{1: {ID: 1, MinLevel: 1, Level: 2}, 2: {ID: 2, MinLevel: 1, Level: 3}}
	entries := []routeQuest{{id: 1}, {gate: 5}, {id: 2}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{}, waitUntil: map[uint32]time.Time{}}
	if batch, need, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 4, 2, 5, nil); len(batch) != 1 || need != 0 {
		t.Fatalf("batch %v need %d: quest 1, then stop at the gate", batch, need)
	}
	ps.finished[1] = true
	if batch, need, blocker := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 4, 2, 5, nil); batch != nil || need != 5 || blocker != 0 {
		t.Fatalf("batch %v need %d blocker %d: grind to 5 here", batch, need, blocker)
	}
	if batch, _, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 5, 2, 5, nil); len(batch) != 1 || batch[0] != 2 {
		t.Fatalf("at level 5 the route goes on: %v", batch)
	}
}

func TestPostponedQuestLetsTheNextOnesGo(t *testing.T) {
	cache := map[uint32]*quest{1: {ID: 1, MinLevel: 1, Level: 2}, 2: {ID: 2, MinLevel: 1, Level: 2}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{},
		waitUntil: map[uint32]time.Time{1: time.Now().Add(time.Minute)}}
	if batch, _, _ := nextBatch([]routeQuest{{id: 1}, {id: 2}}, cache, nil, ps, 1, 1, "human", "warrior", 3, 2, 5, nil); len(batch) != 1 || batch[0] != 2 {
		t.Fatalf("batch %v: quest 1 waits, quest 2 goes", batch)
	}
	ps.waitUntil[1] = time.Now().Add(-time.Second)
	if batch, _, _ := nextBatch([]routeQuest{{id: 1}, {id: 2}}, cache, nil, ps, 1, 1, "human", "warrior", 3, 2, 5, nil); len(batch) != 2 || batch[0] != 1 {
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

func TestMergeTargets(t *testing.T) {
	if got := mergeTargets([]uint32{822}, []uint32{525, 822, 30, 299, 113}, 4); !slices.Equal(got, []uint32{822, 525, 30, 299}) {
		t.Errorf("merge = %v", got)
	}
}

// Near quests first: a far one (Elwynn seen from Dun Morogh) waits until nothing near is left, even
// when it comes first in the route or is waiting for a retry.
func TestNextBatchNearFirst(t *testing.T) {
	cache := map[uint32]*quest{
		11:  {ID: 11, MinLevel: 1, Level: 10},
		353: {ID: 353, MinLevel: 1, Level: 11},
		433: {ID: 433, MinLevel: 1, Level: 11},
		432: {ID: 432, MinLevel: 1, Level: 11},
	}
	entries := []routeQuest{{id: 11}, {id: 353}, {id: 433}, {id: 432}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{},
		waitUntil: map[uint32]time.Time{353: time.Now().Add(time.Minute)}}
	far := func(id uint32) bool { return id == 11 }
	if batch, _, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 11, 2, 5, far); !reflect.DeepEqual(batch, []uint32{433, 432}) {
		t.Fatalf("batch %v, want the near quests [433 432]", batch)
	}
	ps.finished[433], ps.finished[432] = true, true
	// The near quest waiting for a retry goes before the far one.
	if batch, _, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 11, 2, 5, far); !reflect.DeepEqual(batch, []uint32{353}) {
		t.Fatalf("batch %v, want the near waiting quest [353]", batch)
	}
	ps.finished[353] = true
	if batch, _, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 11, 2, 5, far); !reflect.DeepEqual(batch, []uint32{11}) {
		t.Fatalf("batch %v, want the far quest [11] once nothing near is left", batch)
	}
}
