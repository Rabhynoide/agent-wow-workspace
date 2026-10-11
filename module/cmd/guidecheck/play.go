package main

// -play: "continue leveling the character", with the Zygor route as the only reference. In route order
// it runs the next quests the character can do (auto.run); when the next quest needs a higher level
// (minimum level, quest far above, or found too dangerous), it grinds one level on attackable monsters
// around the character (auto.grind) and comes back to the route. Quests auto cannot do, already done
// or not for this race/class are skipped. It stops at the target level, after repeated deaths, or when
// nothing progresses; details go to logs/level_<name>.log, short lines to stdout.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"wowagent/internal/dbc"
)

// playState is what the route walk needs besides the route: quests finished (rewarded before the run
// or done during it), quests skipped for this run, and the level from which a quest may be retried.
type playState struct {
	finished map[uint32]bool
	skipped  map[uint32]string
	retryAt  map[uint32]uint32
}

// nextBatch walks the route in order. It returns the first quests doable now (up to n, stopping at the
// first one that needs a higher level, so the order is kept), or, when the very first pending quest is
// too high, the level it needs and its id.
func nextBatch(entries []routeQuest, cache map[uint32]*quest, manual map[uint32]string, ps *playState,
	race, class uint32, raceName, className string, level uint32, above, n int) (batch []uint32, need, blocker uint32) {
	seen := map[uint32]bool{}
	for _, e := range entries {
		q := cache[e.id]
		if q == nil || seen[e.id] || ps.finished[e.id] || ps.skipped[e.id] != "" || manual[e.id] != "" {
			continue
		}
		seen[e.id] = true
		if len(e.only) > 0 && !onlyMatches(e.only, race, class) {
			continue
		}
		if !allowed(q, step{Only: e.only, Class: className}, race) {
			continue
		}
		if q.Prev != 0 && !ps.finished[q.Prev] && !inList(batch, q.Prev) {
			continue // chain broken by a skipped or manual quest
		}
		req := uint32(max(q.MinLevel, 1))
		if q.Level-above > int(req) {
			req = uint32(q.Level - above)
		}
		req = max(req, ps.retryAt[e.id])
		if req > level {
			if len(batch) == 0 {
				return nil, req, e.id
			}
			return batch, 0, 0
		}
		batch = append(batch, e.id)
		if len(batch) >= n {
			break
		}
	}
	return batch, 0, 0
}

func inList(l []uint32, v uint32) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// grayLevel is the highest monster level giving no experience (Trinity::XP::GetGrayLevel).
func grayLevel(l uint32) uint32 {
	switch {
	case l <= 5:
		return 0
	case l <= 39:
		return l - 5 - l/10
	case l <= 59:
		return l - 1 - l/5
	}
	return l - 9
}

// dangerMargin mirrors auto: monsters more than this many levels above the character are refused.
func dangerMargin(class uint32) uint32 {
	switch class {
	case 5, 8, 9:
		return 2
	}
	return 1
}

// playerFactions maps a race to its player faction template (ChrRaces.dbc).
var playerFactions = map[uint32]uint32{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 115, 8: 116, 10: 1610, 11: 1629}

type factionTpl struct {
	id, faction, group, friendGroup uint32
	enemies, friends                [4]uint32
}

func loadFactions(dir string) (map[uint32]factionTpl, error) {
	f, err := dbc.Open(filepath.Join(dir, "FactionTemplate.dbc"))
	if err != nil {
		return nil, err
	}
	out := make(map[uint32]factionTpl, f.Records)
	for r := 0; r < f.Records; r++ {
		t := factionTpl{id: f.U32(r, 0), faction: f.U32(r, 1), group: f.U32(r, 3), friendGroup: f.U32(r, 4)}
		for i := 0; i < 4; i++ {
			t.enemies[i], t.friends[i] = f.U32(r, 6+i), f.U32(r, 10+i)
		}
		out[t.id] = t
	}
	return out, nil
}

