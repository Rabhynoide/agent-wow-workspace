// Command rotsim finds the warrior rotation that kills fastest, by simulating fights instead of
// measuring them in series (idea of the user, 2026-10-29: the costs, damage, targets and cooldowns
// are known). The spells come from the client (Spell.dbc in the MPQ archives: ranks, base points,
// durations, cooldowns), our stats and the monsters' from the world database (player_class_stats,
// creature_classlevelstats), the rules from AzerothCore: rage from the blows dealt and taken
// (Unit::RewardRage), none on a Heroic Strike swing, the attack table against a monster, armor,
// bleeds that ignore it, out of combat regeneration (gtOCTRegenHP).
//
// A scenario is a chain of pulls of 1, 2 or 3 monsters of a level difference, with the walking
// between them (rage decays, health comes back, the shout and the cooldowns run). Its cost is the
// seconds per pull: fighting, resting, and deaths at the tuning's death cost. For each level and
// pack size, the rotation of journal/classes/warrior.yaml is compared with the best one a coordinate
// search finds (one choice at a time, on the same random draws).
//
//	go run ./cmd/rotsim -levels 6,7,8 -weapon 5579
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// current is the rotation of journal/classes/warrior.yaml. Before this command (2026-10-29): Rend on
// the target only, Thunder Clap first on 2+ enemies, Heroic Strike from 30 rage, Cleave from 20, the
// shout renewed under 30 s, no Demoralizing Shout.
var current = policy{RendMin: 8, RendAll: true, DiesIn: 0, TCOne: 20, TCMulti: true, HS: 50, HSEvery: 3.5,
	Shout: 3, Demo: 2, Blood: 3, Cleave: 30}

// choices are the values the search tries for each field of the policy.
var choices = map[string][]any{
	"RendMin":      {-1.0, 0.0, 4.0, 6.0, 8.0, 10.0, 12.0},
	"RendAll":      {false, true},
	"DiesIn":       {0, 1},
	"TCOne":        {0.0, 20.0, 30.0, 40.0, 50.0, 60.0},
	"TCMulti":      {false, true},
	"TCFirst":      {false, true},
	"TCMultiFirst": {false, true},
	"HS":           {0.0, 15.0, 20.0, 30.0, 40.0, 50.0, 60.0, 80.0},
	"HSEvery":      {0.0, 3.5},
	"Shout":        {0, 1, 2, 3},
	"Demo":         {0, 1, 2, 3},
	"Blood":        {0, 1, 2, 3, 4},
	"Cleave":       {0.0, 20.0, 30.0, 40.0, 60.0},
}

type share struct {
	d int
	w float64
}

// diffs: the level differences of the monsters fought and their share (fights of the 6 → 8 series:
// mostly the zone's level, some under, a few over). Packs of 2+ are pulled at or under our level (the
// pull choice keeps away from the others; 3 monsters 1 level over kill us in 3 pulls of 4 in the model,
// the bot flees them).
var diffs = []share{{-1, 0.25}, {0, 0.4}, {1, 0.25}, {2, 0.1}}
var packDiffs = []share{{-1, 0.4}, {0, 0.5}, {1, 0.1}}

func diffsFor(foes int) []share {
	if foes >= 2 {
		return packDiffs
	}
	return diffs
}

