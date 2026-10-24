// Command guidecheck validates extracted leveling guides (journal/guides/*.json) against the world
// database: quest titles/ids, races, classes, levels, quest givers, prerequisites, and which quests the
// automation cannot do. With -ids it prints a quest list for auto.run.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"wowagent/internal/xpscore"

	_ "github.com/go-sql-driver/mysql"
)

type only struct {
	Race  string `json:"race"`
	Class string `json:"class"`
}

type step struct {
	Only    []only          `json:"only"`
	N       int             `json:"n"`
	Action  string          `json:"action"`
	Quest   string          `json:"quest"`
	QuestID uint32          `json:"quest_id"`
	NPC     string          `json:"npc"`
	Level   json.RawMessage `json:"level"`
	Class   string          `json:"class"`
	Note    string          `json:"note"`
}

type guide struct {
	Source  string `json:"source"`
	Faction string `json:"faction"`
	Race    string `json:"race"`
	Zone    string `json:"zone"`
	Levels  string `json:"levels"`
	Steps   []step `json:"steps"`
}

type quest struct {
	ID                        uint32
	Title                     string
	MinLevel, Level           int
	Races, Classes            uint32
	Prev                      uint32
	Special                   uint32
	Group                     int
	Type                      int // QuestInfoID: 1 elite/group, 41 PvP, 62/88/89 raid, 81 dungeon, 84 escort...
	Starters, Enders          []string
	hasObjectives, startsItem bool
}

var actions = map[string]bool{"accept": true, "turnin": true, "complete": true, "kill": true, "collect": true, "use_object": true,
	"talk": true, "travel": true, "flight_path": true, "set_hearth": true, "hearth": true, "train": true, "buy": true, "grind": true, "note": true}

var raceIDs = map[string]uint32{"human": 1, "orc": 2, "dwarf": 3, "night elf": 4, "undead": 5, "tauren": 6, "gnome": 7, "troll": 8,
	"blood elf": 10, "draenei": 11}

var classIDs = map[string]uint32{"warrior": 1, "paladin": 2, "hunter": 3, "rogue": 4, "priest": 5, "death knight": 6, "shaman": 7,
	"mage": 8, "warlock": 9, "druid": 11}

var firstInt = regexp.MustCompile(`\d+`)

// chainSuffix is the " (2)" some guides append to the titles of chained quests.
var chainSuffix = regexp.MustCompile(`\s*\(\d+\)\s*$`)

type checker struct {
	db       *sql.DB
	cache    map[uint32]*quest
	problems []string
	manual   map[uint32]string
	accepted map[uint32]bool
	done     map[uint32]bool
	order    []uint32
	entries  []routeQuest
	where    map[uint32]string // quest -> route file where it is first accepted
	fights   *xpscore.Table    // learned fights (-play)
}

