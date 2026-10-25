package auto

import (
	"log/slog"
	"math"
	"time"

	questpb "wowagent/gen/questv1"
)

// Chests (user's request of 2026-10-23): a treasure chest in sight becomes the goal before the quest.
// The fight under way ends first, then the way to the chest is cleared (clearWalk fights the
// monsters guarding it), the chest is opened and the quest goes on.
const (
	// chestRadius: chests this close (yards) are seen; a detour of ~10 s on foot each way.
	chestRadius = 40
	// chestMaxDz: a chest on a ledge or a floor above or below is left alone.
	chestMaxDz = 10
	// chestRetry: a chest we went to (opened or not) is not tried again for this long.
	chestRetry = 15 * time.Minute
)

// chestInSight returns the nearest chest not tried lately, nil when none (or in a detour already).
func (r *run) chestInSight() *questpb.Chest {
	// Not on journeys: the walk to the Deeprun Tram stopped for crates in Stormwind.
	if r.detour || r.travelling || time.Since(r.lastChestCheck) < 2*time.Second {
		return nil
	}
	r.lastChestCheck = time.Now()
	cs, err := r.s.api.Chests(r.ctx, chestRadius)
	if err != nil || len(cs) == 0 {
		return nil
	}
	b, err := r.s.api.Brief(r.ctx)
	if err != nil || b.Self == nil {
		return nil
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, c := range cs {
		if t, ok := r.s.chestTried[c.Guid]; ok && time.Since(t) < chestRetry {
			continue
		}
		if math.Abs(float64(c.Z-b.Self.Z)) > chestMaxDz {
			continue
		}
		return c
	}
	return nil
}

// openChest finishes the fight under way, clears the way to the chest, opens it and takes the loot.
func (r *run) openChest(c *questpb.Chest) {
	r.detour = true
	defer func() { r.detour = false }()
	r.s.mu.Lock()
	r.s.chestTried[c.Guid] = time.Now()
	r.s.mu.Unlock()
	slog.Info("chest seen", "entry", c.Entry, "name", c.Name, "dist", c.Dist)
	r.step("chest %s %.0f yards away: opening it before the quest", c.Name, c.Dist)
	if r.inCombat() {
		if _, err := r.defend(); err != nil {
			return
		}
	}
	if err := r.clearWalk(c.X, c.Y, c.Z, 2.5); err != nil {
		slog.Info("chest skipped", "entry", c.Entry, "err", err)
		r.note("chest %s skipped: %v", c.Name, err)
		return
	}
	if r.inCombat() {
		if _, err := r.defend(); err != nil {
			return
		}
	}
	res, err := r.s.api.OpenChest(r.ctx, c)
	if err != nil || !res.Ok {
		slog.Info("chest not opened", "entry", c.Entry, "err", err, "reason", res.GetReason())
		r.note("chest %s not opened: %v %s", c.Name, err, res.GetReason())
		return
	}
	slog.Info("chest opened", "entry", c.Entry, "name", c.Name, "items", res.Items)
	r.note("opened %s: items %v", c.Name, res.Items)
	r.gearUp()
}
