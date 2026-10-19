package combat

import (
	"context"
	"log/slog"
	"math"
	"time"

	pb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
)

// Group pull (tuning chain_pull): a player takes two or three monsters at once while it is easy
// (Thunder Clap, rage from the hits, one approach: 9.8 s per kill with two against 12.9 alone, logs
// of 2026-10-13/14). Monsters rarely stand within aggro range of each other (4 targets with idle adds
// in two benches), so the adds have to be brought: idle hostile monsters near the target are added
// while the pull size allows and the expected loss fits, then we stand where all of them see us and
// nobody else does, and wait for them to come.

const (
	chainWaitS  = 6  // seconds waited at the spot for the monsters to come
	chainSearch = 40 // yards around the target where extra monsters are looked for
	chainInside = 5  // yards inside each monster's aggro range for the spot
)

// chainPull brings extra monsters along with the target (already chosen, its adds fit) and returns
// how many it added (0 = plain pull).
func (s *Service) chainPull(ctx context.Context, j *job, me *statepb.SelfInfo, t *statepb.Obj, hostiles []*statepb.Obj,
	fits func(*statepb.Obj, []*statepb.Obj) (bool, float32, float32)) int {
	tune := s.tuneGet()
	size := s.pullSize.cap(tune)
	if !tune.ChainPull || size < 2 || t.Target != 0 || t.Reaction != "hostile" {
		return 0
	}
	adds := addsOf(t, hostiles, me.Level)
	if 1+len(adds) >= size {
		return 0
	}
	group := append([]*statepb.Obj{t}, adds...)
	in := map[uint64]bool{}
	for _, g := range group {
		in[g.Guid] = true
	}
	zones, now := s.fledZones(), time.Now()
	var extra []*statepb.Obj
	why := map[string]int{} // candidates left out, by reason (log "chain pull skipped")
	for len(group) < size {
		var pick *statepb.Obj
		pickD := float32(chainSearch)
		for _, h := range hostiles {
			if in[h.Guid] || h.Target != 0 || h.Reaction != "hostile" || dist2D(h.X, h.Y, t.X, t.Y) >= chainSearch {
				continue
			}
			if h.Level > me.Level+1 || h.Level+5 < me.Level {
				why["level"]++
				continue
			}
			if s.wasAbandoned(h.Guid) || s.wasCrowded(h.Guid) || inFledZone(zones, me.Map, h.X, h.Y, now) {
				why["avoided"]++
				continue
			}
			d := dist2D(h.X, h.Y, t.X, t.Y)
			if d >= pickD {
				continue
			}
			if own := addsOf(h, hostiles, me.Level); len(own) > countIn(own, in) {
				why["own adds"]++
				continue
			}
			try := append(append([]*statepb.Obj{}, group...), h)
			if _, _, ok := groupSpot(me, try, nil, me.Level); !ok {
				why["too far apart"]++
				continue
			}
			if _, _, ok := groupSpot(me, try, others(hostiles, try), me.Level); !ok {
				why["spot seen by others"]++
				continue
			}
			if ok, _, _ := fits(t, try[1:]); !ok {
				why["loss"]++
				continue
			}
			pick, pickD = h, d
		}
		if pick == nil {
			break
		}
		group = append(group, pick)
		extra = append(extra, pick)
		in[pick.Guid] = true
	}
	if len(extra) == 0 {
		if len(why) > 0 {
			slog.Info("chain pull skipped", "entry", t.Entry, "size", size, "why", why)
		}
		return 0
	}
	x, y, _ := groupSpot(me, group, others(hostiles, group), me.Level)
	j.Update(func(st *pb.CombatStatus) { st.Doing = "group pulling" })
	if st, err := s.api.GotoWait(ctx, x, y, t.Z, 1.5, 30); err != nil || st.State != "arrived" {
		got := ""
		if err != nil {
			got = err.Error()
		} else {
			got = st.State + " " + st.Reason
		}
		slog.Info("chain pull", "entry", t.Entry, "monsters", len(group), "came", "no spot", "spot", [2]float32{x, y}, "goto", got)
		return 0
	}
	came := 0
	for i := 0; i < chainWaitS*2; i++ {
		came = 0
		for _, g := range group {
			if s.onUs(ctx, g.Guid) {
				came++
			}
		}
		if came == len(group) || !sleep(ctx, 500*time.Millisecond) {
			break
		}
	}
	slog.Info("chain pull", "entry", t.Entry, "monsters", len(group), "came", came, "spot", [2]float32{x, y})
	return len(extra)
}

func countIn(objs []*statepb.Obj, in map[uint64]bool) int {
	n := 0
	for _, o := range objs {
		if in[o.Guid] {
			n++
		}
	}
	return n
}

// others are the idle hostile monsters not in the group: the spot must stay out of their sight.
func others(hostiles, group []*statepb.Obj) []*statepb.Obj {
	in := map[uint64]bool{}
	for _, g := range group {
		in[g.Guid] = true
	}
	var out []*statepb.Obj
	for _, h := range hostiles {
		if !in[h.Guid] && h.Target == 0 {
			out = append(out, h)
		}
	}
	return out
}

// groupSpot finds where to stand so that every monster of the group sees us (chainInside yards
// within its aggro range) and no other one does (3 yards of margin), nearest to us.
func groupSpot(me *statepb.SelfInfo, group, rest []*statepb.Obj, level uint32) (x, y float32, ok bool) {
	var cx, cy float32
	for _, g := range group {
		cx, cy = cx+g.X, cy+g.Y
	}
	cx, cy = cx/float32(len(group)), cy/float32(len(group))
	best := float32(math.MaxFloat32)
	for r := float32(0); r <= 20; r += 4 {
		for i := 0; i < 16; i++ {
			a := float64(i) * math.Pi / 8
			px, py := cx+r*float32(math.Cos(a)), cy+r*float32(math.Sin(a))
			if !seenByAll(px, py, group, level) || !unseen(px, py, rest, level) {
				continue
			}
			if d := dist2D(px, py, me.X, me.Y); d < best {
				best, x, y, ok = d, px, py, true
			}
			if r == 0 {
				break
			}
		}
	}
	return x, y, ok
}

func seenByAll(x, y float32, group []*statepb.Obj, level uint32) bool {
	for _, g := range group {
		if dist2D(x, y, g.X, g.Y) > aggroRange(level, g.Level)-chainInside {
			return false
		}
	}
	return true
}

func unseen(x, y float32, rest []*statepb.Obj, level uint32) bool {
	for _, o := range rest {
		if dist2D(x, y, o.X, o.Y) < max(aggroRange(level, o.Level), assistRange)+3 {
			return false
		}
	}
	return true
}
