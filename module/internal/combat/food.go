package combat

import (
	"context"
	"log/slog"
	"time"

	"github.com/agent-wow/agent-wow/pkg/opcode"

	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/itemuse"
	"wowagent/internal/moveio"
)

// Spell categories of the food and drink items (item_template.spellcategory_1).
const (
	CategoryFood  = 11
	CategoryDrink = 59
)

// Eat or drink below these percentages when resting (bandage-free classes rely on food).
const (
	eatBelow   = 80
	drinkBelow = 70
)

// bestConsumable picks, among the bag items of a category usable at this level, the one with the
// highest required level (it restores the most); nil when there is none.
func bestConsumable(items []*statepb.Item, info func(uint32) *worldpb.ItemInfo, category, level uint32) *statepb.Item {
	var best *statepb.Item
	var bestLevel uint32
	for _, it := range items {
		if it.Bag == 0 || it.Count == 0 {
			continue
		}
		in := info(it.Entry)
		if in == nil || in.SpellCategory != category || in.UseSpell == 0 || in.RequiredLevel > level {
			continue
		}
		if best == nil || in.RequiredLevel > bestLevel {
			best, bestLevel = it, in.RequiredLevel
		}
	}
	return best
}

// itemInfo caches world.item_info for the food checks.
func (s *Service) itemInfo(ctx context.Context, entry uint32) *worldpb.ItemInfo {
	s.mu.Lock()
	in, ok := s.items[entry]
	s.mu.Unlock()
	if ok {
		return in
	}
	in, err := s.api.ItemInfo(ctx, entry)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	s.items[entry] = in
	s.mu.Unlock()
	return in
}

// eatAndDrink uses the best food when health is low and the best drink when mana is low (the
// server sits the character down). It returns whether something was consumed.
func (s *Service) eatAndDrink(ctx context.Context, me *statepb.SelfInfo) bool {
	wantFood := pct(me.Hp, me.MaxHp) < eatBelow && me.Level >= s.tuneGet().EatFromLevel
	wantDrink := me.PowerType == 0 && me.MaxPower > 0 && pct(me.Power, me.MaxPower) < drinkBelow
	if !wantFood && !wantDrink {
		return false
	}
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return false
	}
	info := func(e uint32) *worldpb.ItemInfo { return s.itemInfo(ctx, e) }
	used := false
	for _, c := range []struct {
		want     bool
		category uint32
	}{{wantFood, CategoryFood}, {wantDrink, CategoryDrink}} {
		if !c.want {
			continue
		}
		it := bestConsumable(snap.Items, info, c.category, me.Level)
		if it == nil {
			continue
		}
		in := info(it.Entry)
		s.mu.Lock()
		if s.castCount++; s.castCount == 0 {
			s.castCount = 1
		}
		cnt := s.castCount
		s.mu.Unlock()
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGUseItem), itemuse.Packet(it, in.UseSpell, cnt, nil)); err != nil {
			continue
		}
		slog.Info("consuming", "item", in.Name, "category", c.category)
		used = true
		sleep(ctx, 600*time.Millisecond)
	}
	return used
}
