package travel

import (
	"math"

	"wowagent/internal/dbc"
)

// Boats and zeppelins (MO_TRANSPORT game objects) follow a TaxiPathNode path in a fixed period. The
// server sends the period (GAMEOBJECT_LEVEL) and the progress into it (UPDATEFLAG_TRANSPORT); when
// they stop at a dock and when they change maps come from TransportMgr::GeneratePath, ported here
// (times only).

// keyFrame is a node of a transport path with its timing (ms into the period).
type keyFrame struct {
	node                           dbc.PathNode
	teleport                       bool
	distFromPrev, nextDistFromPrev float64
	distSinceStop, distUntilStop   float64
	timeTo                         float64
	arrive, depart                 uint32
}

func (k *keyFrame) stop() bool { return k.node.Flags == 2 }

type vec3 struct{ x, y, z float64 }

func (a vec3) sub(b vec3) vec3    { return vec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func (a vec3) length() float64    { return math.Sqrt(a.x*a.x + a.y*a.y + a.z*a.z) }
func nodePos(n dbc.PathNode) vec3 { return vec3{float64(n.X), float64(n.Y), float64(n.Z)} }

// catmull is Movement::Spline in ModeCatmullrom, not cyclic, initial orientation 0: lengths only.
type catmull struct {
	points  []vec3
	lengths []float64
}

var catmullCoeffs = [4][4]float64{
	{-0.5, 1.5, -1.5, 0.5},
	{1, -2.5, 2, -0.5},
	{-0.5, 0, 0.5, 0},
	{0, 1, 0, 0},
}

func newCatmull(controls []vec3) *catmull {
	n := len(controls)
	pts := make([]vec3, n+2)
	pts[0] = controls[0].sub(vec3{1, 0, 0}) // controls[0] - (cos 0, sin 0, 0)
	copy(pts[1:], controls)
	pts[n+1] = controls[n-1]
	c := &catmull{points: pts, lengths: make([]float64, n+1)}
	// index_lo = 1, index_hi = n; lengths[i+1] = lengths[i] + SegLengthCatmullRom(i)
	var l float64
	for i := 1; i < n; i++ {
		l += c.segLength(i)
		c.lengths[i+1] = l
	}
	return c
}

func evaluate(p []vec3, t float64) vec3 {
	tv := [4]float64{t * t * t, t * t, t, 1}
	var out vec3
	for i := 0; i < 4; i++ {
		w := tv[0]*catmullCoeffs[0][i] + tv[1]*catmullCoeffs[1][i] + tv[2]*catmullCoeffs[2][i] + tv[3]*catmullCoeffs[3][i]
		out.x += w * p[i].x
		out.y += w * p[i].y
		out.z += w * p[i].z
	}
	return out
}

// segLength: SegLengthCatmullRom, STEPS_PER_SEGMENT = 3.
func (c *catmull) segLength(i int) float64 {
	p := c.points[i-1 : i+3]
	cur := p[1]
	var l float64
	for s := 1; s <= 3; s++ {
		next := evaluate(p, float64(float32(s)/3))
		l += next.sub(cur).length()
		cur = next
	}
	return l
}

func (c *catmull) length(a, b int) float64 { return c.lengths[b] - c.lengths[a] }

// keyFrames ports TransportMgr::GeneratePath: the frames of a transport path and its period (ms).
func keyFrames(path []dbc.PathNode, speed, accel float64) ([]keyFrame, uint32) {
	if len(path) < 4 || speed <= 0 || accel <= 0 {
		return nil, 0
	}
	var kf []keyFrame
	var spl []vec3
	mapChange := false
	for i := range path {
		if mapChange {
			mapChange = false
			continue
		}
		if i != len(path)-1 && (path[i].Flags&1 != 0 || path[i].Map != path[i+1].Map) {
			if len(kf) > 0 {
				kf[len(kf)-1].teleport = true
			}
			mapChange = true
			continue
		}
		kf = append(kf, keyFrame{node: path[i]})
		spl = append(spl, nodePos(path[i]))
	}
	if len(spl) >= 2 {
		spl, kf = spl[1:len(spl)-1], kf[1:len(kf)-1]
	}
	n := len(kf)
	if n < 2 {
		return nil, 0
	}
	kf[n-1].teleport = true
	accelDist := 0.5 * speed * speed / accel
	firstStop, lastStop := -1, -1
	kf[0].distFromPrev = 0
	if kf[0].stop() {
		firstStop, lastStop = 0, 0
	}
	start := 0
	for i := 1; i < n; i++ {
		if kf[i-1].teleport || i+1 == n {
			extra := 1
			if kf[i-1].teleport {
				extra = 0
			}
			sp := newCatmull(spl[start : i+extra])
			for j := start; j < i+extra; j++ {
				kf[j].distFromPrev = sp.length(j-start, j+1-start)
				if j > 0 {
					kf[j-1].nextDistFromPrev = kf[j].distFromPrev
				}
			}
			if kf[i-1].teleport {
				kf[i].distFromPrev = 0
				kf[i-1].nextDistFromPrev = 0
			}
			start = i
		}
		if kf[i].stop() {
			if firstStop == -1 {
				firstStop = i
			}
			lastStop = i
		}
	}
	kf[n-1].nextDistFromPrev = kf[0].distFromPrev
	if firstStop == -1 || lastStop == -1 {
		firstStop, lastStop = 0, 0
	}
	tmp := 0.0
	for i := 0; i < n; i++ {
		j := (i + lastStop) % n
		if kf[j].stop() || j == lastStop {
			tmp = 0
		} else {
			tmp += kf[j].distFromPrev
		}
		kf[j].distSinceStop = tmp
	}
	tmp = 0
	for i := n - 1; i >= 0; i-- {
		j := (i + firstStop) % n
		tmp += kf[(j+1)%n].distFromPrev
		kf[j].distUntilStop = tmp
		if kf[j].stop() || j == firstStop {
			tmp = 0
		}
	}
	for i := range kf {
		k := &kf[i]
		total := k.distSinceStop + k.distUntilStop
		switch {
		case total < 2*accelDist:
			if k.distSinceStop < k.distUntilStop {
				seg := 2 * math.Sqrt((k.distUntilStop+k.distSinceStop)/accel)
				k.timeTo = seg - math.Sqrt(2*k.distSinceStop/accel)
			} else {
				k.timeTo = math.Sqrt(2 * k.distUntilStop / accel)
			}
		case k.distSinceStop < accelDist:
			seg := (k.distUntilStop+k.distSinceStop)/speed + speed/accel
			k.timeTo = seg - math.Sqrt(2*k.distSinceStop/accel)
		case k.distUntilStop < accelDist:
			k.timeTo = math.Sqrt(2 * k.distUntilStop / accel)
		default:
			k.timeTo = k.distUntilStop/speed + 0.5*speed/accel
		}
	}
	// The server keeps these in float.
	cur := float32(0)
	if kf[0].stop() {
		cur = float32(kf[0].node.Delay)
		kf[0].depart = uint32(cur * 1000)
	}
	for i := 1; i < n; i++ {
		cur += float32(kf[i-1].timeTo)
		if kf[i].stop() {
			kf[i].arrive = uint32(cur * 1000)
			cur += float32(kf[i].node.Delay)
			kf[i].depart = uint32(cur * 1000)
		} else {
			cur -= float32(kf[i].timeTo)
			kf[i].arrive = uint32(cur * 1000)
			kf[i].depart = kf[i].arrive
		}
	}
	return kf, kf[n-1].depart
}

// Dock is a stop of a transport: where it waits (its position), from arrive to depart (ms into the
// period), on map.
type Dock struct {
	Map            uint32
	X, Y, Z        float32
	Arrive, Depart uint32
}

func docks(kf []keyFrame) []Dock {
	var out []Dock
	for _, k := range kf {
		if k.stop() {
			out = append(out, Dock{Map: k.node.Map, X: k.node.X, Y: k.node.Y, Z: k.node.Z, Arrive: k.arrive, Depart: k.depart})
		}
	}
	return out
}

// waitFor is how long (ms) until the transport, timer ms into its period, is next docked at d with at
// least margin ms before it leaves (0 when it is there now).
func waitFor(d Dock, timer, period, margin uint32) uint32 {
	if timer >= d.Arrive && timer+margin <= d.Depart {
		return 0
	}
	if timer < d.Arrive {
		return d.Arrive - timer
	}
	return period - timer + d.Arrive
}

// rideTime is the time (ms) from leaving dock a to arriving at dock b.
func rideTime(a, b Dock, period uint32) uint32 {
	if b.Arrive >= a.Depart {
		return b.Arrive - a.Depart
	}
	return period - a.Depart + b.Arrive
}
