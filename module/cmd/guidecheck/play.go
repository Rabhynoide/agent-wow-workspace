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
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"wowagent/internal/dpslearn"
	"wowagent/internal/faction"
	"wowagent/internal/tuning"
	"wowagent/internal/xpscore"
)

// playState is what the route walk needs besides the route: quests finished (rewarded before the run
// or done during it), quests skipped for this run, and the level from which a quest may be retried.
type playState struct {
	finished map[uint32]bool
	skipped  map[uint32]string
	retryAt  map[uint32]uint32
	// when each skip was decided (kept skipKeep across runs, planstate.go)
	skippedAt map[uint32]time.Time
	// quests whose monsters were all dead or away: done later (after the next ones), at most 3 tries
	waitUntil map[uint32]time.Time
	tries     map[uint32]int
	flights   map[uint32]int    // failures (flight, death, danger) in each quest at failLevel, against tuning postpone_after_flights
	failLevel map[uint32]uint32 // the level of those failures (a new level starts again from 0)
	// hard: quests that often failed at this level (questlearn.go), passed over like the postponed
	// ones but tried when nothing else is left
	hard map[uint32]bool
}

// nextBatch walks the route in order. It returns the first quests doable now (up to n), or, when none
// is, the lowest level one of them needs and its id. A quest too high (minimum level not reached, or
// red: more than `above` levels over us) is passed over, like one postponed after a death or a flight
// (retryAt): stopping the route there left the next quests undone. Quest 76 (Jasperlode, level 10),
// given by the Fargodeep quest done at level 6, stopped the route at level 7 for 34 minutes of grind
// (2026-10-24); the user's rule: only red quests wait (above = 4, quest color by level difference).
// It is the blocker only when nothing else can be done. far (nil: none) marks the quests whose NPC is
// far away (nearfirst.go): they come after the near ones, waiting ones included.
func nextBatch(entries []routeQuest, cache map[uint32]*quest, manual map[uint32]string, ps *playState,
	race, class uint32, raceName, className string, level uint32, above, n int, far func(uint32) bool) (batch []uint32, need, blocker uint32) {
	seen := map[uint32]bool{}
	isFar := func(id uint32) bool { return far != nil && far(id) }
	var farBatch, farWaiting []uint32 // doable but far, in route order
	var laterNeed, laterID uint32     // the passed-over quest (too high or postponed) needing the lowest level
	var hard []uint32                 // learned hard quests passed over, in route order
	var waiting []uint32              // quests waiting for a retry (waitUntil), in route order
	defer func() {
		// Nothing else to do: the quests waiting for a retry go now rather than "route finished" or a
		// grind (only red quests wait for a level).
		if len(batch) == 0 && len(waiting) > 0 {
			batch, need, blocker = waiting[:min(len(waiting), n)], 0, 0
			return
		}
		if len(batch) == 0 && len(farBatch) > 0 {
			batch, need, blocker = farBatch[:min(len(farBatch), n)], 0, 0
			return
		}
		if len(batch) == 0 && len(farWaiting) > 0 {
			batch, need, blocker = farWaiting[:min(len(farWaiting), n)], 0, 0
			return
		}
		if len(batch) == 0 && len(hard) > 0 {
			batch, need, blocker = hard[:min(len(hard), n)], 0, 0 // nothing else: try them, no grind
			return
		}
		if len(batch) == 0 && laterID != 0 && (need == 0 || laterNeed < need) {
			need, blocker = laterNeed, laterID
		}
	}()
	for _, e := range entries {
		if e.gate > 0 {
			if len(e.only) > 0 && !onlyMatches(e.only, race, class) {
				continue
			}
			if e.gate > level {
				if len(batch) == 0 && len(farBatch) == 0 {
					return nil, e.gate, 0 // grind here, where the guide says so
				}
				return batch, 0, 0
			}
			continue
		}
		q := cache[e.id]
		if q == nil || seen[e.id] || ps.finished[e.id] || ps.skipped[e.id] != "" || manual[e.id] != "" {
			continue
		}
		if time.Now().Before(ps.waitUntil[e.id]) {
			seen[e.id] = true
			if q.MinLevel <= int(level) {
				if isFar(e.id) {
					farWaiting = append(farWaiting, e.id)
				} else {
					waiting = append(waiting, e.id)
				}
			}
			continue // its monsters respawn, or it failed: the next quests first
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
		if r := ps.retryAt[e.id]; r > level && r > req {
			req = r
		}
		if req > level {
			if laterID == 0 || req < laterNeed {
				laterNeed, laterID = req, e.id
			}
			continue
		}
		if ps.hard[e.id] {
			hard = append(hard, e.id)
			continue
		}
		if isFar(e.id) {
			farBatch = append(farBatch, e.id)
			continue
		}
		batch = append(batch, e.id)
		if len(batch) >= n {
			break
		}
	}
	return batch, 0, 0
}

// redAbove: a quest more than this many levels over the character is red (GetQuestDifficultyColor:
// 5 and more) and waits; orange (3-4 over) and below are done now (user's rule, 2026-10-24).
const redAbove = 4

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

// playerFactions maps a race to its player faction template (ChrRaces.dbc).
var playerFactions = map[uint32]uint32{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 115, 8: 116, 10: 1610, 11: 1629}

// Learned fights (xpscore.Table) and the combat log they come from, relative to module/.
const (
	fightsFile = "../journal/learn/fights.json"
	combatLog  = "../logs/combat.log"
)

// dpsFile: the learned damage rates (internal/dpslearn), rebuilt from the whole combat log (its averages
// forget, so the order of the lines is what matters).
const dpsFile = "../journal/learn/dps.json"

func learnDPS() (int, error) {
	t := dpslearn.New()
	n, err := t.Learn(combatLog)
	if err != nil {
		return 0, err
	}
	return n, t.Save(dpsFile)
}

// learnFights adds the new fight lines of the combat log to the learned table and saves it.
func (c *checker) learnFights() (int, error) {
	if c.fights == nil {
		t, err := xpscore.Load(fightsFile)
		if err != nil {
			return 0, err
		}
		c.fights = t
	}
	f, err := os.Open(combatLog)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := c.fights.Learn(f)
	if n > 0 {
		return n, c.fights.Save(fightsFile)
	}
	return 0, nil
}

// printScore prints the grind ranking around "map,x,y" (human player faction) without a session.
func (c *checker) printScore(where string, level, class uint32, dbcDir string) int {
	var mapID uint32
	var x, y float64
	if _, err := fmt.Sscanf(where, "%d,%f,%f", &mapID, &x, &y); err != nil || class == 0 {
		fmt.Println("-score map,x,y with -level and -class:", err)
		return 1
	}
	factions, err := faction.Load(dbcDir)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	if _, err := c.learnFights(); err != nil {
		fmt.Println("learned fights:", err)
	}
	fmt.Println("learned:", strings.Join(c.fights.Summary(class), " | "))
	targets, report := c.grindTargets(mapID, x, y, 250, level, class, factions[playerFactions[1]], factions)
	for _, l := range report {
		fmt.Println(l)
	}
	fmt.Println("chosen:", targets)
	return 0
}

// campRadius: hostile spawn points this close to a monster's spawn point may join its fight.
const campRadius = 15

// grindTargets ranks the attackable, non-elite monsters spawned at least 3 times within radius of
// (x, y) by experience per minute (xpscore: kill time, rest, search, death risk from the learned
// fights and the spawns around), and returns the best ones with a report line for each candidate.
func (c *checker) grindTargets(mapID uint32, x, y, radius float64, level, class uint32, player faction.Template, factions faction.Table) ([]uint32, []string) {
	lo := max(grayLevel(level)+1, level-min(level-1, 3))
	hi := level + 3 // searched; the too strong ones are refused by xpscore.Risky, as in auto
	rows, err := c.db.Query(`SELECT c.id, t.name, t.faction, t.minlevel, t.maxlevel, t.HealthModifier, t.DamageModifier,
			t.BaseAttackTime, t.ExperienceModifier, c.spawntimesecs, c.position_x, c.position_y, c.wander_distance,
			t.`+"`rank`"+`=0 AND t.npcflag=0 AND t.type NOT IN (8,11,12)
			AND (t.unit_flags & (0x2|0x100|0x2000000))=0 AND (t.flags_extra & (0x2|0x80))=0
			AND NOT EXISTS (SELECT 1 FROM game_event_creature g WHERE g.guid=c.guid) huntable
		FROM creature c JOIN creature_template t ON t.entry=c.id
		WHERE c.map=? AND POW(c.position_x-?,2)+POW(c.position_y-?,2) < ?`,
		mapID, x, y, (radius+campRadius)*(radius+campRadius))
	if err != nil {
		return nil, []string{"grind targets: " + err.Error()}
	}
	defer rows.Close()
	type spawn struct {
		entry, fac uint32
		x, y       float64
		wander     float64
		hostile    bool
	}
	var spawns []spawn
	mobs := map[uint32]*xpscore.Mob{}
	var order []uint32
	for rows.Next() {
		var m xpscore.Mob
		var fac uint32
		var respawn, sx, sy, wander float64
		var huntable bool
		if rows.Scan(&m.Entry, &m.Name, &fac, &m.MinLevel, &m.MaxLevel, &m.HealthMod, &m.DamageMod, &m.AttackMs, &m.ExpMod,
			&respawn, &sx, &sy, &wander, &huntable) != nil {
			continue
		}
		t, known := factions[fac]
		hostile := known && faction.Hostile(t, player)
		spawns = append(spawns, spawn{m.Entry, fac, sx, sy, wander, hostile})
		if !huntable || m.MinLevel < lo || m.MaxLevel > hi || (known && faction.Friendly(t, player)) ||
			math.Hypot(sx-x, sy-y) > radius {
			continue
		}
		if mobs[m.Entry] == nil {
			m.AreaRadius = radius
			mobs[m.Entry] = &m
			order = append(order, m.Entry)
		}
		mobs[m.Entry].Spawns++
		mobs[m.Entry].RespawnS += respawn
	}
	// Neighbours: hostile spawn points near each spawn point of the monster (an add risk).
	near := map[uint32]int{}
	for _, a := range spawns {
		m := mobs[a.entry]
		if m == nil || math.Hypot(a.x-x, a.y-y) > radius {
			continue
		}
		for _, b := range spawns {
			// Wandering monsters (wolves) come closer than their spawn point.
			if b != a && b.hostile && math.Hypot(a.x-b.x, a.y-b.y) < campRadius+a.wander+b.wander {
				near[a.entry]++
			}
		}
	}
	type ranked struct {
		m *xpscore.Mob
		s xpscore.Score
	}
	var all []ranked
	for _, e := range order {
		m := mobs[e]
		if m.Spawns < 3 {
			continue
		}
		m.RespawnS /= float64(m.Spawns)
		m.Neighbours = float64(near[e]) / float64(m.Spawns)
		all = append(all, ranked{m, xpscore.Of(*m, level, class, c.fights, tuning.NewSource(learnPath).Get().AddCost)})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].s.XPMin > all[j].s.XPMin })
	var out []uint32
	var report []string
	for _, r := range all {
		risky := xpscore.Risky(*r.m, level, class, c.fights)
		keep := risky == "" && r.s.PDeath <= xpscore.MaxDeathRisk && r.s.XP > 0 && len(out) < 4
		if keep {
			out = append(out, r.m.Entry)
		}
		report = append(report, fmt.Sprintf("%-5d %-22s L%d-%d spawns %2d near %.1f: %3.0f xp/min (xp %.0f, kill %.0fs, rest %.0fs, search %.0fs, loss %.0f%%, death %.0f%%; measured %.0f fights, %.1f deaths, chained %.1f/%.0f)%s",
			r.m.Entry, r.m.Name, r.m.MinLevel, r.m.MaxLevel, r.m.Spawns, r.m.Neighbours, r.s.XPMin, r.s.XP, r.s.KillS, r.s.RestS, r.s.SearchS,
			r.s.Loss, 100*r.s.PDeath, r.s.Fights, r.s.Deaths, r.s.ChainedDeaths, r.s.Chained, map[bool]string{true: " <-", false: ""}[keep]))
		if risky != "" {
			report[len(report)-1] += " refused: " + risky
		}
	}
	return out, report
}

