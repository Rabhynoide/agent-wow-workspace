package combat

import (
	"testing"
	"time"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/tuning"
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
	if target, from, amount, ok := spellDamage(w.B); !ok || target != me || from != mob || amount != 40 {
		t.Fatalf("spell damage: %x %x %d %v", target, from, amount, ok)
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
	if target, from, amount, ok := periodicDamage(p.B); !ok || target != me || from != mob || amount != 12 {
		t.Fatalf("periodic damage: %x %x %d %v", target, from, amount, ok)
	}
	s := &Service{guid: me}
	s.damageSeen(me, mob, 10)
	s.damageSeen(mob, me, 25)
	if len(s.dmg.hits) != 1 || len(s.dealt.hits) != 1 || s.dealt.hits[0].from != mob {
		t.Fatalf("routing: taken %v dealt %v", s.dmg.hits, s.dealt.hits)
	}
}

// fill puts damage over the last 4 s in a log.
func fill(d *damageLog, from uint64, perSecond uint32) {
	now := time.Now()
	d.first = now.Add(-4 * time.Second)
	for i := 0; i < 4; i++ {
		d.hits = append(d.hits, damageHit{now.Add(-time.Duration(i) * time.Second), from, perSecond})
	}
}

func TestKillOrderByDamage(t *testing.T) {
	strong := &statepb.Obj{Guid: 1, Hp: 100, MaxHp: 100, Dist: 3}
	weak := &statepb.Obj{Guid: 2, Hp: 60, MaxHp: 100, Dist: 3}
	// Without measures: the weakest first.
	if a := pickAttacker([]*statepb.Obj{strong, weak}, nil); a.Guid != 2 {
		t.Fatalf("no measure: %d", a.Guid)
	}
	// The full-health one hits 6 times harder: 30/100 beats 5/60.
	dps := map[uint64]float64{1: 30, 2: 5}
	if a := pickAttacker([]*statepb.Obj{strong, weak}, dps); a.Guid != 1 {
		t.Fatalf("by damage: %d", a.Guid)
	}
	if b := focusTarget(weak, []*statepb.Obj{strong, weak}, dps); b == nil || b.Guid != 1 {
		t.Fatalf("switch to the strong hitter: %v", b)
	}
}

func TestLateHold(t *testing.T) {
	me := &statepb.SelfInfo{Hp: 200, MaxHp: 250} // 80 %
	mob := &statepb.Obj{Guid: 9, Entry: 46, Hp: 60, MaxHp: 200, Dist: 3}
	s := &Service{guid: 1}
	// Not measured yet: hold above flee_late_hold (60 %).
	if hold, why := s.lateHold(me, []*statepb.Obj{mob}); !hold || why != "measuring" {
		t.Fatalf("measuring: %v %s", hold, why)
	}
	// 10 damage/s taken (4 %/s: threshold 4*6+5 = 29 %, 12.75 s away), 20 dealt/s: 3 s to kill.
	fill(&s.dmg, 9, 10)
	fill(&s.dealt, 9, 20)
	if hold, _ := s.lateHold(me, []*statepb.Obj{mob}); !hold {
		t.Fatal("should finish the weak monster")
	}
	// A tough one (400 health: 20 s to kill) is not worth it.
	tough := &statepb.Obj{Guid: 9, Entry: 46, Hp: 400, MaxHp: 400, Dist: 3}
	if hold, _ := s.lateHold(me, []*statepb.Obj{tough}); hold {
		t.Fatal("should run from a monster it cannot finish")
	}
}

// Against a group, no hold before the measure; no hopeless exploration (run of 2026-10-26 00:09).
func TestGroupGuards(t *testing.T) {
	me := &statepb.SelfInfo{Hp: 230, MaxHp: 250}
	group := []*statepb.Obj{{Guid: 1, Hp: 100, MaxHp: 100}, {Guid: 2, Hp: 100, MaxHp: 100}, {Guid: 3, Hp: 100, MaxHp: 100}}
	if hold, _ := (&Service{}).lateHold(me, group); hold {
		t.Fatal("held against 3 attackers before measuring")
	}
	tu := tuning.Defaults()
	if p := exploreProb(tu, -56, true); p != 0 {
		t.Fatalf("hopeless exploration: %v", p)
	}
}

// Two Defias at 6 damage/s each, 200 health each, we deal 20/s: the first dies at 10 s, the second at
// 20 s: 6*10 + 6*20 = 180 damage.
func TestOrderedDamage(t *testing.T) {
	g := []*statepb.Obj{{Guid: 1, Hp: 200}, {Guid: 2, Hp: 200}}
	if d := orderedDamage(g, map[uint64]float64{1: 6, 2: 6}, 20); d < 179.9 || d > 180.1 {
		t.Fatalf("damage %v", d)
	}
	// The strong hitter first: 12/s with 200 health and 2/s with 100: 12*10 + 2*15 = 150 (the other
	// order: 2*5 + 12*15 = 190).
	g = []*statepb.Obj{{Guid: 1, Hp: 200}, {Guid: 2, Hp: 100}}
	if d := orderedDamage(g, map[uint64]float64{1: 12, 2: 2}, 20); d < 149.9 || d > 150.1 {
		t.Fatalf("ordered damage %v", d)
	}
}

// 4 kobolds at ~4 damage/s each on 232 health: a flight costs far more than the 57 % the fight is
// expected to cost, at 60 %: stay. At 90 % the flight is still tried.
func TestHopelessFlight(t *testing.T) {
	att := []*statepb.Obj{{Guid: 2, Hp: 90, MaxHp: 100}, {Guid: 3, Hp: 90, MaxHp: 100}, {Guid: 4, Hp: 90, MaxHp: 100}, {Guid: 5, Hp: 90, MaxHp: 100}}
	s := &Service{guid: 1}
	fill(&s.dmg, 2, 16)
	if h, why := s.hopelessFlight(&statepb.SelfInfo{Hp: 139, MaxHp: 232}, att, 57); !h {
		t.Fatalf("60 %%, fight 57 %%: should stay (%s)", why)
	}
	if h, _ := s.hopelessFlight(&statepb.SelfInfo{Hp: 139, MaxHp: 232}, att, 70); !h {
		t.Fatal("the fight kills us too, but the flight surely: stay")
	}
	if h, _ := s.hopelessFlight(&statepb.SelfInfo{Hp: 209, MaxHp: 232}, att, 57); h {
		t.Fatal("90 %: the flight is tried")
	}
}

// Staying or fleeing by cost, nothing learned: 2 kobolds at 94 % with 40 % to lose: stay; with 4
// more coming (joiners), the fight to come costs 136 %: flee.
func TestDecideByCost(t *testing.T) {
	me := &statepb.SelfInfo{Hp: 190, MaxHp: 203}
	att := []*statepb.Obj{{Guid: 7, Entry: 475, Hp: 150, MaxHp: 160}, {Guid: 8, Entry: 475, Hp: 150, MaxHp: 160}}
	s := &Service{guid: 1, fleeTab: tuning.NewFleeTable(t.TempDir())}
	if s.decide(me, att, 40, 0) {
		t.Fatal("slack 54: stay")
	}
	s.decision = nil
	if !s.decide(me, att, 40, 4) {
		t.Fatal("four kobolds coming: flee")
	}
}
