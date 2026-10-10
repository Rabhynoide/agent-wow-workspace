// Package state tracks the visible world from server packets.
package state

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"wowagent/internal/wire"
)

// Update field indexes for 3.3.5a (OBJECT_END=6, UNIT_END=0x94).
const (
	fEntry       = 3
	fItemCount   = 14
	fItemHolder  = 8 // ITEM_FIELD_CONTAINED
	fUnitTarget  = 18
	fUnitBytes0  = 23
	fUnitHealth  = 24
	fUnitPower1  = 25
	fUnitMaxHP   = 32
	fUnitMaxPow1 = 33
	fUnitLevel   = 54
	fUnitFaction = 55
	fUnitFlags   = 59
	fUnitDyn     = 79
	fUnitNpc     = 82
	fQuestLog    = 158
	fInvSlots    = 324
	fPlayerFlags = 150
	fTalentPts   = 1020 // PLAYER_CHARACTER_POINTS1
	fXP          = 634
	fNextXP      = 635
	fCoinage     = 1170
	fItemDur     = 60 // ITEM_FIELD_DURABILITY
	fItemMaxDur  = 61 // ITEM_FIELD_MAXDURABILITY
	fBagSlots    = 64 // CONTAINER_FIELD_NUM_SLOTS
	fBagSlot1    = 66 // CONTAINER_FIELD_SLOT_1 (guid pairs)
)

// Inventory slots: equipped bags 19..22, backpack 23..38.
const (
	slotBagFirst  = 19
	slotBagLast   = 22
	slotPackFirst = 23
	slotPackLast  = 38
)

// Spline flags of SMSG_MONSTER_MOVE.
const (
	splineParabolic = 0x00000800
	splineFlying    = 0x00002000
	splineCatmull   = 0x00040000
	splineCyclic    = 0x00080000
	splineAnimation = 0x00200000
)

const (
	TypeItem       = 1
	TypeContainer  = 2
	TypeUnit       = 3
	TypePlayer     = 4
	TypeGameObject = 5
)

// Obj is a tracked world object with its raw update fields.
type Obj struct {
	Guid       uint64
	Type       uint8
	F          map[uint16]uint32
	X, Y, Z, O float32
	HasPos     bool
	Speeds     [9]float32
}

func (o *Obj) u(i uint16) uint32 { return o.F[i] }
func (o *Obj) guid(i uint16) uint64 {
	return uint64(o.F[i]) | uint64(o.F[i+1])<<32
}

// Store is the concurrent object table.
type Store struct {
	mu      sync.Mutex
	self    uint64
	Map     uint32
	objs    map[uint64]*Obj
	Spells  map[uint32]bool
	Packets uint32
}

func NewStore(self uint64) *Store {
	return &Store{self: self, objs: map[uint64]*Obj{}, Spells: map[uint32]bool{}}
}

func (s *Store) obj(guid uint64, typ uint8) *Obj {
	o := s.objs[guid]
	if o == nil {
		o = &Obj{Guid: guid, Type: typ, F: map[uint16]uint32{}}
		s.objs[guid] = o
	}
	return o
}

// Handle dispatches one server packet.
func (s *Store) Handle(op uint32, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Packets++
	r := wire.NewReader(payload)
	switch op {
	case opUpdateObject:
		s.update(r)
	case opCompressedUpdate:
		size := r.U32()
		zr, err := zlib.NewReader(bytes.NewReader(payload[min(4, len(payload)):]))
		if err != nil {
			slog.Warn("compressed update: zlib", "err", err)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(zr, int64(size)+1))
		if err != nil && len(raw) == 0 {
			slog.Warn("compressed update: read", "err", err)
			return
		}
		s.update(wire.NewReader(raw))
	case opDestroyObject:
		delete(s.objs, r.U64())
	case opLoginVerifyWorld, opNewWorld:
		m := r.U32()
		x, y, z, o := r.F32(), r.F32(), r.F32(), r.F32()
		if !r.Err {
			s.Map = m
			self := s.obj(s.self, TypePlayer)
			self.X, self.Y, self.Z, self.O, self.HasPos = x, y, z, o, true
		}
	case opMonsterMove:
		s.monsterMove(r)
	case opInitialSpells:
		r.U8()
		n := int(r.U16())
		for i := 0; i < n && !r.Err; i++ {
			id := r.U32()
			r.U16()
			s.Spells[id] = true
		}
	case opLearnedSpell:
		s.Spells[r.U32()] = true
	case opSupercededSpell:
		newID, oldID := r.U32(), r.U32()
		delete(s.Spells, oldID)
		s.Spells[newID] = true
	case opRemovedSpell:
		delete(s.Spells, r.U32())
	}
}

var dumped int

func (s *Store) dumpIfBad(r *wire.Reader, op uint32) {
	if !r.Err || dumped >= 5 {
		return
	}
	dumped++
	_ = os.WriteFile(fmt.Sprintf("/logs/state_bad_%d.hex", dumped), []byte(hex.EncodeToString(r.B)+fmt.Sprintf("\nstopped at %d of %d\n", r.I, len(r.B))), 0o644)
}

