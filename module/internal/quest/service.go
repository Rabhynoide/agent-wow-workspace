// Package quest talks to quest givers and interactive objects.
package quest

import (
	"context"
	"fmt"
	"sync"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/questv1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/api"
	"wowagent/internal/loot"
	"wowagent/internal/moveio"
	"wowagent/internal/wire"
)

const interact = 5.0

type Service struct {
	pb.UnimplementedQuestServer
	s    modv1.SessionClient
	api  api.C
	loot *loot.Tracker

	mu        sync.Mutex
	castCount uint8
	failed    chan castFailed // SMSG_CAST_FAILED of our item uses
}

type castFailed struct {
	count  uint8
	spell  uint32
	result uint8
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	pb.RegisterQuestServer(g, &Service{s: s, api: api.C{S: s}, loot: loot.NewTracker(), failed: make(chan castFailed, 8)})
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	switch p.GetOpcode() {
	case uint32(opcode.SMSGLootResponse):
		s.loot.OnPacket(p.GetPayload())
	case uint32(opcode.SMSGCastFailed):
		if b := p.GetPayload(); len(b) >= 6 {
			f := castFailed{count: b[0], spell: uint32(b[1]) | uint32(b[2])<<8 | uint32(b[3])<<16 | uint32(b[4])<<24, result: b[5]}
			select {
			case s.failed <- f:
			default:
			}
		}
	}
	return &emptypb.Empty{}, nil
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func fail(format string, a ...any) (*pb.QuestResult, error) {
	return &pb.QuestResult{Reason: fmt.Sprintf(format, a...)}, nil
}

func (s *Service) find(ctx context.Context, kind string, entry uint32) (*statepb.Obj, error) {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: kind, Entry: entry, Radius: 40, Max: 1})
	if err != nil {
		return nil, err
	}
	if len(r.Objs) == 0 {
		return nil, fmt.Errorf("entry %d not visible within 40 yards", entry)
	}
	o := r.Objs[0]
	if o.Dist > interact {
		return nil, fmt.Errorf("entry %d is %.1f yards away (need <= %.0f)", entry, o.Dist, interact)
	}
	return o, nil
}

func (s *Service) inLog(ctx context.Context, id uint32) (bool, error) {
	snap, err := s.api.Brief(ctx)
	if err != nil {
		return false, err
	}
	for _, q := range snap.Quests {
		if q.Id == id {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) waitLog(ctx context.Context, id uint32, want bool) bool {
	for i := 0; i < 40; i++ {
		if has, err := s.inLog(ctx, id); err == nil && has == want {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (s *Service) Accept(ctx context.Context, r *pb.AcceptRequest) (*pb.QuestResult, error) {
	if has, err := s.inLog(ctx, r.QuestId); err == nil && has {
		return &pb.QuestResult{Ok: true, Reason: "already in log"}, nil
	}
	npc, err := s.find(ctx, "unit", r.NpcEntry)
	if err != nil {
		return fail("%v", err)
	}
	w := &wire.Writer{}
	w.U64(npc.Guid)
	w.U32(r.QuestId)
	w.U32(0)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGQuestgiverAcceptQuest), w.B); err != nil {
		return fail("send: %v", err)
	}
	if !s.waitLog(ctx, r.QuestId, true) {
		return fail("quest %d not added to the log (prerequisites, level or already done?)", r.QuestId)
	}
	return &pb.QuestResult{Ok: true}, nil
}

func (s *Service) TurnIn(ctx context.Context, r *pb.TurnInRequest) (*pb.QuestResult, error) {
	if has, err := s.inLog(ctx, r.QuestId); err == nil && !has {
		return fail("quest %d is not in the log", r.QuestId)
	}
	npc, err := s.find(ctx, "unit", r.NpcEntry)
	if err != nil {
		return fail("%v", err)
	}
	w := &wire.Writer{}
	w.U64(npc.Guid)
	w.U32(r.QuestId)
	w.U32(r.Reward)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGQuestgiverChooseReward), w.B); err != nil {
		return fail("send: %v", err)
	}
	if !s.waitLog(ctx, r.QuestId, false) {
		return fail("quest %d still in log: objectives incomplete or wrong reward index", r.QuestId)
	}
	return &pb.QuestResult{Ok: true}, nil
}

func (s *Service) Use(ctx context.Context, r *pb.UseRequest) (*pb.QuestResult, error) {
	o, err := s.find(ctx, "gameobject", r.Entry)
	if err != nil {
		return fail("%v", err)
	}
	s.loot.Drain()
	w := &wire.Writer{}
	w.U64(o.Guid)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGGameobjUse), w.B); err != nil {
		return fail("send: %v", err)
	}
	if !r.Loot {
		return &pb.QuestResult{Ok: true}, nil
	}
	res, err := s.loot.Take(ctx, s.s, o.Guid, false, 1500*time.Millisecond)
	if err != nil {
		// Locked objects need the client to cast the opening spell (22810) on them.
		s.loot.Drain()
		cw := &wire.Writer{}
		cw.U8(1)
		cw.U32(22810)
		cw.U8(0)
		cw.U32(0x800)
		cw.PackedGUID(o.Guid)
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGCastSpell), cw.B); err != nil {
			return fail("cast: %v", err)
		}
		if res, err = s.loot.Take(ctx, s.s, o.Guid, false, loot.DefaultWait); err != nil {
			return fail("loot: %v", err)
		}
	}
	return &pb.QuestResult{Ok: true, Items: res.Items}, nil
}

