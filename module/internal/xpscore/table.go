package xpscore

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cell sums the fights won against one monster, started with at least 90 % health, for one class and
// level gap (monster level - ours).
type Cell struct {
	N       float64 `json:"n"`        // fights, scaled down past keepFights (forget)
	LossSum float64 `json:"loss_sum"` // percent of our health
	SecSum  float64 `json:"sec_sum"`
}

// MobCell counts every fight against one monster type (the fight's target) at one level gap, won or
// lost, whatever the health at the start: the measured death risk, chained fights included.
type MobCell struct {
	N      float64 `json:"n"`
	Deaths float64 `json:"deaths"`
	// Chained fights: started below chainedBelow % health (attacked again before resting).
	Chained       float64 `json:"chained"`
	ChainedDeaths float64 `json:"chained_deaths"`
	// Fled: fights we ran away from (combat flees when the loss to come exceeds our health).
	Fled float64 `json:"fled,omitempty"`
}

// Table is the measured side of the model, kept in journal/learn/fights.json.
type Table struct {
	// Last fight line read (its time field), so a log is never counted twice.
	Until string `json:"until"`
	// Class id -> gap -> cell.
	Classes map[string]map[string]*Cell `json:"classes"`
	// Same as Until for Mobs (added later: the logs read before are read again once).
	MobsUntil string `json:"mobs_until"`
	// Class id -> monster entry -> gap -> cell.
	Mobs map[string]map[string]map[string]*MobCell `json:"mobs"`
}

// chainedBelow: a fight started below this health (percent) is a chained one.
const chainedBelow = 60

// keepFights: past this many fights in a cell, all its counts are scaled down to it, so the old
// fights weigh less and less (each new one counts 1, the past keepFights-1). The learning adds up runs
// of code that keeps changing: deaths from before the flights (7 Mangy Wolf deaths in 58 fights) still
// made the grind avoid wolves (user question, 2026-10-13).
const keepFights = 40

// forget scales the counts of the cells down to keepFights fights.
func (c *Cell) forget() {
	if c.N > keepFights {
		k := keepFights / c.N
		c.N, c.LossSum, c.SecSum = keepFights, c.LossSum*k, c.SecSum*k
	}
}

func (c *MobCell) forget() {
	if c.N > keepFights {
		k := keepFights / c.N
		c.N, c.Deaths, c.Chained, c.ChainedDeaths, c.Fled = keepFights, c.Deaths*k, c.Chained*k, c.ChainedDeaths*k, c.Fled*k
	}
}

// priorFights: weight of the model's death risk, in fights, against the measured deaths.
const priorFights = 10

// minFights below which a gap is extrapolated from its neighbours.
const minFights = 3

// Per level of gap, without measurements (from the warrior measurements of 2026-10-12: -1 → 0 → +1
// lost 4 %, 14 %, 26 %).
const (
	lossPerGap = 1.85
	secPerGap  = 1.15
)

// Default when a class has no fight at all: the human warrior measured at level 5 for gap 0.
var defaultCell = Cell{N: 1, LossSum: 14, SecSum: 12}

