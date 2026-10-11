// Package town handles vendors: selling junk and useless items to free bag space and make money.
package town

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	statepb "wowagent/gen/statev1"
	pb "wowagent/gen/townv1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/api"
	"wowagent/internal/moveio"
	"wowagent/internal/sellrule"
	"wowagent/internal/wire"
)

// interact is the server's interaction distance (INTERACTION_DISTANCE 5.5), with a small margin.
const interact = 5.0

type Service struct {
	pb.UnimplementedTownServer
	s   modv1.SessionClient
	api api.C

	mu    sync.Mutex
	items map[uint32]*worldpb.ItemInfo // item_template rows never change
	quest map[uint32][]uint32          // quest id -> required item entries

	lists chan []byte // SMSG_LIST_INVENTORY payloads
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	pb.RegisterTownServer(g, &Service{s: s, api: api.C{S: s}, items: map[uint32]*worldpb.ItemInfo{}, quest: map[uint32][]uint32{},
		lists: make(chan []byte, 4)})
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func fail(format string, a ...any) (*pb.SellResult, error) {
	return &pb.SellResult{Reason: fmt.Sprintf(format, a...)}, nil
}

func (s *Service) itemInfo(ctx context.Context, entry uint32) *worldpb.ItemInfo {
	s.mu.Lock()
	it, ok := s.items[entry]
	s.mu.Unlock()
	if ok {
		return it
	}
	it, err := s.api.ItemInfo(ctx, entry)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	s.items[entry] = it
	s.mu.Unlock()
	return it
}

// questItems lists the item entries required by the quests in the log (they must not be sold).
func (s *Service) questItems(ctx context.Context, quests []*statepb.Quest) map[uint32]bool {
	out := map[uint32]bool{}
	for _, q := range quests {
		s.mu.Lock()
		entries, ok := s.quest[q.Id]
		s.mu.Unlock()
		if !ok {
			info, err := s.api.QuestInfo(ctx, q.Id)
			if err != nil {
				continue
			}
			for _, o := range info.Objectives {
				if o.Kind == "item" {
					entries = append(entries, o.Entry)
				}
			}
			s.mu.Lock()
			s.quest[q.Id] = entries
			s.mu.Unlock()
		}
		for _, e := range entries {
			out[e] = true
		}
	}
	return out
}

// sellable lists the stacks in the backpack and the equipped bags (never the bank).
func sellable(items []*statepb.Item) []*statepb.Item {
	var out []*statepb.Item
	for _, it := range items {
		pack := it.Bag == 255 && it.BagSlot >= 23 && it.BagSlot <= 38
		bag := it.Bag >= 19 && it.Bag <= 22
		if pack || bag {
			out = append(out, it)
		}
	}
	return out
}

func (s *Service) SellJunk(ctx context.Context, r *pb.SellRequest) (*pb.SellResult, error) {
	snap, err := s.api.Snapshot(ctx)
	if err != nil || snap.Self == nil {
		return fail("state unavailable: %v", err)
	}
	me := snap.Self
	known, err := s.api.Spells(ctx)
	if err != nil {
		return fail("spells unavailable: %v", err)
	}
	c := sellrule.Ctx{Class: me.ClassId, Race: me.Race, Level: me.Level, Known: known,
		Equipped: map[int]*worldpb.ItemInfo{}, QuestItems: s.questItems(ctx, snap.Quests)}
	for _, it := range snap.Items {
		if it.Bag == 255 && it.BagSlot <= 18 {
			c.Equipped[int(it.BagSlot)] = s.itemInfo(ctx, it.Entry)
		}
	}
	stacks := sellable(snap.Items)
	in := make([]sellrule.Stack, len(stacks))
	for i, it := range stacks {
		in[i] = sellrule.Stack{Entry: it.Entry, Count: it.Count, Info: s.itemInfo(ctx, it.Entry)}
	}
	// Plan keeps the input order only within equal verdicts: map decisions back by index.
	byEntry := map[uint32][]*statepb.Item{}
	for _, it := range stacks {
		byEntry[it.Entry] = append(byEntry[it.Entry], it)
	}
	plan := sellrule.Plan(in, c)

	res := &pb.SellResult{Ok: true, FreeSlots: snap.FreeSlots}
	type sale struct {
		item *statepb.Item
		line *pb.SellLine
	}
	var sales []sale
	free := snap.FreeSlots
	for _, d := range plan {
		line := &pb.SellLine{Entry: d.Entry, Count: d.Count, Copper: d.Copper, Action: "keep", Reason: d.Reason}
		if d.Info != nil {
			line.Name = d.Info.Name
		}
		if d.Sell && (r.MinFree == 0 || free < r.MinFree) {
			line.Action = "sell"
			list := byEntry[d.Entry]
			if len(list) == 0 {
				continue
			}
			// Stacks of one entry are interchangeable: take the one with this count.
			pick := 0
			for i, it := range list {
				if it.Count == d.Count {
					pick = i
					break
				}
			}
			sales = append(sales, sale{list[pick], line})
			byEntry[d.Entry] = append(list[:pick], list[pick+1:]...)
			free++
			res.Lines = append(res.Lines, line)
		} else if r.ShowKept {
			res.Lines = append(res.Lines, line)
		}
	}
	for _, sl := range sales {
		res.Sold++
		res.Copper += sl.line.Copper
	}
	if len(sales) == 0 || r.DryRun {
		return res, nil
	}

	near, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 12, Max: 6, NpcFlags: 128})
	if err != nil {
		return fail("nearby: %v", err)
	}
	var vendor *statepb.Obj
	closest := float32(1e9)
	for _, o := range near.Objs {
		if o.Hp > 0 && o.Dist <= interact && (vendor == nil || o.Dist < vendor.Dist) {
			vendor = o
		}
		closest = min(closest, o.Dist)
	}
	if vendor == nil {
		if closest < 1e9 {
			return fail("nearest vendor is %.1f yards away; the server needs <= 5.5", closest)
		}
		return fail("no vendor nearby; walk to one first")
	}
	res.Vendor = fmt.Sprint(vendor.Entry)
	money := me.Money
	for _, sl := range sales {
		w := &wire.Writer{}
		w.U64(vendor.Guid)
		w.U64(sl.item.Guid)
		w.U32(sl.item.Count)
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGSellItem), w.B); err != nil {
			return fail("send: %v", err)
		}
		select {
		case <-ctx.Done():
			return fail("cancelled")
		case <-time.After(150 * time.Millisecond):
		}
	}
	time.Sleep(700 * time.Millisecond)
	after, err := s.api.Snapshot(ctx)
	if err != nil || after.Self == nil {
		return res, nil
	}
	// Report what the server really did: sold stacks leave the bags.
	gone := map[uint64]bool{}
	left := map[uint64]bool{}
	for _, it := range sellable(after.Items) {
		left[it.Guid] = true
	}
	res.Sold, res.Copper = 0, after.Self.Money-money
	for _, sl := range sales {
		if !left[sl.item.Guid] {
			gone[sl.item.Guid] = true
			res.Sold++
		}
	}
	for _, sl := range sales {
		if !gone[sl.item.Guid] {
			sl.line.Action, sl.line.Reason = "failed", "the vendor did not take it"
		}
	}
	res.FreeSlots = after.FreeSlots
	sort.SliceStable(res.Lines, func(i, j int) bool { return res.Lines[i].Action == "sell" && res.Lines[j].Action != "sell" })
	return res, nil
}

