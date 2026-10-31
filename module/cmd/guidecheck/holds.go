package main

import "time"

// Turn-ins where the guide puts them. auto does a quest's objectives where the batch takes it, but
// a quest the guide turns in much later (Shipment to Stormwind, taken at Goldshire at level 6 and
// handed in at level 10 on the way through Stormwind: every run of the 6 → 8 series walked there and
// back, ~4 min) is kept in the log until the route reaches its turn-in (RunRequest.hold).

// Held quests are turned in anyway this many levels over their quest level, or after holdMax.
const (
	// holdFar: only turn-ins farther than this from us are held (yards, or another map): holding every
	// later turn-in made the route walk more (hold_turnins, 2026-10-28); the one that costs is
	// Shipment to Stormwind, a 4.5 min round trip per run for 790 XP (walking analysis, 2026-10-31).
	holdFar    = 800
	holdLevels = 3
	holdMax    = 150 * time.Minute // Shipment to Stormwind waits from level 6 to the level-10 visit (45 min released it at level 8)
	holdAtMost = 8 // quests kept at once (the log holds 25)
)

// horizon is the route position up to which the batch takes us: the position of the first entry
// after the batch's last quest (the steps until then are the batch's part of the guide).
func horizon(entries []routeQuest, batch []uint32) int {
	in := map[uint32]bool{}
	for _, id := range batch {
		in[id] = true
	}
	last := -1
	for i, e := range entries {
		if e.gate == 0 && in[e.id] && e.at > 0 {
			last = i
		}
	}
	if last < 0 {
		return -1 // no position known (inserted class quests): nothing held
	}
	for i := last + 1; i < len(entries); i++ {
		if entries[i].at > entries[last].at {
			return entries[i].at
		}
	}
	return int(^uint(0) >> 1) // end of the route
}

// holdsFor returns the quests of the batch to keep for a later turn-in (their turn-in far: far), and the held quests due
// now (their turn-in step reached, too low for us now, or held too long), to add to the batch.
func holdsFor(entries []routeQuest, turnAt map[uint32]int, batch []uint32, held map[uint32]time.Time,
	levelOf func(uint32) int, level uint32, now time.Time, far func(uint32) bool) (hold, due []uint32) {
	h := horizon(entries, batch)
	for id, since := range held {
		at, ok := turnAt[id]
		if !ok || (h >= 0 && at <= h) || int(level) >= levelOf(id)+holdLevels || now.Sub(since) > holdMax {
			due = append(due, id)
		}
	}
	if h < 0 {
		return nil, due
	}
	kept := len(held) - len(due)
	for _, id := range batch {
		if at, ok := turnAt[id]; ok && at > h && kept < holdAtMost && far(id) {
			hold = append(hold, id)
			kept++
		}
	}
	return hold, due
}