func main() {
	client := flag.String("client", "../assets/client/Data", "client Data directory (MPQ archives)")
	learn := flag.String("learn", "../journal/learn", "learned tables (dps.json)")
	levelsF := flag.String("levels", "6,7,8,9", "our levels")
	foesF := flag.String("foes", "1,2,3", "pack sizes")
	weaponF := flag.String("weapon", "", "weapon item entry (default: a leveling two-hander of the level, see weaponFor)")
	gearStr := flag.Float64("str", 0, "strength from gear")
	armorF := flag.Float64("armor", 0, "our armor (default 45 per level: ~300 at 6-8 in the series)")
	trials := flag.Int("trials", 40, "chains per evaluation")
	pulls := flag.Int("pulls", 30, "pulls per chain")
	downtime := flag.Float64("downtime", 25, "seconds between two pulls (series 6 → 8: 46 encounters in 31 min, 40 % in combat)")
	check := flag.Bool("check", false, "print the model's fights with the current rotation next to what was measured")
	flag.Parse()

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/acore_world?timeout=5s&transaction_read_only=1",
		env("WORLD_DB_USER", "root"), env("WORLD_DB_PASSWORD", "password"), env("WORLD_DB_HOST", "192.168.100.7:3306")))
	if err != nil {
		fail(err)
	}
	g, err := loadGame(*client, *learn, db)
	if err != nil {
		fail(err)
	}
	for _, level := range ints(*levelsF) {
		w := weaponFor(level)
		if *weaponF != "" {
			n, _ := strconv.Atoi(*weaponF)
			if w, err = weaponOf(db, n); err != nil {
				fail(err)
			}
		}
		armor := *armorF
		if armor == 0 {
			armor = 45 * float64(level)
		}
		c := g.char(level, w, *gearStr, armor)
		fmt.Printf("== level %d: %s (%.0f-%.0f, %.1f s), AP %.0f, crit %.1f %%, %0.f HP, armor %.0f, regen %.1f HP/s\n",
			level, w.name, w.min, w.max, w.speed, c.ap, 100*c.crit, c.maxhp, armor, c.regen)
		if *check {
			checkModel(g, c, *trials, *pulls, *downtime)
			continue
		}
		for _, foes := range ints(*foesF) {
			sc := scenario{level: level, foes: foes, pulls: *pulls, downtime: *downtime, restBelow: 0.5, deathCost: 180}
			ev := func(p *policy) float64 { return evaluate(g, c, p, sc, *trials).cost }
			best := search(current, ev)
			cur, bst := evaluate(g, c, &current, sc, *trials), evaluate(g, c, &best, sc, *trials)
			fmt.Printf("  %d foe(s): current %.1f s/pull (fight %.1f, rest %.1f, deaths %.2f %%), best %.1f s/pull (fight %.1f, rest %.1f, deaths %.2f %%): %+.1f s (%+.0f %%)\n",
				foes, cur.cost, cur.fight, cur.rest, 100*cur.deaths, bst.cost, bst.fight, bst.rest, 100*bst.deaths,
				bst.cost-cur.cost, 100*(bst.cost-cur.cost)/cur.cost)
			fmt.Printf("    best: %s\n", diff(current, best))
			fmt.Printf("    casts per pull: current %s | best %s\n", casts(cur.casts), casts(bst.casts))
			for _, d := range diffsFor(foes) {
				one := sc
				one.diff = d.d
				a, b := run(g, c, &current, one, *trials), run(g, c, &best, one, *trials)
				fmt.Printf("    level %+d: %.1f -> %.1f s/pull\n", d.d, a.perPull(sc.deathCost), b.perPull(sc.deathCost))
			}
		}
	}
}

// weaponFor is a two-hander of the level when none is given: the level-6 snapshot's Militia Warhammer
// (2.6 damage per second) to level 9, then a quest or vendor weapon of about 1 + 0.65 x level damage per
// second at 3.3 s (Wooden Mallet 5.0 at 5, Severing Axe 7.0 at ~9).
func weaponFor(level int) weapon {
	if level <= 9 {
		return weapon{name: "Militia Warhammer", min: 6, max: 9, speed: 2.9}
	}
	dps := 1 + 0.65*float64(level)
	avg := dps * 3.3
	return weapon{name: fmt.Sprintf("two-hander %.1f dps", dps), min: avg * 0.8, max: avg * 1.2, speed: 3.3}
}

type eval struct {
	cost, fight, rest, deaths float64
	casts                     map[string]float64
}

