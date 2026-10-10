// Package town handles vendors: selling junk to free bag space.
package town

import (
	"context"
	"fmt"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	statepb "wowagent/gen/statev1"
	pb "wowagent/gen/townv1"
	"wowagent/internal/api"
	"wowagent/internal/moveio"
	"wowagent/internal/wire"
)

type Service struct {
	pb.UnimplementedTownServer
	s   modv1.SessionClient
	api api.C
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	pb.RegisterTownServer(g, &Service{s: s, api: api.C{S: s}})
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func fail(format string, a ...any) (*pb.SellResult, error) {
	return &pb.SellResult{Reason: fmt.Sprintf(format, a...)}, nil
}

func (s *Service) SellJunk(ctx context.Context, r *pb.SellRequest) (*pb.SellResult, error) {
	snap, err := s.api.Snapshot(ctx)
	if err != nil || snap.Self == nil {
		return fail("state unavailable: %v", err)
	}
	// Sell only from the backpack (slots 23..38) and equipped bags (slots 19..22), never the bank.
	bags := map[uint64]bool{}
	for _, it := range snap.Items {
		if it.Slot >= 19 && it.Slot <= 22 {
			bags[it.Guid] = true
		}
	}
	quality := map[uint32]uint32{}
	var junk []*statepb.Item
	for _, it := range snap.Items {
		inPack := it.Slot >= 23 && it.Slot <= 38
		if !inPack && !(it.Slot == -1 && bags[it.Container]) {
			continue
		}
		q, ok := quality[it.Entry]
		if !ok {
			info, err := s.api.ItemInfo(ctx, it.Entry)
			if err != nil {
				continue
			}
			q = info.Quality
			quality[it.Entry] = q
			if info.SellPrice == 0 {
				quality[it.Entry] = 99
			}
		}
		if q == 0 {
			junk = append(junk, it)
		}
	}
	res := &pb.SellResult{Ok: true, FreeSlots: snap.FreeSlots}
	if len(junk) == 0 || r.DryRun {
		res.Sold = uint32(len(junk))
		return res, nil
	}
	near, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 10, Max: 5, NpcFlags: 128})
	if err != nil {
		return fail("nearby: %v", err)
	}
	var vendor *statepb.Obj
	for _, o := range near.Objs {
		if o.Hp > 0 && o.Dist <= 8 {
			vendor = o
			break
		}
	}
	if vendor == nil {
		return fail("no vendor within 8 yards; walk to one first")
	}
	res.Vendor = fmt.Sprint(vendor.Entry)
	money := snap.Self.Money
	for _, it := range junk {
		w := &wire.Writer{}
		w.U64(vendor.Guid)
		w.U64(it.Guid)
		w.U32(it.Count)
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGSellItem), w.B); err != nil {
			return fail("send: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(800 * time.Millisecond)
	after, err := s.api.Snapshot(ctx)
	if err == nil && after.Self != nil {
		res.Copper = after.Self.Money - money
		res.FreeSlots = after.FreeSlots
	}
	res.Sold = uint32(len(junk))
	return res, nil
}