// friendly mirrors FactionTemplateEntry::IsFriendlyTo: such creatures cannot be attacked.
func friendly(a, p factionTpl) bool {
	if a.id == p.id {
		return true
	}
	for _, e := range a.enemies {
		if e != 0 && e == p.faction {
			return false
		}
	}
	for _, f := range a.friends {
		if f != 0 && f == p.faction {
			return true
		}
	}
	return a.friendGroup&p.group != 0 || a.group&p.friendGroup != 0
}

// grindTargets lists attackable, non-elite monsters spawned at least 3 times within radius of (x, y)
// that give experience and are not too dangerous, nearest first.
func (c *checker) grindTargets(mapID uint32, x, y, radius float64, level, class uint32, player factionTpl, factions map[uint32]factionTpl) []uint32 {
	lo := max(grayLevel(level)+1, level-min(level-1, 3))
	hi := level + dangerMargin(class)
	rows, err := c.db.Query(`SELECT t.entry, t.faction, COUNT(*) n,
			MIN(POW(c.position_x-?,2)+POW(c.position_y-?,2)) d
		FROM creature c JOIN creature_template t ON t.entry=c.id
		WHERE c.map=? AND POW(c.position_x-?,2)+POW(c.position_y-?,2) < ?
			AND t.`+"`rank`"+`=0 AND t.npcflag=0 AND t.type NOT IN (8,11,12)
			AND (t.unit_flags & (0x2|0x100|0x2000000))=0 AND (t.flags_extra & (0x2|0x80))=0
			AND t.minlevel >= ? AND t.maxlevel <= ?
			AND NOT EXISTS (SELECT 1 FROM game_event_creature g WHERE g.guid=c.guid)
		GROUP BY t.entry, t.faction HAVING n >= 3 ORDER BY d LIMIT 12`,
		x, y, mapID, x, y, radius*radius, lo, hi)
	if err != nil {
		fmt.Println("grind targets:", err)
		return nil
	}
	defer rows.Close()
	var out []uint32
	for rows.Next() {
		var entry, faction, n uint32
		var d float64
		if rows.Scan(&entry, &faction, &n, &d) != nil {
			continue
		}
		if t, ok := factions[faction]; ok && friendly(t, player) {
			continue
		}
		out = append(out, entry)
		if len(out) >= 5 {
			break
		}
	}
	return out
}

// rpc talks to the agent-wow world RPC (same endpoint as scripts/rpc.sh).
type rpc struct{ url string }

