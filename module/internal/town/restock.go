package town

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/protobuf/types/known/emptypb"

	statepb "wowagent/gen/statev1"
	pb "wowagent/gen/townv1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/dbc"
	"wowagent/internal/moveio"
	"wowagent/internal/wire"
)

// Spell categories of food and drink (item_template.spellcategory_1).
const (
	categoryFood  = 11
	categoryDrink = 59
	// Potions share category 4 (one cooldown); healing ones are told apart by their spell (heals).
	categoryPotion = 4
	vendorFlag     = 128
)

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	if p.GetOpcode() == uint32(opcode.SMSGListInventory) {
		select {
		case s.lists <- p.GetPayload():
		default:
		}
	}
	return &emptypb.Empty{}, nil
}

// vendorItem is one line of SMSG_LIST_INVENTORY.
type vendorItem struct {
	slot, entry     uint32 // slot: vendor slot + 1, as the client sends it back
	stock           int32  // -1 = unlimited
	price, buyCount uint32 // price of one lot of buyCount items
	extendedCost    uint32
}

func parseInventory(b []byte) (uint64, []vendorItem) {
	r := wire.NewReader(b)
	guid := r.U64()
	n := int(r.U8())
	var out []vendorItem
	for i := 0; i < n && !r.Err; i++ {
		it := vendorItem{slot: r.U32(), entry: r.U32()}
		r.U32() // display id
		it.stock = int32(r.U32())
		it.price = r.U32()
		r.U32() // durability
		it.buyCount = r.U32()
		it.extendedCost = r.U32()
		if !r.Err {
			out = append(out, it)
		}
	}
	return guid, out
}

// pickVendorItem chooses the best item of a category usable at this level: highest required level,
// then the cheapest; only items sold for money and in stock.
func pickVendorItem(list []vendorItem, info func(uint32) *worldpb.ItemInfo, category, level uint32) (vendorItem, *worldpb.ItemInfo, bool) {
	var best vendorItem
	var bestInfo *worldpb.ItemInfo
	for _, v := range list {
		in := info(v.entry)
		if in == nil || in.SpellCategory != category || in.UseSpell == 0 || in.RequiredLevel > level ||
			v.extendedCost != 0 || v.price == 0 || v.stock == 0 {
			continue
		}
		if bestInfo == nil || in.RequiredLevel > bestInfo.RequiredLevel ||
			(in.RequiredLevel == bestInfo.RequiredLevel && v.price < best.price) {
			best, bestInfo = v, in
		}
	}
	return best, bestInfo, bestInfo != nil
}

// lotsToBuy is how many lots fill the stock up to want, within the budget, the vendor stock and the
// free bag slots (one slot kept free).
func lotsToBuy(have, want uint32, v vendorItem, stack uint32, budget uint64, freeSlots uint32) uint32 {
	if have >= want || v.price == 0 {
		return 0
	}
	lot := max(v.buyCount, 1)
	lots := (want - have + lot - 1) / lot
	lots = min(lots, uint32(budget/uint64(v.price)))
	if v.stock > 0 {
		lots = min(lots, uint32(v.stock)/lot)
	}
	stack = max(stack, 1)
	if freeSlots <= 1 {
		return 0
	}
	maxItems := (freeSlots - 1) * stack
	return min(lots, maxItems/lot)
}

func (s *Service) Restock(ctx context.Context, r *pb.RestockRequest) (*pb.RestockResult, error) {
	failR := func(format string, a ...any) (*pb.RestockResult, error) {
		return &pb.RestockResult{Reason: fmt.Sprintf(format, a...)}, nil
	}
	snap, err := s.api.Snapshot(ctx)
	if err != nil || snap.Self == nil {
		return failR("state: %v", err)
	}
	me := snap.Self
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", NpcFlags: vendorFlag, Radius: 6, Max: 1})
	if err != nil || len(vis.Objs) == 0 {
		return failR("no vendor within 5 yards")
	}
	vendor := vis.Objs[0]
	for len(s.lists) > 0 {
		<-s.lists
	}
	w := &wire.Writer{}
	w.U64(vendor.Guid)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGListInventory), w.B); err != nil {
		return failR("send: %v", err)
	}
	var list []vendorItem
	select {
	case b := <-s.lists:
		_, list = parseInventory(b)
	case <-time.After(3 * time.Second):
		return failR("vendor did not answer the inventory request")
	case <-ctx.Done():
		return failR("%v", ctx.Err())
	}
	info := func(e uint32) *worldpb.ItemInfo { return s.itemInfo(ctx, e) }
	budget := uint64(r.MaxCopper)
	if budget == 0 {
		budget = uint64(me.Money) / 3
	}
	res := &pb.RestockResult{Ok: true, Vendor: fmt.Sprint(vendor.Entry)}
	free := snap.FreeSlots
	for _, want := range []struct{ category, count uint32 }{{categoryFood, r.Food}, {categoryDrink, r.Drink}, {categoryPotion, r.Potions}} {
		if want.count == 0 {
			continue
		}
		pick := info
		if want.category == categoryPotion {
			pick = func(e uint32) *worldpb.ItemInfo {
				if in := info(e); in != nil && heals(in.UseSpell) {
					return in
				}
				return nil
			}
		}
		var have uint32
		for _, it := range snap.Items {
			if in := pick(it.Entry); it.Bag != 0 && in != nil && in.SpellCategory == want.category && in.RequiredLevel <= me.Level {
				have += it.Count
			}
		}
		v, in, ok := pickVendorItem(list, pick, want.category, me.Level)
		if !ok {
			continue
		}
		lots := lotsToBuy(have, want.count, v, in.Stackable, budget, free)
		bought := uint32(0)
		for i := uint32(0); i < lots; i++ {
			bw := &wire.Writer{}
			bw.U64(vendor.Guid)
			bw.U32(v.entry)
			bw.U32(v.slot)
			bw.U32(1) // one lot per packet
			bw.U8(0)
			if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGBuyItem), bw.B); err != nil {
				break
			}
			bought++
			time.Sleep(150 * time.Millisecond)
		}
		if bought > 0 {
			n := bought * max(v.buyCount, 1)
			res.Bought = append(res.Bought, &pb.BuyLine{Entry: v.entry, Name: in.Name, Count: n, Copper: bought * v.price})
			budget -= uint64(bought * v.price)
			free -= min(free, (n+max(in.Stackable, 1)-1)/max(in.Stackable, 1))
		}
	}
	time.Sleep(500 * time.Millisecond)
	if after, err := s.api.Brief(ctx); err == nil && after.Self != nil && after.Self.Money <= me.Money {
		res.Copper = me.Money - after.Self.Money
	}
	if len(res.Bought) > 0 && res.Copper == 0 {
		res.Ok, res.Reason = false, "the vendor took no money: purchases refused (bags, reputation or stock?)"
	}
	return res, nil
}

var (
	spellsOnce sync.Once
	spells     map[uint32]dbc.SpellInfo
)

// heals tells whether a spell has a heal effect (Spell.dbc, loaded once).
func heals(spell uint32) bool {
	spellsOnce.Do(func() {
		dir := os.Getenv("DBC_DIR")
		if dir == "" {
			dir = "/dbc"
		}
		spells, _ = dbc.LoadSpellInfo(dir)
	})
	for _, e := range spells[spell].Effects {
		if e == 10 { // SPELL_EFFECT_HEAL
			return true
		}
	}
	return false
}
