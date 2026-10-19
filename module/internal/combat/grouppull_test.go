package combat

import (
	"testing"

	statepb "wowagent/gen/statev1"
)

func TestGroupSpot(t *testing.T) {
	me := &statepb.SelfInfo{X: 0, Y: -30}
	a := &statepb.Obj{Guid: 1, X: -15, Y: 0, Level: 6}
	b := &statepb.Obj{Guid: 2, X: 15, Y: 0, Level: 6}
	x, y, ok := groupSpot(me, []*statepb.Obj{a, b}, nil, 6)
	if !ok || dist2D(x, y, a.X, a.Y) > 15 || dist2D(x, y, b.X, b.Y) > 15 {
		t.Fatalf("30 yards apart: %v %v %v", x, y, ok)
	}
	far := &statepb.Obj{Guid: 3, X: 25, Y: 0, Level: 6}
	if _, _, ok := groupSpot(me, []*statepb.Obj{a, far}, nil, 6); ok {
		t.Fatal("40 yards apart: no spot within both aggro ranges")
	}
	near := &statepb.Obj{Guid: 4, X: 0, Y: 10, Level: 6}
	x, y, ok = groupSpot(me, []*statepb.Obj{a, b}, []*statepb.Obj{near}, 6)
	if ok && dist2D(x, y, near.X, near.Y) < 23 {
		t.Fatalf("seen by the other one: %v %v", x, y)
	}
}
