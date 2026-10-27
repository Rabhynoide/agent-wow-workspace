package danger

import "testing"

func TestField(t *testing.T) {
	f := &Field{Src: []Source{{X: 0, Y: 0, Z: 0, R: 15, Halo: 10, W: 1}, {X: 0, Y: 0, Z: 20, R: 15, Halo: 10, W: 1}}}
	if d := f.At(5, 0, 0); d != 1 {
		t.Fatalf("inside: %v", d)
	}
	if d := f.At(20, 0, 0); d < 0.49 || d > 0.51 {
		t.Fatalf("halo: %v", d)
	}
	if d := f.At(30, 0, 0); d != 0 {
		t.Fatalf("outside: %v", d)
	}
	// Through the camp costs more than along its edge.
	through := f.Along([][3]float32{{-40, 0, 0}, {40, 0, 0}})
	edge := f.Along([][3]float32{{-40, 22, 0}, {40, 22, 0}})
	if !(through > 3*edge && edge > 0) {
		t.Fatalf("through %v edge %v", through, edge)
	}
}