// run is one scenario (one level difference) over trials chains, on the same draws for every policy.
func run(g *gameData, c *char, p *policy, sc scenario, trials int) *result {
	tot := &result{casts: map[string]float64{}}
	for i := 0; i < trials; i++ {
		r := newSim(g, c, p, &sc, int64(1000*sc.diff+i)).chain()
		tot.fight += r.fight
		tot.rest += r.rest
		tot.deaths += r.deaths
		tot.hpLost += r.hpLost
		tot.pulls += r.pulls
		for k, v := range r.casts {
			tot.casts[k] += v
		}
	}
	return tot
}

// evaluate mixes the level differences.
func evaluate(g *gameData, c *char, p *policy, sc scenario, trials int) eval {
	e := eval{casts: map[string]float64{}}
	for _, d := range diffsFor(sc.foes) {
		if g.mobs[c.level+d.d].hp == 0 {
			continue
		}
		one := sc
		one.diff = d.d
		r := run(g, c, p, one, trials)
		n := float64(r.pulls)
		e.cost += d.w * r.perPull(sc.deathCost)
		e.fight += d.w * r.fight / n
		e.rest += d.w * r.rest / n
		e.deaths += d.w * r.deaths / n
		for k, v := range r.casts {
			e.casts[k] += d.w * v / n
		}
	}
	return e
}

// search changes one field at a time to its best value, until nothing changes.
func search(start policy, cost func(*policy) float64) policy {
	best := start
	bestCost := cost(&best)
	names := make([]string, 0, len(choices))
	for k := range choices {
		names = append(names, k)
	}
	sort.Strings(names)
	for round := 0; round < 4; round++ {
		changed := false
		for _, name := range names {
			for _, v := range choices[name] {
				p := best
				f := reflect.ValueOf(&p).Elem().FieldByName(name)
				f.Set(reflect.ValueOf(v))
				if p == best {
					continue
				}
				// a gain under 0.5 % is noise
				if c := cost(&p); c < bestCost*0.995 {
					best, bestCost, changed = p, c, true
				}
			}
		}
		if !changed {
			break
		}
	}
	return best
}

// diff names the fields where b differs from a.
func diff(a, b policy) string {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	var out []string
	for i := 0; i < va.NumField(); i++ {
		if va.Field(i).Interface() != vb.Field(i).Interface() {
			out = append(out, fmt.Sprintf("%s %v -> %v", va.Type().Field(i).Name, va.Field(i).Interface(), vb.Field(i).Interface()))
		}
	}
	if len(out) == 0 {
		return "same as current"
	}
	return strings.Join(out, ", ")
}

func casts(m map[string]float64) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		if m[k] >= 0.05 {
			out = append(out, fmt.Sprintf("%s %.1f", k, m[k]))
		}
	}
	return strings.Join(out, ", ")
}

// checkModel prints the model's fights against one monster with the current rotation, to compare with
// what the bot measured (journal/learn/dps.json, the bench reports).
func checkModel(g *gameData, c *char, trials, pulls int, downtime float64) {
	for _, d := range diffs {
		sc := scenario{level: c.level, foes: 1, diff: d.d, pulls: pulls, downtime: downtime, restBelow: 0.5, deathCost: 180}
		r := run(g, c, &current, sc, trials)
		n := float64(r.pulls)
		hp := g.mobs[c.level+d.d].hp
		fmt.Printf("  1 foe %+d (%.0f HP): fight %.1f s, %.0f damage per second, health lost %.0f per pull, casts %s\n",
			d.d, hp, r.fight/n, hp/(r.fight/n), r.hpLost/n, casts(scale(r.casts, 1/n)))
	}
	if v, ok := g.learnedUs[c.level]; ok {
		fmt.Printf("  learned: %.1f damage per second\n", v)
	}
}

func scale(m map[string]float64, f float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range m {
		out[k] = v * f
	}
	return out
}

func ints(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(err error) {
	fmt.Println(err)
	os.Exit(1)
}
