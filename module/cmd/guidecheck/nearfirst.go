package main

import "math"

// Quests near the character first (2026-10-25): the route order alone sent the bot back and forth
// between Elwynn and Dun Morogh (a quest retried "in 3 min" in Elwynn pulled it back through the tram
// just after it arrived in Dun Morogh, then the tram again: 15-20 min lost in the bench 10 -> 12). A
// quest whose next NPC (its giver, or its ender once taken) is farther than nearQuest yards, or on
// another map, waits until nothing near is left (nextBatch).
const nearQuest = 2500

type place struct {
	m    uint32
	x, y float64
}

// questPlaces lists where a quest is given (end false) or turned in (end true): creature and object
// spawns (cached).
func (c *checker) questPlaces(id uint32, end bool) []place {
	k := [2]uint32{id, 0}
	if end {
		k[1] = 1
	}
	if p, ok := c.places[k]; ok {
		return p
	}
	if c.places == nil {
		c.places = map[[2]uint32][]place{}
	}
	kind := "queststarter"
	if end {
		kind = "questender"
	}
	var out []place
	for _, q := range []string{
		"SELECT c.map, c.position_x, c.position_y FROM creature_" + kind + " s JOIN creature c ON c.id=s.id WHERE s.quest=? LIMIT 20",
		"SELECT g.map, g.position_x, g.position_y FROM gameobject_" + kind + " s JOIN gameobject g ON g.id=s.id WHERE s.quest=? LIMIT 20",
	} {
		rows, err := c.db.Query(q, id)
		if err != nil {
			continue
		}
		for rows.Next() {
			var p place
			if rows.Scan(&p.m, &p.x, &p.y) == nil {
				out = append(out, p)
			}
		}
		rows.Close()
	}
	c.places[k] = out
	return out
}

// farFunc tells whether a quest's next NPC is far from (m, x, y); quests in the log (taken) are judged
// by their ender. A quest without a known NPC is near.
func (c *checker) farFunc(m uint32, x, y float64, taken map[uint32]bool) func(uint32) bool {
	return func(id uint32) bool {
		ps := c.questPlaces(id, taken[id])
		if len(ps) == 0 {
			return false
		}
		for _, p := range ps {
			if p.m == m && math.Hypot(p.x-x, p.y-y) <= nearQuest {
				return false
			}
		}
		return true
	}
}
