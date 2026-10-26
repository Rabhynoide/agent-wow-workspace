package combat

import (
	"path/filepath"
	"testing"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/tuning"
	"wowagent/internal/safe"
	"wowagent/internal/xpscore"
)

func TestAggroAndWeights(t *testing.T) {
	if aggroRange(10, 10) != 20 || aggroRange(10, 5) != 15 || aggroRange(30, 1) != 5 || aggroRange(1, 80) != 45 {
		t.Fatal("aggro range")
	}
	if pullWeight(10, 4) != 0.25 || pullWeight(10, 7) != 0.5 || pullWeight(10, 10) != 1 || pullWeight(10, 13) != 2 {
		t.Fatal("pull weights")
	}
}

// A camp of kobolds: the target with a friend 8 yards away is crowded; a lone one is not.
func TestAddsAndPullSpot(t *testing.T) {
	const lvl = 10
	target := &statepb.Obj{Guid: 1, X: 0, Y: 0, Level: 9, Reaction: "hostile"}
	friend := &statepb.Obj{Guid: 2, X: 8, Y: 0, Level: 9, Reaction: "hostile"}
	busy := &statepb.Obj{Guid: 3, X: 5, Y: 5, Level: 9, Target: 99} // fighting someone else
	far := &statepb.Obj{Guid: 4, X: 200, Y: 0, Level: 9}
	all := []*statepb.Obj{target, friend, busy, far}
	adds := addsOf(target, all, levelAggro(lvl))
	if len(adds) != 1 || adds[0].Guid != 2 {
		t.Fatalf("adds = %+v, want only the idle friend", adds)
	}
	if load := pullLoad(target, adds, lvl); load != 2 {
		t.Fatalf("load %v, want 2", load)
	}
	// Alone, a monster above our level still counts 1.
	strong := &statepb.Obj{Guid: 5, Level: 12}
	if load := pullLoad(strong, nil, lvl); load > 1 {
		t.Fatalf("a lone level-12 monster has load %v at level 10", load)
	}
	if len(addsOf(far, all, levelAggro(lvl))) != 0 {
		t.Fatal("the far one is alone")
	}
	// Body pull spot: west of the target (away from the friend at +8), within the target's aggro range
	// (21 yards at level 9 vs 10) but out of the friend's (21) + margin.
	me := &statepb.Obj{X: -60, Y: 0}
	x, y, ok := pullSpot(me, target, []*statepb.Obj{friend}, levelAggro(lvl))
	if !ok {
		t.Fatal("no spot found")
	}
	if d := dist2D(x, y, 0, 0); d > aggroRange(lvl, 9) {
		t.Fatalf("spot %v,%v is %v from the target: out of its aggro range", x, y, d)
	}
	if d := dist2D(x, y, 8, 0); d < aggroRange(lvl, 9)+3 {
		t.Fatalf("spot %v,%v is %v from the friend: inside its aggro range", x, y, d)
	}
	// Surrounded: no spot.
	ring := []*statepb.Obj{}
	for _, p := range [][2]float32{{10, 0}, {-10, 0}, {0, 10}, {0, -10}} {
		ring = append(ring, &statepb.Obj{X: p[0], Y: p[1], Level: 9})
	}
	if _, _, ok := pullSpot(me, target, ring, levelAggro(lvl)); ok {
		t.Fatal("a surrounded target cannot be pulled alone")
	}
}

// Server aggro rules: a monster 8 yards above the target (another floor) is no add, unless within the
// assistance range in 3D; the detection range of the template moves the radius.
func TestAddsHeightAndDetection(t *testing.T) {
	target := &statepb.Obj{Guid: 1, X: 0, Y: 0, Z: 20, Level: 5}
	above := &statepb.Obj{Guid: 2, X: 9, Y: 0, Z: 28, Level: 5}   // 12 yards in 3D: no aggro (dz 8), no assist
	beside := &statepb.Obj{Guid: 3, X: 16, Y: 0, Z: 21, Level: 5} // in aggro (17 at level 6 vs 5, + margin)
	near := &statepb.Obj{Guid: 4, X: 3, Y: 0, Z: 25, Level: 5}    // dz 5 but 5.8 yards in 3D: assists
	adds := addsOf(target, []*statepb.Obj{target, above, beside, near}, func(o *statepb.Obj) float32 { return safe.AggroRangeOf(18, 6, o.Level) })
	if len(adds) != 2 || adds[0].Guid != 3 || adds[1].Guid != 4 {
		t.Fatalf("adds %+v, want the one beside and the near one", adds)
	}
	if safe.AggroRangeOf(18, 6, 5) != 17 || safe.AggroRangeOf(18, 40, 1) != 5 || safe.AggroRangeOf(20, 1, 60) != 45 {
		t.Fatal("aggro range")
	}
}