func (s *Store) monsterMove(r *wire.Reader) {
	guid := r.PackedGUID()
	r.U8()
	x, y, z := r.F32(), r.F32(), r.F32()
	r.U32()
	typ := r.U8()
	o := s.objs[guid]
	if o == nil || r.Err {
		return
	}
	// Our own character moves by spline only when the server drives it (Charge, knockback, fear...):
	// keep our position model in sync with the destination.
	o.X, o.Y, o.Z, o.HasPos = x, y, z, true
	switch typ {
	case 1:
		return
	case 2:
		r.Skip(12)
	case 3:
		r.Skip(8)
	case 4:
		r.Skip(4)
	}
	flags := r.U32()
	if flags&splineAnimation != 0 {
		r.Skip(5)
	}
	r.U32() // duration
	if flags&splineParabolic != 0 {
		r.Skip(8)
	}
	n := r.U32()
	if n == 0 || r.Err {
		return
	}
	// Linear paths start with the destination; Catmull-Rom paths list every node, the last being
	// the destination (cyclic paths have no destination: keep the current position).
	if flags&(splineFlying|splineCatmull) != 0 {
		if flags&splineCyclic != 0 || n > 1000 {
			return
		}
		r.Skip(int(n-1) * 12)
	}
	dx, dy, dz := r.F32(), r.F32(), r.F32()
	if !r.Err {
		o.X, o.Y, o.Z = dx, dy, dz
	}
}

func (s *Store) update(r *wire.Reader) {
	n := r.U32()
	for i := uint32(0); i < n && !r.Err; i++ {
		switch t := r.U8(); t {
		case 0:
			guid := r.PackedGUID()
			o := s.obj(guid, 0)
			s.values(o, r)
		case 1:
			guid := r.PackedGUID()
			o := s.obj(guid, 0)
			s.movement(o, r)
		case 2, 3:
			guid := r.PackedGUID()
			typ := r.U8()
			o := s.obj(guid, typ)
			o.Type = typ
			s.movement(o, r)
			s.values(o, r)
		case 4:
			c := r.U32()
			for j := uint32(0); j < c && !r.Err; j++ {
				delete(s.objs, r.PackedGUID())
			}
		case 5:
			c := r.U32()
			for j := uint32(0); j < c && !r.Err; j++ {
				r.PackedGUID()
			}
		default:
			slog.Warn("update object: unknown block", "type", t)
			return
		}
	}
	if r.Err {
		slog.Warn("update object: truncated packet", "blocks", n)
		s.dumpIfBad(r, 0)
	}
}

func (s *Store) values(o *Obj, r *wire.Reader) {
	nb := int(r.U8())
	masks := make([]uint32, nb)
	for i := range masks {
		masks[i] = r.U32()
	}
	for i, m := range masks {
		for b := 0; b < 32; b++ {
			if m&(1<<b) != 0 {
				v := r.U32()
				if !r.Err {
					o.F[uint16(i*32+b)] = v
				}
			}
		}
	}
}

func (s *Store) movement(o *Obj, r *wire.Reader) {
	flags := r.U16()
	switch {
	case flags&0x20 != 0:
		mf := r.U32()
		mf2 := r.U16()
		r.U32()
		x, y, z, orient := r.F32(), r.F32(), r.F32(), r.F32()
		if mf&0x200 != 0 {
			r.PackedGUID()
			r.Skip(16)
			r.U32()
			r.U8()
			if mf2&0x400 != 0 {
				r.U32()
			}
		}
		if mf&0x02200000 != 0 || mf2&0x20 != 0 {
			r.F32()
		}
		r.U32() // fall time
		if mf&0x1000 != 0 {
			r.Skip(16)
		}
		if mf&0x04000000 != 0 {
			r.F32()
		}
		for i := range o.Speeds {
			o.Speeds[i] = r.F32()
		}
		if mf&0x08000000 != 0 {
			s.skipSpline(r)
		}
		if !r.Err {
			o.X, o.Y, o.Z, o.O, o.HasPos = x, y, z, orient, true
		}
	case flags&0x100 != 0:
		r.PackedGUID()
		x, y, z := r.F32(), r.F32(), r.F32()
		r.Skip(12)
		orient := r.F32()
		r.F32()
		if !r.Err {
			o.X, o.Y, o.Z, o.O, o.HasPos = x, y, z, orient, true
		}
	case flags&0x40 != 0:
		x, y, z, orient := r.F32(), r.F32(), r.F32(), r.F32()
		if !r.Err {
			o.X, o.Y, o.Z, o.O, o.HasPos = x, y, z, orient, true
		}
	}
	if flags&0x8 != 0 {
		r.U32()
	}
	if flags&0x10 != 0 {
		r.U32()
	}
	if flags&0x4 != 0 {
		r.PackedGUID()
	}
	if flags&0x2 != 0 {
		r.U32()
	}
	if flags&0x80 != 0 {
		r.Skip(8)
	}
	if flags&0x200 != 0 {
		r.Skip(8)
	}
}

func (s *Store) skipSpline(r *wire.Reader) {
	sf := r.U32()
	switch {
	case sf&0x20000 != 0:
		r.Skip(4)
	case sf&0x10000 != 0:
		r.Skip(8)
	case sf&0x8000 != 0:
		r.Skip(12)
	}
	r.Skip(4 * 6) // timePassed, duration, id, 2 duration mods, vertical accel
	r.U32()       // effect start
	n := r.U32()
	if n > 1000 {
		r.Err = true
		return
	}
	r.Skip(int(n) * 12)
	r.U8()
	r.Skip(12)
}
