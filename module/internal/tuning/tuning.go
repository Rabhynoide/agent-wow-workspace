// Package tuning holds the knobs of the bot's caution (journal/learn/tuning.json, read again when it
// changes) and what it learns about them: the death rate of the fights it did not flee from, by
// number of fresh attackers and health slack (journal/learn/flee.json). Goal: speedrun 1 → 80, so a
// flight is only worth it when the expected cost of staying (P(death) × death cost) exceeds the cost
// of the flight (run, rest, quest postponed).
package tuning

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Tuning: the defaults are the rules of commit eb70f0b, without the monster avoidance (variant E, 2026-10-17).
type Tuning struct {
	FleeMargin  float64 `json:"flee_margin"`  // % of health kept after the expected loss
	HighMargin  float64 `json:"high_margin"`  // + per attacker 2+ levels above us
	ChaseMargin float64 `json:"chase_margin"` // + per fresh attacker beyond the first
	// FleeDPS: once the damage we take is measured (combat/dps.go), the margin is at least what that
	// damage costs during a flight: dps × FleeExposure seconds + FleeReserve (% of health), when it is
	// above the three fixed margins (2026-10-25: flights started under 30 % killed 41 % of the time, under
	// 45 % 18 %, over 45 % 4-9 %: the fixed margins fled too late against strong hits).
	FleeDPS      bool    `json:"flee_dps"`
	FleeExposure float64 `json:"flee_exposure"` // seconds the attackers keep hitting while we run
	FleeReserve  float64 `json:"flee_reserve"`  // % of health kept on top
	CrowdFlee    int     `json:"crowd_flee"`    // fresh attackers that make a crowd (0 = no crowd rule)
	CrowdLoss    float64 `json:"crowd_loss"`    // expected loss over which a crowd makes us flee
	Avoid        bool    `json:"avoid"`         // paths walk around monsters (move/avoid.go)
	// guidecheck: flights in a quest before it is postponed to the next level.
	PostponeAfterFlights int `json:"postpone_after_flights"`
	// Learning: in a borderline case (expected to survive, the rule says flee), stay and fight with
	// this probability, to measure the death rate; once a case has MinFights, decide from it.
	Explore    float64 `json:"explore"`
	Learn      bool    `json:"learn"`
	MinFights  float64 `json:"min_fights"`
	DeathCostS float64 `json:"death_cost_s"` // run to the corpse, rest, durability
	FleeCostS  float64 `json:"flee_cost_s"`  // run, rest, the quest or spot left
	// A flight can kill too: since 2026-10-21, 8 of the 19 flights the rule ordered died, nearly all
	// with no health slack left. The learned death rate of the flights of a case (prior FleeDeathPrior,
	// worth fleePriorN flights) adds P(death) × DeathCostS to FleeCostS. Where it reaches
	// ExploreFleeDeath, staying is explored even without slack (the rule never stays there).
	FleeDeathPrior   float64 `json:"flee_death_prior"`
	ExploreFleeDeath float64 `json:"explore_flee_death"` // 0: no exploration without slack
	// Extra health loss per add, times the single fight (xpscore.AddCost): the pull choice and the
	// flight rule. Engagements of 2026-10-13/14 (levels 6-9, won, started at 90 %+): 2 monsters lost
	// 1.5 times a single fight, 3 monsters 1.7 times (about 0.5 per add, but the lost ones are missing).
	AddCost float64 `json:"add_cost"`
	// Adaptive pull size (combat/pullsize.go): the monsters we accept at once (target and its adds)
	// start at PullSize, go up after PullUpAfter easy engagements of that size, down after a hard
	// one, within 1..PullSizeMax. The crowd rule only counts attackers beyond the pull size. Off:
	// the old rules (any adds that fit the expected loss, crowd rule at CrowdFlee).
	PullAdapt   bool `json:"pull_adapt"`
	PullSize    int  `json:"pull_size"`
	PullSizeMax int  `json:"pull_size_max"`
	PullUpAfter int  `json:"pull_up_after"`
	// QuestHub: auto.run takes all the quests of its batch first, works on their objectives, then
	// turns them in, by rounds (auto/hub.go), instead of accept, objectives, turn-in one by one.
	QuestHub bool `json:"quest_hub"`
	// WarriorRestBelow: a kill job rests (and eats) before the next pull below this health (percent)
	// for a warrior; the classes with a heal rest below 55.
	WarriorRestBelow uint32 `json:"warrior_rest_below"`
	// SafeRestBelow: below this health (percent), a rest first walks to the spot within 40 yards
	// farthest from the hostile monsters when one stands closer than safeRestClear (combat/saferest.go).
	// 0 = rest where we stand.
	SafeRestBelow uint32 `json:"safe_rest_below"`
	// VerticalGiveUp: a target right above or below that our walk to its floor did not reach is given
	// up at once, without a flight (combat fightWarrior). false = the old way (step aside, then flee).
	VerticalGiveUp bool `json:"vertical_give_up"`
	// EatFromLevel: a rest eats food only from this character level; below, it waits standing (health
	// came back from 52 to 100 % in 4 s at levels 6-7, series M of 2026-10-24). 1 = always eat.
	EatFromLevel uint32 `json:"eat_from_level"`
	// ExploreSafe: exploration probability (instead of Explore) for a borderline case expected to leave
	// at least ExploreSafeSlack % of health: such stays are cheap to try, and the cells of 3 fresh
	// attackers filled too slowly at 25 % (13 stays in 8 runs, 2026-10-24).
	ExploreSafe      float64 `json:"explore_safe"`
	ExploreSafeSlack float64 `json:"explore_safe_slack"`
	// ExploreMinSlack: a borderline case is explored only when the fight is expected to leave us more
	// than this % of health (an exploration at a 3 % margin against 4 fresh kobolds killed us, series
	// ES run 3). Where fleeing kills often (ExploreFleeDeath) it is explored anyway.
	ExploreMinSlack float64 `json:"explore_min_slack"`
	// QuestDefer: guidecheck passes over a quest whose learned failure rate at our level
	// (journal/learn/quests.json) reaches this, after QuestDeferMin tries; it is tried when nothing
	// else is left. 0 = off.
	QuestDefer    float64 `json:"quest_defer"`
	QuestDeferMin float64 `json:"quest_defer_min"`
}

