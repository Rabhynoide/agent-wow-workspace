package state

import (
	"context"
	"math"
	"os"
	"sort"
	"strconv"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/statev1"
)

// Service exposes the store over gRPC.
type Service struct {
	pb.UnimplementedStateServer
	st *Store
}

func Register(s grpc.ServiceRegistrar, _ modv1.SessionClient) {
	guid, _ := strconv.ParseUint(os.Getenv("AGENT_WOW_CHARACTER_GUID"), 10, 64)
	pb.RegisterStateServer(s, &Service{st: NewStore(guid)})
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	s.st.Handle(p.GetOpcode(), p.GetPayload())
	return &emptypb.Empty{}, nil
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *Service) SetPos(_ context.Context, r *pb.SetPosRequest) (*emptypb.Empty, error) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	o := s.st.obj(s.st.self, TypePlayer)
	o.X, o.Y, o.Z, o.O, o.HasPos = r.X, r.Y, r.Z, r.O, true
	if r.Map != 0 {
		s.st.Map = r.Map
	}
	return &emptypb.Empty{}, nil
}

func (s *Service) Snapshot(context.Context, *emptypb.Empty) (*pb.SnapshotResponse, error) {
	st := s.st
	st.mu.Lock()
	defer st.mu.Unlock()
	resp := &pb.SnapshotResponse{Objects: uint32(len(st.objs)), Packets: st.Packets}
	me := st.objs[st.self]
	if me == nil {
		return resp, nil
	}
	resp.Self = st.selfInfo(me)
	resp.Quests = questLog(me)
	resp.FreeSlots = st.freeSlots(me)
	slotOf := map[uint64]int32{}
	for slot := 0; slot <= slotPackLast; slot++ {
		if g := me.guid(uint16(fInvSlots + 2*slot)); g != 0 {
			slotOf[g] = int32(slot)
		}
	}
	for _, o := range st.objs {
		if o.Type != TypeItem && o.Type != TypeContainer {
			continue
		}
		it := &pb.Item{Guid: o.Guid, Entry: o.u(fEntry), Count: o.u(fItemCount), Slot: -1, Container: o.guid(fItemHolder)}
		if sl, ok := slotOf[o.Guid]; ok {
			it.Slot = sl
		}
		resp.Items = append(resp.Items, it)
	}
	sort.Slice(resp.Items, func(i, j int) bool { return resp.Items[i].Slot < resp.Items[j].Slot })
	return resp, nil
}

// Brief is Snapshot without the item list.
func (s *Service) Brief(context.Context, *emptypb.Empty) (*pb.BriefResponse, error) {
	st := s.st
	st.mu.Lock()
	defer st.mu.Unlock()
	me := st.objs[st.self]
	if me == nil {
		return &pb.BriefResponse{}, nil
	}
	return &pb.BriefResponse{Self: st.selfInfo(me), Quests: questLog(me), FreeSlots: st.freeSlots(me)}, nil
}

func (st *Store) selfInfo(me *Obj) *pb.SelfInfo {
	pt := (me.u(fUnitBytes0) >> 24) & 0xff
	return &pb.SelfInfo{
		Guid: me.Guid, Map: st.Map, X: me.X, Y: me.Y, Z: me.Z, O: me.O,
		Level: me.u(fUnitLevel), Hp: me.u(fUnitHealth), MaxHp: me.u(fUnitMaxHP),
		Power: me.u(fUnitPower1 + uint16(pt)), MaxPower: me.u(fUnitMaxPow1 + uint16(pt)), PowerType: pt,
		Xp: me.u(fXP), NextXp: me.u(fNextXP), Money: me.u(fCoinage),
		Target: me.guid(fUnitTarget), Ghost: me.u(fPlayerFlags)&0x10 != 0,
		Dead:     (me.u(fUnitHealth) == 0 && me.u(fUnitMaxHP) > 0) || me.u(fPlayerFlags)&0x10 != 0,
		RunSpeed: me.Speeds[1], UnitFlags: me.u(fUnitFlags),
	}
}

// questLog reads the 25 quest log entries (id, state, 4 x uint16 counters over two fields, time).
func questLog(me *Obj) []*pb.Quest {
	var out []*pb.Quest
	for i := 0; i < 25; i++ {
		base := uint16(fQuestLog + 5*i)
		if id := me.u(base); id != 0 {
			c0, c1 := me.u(base+2), me.u(base+3)
			out = append(out, &pb.Quest{
				Id: id, State: me.u(base + 1),
				Counts: []uint32{c0 & 0xffff, c0 >> 16, c1 & 0xffff, c1 >> 16},
			})
		}
	}
	return out
}

