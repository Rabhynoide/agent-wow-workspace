package auto

import (
	"testing"

	movepb "wowagent/gen/movev1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/safe"
)

func TestStretchAndBlockers(t *testing.T) {
	var pts []*movepb.PathPoint
	for x := float32(0); x <= 60; x += 2 { // straight path east, 60 yards
		pts = append(pts, &movepb.PathPoint{X: x})
	}
	seg := stretch(pts, 25)
	if last := seg[len(seg)-1]; last.X > 25 || last.X < 23 {
		t.Fatalf("stretch ends at %v", last.X)
	}
	// Level 5 character: kobolds level 6 (aggro 21 with the default detection + margin 3 = 24 yards).
	near := &statepb.Obj{Guid: 1, X: 20, Y: 15, Level: 6, Dist: 25}       // 15 yards off the stretch: blocks
	nearer := &statepb.Obj{Guid: 2, X: 10, Y: 20, Level: 6, Dist: 22}     // also blocks, nearer to us
	far := &statepb.Obj{Guid: 3, X: 55, Y: 0, Level: 6, Dist: 55}         // on the path but beyond the stretch reach
	deep := &statepb.Obj{Guid: 4, X: 10, Y: 5, Z: 20, Level: 6, Dist: 21} // another floor of the mine
	bl := blockers(seg, []*statepb.Obj{near, far, nearer, deep}, func(o *statepb.Obj) float32 { return safe.AggroRangeOf(safe.DefaultDetection, 5, o.Level) })
	if len(bl) != 2 || bl[0].Guid != 2 || bl[1].Guid != 1 {
		t.Fatalf("blockers %+v, want 2 then 1", bl)
	}
}