// Defaults. Why these margins (benches of 2026-10-13):
//   - flee_margin 15: health we want left when the fight is expected to end; a fight can cost twice its
//     average, and a death (150 s, durability) costs far more than a flight (a kill lost).
//   - crowd_flee 3: with 5-6 kobolds of a pack (Fargodeep mine) the ones still on us at the start of
//     the run killed us twice; crowd_loss 30: three kobolds two levels below (19 %) made us flee at
//     92 % health and postponed the mine quest level after level (0 quests in 100 minutes).
//   - high_margin 10 per attacker 2+ levels above: two deaths to a level-10 Riverpaw Outrunner at
//     level 8, flights started at 44 and 20 %.
//   - chase_margin 10 per fresh attacker beyond the first: Prowler + Young Forest Bear at level 9,
//     flight started at 34 %, dead 8 s later.
func Defaults() Tuning {
	return Tuning{FleeMargin: 15, HighMargin: 10, ChaseMargin: 10, FleeDPS: true, FleeExposure: 6, FleeReserve: 5, CrowdFlee: 3, CrowdLoss: 30, Avoid: false,
		PostponeAfterFlights: 1, Explore: 0.25, Learn: true, MinFights: 6, DeathCostS: 180, FleeCostS: 90,
		FleeDeathPrior: 0.1, ExploreFleeDeath: 0.2,
		AddCost: 1.5, PullAdapt: false, PullSize: 2, PullSizeMax: 4, PullUpAfter: 2,
		WarriorRestBelow: 55, SafeRestBelow: 40, VerticalGiveUp: true, EatFromLevel: 10, ExploreSafe: 0.5, ExploreSafeSlack: 40, ExploreMinSlack: 15, QuestDefer: 0, QuestDeferMin: 4}
}

// cached reads a JSON file again when it changes (looked at every 10 s at most).
type cached[T any] struct {
	mu   sync.Mutex
	path string
	mod  time.Time
	seen time.Time
	v    T
	load func([]byte) (T, error)
}

func (c *cached[T]) get() T {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.seen) < 10*time.Second {
		return c.v
	}
	c.seen = time.Now()
	if st, err := os.Stat(c.path); err == nil && !st.ModTime().Equal(c.mod) {
		if b, err := os.ReadFile(c.path); err == nil {
			if v, err := c.load(b); err == nil {
				c.v, c.mod = v, st.ModTime()
			}
		}
	}
	return c.v
}

func learnDir() string {
	if d := os.Getenv("LEARN_DIR"); d != "" {
		return d
	}
	return "/learn"
}

// Source gives the current tuning ($LEARN_DIR/tuning.json over the defaults).
type Source struct{ c *cached[Tuning] }

func NewSource(dir string) *Source {
	if dir == "" {
		dir = learnDir()
	}
	return &Source{&cached[Tuning]{path: filepath.Join(dir, "tuning.json"), v: Defaults(), load: func(b []byte) (Tuning, error) {
		t := Defaults()
		err := json.Unmarshal(b, &t)
		return t, err
	}}}
}

func (s *Source) Get() Tuning { return s.c.get() }

// ---- learned borderline fights ----

// Cell counts the fights we stayed in, from a borderline decision, and the flights.
type Cell struct {
	N          float64 `json:"n"`
	Deaths     float64 `json:"deaths"`
	FleeN      float64 `json:"flee_n,omitempty"`
	FleeDeaths float64 `json:"flee_deaths,omitempty"`
}

