package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Class quests are not in the Zygor route: they come from journal/classes/<class>.yaml
// (class_quests: level, quest per race group), followed by their chain in the world database, and are
// inserted in the route where it reaches their level. The steps auto cannot do alone (Beat Bartleby:
// a duel) are classified manual like any route quest, and the chain stops there.

var (
	classQuestLine = regexp.MustCompile(`level:\s*(\d+).*quests:\s*\{(.*)\}`)
	classQuestRace = regexp.MustCompile(`(\w+):\s*"[^"]*\((\d+)\)"`)
)

// classQuestStarts reads the first quest of each class quest for a race: level -> quest id.
func classQuestStarts(yaml, raceName string) map[uint32]uint32 {
	out := map[uint32]uint32{}
	race := strings.ReplaceAll(strings.ToLower(raceName), " ", "")
	for _, line := range strings.Split(yaml, "\n") {
		m := classQuestLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lvl, _ := strconv.Atoi(m[1])
		for _, q := range classQuestRace.FindAllStringSubmatch(m[2], -1) {
			key := strings.ToLower(q[1])
			match := key == "all"
			for _, part := range strings.Split(key, "_") {
				match = match || part == race
			}
			if match {
				id, _ := strconv.Atoi(q[2])
				out[uint32(lvl)] = uint32(id)
				break
			}
		}
	}
	return out
}

// chain follows a quest's successors for this class (quest_template_addon.PrevQuestID).
func (c *checker) chain(first, classBit uint32) []uint32 {
	out := []uint32{first}
	for cur := first; len(out) < 10; {
		var next uint32
		err := c.db.QueryRow(`SELECT ID FROM quest_template_addon WHERE PrevQuestID=? AND (AllowableClasses=0 OR AllowableClasses & ?<>0) ORDER BY ID LIMIT 1`,
			cur, classBit).Scan(&next)
		if err != nil || next == 0 {
			break
		}
		out = append(out, next)
		cur = next
	}
	return out
}

// insertAt puts quests in the route before the first entry needing level or more.
func insertAt(entries []routeQuest, cache map[uint32]*quest, level uint32, ids []uint32) []routeQuest {
	at := len(entries)
	for i, e := range entries {
		if e.gate >= level || (e.id != 0 && cache[e.id] != nil && uint32(max(cache[e.id].MinLevel, 0)) >= level) {
			at = i
			break
		}
	}
	add := make([]routeQuest, 0, len(ids))
	for _, id := range ids {
		add = append(add, routeQuest{id: id})
	}
	return append(entries[:at:at], append(add, entries[at:]...)...)
}

// addClassQuests inserts the class quests of our race and class; it returns the ids added.
func (c *checker) addClassQuests(className, raceName string, classID uint32) []uint32 {
	b, err := os.ReadFile(filepath.Join("..", "journal", "classes", strings.ReplaceAll(className, " ", "_")+".yaml"))
	if err != nil {
		return nil
	}
	inRoute := map[uint32]bool{}
	for _, e := range c.entries {
		inRoute[e.id] = true
	}
	var added []uint32
	for lvl, first := range classQuestStarts(string(b), raceName) {
		var ids []uint32
		for _, id := range c.chain(first, 1<<(classID-1)) {
			q := c.load(id)
			if q == nil || inRoute[id] {
				continue
			}
			c.classify(q)
			ids = append(ids, id)
		}
		c.entries = insertAt(c.entries, c.cache, lvl, ids)
		added = append(added, ids...)
	}
	return added
}
