package vmap

import (
	"os"
	"testing"
	"time"
)

const dir = "../../../assets/vmaps"

func open(t *testing.T) *Tree {
	if _, err := os.Stat(dir + "/000.vmtree"); err != nil {
		t.Skip("assets/vmaps not available")
	}
	tr, err := Open(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// TestFloorsUnderCreatures uses creature spawns (placed on floors by the server data) as references:
// two floors of the Goldshire inn, the Northshire abbey, the Stormwind cathedral.
func TestFloorsUnderCreatures(t *testing.T) {
	tr := open(t)
	cases := []struct {
		name    string
		x, y, z float32
	}{
		{"Innkeeper Farley (inn ground floor)", -9462.66, 16.19, 57.05},
		{"Brog Hamfist (inn ground floor)", -9465.29, 9.63, 57.15},
		{"Keryn Sylvius (inn first floor)", -9465.79, 12.65, 63.9},
		{"Michelle Belle (inn first floor)", -9456.81, 29.36, 63.9},
		{"Marshal McBride (abbey)", -8902.59, -162.61, 82.02},
		{"Brother Paxton (abbey)", -8874.87, -186.33, 82.12},
		{"Archbishop Benedictus (cathedral)", -8523, 848.78, 106.7},
	}
	for _, c := range cases {
		start := time.Now()
		h, ok := tr.Height(c.x, c.y, c.z+0.5)
		if !ok || h > c.z+0.3 || h < c.z-0.6 {
			t.Errorf("%s: floor %.2f (found %v), spawn z %.2f", c.name, h, ok, c.z)
			continue
		}
		t.Logf("%s: floor %.2f, spawn z %.2f (%v)", c.name, h, c.z, time.Since(start))
	}
	// Under the inn's first floor, from the ground floor: the ground floor, not the upper one.
	if h, ok := tr.Height(-9465.79, 12.65, 57.5); !ok || h > 58 {
		t.Errorf("inn ground floor under the first floor: %.2f %v", h, ok)
	}
}

func TestLineOfSight(t *testing.T) {
	tr := open(t)
	// Inside the inn, same room.
	if !tr.LineOfSight([3]float32{-9462.66, 16.19, 58.5}, [3]float32{-9465.29, 9.63, 58.5}) {
		t.Error("two NPCs of the inn's ground floor should see each other")
	}
	// From the inn's ground floor to its first floor: the ceiling is in between.
	if tr.LineOfSight([3]float32{-9465.29, 9.63, 58.5}, [3]float32{-9465.79, 12.65, 65.4}) {
		t.Error("the inn's ceiling should block the line of sight")
	}
}
