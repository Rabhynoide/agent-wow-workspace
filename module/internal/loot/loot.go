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
	guid   uint64
	gold   uint32
	slots  []uint8
	items  []uint32
	counts []uint32
	err    bool
}

// Tracker receives SMSG_LOOT_RESPONSE packets.
type Tracker struct {
	ch chan response
}

// DefaultWait is how long Take waits for the loot window.
const DefaultWait = 8 * time.Second

func NewTracker() *Tracker { return &Tracker{ch: make(chan response, 8)} }

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
			count := r.U32()
			r.Skip(12 + 1) // display id, random suffix and property, slot type
			resp.slots = append(resp.slots, slot)
			resp.items = append(resp.items, id)
			resp.counts = append(resp.counts, count)
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
	// Left: items left in the loot because the bags were full and nothing was worth throwing away.
	Left []uint32
}

// Room makes bag space for an item about to be taken (town.make_room: it may throw away the
// cheapest junk); false leaves the item where it is.
type Room func(ctx context.Context, entry, count uint32) bool

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
// wait bounds how long to wait for the loot window. room (nil: none) is asked before each item.
func (t *Tracker) Take(ctx context.Context, s modv1.SessionClient, guid uint64, open bool, wait time.Duration, room Room) (Result, error) {
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
	case <-time.After(wait):
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
	res := Result{Gold: resp.gold}
	for i, slot := range resp.slots {
		if room != nil && !room(ctx, resp.items[i], resp.counts[i]) {
			res.Left = append(res.Left, resp.items[i])
			continue
		}
		_ = send(ctx, s, uint32(opcode.CMSGAutostoreLootItem), []byte{slot})
		time.Sleep(150 * time.Millisecond)
		res.Items = append(res.Items, resp.items[i])
	}
	rel := &wire.Writer{}
	rel.U64(guid)
	_ = send(ctx, s, uint32(opcode.CMSGLootRelease), rel.B)
	return res, nil
}
