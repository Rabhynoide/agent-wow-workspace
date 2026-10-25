package combat

import (
	"testing"
	"time"

	"wowagent/internal/wire"
)

func TestDamageRate(t *testing.T) {
	var d damageLog
	if _, ok := d.rate(); ok {
		t.Fatal("rate without hits")
	}
	d.add(7, 30)
	if _, ok := d.rate(); ok {
		t.Fatal("one swing is not a rate")
	}
	// A fight going on for 4 s: 120 damage in 4 s.
	d.mu.Lock()
	d.first = time.Now().Add(-4 * time.Second)
	d.hits[0].at = d.first
	d.mu.Unlock()
	d.add(8, 90)
	r, ok := d.rate()
	if !ok || r < 29 || r > 31 {
		t.Fatalf("rate %v %v, want 30/s", r, ok)
	}
	by := d.byAttacker()
	if by[7] < 7 || by[7] > 8 || by[8] < 22 || by[8] > 23 {
		t.Fatalf("by attacker %v", by)
	}
}

func TestDamagePackets(t *testing.T) {
	const me, mob = 0x10, 0xF130000001000001
	w := &wire.Writer{}
	w.PackedGUID(me)
	w.PackedGUID(mob)
	w.U32(9053) // spell
	w.U32(40)   // damage
	w.U32(0)    // overkill
	w.U8(4)
	w.U32(0)
	w.U32(0)
	w.U8(0)
	w.U8(0)
	w.U32(0)
	w.U32(0)
	w.U8(0)
	if from, amount, ok := spellDamage(me, w.B); !ok || from != mob || amount != 40 {
		t.Fatalf("spell damage: %x %d %v", from, amount, ok)
	}
	if _, _, ok := spellDamage(0x11, w.B); ok {
		t.Fatal("spell damage on someone else")
	}
	p := &wire.Writer{}
	p.PackedGUID(me)
	p.PackedGUID(mob)
	p.U32(11977) // Rend
	p.U32(1)
	p.U32(auraPeriodicDamage)
	p.U32(12) // damage
	p.U32(0)  // overkill
	p.U32(1)  // school
	p.U32(0)
	p.U32(0)
	p.U8(0)
	if from, amount, ok := periodicDamage(me, p.B); !ok || from != mob || amount != 12 {
		t.Fatalf("periodic damage: %x %d %v", from, amount, ok)
	}
}
