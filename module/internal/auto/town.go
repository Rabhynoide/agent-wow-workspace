package auto

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	statepb "wowagent/gen/statev1"
)

// Town visits (user's rule of 2026-10-23): we go back to town only to turn in quests, train or repair.
// Selling and buying food happen when we are next to a vendor anyway: at a quest giver or ender, the
// trainer, a repair, or a walk that goes past one. Full bags far from a vendor make room by throwing
// away the cheapest junk for a better item (town.make_room, at loot time).
const (
	vendorFlag, repairFlag = 128, 4096
	// shopRadius: a vendor this close (yards) is visited on the way.
	shopRadius = 40
	// shopEvery: no new visit on the way sooner than this after the last one (a turn-in visits anyway).
	shopEvery = 3 * time.Minute
	// shopRepairBelow: repair at a vendor we visit anyway below this durability (percent).
	shopRepairBelow = 90
)

// ensureRepair goes to the nearest repair vendor when the gear is worn out, and trades there.
func (r *run) ensureRepair() error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	if snap.DurabilityPct >= repairBelow {
		return nil
	}
	if failed := r.s.repairFailedAt.Load(); failed > 0 && me.Money < failed+100 {
		return nil
	}
	r.step("going to repair (durability %d%%)", snap.DurabilityPct)
	vendors, err := r.s.api.FindVendor(r.ctx, me.Map, me.X, me.Y, repairFlag)
	if err != nil {
		return err
	}
	var lastErr error
	for _, v := range vendors {
		if len(v.Name) == 0 || v.Name[0] == '[' {
			continue
		}
		npc, err := r.reachVendor(v.Entry, v.X, v.Y, v.Z)
		if err != nil {
			lastErr = err
			continue
		}
		r.trade(npc)
		return nil
	}
	return fmt.Errorf("no reachable repair vendor (%d candidates): %v", len(vendors), lastErr)
}

// reachVendor walks to a vendor and returns it once within the server's 5.5 yards.
func (r *run) reachVendor(entry uint32, x, y, z float32) (*statepb.Obj, error) {
	was := r.detour
	r.detour = true // no other detour (chest, vendor) on the way there
	defer func() { r.detour = was }()
	if err := r.approachNpc(entry, x, y, z); err != nil {
		return nil, err
	}
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Entry: entry, Radius: 8, Max: 1})
	if err != nil || len(vis.Objs) == 0 || vis.Objs[0].Dist > 5 {
		return nil, fmt.Errorf("vendor %d not found at its spawn", entry)
	}
	return vis.Objs[0], nil
}

// supplyNeeds tells what to buy at a vendor: food and drink below half the wanted count, potions.
func (r *run) supplyNeeds(me *statepb.SelfInfo) (wantDrink uint32, needFood, needPotion bool) {
	food, drink, potions := r.supplies(me)
	if me.PowerType == 0 {
		wantDrink = drinkWanted
	}
	needFood = (food < foodWanted/2 || (wantDrink > 0 && drink < wantDrink/2)) && me.Money >= minMoneyForFood
	if failed := r.s.foodFailedAt.Load(); failed > 0 && me.Money < failed+100 {
		needFood = false
	}
	needPotion = potions < potionsWanted && me.Level >= 3 && me.Money >= minMoneyForPotions
	if failed := r.s.potionFailedAt.Load(); failed > 0 && me.Money < failed+100 {
		needPotion = false
	}
	return wantDrink, needFood, needPotion
}

// shop visits a vendor within shopRadius when there is something to sell, buy or repair. force skips
// the shopEvery delay (at a quest ender: the reward needs bag space).
func (r *run) shop(force bool) {
	if npc := r.shopTarget(force); npc != nil {
		r.visit(npc)
	}
}

// shopTarget returns the vendor within shopRadius worth a visit now, nil when none or nothing to do.
func (r *run) shopTarget(force bool) *statepb.Obj {
	if r.detour || (!force && time.Since(r.lastShop) < shopEvery) {
		return nil
	}
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Radius: shopRadius, Max: 8, AliveOnly: true, NpcFlags: vendorFlag})
	if err != nil || len(vis.Objs) == 0 {
		return nil
	}
	snap, err := r.snapshot()
	if err != nil {
		return nil
	}
	me := snap.Self
	r.lastShop = time.Now()
	plan, err := r.s.api.SellPlan(r.ctx)
	sell := err == nil && plan.Ok && plan.Sold > 0
	_, needFood, needPotion := r.supplyNeeds(me)
	repair := snap.DurabilityPct < shopRepairBelow
	if failed := r.s.repairFailedAt.Load(); failed > 0 && me.Money < failed+100 {
		repair = false
	}
	if !sell && !needFood && !needPotion && !repair {
		return nil
	}
	// A repairer when repairing, else one that sells what we need (the innkeeper, not the smith), else
	// the nearest.
	npc := vis.Objs[0]
	if want := r.supplyVendor(me, vis.Objs, needFood); !repair && (needFood || needPotion) && want != nil {
		npc = want
	}
	for _, o := range vis.Objs {
		if repair && o.NpcFlags&repairFlag != 0 {
			npc = o
			break
		}
	}
	slog.Info("vendor on the way", "entry", npc.Entry, "dist", npc.Dist, "sell", plan.GetSold(), "food", needFood, "potion", needPotion, "repair", repair)
	return npc
}

