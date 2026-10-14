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

// Healing potions (and healthstones) at critical health, in a fight or while fleeing: the only death
// of the 6 → 10 bench of 2026-10-13 came 8 s into a flight started at 34 %, with looted potions in
// the bags.

const (
	potionBelow      = 35 // % of health
	potionCooldown   = 60 * time.Second
	itemSubclassPot  = 1
	spellEffectHeal  = 10
	itemSubclassMisc = 0 // healthstones
	// potionHeal: % of our health a potion is counted for in the flight decision (Lesser Healing
	// Potion: 70-90 health, 33-42 % at level 8-10).
	potionHeal = 30
	potionLook = 10 * time.Second // the bags are read again after this
)

// potionReady tells whether a healing potion could be drunk now (one in the bags, cooldown over).
func (s *Service) potionReady(ctx context.Context, me *statepb.SelfInfo) bool {
	if s.since(&s.potionAt) < potionCooldown {
		return false
	}
	s.mu.Lock()
	if time.Since(s.potionSeenAt) < potionLook {
		have := s.potionHave
		s.mu.Unlock()
		return have
	}
	s.mu.Unlock()
	have := false
	if snap, err := s.api.Snapshot(ctx); err == nil {
		for _, it := range snap.Items {
			if (it.Bag != 255 || it.BagSlot >= 23) && s.healingItem(s.itemInfo(ctx, it.Entry), me) {
				have = true
				break
			}
		}
	}
	s.mu.Lock()
	s.potionHave, s.potionSeenAt = have, time.Now()
	s.mu.Unlock()
	return have
}

// healingItem tells whether an item heals us when used (a potion or a healthstone we may use).
func (s *Service) healingItem(in *worldpb.ItemInfo, me *statepb.SelfInfo) bool {
	if in == nil || in.Class != itemClassConsumable || in.UseSpell == 0 || in.RequiredLevel > me.Level ||
		(in.Subclass != itemSubclassPot && in.Subclass != itemSubclassMisc) {
		return false
	}
	sp, ok := s.book.load()[in.UseSpell]
	if !ok {
		return false
	}
	for _, e := range sp.Effects {
		if e == spellEffectHeal {
			return true
		}
	}
	return false
}

// drinkPotion uses the strongest healing item in our bags when our health is critical, once per
// potion cooldown. It returns whether one was used.
func (s *Service) drinkPotion(ctx context.Context, me *statepb.SelfInfo) bool {
	if me.MaxHp == 0 || pct(me.Hp, me.MaxHp) >= potionBelow || s.since(&s.potionAt) < potionCooldown {
		return false
	}
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return false
	}
	var best *statepb.Item
	var bestInfo *worldpb.ItemInfo
	for _, it := range snap.Items {
		if it.Bag == 255 && it.BagSlot < 23 {
			continue
		}
		in := s.itemInfo(ctx, it.Entry)
		if !s.healingItem(in, me) {
			continue
		}
		if bestInfo == nil || in.RequiredLevel > bestInfo.RequiredLevel || in.ItemLevel > bestInfo.ItemLevel {
			best, bestInfo = it, in
		}
	}
	if best == nil {
		return false
	}
	s.mark(&s.potionAt)
	s.mu.Lock()
	s.potionSeenAt = time.Time{} // maybe the last one: read the bags again
	if s.castCount++; s.castCount == 0 {
		s.castCount = 1
	}
	cnt := s.castCount
	s.mu.Unlock()
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGUseItem), itemuse.Packet(best, bestInfo.UseSpell, cnt, nil)); err != nil {
		return false
	}
	slog.Info("drinking potion", "item", bestInfo.Name, "hp", pct(me.Hp, me.MaxHp))
	return true
}
