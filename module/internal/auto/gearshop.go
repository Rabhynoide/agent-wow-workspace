package auto

import (
	"time"

	statepb "wowagent/gen/statev1"
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
	gearShare    = 0.8              // of our money at most (runs reach level 8 with 10-19 silver)
	gearMinGain  = 3                // gear score points
	gearMinRatio = 0.25             // of the score of what it replaces
)

// gearBuy is a purchase worth a detour.
type gearBuy struct {
	vendor *statepb.Obj
	item   uint32
	price  uint32
	gain   float32
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
	if err != nil {
		return nil
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
		var ids, prices []uint32
		for _, it := range items {
			if it.RequiredLevel <= me.Level && it.Price <= budget {
				ids, prices = append(ids, it.Entry), append(prices, it.Price)
			}
		}
		if len(ids) == 0 {
			continue
		}
		res, err := r.s.api.BestPurchase(r.ctx, ids, prices, budget)
		if err != nil || res.Reason != "upgrade" || res.Gain < gearMinGain || res.Gain < gearMinRatio*max(res.Replaced, 4) {
			continue
		}
		if best == nil || res.Gain > best.gain {
			best = &gearBuy{vendor: v, item: ids[res.Index], price: prices[res.Index], gain: res.Gain}
		}
	}
	return best
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
	r.gearUp()
}
