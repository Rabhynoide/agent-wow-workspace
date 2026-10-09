package terrain

import (
	"os"
	"testing"

	"wowagent/internal/nav"
)

func TestCompareWithNavmesh(t *testing.T) {
	if _, err := os.Stat("../../../assets/maps"); err != nil {
		t.Skip("no maps")
	}
	tm := New("../../../assets/maps", 0)
	m, _ := nav.LoadMap("../../../assets/mmaps", 0)
	p, err := m.Path(nav.ToRecast(-8949.95, -132.49, 83.53), nav.ToRecast(-9462, 16, 58), nav.GroundOnly)
	if err != nil {
		t.Fatal(err)
	}
	var sum, worst float32
	n := 0
	for i := 1; i < len(p); i++ {
		for k := 0; k < 20; k++ {
			f := float32(k) / 20
			v := nav.V3{p[i-1][0] + f*(p[i][0]-p[i-1][0]), p[i-1][1] + f*(p[i][1]-p[i-1][1]), p[i-1][2] + f*(p[i][2]-p[i-1][2])}
			x, y, z := v.ToWoW()
			nz, ok := m.Height(v, nav.GroundOnly)
			th, ok2 := tm.Height(x, y)
			if !ok || !ok2 {
				continue
			}
			_ = z
			d := nz - th
			if d < 0 {
				d = -d
			}
			sum += d
			worst = max(worst, d)
			n++
		}
	}
	t.Logf("%d samples, mean |nav-terrain| %.2f, worst %.2f", n, sum/float32(n), worst)
}
