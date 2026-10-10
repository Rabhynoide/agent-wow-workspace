// Command classcheck validates a class leveling file (journal/classes/<class>.yaml) against the client
// data (talents, trees, spells: assets/dbc) and the world database (trainer levels, class quests).
// The talent order is replayed level by level with the server rules (one point per level from 10,
// 5 points per tier in the tree, prerequisites, maximum ranks, respecs).
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"

	"wowagent/internal/dbc"
)

type talentEntry struct {
	Levels    string `yaml:"levels"`
	Tree      string `yaml:"tree"`
	Talent    string `yaml:"talent"`
	Points    int    `yaml:"points"`
	RemovedAt int    `yaml:"removed_at"`
	Note      string `yaml:"note"`
}

type ability struct {
	Name  string `yaml:"name"`
	Level *int   `yaml:"level"`
	Use   string `yaml:"use"`
}

type classQuest struct {
	Level   int               `yaml:"level"`
	Unlocks string            `yaml:"unlocks"`
	Quests  map[string]string `yaml:"quests"`
}

type classFile struct {
	Class        string        `yaml:"class"`
	Spec         string        `yaml:"recommended_leveling_spec"`
	TalentOrder  []talentEntry `yaml:"talent_order"`
	KeyAbilities []ability     `yaml:"key_abilities"`
	ClassQuests  []classQuest  `yaml:"class_quests"`
	Training     struct {
		MaxRank     []string `yaml:"max_rank"`
		SingleRank  []string `yaml:"train_asap_single_rank"`
		LowPriority []string `yaml:"low_priority"`
		Skip        []string `yaml:"skip"`
	} `yaml:"training"`
}

var classIDs = map[string]uint32{"warrior": 1, "paladin": 2, "hunter": 3, "rogue": 4, "priest": 5, "death knight": 6, "deathknight": 6,
	"shaman": 7, "mage": 8, "warlock": 9, "druid": 11}

// raceKeys maps the race keys of class_quests to race ids.
var raceKeys = map[string][]uint32{"human": {1}, "orc": {2}, "dwarf": {3}, "nightelf": {4}, "night elf": {4}, "undead": {5}, "tauren": {6},
	"gnome": {7}, "troll": {8}, "bloodelf": {10}, "blood elf": {10}, "draenei": {11}}

type report struct {
	problems, info []string
}

func (r *report) bad(format string, a ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, a...))
}
func (r *report) note(format string, a ...any) { r.info = append(r.info, fmt.Sprintf(format, a...)) }

