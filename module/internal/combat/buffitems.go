package combat

import (
	"context"
	"log/slog"
	"time"

	"github.com/agent-wow/agent-wow/pkg/opcode"

	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/dbc"
	"wowagent/internal/itemuse"
	"wowagent/internal/moveio"
)

// Buff items (scrolls, elixirs, flasks): loot that used to wait in the bags until a vendor bought it
// (Scroll of Protection, Elixir of Lion's Strength, 2026-10-13). One whose buff is not on us and helps
// our class is used once a target is chosen, right before the pull: used earlier (before choosing, or
// on the way to the hunting spot) its 30-60 minutes would run out in walks. The server refuses one
// when a stronger buff of the same kind is active: that item waits buffItemRetry.

const (
	itemClassConsumable = 0
	itemSubclassElixir  = 2
	itemSubclassFlask   = 3
	itemSubclassScroll  = 4
	buffItemRetry       = 2 * time.Minute
)

// Aura types (EffectApplyAuraName) worth an item, and the stats of SPELL_AURA_MOD_STAT.
const (
	auraModDamageDone   = 13
	auraModResistance   = 22
	auraModStat         = 29
	auraModIncreaseHP   = 34
	auraModRegen        = 84
	auraModPowerRegen   = 85
	auraModAttackPower  = 99
	auraModRating       = 189
	statStrength        = 0
	statAgility         = 1
	statStamina         = 2
	statIntellect       = 3
	statSpirit          = 4
	resistanceArmorMask = 1
)

// pureCaster: priests, mages and warlocks fight with spells, not weapons.
func pureCaster(class uint32) bool { return class == 5 || class == 8 || class == 9 }

// helps tells whether a spell's auras do something for this character.
func helps(sp dbc.SpellInfo, me *statepb.SelfInfo) bool {
	mana := me.PowerType == 0
	melee := !pureCaster(me.ClassId)
	for e, a := range sp.Auras {
		m := sp.Misc[e]
		switch a {
		case auraModStat:
			if m == -1 || m == statStamina || (melee && (m == statStrength || m == statAgility)) ||
				(mana && (m == statIntellect || m == statSpirit)) {
				return true
			}
		case auraModResistance:
			if m&resistanceArmorMask != 0 {
				return true
			}
		case auraModIncreaseHP, auraModRegen, auraModRating:
			return true
		case auraModAttackPower:
			if melee {
				return true
			}
		case auraModDamageDone, auraModPowerRegen:
			if mana {
				return true
			}
		}
	}
	return false
}

// buffItem tells whether an item is a buff for us that is not on us yet.
func (s *Service) buffItem(in *worldpb.ItemInfo, me *statepb.SelfInfo) bool {
	if in == nil || in.Class != itemClassConsumable || in.UseSpell == 0 || in.RequiredLevel > me.Level ||
		(in.Subclass != itemSubclassElixir && in.Subclass != itemSubclassFlask && in.Subclass != itemSubclassScroll) ||
		hasAura(me.Auras, in.UseSpell) {
		return false
	}
	sp, ok := s.book.load()[in.UseSpell]
	return ok && helps(sp, me)
}

// useBuffItems uses the buff items we lack, and returns whether one was used.
func (s *Service) useBuffItems(ctx context.Context, me *statepb.SelfInfo) bool {
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return false
	}
	used := false
	for _, it := range snap.Items {
		if it.Bag == 255 && it.BagSlot < 23 { // equipped, or a bag itself
			continue
		}
		in := s.itemInfo(ctx, it.Entry)
		if !s.buffItem(in, me) {
			continue
		}
		s.mu.Lock()
		if s.buffTried == nil {
			s.buffTried = map[uint32]time.Time{}
		}
		if time.Since(s.buffTried[in.UseSpell]) < buffItemRetry {
			s.mu.Unlock()
			continue
		}
		s.buffTried[in.UseSpell] = time.Now()
		if s.castCount++; s.castCount == 0 {
			s.castCount = 1
		}
		cnt := s.castCount
		s.mu.Unlock()
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGUseItem), itemuse.Packet(it, in.UseSpell, cnt, nil)); err != nil {
			continue
		}
		slog.Info("using buff item", "item", in.Name, "spell", in.UseSpell)
		used = true
		sleep(ctx, 1600*time.Millisecond) // global cooldown
	}
	return used
}