// Pull capacity from the learned fights (no table: 14 % per fight at the same level).
func TestPullFits(t *testing.T) {
	s := &Service{fights: xpscore.NewFile(filepath.Join(t.TempDir(), "none.json"))}
	full := &statepb.SelfInfo{Level: 5, ClassId: 1, Hp: 100, MaxHp: 100}
	hurt := &statepb.SelfInfo{Level: 5, ClassId: 1, Hp: 75, MaxHp: 100}
	mob := func(lv uint32) *statepb.Obj { return &statepb.Obj{Level: lv} }
	for _, c := range []struct {
		me   *statepb.SelfInfo
		adds []*statepb.Obj
		ok   bool
	}{
		{hurt, nil, true},                     // alone
		{full, []*statepb.Obj{mob(5)}, true},  // 14 × 2.5 = 35 ≤ 65
		{hurt, []*statepb.Obj{mob(5)}, true},  // 35 ≤ 40
		{hurt, []*statepb.Obj{mob(6)}, false}, // (14 + 26) / 2 × 2.5 = 50 > 40
		{full, []*statepb.Obj{mob(5), mob(5)}, true},
		{full, []*statepb.Obj{mob(5), mob(5), mob(5)}, false},
	} {
		ok, loss, limit := s.pullFits(c.me, 0)(mob(5), c.adds)
		if ok != c.ok {
			t.Errorf("hp %d, %d adds: fits=%v (loss %.0f, limit %.0f), want %v", c.me.Hp, len(c.adds), ok, loss, limit, c.ok)
		}
	}
	if ok, _, _ := s.pullFits(full, 1)(mob(5), []*statepb.Obj{mob(5)}); ok {
		t.Error("max_pull 1 must refuse a pair")
	}
}

// A camp joins link by link: each monster in the fight calls for help within 10 yards of itself.
func TestAddsChain(t *testing.T) {
	target := &statepb.Obj{Guid: 1, X: 0, Y: 0, Z: 0, Level: 7}
	var hostiles []*statepb.Obj
	hostiles = append(hostiles, target)
	for i := 1; i <= 4; i++ { // a line of wolves 8 yards apart, beyond aggro range of the target
		hostiles = append(hostiles, &statepb.Obj{Guid: uint64(1 + i), X: float32(8 * i), Y: 0, Z: 0, Level: 7})
	}
	adds := addsOf(target, hostiles, func(o *statepb.Obj) float32 { return 5 })
	if len(adds) != 4 {
		t.Fatalf("%d adds, want the whole line of 4", len(adds))
	}
}

// Rest before a pull: the learned cost plus flee_margin, at least rest_floor, at most 90.
func TestRestNeed(t *testing.T) {
	tu := tuning.Defaults()
	wolf := []*statepb.Obj{{Level: 7}}
	known := func(l float32) func([]*statepb.Obj) (float32, bool) {
		return func([]*statepb.Obj) (float32, bool) { return l, true }
	}
	unknown := func([]*statepb.Obj) (float32, bool) { return 0, false }
	if need, _ := restNeedFor(wolf, tu, known(8), nil); need != tu.RestFloor {
		t.Errorf("weak wolf: need %d, want the floor %d", need, tu.RestFloor)
	}
	if need, _ := restNeedFor(wolf, tu, known(50), nil); need != 65 {
		t.Errorf("hard monster: need %d, want 50+15", need)
	}
	if need, _ := restNeedFor(wolf, tu, known(120), nil); need != 90 {
		t.Errorf("capped at 90: %d", need)
	}
	if need, how := restNeedFor(wolf, tu, unknown, func([]uint32) float32 { return 30 }); need != 45 || how[:6] != "fights" {
		t.Errorf("from the fights: %d %s", need, how)
	}
	if need, _ := restNeedFor(wolf, tu, unknown, func([]uint32) float32 { return 0 }); need != tu.WarriorRestBelow {
		t.Errorf("unknown: %d", need)
	}
}
