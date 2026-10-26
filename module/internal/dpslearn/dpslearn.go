// Package dpslearn learns the damage per second each monster deals us and the damage we deal, from
// the combat log lines "damage taken" and "damage dealt" (combat/encounter.go), into
// journal/learn/dps.json (idea of the user, 2026-10-26). The pull choice and the flee rules use it
// before anything is measured in a fight.
//
// The character changes: a monster's damage is kept before our armor (Unit::CalcArmorReducedDamage
// undone) and the armor of the moment is applied back; our own damage is kept by class and level, and
// every value is an average that forgets (an observation never weighs less than 1/(MaxWeight+1)), so a new
// weapon or talents show after a few fights.
package dpslearn

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// MaxWeight: the weight of what was learned is capped there (the newest fight weighs at least 9 %:
// 85 % of a change of weapon shows after 20 fights).
const MaxWeight = 10

// Rate is a learned damage per second.
type Rate struct {
	DPS float64 `json:"dps"`
	N   float64 `json:"n"`
}

func (r *Rate) add(x float64) {
	r.DPS = (r.DPS*r.N + x) / (r.N + 1)
	r.N = min(r.N+1, MaxWeight)
}

// Table: Mobs by "entry:level" (damage before our armor), Ours by "class:level".
type Table struct {
	Mobs map[string]*Rate `json:"mobs"`
	Ours map[string]*Rate `json:"ours"`
}

func New() *Table { return &Table{Mobs: map[string]*Rate{}, Ours: map[string]*Rate{}} }

// ArmorReduction is the share of a physical blow of a monster of this level our armor removes
// (Unit::CalcArmorReducedDamage, 3.3.5), at most 75 %.
func ArmorReduction(armor, attackerLevel uint32) float64 {
	a := float64(armor)
	l := float64(attackerLevel)
	k := 400 + 85*l
	if attackerLevel >= 60 {
		k = 467.5*l - 22167.5
	}
	if a <= 0 || k <= 0 {
		return 0
	}
	return min(a/(a+k), 0.75)
}

func mobKey(entry, level uint32) string { return fmt.Sprintf("%d:%d", entry, level) }
func ourKey(class, level uint32) string { return fmt.Sprintf("%d:%d", class, level) }

// AddTaken learns one monster's damage per second against our armor.
func (t *Table) AddTaken(entry, level, armor uint32, dps float64) {
	raw := dps / (1 - ArmorReduction(armor, level))
	k := mobKey(entry, level)
	if t.Mobs[k] == nil {
		t.Mobs[k] = &Rate{}
	}
	t.Mobs[k].add(raw)
}

// AddDealt learns our damage per second.
func (t *Table) AddDealt(class, level uint32, dps float64) {
	k := ourKey(class, level)
	if t.Ours[k] == nil {
		t.Ours[k] = &Rate{}
	}
	t.Ours[k].add(dps)
}

// Taken predicts the damage per second a monster deals us with this armor (ok false: never met at
// this level; the nearest level of the same monster is used, scaled 10 % a level).
func (t *Table) Taken(entry, level, armor uint32) (float64, bool) {
	if t == nil {
		return 0, false
	}
	if r := t.Mobs[mobKey(entry, level)]; r != nil {
		return r.DPS * (1 - ArmorReduction(armor, level)), true
	}
	for d := uint32(1); d <= 3; d++ {
		for _, l := range []uint32{level - d, level + d} {
			if r := t.Mobs[mobKey(entry, l)]; r != nil && l > 0 {
				f := 1 + 0.1*(float64(level)-float64(l))
				return r.DPS * f * (1 - ArmorReduction(armor, level)), true
			}
		}
	}
	return 0, false
}

// Dealt predicts our damage per second (the nearest learned level below, else above).
func (t *Table) Dealt(class, level uint32) (float64, bool) {
	if t == nil {
		return 0, false
	}
	for d := uint32(0); d <= 5; d++ {
		if r := t.Ours[ourKey(class, level-d)]; r != nil && level > d {
			return r.DPS, true
		}
		if r := t.Ours[ourKey(class, level+d)]; r != nil && d > 0 {
			return r.DPS, true
		}
	}
	return 0, false
}

var (
	field    = regexp.MustCompile(`(\w+)=("[^"]*"|\S+)`)
	takenMsg = regexp.MustCompile(`msg="damage taken"`)
	dealtMsg = regexp.MustCompile(`msg="damage dealt"`)
)

// Learn reads combat log lines (in time order) into the table; minimum fight length and hits are
// checked by the writer. It returns the observations read.
func (t *Table) Learn(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		taken := takenMsg.MatchString(l)
		dealt := !taken && dealtMsg.MatchString(l)
		if !taken && !dealt {
			continue
		}
		kv := map[string]string{}
		for _, m := range field.FindAllStringSubmatch(l, -1) {
			kv[m[1]] = m[2]
		}
		u := func(k string) uint32 { v, _ := strconv.ParseUint(kv[k], 10, 32); return uint32(v) }
		dps, err := strconv.ParseFloat(kv["dps"], 64)
		if err != nil || dps <= 0 {
			continue
		}
		if taken {
			t.AddTaken(u("entry"), u("level"), u("armor"), dps)
		} else {
			t.AddDealt(u("class"), u("level"), dps)
		}
		n++
	}
	return n, sc.Err()
}

func (t *Table) Save(path string) error {
	b, err := json.MarshalIndent(t, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// File is the table of the modules ($LEARN_DIR/dps.json), read again when it changes.
type File struct {
	mu    sync.Mutex
	path  string
	mod   time.Time
	check time.Time
	t     *Table
}

func NewFile(dir string) *File {
	if dir == "" {
		dir = os.Getenv("LEARN_DIR")
	}
	if dir == "" {
		dir = "/learn"
	}
	return &File{path: filepath.Join(dir, "dps.json")}
}

// Get returns the table (nil when there is none yet).
func (f *File) Get() *Table {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if time.Since(f.check) < 10*time.Second && f.t != nil {
		return f.t
	}
	f.check = time.Now()
	st, err := os.Stat(f.path)
	if err != nil {
		return f.t
	}
	if f.t != nil && !st.ModTime().After(f.mod) {
		return f.t
	}
	b, err := os.ReadFile(f.path)
	if err != nil {
		return f.t
	}
	t := New()
	if json.Unmarshal(b, t) == nil {
		f.t, f.mod = t, st.ModTime()
	}
	return f.t
}
