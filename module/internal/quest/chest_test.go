package quest

import (
	"testing"

	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/dbc"
)

func TestClassify(t *testing.T) {
	open := dbc.Lock{}
	open.Type[0], open.Index[0] = 2, 5
	herb := dbc.Lock{}
	herb.Type[0], herb.Index[0] = 2, 2
	locks := map[uint32]dbc.Lock{57: open, 29: herb}
	for _, c := range []struct {
		name  string
		g     *worldpb.GameObjectInfo
		spell uint32
		ok    bool
	}{
		{"battered chest", &worldpb.GameObjectInfo{Type: 3, LockId: 57, LootItems: 28}, 3365, true},
		{"no lock", &worldpb.GameObjectInfo{Type: 3, LootItems: 12}, 0, true},
		{"food crate", &worldpb.GameObjectInfo{Type: 3, LockId: 57, LootItems: 6}, 0, false},
		{"silverleaf", &worldpb.GameObjectInfo{Type: 3, LockId: 29, LootItems: 20}, 0, false},
		{"quest chest", &worldpb.GameObjectInfo{Type: 3, LockId: 57, LootItems: 28, QuestId: 2746}, 0, false},
		{"door", &worldpb.GameObjectInfo{Type: 0, LootItems: 28}, 0, false},
		{"unknown lock", &worldpb.GameObjectInfo{Type: 3, LockId: 999, LootItems: 28}, 0, false},
	} {
		k := classify(c.g, locks)
		if (k != nil) != c.ok || (k != nil && k.spell != c.spell) {
			t.Errorf("%s: got %+v, want ok=%v spell=%d", c.name, k, c.ok, c.spell)
		}
	}
}
