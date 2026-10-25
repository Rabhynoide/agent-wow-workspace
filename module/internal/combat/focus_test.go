package combat

import (
	"testing"

	statepb "wowagent/gen/statev1"
)

func TestFocus(t *testing.T) {
	wolf := func(guid uint64, hp uint32, dist float32) *statepb.Obj {
		return &statepb.Obj{Guid: guid, Entry: 525, Hp: hp, MaxHp: 100, Dist: dist}
	}
	if pickAttacker(nil, nil) != nil {
		t.Error("an attacker picked from nobody")
	}
	if a := pickAttacker([]*statepb.Obj{wolf(1, 90, 3), wolf(2, 40, 5), wolf(3, 10, 20)}, nil); a.Guid != 2 {
		t.Errorf("picked %d, want the weakest within reach (2)", a.Guid)
	}
	if a := pickAttacker([]*statepb.Obj{wolf(1, 90, 15), wolf(2, 40, 25)}, nil); a.Guid != 1 {
		t.Errorf("picked %d, want the nearest when none is within reach (1)", a.Guid)
	}
	cur := wolf(1, 80, 2)
	for _, c := range []struct {
		name string
		cur  *statepb.Obj
		att  []*statepb.Obj
		want uint64
	}{
		{"alone", cur, []*statepb.Obj{cur}, 0},
		{"weaker wolf joins", cur, []*statepb.Obj{cur, wolf(2, 30, 4)}, 2},
		{"near tie", cur, []*statepb.Obj{cur, wolf(2, 60, 4)}, 0},
		{"weaker wolf out of reach", cur, []*statepb.Obj{cur, wolf(2, 30, 15)}, 0},
		{"current target in execute zone", wolf(1, 18, 2), []*statepb.Obj{wolf(1, 18, 2), wolf(2, 5, 4)}, 0},
		{"two others: the weakest", cur, []*statepb.Obj{cur, wolf(2, 40, 4), wolf(3, 20, 6)}, 3},
	} {
		got := uint64(0)
		if b := focusTarget(c.cur, c.att, nil); b != nil {
			got = b.Guid
		}
		if got != c.want {
			t.Errorf("%s: switch to %d, want %d", c.name, got, c.want)
		}
	}
}
