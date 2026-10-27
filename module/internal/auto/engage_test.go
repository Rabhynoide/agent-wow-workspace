package auto

import (
	"testing"

	statepb "wowagent/gen/statev1"
)

// A wolf beside the road ahead is engaged; one behind us or far off the road is not; a kobold with a
// friend next to it is not lonely.
func TestEngageGeometry(t *testing.T) {
	me := &statepb.SelfInfo{X: 0, Y: 0}
	const x, y = 100, 0
	if o := (&statepb.Obj{X: 20, Y: 8}); !ahead(me, o, x, y) || offLine(me, o, x, y) > engageOffLine {
		t.Fatal("wolf beside the road ahead")
	}
	if o := (&statepb.Obj{X: -15, Y: 2}); ahead(me, o, x, y) {
		t.Fatal("behind us")
	}
	if o := (&statepb.Obj{X: 20, Y: 20}); offLine(me, o, x, y) <= engageOffLine {
		t.Fatal("far off the road")
	}
	k := &statepb.Obj{Guid: 1, X: 20, Y: 5}
	all := []*statepb.Obj{k, {Guid: 2, X: 26, Y: 5}, {Guid: 3, X: 60, Y: 5}}
	rangeOf := func(*statepb.Obj) float32 { return 15 }
	if f := friendsOf(k, all, rangeOf); len(f) != 1 || f[0].Guid != 2 {
		t.Fatalf("friends: %v", f)
	}
}