const learnPath = "../journal/learn"

// learnFlee adds the borderline fights of logs/combat.log to journal/learn/flee.json.
func learnFlee() (int, error) {
	path := filepath.Join(learnPath, "flee.json")
	f, err := tuning.LoadFlee(path)
	if err != nil {
		return 0, err
	}
	lf, err := os.Open(combatLog)
	if err != nil {
		return 0, err
	}
	defer lf.Close()
	n := f.Learn(lf)
	if n > 0 {
		return n, f.Save(path)
	}
	return 0, nil
}

// fledWait: pause after a grind found nothing to hunt (four of them outlast a fled zone, 5 min).
const fledWait = 90 * time.Second

// mergeTargets appends the far targets missing from near, up to n in all.
func mergeTargets(near, far []uint32, n int) []uint32 {
	out := append([]uint32(nil), near...)
	for _, e := range far {
		if len(out) >= n {
			break
		}
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return out
}

// rpc talks to the agent-wow world RPC (same endpoint as scripts/rpc.sh).
type rpc struct {
	url      string
	deadline time.Time // jobs are stopped past it (zero = none)
	until    uint32    // jobs are stopped as soon as the character reaches this level (0 = never)
}

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
	Deaths uint32   `json:"deaths"`
	// Quests that failed while the batch went on (auto quest_hub).
	Failed []struct {
		Quest  uint32 `json:"quest"`
		Reason string `json:"reason"`
	} `json:"failed"`
}