// supplyVendor returns the vendor in sight that sells food (or potions when food is not needed).
func (r *run) supplyVendor(me *statepb.SelfInfo, near []*statepb.Obj, needFood bool) *statepb.Obj {
	category := uint32(categoryFood)
	if !needFood {
		category = categoryPotion
	}
	sp, err := r.s.api.FindFoodVendor(r.ctx, me.Map, me.X, me.Y, category, me.Level)
	if err != nil {
		return nil
	}
	for _, v := range sp {
		for _, o := range near {
			if o.Entry == v.Entry {
				return o
			}
		}
	}
	return nil
}

// visit walks to a vendor and trades.
func (r *run) visit(npc *statepb.Obj) {
	r.step("vendor %d %.0f yards away: trading on the way", npc.Entry, npc.Dist)
	at, err := r.reachVendor(npc.Entry, npc.X, npc.Y, npc.Z)
	if err != nil {
		r.note("vendor on the way: %v", err)
		return
	}
	r.trade(at)
}

// trade sells, buys supplies and repairs at a vendor within 5 yards.
func (r *run) trade(npc *statepb.Obj) {
	snap, err := r.snapshot()
	if err != nil {
		return
	}
	me := snap.Self
	r.s.lastVendor.Store(&[3]float32{float32(me.Map), npc.X, npc.Y})
	if npc.NpcFlags&vendorFlag != 0 {
		if res, err := r.s.api.SellJunk(r.ctx); err == nil && res.Ok && res.Sold > 0 {
			r.note("sold %d stacks for %d copper (%d free slots)", res.Sold, res.Copper, res.FreeSlots)
		}
	}
	wantDrink, needFood, needPotion := r.supplyNeeds(me)
	if npc.NpcFlags&vendorFlag != 0 && snap.DurabilityPct >= repairBelow && (needFood || needPotion) {
		food, potions := uint32(0), uint32(0)
		if needFood {
			food = foodWanted
		} else {
			wantDrink = 0
		}
		if needPotion {
			potions = potionsWanted
		}
		if res, err := r.s.api.Restock(r.ctx, food, wantDrink, potions); err == nil && res.Ok && len(res.Bought) > 0 {
			r.s.foodFailedAt.Store(0)
			var what []string
			for _, b := range res.Bought {
				what = append(what, fmt.Sprintf("%d %s", b.Count, b.Name))
			}
			r.note("bought %s for %d copper", strings.Join(what, ", "), res.Copper)
		} else {
			r.note("restock: nothing bought %v %s", err, res.GetReason())
			// A vendor of what we need sold none (too poor for its best item): no new visit for it
			// before we earn a silver more. Another kind of vendor is just tried at the next visit.
			if r.supplyVendor(me, []*statepb.Obj{npc}, needFood) != nil {
				if needFood {
					r.s.foodFailedAt.Store(max(me.Money, 1))
				} else {
					r.s.potionFailedAt.Store(max(me.Money, 1))
				}
			}
		}
	}
	if npc.NpcFlags&repairFlag != 0 && snap.DurabilityPct < shopRepairBelow {
		if res, err := r.s.api.Repair(r.ctx); err == nil && res.Ok {
			r.s.repairFailedAt.Store(0)
			r.note("repaired for %d copper (durability %d%%)", res.Copper, res.DurabilityPct)
		} else {
			// Not enough money: play on with worn gear, the next sales pay for it.
			r.note("could not repair: %v %s", err, res.GetReason())
			r.s.repairFailedAt.Store(max(me.Money, 1))
		}
	}
}

// rewardRoom makes the bag space a quest reward needs, at the quest ender (a vendor near it was
// visited first): the cheapest junk is thrown away.
func (r *run) rewardRoom(items int) {
	for i := 0; i < items; i++ {
		b, err := r.s.api.Brief(r.ctx)
		if err != nil || b.FreeSlots >= uint32(items) {
			return
		}
		res, err := r.s.api.MakeRoom(r.ctx, 0, 1)
		if err == nil && res.Ok && res.Destroyed == 0 {
			return // a slot is free: town.make_room only throws away with full bags
		}
		if err != nil || !res.Ok {
			r.note("no room for the quest reward: %v %s", err, res.GetReason())
			return
		}
		r.note("threw away item %d (%d copper) for the quest reward", res.Destroyed, res.DestroyedCopper)
	}
}

// vendorOnTheWay returns a vendor a walk goes past and should stop at (checked every 5 s at most).
func (r *run) vendorOnTheWay() *statepb.Obj {
	if r.detour || time.Since(r.lastShop) < shopEvery || time.Since(r.lastVendorCheck) < 5*time.Second {
		return nil
	}
	r.lastVendorCheck = time.Now()
	return r.shopTarget(false)
}
