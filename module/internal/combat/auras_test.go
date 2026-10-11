package combat

import (
	"testing"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/warrior"
)

func TestAuraHelpers(t *testing.T) {
	target := []*statepb.Aura{
		{Spell: 6546, LeftMs: 1500, Mine: true},  // our Rend rank 2, about to expire
		{Spell: 7922, LeftMs: 400, Mine: false},  // someone's stun
		{Spell: 2457, LeftMs: 0, Positive: true}, // permanent
	}
	if !needsRefresh(target, warrior.Rend, 2000) || needsRefresh(target, warrior.Rend, 1000) {
		t.Fatal("Rend refresh threshold")
	}
	if left, ok := auraLeft(target, []uint32{2457}, false); !ok || left < 1e9 {
		t.Fatal("a permanent aura has no end")
	}
	if _, ok := auraLeft(target, []uint32{7922}, true); ok {
		t.Fatal("not ours")
	}
	if !needsRefresh(nil, warrior.BattleShout, 3000) {
		t.Fatal("missing shout must be cast")
	}
	if !auraState(1<<9, warrior.StateVictoryRush) || auraState(1<<9, warrior.StateHealthBelow20) || !auraState(0x2, warrior.StateHealthBelow20) {
		t.Fatal("aura state bits")
	}
	if !hasAura([]*statepb.Aura{{Spell: warrior.TasteForBlood}}, warrior.TasteForBlood) {
		t.Fatal("proc")
	}
}
