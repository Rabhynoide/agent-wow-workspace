package state

import (
	"testing"

	"wowagent/internal/wire"
)

func TestAuraUpdates(t *testing.T) {
	const me, wolf = uint64(3), uint64(0xF130000301000042)
	st := NewStore(me)
	// Full list on the wolf: our Rend (slot 0, 15 s, cast by us) and its own buff (slot 1, no caster).
	w := &wire.Writer{}
	w.PackedGUID(wolf)
	w.U8(0)
	w.U32(772)
	w.U8(0x01 | auraDuration) // effect 0, has duration, caster follows
	w.U8(11)
	w.U8(1)
	w.PackedGUID(me)
	w.U32(15000)
	w.U32(14000)
	w.U8(1)
	w.U32(9999)
	w.U8(auraNoCaster | auraPositive)
	w.U8(5)
	w.U8(3)
	st.Handle(opAuraUpdateAll, w.B)
	o := st.objs[wolf]
	if o == nil || len(o.Auras) != 2 {
		t.Fatalf("auras %+v", o)
	}
	got := map[uint32]bool{}
	for _, a := range aurasPB(o, me) {
		got[a.Spell] = a.Mine
		switch a.Spell {
		case 772:
			if a.LeftMs == 0 || a.LeftMs > 14000 || a.MaxMs != 15000 || a.Positive {
				t.Fatalf("rend %+v", a)
			}
		case 9999:
			if a.Stacks != 3 || !a.Positive || a.LeftMs != 0 {
				t.Fatalf("buff %+v", a)
			}
		}
	}
	if !got[772] || got[9999] {
		t.Fatalf("mine flags %v (Rend is ours, the buff is the wolf's)", got)
	}
	// Rend removed (single update, spell 0).
	w = &wire.Writer{}
	w.PackedGUID(wolf)
	w.U8(0)
	w.U32(0)
	st.Handle(opAuraUpdate, w.B)
	if len(o.Auras) != 1 {
		t.Fatalf("after removal %d auras", len(o.Auras))
	}
	// Our own buff, cast on ourselves.
	w = &wire.Writer{}
	w.PackedGUID(me)
	w.U8(4)
	w.U32(6673)
	w.U8(auraNoCaster | auraPositive | auraDuration)
	w.U8(11)
	w.U8(0)
	w.U32(120000)
	w.U32(120000)
	st.Handle(opAuraUpdate, w.B)
	if a := aurasPB(st.objs[me], me); len(a) != 1 || !a[0].Mine || a[0].Spell != 6673 {
		t.Fatalf("self buff %+v", a)
	}
}