// job starts an auto job and waits for its end, logging each new step. Past the deadline the job is
// stopped (a job stuck in a fight must not outlive the run's time limit).
func (r rpc) job(method string, params map[string]any, log func(string, ...any)) (*jobStatus, error) {
	params["wait_s"] = 25
	st := &jobStatus{}
	if err := r.call(method, params, st); err != nil {
		return nil, err
	}
	last := ""
	for st.State == "running" {
		if !r.deadline.IsZero() && time.Now().After(r.deadline) {
			_ = r.call("auto.stop", map[string]any{}, nil)
			log("  time limit: auto stopped")
			return &jobStatus{State: "stopped", Reason: "time limit"}, nil
		}
		if r.until > 0 && st.Level >= r.until {
			// Level reached: stop at once (user request, 2026-10-12), then fight off whatever is still
			// attacking us, since auto.stop also stops the guard.
			_ = r.call("auto.stop", map[string]any{}, nil)
			r.defend()
			log("  level %d reached: auto stopped", st.Level)
			return &jobStatus{State: "stopped", Reason: "level reached", Level: st.Level}, nil
		}
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

// defend fights back until nothing attacks us (combat.defend, at most 3 minutes).
func (r rpc) defend() {
	st := &jobStatus{}
	if r.call("combat.defend", map[string]any{"wait_s": 25}, st) != nil {
		return
	}
	for i := 0; i < 8 && st.State == "running"; i++ {
		st = &jobStatus{}
		if r.call("combat.status", map[string]any{"wait_s": 25}, st) != nil {
			return
		}
	}
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
	Quests []struct {
		ID uint32 `json:"id"`
	} `json:"quests"`
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
	r.deadline = start.Add(time.Duration(maxMin) * time.Minute)
	r.until = until
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

	factions, err := faction.Load(dbcDir)
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
	ps := &playState{finished: c.rewardedOf(char), skipped: map[uint32]string{}, retryAt: map[uint32]uint32{},
		waitUntil: map[uint32]time.Time{}, tries: map[uint32]int{}, skippedAt: map[uint32]time.Time{}, flights: map[uint32]int{}, failLevel: map[uint32]uint32{}}
	say("%s (%s %s) level %d -> %d, %d quests already rewarded", char, raceName, className, b.Self.Level, until, len(ps.finished))
	if ids := c.addClassQuests(className, raceName, class); len(ids) > 0 {
		var why []string
		for _, id := range ids {
			if m := c.manual[id]; m != "" {
				why = append(why, fmt.Sprintf("%d (manual: %s)", id, m))
			} else {
				why = append(why, fmt.Sprint(id))
			}
		}
		say("class quests added to the route: %s", strings.Join(why, ", "))
	}
	if n := loadPlan(planPath(char), ps, time.Now()); n > 0 {
		say("restored %d postponed or skipped quests from %s (postponed %v, skipped %v)", n, planPath(char), ps.retryAt, ps.skipped)
	}
	defer savePlan(planPath(char), ps)
	deaths, idle := 0, 0
	tune := tuning.NewSource(learnPath)
	var quests *questTable
	// learnAll adds the new log lines to the learned tables (fights, borderline flee decisions, quest
	// outcomes). It ran only before a grind: with no grind left (series QR and DE, 2026-10-24) the flee
	// table learned nothing for 8 runs. Now also at the start and at each new level.
	learnAll := func() {
		if n, err := c.learnFights(); err != nil {
			say("learned fights: %v", err)
		} else if n > 0 {
			say("learned %d fights: %s", n, strings.Join(c.fights.Summary(class), " | "))
		}
		if n, err := learnFlee(); err != nil {
			say("learned borderline fights: %v", err)
		} else if n > 0 {
			say("learned %d borderline fights", n)
		}
		if n, err := learnDPS(); err != nil {
			say("learned damage rates: %v", err)
		} else if n > 0 {
			say("learned %d damage rates", n)
		}
		if t, n, err := learnQuests(); err != nil {
			say("learned quests: %v", err)
		} else {
			quests = t
			if n > 0 {
				say("learned %d quest outcomes", n)
			}
		}
	}
	learnAll()
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
		if t := tune.Get(); t.QuestDefer > 0 {
			ps.hard = quests.hard(me.Level, t.QuestDefer, t.QuestDeferMin)
			if len(ps.hard) > 0 {
				detail("  quests passed over at level %d (learned failures): %v", me.Level, keys(ps.hard))
			}
		}
		taken := map[uint32]bool{}
		for _, q := range b.Quests {
			taken[q.ID] = true
		}
		batch, need, blocker := nextBatch(c.entries, c.cache, c.manual, ps, race, class, raceName, className, me.Level, redAbove, 5,
			c.farFunc(me.Map, me.X, me.Y, taken))
		levelBefore, xpBefore := me.Level, me.XP
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
			deaths += int(st.Deaths) // recovered inside the job (3 deaths in quest 62 were counted as 1)
			for _, id := range st.Done {
				ps.finished[id] = true
			}
			if len(st.Done) > 0 {
				say("  done %v", st.Done)
			}
			if st.State != "done" {
				switch {
				case st.State == "stopped" && st.Reason == "time limit":
					say("time limit (%d min) reached during a job", maxMin)
					return 2
				case st.Reason == "level reached":
					// Not an incident: exit 0, or bench.sh stops before its SAVE_AT snapshot and the
					// rest of the run (bench 8 → 12 of 2026-10-24 ended at level 10).
					say("level %d reached", st.Level)
					return 0
				case st.State == "stopped":
					say("auto stopped from outside")
					return 1
				}
				failures := st.Failed
				if len(failures) == 0 { // one by one: the job stops at the first failure
					failures = append(failures, struct {
						Quest  uint32 `json:"quest"`
						Reason string `json:"reason"`
					}{st.Quest, st.Reason})
				}
				for _, f := range failures {
					q, reason := f.Quest, f.Reason
					switch {
					case strings.Contains(reason, "needs level"):
						ps.retryAt[q] = me.Level + 1 // the server refuses it before
						say("  %d postponed to level %d: %s", q, me.Level+1, reason)
					case strings.Contains(reason, "too dangerous") || strings.Contains(reason, "dead"):
						// Only red quests wait for a level (user's rule, 2026-10-24): after a flight, a death
						// or a danger check, the other quests first, then again in 3 min; postponed to the next
						// level only after tuning postpone_after_flights failures at this level (2), so that
						// a deadly quest does not send us back to die again and again.
						if ps.failLevel[q] != me.Level {
							ps.flights[q], ps.failLevel[q] = 0, me.Level
						}
						ps.flights[q]++
						limit := max(tune.Get().PostponeAfterFlights, 1)
						died := strings.Contains(reason, "dead")
						what := "too dangerous"
						if strings.Contains(reason, "fled from monsters") {
							what = "fled"
						}
						switch {
						case ps.flights[q] < limit && died:
							ps.waitUntil[q] = time.Now().Add(3 * time.Minute)
							say("  DEATH on %d (%d in this run), failure %d/%d at level %d, retried in 3 min: %s", q, deaths+1, ps.flights[q], limit, me.Level, reason)
						case ps.flights[q] < limit:
							ps.waitUntil[q] = time.Now().Add(3 * time.Minute)
							say("  %d %s (%d/%d at level %d), retried in 3 min: %s", q, what, ps.flights[q], limit, me.Level, reason)
						case died:
							ps.retryAt[q] = me.Level + 1
							say("  DEATH on %d (%d in this run), postponed to level %d: %s", q, deaths+1, me.Level+1, reason)
						default:
							ps.retryAt[q] = me.Level + 1
							say("  %d postponed to level %d: %s", q, me.Level+1, reason)
						}
					case strings.Contains(reason, "no live entry") && ps.tries[q] < 2:
						ps.tries[q]++
						ps.waitUntil[q] = time.Now().Add(4 * time.Minute)
						say("  %d %s postponed 4 min (monsters dead or away, try %d/3)", q, c.cache[q].Title, ps.tries[q])
					default:
						ps.skipped[q] = reason
						ps.skippedAt[q] = time.Now()
						say("  SKIP %d %s: %s", q, c.cache[q].Title, reason)
					}
				}
				savePlan(planPath(char), ps)
			}
		case need > 0:
			learnAll()
			targets, report := c.grindTargets(me.Map, me.X, me.Y, 250, me.Level, class, factions[playerFactions[race]], factions)
			// One target is not enough: after a flight its spawn points are avoided for minutes and the
			// grind ends with nothing to hunt (Young Forest Bears among Prowlers, bench of 2026-10-13).
			// The farther ones come after the near ones.
			if len(targets) < 2 {
				far, farReport := c.grindTargets(me.Map, me.X, me.Y, 600, me.Level, class, factions[playerFactions[race]], factions)
				targets, report = mergeTargets(targets, far, 4), farReport // the far search covers the near one
			}
			for _, l := range report {
				detail("  score %s", l)
			}
			if len(targets) == 0 {
				say("STOP: level %d needed (quest %d) and no monster to grind near (%.0f,%.0f)", need, blocker, me.X, me.Y)
				return 1
			}
			why := fmt.Sprintf("the guide says reach level %d here", need)
			if blocker != 0 {
				why = fmt.Sprintf("next quest %d %s needs %d", blocker, c.cache[blocker].Title, need)
			}
			say("L%d grind to %d (%s) on %v", me.Level, me.Level+1, why, targets)
			left := time.Duration(maxMin)*time.Minute - time.Since(start)
			grindS := min(1500, max(60, int(left.Seconds())))
			st, err := r.job("auto.grind", map[string]any{"entries": targets, "until_level": me.Level + 1, "max_s": grindS}, detail)
			if err != nil {
				say("auto.grind: %v", err)
				return 1
			}
			deaths += int(st.Deaths)
			if st.State != "done" {
				say("  grind ended: %s %s", st.State, st.Reason)
				// The spawn points near a flight are avoided for 5 minutes: four rounds in the same
				// second ended the run (bench of 2026-10-14). Rest and let them clear first.
				if strings.Contains(st.Reason, "nothing to hunt right now") {
					say("  waiting %d s for the places we fled from to clear", int(fledWait.Seconds()))
					_ = r.call("combat.rest", map[string]any{"wait_s": 25}, nil)
					for t := time.Duration(0); t < fledWait; t += 10 * time.Second {
						time.Sleep(10 * time.Second)
						r.defend() // returns at once when nothing attacks us
					}
				}
			}
		default:
			say("route finished for this character")
			return 0
		}
		if err := r.call("state.brief", map[string]any{}, &b); err == nil {
			if b.Self.Dead {
				deaths++ // the job ended dead: not in its count
				say("  dead: recovering")
				if st, err := r.job("auto.recover", map[string]any{}, detail); err != nil || st.State != "done" {
					say("STOP: could not recover (%v)", err)
					return 1
				}
			}
			if b.Self.Level > levelBefore {
				say("LEVEL %d", b.Self.Level)
				learnAll()
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

func keys(m map[uint32]bool) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