// Load reads a table; a missing file gives an empty one.
func Load(path string) (*Table, error) {
	t := &Table{Classes: map[string]map[string]*Cell{}, Mobs: map[string]map[string]map[string]*MobCell{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, t); err != nil {
		return nil, err
	}
	if t.Classes == nil {
		t.Classes = map[string]map[string]*Cell{}
	}
	if t.Mobs == nil {
		t.Mobs = map[string]map[string]map[string]*MobCell{}
	}
	// Tables learned before forget, or cells of an old run: scaled down at once.
	for _, cells := range t.Classes {
		for _, c := range cells {
			c.forget()
		}
	}
	for _, entries := range t.Mobs {
		for _, gaps := range entries {
			for _, c := range gaps {
				c.forget()
			}
		}
	}
	return t, nil
}

// Save writes the table.
func (t *Table) Save(path string) error {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Fight is the expected health lost (percent) and duration (seconds) of a fight against one
// monster at this gap.
func (t *Table) Fight(class uint32, gap int) (loss, secs float64) {
	var cells map[string]*Cell
	if t != nil {
		cells = t.Classes[strconv.Itoa(int(class))]
	}
	best, bestGap := (*Cell)(nil), 0
	for k, c := range cells {
		g, err := strconv.Atoi(k)
		if err != nil || c.N < minFights {
			continue
		}
		if best == nil || abs(g-gap) < abs(bestGap-gap) || (abs(g-gap) == abs(bestGap-gap) && c.N > best.N) {
			best, bestGap = c, g
		}
	}
	if best == nil {
		best, bestGap = &defaultCell, 0
	}
	d := float64(gap - bestGap)
	return best.LossSum / float64(best.N) * math.Pow(lossPerGap, d), best.SecSum / float64(best.N) * math.Pow(secPerGap, d)
}

var field = regexp.MustCompile(`(\w+)=("[^"]*"|\S+)`)

// Learn adds the fight lines of a combat log written after t.Until (t.MobsUntil for the death
// counts); it returns how many lines were added to either.
func (t *Table) Learn(r io.Reader) int {
	if t.Classes == nil {
		t.Classes = map[string]map[string]*Cell{}
	}
	if t.Mobs == nil {
		t.Mobs = map[string]map[string]map[string]*MobCell{}
	}
	added := 0
	last, lastMobs := t.Until, t.MobsUntil
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, "msg=fight ") {
			continue
		}
		f := map[string]string{}
		for _, m := range field.FindAllStringSubmatch(line, -1) {
			f[m[1]] = m[2]
		}
		newFight, newMob := f["time"] > t.Until, f["time"] > t.MobsUntil
		if (!newFight && !newMob) || f["class"] == "" {
			continue
		}
		start, _ := strconv.Atoi(f["hp_start"])
		end, _ := strconv.Atoi(f["hp_end"])
		secs, _ := strconv.Atoi(f["s"])
		lv, _ := strconv.Atoi(f["level"])
		tl, _ := strconv.Atoi(f["target_level"])
		if tl == 0 {
			continue
		}
		key := strconv.Itoa(tl - lv)
		counted := false
		// "switched": the same fight goes on against another attacker (combat/focus.go), not a fight.
		if newMob && f["target"] != "" && f["target"] != "0" && f["result"] != "switched" {
			lastMobs = max(lastMobs, f["time"])
			c := t.mobCell(f["class"], f["target"], key)
			c.N++
			died := f["result"] == "died"
			if died {
				c.Deaths++
			}
			if f["result"] == "fled" {
				c.Fled++
			}
			if start < chainedBelow {
				c.Chained++
				if died {
					c.ChainedDeaths++
				}
			}
			c.forget()
			counted = true
		}
		if newFight {
			last = max(last, f["time"])
			if f["result"] == "won" && f["attackers_max"] == "1" && start >= 90 && secs > 0 {
				cells := t.Classes[f["class"]]
				if cells == nil {
					cells = map[string]*Cell{}
					t.Classes[f["class"]] = cells
				}
				c := cells[key]
				if c == nil {
					c = &Cell{}
					cells[key] = c
				}
				c.N++
				c.LossSum += float64(max(start-end, 0))
				c.SecSum += float64(secs)
				c.forget()
				counted = true
			}
		}
		if counted {
			added++
		}
	}
	t.Until, t.MobsUntil = last, lastMobs
	return added
}

func (t *Table) mobCell(class, entry, gap string) *MobCell {
	entries := t.Mobs[class]
	if entries == nil {
		entries = map[string]map[string]*MobCell{}
		t.Mobs[class] = entries
	}
	gaps := entries[entry]
	if gaps == nil {
		gaps = map[string]*MobCell{}
		entries[entry] = gaps
	}
	c := gaps[gap]
	if c == nil {
		c = &MobCell{}
		gaps[gap] = c
	}
	return c
}

// Measured is the fights and deaths counted against a monster type at a level gap (zero cell when
// none).
func (t *Table) Measured(class, entry uint32, gap int) MobCell {
	if t == nil {
		return MobCell{}
	}
	if c := t.Mobs[strconv.Itoa(int(class))][strconv.Itoa(int(entry))][strconv.Itoa(gap)]; c != nil {
		return *c
	}
	return MobCell{}
}

// DeathRisk blends the model's death risk per fight with the measured one: the model counts as
// priorFights fights.
func (t *Table) DeathRisk(class, entry uint32, gap int, model float64) float64 {
	c := t.Measured(class, entry, gap)
	return (float64(c.Deaths) + priorFights*model) / float64(c.N+priorFights)
}

// Summary lists the measured gaps of a class ("gap: n fights, loss %, seconds").
func (t *Table) Summary(class uint32) []string {
	cells := t.Classes[strconv.Itoa(int(class))]
	var gaps []int
	for k := range cells {
		if g, err := strconv.Atoi(k); err == nil {
			gaps = append(gaps, g)
		}
	}
	sort.Ints(gaps)
	var out []string
	for _, g := range gaps {
		c := cells[strconv.Itoa(g)]
		out = append(out, strconv.Itoa(g)+": "+strconv.FormatFloat(c.N, 'f', 0, 64)+" fights, "+
			strconv.FormatFloat(c.LossSum/float64(c.N), 'f', 0, 64)+" % health, "+
			strconv.FormatFloat(c.SecSum/float64(c.N), 'f', 0, 64)+" s")
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// File is a table file read again when it changes (the modules read journal/learn/fights.json,
// which level.sh updates).
type File struct {
	path string
	mu   sync.Mutex
	mod  time.Time
	t    *Table
}

// NewFile watches path; an unreadable file gives the last table read (empty at first).
func NewFile(path string) *File { return &File{path: path, t: &Table{}} }

// Get returns the table, read again if the file changed.
func (f *File) Get() *Table {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, err := os.Stat(f.path); err == nil && !st.ModTime().Equal(f.mod) {
		if t, err := Load(f.path); err == nil {
			f.t, f.mod = t, st.ModTime()
		}
	}
	return f.t
}

// LearnedFile is the table of the modules: $LEARN_DIR/fights.json (journal/learn mounted at /learn).
func LearnedFile() *File {
	dir := os.Getenv("LEARN_DIR")
	if dir == "" {
		dir = "/learn"
	}
	return NewFile(filepath.Join(dir, "fights.json"))
}
