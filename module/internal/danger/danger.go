// Package danger is a danger map: how risky it is to stand at (or walk through) a point, from the
// monsters around it. Each monster is a source: its aggro disc (it attacks us there) and, around
// it, the server's call for help (a monster chasing us calls its friends within 10 yards every 3 s:
// a flight that runs past a camp brings it along). The field is evaluated on demand from the
// sources (a few hundred around us), so it needs no grid; a grid is only for display.
//
// Layers (the caller builds the sources): live monsters in sight (where they stand), spawn points
// out of sight (from the database: their camps beyond the ~100 yards the server shows), places we
// learned to be deadly. Monsters engaged with us are not sources: they follow us anyway.
package danger

import "math"

// Source is one monster (or one learned place): a disc of radius R (yards) where the danger is
// full, fading to nothing Halo yards beyond it.
type Source struct {
	X, Y, Z float32
	R       float32
	Halo    float32
	W       float32 // weight: 1 for a monster of our level
}

// Floor is the height difference (yards) beyond which a source does not count (another floor of a
// mine, a ledge): the server's aggro stops at 3 yards of height (CREATURE_Z_ATTACK_RANGE), the call
// for help is 3D within 10 yards.
const Floor = 6

// Field is a set of sources.
type Field struct {
	Src []Source
}

// At is the danger at a point: the sum of the sources' weights, full inside their disc, fading
// linearly over their halo.
func (f *Field) At(x, y, z float32) float32 {
	var sum float32
	for _, s := range f.Src {
		if dz := s.Z - z; dz > Floor || dz < -Floor {
			continue
		}
		d := float32(math.Hypot(float64(s.X-x), float64(s.Y-y)))
		switch {
		case d <= s.R:
			sum += s.W
		case s.Halo > 0 && d < s.R+s.Halo:
			sum += s.W * (1 - (d-s.R)/s.Halo)
		}
	}
	return sum
}

// Step is the distance between two samples of a path.
const Step = 3

// Along is the danger met along a path: the sum over samples every Step yards of At times the
// yards walked, so a long walk through a camp costs more than brushing its edge.
func (f *Field) Along(path [][3]float32) float32 {
	var cost float32
	for i := 1; i < len(path); i++ {
		a, b := path[i-1], path[i]
		l := float32(math.Hypot(float64(b[0]-a[0]), float64(b[1]-a[1])))
		n := int(l/Step) + 1
		for k := 1; k <= n; k++ {
			t := float32(k) / float32(n)
			cost += f.At(a[0]+(b[0]-a[0])*t, a[1]+(b[1]-a[1])*t, a[2]+(b[2]-a[2])*t) * l / float32(n)
		}
	}
	return cost
}

// Weight of a monster from the level difference (theirs − ours): a level above hits about 30 %
// harder and lasts longer; elites count for three.
func Weight(levelDiff int, elite bool) float32 {
	w := float32(math.Pow(1.3, float64(levelDiff)))
	if elite {
		w *= 3
	}
	return w
}
