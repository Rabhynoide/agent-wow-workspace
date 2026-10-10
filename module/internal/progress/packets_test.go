package progress

import (
	"context"
	"testing"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"

	"wowagent/internal/wire"
)

func newService() *Service {
	return &Service{trainerList: make(chan []trainerSpell, 1), bought: make(chan buyResult, 8), talents: map[uint32]int{}}
}

func TestTrainerListPacket(t *testing.T) {
	w := &wire.Writer{}
	w.U64(0xF130000177000001)
	w.U32(0) // class trainer
	w.U32(2)
	for _, sp := range []struct {
		id, cost   uint32
		state, lvl uint8
	}{{589, 100, 0, 4}, {17, 100, 1, 6}} {
		w.U32(sp.id)
		w.U8(sp.state)
		w.U32(sp.cost)
		w.U32(0)
		w.U32(0)
		w.U8(sp.lvl)
		w.U32(0)
		w.U32(0)
		w.U32(0)
		w.U32(0)
		w.U32(0)
	}
	w.B = append(w.B, "Hello\x00"...)
	s := newService()
	s.OnPacket(context.Background(), &modv1.WorldPacket{Opcode: uint32(opcode.SMSGTrainerList), Payload: w.B})
	list := <-s.trainerList
	if len(list) != 2 || list[0].spell != 589 || list[0].state != 0 || list[1].spell != 17 || list[1].state != 1 || list[1].reqLevel != 6 {
		t.Fatalf("list = %+v", list)
	}
}

func TestTalentsInfoPacket(t *testing.T) {
	w := &wire.Writer{}
	w.U8(0)  // player
	w.U32(3) // unspent
	w.U8(2)  // two specs
	w.U8(1)  // second one active
	// spec 0: one talent, no glyph slots
	w.U8(1)
	w.U32(100)
	w.U8(4)
	w.U8(0)
	// spec 1: two talents, 6 glyph slots
	w.U8(2)
	w.U32(465) // Spirit Tap, rank index 2 -> 3 ranks
	w.U8(2)
	w.U32(466)
	w.U8(0)
	w.U8(6)
	w.B = append(w.B, make([]byte, 12)...)
	s := newService()
	s.OnPacket(context.Background(), &modv1.WorldPacket{Opcode: uint32(opcode.SMSGTalentsInfo), Payload: w.B})
	if s.talents[465] != 3 || s.talents[466] != 1 || s.talents[100] != 0 {
		t.Fatalf("talents = %v", s.talents)
	}
}
