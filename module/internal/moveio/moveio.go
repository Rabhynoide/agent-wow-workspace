// Package moveio sends client movement packets.
package moveio

import (
	"context"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"wowagent/internal/wire"
)

// Send writes a MSG_MOVE_* packet for the character (packed GUID, flags, time, position).
func Send(ctx context.Context, s modv1.SessionClient, guid uint64, op uint32, flags uint32, x, y, z, o float32) error {
	clock, err := s.GetClock(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	w := &wire.Writer{}
	w.PackedGUID(guid)
	w.U32(flags)
	w.U16(0)
	w.U32(clock.ClientTimeMs)
	w.F32(x)
	w.F32(y)
	w.F32(z)
	w.F32(o)
	w.U32(0) // fall time
	_, err = s.SendPacket(ctx, &modv1.SendPacketRequest{Opcode: op, Payload: w.B})
	return err
}

// Packet sends an arbitrary client packet.
func Packet(ctx context.Context, s modv1.SessionClient, op uint32, payload []byte) error {
	_, err := s.SendPacket(ctx, &modv1.SendPacketRequest{Opcode: op, Payload: payload})
	return err
}
