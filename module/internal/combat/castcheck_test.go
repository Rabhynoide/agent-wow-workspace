package combat

import (
	"testing"
	"time"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/dbc"
)

func TestCanCast(t *testing.T) {
	b := &spellBook{ready: map[uint32]time.Time{}}
	b.once.Do(func() {
		b.info = map[uint32]dbc.SpellInfo{
			100:   {ID: 100, PowerType: 1, MinRange: 8, MaxRange: 25},          // Charge
			772:   {ID: 772, PowerType: 1, Cost: 100, Melee: true},             // Rend
			34428: {ID: 34428, PowerType: 1, CasterAuraState: 10, Melee: true}, // Victory Rush
			5308:  {ID: 5308, PowerType: 1, Cost: 150, TargetAuraState: 2, Melee: true},
			78:    {ID: 78, PowerType: 1, Cost: 50, Melee: true, NeedsWeapon: true}, // weapon spell
		}
	})
	me := &statepb.SelfInfo{PowerType: 1, Power: 120}
	wolf := &statepb.Obj{}
	if ok, why := b.canCast(100, me, wolf, 5); ok || why != "too close" {
		t.Fatalf("charge at 5 yards: %v %s", ok, why)
	}
	if ok, _ := b.canCast(100, me, wolf, 20); !ok {
		t.Fatal("charge at 20 yards")
	}
	if ok, why := b.canCast(772, me, wolf, 12); ok || why != "out of melee range" {
		t.Fatalf("rend at 12 yards: %v %s", ok, why)
	}
	if ok, _ := b.canCast(772, me, wolf, 3); !ok {
		t.Fatal("rend in melee with 12 rage")
	}
	if ok, why := b.canCast(5308, me, wolf, 3); ok || why != "power" {
		t.Fatalf("execute with 12 rage: %v %s", ok, why)
	}
	if ok, why := b.canCast(34428, me, wolf, 3); ok || why != "caster aura state" {
		t.Fatalf("victory rush without the state: %v %s", ok, why)
	}
	me.AuraState = 1 << 9
	if ok, _ := b.canCast(34428, me, wolf, 3); !ok {
		t.Fatal("victory rush with the state")
	}
	b.ready[772] = time.Now().Add(time.Second)
	if ok, why := b.canCast(772, me, wolf, 3); ok || why != "cooldown" {
		t.Fatalf("rend on cooldown: %v %s", ok, why)
	}
	// Disarmed: weapon spells wait only while the flag is set.
	me.UnitFlags = unitFlagDisarmed
	if ok, why := b.canCast(78, me, wolf, 3); ok || why != "no usable weapon" {
		t.Fatalf("weapon spell while disarmed: %v %s", ok, why)
	}
	me.UnitFlags = 0
	if ok, _ := b.canCast(78, me, wolf, 3); !ok {
		t.Fatal("weapon spell once rearmed")
	}
	// An unexplained weapon refusal pauses weapon spells a few seconds only.
	b.weaponRefused()
	if ok, _ := b.canCast(78, me, wolf, 3); ok {
		t.Fatal("short pause after a refusal")
	}
	if left := time.Until(b.noWeapon); left > 4*time.Second {
		t.Fatalf("pause of %v, want about 3 s", left)
	}
	if ok, _ := b.canCast(9999, me, wolf, 50); !ok {
		t.Fatal("unknown spells are left to the server")
	}
}
