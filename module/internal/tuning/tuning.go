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

// Tuning: the defaults are the rules of commit eb70f0b.
type Tuning struct {
	FleeMargin  float64 `json:"flee_margin"`  // % of health kept after the expected loss
	HighMargin  float64 `json:"high_margin"`  // + per attacker 2+ levels above us
	ChaseMargin float64 `json:"chase_margin"` // + per fresh attacker beyond the first
	CrowdFlee   int     `json:"crowd_flee"`   // fresh attackers that make a crowd (0 = no crowd rule)
	CrowdLoss   float64 `json:"crowd_loss"`   // expected loss over which a crowd makes us flee
	Avoid       bool    `json:"avoid"`        // paths walk around monsters (move/avoid.go)
	// guidecheck: flights in a quest before it is postponed to the next level.
	PostponeAfterFlights int `json:"postpone_after_flights"`
	// Learning: in a borderline case (expected to survive, the rule says flee), stay and fight with
	// this probability, to measure the death rate; once a case has MinFights, decide from it.
	Explore    float64 `json:"explore"`
	Learn      bool    `json:"learn"`
	MinFights  float64 `json:"min_fights"`
	DeathCostS float64 `json:"death_cost_s"` // run to the corpse, rest, durability
	FleeCostS  float64 `json:"flee_cost_s"`  // run, rest, the quest or spot left
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
}

func Defaults() Tuning {
	return Tuning{FleeMargin: 15, HighMargin: 10, ChaseMargin: 10, CrowdFlee: 3, CrowdLoss: 30, Avoid: true,
		PostponeAfterFlights: 1, Explore: 0.25, Learn: true, MinFights: 6, DeathCostS: 180, FleeCostS: 90,
		AddCost: 1.5, PullAdapt: false, PullSize: 2, PullSizeMax: 4, PullUpAfter: 2}
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

// Cell counts the fights we stayed in, from a borderline decision.
type Cell struct {
	N      float64 `json:"n"`
	Deaths float64 `json:"deaths"`
}

// Flee is journal/learn/flee.json.
type Flee struct {
	Until string           `json:"until"` // time of the last log line learned
	Cells map[string]*Cell `json:"cells"`
}

// Key of a decision: fresh attackers (1, 2, 3+) and health slack (effective health minus expected
// loss) in 10 % steps, from -30 to +60.
func Key(fresh int, slack float64) string {
	b := int(math.Floor(slack/10)) * 10
	b = max(-30, min(60, b))
	return fmt.Sprintf("%d:%d", max(1, min(fresh, 3)), b)
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

// Decide returns whether to flee in a case the rule would flee from, from the learned death rate
// (with one death in two fights as prior), and whether the table decided (enough fights).
func Decide(f *Flee, key string, t Tuning) (flee, learned bool) {
	c := f.Cells[key]
	if !t.Learn || c == nil || c.N < t.MinFights {
		return true, false
	}
	p := (c.Deaths + 1) / (c.N + 2)
	return p*t.DeathCostS > t.FleeCostS, true
}

// outcome reads the "flee outcome" lines of logs/combat.log (combat/flee.go).
var outcome = regexp.MustCompile(`^time=(\S+) .*msg="flee outcome" key=(\S+) stayed=(true|false) .*result=(\w+)`)

// Learn adds the outcomes of the stays logged after Until; returns how many.
func (f *Flee) Learn(r io.Reader) int {
	if f.Cells == nil {
		f.Cells = map[string]*Cell{}
	}
	n := 0
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		m := outcome.FindStringSubmatch(sc.Text())
		if m == nil || m[1] <= f.Until {
			continue
		}
		f.Until = m[1]
		if m[3] != "true" || m[4] == "fled" {
			continue // fled (at once, or later in the fight): says nothing about staying to the end
		}
		c := f.Cells[m[2]]
		if c == nil {
			c = &Cell{}
			f.Cells[m[2]] = c
		}
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

// Summary: "2:0 6/1" per cell (fights, deaths).
func (f *Flee) Summary() []string {
	var out []string
	for k, c := range f.Cells {
		out = append(out, k+" "+strconv.FormatFloat(c.N, 'f', 0, 64)+"/"+strconv.FormatFloat(c.Deaths, 'f', 0, 64))
	}
	return out
}
