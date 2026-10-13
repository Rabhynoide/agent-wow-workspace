package combat

import (
	"path/filepath"
	"testing"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/xpscore"
)

// Flight decision from the learned fights (no table: 14 % per fight at the same level, 26 % at +1).
func TestLosing(t *testing.T) {
	s := &Service{fights: xpscore.NewFile(filepath.Join(t.TempDir(), "none.json"))}
	me := func(hp uint32) *statepb.SelfInfo { return &statepb.SelfInfo{Level: 5, ClassId: 1, Hp: hp, MaxHp: 100} }
	mob := func(lv, hp uint32) *statepb.Obj { return &statepb.Obj{Level: lv, Hp: hp, MaxHp: 100} }
	for _, c := range []struct {
		name string
		hp   uint32
		att  []*statepb.Obj
		flee bool
	}{
		{"nobody", 10, nil, false},
		{"one fresh monster at full health", 100, []*statepb.Obj{mob(5, 100)}, false},
		{"one fresh monster at 25 %", 25, []*statepb.Obj{mob(5, 100)}, true}, // 14 > 10 %
		{"one dying monster at 25 %", 25, []*statepb.Obj{mob(5, 20)}, false},
		{"2 wolves join at 70 %", 70, []*statepb.Obj{mob(5, 50), mob(5, 100), mob(5, 100)}, false}, // 46.7 ≤ 55
		{"2 wolves join at 60 %", 60, []*statepb.Obj{mob(5, 50), mob(5, 100), mob(5, 100)}, true},  // 46.7 > 45 (bench death)
		{"2 wolves left at 49 %", 49, []*statepb.Obj{mob(5, 100), mob(5, 100)}, true},              // 35 > 34
		{"last wolf at 19 %", 19, []*statepb.Obj{mob(5, 100)}, true},
		{"+1 monster at 40 %", 40, []*statepb.Obj{mob(6, 100)}, true},                                       // 26 > 25
		{"+1 monster at 45 %", 45, []*statepb.Obj{mob(6, 100)}, false},                                      // 26 ≤ 30
		{"+2 monster at 55 %", 55, []*statepb.Obj{mob(7, 100)}, true},                                       // margin 25
		{"3 kobolds 2 levels below", 92, []*statepb.Obj{mob(3, 100), mob(3, 100), mob(3, 100)}, false},      // loss under crowdLoss
		{"3 fresh kobolds at full health", 100, []*statepb.Obj{mob(5, 100), mob(5, 100), mob(5, 90)}, true}, // crowd (Fargodeep deaths)
		{"3 kobolds, one at half", 100, []*statepb.Obj{mob(5, 50), mob(5, 100), mob(5, 100)}, false},
	} {
		got, loss := s.losing(me(c.hp), c.att)
		if got != c.flee {
			t.Errorf("%s: flee=%v (loss left %.1f), want %v", c.name, got, loss, c.flee)
		}
	}
}
