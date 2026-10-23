package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestQuestLearnAndHard(t *testing.T) {
	log := `2026-10-23 09:04:24 [0s] L6 quests: 62 The Fargodeep Mine | 60 Kobold Candles
2026-10-23 09:08:00 [4m] 62 is not a done line
2026-10-23 09:09:00 [5m]   62 postponed to level 7: quest 62: too dangerous for now: path guarded
2026-10-23 09:10:00 [6m]   done [60]
2026-10-23 09:11:00 [7m]   60 Kobold Candles postponed 4 min (monsters dead or away, try 1/3)
2026-10-23 09:12:00 [8m]   DEATH on 47 (1 in this run), postponed to level 7: dead
2026-10-23 09:20:00 [16m] L7 quests: 62 The Fargodeep Mine
2026-10-23 09:25:00 [21m]   done [62 47]
`
	q := &questTable{Cells: map[string]*questCell{}}
	last, n := q.learn(strings.NewReader(log))
	if n != 5 || last != "2026-10-23 09:25:00" {
		t.Fatalf("learned %d until %s, want 5 until 09:25:00", n, last)
	}
	want := map[string]questCell{"62:6": {1, 1}, "60:6": {1, 0}, "47:6": {1, 1}, "62:7": {1, 0}, "47:7": {1, 0}}
	for k, w := range want {
		if c := q.Cells[k]; c == nil || *c != w {
			t.Errorf("cell %s = %v, want %v", k, c, w)
		}
	}
	// Lines up to Until are not learned again.
	q.Until = last
	if _, n := q.learn(strings.NewReader(log)); n != 0 {
		t.Fatalf("learned %d again", n)
	}
	for i := 0; i < 5; i++ {
		q.add(62, 6, true)
	}
	if h := q.hard(6, 0.6, 4); !reflect.DeepEqual(h, map[uint32]bool{62: true}) {
		t.Fatalf("hard at 6 = %v, want 62 (6 failures in 6 tries)", h)
	}
	if h := q.hard(6, 0, 4); len(h) != 0 {
		t.Fatalf("rate 0 must turn it off: %v", h)
	}
	for i := 0; i < 50; i++ {
		q.add(60, 6, false)
	}
	if c := q.Cells["60:6"]; c.N != questKeep {
		t.Fatalf("cell kept %v tries, want %d", c.N, questKeep)
	}
}

func TestNextBatchPassesOverHardQuests(t *testing.T) {
	cache := map[uint32]*quest{
		62: {ID: 62, MinLevel: 1, Level: 6},
		60: {ID: 60, MinLevel: 1, Level: 6},
		76: {ID: 76, MinLevel: 1, Level: 10, Prev: 62}, // follows the hard quest
		85: {ID: 85, MinLevel: 1, Level: 6},
		99: {ID: 99, MinLevel: 9, Level: 9},
	}
	entries := []routeQuest{{id: 62}, {id: 60}, {id: 76}, {id: 85}, {id: 99}}
	ps := &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{},
		waitUntil: map[uint32]time.Time{}, hard: map[uint32]bool{62: true}}
	if batch, need, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 6, 2, 5); !reflect.DeepEqual(batch, []uint32{60, 85}) || need != 0 {
		t.Fatalf("batch %v need %d, want [60 85]: 62 and its follower 76 passed over", batch, need)
	}
	ps.finished[60], ps.finished[85] = true, true
	if batch, need, _ := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 6, 2, 5); !reflect.DeepEqual(batch, []uint32{62}) || need != 0 {
		t.Fatalf("batch %v need %d, want [62]: nothing else left, tried before a grind", batch, need)
	}
	// Failed again: postponed to the next level like any quest, then the grind.
	ps.retryAt[62] = 7
	if batch, need, blocker := nextBatch(entries, cache, nil, ps, 1, 1, "human", "warrior", 6, 2, 5); batch != nil || need != 7 || blocker != 62 {
		t.Fatalf("batch %v need %d blocker %d, want grind to 7 for 62", batch, need, blocker)
	}
}
