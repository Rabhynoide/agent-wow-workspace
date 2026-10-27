package auto

import (
	"testing"

	statepb "wowagent/gen/statev1"
)

// A wolf beside the road ahead is engaged; one behind us or far off the road is not.
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
}
