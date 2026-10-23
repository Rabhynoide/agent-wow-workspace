package combat

import (
	"testing"
	"time"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/wire"
)

func attackPacket(hitInfo uint32, attacker, target uint64, damage uint32, state uint8) []byte {
	w := &wire.Writer{}
	w.U32(hitInfo)
	w.PackedGUID(attacker)
	w.PackedGUID(target)
	w.U32(damage)
	w.U32(0) // overkill
	w.U8(1)  // one sub damage
	w.U32(1)
	w.F32(float32(damage))
	w.U32(damage)
	if hitInfo&hitPartialAbsorb != 0 {
		w.U32(3)
	}
	w.U8(state)
	w.U32(0)
	w.U32(0)
	return w.B
}

func TestAttackerState(t *testing.T) {
	m := newMeleeLog()
	const me, mob = 3, 0xF13000020D000042
	m.attackerState(me, attackPacket(0x2, me, mob, 12, 1))
	if time.Since(m.lastHit(mob)) > time.Second {
		t.Fatal("our hit was not recorded")
	}
	m.attackerState(me, attackPacket(0x2|hitPartialAbsorb, me, mob, 0, victimDodge))
	if !m.dodged(mob, time.Second) {
		t.Fatal("the dodge (after an absorb block) was not recorded")
	}
	other := uint64(0xF13000020D000099)
	m.attackerState(me, attackPacket(0x2, mob, me, 20, 1)) // the mob hits us: not our hit
	if !m.lastHit(other).IsZero() {
		t.Fatal("unrelated target recorded")
	}
	if _, a, tg, d, st, ok := parseAttackerState(attackPacket(0x10, me, mob, 0, 0)); !ok || a != me || tg != mob || d != 0 || st != 0 {
		t.Fatalf("miss: %v %x %d %d", ok, tg, d, st)
	}
}

// Sides: facing east (O 0), a unit east is in front, west behind; blows count by the side last seen.
func TestBlowsBySide(t *testing.T) {
	me := &statepb.SelfInfo{X: 0, Y: 0, O: 0}
	if !inFront(me, &statepb.Obj{X: 3, Y: 1}) || inFront(me, &statepb.Obj{X: -3, Y: 1}) || !inFront(me, &statepb.Obj{X: 0.1, Y: -3}) {
		t.Fatal("front half")
	}
	if !inFront(&statepb.SelfInfo{O: 6.2}, &statepb.Obj{X: 3, Y: 0.5}) {
		t.Fatal("orientation near 2 pi")
	}
	m := newMeleeLog()
	m.saw(1, true)
	m.saw(2, false)
	m.takeBlow(1, 0, 0, victimParry)
	m.takeBlow(2, 0, 12, 1)
	m.takeBlow(3, hitMiss, 0, 0)
	got := m.takenSoFar()
	if got[sideFront].Parry != 1 || got[sideBack].Damage != 12 || got[sideUnknown].Miss != 1 {
		t.Fatalf("taken %+v", got)
	}
}
