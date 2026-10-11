package combat

import (
	"testing"

	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
)

func TestBestConsumable(t *testing.T) {
	infos := map[uint32]*worldpb.ItemInfo{
		117:  {Entry: 117, SpellCategory: CategoryFood, UseSpell: 433, RequiredLevel: 1},   // Tough Jerky
		2287: {Entry: 2287, SpellCategory: CategoryFood, UseSpell: 434, RequiredLevel: 5},  // Haunch of Meat
		3770: {Entry: 3770, SpellCategory: CategoryFood, UseSpell: 435, RequiredLevel: 15}, // too high
		159:  {Entry: 159, SpellCategory: CategoryDrink, UseSpell: 430, RequiredLevel: 1},  // Refreshing Spring Water
	}
	info := func(e uint32) *worldpb.ItemInfo { return infos[e] }
	items := []*statepb.Item{
		{Entry: 117, Count: 4, Bag: 255, BagSlot: 23},
		{Entry: 2287, Count: 2, Bag: 255, BagSlot: 24},
		{Entry: 3770, Count: 5, Bag: 255, BagSlot: 25},
		{Entry: 159, Count: 5, Bag: 255, BagSlot: 26},
		{Entry: 2287, Count: 1}, // bank (no bag position)
	}
	if got := bestConsumable(items, info, CategoryFood, 10); got == nil || got.Entry != 2287 || got.BagSlot != 24 {
		t.Fatalf("food at level 10 = %+v, want the Haunch of Meat in the bags", got)
	}
	if got := bestConsumable(items, info, CategoryFood, 3); got == nil || got.Entry != 117 {
		t.Fatalf("food at level 3 = %+v, want Tough Jerky", got)
	}
	if got := bestConsumable(items, info, CategoryDrink, 10); got == nil || got.Entry != 159 {
		t.Fatalf("drink = %+v, want water", got)
	}
	if got := bestConsumable(nil, info, CategoryFood, 10); got != nil {
		t.Fatalf("empty bags gave %+v", got)
	}
}
