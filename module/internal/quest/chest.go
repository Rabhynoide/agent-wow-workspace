package quest

import (
	"context"
	"log/slog"
	"os"
	"sync"

	pb "wowagent/gen/questv1"
	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/dbc"
)

// Chests worth a detour (user's request of 2026-10-23: a chest in sight comes before the quest):
// treasure chests that anyone opens. Herbs and veins are chests too, but their lock needs a
// profession; crates and barrels (a few food or drink items) are left alone.
const (
	goChest = 3
	// A treasure chest draws from tens of loot entries (Battered Chest 28-107, Solid Chest 712), a
	// Food Crate or a Water Barrel from 1 to 8.
	chestMinLoot = 10
)

var (
	locksOnce sync.Once
	locks     map[uint32]dbc.Lock
)

func lockTable() map[uint32]dbc.Lock {
	locksOnce.Do(func() {
		dir := os.Getenv("DBC_DIR")
		if dir == "" {
			dir = "/dbc"
		}
		var err error
		if locks, err = dbc.LoadLocks(dir); err != nil {
			slog.Warn("Lock.dbc unavailable: no chests", "err", err)
		}
	})
	return locks
}

// chestKind is what a game object entry is to us: nil when it is not a chest we open.
type chestKind struct {
	name  string
	spell uint32
	loot  uint32
}

func (s *Service) chestKind(ctx context.Context, entry uint32) *chestKind {
	s.mu.Lock()
	k, ok := s.kinds[entry]
	s.mu.Unlock()
	if ok {
		return k
	}
	g, err := s.api.GameObject(ctx, entry)
	if err != nil {
		return nil // not cached: the world database may come back
	}
	k = classify(g, lockTable())
	s.mu.Lock()
	s.kinds[entry] = k
	s.mu.Unlock()
	return k
}

// classify keeps the treasure chests any character opens, without a quest.
func classify(g *worldpb.GameObjectInfo, locks map[uint32]dbc.Lock) *chestKind {
	if g.Type != goChest || g.QuestId != 0 || g.LootItems < chestMinLoot {
		return nil
	}
	k := &chestKind{name: g.Name, loot: g.LootItems}
	if g.LockId != 0 {
		l, ok := locks[g.LockId]
		if !ok {
			return nil
		}
		if k.spell = l.OpenSpell(); k.spell == 0 {
			return nil
		}
	}
	return k
}

// Chests lists the chests in sight that we may open, nearest first.
func (s *Service) Chests(ctx context.Context, r *pb.ChestsRequest) (*pb.ChestsResponse, error) {
	radius := r.Radius
	if radius <= 0 {
		radius = 40
	}
	vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "gameobject", Radius: radius, Max: 40})
	if err != nil {
		return nil, err
	}
	out := &pb.ChestsResponse{}
	for _, o := range vis.Objs {
		if k := s.chestKind(ctx, o.Entry); k != nil {
			out.Chests = append(out.Chests, &pb.Chest{Guid: o.Guid, Entry: o.Entry, Name: k.name, X: o.X, Y: o.Y, Z: o.Z,
				Dist: o.Dist, OpenSpell: k.spell, LootItems: k.loot})
		}
	}
	return out, nil
}
