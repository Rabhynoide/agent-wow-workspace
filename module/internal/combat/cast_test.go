package combat

import (
	"context"
	"testing"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"

	"wowagent/internal/wire"
)

func castFailed(cnt uint8, spell uint32) *modv1.WorldPacket {
	w := &wire.Writer{}
	w.U8(cnt)
	w.U32(spell)
	w.U8(40)
	return &modv1.WorldPacket{Opcode: uint32(opcode.SMSGCastFailed), Payload: w.B}
}

func TestCastFailureIsMatchedToOurCast(t *testing.T) {
	s := &Service{failures: map[uint8]castFailure{}}
	s.OnPacket(context.Background(), castFailed(0, 585))   // server-side cast: ignored
	s.OnPacket(context.Background(), castFailed(7, 55164)) // another of our casts
	if s.failedCast(0, 585) || s.failedCast(7, 585) {
		t.Fatal("foreign failures must not count as a failed Smite")
	}
	s.OnPacket(context.Background(), castFailed(8, 585))
	if !s.failedCast(8, 585) {
		t.Fatal("our failed Smite (count 8) not detected")
	}
}
