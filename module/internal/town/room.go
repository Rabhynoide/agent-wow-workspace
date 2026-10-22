package town

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/agent-wow/agent-wow/pkg/opcode"

	statepb "wowagent/gen/statev1"
	pb "wowagent/gen/townv1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/moveio"
	"wowagent/internal/sellrule"
)

// MakeRoom frees a bag slot for an item about to be taken when the bags are full, far from a vendor
// (user's rule of 2026-10-23: no trip to town to sell; sales happen when we are there anyway). It
// throws away the cheapest stack that the sell rules would sell, never a quest item, and only for
// an item worth more than it, a quest item, an upgrade or a rare one.
func (s *Service) MakeRoom(ctx context.Context, r *pb.RoomRequest) (*pb.RoomResult, error) {
	snap, err := s.api.Snapshot(ctx)
	if err != nil || snap.Self == nil {
		return &pb.RoomResult{Reason: fmt.Sprintf("state unavailable: %v", err)}, nil
	}
	if snap.FreeSlots > 0 {
		return &pb.RoomResult{Ok: true, FreeSlots: snap.FreeSlots}, nil
	}
	var in *worldpb.ItemInfo
	if r.Entry != 0 {
		in = s.itemInfo(ctx, r.Entry)
	}
	count := max(r.Count, 1)
	if r.Entry != 0 && in != nil && in.Stackable > 1 {
		for _, it := range sellable(snap.Items) {
			if it.Entry == r.Entry && it.Count+count <= in.Stackable {
				return &pb.RoomResult{Ok: true, Reason: "goes onto a stack"}, nil
			}
		}
	}
	c, stacks, list, err := s.rules(ctx, snap)
	if err != nil {
		return &pb.RoomResult{Reason: err.Error()}, nil
	}
	// The cheapest stack the rules would sell (the whole plan: extra food depends on the other stacks).
	var cheapest *sellrule.Decision
	for _, d := range sellrule.Plan(list, c) {
		if d.Sell && (cheapest == nil || d.Copper < cheapest.Copper) {
			cheapest = &d
		}
	}
	if cheapest == nil {
		return &pb.RoomResult{Reason: "bags full of items the rules keep"}, nil
	}
	cheapestCopper := cheapest.Copper
	if r.Entry != 0 {
		if in == nil {
			return &pb.RoomResult{Reason: fmt.Sprintf("item %d unknown", r.Entry)}, nil
		}
		incoming := sellrule.Stack{Entry: r.Entry, Count: count, Info: in}
		if value := in.SellPrice * count; !sellrule.Prized(incoming, c) && value <= cheapestCopper {
			return &pb.RoomResult{Reason: fmt.Sprintf("%s (%d copper) is worth no more than the cheapest junk (%d copper)", in.Name, value, cheapestCopper)}, nil
		}
	}
	var it *statepb.Item
	for _, st := range stacks {
		if st.Entry == cheapest.Entry && (it == nil || st.Count == cheapest.Count) {
			it = st
		}
	}
	// CMSG_DESTROYITEM: bag, slot, count (0 = the whole stack), three unused bytes.
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGDestroyitem), []byte{byte(it.Bag), byte(it.BagSlot), 0, 0, 0, 0}); err != nil {
		return &pb.RoomResult{Reason: "send: " + err.Error()}, nil
	}
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if after, err := s.api.Brief(ctx); err == nil && after.FreeSlots > 0 {
			slog.Info("destroyed to make room", "entry", it.Entry, "count", it.Count, "copper", cheapestCopper, "for", r.Entry)
			return &pb.RoomResult{Ok: true, Destroyed: it.Entry, DestroyedCopper: cheapestCopper, FreeSlots: after.FreeSlots}, nil
		}
	}
	return &pb.RoomResult{Reason: fmt.Sprintf("item %d still in the bags after destroying it", it.Entry)}, nil
}
