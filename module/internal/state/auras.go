package state

import (
	"time"

	pb "wowagent/gen/statev1"
	"wowagent/internal/wire"
)

// Aura flags of SMSG_AURA_UPDATE (AuraFlags, 3.3.5a).
const (
	auraNoCaster = 0x08 // caster is the unit itself: no caster GUID follows
	auraPositive = 0x10
	auraDuration = 0x20
)

// aura is one aura slot of a unit; the remaining time counts down from `at`.
type aura struct {
	spell                uint32
	flags, level, stacks uint8
	caster               uint64
	maxMs, leftMs        uint32
	at                   time.Time
}

// auraUpdate reads SMSG_AURA_UPDATE (one slot) or SMSG_AURA_UPDATE_ALL (every slot, replacing the
// list): target, then per slot: slot, spell (0 = removed), flags, caster level, stacks/charges, the
// caster unless it is the target, and the durations when flagged.
func (s *Store) auraUpdate(r *wire.Reader, all bool) {
	guid := r.PackedGUID()
	if r.Err {
		return
	}
	o := s.objs[guid]
	if o == nil {
		o = s.obj(guid, 0)
	}
	if all || o.Auras == nil {
		o.Auras = map[uint8]*aura{}
	}
	now := time.Now()
	for !r.Err && r.Left() > 0 {
		slot := r.U8()
		spell := r.U32()
		if r.Err {
			return
		}
		if spell == 0 {
			delete(o.Auras, slot)
			continue
		}
		a := &aura{spell: spell, flags: r.U8(), level: r.U8(), stacks: r.U8(), at: now, caster: guid}
		if a.flags&auraNoCaster == 0 {
			a.caster = r.PackedGUID()
		}
		if a.flags&auraDuration != 0 {
			a.maxMs, a.leftMs = r.U32(), r.U32()
		}
		if r.Err {
			return
		}
		o.Auras[slot] = a
		if !all {
			return
		}
	}
}

// aurasPB lists a unit's auras with the time left now; me is our GUID (for "mine").
func aurasPB(o *Obj, me uint64) []*pb.Aura {
	var out []*pb.Aura
	now := time.Now()
	for slot, a := range o.Auras {
		left := uint32(0)
		if a.leftMs > 0 {
			gone := uint32(now.Sub(a.at).Milliseconds())
			if gone >= a.leftMs {
				continue // expired (the removal packet may still be on its way)
			}
			left = a.leftMs - gone
		}
		out = append(out, &pb.Aura{Slot: uint32(slot), Spell: a.spell, Stacks: uint32(a.stacks), LeftMs: left, MaxMs: a.maxMs,
			Mine: a.caster == me, Positive: a.flags&auraPositive != 0})
	}
	return out
}
