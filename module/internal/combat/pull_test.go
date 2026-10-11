package combat

import (
	"testing"

	statepb "wowagent/gen/statev1"
)

func TestAggroAndWeights(t *testing.T) {
	if aggroRange(10, 10) != 20 || aggroRange(10, 5) != 15 || aggroRange(30, 1) != 5 || aggroRange(1, 80) != 45 {
		t.Fatal("aggro range")
	}
	if pullWeight(10, 4) != 0.25 || pullWeight(10, 7) != 0.5 || pullWeight(10, 10) != 1 || pullWeight(10, 13) != 2 {
		t.Fatal("pull weights")
	}
	if pullCapacity(8) != 1 || pullCapacity(25) != 1.5 || pullCapacity(60) != 2 {
		t.Fatal("capacity")
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
	adds := addsOf(target, all, lvl)
	if len(adds) != 1 || adds[0].Guid != 2 {
		t.Fatalf("adds = %+v, want only the idle friend", adds)
	}
	if load := pullLoad(target, adds, lvl); load != 2 || load <= pullCapacity(lvl) {
		t.Fatalf("load %v: the pair must exceed a level-10 capacity", load)
	}
	// Alone, a monster above our level still fits the capacity.
	strong := &statepb.Obj{Guid: 5, Level: 12}
	if load := pullLoad(strong, nil, lvl); load > pullCapacity(lvl) {
		t.Fatalf("a lone level-12 monster has load %v at level 10", load)
	}
	if len(addsOf(far, all, lvl)) != 0 {
		t.Fatal("the far one is alone")
	}
	// Body pull spot: west of the target (away from the friend at +8), within the target's aggro range
	// (21 yards at level 9 vs 10) but out of the friend's (21) + margin.
	me := &statepb.Obj{X: -60, Y: 0}
	x, y, ok := pullSpot(me, target, []*statepb.Obj{friend}, lvl)
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
	if _, _, ok := pullSpot(me, target, ring, lvl); ok {
		t.Fatal("a surrounded target cannot be pulled alone")
	}
}