// Explore tells the server the character stands inside an area trigger (the client normally sends
// it on entering the zone); the server checks the distance and credits exploration quests.
func (s *Service) Explore(ctx context.Context, r *pb.ExploreRequest) (*pb.QuestResult, error) {
	w := &wire.Writer{}
	w.U32(r.Trigger)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGAreatrigger), w.B); err != nil {
		return fail("send: %v", err)
	}
	return &pb.QuestResult{Ok: true}, nil
}

// Target flags of SpellCastTargets.
const (
	targetUnit       = 0x2
	targetDest       = 0x40
	targetGameObject = 0x800
)

// useRange is how close the target must be: quest item spells reach 5 to 30 yards or more.
const useRange = 30.0

// UseItem uses a bag item (CMSG_USE_ITEM) on the nearest visible target with the given entry, also
// giving its position for spells aimed at a place. It waits briefly for a cast failure.
func (s *Service) UseItem(ctx context.Context, r *pb.UseItemRequest) (*pb.QuestResult, error) {
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return fail("%v", err)
	}
	var item *statepb.Item
	for _, it := range snap.Items {
		if it.Entry == r.ItemEntry && it.Bag != 0 {
			item = it
			break
		}
	}
	if item == nil {
		return fail("item %d not in the bags", r.ItemEntry)
	}
	spell := r.Spell
	if spell == 0 {
		info, err := s.api.ItemInfo(ctx, r.ItemEntry)
		if err != nil {
			return fail("%v", err)
		}
		if spell = info.UseSpell; spell == 0 {
			return fail("item %d has no on-use spell", r.ItemEntry)
		}
	}
	var target *statepb.Obj
	if r.TargetEntry != 0 {
		kind := r.TargetKind
		if kind == "" || kind == "creature" {
			kind = "unit"
		}
		vis, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: kind, Entry: r.TargetEntry, Radius: 40, Max: 1})
		if err != nil {
			return fail("%v", err)
		}
		if len(vis.Objs) == 0 {
			return fail("%s %d not visible within 40 yards", kind, r.TargetEntry)
		}
		if target = vis.Objs[0]; target.Dist > useRange {
			return fail("%s %d is %.1f yards away (need <= %.0f)", kind, r.TargetEntry, target.Dist, useRange)
		}
	}
	s.mu.Lock()
	if s.castCount++; s.castCount == 0 {
		s.castCount = 1
	}
	cnt := s.castCount
	s.mu.Unlock()
	for len(s.failed) > 0 {
		<-s.failed
	}
	w := &wire.Writer{}
	w.U8(uint8(item.Bag))
	w.U8(uint8(item.BagSlot))
	w.U8(cnt)
	w.U32(spell)
	w.U64(item.Guid)
	w.U32(0) // glyph index
	w.U8(0)  // cast flags
	switch {
	case target == nil:
		w.U32(0)
	default:
		flag := uint32(targetUnit)
		if target.Kind == "gameobject" {
			flag = targetGameObject
		}
		w.U32(flag | targetDest)
		w.PackedGUID(target.Guid)
		w.PackedGUID(0) // destination transport
		w.F32(target.X)
		w.F32(target.Y)
		w.F32(target.Z)
	}
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGUseItem), w.B); err != nil {
		return fail("send: %v", err)
	}
	timer := time.NewTimer(1500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case f := <-s.failed:
			if f.count == cnt {
				return fail("cast of spell %d failed (SpellCastResult %d)", f.spell, f.result)
			}
		case <-timer.C:
			return &pb.QuestResult{Ok: true}, nil
		case <-ctx.Done():
			return fail("%v", ctx.Err())
		}
	}
}
