// Package loot opens a corpse or chest and takes everything.
package loot

import (
	"context"
	"fmt"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"

	"wowagent/internal/wire"
)

type response struct {
	guid  uint64
	gold  uint32
	slots []uint8
	items []uint32
	err   bool
}

// Tracker receives SMSG_LOOT_RESPONSE packets.
type Tracker struct {
	ch   chan response
	Wait time.Duration
}

func NewTracker() *Tracker { return &Tracker{ch: make(chan response, 8), Wait: 8 * time.Second} }

func (t *Tracker) OnPacket(payload []byte) {
	r := wire.NewReader(payload)
	var resp response
	resp.guid = r.U64()
	typ := r.U8()
	if r.Left() == 0 || typ == 0 {
		resp.err = true
	} else {
		resp.gold = r.U32()
		n := int(r.U8())
		for i := 0; i < n && !r.Err; i++ {
			slot := r.U8()
			id := r.U32()
			r.Skip(16 + 1)
			resp.slots = append(resp.slots, slot)
			resp.items = append(resp.items, id)
		}
	}
	select {
	case t.ch <- resp:
	default:
	}
}

type Result struct {
	Gold  uint32
	Items []uint32
}

func send(ctx context.Context, s modv1.SessionClient, op uint32, payload []byte) error {
	_, err := s.SendPacket(ctx, &modv1.SendPacketRequest{Opcode: op, Payload: payload})
	return err
}

// Drain forgets any pending loot response.
func (t *Tracker) Drain() {
	for len(t.ch) > 0 {
		<-t.ch
	}
}

// Take loots everything from guid. The caller must already be within interaction range.
// With open=false the server already sent the loot window (e.g. after using a chest).
func (t *Tracker) Take(ctx context.Context, s modv1.SessionClient, guid uint64, open bool) (Result, error) {
	if open {
		t.Drain()
		w := &wire.Writer{}
		w.U64(guid)
		if err := send(ctx, s, uint32(opcode.CMSGLoot), w.B); err != nil {
			return Result{}, err
		}
	}
	var resp response
	select {
	case resp = <-t.ch:
	case <-time.After(t.Wait):
		return Result{}, fmt.Errorf("no loot response")
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	if resp.err {
		return Result{}, fmt.Errorf("loot refused")
	}
	if resp.gold > 0 {
		_ = send(ctx, s, uint32(opcode.CMSGLootMoney), nil)
		time.Sleep(150 * time.Millisecond)
	}
	for _, slot := range resp.slots {
		_ = send(ctx, s, uint32(opcode.CMSGAutostoreLootItem), []byte{slot})
		time.Sleep(150 * time.Millisecond)
	}
	rel := &wire.Writer{}
	rel.U64(guid)
	_ = send(ctx, s, uint32(opcode.CMSGLootRelease), rel.B)
	return Result{Gold: resp.gold, Items: resp.items}, nil
}
