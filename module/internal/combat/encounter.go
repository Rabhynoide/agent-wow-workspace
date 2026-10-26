package combat

import (
	"context"
	"log/slog"
	"time"

	statepb "wowagent/gen/statev1"
)

// An encounter runs from our first fight until we are out of combat with nobody on us (or dead, or
// gone after a flight), across the fights against each of its monsters. The borderline flee decisions
// taken during it are judged on how it ends, not on their own fight: a stay against a pack was "won"
// as soon as its target died, and the rest of the pack killed us 6-7 s later (series RF runs 1 and 4,
// 2026-10-24), so the learned table took deadly stays for good ones. The user's rule: count the
// monsters fought between entering combat and leaving it. The decision holds for the whole encounter
// too (it was taken again at each fight): 5 explored stays of 9 against 3 fresh attackers ended "fled"
// because the next fight of the pack decided again by the rule and ran, so they taught nothing (series
// QR), and the rule's flights from 3 wolves at 93-100 % health were most of the flights on the way.
type encounter struct {
	start     time.Time
	lastFoe   time.Time       // last sample with a monster on us
	foes      map[uint64]bool // monsters that targeted us
	decisions []*fleeDecision
	hpStart   uint32
	hpMin     uint32
	// for the damage learning (dpslearn): who the monsters were, and us at the start
	mobs                map[uint64][2]uint32 // guid -> entry, level
	armor, level, class uint32
}

// encounterStale: an encounter with no monster on us for this long is over even if no check saw it
// end (we left combat between two jobs).
const encounterStale = 10 * time.Second

// encounterBegin opens an encounter at the start of a fight unless one is running.
func (s *Service) encounterBegin(me *statepb.SelfInfo) {
	s.mu.Lock()
	e := s.enc
	s.mu.Unlock()
	if e != nil && time.Since(e.lastFoe) > encounterStale {
		s.encounterEnd("won")
		e = nil
	}
	if e != nil {
		return
	}
	hp := uint32(0)
	if me != nil {
		hp = pct(me.Hp, me.MaxHp)
	}
	s.mu.Lock()
	s.enc = &encounter{start: time.Now(), lastFoe: time.Now(), foes: map[uint64]bool{}, hpStart: hp, hpMin: hp,
		mobs: map[uint64][2]uint32{}}
	if me != nil {
		s.enc.armor, s.enc.level, s.enc.class = me.Armor, me.Level, me.ClassId
	}
	s.dmg.resetTotals()
	s.dealt.resetTotals()
	s.decision = nil
	s.mu.Unlock()
}

// encounterSaw records the monsters on us and our health (fight samples, once a second).
func (s *Service) encounterSaw(foes []*statepb.Obj, hp uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.enc
	if e == nil {
		return
	}
	for _, o := range foes {
		e.foes[o.Guid] = true
		e.mobs[o.Guid] = [2]uint32{o.Entry, o.Level}
	}
	if len(foes) > 0 {
		e.lastFoe = time.Now()
	}
	e.hpMin = min(e.hpMin, hp)
}

// encounterDecision adds a borderline flee decision to the running encounter.
func (s *Service) encounterDecision(d *fleeDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enc != nil {
		s.enc.decisions = append(s.enc.decisions, d)
	}
}

// encounterCalm ends the encounter as won when we are out of combat and no monster is on us.
func (s *Service) encounterCalm(ctx context.Context, me *statepb.SelfInfo) {
	s.mu.Lock()
	e := s.enc
	s.mu.Unlock()
	if e == nil || me == nil || me.Dead || me.UnitFlags&unitInCombat != 0 || len(s.onMe(ctx, me)) > 0 {
		return
	}
	s.encounterEnd("won")
}

// encounterEnd logs the encounter (monsters fought, length, health) and the outcome of each of its
// borderline decisions: died, fled (says nothing about staying) or won.
func (s *Service) encounterEnd(result string) {
	s.mu.Lock()
	e := s.enc
	s.enc = nil
	s.decision = nil
	s.mu.Unlock()
	if e == nil {
		return
	}
	secs := int(time.Since(e.start).Seconds())
	slog.Info("encounter", "foes", len(e.foes), "s", secs, "hp_start", e.hpStart, "hp_min", e.hpMin, "decisions", len(e.decisions), "result", result)
	for _, d := range e.decisions {
		slog.Info("flee outcome", "key", d.key, "stayed", d.stayed, "how", d.how, "hp", d.hp, "slack", int(d.slack), "foes", len(e.foes), "result", result)
	}
	// Damage learning (dpslearn, learned by guidecheck from these lines).
	taken, _ := s.dmg.totals()
	for g, t := range taken {
		m, ok := e.mobs[g]
		if r := t.rate(); ok && r > 0 && e.armor > 0 {
			slog.Info("damage taken", "entry", m[0], "level", m[1], "armor", e.armor, "dps", round1(r), "hits", t.hits, "s", int(t.last.Sub(t.first).Seconds()))
		}
	}
	if _, all := s.dealt.totals(); all.rate() > 0 && e.level > 0 {
		slog.Info("damage dealt", "class", e.class, "level", e.level, "dps", round1(all.rate()), "hits", all.hits, "s", int(all.last.Sub(all.first).Seconds()))
	}
}

func round1(x float64) float64 { return float64(int(x*10+0.5)) / 10 }