func (r rpc) call(method string, params, out any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	cl := &http.Client{Timeout: 60 * time.Second}
	resp, err := cl.Post(r.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return fmt.Errorf("%s: %s", method, b)
	}
	if len(env.Error) > 0 && string(env.Error) != "null" {
		return fmt.Errorf("%s: %s", method, env.Error)
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

type jobStatus struct {
	State  string   `json:"state"`
	Reason string   `json:"reason"`
	Quest  uint32   `json:"quest"`
	Step   string   `json:"step"`
	Done   []uint32 `json:"done"`
	Level  uint32   `json:"level"`
}

// job starts an auto job and waits for its end, logging each new step.
func (r rpc) job(method string, params map[string]any, log func(string, ...any)) (*jobStatus, error) {
	params["wait_s"] = 25
	st := &jobStatus{}
	if err := r.call(method, params, st); err != nil {
		return nil, err
	}
	last := ""
	for st.State == "running" {
		if st.Step != last {
			log("  %s", st.Step)
			last = st.Step
		}
		st = &jobStatus{}
		if err := r.call("auto.status", map[string]any{"wait_s": 25}, st); err != nil {
			return nil, err
		}
	}
	return st, nil
}

type brief struct {
	Self struct {
		Level   uint32  `json:"level"`
		XP      uint32  `json:"xp"`
		NextXP  uint32  `json:"nextXp"`
		Map     uint32  `json:"map"`
		X       float64 `json:"x"`
		Y       float64 `json:"y"`
		Dead    bool    `json:"dead"`
		ClassID uint32  `json:"classId"`
		Race    uint32  `json:"race"`
	} `json:"self"`
}

// routeFiles picks the route of the race, then the common Alliance route (Horde routes: not converted yet).
func routeFiles(dir string, race uint32) ([]string, error) {
	first := map[uint32]string{1: "alliance_01_", 4: "alliance_02_", 7: "alliance_03_", 3: "alliance_04_", 11: "alliance_05_"}[race]
	if first == "" {
		return nil, fmt.Errorf("no route for race %d (Alliance races only)", race)
	}
	all, _ := filepath.Glob(filepath.Join(dir, "alliance_*.json"))
	sort.Strings(all)
	var out []string
	for _, f := range all {
		b := filepath.Base(f)
		if strings.HasPrefix(b, first) || b >= "alliance_07_" {
			out = append(out, f)
		}
	}
	return out, nil
}

func nameOf(m map[string]uint32, id uint32) string {
	for k, v := range m {
		if v == id {
			return k
		}
	}
	return ""
}

// play is the -play loop.
func (c *checker) play(r rpc, char string, until uint32, dbcDir string, maxMin int) int {
	logf, _ := os.OpenFile(filepath.Join("..", "logs", "level_"+char+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	start := time.Now()
	stamp := func() string { return fmt.Sprintf("[%s]", time.Since(start).Round(time.Second)) }
	say := func(format string, a ...any) { // stdout and log
		line := stamp() + " " + fmt.Sprintf(format, a...)
		fmt.Println(line)
		if logf != nil {
			fmt.Fprintln(logf, time.Now().Format(time.DateTime), line)
		}
	}
	detail := func(format string, a ...any) { // log only
		if logf != nil {
			fmt.Fprintln(logf, time.Now().Format(time.DateTime), stamp(), fmt.Sprintf(format, a...))
		}
	}
	// Stopping the command stops the character too.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		_ = r.call("auto.stop", map[string]any{}, nil)
		say("interrupted: auto stopped")
		os.Exit(130)
	}()

	factions, err := loadFactions(dbcDir)
	if err != nil {
		say("warning: %v (friendly monsters are not filtered)", err)
	}
	var b brief
	if err := r.call("state.brief", map[string]any{}, &b); err != nil {
		say("no session: %v", err)
		return 1
	}
	race, class := b.Self.Race, b.Self.ClassID
	raceName, className := nameOf(raceIDs, race), nameOf(classIDs, class)
	ps := &playState{finished: c.rewardedOf(char), skipped: map[uint32]string{}, retryAt: map[uint32]uint32{}}
	say("%s (%s %s) level %d -> %d, %d quests already rewarded", char, raceName, className, b.Self.Level, until, len(ps.finished))
	deaths, idle := 0, 0
	for {
		if time.Since(start) > time.Duration(maxMin)*time.Minute {
			say("time limit (%d min) reached", maxMin)
			return 2
		}
		if err := r.call("state.brief", map[string]any{}, &b); err != nil {
			say("state.brief: %v", err)
			return 1
		}
		me := b.Self
		if me.Level >= until {
			say("level %d reached", me.Level)
			return 0
		}
		if deaths >= 3 {
			say("STOP: 3 deaths in this run (see %s)", "logs/level_"+char+".log")
			return 1
		}
		if idle >= 4 {
			say("STOP: no progress in 4 rounds (see logs/level_%s.log)", char)
			return 1
		}
		batch, need, blocker := nextBatch(c.entries, c.cache, c.manual, ps, race, class, raceName, className, me.Level, 2, 5)
		levelBefore, xpBefore, deathsBefore := me.Level, me.XP, deaths
		switch {
		case len(batch) > 0:
			var titles []string
			for _, id := range batch {
				titles = append(titles, fmt.Sprintf("%d %s", id, c.cache[id].Title))
			}
			say("L%d quests: %s", me.Level, strings.Join(titles, " | "))
			st, err := r.job("auto.run", map[string]any{"quests": batch}, detail)
			if err != nil {
				say("auto.run: %v", err)
				return 1
			}
			for _, id := range st.Done {
				ps.finished[id] = true
			}
			if len(st.Done) > 0 {
				say("  done %v", st.Done)
			}
			if st.State != "done" {
				q := st.Quest
				reason := st.Reason
				switch {
				case strings.Contains(reason, "too dangerous") || strings.Contains(reason, "needs level"):
					ps.retryAt[q] = me.Level + 1
					say("  %d postponed to level %d: %s", q, me.Level+1, reason)
				case strings.Contains(reason, "dead"):
					deaths++
					ps.retryAt[q] = me.Level + 1
					say("  DEATH on %d (%d in this run), postponed to level %d: %s", q, deaths, me.Level+1, reason)
				case st.State == "stopped":
					say("auto stopped from outside")
					return 1
				default:
					ps.skipped[q] = reason
					say("  SKIP %d %s: %s", q, c.cache[q].Title, reason)
				}
			}
		case need > 0:
			targets := c.grindTargets(me.Map, me.X, me.Y, 250, me.Level, class, factions[playerFactions[race]], factions)
			if len(targets) == 0 {
				targets = c.grindTargets(me.Map, me.X, me.Y, 600, me.Level, class, factions[playerFactions[race]], factions)
			}
			if len(targets) == 0 {
				say("STOP: quest %d %s needs level %d and no monster to grind near (%.0f,%.0f)", blocker, c.cache[blocker].Title, need, me.X, me.Y)
				return 1
			}
			say("L%d grind to %d (next quest %d %s needs %d) on %v", me.Level, me.Level+1, blocker, c.cache[blocker].Title, need, targets)
			left := time.Duration(maxMin)*time.Minute - time.Since(start)
			grindS := min(1500, max(60, int(left.Seconds())))
			st, err := r.job("auto.grind", map[string]any{"entries": targets, "until_level": me.Level + 1, "max_s": grindS}, detail)
			if err != nil {
				say("auto.grind: %v", err)
				return 1
			}
			if st.State != "done" {
				if strings.Contains(st.Reason, "dead") {
					deaths++
				}
				say("  grind ended: %s %s", st.State, st.Reason)
			}
		default:
			say("route finished for this character")
			return 0
		}
		if err := r.call("state.brief", map[string]any{}, &b); err == nil {
			if b.Self.Dead {
				if deaths == deathsBefore { // not counted yet by the job result
					deaths++
				}
				say("  dead: recovering")
				if st, err := r.job("auto.recover", map[string]any{}, detail); err != nil || st.State != "done" {
					say("STOP: could not recover (%v)", err)
					return 1
				}
			}
			if b.Self.Level > levelBefore {
				say("LEVEL %d", b.Self.Level)
			}
			if b.Self.Level == levelBefore && b.Self.XP == xpBefore {
				idle++
			} else {
				idle = 0
			}
		}
	}
}

// rewardedOf reads the quests the character has already turned in.
func (c *checker) rewardedOf(char string) map[uint32]bool {
	out := map[uint32]bool{}
	cdb, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s?timeout=5s&transaction_read_only=1",
		env("WORLD_DB_USER", "root"), env("WORLD_DB_PASSWORD", "password"), env("WORLD_DB_HOST", "192.168.100.7:3306"), env("CHAR_DB_NAME", "acore_characters")))
	if err != nil {
		return out
	}
	defer cdb.Close()
	rows, err := cdb.Query("SELECT r.quest FROM character_queststatus_rewarded r JOIN characters c ON c.guid=r.guid WHERE c.name=?", char)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}
