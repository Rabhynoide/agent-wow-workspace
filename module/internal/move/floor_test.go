package move

import (
	"math"
	"os"
	"testing"

	"wowagent/internal/terrain"
	"wowagent/internal/vmap"
)

func TestFloorLikeTheServer(t *testing.T) {
	if _, err := os.Stat("../../../assets/vmaps/000.vmtree"); err != nil {
		t.Skip("assets not available")
	}
	tm := terrain.New("../../../assets/maps", 0)
	vm, err := vmap.Open("../../../assets/vmaps", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Inside the Goldshire inn: the wooden floor (vmap), on both storeys.
	for _, c := range []struct{ x, y, z, want float32 }{
		{-9462.66, 16.19, 57.3, 56.96},
		{-9465.79, 12.65, 64.1, 63.82},
	} {
		if h, ok := floor(c.x, c.y, c.z, tm, vm); !ok || math.Abs(float64(h-c.want)) > 0.2 {
			t.Errorf("inn (%.0f,%.0f,%.1f): floor %.2f %v, want %.2f", c.x, c.y, c.z, h, ok, c.want)
		}
	}
	// Outdoors on open ground the terrain height wins (no model there).
	x, y := float32(-9520), float32(60)
	th, _ := tm.Height(x, y)
	if h, ok := floor(x, y, th+0.4, tm, vm); !ok || math.Abs(float64(h-th)) > 0.05 {
		t.Errorf("open ground: floor %.2f %v, terrain %.2f", h, ok, th)
	}
}
