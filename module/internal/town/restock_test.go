package town

import (
	"testing"

	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/wire"
)

func TestParseInventory(t *testing.T) {
	w := &wire.Writer{}
	w.U64(42)
	w.U8(2)
	for _, it := range [][8]uint32{{1, 117, 2473, 0xFFFFFFFF, 25, 0, 5, 0}, {2, 159, 18084, 3, 25, 0, 5, 0}} {
		for _, v := range it {
			w.U32(v)
		}
	}
	guid, list := parseInventory(w.B)
	if guid != 42 || len(list) != 2 || list[0].stock != -1 || list[1].stock != 3 || list[1].entry != 159 || list[0].buyCount != 5 {
		t.Fatalf("parsed %d %+v", guid, list)
	}
}

func TestPickAndLots(t *testing.T) {
	infos := map[uint32]*worldpb.ItemInfo{
		117:  {Entry: 117, SpellCategory: categoryFood, UseSpell: 433, RequiredLevel: 1, Stackable: 20},
		2287: {Entry: 2287, SpellCategory: categoryFood, UseSpell: 434, RequiredLevel: 5, Stackable: 20},
		3770: {Entry: 3770, SpellCategory: categoryFood, UseSpell: 435, RequiredLevel: 15, Stackable: 20},
		159:  {Entry: 159, SpellCategory: categoryDrink, UseSpell: 430, RequiredLevel: 1, Stackable: 20},
	}
	info := func(e uint32) *worldpb.ItemInfo { return infos[e] }
	list := []vendorItem{
		{slot: 1, entry: 117, stock: -1, price: 25, buyCount: 5},
		{slot: 2, entry: 2287, stock: -1, price: 100, buyCount: 5},
		{slot: 3, entry: 3770, stock: -1, price: 400, buyCount: 5},
		{slot: 4, entry: 159, stock: -1, price: 25, buyCount: 5},
	}
	v, _, ok := pickVendorItem(list, info, categoryFood, 10)
	if !ok || v.entry != 2287 {
		t.Fatalf("food at level 10 = %+v", v)
	}
	if _, _, ok := pickVendorItem(list[:1], info, categoryDrink, 10); ok {
		t.Fatal("no drink on that list")
	}
	// Want 20, have 3: 4 lots of 5 (17 needed), budget for 4, plenty of space.
	if n := lotsToBuy(3, 20, v, 20, 1000, 10); n != 4 {
		t.Fatalf("lots = %d, want 4", n)
	}
	// Budget for only 2 lots.
	if n := lotsToBuy(0, 20, v, 20, 250, 10); n != 2 {
		t.Fatalf("lots with a small budget = %d, want 2", n)
	}
	// Bags: one free slot is kept, so nothing.
	if n := lotsToBuy(0, 20, v, 20, 1000, 1); n != 0 {
		t.Fatalf("lots with full bags = %d, want 0", n)
	}
	if n := lotsToBuy(25, 20, v, 20, 1000, 10); n != 0 {
		t.Fatalf("lots when stocked = %d, want 0", n)
	}
}
