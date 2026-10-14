package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// The postponed and skipped quests of a character outlive the run: a restart used to retry at once
// the quest that had just killed us or made us flee (quest 47 in the mine right after a death).
// Saved in logs/level_<name>.plan.json; a skip is kept skipKeep (a code change may fix it), a
// postponement until its level.

const skipKeep = 6 * time.Hour

type savedPlan struct {
	RetryAt map[uint32]uint32    `json:"retry_at"`
	Skipped map[uint32]savedSkip `json:"skipped"`
}

type savedSkip struct {
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

func planPath(char string) string { return filepath.Join("..", "logs", "level_"+char+".plan.json") }

// loadPlan fills the play state from the saved plan; it returns how many quests it restored.
func loadPlan(path string, ps *playState, now time.Time) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var sp savedPlan
	if json.Unmarshal(b, &sp) != nil {
		return 0
	}
	n := 0
	for q, l := range sp.RetryAt {
		ps.retryAt[q] = l
		n++
	}
	for q, s := range sp.Skipped {
		if now.Sub(s.At) < skipKeep {
			ps.skipped[q] = s.Reason
			ps.skippedAt[q] = s.At
			n++
		}
	}
	return n
}

// savePlan writes the postponed and skipped quests.
func savePlan(path string, ps *playState) {
	sp := savedPlan{RetryAt: ps.retryAt, Skipped: map[uint32]savedSkip{}}
	for q, r := range ps.skipped {
		at := ps.skippedAt[q]
		if at.IsZero() {
			at = time.Now()
			ps.skippedAt[q] = at
		}
		sp.Skipped[q] = savedSkip{Reason: r, At: at}
	}
	if b, err := json.MarshalIndent(sp, "", " "); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
}
