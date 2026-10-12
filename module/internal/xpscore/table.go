package xpscore

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Cell sums the fights won against one monster, started with at least 90 % health, for one class and
// level gap (monster level - ours).
type Cell struct {
	N       int     `json:"n"`
	LossSum float64 `json:"loss_sum"` // percent of our health
	SecSum  float64 `json:"sec_sum"`
}

// Table is the measured side of the model, kept in journal/learn/fights.json.
type Table struct {
	// Last fight line read (its time field), so a log is never counted twice.
	Until string `json:"until"`
	// Class id -> gap -> cell.
	Classes map[string]map[string]*Cell `json:"classes"`
}

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
	t := &Table{Classes: map[string]map[string]*Cell{}}
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

// Learn adds the fight lines of a combat log written after t.Until; it returns how many were added.
func (t *Table) Learn(r io.Reader) int {
	added := 0
	last := t.Until
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
		if f["time"] <= t.Until || f["class"] == "" {
			continue
		}
		last = max(last, f["time"])
		start, _ := strconv.Atoi(f["hp_start"])
		end, _ := strconv.Atoi(f["hp_end"])
		secs, _ := strconv.Atoi(f["s"])
		lv, _ := strconv.Atoi(f["level"])
		tl, _ := strconv.Atoi(f["target_level"])
		if f["result"] != "won" || f["attackers_max"] != "1" || start < 90 || tl == 0 || secs == 0 {
			continue
		}
		key := strconv.Itoa(tl - lv)
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
		added++
	}
	t.Until = last
	return added
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
		out = append(out, strconv.Itoa(g)+": "+strconv.Itoa(c.N)+" fights, "+
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
