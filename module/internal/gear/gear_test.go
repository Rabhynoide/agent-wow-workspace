package gear

import (
	"testing"

	pb "wowagent/gen/worldv1"
)

const priestID, human = 5, 1

func TestUsable(t *testing.T) {
	staff := &Item{Class: 2, Subclass: 10, InventoryType: 17, AllowableClass: -1, AllowableRace: -1}
	sword := &Item{Class: 2, Subclass: 7, InventoryType: 13, AllowableClass: -1, AllowableRace: -1}
	mail := &Item{Class: 4, Subclass: 3, InventoryType: 5, AllowableClass: -1, AllowableRace: -1}
	high := &Item{Class: 4, Subclass: 1, InventoryType: 5, AllowableClass: -1, AllowableRace: -1, RequiredLevel: 20}
	warriorOnly := &Item{Class: 4, Subclass: 1, InventoryType: 5, AllowableClass: 1, AllowableRace: -1}
	dagger := &Item{Class: 2, Subclass: 15, InventoryType: 13, AllowableClass: -1, AllowableRace: -1}
	known := map[uint32]bool{227: true, 198: true, 5009: true, 9078: true} // staves, maces, wands, cloth
	if !Usable(staff, priestID, human, 5, known) || Usable(sword, priestID, human, 5, known) || Usable(mail, priestID, human, 5, known) ||
		Usable(high, priestID, human, 5, known) || Usable(warriorOnly, priestID, human, 5, known) || Usable(dagger, priestID, human, 5, known) {
		t.Fatal("wrong proficiency/level/class checks")
	}
}

func TestGain(t *testing.T) {
	robe := &Item{Class: 4, Subclass: 1, InventoryType: 20, Armor: 3}
	better := &Item{Class: 4, Subclass: 1, InventoryType: 5, Armor: 5, Stats: []*pb.ItemStat{{Type: 6, Value: 1}}}
	if slot, g := Gain(better, map[int]*Item{4: robe}); slot != 4 || g < 1.1 {
		t.Fatalf("chest gain = %d %v", slot, g)
	}
	// Ring goes to the weaker of the two finger slots.
	r1 := &Item{Class: 4, InventoryType: 11, Stats: []*pb.ItemStat{{Type: 6, Value: 3}}}
	r2 := &Item{Class: 4, InventoryType: 11, Stats: []*pb.ItemStat{{Type: 6, Value: 1}}}
	nr := &Item{Class: 4, InventoryType: 11, Stats: []*pb.ItemStat{{Type: 6, Value: 2}}}
	if slot, g := Gain(nr, map[int]*Item{10: r1, 11: r2}); slot != 11 || g != 1 {
		t.Fatalf("ring gain = %d %v", slot, g)
	}
	// A two-hander must beat main hand + off hand together.
	mace := &Item{Class: 2, Subclass: 4, InventoryType: 13, Dps: 3}
	orb := &Item{Class: 4, Subclass: 0, InventoryType: 23, Stats: []*pb.ItemStat{{Type: 6, Value: 2}}}
	staff := &Item{Class: 2, Subclass: 10, InventoryType: 17, Dps: 4}
	if _, g := Gain(staff, map[int]*Item{15: mace, 16: orb}); g >= 0 {
		t.Fatalf("staff should not replace mace+orb, gain %v", g)
	}
}

func TestStatOrder(t *testing.T) {
	score := func(typ uint32) float32 { return Score(&Item{Stats: []*pb.ItemStat{{Type: typ, Value: 1}}}) }
	order := []uint32{45, 6, 30, 21, 5, 7, 31} // spell power > spirit > haste > crit > intellect > stamina > hit
	for i := 1; i < len(order); i++ {
		if score(order[i-1]) <= score(order[i]) {
			t.Fatalf("stat %d should outweigh stat %d", order[i-1], order[i])
		}
	}
	// A wand beats a same-DPS melee weapon.
	if Score(&Item{Class: 2, Subclass: 19, Dps: 5}) <= Score(&Item{Class: 2, Subclass: 10, Dps: 5}) {
		t.Fatal("wand should outscore melee weapons")
	}
}