func main() {
	dbcDir := flag.String("dbc", "../assets/dbc", "client data directory")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Println("usage: classcheck [-dbc dir] ../journal/classes/<class>.yaml ...")
		os.Exit(2)
	}
	d, err := dbc.Load(*dbcDir)
	if err != nil {
		fmt.Println("dbc:", err)
		os.Exit(1)
	}
	db, _ := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/acore_world?timeout=5s&transaction_read_only=1",
		env("WORLD_DB_USER", "root"), env("WORLD_DB_PASSWORD", "password"), env("WORLD_DB_HOST", "192.168.100.7:3306")))
	failed := false
	for _, path := range flag.Args() {
		b, err := os.ReadFile(path)
		var cf classFile
		if err == nil {
			err = yaml.Unmarshal(b, &cf)
		}
		if err != nil {
			fmt.Printf("%s: %v\n", path, err)
			failed = true
			continue
		}
		r := &report{}
		class := classIDs[strings.ToLower(cf.Class)]
		if class == 0 {
			r.bad("unknown class %q", cf.Class)
		} else {
			checkTalents(r, d, class, cf.TalentOrder)
			checkSpells(r, d, db, class, cf)
			checkQuests(r, db, class, cf.ClassQuests)
		}
		fmt.Printf("== %s (%s, spec %s): %d problems\n", path, cf.Class, cf.Spec, len(r.problems))
		for _, p := range r.problems {
			fmt.Println("  !", p)
		}
		for _, p := range r.info {
			fmt.Println("  -", p)
		}
		failed = failed || len(r.problems) > 0
	}
	if failed {
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// levelRange parses "12-14" or "25".
func levelRange(s string) (int, int, error) {
	parts := strings.SplitN(strings.TrimSpace(s), "-", 2)
	a, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	b := a
	if len(parts) == 2 {
		if b, err = strconv.Atoi(strings.TrimSpace(parts[1])); err != nil {
			return 0, 0, err
		}
	}
	return a, b, nil
}

type pointAdd struct {
	level, entry int
}

// checkTalents replays the order level by level with the server rules.
func checkTalents(r *report, d *dbc.Data, class uint32, order []talentEntry) {
	talents := make([]*dbc.Talent, len(order))
	var adds []pointAdd
	removals := map[int][]int{} // level -> entries whose points are removed at that level (respec)
	for i, e := range order {
		t := d.TalentBySpellName(class, e.Talent)
		if t == nil {
			r.bad("talent %q is not a %s talent in this client", e.Talent, className(class))
			continue
		}
		talents[i] = t
		if tab := d.Tabs[t.Tab]; tab != nil && e.Tree != "" && !strings.EqualFold(tab.Name, e.Tree) {
			r.bad("talent %q is in the %s tree, not %s", e.Talent, tab.Name, e.Tree)
		}
		a, b, err := levelRange(e.Levels)
		if err != nil {
			r.bad("talent %q: bad levels %q", e.Talent, e.Levels)
			continue
		}
		n := b - a + 1
		if e.Points != n && n != 1 {
			r.note("talent %q: %d points over levels %s (%d levels): spread evenly", e.Talent, e.Points, e.Levels, n)
		}
		for k := 0; k < e.Points; k++ {
			lvl := a + k*n/max(e.Points, 1)
			adds = append(adds, pointAdd{lvl, i})
		}
		if e.RemovedAt > 0 {
			removals[e.RemovedAt] = append(removals[e.RemovedAt], i)
		}
	}
	sort.SliceStable(adds, func(i, j int) bool { return adds[i].level < adds[j].level })
	rank := map[uint32]int{}
	spent := map[uint32]int{}
	total, ai := 0, 0
	for lvl := 10; lvl <= 80; lvl++ {
		for _, i := range removals[lvl] {
			if t := talents[i]; t != nil {
				rank[t.ID] -= order[i].Points
				spent[t.Tab] -= order[i].Points
				total -= order[i].Points
				r.note("level %d: respec removes %d points of %s", lvl, order[i].Points, order[i].Talent)
			}
		}
		for ; ai < len(adds) && adds[ai].level == lvl; ai++ {
			e, t := order[adds[ai].entry], talents[adds[ai].entry]
			if t == nil {
				continue
			}
			switch {
			case rank[t.ID] >= t.MaxRank():
				r.bad("level %d: %s already at its maximum rank %d", lvl, e.Talent, t.MaxRank())
			case spent[t.Tab] < int(t.Row)*5:
				r.bad("level %d: %s (tier %d) needs %d points in %s, only %d spent", lvl, e.Talent, t.Row+1, t.Row*5, d.Tabs[t.Tab].Name, spent[t.Tab])
			case t.DependsOn != 0 && rank[t.DependsOn] < int(t.DependsOnRank)+1:
				r.bad("level %d: %s needs %s rank %d first", lvl, e.Talent, talentName(d, t.DependsOn), t.DependsOnRank+1)
			}
			rank[t.ID]++
			spent[t.Tab]++
			total++
			if total > lvl-9 {
				r.bad("level %d: %d points spent but only %d available", lvl, total, lvl-9)
			}
		}
		if ai < len(adds) && adds[ai].level < lvl {
			ai++ // levels outside 10-80
		}
	}
	r.note("talent order: %d points at level 80 (71 available)", total)
}

func talentName(d *dbc.Data, id uint32) string {
	if t := d.Talents[id]; t != nil {
		if s := d.Spells[t.Ranks[0]]; s != nil {
			return s.Name
		}
	}
	return fmt.Sprint(id)
}

func className(class uint32) string {
	for k, v := range classIDs {
		if v == class && !strings.Contains(k, " ") {
			return k
		}
	}
	return fmt.Sprint(class)
}

// checkSpells checks every spell name of the file and gives the level at which trainers teach it.
func checkSpells(r *report, d *dbc.Data, db *sql.DB, class uint32, cf classFile) {
	learnLevel := trainerLevels(d, db, class)
	seen := map[string]bool{}
	var names []string
	for _, a := range cf.KeyAbilities {
		names = append(names, a.Name)
	}
	t := cf.Training
	for _, l := range [][]string{t.MaxRank, t.SingleRank, t.LowPriority, t.Skip} {
		names = append(names, l...)
	}
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		if len(d.ClassSpells(class, n)) == 0 && d.TalentBySpellName(class, n) == nil {
			r.bad("spell %q is not a %s spell in this client", n, className(class))
		}
	}
	for _, a := range cf.KeyAbilities {
		tl, trained := learnLevel[a.Name]
		talent := d.TalentBySpellName(class, a.Name) != nil
		switch {
		case talent:
			// Rank 1 comes from the talent; trainers only teach the next ranks.
			r.note("%s: talent (rank 1 from the talent tree)", a.Name)
		case !trained:
			r.note("%s: not taught by class trainers (quest or starting ability)", a.Name)
		case tl.rank != "" && tl.rank != "Rank 1":
			r.note("%s: rank 1 known from character creation; trainers teach %s at level %d", a.Name, tl.rank, tl.level)
		case a.Level == nil:
			r.note("%s: learned at level %d (missing in the file)", a.Name, tl.level)
		case *a.Level != tl.level:
			r.bad("%s: the file says level %d, class trainers teach it at %d", a.Name, *a.Level, tl.level)
		}
	}
}

