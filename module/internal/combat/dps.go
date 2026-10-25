package combat

import (
	"sync"
	"time"

	"wowagent/internal/wire"
)

// Damage we take, measured from what the server tells the client: melee blows
// (SMSG_ATTACKERSTATEUPDATE), spells (SMSG_SPELLNONMELEEDAMAGELOG) and damage over time
// (SMSG_PERIODICAURALOG). The flee margin is what that damage costs during a flight (losing, flee.go).

// dpsWindow: damage of the last seconds; dpsMinSpan: a measure needs that much fight behind it (one
// swing is not a rate).
const (
	dpsWindow  = 6 * time.Second
	dpsMinSpan = 3 * time.Second
)

type damageHit struct {
	at     time.Time
	from   uint64
	amount uint32
}

type damageLog struct {
	mu    sync.Mutex
	hits  []damageHit
	first time.Time // first hit of the current run of hits (a pause of dpsWindow starts a new one)
}

func (d *damageLog) add(from uint64, amount uint32) {
	if amount == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if len(d.hits) == 0 || now.Sub(d.hits[len(d.hits)-1].at) > dpsWindow {
		d.first = now
	}
	d.hits = append(d.hits, damageHit{now, from, amount})
	// Keep the window only.
	i := 0
	for i < len(d.hits) && now.Sub(d.hits[i].at) > dpsWindow {
		i++
	}
	d.hits = d.hits[i:]
}

// rate returns the damage per second taken over the window, and false while the fight has gone on
// for less than dpsMinSpan (or nothing hit us in the window).
func (d *damageLog) rate() (float64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	var sum uint32
	for _, h := range d.hits {
		if now.Sub(h.at) <= dpsWindow {
			sum += h.amount
		}
	}
	if sum == 0 || now.Sub(d.first) < dpsMinSpan {
		return 0, false
	}
	span := min(now.Sub(d.first), dpsWindow)
	return float64(sum) / span.Seconds(), true
}

// byAttacker returns the damage per second of each source (or target, for the damage we deal) over
// the window (focus order, logs).
func (d *damageLog) byAttacker() map[uint64]float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	span := min(now.Sub(d.first), dpsWindow).Seconds()
	if span < 1 {
		span = 1
	}
	out := map[uint64]float64{}
	for _, h := range d.hits {
		if now.Sub(h.at) <= dpsWindow {
			out[h.from] += float64(h.amount) / span
		}
	}
	return out
}

// spellDamage reads SMSG_SPELLNONMELEEDAMAGELOG (Unit::SendSpellNonMeleeDamageLog): packed target,
// packed caster, spell, damage, overkill...
func spellDamage(payload []byte) (target, caster uint64, amount uint32, ok bool) {
	r := wire.NewReader(payload)
	target = r.PackedGUID()
	caster = r.PackedGUID()
	r.U32() // spell
	damage := r.U32()
	overkill := r.U32()
	if r.Err {
		return 0, 0, 0, false
	}
	if overkill <= damage {
		damage -= overkill
	}
	return target, caster, damage, true
}

// Aura types of SMSG_PERIODICAURALOG that hurt (SPELL_AURA_PERIODIC_DAMAGE, _PERCENT).
const (
	auraPeriodicDamage        = 3
	auraPeriodicDamagePercent = 89
)

// periodicDamage reads SMSG_PERIODICAURALOG (Unit::SendPeriodicAuraLog): packed target, packed
// caster, spell, count (1), aura type, then for damage: damage, overkill, school, absorb, resist, crit.
func periodicDamage(payload []byte) (target, caster uint64, amount uint32, ok bool) {
	r := wire.NewReader(payload)
	target = r.PackedGUID()
	caster = r.PackedGUID()
	r.U32() // spell
	if n := r.U32(); n == 0 {
		return 0, 0, 0, false
	}
	aura := r.U32()
	if aura != auraPeriodicDamage && aura != auraPeriodicDamagePercent {
		return 0, 0, 0, false
	}
	damage := r.U32()
	overkill := r.U32()
	if r.Err {
		return 0, 0, 0, false
	}
	if overkill <= damage {
		damage -= overkill
	}
	return target, caster, damage, true
}

// damageSeen routes one damage event: to the damage we take (from its source) or the damage we deal
// (to its target).
func (s *Service) damageSeen(target, caster uint64, amount uint32) {
	switch {
	case target == s.guid && caster != s.guid:
		s.dmg.add(caster, amount)
	case caster == s.guid && target != s.guid:
		s.dealt.add(target, amount)
	}
}
