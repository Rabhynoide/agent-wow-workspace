// Package quest talks to quest givers and interactive objects.
package quest

import (
	"context"
	"fmt"
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
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	pb.RegisterQuestServer(g, &Service{s: s, api: api.C{S: s}, loot: loot.NewTracker()})
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	if p.GetOpcode() == uint32(opcode.SMSGLootResponse) {
		s.loot.OnPacket(p.GetPayload())
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
