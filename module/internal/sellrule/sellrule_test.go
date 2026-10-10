package sellrule

import (
	"testing"

	worldpb "wowagent/gen/worldv1"
)

type I = worldpb.ItemInfo

var known = map[uint32]bool{227: true, 198: true, 5009: true, 9078: true} // staves, maces, wands, cloth

func ctx() Ctx {
	return Ctx{Class: 5, Race: 1, Level: 5, Known: known, Equipped: map[int]*I{}, QuestItems: map[uint32]bool{}}
}

func plan(t *testing.T, c Ctx, items ...*I) map[uint32]Decision {
	var ss []Stack
	for _, it := range items {
		ss = append(ss, Stack{Entry: it.Entry, Count: 1, Info: it})
	}
	m := map[uint32]Decision{}
	for _, d := range Plan(ss, c) {
		m[d.Entry] = d
	}
	return m
}

func TestRules(t *testing.T) {
	c := ctx()
	c.QuestItems[772] = true
	c.Equipped[1] = nil
	items := []*I{
		{Entry: 1, Quality: 0, SellPrice: 3}, // gray
		{Entry: 2, Quality: 1, Class: 4, Subclass: 6, InventoryType: 14, SellPrice: 15, AllowableClass: -1, AllowableRace: -1},         // shield: priests cannot
		{Entry: 3, Quality: 1, Class: 4, Subclass: 2, InventoryType: 6, SellPrice: 6, AllowableClass: -1, AllowableRace: -1},           // leather belt
		{Entry: 4, Quality: 1, Class: 2, Subclass: 15, InventoryType: 13, SellPrice: 24, AllowableClass: -1, AllowableRace: -1},        // dagger: no skill
		{Entry: 5, Quality: 1, Class: 4, Subclass: 1, InventoryType: 6, SellPrice: 6, Armor: 2, AllowableClass: -1, AllowableRace: -1}, // cloth belt, upgrade over nothing
		{Entry: 772, Quality: 1, Class: 5, SellPrice: 5},                                                                               // quest item
		{Entry: 6948, Quality: 1, Class: 15, SellPrice: 0},                                                                             // hearthstone
		{Entry: 7, Quality: 3, Class: 4, Subclass: 6, InventoryType: 14, SellPrice: 900, AllowableClass: -1, AllowableRace: -1},        // rare
		{Entry: 8, Quality: 1, Class: 7, SellPrice: 4},                                                                                 // trade good
		{Entry: 9, Quality: 1, Class: 1, SellPrice: 10},                                                                                // bag
	}
	m := plan(t, c, items...)
	want := map[uint32]bool{1: true, 2: true, 3: true, 4: true, 5: false, 772: false, 6948: false, 7: false, 8: true, 9: false}
	for e, sell := range want {
		if m[e].Sell != sell {
			t.Errorf("item %d: sell=%v (%s), want %v", e, m[e].Sell, m[e].Reason, sell)
		}
	}
}

func TestEquippedItemMakesBeltNoUpgrade(t *testing.T) {
	c := ctx()
	c.Equipped[5] = &I{Class: 4, Subclass: 1, InventoryType: 6, Armor: 5} // slot 5 = waist
	belt := &I{Entry: 5, Quality: 1, Class: 4, Subclass: 1, InventoryType: 6, SellPrice: 6, Armor: 2, AllowableClass: -1, AllowableRace: -1}
	if d := plan(t, c, belt)[5]; !d.Sell || d.Reason != "no upgrade" {
		t.Fatalf("decision = %+v", d)
	}
}

func TestFoodKeepsBestOfEachKind(t *testing.T) {
	c := ctx()
	food := func(e uint32, cat, req, lvl uint32) *I {
		return &I{Entry: e, Quality: 1, Class: 0, Subclass: 5, SpellCategory: cat, RequiredLevel: req, ItemLevel: lvl, SellPrice: 1}
	}
	m := plan(t, c, food(10, 11, 1, 5), food(11, 11, 5, 15), food(12, 11, 20, 40), food(13, 59, 1, 5), food(14, 59, 5, 15))
	if m[11].Sell || !m[10].Sell || !m[12].Sell {
		t.Errorf("food: keep 11 (best usable), sell 10 and 12: %+v %+v %+v", m[10], m[11], m[12])
	}
	if m[14].Sell || !m[13].Sell {
		t.Errorf("drink: keep 14, sell 13: %+v %+v", m[13], m[14])
	}
}

func TestObsoleteConsumablesAreSold(t *testing.T) {
	snack := &I{Entry: 1, Quality: 1, Class: 0, Subclass: 5, SpellCategory: 11, RequiredLevel: 1, ItemLevel: 5, SellPrice: 1}
	potion := &I{Entry: 2, Quality: 1, Class: 0, Subclass: 1, RequiredLevel: 3, ItemLevel: 5, SellPrice: 5}
	noReq := &I{Entry: 3, Quality: 1, Class: 0, Subclass: 1, RequiredLevel: 0, ItemLevel: 12, SellPrice: 5}
	c := ctx()
	m := plan(t, c, snack, potion, noReq) // level 5
	if m[1].Sell || m[2].Sell || m[3].Sell {
		t.Fatalf("level 5 keeps everything: %+v %+v %+v", m[1], m[2], m[3])
	}
	c.Level = 13
	m = plan(t, c, snack, potion, noReq)
	if !m[1].Sell || !m[2].Sell || m[3].Sell {
		t.Fatalf("level 13: snack and potion are obsolete, entry 3 (level 12) is not: %+v %+v %+v", m[1], m[2], m[3])
	}
	if m[1].Reason == "" {
		t.Fatal("missing reason")
	}
}

func TestSalesComeFirstGrayBeforeGear(t *testing.T) {
	c := ctx()
	items := []*I{
		{Entry: 3, Quality: 1, Class: 4, Subclass: 2, InventoryType: 6, SellPrice: 6, AllowableClass: -1, AllowableRace: -1},
		{Entry: 5, Quality: 1, Class: 9, SellPrice: 6},
		{Entry: 1, Quality: 0, SellPrice: 1},
		{Entry: 6948, Quality: 1, Class: 15, SellPrice: 0},
	}
	var ss []Stack
	for _, it := range items {
		ss = append(ss, Stack{Entry: it.Entry, Count: 2, Info: it})
	}
	ds := Plan(ss, c)
	if ds[0].Entry != 1 || ds[1].Entry != 3 || ds[2].Entry != 5 || ds[3].Sell {
		t.Fatalf("order = %d %d %d %v", ds[0].Entry, ds[1].Entry, ds[2].Entry, ds[3].Sell)
	}
	if ds[0].Copper != 2 || ds[2].Copper != 12 {
		t.Fatalf("copper = %d %d", ds[0].Copper, ds[2].Copper)
	}
}
