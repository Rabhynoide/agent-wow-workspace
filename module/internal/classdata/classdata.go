// Package classdata reads the class leveling files (journal/classes/<class>.yaml) and turns their
// talent order into the steps progress.talents follows.
package classdata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"wowagent/internal/dbc"
)

// Entry is one line of talent_order.
type Entry struct {
	Levels    string `yaml:"levels"`
	Tree      string `yaml:"tree"`
	Talent    string `yaml:"talent"`
	Points    int    `yaml:"points"`
	RemovedAt int    `yaml:"removed_at"`
}

// File is the part of a class file the modules use.
type File struct {
	Class       string  `yaml:"class"`
	Spec        string  `yaml:"recommended_leveling_spec"`
	TalentOrder []Entry `yaml:"talent_order"`
	Training    struct {
		Skip []string `yaml:"skip"`
	} `yaml:"training"`
}

// Step raises a talent to Rank.
type Step struct {
	Talent string
	Rank   int
}

// FileName is the file of a class id ("warrior.yaml"...).
func FileName(class uint32) string {
	names := map[uint32]string{1: "warrior", 2: "paladin", 3: "hunter", 4: "rogue", 5: "priest", 6: "deathknight", 7: "shaman",
		8: "mage", 9: "warlock", 11: "druid"}
	return names[class] + ".yaml"
}

// Load reads the class file of a class id from dir.
func Load(dir string, class uint32) (*File, error) {
	b, err := os.ReadFile(filepath.Join(dir, FileName(class)))
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", FileName(class), err)
	}
	return &f, nil
}

// Build turns the talent order into "raise to rank" steps. Points that a guide removes at a later
// respec (removed_at) are skipped: the character never takes them, so it never needs a paid respec
// and spends those points on the next talents instead (a guide usually takes them back later).
func (f *File) Build() []Step {
	rank := map[string]int{}
	var out []Step
	for _, e := range f.TalentOrder {
		if e.RemovedAt > 0 || e.Points <= 0 {
			continue
		}
		rank[e.Talent] += e.Points
		out = append(out, Step{Talent: e.Talent, Rank: rank[e.Talent]})
	}
	return out
}

// Skip reports whether the trainer spell name is in training.skip.
func (f *File) Skip(name string) bool {
	for _, n := range f.Training.Skip {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// Replay checks the steps with the server rules, one point per level from level 10: tier needs 5
// points per row in its tree, prerequisites need their rank, ranks stay under the maximum.
// It returns the points spent.
func Replay(d *dbc.Data, class uint32, steps []Step) (int, error) {
	rank := map[uint32]int{}
	spent := map[uint32]int{}
	total := 0
	for _, st := range steps {
		t := d.TalentBySpellName(class, st.Talent)
		if t == nil {
			return total, fmt.Errorf("unknown talent %q", st.Talent)
		}
		if st.Rank > t.MaxRank() {
			return total, fmt.Errorf("%s: rank %d > max %d", st.Talent, st.Rank, t.MaxRank())
		}
		for rank[t.ID] < st.Rank {
			if spent[t.Tab] < int(t.Row)*5 {
				return total, fmt.Errorf("%s (row %d) at level %d: only %d points in its tree", st.Talent, t.Row, 10+total, spent[t.Tab])
			}
			if t.DependsOn != 0 && rank[t.DependsOn] < int(t.DependsOnRank)+1 {
				return total, fmt.Errorf("%s needs talent %d rank %d", st.Talent, t.DependsOn, t.DependsOnRank+1)
			}
			rank[t.ID]++
			spent[t.Tab]++
			total++
		}
	}
	if total > 71 {
		return total, fmt.Errorf("build uses %d points (max 71)", total)
	}
	return total, nil
}