// fleePriorN: weight of FleeDeathPrior, in flights.
const fleePriorN = 4.0

// FleeDeath is the death rate of the flights of a case (prior FleeDeathPrior).
func FleeDeath(f *Flee, key string, t Tuning) float64 {
	c := f.Cells[key]
	if !t.Learn || c == nil {
		return t.FleeDeathPrior
	}
	return (c.FleeDeaths + fleePriorN*t.FleeDeathPrior) / (c.FleeN + fleePriorN)
}

// Flee is journal/learn/flee.json.
type Flee struct {
	Until     string           `json:"until"`      // time of the last stay learned
	FleeUntil string           `json:"flee_until"` // time of the last flight learned (empty: the whole log)
	Cells     map[string]*Cell `json:"cells"`
}

// Key of a decision: fresh attackers (1, 2, 3, 4+) and health slack (effective health minus expected
// loss) in 10 % steps, from -30 to +60. 4+ apart since 2026-10-23: the stays learned against 3 held
// against 6 kobolds of a Fargodeep pack (two of the three deaths of the first QD60 run); its cells
// start empty, so the rule decides until they are learned.
func Key(fresh int, slack float64) string {
	b := int(math.Floor(slack/10)) * 10
	b = max(-30, min(60, b))
	return fmt.Sprintf("%d:%d", max(1, min(fresh, 4)), b)
}

// FleeTable gives the learned table ($LEARN_DIR/flee.json).
type FleeTable struct{ c *cached[*Flee] }

func NewFleeTable(dir string) *FleeTable {
	if dir == "" {
		dir = learnDir()
	}
	return &FleeTable{&cached[*Flee]{path: filepath.Join(dir, "flee.json"), v: &Flee{Cells: map[string]*Cell{}}, load: func(b []byte) (*Flee, error) {
		f := &Flee{}
		err := json.Unmarshal(b, f)
		if f.Cells == nil {
			f.Cells = map[string]*Cell{}
		}
		return f, err
	}}}
}

func (t *FleeTable) Get() *Flee { return t.c.get() }

// Decide returns whether to flee in a case the rule would flee from, from the learned death rates
// of staying (with one death in two fights as prior) and of fleeing, and whether the table decided
// (enough fights).
func Decide(f *Flee, key string, t Tuning) (flee, learned bool) {
	c := f.Cells[key]
	if !t.Learn || c == nil || c.N < t.MinFights {
		return true, false
	}
	p := (c.Deaths + 1) / (c.N + 2)
	return p*t.DeathCostS > t.FleeCostS+FleeDeath(f, key, t)*t.DeathCostS, true
}

// outcome reads the "flee outcome" lines of logs/combat.log (combat/flee.go).
var outcome = regexp.MustCompile(`^time=(\S+) .*msg="flee outcome" key=(\S+) stayed=(true|false) .*result=(\w+)`)

// Learn adds the outcomes of the stays logged after Until and of the flights logged after
// FleeUntil; returns how many.
func (f *Flee) Learn(r io.Reader) int {
	if f.Cells == nil {
		f.Cells = map[string]*Cell{}
	}
	n := 0
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		m := outcome.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		cell := func() *Cell {
			c := f.Cells[m[2]]
			if c == nil {
				c = &Cell{}
				f.Cells[m[2]] = c
			}
			return c
		}
		if m[3] == "false" {
			// The rule fled: died on the way or got away.
			if m[1] <= f.FleeUntil {
				continue
			}
			f.FleeUntil = m[1]
			if m[4] != "won" {
				c := cell()
				c.FleeN++
				if m[4] == "died" {
					c.FleeDeaths++
				}
				n++
			}
			continue
		}
		if m[1] <= f.Until {
			continue
		}
		f.Until = m[1]
		if m[4] == "fled" {
			continue // fled later in the fight: says nothing about staying to the end
		}
		c := cell()
		c.N++
		if m[4] == "died" {
			c.Deaths++
		}
		n++
	}
	return n
}

func LoadFlee(path string) (*Flee, error) {
	f := &Flee{Cells: map[string]*Cell{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, f); err != nil {
		return nil, err
	}
	if f.Cells == nil {
		f.Cells = map[string]*Cell{}
	}
	return f, nil
}

func (f *Flee) Save(path string) error {
	b, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Summary: "2:0 6/1 flee 4/2" per cell (fights, deaths; flights, deaths).
func (f *Flee) Summary() []string {
	var out []string
	ff := func(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }
	for k, c := range f.Cells {
		out = append(out, k+" "+ff(c.N)+"/"+ff(c.Deaths)+" flee "+ff(c.FleeN)+"/"+ff(c.FleeDeaths))
	}
	return out
}
