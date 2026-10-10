package classdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wowagent/internal/dbc"
)

const (
	classesDir = "../../../journal/classes"
	dbcDir     = "../../../assets/dbc"
)

var ids = map[string]uint32{"warrior": 1, "paladin": 2, "hunter": 3, "rogue": 4, "priest": 5, "deathknight": 6, "shaman": 7, "mage": 8, "warlock": 9, "druid": 11}

// TestClassFiles replays every class file the way progress.talents follows it (respec points skipped).
func TestClassFiles(t *testing.T) {
	if _, err := os.Stat(dbcDir + "/Talent.dbc"); err != nil {
		t.Skip("assets/dbc not available")
	}
	d, err := dbc.Load(dbcDir)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(classesDir, "*.yaml"))
	if len(files) == 0 {
		t.Skip("no class files")
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		class := ids[name]
		f, err := Load(classesDir, class)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		pts, err := Replay(d, class, f.Build())
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		t.Logf("%s (%s): %d points without respec", name, f.Spec, pts)
	}
}

func TestBuildSkipsRespecPoints(t *testing.T) {
	f := &File{TalentOrder: []Entry{
		{Talent: "A", Points: 2},
		{Talent: "B", Points: 3, RemovedAt: 25},
		{Talent: "A", Points: 1},
		{Talent: "B", Points: 3},
	}}
	got := f.Build()
	want := []Step{{"A", 2}, {"A", 3}, {"B", 3}}
	if len(got) != len(want) {
		t.Fatalf("build = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("build = %v, want %v", got, want)
		}
	}
}
