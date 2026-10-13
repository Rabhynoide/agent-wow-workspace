package combat

import (
	"context"
	"log/slog"
	"time"

	"github.com/agent-wow/agent-wow/pkg/opcode"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/itemuse"
	"wowagent/internal/moveio"
)

// Scrolls (Scroll of Protection, Stamina, Strength...): loot that used to wait in the bags until a
// vendor bought it (2026-10-13). Before a pull, a scroll whose buff is not on us is read. The server
// refuses one when a stronger buff of the same kind is active: that scroll waits scrollRetry.

const (
	itemClassConsumable = 0
	itemSubclassScroll  = 4
	scrollRetry         = 2 * time.Minute
)

// readScrolls uses the scrolls whose buff we lack, and returns whether one was read.
func (s *Service) readScrolls(ctx context.Context, me *statepb.SelfInfo) bool {
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return false
	}
	read := false
	for _, it := range snap.Items {
		if it.Bag == 255 && it.BagSlot < 23 { // equipped, or a bag itself
			continue
		}
		in := s.itemInfo(ctx, it.Entry)
		if in == nil || in.Class != itemClassConsumable || in.Subclass != itemSubclassScroll || in.UseSpell == 0 ||
			in.RequiredLevel > me.Level || hasAura(me.Auras, in.UseSpell) {
			continue
		}
		s.mu.Lock()
		if s.scrollTried == nil {
			s.scrollTried = map[uint32]time.Time{}
		}
		if time.Since(s.scrollTried[in.UseSpell]) < scrollRetry {
			s.mu.Unlock()
			continue
		}
		s.scrollTried[in.UseSpell] = time.Now()
		if s.castCount++; s.castCount == 0 {
			s.castCount = 1
		}
		cnt := s.castCount
		s.mu.Unlock()
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGUseItem), itemuse.Packet(it, in.UseSpell, cnt, nil)); err != nil {
			continue
		}
		slog.Info("reading scroll", "item", in.Name, "spell", in.UseSpell)
		read = true
		sleep(ctx, 1600*time.Millisecond) // global cooldown
	}
	return read
}
