package state

import (
	"testing"

	"wowagent/internal/wire"
)

func setGUID(o *Obj, field uint16, g uint64) {
	o.F[field], o.F[field+1] = uint32(g), uint32(g>>32)
}

func TestQuestCountsAreUint16Pairs(t *testing.T) {
	st := NewStore(1)
	me := st.obj(1, TypePlayer)
	base := uint16(fQuestLog + 5*3)
	me.F[base] = 33
	me.F[base+1] = 0
	me.F[base+2] = 7 | 300<<16 // counter 0 = 7, counter 1 = 300
	me.F[base+3] = 2           // counter 2 = 2
	qs := questLog(me)
	if len(qs) != 1 || qs[0].Id != 33 {
		t.Fatalf("quests = %v", qs)
	}
	want := []uint32{7, 300, 2, 0}
	for i, c := range want {
		if qs[0].Counts[i] != c {
			t.Fatalf("counts = %v, want %v", qs[0].Counts, want)
		}
	}
}

func TestFreeSlotsCountsEquippedBags(t *testing.T) {
	st := NewStore(1)
	me := st.obj(1, TypePlayer)
	for slot := slotPackFirst; slot < slotPackFirst+10; slot++ { // 10 of 16 used
		setGUID(me, uint16(fInvSlots+2*slot), uint64(100+slot))
	}
	bag := st.obj(500, TypeContainer)
	bag.F[fBagSlots] = 6
	setGUID(bag, fBagSlot1, 900) // 1 of 6 used
	setGUID(me, uint16(fInvSlots+2*slotBagFirst), 500)
	if got := st.freeSlots(me); got != 6+5 {
		t.Fatalf("free = %d, want 11", got)
	}
}

func monsterMove(guid uint64, flags uint32, pts [][3]float32) []byte {
	w := &wire.Writer{}
	w.PackedGUID(guid)
	w.U8(0)
	w.F32(1)
	w.F32(2)
	w.F32(3) // start position
	w.U32(9) // spline id
	w.U8(0)  // normal
	w.U32(flags)
	w.U32(1000) // duration
	w.U32(uint32(len(pts)))
	for _, p := range pts {
		w.F32(p[0])
		w.F32(p[1])
		w.F32(p[2])
	}
	return w.B
}

func TestMonsterMoveDestination(t *testing.T) {
	st := NewStore(1)
	mob := st.obj(42, TypeUnit)
	// Linear: the first point is the destination (offsets follow).
	st.Handle(opMonsterMove, append(monsterMove(42, 0, [][3]float32{{10, 20, 30}}), 0, 0, 0, 0))
	if mob.X != 10 || mob.Y != 20 || mob.Z != 30 {
		t.Fatalf("linear: pos = %v %v %v", mob.X, mob.Y, mob.Z)
	}
	// Catmull-Rom: every node is listed, the destination is the last one.
	st.Handle(opMonsterMove, monsterMove(42, splineCatmull, [][3]float32{{4, 5, 6}, {7, 8, 9}, {11, 12, 13}}))
	if mob.X != 11 || mob.Y != 12 || mob.Z != 13 {
		t.Fatalf("catmull: pos = %v %v %v", mob.X, mob.Y, mob.Z)
	}
	// Cyclic: keep the current (start) position.
	st.Handle(opMonsterMove, monsterMove(42, splineCatmull|splineCyclic, [][3]float32{{50, 50, 50}, {60, 60, 60}}))
	if mob.X != 1 || mob.Y != 2 || mob.Z != 3 {
		t.Fatalf("cyclic: pos = %v %v %v", mob.X, mob.Y, mob.Z)
	}
}

func TestUpdateValuesBlock(t *testing.T) {
	st := NewStore(1)
	w := &wire.Writer{}
	w.U32(1) // one block
	w.U8(0)  // UPDATETYPE_VALUES
	w.PackedGUID(1)
	// mask with bits for health (24) and level (54) -> 2 mask blocks
	w.U8(2)
	w.U32(1 << 24)
	w.U32(1 << (54 - 32))
	w.U32(77) // health
	w.U32(5)  // level
	st.Handle(opUpdateObject, w.B)
	me := st.objs[1]
	if me == nil || me.u(fUnitHealth) != 77 || me.u(fUnitLevel) != 5 {
		t.Fatalf("fields = %v", me)
	}
}

func TestSnapshotIgnoresItemsNotCarried(t *testing.T) {
	svc := &Service{st: NewStore(1)}
	me := svc.st.obj(1, TypePlayer)
	item := svc.st.obj(100, TypeItem)
	item.F[fEntry], item.F[fItemCount] = 4536, 3
	setGUID(me, uint16(fInvSlots+2*slotPackFirst), 100) // carried in the backpack
	sold := svc.st.obj(101, TypeItem)                   // sold: lives on in the buyback list
	sold.F[fEntry] = 1411
	resp, err := svc.Snapshot(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Entry != 4536 || resp.Items[0].Bag != 255 || resp.Items[0].BagSlot != slotPackFirst {
		t.Fatalf("items = %v", resp.Items)
	}
}
