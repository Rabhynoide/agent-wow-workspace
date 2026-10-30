package main

import (
	"testing"
	"time"
)

// The Fargodeep batch of the Zygor route: 61 (Shipment to Stormwind) is turned in at step 148, far
// past the batch: held; 47 and 60 are turned in within it. Later, a batch reaching step 148 makes it
// due.
func TestHolds(t *testing.T) {
	entries := []routeQuest{{id: 62, at: 62}, {id: 60, at: 63}, {id: 47, at: 66}, {id: 61, at: 76}, {id: 85, at: 77},
		{id: 1097, at: 138}, {id: 353, at: 150}}
	turnAt := map[uint32]int{62: 73, 60: 75, 47: 71, 61: 148, 85: 78, 1097: 149, 353: 193}
	lvl := func(uint32) int { return 7 }
	all := func(uint32) bool { return true }
	now := time.Now()
	hold, due := holdsFor(entries, turnAt, []uint32{62, 60, 47, 61}, map[uint32]time.Time{}, lvl, 6, now, all)
	if len(hold) != 1 || hold[0] != 61 || len(due) != 0 {
		t.Fatalf("hold %v due %v", hold, due)
	}
	held := map[uint32]time.Time{61: now}
	if _, due := holdsFor(entries, turnAt, []uint32{85}, held, lvl, 7, now, all); len(due) != 0 {
		t.Fatalf("not due yet: %v", due)
	}
	if _, due := holdsFor(entries, turnAt, []uint32{1097}, held, lvl, 9, now, all); len(due) != 1 {
		t.Fatalf("due at step 150: %v", due)
	}
	if _, due := holdsFor(entries, turnAt, []uint32{85}, held, lvl, 10, now, all); len(due) != 1 {
		t.Fatalf("due 3 levels over the quest: %v", due)
	}
}

// Only far turn-ins are held: 61 near is turned in with its batch.
func TestHoldsFarOnly(t *testing.T) {
	entries := []routeQuest{{id: 62, at: 62}, {id: 61, at: 76}, {id: 85, at: 77}}
	turnAt := map[uint32]int{62: 73, 61: 148, 85: 78}
	near := func(uint32) bool { return false }
	if hold, _ := holdsFor(entries, turnAt, []uint32{62, 61}, map[uint32]time.Time{}, func(uint32) int { return 7 }, 6, time.Now(), near); len(hold) != 0 {
		t.Fatalf("near turn-in held: %v", hold)
	}
}
