package auto

import (
	"fmt"
	"log/slog"
	"time"

	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
)

// Gear bought on the way (tuning buy_gear): a vendor in sight that sells a weapon or armor worth
// clearly more than what we wear, by the class's gear score (progress: weapon damage and the stats
// that count for the class, strength for a warrior; usable, level), within a share of our money. The
// level-6 snapshot fights with a Militia Warhammer (2.6 damage per second) while Corina Steele, the
// Goldshire weaponsmith, sells a Wooden Mallet (5.0) for 7 silver; the runs that looted a Severing
// Axe (7.0) were among the fastest (series of 2026-10-28).
const (
	gearRadius   = 60               // yards: vendors in sight
	gearEvery    = 20 * time.Second // between two looks
	gearShare    = 0.8              // of our money at most for armor (a weapon may take it all)
	gearMinGain  = 3                // gear score points
	gearMinRatio = 0.25             // of the score of what it replaces
)

// gearBuy is a purchase worth a detour.
type gearBuy struct {
	vendor *statepb.Obj
	item   uint32
	price  uint32
	gain   float32
	weapon bool
}

// gearOnTheWay returns the best upgrade sold by a vendor in sight, or nil.
func (r *run) gearOnTheWay() *gearBuy {
	if !r.s.tune.Get().BuyGear || r.detour || time.Since(r.lastGearCheck) < gearEvery {
		return nil
	}
	r.lastGearCheck = time.Now()
	b, err := r.s.api.Brief(r.ctx)
	if err != nil || b.Self == nil || b.Self.Money == 0 {
		return nil
	}
	me := b.Self
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Radius: gearRadius, Max: 8, AliveOnly: true, NpcFlags: vendorFlag})
	if err != nil || len(vis.Objs) == 0 {
		return nil
	}
	// What we carry first: an upgrade bought but not put on (refused in combat) was compared again to
	// what we wear and bought again at the next look (money ended 1 100 copper lower with buy_gear,
	// series c497454). Put on what waits in the bags, and never buy an item we already have.
	r.gearUp()
	snap, err := r.s.api.Snapshot(r.ctx)
	if err != nil {
		return nil
	}
	r.owned = map[uint32]bool{}
	for _, it := range snap.Items {
		r.owned[it.Entry] = true
	}
	budget := uint32(float64(me.Money) * gearShare)
	var best *gearBuy
	for _, v := range vis.Objs {
		// Nothing new to buy there while we stay at the same level with the same money.
		key := [3]uint32{v.Entry, me.Level, me.Money / 100}
		if r.gearSeen[key] {
			continue
		}
		r.gearSeen[key] = true
		items, err := r.s.api.VendorItems(r.ctx, v.Entry)
		if err != nil || len(items) == 0 {
			continue
		}
		// The weapon first (the user's priority, 2026-10-28): the best weapon upgrade whatever its
		// price; affordable, it is the purchase; too dear, its price is kept aside (weaponWant) and
		// armor is bought only with the money beyond it.
		// The reserve holds until a weapon is bought or this vendor's weapons are no upgrade any more
		// (a better one looted): cleared at each new level, it let a pair of leggings take the money
		// of the Wooden Mallet (series 7f891bc, 2026-10-28).
		w := r.bestOf(items, me.Level, true, ^uint32(0))
		hasWeapons := r.sellsWeapons(items)
		slog.Info("gear vendor", "entry", v.Entry, "dist", int(v.Dist), "money", me.Money, "weapon", w.String(), "reserve", r.weaponWant)
		switch {
		case w != nil && w.price <= me.Money: // the weapon may take all our money
			w.vendor = v
			if best == nil || !best.weapon || w.gain > best.gain {
				best = w
			}
			continue
		case w != nil:
			if r.weaponWant == 0 || w.price < r.weaponWant {
				r.weaponWant, r.weaponVendor = w.price, v.Entry
			}
		case hasWeapons && v.Entry == r.weaponVendor:
			// The reserved weapon is no upgrade any more (a better one looted). Another vendor's
			// weapons count for nothing here: a tool seller's mining pick lifted the reserve and a pair
			// of leggings took the Wooden Mallet's money (series c497454).
			r.weaponWant, r.weaponVendor = 0, 0
		}
		if best != nil && best.weapon {
			continue
		}
		armorBudget := uint32(0)
		if budget > r.weaponWant {
			armorBudget = budget - r.weaponWant
		}
		if a := r.bestOf(items, me.Level, false, armorBudget); a != nil && (best == nil || a.gain > best.gain) {
			a.vendor = v
			best = a
		}
	}
	return best
}

// String sums a purchase up for the log.
func (g *gearBuy) String() string {
	if g == nil {
		return "none"
	}
	return fmt.Sprintf("%d (%d copper, +%.1f)", g.item, g.price, g.gain)
}

// sellsWeapons tells whether a vendor's items include a weapon.
func (r *run) sellsWeapons(items []*worldpb.VendorItem) bool {
	for _, it := range items {
		if info, err := r.s.api.ItemInfo(r.ctx, it.Entry); err == nil && info.Class == 2 {
			return true
		}
	}
	return false
}

// bestOf is the best upgrade among a vendor's items within budget: weapons (item class 2) or armor
// (the others), worth gearMinGain and gearMinRatio of what it replaces.
func (r *run) bestOf(items []*worldpb.VendorItem, level uint32, weapon bool, budget uint32) *gearBuy {
	var ids, prices []uint32
	for _, it := range items {
		if r.owned[it.Entry] {
			continue
		}
		info, err := r.s.api.ItemInfo(r.ctx, it.Entry)
		if err != nil || (info.Class == 2) != weapon {
			continue
		}
		if it.RequiredLevel <= level && it.Price <= budget {
			ids, prices = append(ids, it.Entry), append(prices, it.Price)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	res, err := r.s.api.BestPurchase(r.ctx, ids, prices, budget)
	if err != nil || res.Reason != "upgrade" || res.Gain < gearMinGain || res.Gain < gearMinRatio*max(res.Replaced, 4) {
		return nil
	}
	return &gearBuy{item: ids[res.Index], price: prices[res.Index], gain: res.Gain, weapon: weapon}
}

// buyGear walks to the vendor, buys the item and puts it on.
func (r *run) buyGear(g *gearBuy) {
	r.step("vendor %d %.0f yards away: buying item %d (%d copper, gear score +%.1f)", g.vendor.Entry, g.vendor.Dist, g.item, g.price, g.gain)
	if _, err := r.reachVendor(g.vendor.Entry, g.vendor.X, g.vendor.Y, g.vendor.Z); err != nil {
		r.note("gear vendor: %v", err)
		return
	}
	res, err := r.s.api.BuyItem(r.ctx, g.item, g.price)
	if err != nil || !res.Ok {
		r.note("gear not bought: %v %s", err, res.GetReason())
		return
	}
	r.note("bought %s for %d copper", res.Bought[0].Name, res.Copper)
	slog.Info("gear bought", "item", g.item, "name", res.Bought[0].Name, "copper", res.Copper, "weapon", g.weapon, "gain", g.gain)
	if g.weapon {
		r.weaponWant, r.weaponVendor = 0, 0 // the weapon kept aside is bought (or a better one): armor may follow
	}
	r.gearUp()
}