type trained struct {
	level int
	rank  string // rank taught at that level ("Rank 1", ...)
}

// trainerLevels gives the lowest level at which the class trainers teach each spell name.
func trainerLevels(d *dbc.Data, db *sql.DB, class uint32) map[string]trained {
	out := map[string]trained{}
	rows, err := db.Query(`SELECT ts.SpellId, MIN(ts.ReqLevel) FROM trainer_spell ts JOIN trainer t ON t.Id=ts.TrainerId
		WHERE t.Type=0 AND t.Requirement=? GROUP BY ts.SpellId`, class)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var lvl int
		if rows.Scan(&id, &lvl) != nil {
			continue
		}
		s := d.Spells[id]
		if s == nil {
			continue
		}
		for _, learned := range s.Learned() {
			if ls := d.Spells[learned]; ls != nil {
				if cur, ok := out[ls.Name]; !ok || lvl < cur.level {
					out[ls.Name] = trained{lvl, ls.Rank}
				}
			}
		}
	}
	return out
}

// checkQuests checks the class quests: id exists, races and class allowed, level.
func checkQuests(r *report, db *sql.DB, class uint32, qs []classQuest) {
	for _, cq := range qs {
		for raceKey, text := range cq.Quests {
			id := 0
			if i, j := strings.LastIndex(text, "("), strings.LastIndex(text, ")"); i >= 0 && j > i {
				id, _ = strconv.Atoi(text[i+1 : j])
			}
			if id == 0 {
				r.bad("class quest %q: no quest id", text)
				continue
			}
			var title string
			var races, classes uint32
			var minLevel int
			err := db.QueryRow(`SELECT q.LogTitle, q.AllowableRaces, COALESCE(a.AllowableClasses,0), q.MinLevel
				FROM quest_template q LEFT JOIN quest_template_addon a ON a.ID=q.ID WHERE q.ID=?`, id).Scan(&title, &races, &classes, &minLevel)
			if err != nil {
				r.bad("class quest %d (%s): not in the database", id, raceKey)
				continue
			}
			if want := strings.TrimSpace(text[:strings.LastIndex(text, "(")]); !strings.EqualFold(want, title) {
				r.bad("class quest %d is %q in the database, the file says %q", id, title, want)
			}
			if classes != 0 && classes&(1<<(class-1)) == 0 {
				r.bad("class quest %d %q is not for this class (classes %d)", id, title, classes)
			}
			for _, part := range strings.Split(strings.ToLower(raceKey), "_") {
				for _, race := range raceKeys[part] {
					if races != 0 && races&(1<<(race-1)) == 0 {
						r.bad("class quest %d %q is not available to %s", id, title, part)
					}
				}
			}
			if minLevel > cq.Level {
				r.bad("class quest %d %q needs level %d, the file says %d", id, title, minLevel, cq.Level)
			}
		}
	}
}