// npcRepair is UNIT_NPC_FLAG_REPAIR.
const npcRepair = 0x1000

// Repair repairs every item (equipped and in bags) at a repair vendor within 5 yards.
func (s *Service) Repair(ctx context.Context, _ *emptypb.Empty) (*pb.RepairResult, error) {
	before, err := s.api.Brief(ctx)
	if err != nil || before.Self == nil {
		return &pb.RepairResult{Reason: fmt.Sprintf("state unavailable: %v", err)}, nil
	}
	if before.DurabilityPct >= 100 {
		return &pb.RepairResult{Ok: true, DurabilityPct: 100, Reason: "nothing to repair"}, nil
	}
	near, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 12, Max: 6, NpcFlags: npcRepair})
	if err != nil {
		return &pb.RepairResult{Reason: "nearby: " + err.Error()}, nil
	}
	var npc *statepb.Obj
	for _, o := range near.Objs {
		if o.Hp > 0 && o.Dist <= interact && (npc == nil || o.Dist < npc.Dist) {
			npc = o
		}
	}
	if npc == nil {
		return &pb.RepairResult{Reason: "no repair vendor within 5 yards"}, nil
	}
	w := &wire.Writer{}
	w.U64(npc.Guid)
	w.U64(0) // 0 = every item
	w.U8(0)  // not from the guild bank
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGRepairItem), w.B); err != nil {
		return &pb.RepairResult{Reason: "send: " + err.Error()}, nil
	}
	time.Sleep(800 * time.Millisecond)
	after, err := s.api.Brief(ctx)
	if err != nil || after.Self == nil {
		return &pb.RepairResult{Ok: true, Vendor: fmt.Sprint(npc.Entry)}, nil
	}
	res := &pb.RepairResult{Ok: after.DurabilityPct > before.DurabilityPct, Vendor: fmt.Sprint(npc.Entry), DurabilityPct: after.DurabilityPct}
	if before.Self.Money > after.Self.Money {
		res.Copper = before.Self.Money - after.Self.Money
	}
	if !res.Ok {
		res.Reason = "durability unchanged (not enough money?)"
	}
	return res, nil
}
