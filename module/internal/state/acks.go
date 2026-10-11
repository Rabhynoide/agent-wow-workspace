package state

import (
	"context"
	"log/slog"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/protobuf/types/known/emptypb"

	"wowagent/internal/wire"
)

// Server-driven changes of the character's movement must be acknowledged as the real client does.
// The agent-wow core leaves them to the modules. Without MSG_MOVE_TELEPORT_ACK after a near teleport
// (spirit release to the graveyard, hearthstone on the same map) the server ignores every movement
// packet and keeps the character where it was; without MSG_MOVE_WORLDPORT_ACK a far teleport
// (SMSG_NEW_WORLD) never finishes.

type speedAck struct {
	ack   uint32
	index int  // Obj.Speeds index (walk, run, run back, swim, swim back, flight, flight back)
	extra bool // SMSG_FORCE_RUN_SPEED_CHANGE carries one more byte before the speed
}

var speedAcks = map[uint32]speedAck{
	uint32(opcode.SMSGForceWalkSpeedChange):       {uint32(opcode.CMSGForceWalkSpeedChangeAck), 0, false},
	uint32(opcode.SMSGForceRunSpeedChange):        {uint32(opcode.CMSGForceRunSpeedChangeAck), 1, true},
	uint32(opcode.SMSGForceRunBackSpeedChange):    {uint32(opcode.CMSGForceRunBackSpeedChangeAck), 2, false},
	uint32(opcode.SMSGForceSwimSpeedChange):       {uint32(opcode.CMSGForceSwimSpeedChangeAck), 3, false},
	uint32(opcode.SMSGForceSwimBackSpeedChange):   {uint32(opcode.CMSGForceSwimBackSpeedChangeAck), 4, false},
	uint32(opcode.SMSGForceFlightSpeedChange):     {uint32(opcode.CMSGForceFlightSpeedChangeAck), 5, false},
	uint32(opcode.SMSGForceFlightBackSpeedChange): {uint32(opcode.CMSGForceFlightBackSpeedChangeAck), 6, false},
}

const moveFlagRoot = 0x800

// ack answers the server-driven movement packets; it returns false for other opcodes.
func (s *Service) ack(ctx context.Context, op uint32, payload []byte) bool {
	r := wire.NewReader(payload)
	switch op {
	case uint32(opcode.MSGMoveTeleportAck):
		guid, counter := r.PackedGUID(), r.U32()
		r.U32() // movement flags
		r.U16()
		r.U32() // time
		x, y, z, o := r.F32(), r.F32(), r.F32(), r.F32()
		if r.Err || guid != s.st.self {
			return true
		}
		s.st.mu.Lock()
		me := s.st.obj(s.st.self, TypePlayer)
		me.X, me.Y, me.Z, me.O, me.HasPos = x, y, z, o, true
		s.st.mu.Unlock()
		w := &wire.Writer{}
		w.PackedGUID(guid)
		w.U32(counter)
		w.U32(s.clock(ctx))
		s.send(ctx, op, w.B)
		slog.Info("teleported", "x", x, "y", y, "z", z)
	case uint32(opcode.SMSGNewWorld):
		s.st.Handle(op, payload)
		s.send(ctx, uint32(opcode.MSGMoveWorldportAck), nil)
		slog.Info("changed map", "map", s.st.Map)
	case uint32(opcode.SMSGForceMoveRoot), uint32(opcode.SMSGForceMoveUnroot):
		guid, counter := r.PackedGUID(), r.U32()
		if r.Err || guid != s.st.self {
			return true
		}
		flags, reply := uint32(moveFlagRoot), uint32(opcode.CMSGForceMoveRootAck)
		if op == uint32(opcode.SMSGForceMoveUnroot) {
			flags, reply = 0, uint32(opcode.CMSGForceMoveUnrootAck)
		}
		s.send(ctx, reply, s.moveAck(ctx, guid, counter, flags))
	default:
		sa, ok := speedAcks[op]
		if !ok {
			return false
		}
		guid, counter := r.PackedGUID(), r.U32()
		if sa.extra {
			r.U8()
		}
		speed := r.F32()
		if r.Err || guid != s.st.self {
			return true
		}
		s.st.mu.Lock()
		s.st.obj(s.st.self, TypePlayer).Speeds[sa.index] = speed
		s.st.mu.Unlock()
		w := &wire.Writer{}
		w.B = s.moveAck(ctx, guid, counter, 0)
		w.F32(speed) // must echo the server's speed, or it kicks for cheating
		s.send(ctx, sa.ack, w.B)
	}
	return true
}

// moveAck is the common body of the movement acks: GUID, counter, movement info at our position.
func (s *Service) moveAck(ctx context.Context, guid uint64, counter, flags uint32) []byte {
	s.st.mu.Lock()
	me := s.st.obj(s.st.self, TypePlayer)
	x, y, z, o := me.X, me.Y, me.Z, me.O
	s.st.mu.Unlock()
	w := &wire.Writer{}
	w.PackedGUID(guid)
	w.U32(counter)
	w.U32(flags)
	w.U16(0)
	w.U32(s.clock(ctx))
	w.F32(x)
	w.F32(y)
	w.F32(z)
	w.F32(o)
	w.U32(0) // fall time
	return w.B
}

func (s *Service) clock(ctx context.Context) uint32 {
	if c, err := s.sess.GetClock(ctx, &emptypb.Empty{}); err == nil {
		return c.ClientTimeMs
	}
	return 0
}

func (s *Service) send(ctx context.Context, op uint32, payload []byte) {
	if _, err := s.sess.SendPacket(ctx, &modv1.SendPacketRequest{Opcode: op, Payload: payload}); err != nil {
		slog.Warn("movement ack", "opcode", opcode.WorldName(op), "err", err)
	}
}
