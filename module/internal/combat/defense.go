package combat

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// What lets the warrior avoid blows from the front (measure of 2026-10-24, melee.go sideStats): dodge
// always, parry once the Parry skill is known, block with a shield in the off hand.

const (
	spellParry   = 3127 // Parry (passive skill)
	slotOffHand  = 16
	itemArmor    = 4 // item class
	armorShield  = 6 // item subclass
	defenseEvery = time.Minute
)

// logDefense logs the off-hand item and whether Parry is known when it changes (looked at once a
// minute at most).
func (s *Service) logDefense(ctx context.Context, known map[uint32]bool) {
	if time.Since(s.defenseAt) < defenseEvery {
		return
	}
	s.defenseAt = time.Now()
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return
	}
	var off uint32
	shield := false
	for _, it := range snap.Items {
		if it.Bag == 255 && it.BagSlot == slotOffHand {
			off = it.Entry
			if in := s.itemInfo(ctx, it.Entry); in != nil {
				shield = in.Class == itemArmor && in.Subclass == armorShield
			}
		}
	}
	line := fmt.Sprintf("%d %v %v", off, shield, known[spellParry])
	if line == s.defense {
		return
	}
	s.defense = line
	slog.Info("defense", "off_hand", off, "shield", shield, "parry_known", known[spellParry])
}
