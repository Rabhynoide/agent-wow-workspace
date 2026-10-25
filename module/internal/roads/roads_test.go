package roads

import (
	"path/filepath"
	"testing"
)

func TestAlphaMaps(t *testing.T) {
	// 4-bit: low nibble first, ×17; the last row and column copied unless told not to.
	d := make([]byte, 2048)
	d[0] = 0xF1
	d[31] = 0x0F // pixel 62 = 15, pixel 63 = 0 -> fixed to 15
	a := alphaMap(d, false, false, false)
	if a[0] != 17 || a[1] != 255 || a[62] != 255 || a[63] != 255 {
		t.Fatalf("4-bit: %v %v %v %v", a[0], a[1], a[62], a[63])
	}
	if b := alphaMap(d, false, false, true); b[63] != 0 {
		t.Fatalf("4-bit without fix: %v", b[63])
	}
	// Compressed: 0x80|n fills n times the next byte, n copies the next n bytes.
	c := alphaMap([]byte{0x83, 9, 0x02, 4, 5, 0xFF, 7}, true, false, false)
	if c[0] != 9 || c[2] != 9 || c[3] != 4 || c[4] != 5 || c[5] != 7 || c[131] != 7 || c[132] != 0 {
		t.Fatalf("compressed: %v", c[:8])
	}
}

func TestGridRoundTrip(t *testing.T) {
	m := New()
	// Goldshire (-9460, 60): grid (32 - x/533.33, 32 - y/533.33).
	fx, fy := 32-(-9460.0)/gridSize, 32-60.0/gridSize
	gx, gy := int(fx), int(fy)
	m.Set(gx, gy, int((fx-float64(gx))*Res), int((fy-float64(gy))*Res))
	path := File(t.TempDir(), 0)
	if err := m.Write(path); err != nil {
		t.Fatal(err)
	}
	r, err := Load(filepath.Dir(path), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !r.On(-9460, 60) || r.On(-9460, 80) || r.On(-9440, 60) {
		t.Fatal("road cell not where it was set")
	}
	if !IsRoad(`Tileset\Elwynn\ElwynnCobblestoneBase.blp`) || IsRoad(`Tileset\Elwynn\ElwynnGrassBase.blp`) {
		t.Fatal("IsRoad")
	}
}
