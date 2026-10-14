package main

import (
	"path/filepath"
	"testing"
	"time"
)

func newPS() *playState {
	return &playState{finished: map[uint32]bool{}, skipped: map[uint32]string{}, retryAt: map[uint32]uint32{},
		waitUntil: map[uint32]time.Time{}, tries: map[uint32]int{}, skippedAt: map[uint32]time.Time{}}
}

// A restart keeps the postponements; an old skip is retried.
func TestPlanSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	ps := newPS()
	ps.retryAt[62] = 9
	ps.skipped[112] = "destination is not on the navmesh"
	ps.skipped[83] = "no walkable path"
	ps.skippedAt[83] = time.Now().Add(-7 * time.Hour)
	savePlan(path, ps)
	again := newPS()
	if n := loadPlan(path, again, time.Now()); n != 2 || again.retryAt[62] != 9 || again.skipped[112] == "" || again.skipped[83] != "" {
		t.Errorf("restored %d: retry %v skipped %v", n, again.retryAt, again.skipped)
	}
}