// freeSlots counts empty backpack slots plus empty slots of the equipped bags.
func (st *Store) freeSlots(me *Obj) uint32 {
	var free uint32
	for slot := slotPackFirst; slot <= slotPackLast; slot++ {
		if me.guid(uint16(fInvSlots+2*slot)) == 0 {
			free++
		}
	}
	for slot := slotBagFirst; slot <= slotBagLast; slot++ {
		g := me.guid(uint16(fInvSlots + 2*slot))
		bag := st.objs[g]
		if g == 0 || bag == nil {
			continue
		}
		n := min(bag.u(fBagSlots), 36)
		for i := uint32(0); i < n; i++ {
			if bag.guid(uint16(fBagSlot1+2*i)) == 0 {
				free++
			}
		}
	}
	return free
}

func kindOf(t uint8) string {
	switch t {
	case TypeUnit:
		return "unit"
	case TypePlayer:
		return "player"
	case TypeGameObject:
		return "gameobject"
	case TypeItem, TypeContainer:
		return "item"
	}
	return "object"
}

func (o *Obj) toPB(cx, cy, cz float32) *pb.Obj {
	d := float32(math.Sqrt(float64((o.X-cx)*(o.X-cx) + (o.Y-cy)*(o.Y-cy) + (o.Z-cz)*(o.Z-cz))))
	return &pb.Obj{
		Guid: o.Guid, Entry: o.u(fEntry), Kind: kindOf(o.Type), X: o.X, Y: o.Y, Z: o.Z, Dist: d,
		Level: o.u(fUnitLevel), Hp: o.u(fUnitHealth), MaxHp: o.u(fUnitMaxHP), Faction: o.u(fUnitFaction),
		UnitFlags: o.u(fUnitFlags), NpcFlags: o.u(fUnitNpc), DynFlags: o.u(fUnitDyn), Target: o.guid(fUnitTarget),
	}
}

func (s *Service) Nearby(_ context.Context, r *pb.NearbyRequest) (*pb.NearbyResponse, error) {
	st := s.st
	st.mu.Lock()
	defer st.mu.Unlock()
	me := st.objs[st.self]
	if me == nil || !me.HasPos {
		return nil, status.Error(codes.FailedPrecondition, "self position unknown")
	}
	kind := r.Kind
	if kind == "" {
		kind = "unit"
	}
	var out []*pb.Obj
	for _, o := range st.objs {
		if o.Guid == st.self || !o.HasPos || (kind != "any" && kindOf(o.Type) != kind) {
			continue
		}
		if r.Entry != 0 && o.u(fEntry) != r.Entry {
			continue
		}
		if r.AliveOnly && (o.Type == TypeUnit || o.Type == TypePlayer) && o.u(fUnitHealth) == 0 {
			continue
		}
		if r.NpcFlags != 0 && o.u(fUnitNpc)&r.NpcFlags == 0 {
			continue
		}
		p := o.toPB(me.X, me.Y, me.Z)
		if r.Radius > 0 && p.Dist > r.Radius {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dist < out[j].Dist })
	total := uint32(len(out))
	max := int(r.Max)
	if max == 0 {
		max = 5
	}
	if len(out) > max {
		out = out[:max]
	}
	return &pb.NearbyResponse{Objs: out, Total: total}, nil
}

func (s *Service) GetObj(_ context.Context, r *pb.GetObjRequest) (*pb.Obj, error) {
	st := s.st
	st.mu.Lock()
	defer st.mu.Unlock()
	o := st.objs[r.Guid]
	if o == nil {
		return nil, status.Error(codes.NotFound, "unknown guid")
	}
	var cx, cy, cz float32
	if me := st.objs[st.self]; me != nil {
		cx, cy, cz = me.X, me.Y, me.Z
	}
	return o.toPB(cx, cy, cz), nil
}

func (s *Service) Spells(context.Context, *emptypb.Empty) (*pb.SpellsResponse, error) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	out := &pb.SpellsResponse{}
	for id := range s.st.Spells {
		out.Spells = append(out.Spells, id)
	}
	sort.Slice(out.Spells, func(i, j int) bool { return out.Spells[i] < out.Spells[j] })
	return out, nil
}
