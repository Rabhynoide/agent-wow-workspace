package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"wowagent/internal/tuning"
)

// Learned quest outcomes (2026-10-23): the Fargodeep mine quests failed at level 6 in 7 runs of 8
// (62: 90 % of 60 tries, 47: 84 %), each try costing 4-13 minutes before the postponement, while
// they take 2-6 minutes at level 7. A quest that fails this often at our level is passed over like
// a postponed one (nextBatch), and tried only when nothing else is left.

// questCell counts the tries of a quest at a character level, and the failures (too dangerous,
// fled, dead).
type questCell struct {
	N      float64 `json:"n"`
	Failed float64 `json:"failed"`
}

// questTable is journal/learn/quests.json, learned from the logs/level_*.log of level.sh.
type questTable struct {
	Until string                `json:"until"` // time of the last line learned
	Cells map[string]*questCell `json:"cells"` // "quest:level"
}

// questKeep: beyond this many tries, a cell's counts are scaled back to it (old tries weigh less).
const questKeep = 40

const questsFile = "quests.json"

var (
	levelLine = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) \[[^\]]*\] L(\d+) (quests|grind)`)
	doneLine  = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) \[[^\]]*\]   done \[([\d ]+)\]`)
	failLine  = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) \[[^\]]*\]   (?:DEATH on (\d+) |(\d+) (?:postponed to level|fled \(|too dangerous \())`)
)

func loadQuests(path string) (*questTable, error) {
	t := &questTable{Cells: map[string]*questCell{}}
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
	if t.Cells == nil {
		t.Cells = map[string]*questCell{}
	}
	return t, nil
}

func (t *questTable) save(path string) error {
	b, err := json.MarshalIndent(t, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (t *questTable) add(q, level uint32, failed bool) {
	k := fmt.Sprintf("%d:%d", q, level)
	c := t.Cells[k]
	if c == nil {
		c = &questCell{}
		t.Cells[k] = c
	}
	c.N++
	if failed {
		c.Failed++
	}
	if c.N > questKeep {
		c.Failed *= questKeep / c.N
		c.N = questKeep
	}
}

// learn adds the outcomes of one level log written after t.Until; returns the last time seen and
// how many outcomes. The level comes from the batch line before (read from the start of the file).
func (t *questTable) learn(r io.Reader) (last string, n int) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var level uint32
	for sc.Scan() {
		l := sc.Text()
		if m := levelLine.FindStringSubmatch(l); m != nil {
			v, _ := strconv.Atoi(m[2])
			level = uint32(v)
			continue
		}
		if level == 0 {
			continue
		}
		if m := doneLine.FindStringSubmatch(l); m != nil && m[1] > t.Until {
			for _, f := range strings.Fields(m[2]) {
				if q, err := strconv.Atoi(f); err == nil {
					t.add(uint32(q), level, false)
					n++
				}
			}
			last = max(last, m[1])
			continue
		}
		// Too dangerous, fled or dead (not the "postponed 4 min": monsters away, not a failure).
		if m := failLine.FindStringSubmatch(l); m != nil && m[1] > t.Until {
			q, _ := strconv.Atoi(m[2] + m[3])
			t.add(uint32(q), level, true)
			n++
			last = max(last, m[1])
		}
	}
	return last, n
}

// learnQuests adds the new lines of all level logs to journal/learn/quests.json.
func learnQuests() (*questTable, int, error) {
	path := filepath.Join(learnPath, questsFile)
	t, err := loadQuests(path)
	if err != nil {
		return nil, 0, err
	}
	logs, _ := filepath.Glob(filepath.Join("..", "logs", "level_*.log"))
	until, total := t.Until, 0
	for _, p := range logs {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		last, n := t.learn(f)
		f.Close()
		until, total = max(until, last), total+n
	}
	if total == 0 {
		return t, 0, nil
	}
	t.Until = until
	return t, total, t.save(path)
}

// hard lists the quests whose learned failure rate at this level, (failed+1)/(n+2), reaches rate
// after at least minTries tries; rate 0 = none.
func (t *questTable) hard(level uint32, rate, minTries float64) map[uint32]bool {
	out := map[uint32]bool{}
	if t == nil || rate <= 0 {
		return out
	}
	suffix := fmt.Sprintf(":%d", level)
	for k, c := range t.Cells {
		if !strings.HasSuffix(k, suffix) || c.N < minTries {
			continue
		}
		if (c.Failed+1)/(c.N+2) >= rate {
			q, _ := strconv.Atoi(strings.TrimSuffix(k, suffix))
			out[uint32(q)] = true
		}
	}
	return out
}

// printLearnedQuests learns the level logs and lists, per level, the quests nextBatch passes over.
func printLearnedQuests() int {
	t, n, err := learnQuests()
	if err != nil {
		fmt.Println("learned quests:", err)
		return 1
	}
	tune := tuning.NewSource(learnPath).Get()
	rate, minTries := tune.QuestDefer, tune.QuestDeferMin
	if rate <= 0 {
		rate = 0.6
	}
	fmt.Printf("learned %d quest outcomes, %d cells, until %s; passed over at a failure rate >= %.2f after %.0f tries:\n", n, len(t.Cells), t.Until, rate, minTries)
	for level := uint32(1); level <= 80; level++ {
		h := t.hard(level, rate, minTries)
		if len(h) == 0 {
			continue
		}
		var parts []string
		for _, q := range keys(h) {
			c := t.Cells[fmt.Sprintf("%d:%d", q, level)]
			parts = append(parts, fmt.Sprintf("%d (%.0f/%.0f)", q, c.Failed, c.N))
		}
		fmt.Printf("  L%d: %s\n", level, strings.Join(parts, ", "))
	}
	return 0
}