func main() {
	dir := flag.String("dir", "../journal/guides", "directory with the guide files")
	ids := flag.Bool("ids", false, "print the quest ids for auto.run instead of the report")
	from := flag.Int("from", 0, "with -ids: first quest min level")
	to := flag.Int("to", 100, "with -ids: last quest min level")
	maxShow := flag.Int("max", 8, "problems shown per category")
	manualList := flag.Bool("manual", false, "list the quests of the route auto cannot do, with the reason and the level")
	plan := flag.Bool("plan", false, "print the next quests of the route for a character (see -char -race -class -level -n)")
	char := flag.String("char", "", "with -plan: character name (quests already rewarded are skipped)")
	race := flag.String("race", "", "with -plan: race of the character (e.g. Human)")
	class := flag.String("class", "", "with -plan: class of the character (e.g. Priest)")
	level := flag.Int("level", 1, "with -plan: character level")
	next := flag.Int("n", 10, "with -plan: number of quests to list")
	playMode := flag.Bool("play", false, "continue leveling the logged-in character -char along the Zygor route until -until (auto.run, auto.grind)")
	until := flag.Int("until", 10, "with -play: target level")
	routes := flag.String("routes", "../assets/routes", "with -play: directory of the converted Zygor routes (files picked by race)")
	dbcDir := flag.String("dbc", "../assets/dbc", "with -play: client DBC directory (FactionTemplate.dbc)")
	maxMin := flag.Int("max_min", 180, "with -play: stop after this many minutes")
	above := flag.Int("maxabove", 2, "with -plan: skip quests more than this many levels above the character (danger)")
	learnAllFlag := flag.Bool("learn", false, "learn the fights, borderline flee decisions and quest outcomes from the logs now (what level.sh does at start)")
	learnQ := flag.Bool("learn_quests", false, "learn journal/learn/quests.json from logs/level_*.log and list the quests passed over per level (tuning quest_defer, or 0.6)")
	score := flag.String("score", "", "rank grind monsters by xp/min around map,x,y for -level and -class (e.g. 0,-9464,62)")
	flag.Parse()
	if *learnQ {
		os.Exit(printLearnedQuests())
	}
	if *learnAllFlag {
		c := &checker{}
		nf, err := c.learnFights()
		fmt.Println("learned fights:", nf, err)
		nb, err := learnFlee()
		fmt.Println("learned borderline fights:", nb, err)
		_, nq, err := learnQuests()
		fmt.Println("learned quest outcomes:", nq, err)
		os.Exit(0)
	}
	// Files given as arguments are checked in that order (one race's chain); else the whole directory.
	files := flag.Args()
	port := env("AGENT_WOW_WORLDRPC_PORT", "8086")
	worldRPC := rpc{url: "http://127.0.0.1:" + port + "/rpc"}
	if *playMode && len(files) == 0 {
		var b brief
		if err := worldRPC.call("state.brief", map[string]any{}, &b); err != nil {
			fmt.Println("no session:", err)
			os.Exit(1)
		}
		var err error
		if files, err = routeFiles(*routes, b.Self.Race); err != nil || len(files) == 0 {
			fmt.Println("route files:", err, *routes)
			os.Exit(1)
		}
	}
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(*dir, "*.json"))
		sort.Strings(files)
	}
	if len(files) == 0 && *score == "" {
		fmt.Println("no guide files in", *dir)
		os.Exit(1)
	}
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s?timeout=5s&transaction_read_only=1",
		env("WORLD_DB_USER", "root"), env("WORLD_DB_PASSWORD", "password"), env("WORLD_DB_HOST", "192.168.100.7:3306"), env("WORLD_DB_NAME", "acore_world")))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	c := &checker{db: db, cache: map[uint32]*quest{}, manual: map[uint32]string{}, accepted: map[uint32]bool{}, done: map[uint32]bool{}, where: map[uint32]string{}}
	if *score != "" {
		os.Exit(c.printScore(*score, uint32(*level), classIDs[strings.ToLower(*class)], *dbcDir))
	}
	total := 0
	for _, f := range files {
		var g guide
		b, err := os.ReadFile(f)
		if err == nil {
			err = json.Unmarshal(b, &g)
		}
		if err != nil {
			c.problems = append(c.problems, fmt.Sprintf("%s: unreadable: %v", filepath.Base(f), err))
			continue
		}
		if g.Steps == nil { // class guides and tip files have another shape
			continue
		}
		total += len(g.Steps)
		c.check(filepath.Base(f), &g)
	}
	if *ids {
		c.printIDs(*from, *to)
		return
	}
	if *manualList {
		c.printManual(raceIDs[strings.ToLower(*race)], classIDs[strings.ToLower(*class)], *class)
		return
	}
	if *playMode {
		if *char == "" {
			fmt.Println("-play needs -char")
			os.Exit(1)
		}
		os.Exit(c.play(worldRPC, *char, uint32(*until), *dbcDir, *maxMin))
	}
	if *plan {
		c.printPlan(*char, *race, *class, uint32(*level), *next, *above)
		return
	}
	fmt.Printf("%d files, %d steps, %d quests, %d manual, %d problems\n", len(files), total, len(c.order), len(c.manual), len(c.problems))
	groups := map[string][]string{}
	var keys []string
	for _, p := range c.problems {
		k := kindOf(p)
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], p)
	}
	for _, k := range keys {
		fmt.Printf(" == %s: %d\n", k, len(groups[k]))
		for i, p := range groups[k] {
			if i >= *maxShow {
				break
			}
			fmt.Println("   !", p)
		}
	}
	var man []uint32
	for id := range c.manual {
		man = append(man, id)
	}
	sort.Slice(man, func(i, j int) bool { return man[i] < man[j] })
	for _, id := range man {
		fmt.Printf(" manual: quest %d %q: %s\n", id, c.cache[id].Title, c.manual[id])
	}
	if len(c.problems) > 0 {
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (c *checker) problem(file string, s step, format string, a ...any) {
	c.problems = append(c.problems, fmt.Sprintf("%s #%d %s %q: %s", file, s.N, s.Action, s.Quest, fmt.Sprintf(format, a...)))
}

func (c *checker) check(file string, g *guide) {
	race := raceIDs[strings.ToLower(g.Race)]
	for i, s := range g.Steps {
		if !actions[s.Action] {
			c.problem(file, s, "unknown action")
		}
		if s.N != i+1 {
			c.problem(file, s, "step number should be %d", i+1)
		}
		// "reach this level N": the guide expects us to level up here before going on.
		if s.Action == "note" && strings.EqualFold(strings.TrimSpace(s.Note), "reach this level") {
			if m := firstInt.Find(s.Level); m != nil {
				lvl, _ := strconv.Atoi(string(m))
				c.entries = append(c.entries, routeQuest{gate: uint32(lvl), only: s.Only})
			}
			continue
		}
		if s.Quest == "" && s.QuestID == 0 {
			continue
		}
		q := c.resolve(file, s, race)
		if q == nil {
			continue
		}
		lvl := 0
		if m := firstInt.Find(s.Level); m != nil {
			lvl, _ = strconv.Atoi(string(m))
		}
		if !allowed(q, s, race) {
			c.problem(file, s, "quest %d (races %d, classes %d) is not available to %s %s%s", q.ID, q.Races, q.Classes, g.Race, s.Class, onlyText(s.Only))
		}
		if lvl > 0 && lvl < q.MinLevel && s.Action == "accept" {
			c.problem(file, s, "quest %d needs level %d, guide level is %d", q.ID, q.MinLevel, lvl)
		}
		switch s.Action {
		case "accept":
			if s.NPC != "" && len(q.Starters) > 0 && !has(q.Starters, s.NPC) {
				c.problem(file, s, "quest giver is %v, guide says %q", q.Starters, s.NPC)
			}
			if q.Prev != 0 && !c.done[q.Prev] && !c.accepted[q.Prev] {
				c.problem(file, s, "prerequisite quest %d not done earlier in the route", q.Prev)
			}
			if !c.accepted[q.ID] {
				c.accepted[q.ID] = true
				c.order = append(c.order, q.ID)
			}
			c.entries = append(c.entries, routeQuest{id: q.ID, only: s.Only})
			if _, ok := c.where[q.ID]; !ok {
				c.where[q.ID] = file
			}
			c.classify(q)
		case "turnin":
			if s.NPC != "" && len(q.Enders) > 0 && !has(q.Enders, s.NPC) {
				c.problem(file, s, "turn-in NPC is %v, guide says %q", q.Enders, s.NPC)
			}
			if !c.accepted[q.ID] && q.Prev == 0 {
				c.accepted[q.ID] = true
				c.order = append(c.order, q.ID)
			}
			c.done[q.ID] = true
		}
	}
}

// allowed checks the quest's race and class masks against the step: with "only" pairs, one pair must
// fit; otherwise the guide's race and the step's class are used.
func allowed(q *quest, s step, race uint32) bool {
	fits := func(r, cl uint32) bool {
		return (q.Races == 0 || r == 0 || q.Races&(1<<(r-1)) != 0) && (q.Classes == 0 || (cl != 0 && q.Classes&(1<<(cl-1)) != 0))
	}
	if len(s.Only) > 0 {
		for _, o := range s.Only {
			r := raceIDs[strings.ToLower(o.Race)]
			if r == 0 {
				r = race
			}
			cl := classIDs[strings.ToLower(o.Class)]
			if o.Class == "" { // a lone word may be a class
				if c := classIDs[strings.ToLower(o.Race)]; c != 0 {
					cl, r = c, race
				}
			}
			if fits(r, cl) {
				return true
			}
		}
		return false
	}
	return fits(race, classIDs[strings.ToLower(s.Class)])
}

func onlyText(o []only) string {
	if len(o) == 0 {
		return ""
	}
	var parts []string
	for _, p := range o {
		parts = append(parts, strings.TrimSpace(p.Race+" "+p.Class))
	}
	return " (only " + strings.Join(parts, ", ") + ")"
}

func kindOf(p string) string {
	for _, k := range []string{"is not available", "quest giver is", "turn-in NPC is", "prerequisite quest", "no quest with this title",
		"ambiguous title", "does not exist", "in the database", "needs level", "unknown action", "step number", "unreadable", "database:", "none is available"} {
		if strings.Contains(p, k) {
			return k
		}
	}
	return "other"
}

func has(list []string, name string) bool {
	for _, n := range list {
		if strings.EqualFold(strings.TrimSpace(n), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// manualTypes are the quest types (QuestInfoID) the automation must not attempt alone.
var manualTypes = map[int]string{1: "elite / group quest", 41: "PvP quest", 62: "raid quest", 81: "dungeon quest", 82: "world event",
	83: "legendary quest", 84: "escort quest", 85: "heroic dungeon quest", 88: "raid quest", 89: "raid quest"}

// classify records why the automation cannot complete a quest.
func (c *checker) classify(q *quest) {
	switch {
	case q.Special&0x2 != 0 && !c.has("SELECT 1 FROM areatrigger_involvedrelation WHERE quest=? LIMIT 1", q.ID):
		c.manual[q.ID] = "event (escort, script, duel)"
	case q.Special&0x20 != 0 && !c.castDoable(q):
		c.manual[q.ID] = "spell cast on a target (no quest item or target known)"
	case q.Group > 0:
		c.manual[q.ID] = fmt.Sprintf("group of %d recommended", q.Group)
	case manualTypes[q.Type] != "":
		c.manual[q.ID] = manualTypes[q.Type]
	default:
		if why := c.objectiveProblem(q); why != "" {
			c.manual[q.ID] = why
		}
	}
}

// objectiveProblem finds objectives auto cannot reach: a creature or object that is never spawned
// (kill credit given by a script or an event), an item nothing drops (given by a script, bought,
// crafted). The quest's own start item counts as owned.
// has tells whether a query returns a row.
func (c *checker) has(query string, args ...any) bool {
	var one int
	return c.db.QueryRow(query, args...).Scan(&one) == nil
}

// castDoable mirrors world.quest_info for spell-credited quests: a quest item with an on-use spell,
// and for each objective a spawned target (the credited entry, or one named by the spell's target
// conditions).
func (c *checker) castDoable(q *quest) bool {
	var npc [4]int64
	var given [5]uint32
	if c.db.QueryRow(`SELECT RequiredNpcOrGo1, RequiredNpcOrGo2, RequiredNpcOrGo3, RequiredNpcOrGo4,
		StartItem, ItemDrop1, ItemDrop2, ItemDrop3, ItemDrop4 FROM quest_template WHERE ID=?`, q.ID).Scan(
		&npc[0], &npc[1], &npc[2], &npc[3], &given[0], &given[1], &given[2], &given[3], &given[4]) != nil {
		return false
	}
	var spell uint32
	for _, it := range given {
		if it != 0 && c.db.QueryRow("SELECT spellid_1 FROM item_template WHERE entry=? AND spellid_1>0 AND spelltrigger_1=0", it).Scan(&spell) == nil {
			break
		}
		spell = 0
	}
	if spell == 0 {
		return false
	}
	for _, n := range npc {
		switch {
		case n == 0:
		case n > 0 && c.has("SELECT 1 FROM creature WHERE id=? LIMIT 1", n):
		case n < 0 && c.has("SELECT 1 FROM gameobject WHERE id=? LIMIT 1", -n):
		case c.has(`SELECT 1 FROM conditions k WHERE k.SourceTypeOrReferenceId IN (13,17) AND k.SourceEntry=?
			AND k.ConditionTypeOrReference=31 AND ((k.ConditionValue1=3 AND EXISTS (SELECT 1 FROM creature x WHERE x.id=k.ConditionValue2))
			OR (k.ConditionValue1=5 AND EXISTS (SELECT 1 FROM gameobject x WHERE x.id=k.ConditionValue2))) LIMIT 1`, spell):
		default:
			return false
		}
	}
	return true
}

func (c *checker) objectiveProblem(q *quest) string {
	var npc [4]int64
	var item [6]uint32
	var startItem uint32
	err := c.db.QueryRow(`SELECT RequiredNpcOrGo1, RequiredNpcOrGo2, RequiredNpcOrGo3, RequiredNpcOrGo4,
		RequiredItemId1, RequiredItemId2, RequiredItemId3, RequiredItemId4, RequiredItemId5, RequiredItemId6, StartItem
		FROM quest_template WHERE ID=?`, q.ID).Scan(&npc[0], &npc[1], &npc[2], &npc[3], &item[0], &item[1], &item[2], &item[3], &item[4], &item[5], &startItem)
	if err != nil {
		return ""
	}
	exists := c.has
	if q.Special&0x20 != 0 {
		npc = [4]int64{} // targets checked by castDoable
	}
	for _, n := range npc {
		switch {
		case n > 0 && !exists("SELECT 1 FROM creature WHERE id=? LIMIT 1", n) &&
			!exists(`SELECT 1 FROM creature_template t JOIN creature c ON c.id=t.entry
				WHERE t.KillCredit1=? OR t.KillCredit2=? LIMIT 1`, n, n):
			return fmt.Sprintf("kill credit for creature %d that is never spawned (script or event)", n)
		case n < 0 && !exists("SELECT 1 FROM gameobject WHERE id=? LIMIT 1", -n):
			return fmt.Sprintf("object %d that is never spawned (script or event)", -n)
		}
	}
	for _, it := range item {
		if it == 0 || it == startItem {
			continue
		}
		dropped := exists("SELECT 1 FROM creature_loot_template WHERE Item=? LIMIT 1", it) ||
			exists("SELECT 1 FROM gameobject_loot_template WHERE Item=? LIMIT 1", it) ||
			exists("SELECT 1 FROM reference_loot_template WHERE Item=? LIMIT 1", it) ||
			exists("SELECT 1 FROM item_loot_template WHERE Item=? LIMIT 1", it)
		if dropped {
			continue
		}
		if exists("SELECT 1 FROM npc_vendor WHERE item=? LIMIT 1", it) {
			return fmt.Sprintf("item %d must be bought from a vendor", it)
		}
		if exists("SELECT 1 FROM quest_template WHERE StartItem=? LIMIT 1", it) {
			continue // given by another quest of the chain
		}
		return fmt.Sprintf("item %d is not dropped by anything (given by a script, crafted or used on a target)", it)
	}
	return ""
}

func (c *checker) resolve(file string, s step, race uint32) *quest {
	if s.QuestID != 0 {
		q := c.load(s.QuestID)
		if q == nil {
			c.problem(file, s, "quest id %d does not exist", s.QuestID)
			return nil
		}
		if s.Quest != "" && !strings.EqualFold(strings.TrimSpace(q.Title), strings.TrimSpace(chainSuffix.ReplaceAllString(s.Quest, ""))) {
			c.problem(file, s, "id %d is %q in the database", s.QuestID, q.Title)
		}
		return q
	}
	rows, err := c.db.Query("SELECT ID FROM quest_template WHERE LogTitle=?", chainSuffix.ReplaceAllString(s.Quest, ""))
	if err != nil {
		c.problem(file, s, "database: %v", err)
		return nil
	}
	var found []uint32
	for rows.Next() {
		var id uint32
		if rows.Scan(&id) == nil {
			found = append(found, id)
		}
	}
	rows.Close()
	var ok []*quest
	for _, id := range found {
		if q := c.load(id); q != nil && (race == 0 || q.Races == 0 || q.Races&(1<<(race-1)) != 0) {
			ok = append(ok, q)
		}
	}
	// Same title, several quests: tell them apart by the NPC of the step, then by the class.
	if len(ok) > 1 && s.NPC != "" {
		var byNPC []*quest
		for _, q := range ok {
			if has(q.Starters, s.NPC) || has(q.Enders, s.NPC) {
				byNPC = append(byNPC, q)
			}
		}
		if len(byNPC) > 0 {
			ok = byNPC
		}
	}
	if cls := classIDs[strings.ToLower(s.Class)]; len(ok) > 1 && cls != 0 {
		var byClass []*quest
		for _, q := range ok {
			if q.Classes == 0 || q.Classes&(1<<(cls-1)) != 0 {
				byClass = append(byClass, q)
			}
		}
		if len(byClass) > 0 {
			ok = byClass
		}
	}
	switch {
	case len(found) == 0:
		c.problem(file, s, "no quest with this title in the database")
	case len(ok) == 1:
		return ok[0]
	case len(ok) == 0:
		c.problem(file, s, "ids %v exist but none is available to this race", found)
	default:
		c.problem(file, s, "ambiguous title, ids %v: set quest_id", found)
	}
	return nil
}

func (c *checker) load(id uint32) *quest {
	if q, ok := c.cache[id]; ok {
		return q
	}
	q := &quest{ID: id}
	var startItem uint32
	var prev int32
	err := c.db.QueryRow(`SELECT q.LogTitle, q.MinLevel, q.QuestLevel, q.AllowableRaces, q.SuggestedGroupNum, q.QuestInfoID, q.StartItem,
		(q.RequiredNpcOrGo1<>0 OR q.RequiredNpcOrGo2<>0 OR q.RequiredNpcOrGo3<>0 OR q.RequiredNpcOrGo4<>0
		 OR q.RequiredItemId1<>0 OR q.RequiredItemId2<>0 OR q.RequiredItemId3<>0 OR q.RequiredItemId4<>0 OR q.RequiredItemId5<>0 OR q.RequiredItemId6<>0),
		COALESCE(a.AllowableClasses,0), COALESCE(a.PrevQuestID,0), COALESCE(a.SpecialFlags,0)
		FROM quest_template q LEFT JOIN quest_template_addon a ON a.ID=q.ID WHERE q.ID=?`, id).
		Scan(&q.Title, &q.MinLevel, &q.Level, &q.Races, &q.Group, &q.Type, &startItem, &q.hasObjectives, &q.Classes, &prev, &q.Special)
	if err != nil {
		c.cache[id] = nil
		return nil
	}
	q.startsItem = startItem != 0
	if prev > 0 { // negative = "must be active", not a hard prerequisite
		q.Prev = uint32(prev)
	}
	q.Starters = c.names(id, "queststarter")
	q.Enders = c.names(id, "questender")
	c.cache[id] = q
	return q
}

func (c *checker) names(id uint32, role string) []string {
	var out []string
	for _, tbl := range []string{"creature_" + role, "gameobject_" + role} {
		tpl := "creature_template"
		if strings.HasPrefix(tbl, "gameobject") {
			tpl = "gameobject_template"
		}
		rows, err := c.db.Query(fmt.Sprintf("SELECT DISTINCT t.name FROM %s g JOIN %s t ON t.entry=g.id WHERE g.quest=?", tbl, tpl), id)
		if err != nil {
			continue
		}
		for rows.Next() {
			var n string
			if rows.Scan(&n) == nil {
				out = append(out, n)
			}
		}
		rows.Close()
	}
	return out
}

func (c *checker) printIDs(from, to int) {
	var ids []uint32
	for _, id := range c.order {
		q := c.cache[id]
		if q == nil || c.manual[id] != "" || q.MinLevel < from || q.MinLevel > to {
			continue
		}
		ids = append(ids, id)
	}
	b, _ := json.Marshal(map[string]any{"quests": ids})
	fmt.Println(string(b))
}

// routeQuest is one accepted quest of a route in order.
type routeQuest struct {
	id   uint32
	only []only
	gate uint32 // a "reach this level" step (no quest): level to reach before going on
}

// printPlan lists the next quests of the route this character has not rewarded yet.
func (c *checker) printPlan(char, race, class string, level uint32, n, above int) {
	rewarded := map[uint32]bool{}
	if char != "" {
		cdb, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s?timeout=5s&transaction_read_only=1",
			env("WORLD_DB_USER", "root"), env("WORLD_DB_PASSWORD", "password"), env("WORLD_DB_HOST", "192.168.100.7:3306"), env("CHAR_DB_NAME", "acore_characters")))
		if err == nil {
			rows, err := cdb.Query("SELECT r.quest FROM character_queststatus_rewarded r JOIN characters c ON c.guid=r.guid WHERE c.name=?", char)
			if err == nil {
				for rows.Next() {
					var id uint32
					if rows.Scan(&id) == nil {
						rewarded[id] = true
					}
				}
				rows.Close()
			}
		}
	}
	fmt.Printf("route for %s %s level %d, %d quests already rewarded:\n", race, class, level, len(rewarded))
	out := planRoute(c.entries, c.cache, c.manual, rewarded, raceIDs[strings.ToLower(race)], classIDs[strings.ToLower(class)], race, class, level, above, n)
	for _, id := range out {
		q := c.cache[id]
		fmt.Printf("  %5d %-42s ql %2d min %2d  from %v\n", id, q.Title, q.Level, q.MinLevel, q.Starters)
	}
	b, _ := json.Marshal(map[string]any{"quests": out})
	fmt.Println(string(b))
}

// planRoute picks, in route order, the quests this character can take now: not rewarded, not manual,
// allowed to its race and class, level reached, not more than `above` levels over it, and whose
// prerequisite is rewarded or planned earlier in the same list (auto.run stops at the first refusal).
func planRoute(entries []routeQuest, cache map[uint32]*quest, manual map[uint32]string, rewarded map[uint32]bool,
	race, class uint32, raceName, className string, level uint32, above, n int) []uint32 {
	var out []uint32
	planned := map[uint32]bool{}
	for _, e := range entries {
		q := cache[e.id]
		if q == nil || rewarded[e.id] || planned[e.id] || manual[e.id] != "" {
			continue
		}
		if len(e.only) > 0 && !onlyMatches(e.only, race, class) {
			continue
		}
		if !allowed(q, step{Only: e.only, Class: className}, race) {
			continue
		}
		if uint32(q.MinLevel) > level || q.Level > int(level)+above {
			continue
		}
		if q.Prev != 0 && !rewarded[q.Prev] && !planned[q.Prev] {
			continue
		}
		planned[e.id] = true
		out = append(out, e.id)
		if len(out) >= n {
			break
		}
	}
	return out
}

// onlyMatches tells whether one of the "only" pairs fits the character. A lone word is a race, or a
// class when it names one ("only Priest").
func onlyMatches(o []only, race, class uint32) bool {
	for _, p := range o {
		pr, pc := raceIDs[strings.ToLower(p.Race)], classIDs[strings.ToLower(p.Class)]
		if p.Class == "" {
			if c := classIDs[strings.ToLower(p.Race)]; c != 0 {
				pr, pc = 0, c
			}
		}
		if (pr == 0 || pr == race) && (pc == 0 || pc == class) {
			return true
		}
	}
	return false
}

// printManual lists, in route order, the quests auto cannot complete alone (markdown table).
func (c *checker) printManual(race, class uint32, className string) {
	counts := map[string]int{}
	fmt.Println("| quête | titre | niv. | min | guide | raison |")
	fmt.Println("|---|---|---|---|---|---|")
	seen := map[uint32]bool{}
	for _, e := range c.entries {
		q := c.cache[e.id]
		why := c.manual[e.id]
		if q == nil || why == "" || seen[e.id] {
			continue
		}
		if race != 0 && ((len(e.only) > 0 && !onlyMatches(e.only, race, class)) || !allowed(q, step{Only: e.only, Class: className}, race)) {
			continue
		}
		seen[e.id] = true
		kind := why
		if i := strings.IndexAny(kind, "0123456789("); i > 0 {
			kind = strings.TrimSpace(kind[:i])
		}
		counts[kind]++
		fmt.Printf("| %d | %s | %d | %d | %s | %s |\n", e.id, q.Title, q.Level, q.MinLevel, strings.TrimSuffix(c.where[e.id], ".json"), why)
	}
	fmt.Println()
	for k, n := range counts {
		fmt.Printf("- %s : %d\n", k, n)
	}
}
