package combat

import (
	"context"
	"math"
	"sync"
	"time"

	"wowagent/internal/api"
)

// Trail of our recent positions. A flight used to run to the point farthest from the monsters
// (move.safe_point), which in the Fargodeep mine led deeper into the tunnels, among more kobolds: two
// deaths in the 6 → 10 bench of 2026-10-13. The way we came is walkable and cleared, so a flight
// runs back along it first; safe_point only when there is no trail or the attackers stand on it.

const (
	trailStep  = 5   // yards between two recorded positions
	trailMax   = 120 // positions kept (about 600 yards of walk)
	trailJump  = 60  // a longer move between two samples is a teleport (death, hearth): new trail
	trailLeg   = 15  // yards of trail per leg of a flight (one goto each)
	trailRun   = 90  // yards of trail run at most
	trailEvery = 2 * time.Second
)

type trailPt struct {
	m       uint32
	x, y, z float32
}

type trail struct {
	mu     sync.Mutex
	pts    []trailPt
	paused bool // while fleeing: the run back is not recorded (a later flight would follow it back)
}

func dist2(ax, ay, bx, by float32) float32 {
	return float32(math.Hypot(float64(ax-bx), float64(ay-by)))
}

// add records a position (every trailStep yards).
func (t *trail) add(p trailPt) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.paused {
		return
	}
	if n := len(t.pts); n > 0 {
		last := t.pts[n-1]
		d := dist2(p.x, p.y, last.x, last.y)
		if last.m != p.m || d > trailJump {
			t.pts = t.pts[:0]
		} else if d < trailStep {
			return
		}
	}
	t.pts = append(t.pts, p)
	if len(t.pts) > trailMax {
		t.pts = append(t.pts[:0], t.pts[len(t.pts)-trailMax:]...)
	}
}

func (t *trail) reset() {
	t.mu.Lock()
	t.pts = t.pts[:0]
	t.mu.Unlock()
}

// back gives the legs of a run back along the trail from where we stand (m, x, y), and the trail index
// of each leg's end. Nil when the trail does not start near us.
func (t *trail) back(m uint32, x, y float32) ([]trailPt, []int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// The trail point nearest to us among the recent ones (we may have moved a little in the fight).
	start, best := -1, float32(trailLeg)
	for i := len(t.pts) - 1; i >= 0 && i >= len(t.pts)-6; i-- {
		if p := t.pts[i]; p.m == m {
			if d := dist2(x, y, p.x, p.y); d < best {
				start, best = i, d
			}
		}
	}
	if start < 0 {
		return nil, nil
	}
	var legs []trailPt
	var idx []int
	leg, run := float32(0), float32(0)
	for i := start - 1; i >= 0 && run < trailRun; i-- {
		d := dist2(t.pts[i].x, t.pts[i].y, t.pts[i+1].x, t.pts[i+1].y)
		leg += d
		run += d
		if leg >= trailLeg || i == 0 || run >= trailRun {
			legs, idx = append(legs, t.pts[i]), append(idx, i)
			leg = 0
		}
	}
	return legs, idx
}

// cut keeps the trail up to index i (where the flight ended) and records again.
func (t *trail) cut(i int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if i >= 0 && i < len(t.pts) {
		t.pts = t.pts[:i+1]
	}
	t.paused = false
}

func (t *trail) pause(p bool) {
	t.mu.Lock()
	t.paused = p
	t.mu.Unlock()
}

// follow samples our position for the module's lifetime.
func (t *trail) follow(c api.C) {
	ctx := context.Background()
	for {
		time.Sleep(trailEvery)
		b, err := c.Brief(ctx)
		if err != nil || b.Self == nil {
			continue
		}
		if b.Self.Dead {
			t.reset()
			continue
		}
		t.add(trailPt{m: b.Self.Map, x: b.Self.X, y: b.Self.Y, z: b.Self.Z})
	}
}
