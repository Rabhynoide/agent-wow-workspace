package combat

import (
	"sync"
	"time"

	"wowagent/internal/wire"
)

// Melee feedback from the server: refused swings (out of range, bad facing) and the outcome of every
// attack (SMSG_ATTACKERSTATEUPDATE): whether our hits land and whether the target dodged (Overpower).

const (
	victimDodge = 2 // VictimState VICTIMSTATE_DODGE

	hitFullAbsorb    = 0x20
	hitPartialAbsorb = 0x40
	hitFullResist    = 0x80
	hitPartialResist = 0x100
)

type meleeLog struct {
	mu         sync.Mutex
	notInRange time.Time
	badFacing  time.Time
	hit        map[uint64]time.Time // target -> last time one of our attacks dealt damage
	dodge      map[uint64]time.Time // target -> last time it dodged us
}

func newMeleeLog() *meleeLog {
	return &meleeLog{hit: map[uint64]time.Time{}, dodge: map[uint64]time.Time{}}
}

func (m *meleeLog) refusedRange()  { m.mu.Lock(); m.notInRange = time.Now(); m.mu.Unlock() }
func (m *meleeLog) refusedFacing() { m.mu.Lock(); m.badFacing = time.Now(); m.mu.Unlock() }

// recent reports whether the server refused a swing for range / facing in the last d.
func (m *meleeLog) recent(d time.Duration) (outOfRange, badFacing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Since(m.notInRange) < d, time.Since(m.badFacing) < d
}

func (m *meleeLog) lastHit(target uint64) time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hit[target]
}

func (m *meleeLog) dodged(target uint64, within time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Since(m.dodge[target]) < within
}

// attackerState records one SMSG_ATTACKERSTATEUPDATE when we are the attacker.
func (m *meleeLog) attackerState(self uint64, payload []byte) {
	attacker, target, damage, state, ok := parseAttackerState(payload)
	if !ok || attacker != self {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if damage > 0 {
		m.hit[target] = time.Now()
	}
	if state == victimDodge {
		m.dodge[target] = time.Now()
	}
}

// parseAttackerState decodes Unit::SendAttackStateUpdate: hit info, packed attacker and target,
// damage, overkill, sub damages (school, float, int), absorbs/resists when flagged, victim state.
func parseAttackerState(payload []byte) (attacker, target uint64, damage uint32, state uint8, ok bool) {
	r := wire.NewReader(payload)
	hitInfo := r.U32()
	attacker = r.PackedGUID()
	target = r.PackedGUID()
	damage = r.U32()
	r.U32() // overkill
	n := int(r.U8())
	r.Skip(12 * n)
	if hitInfo&(hitFullAbsorb|hitPartialAbsorb) != 0 {
		r.Skip(4 * n)
	}
	if hitInfo&(hitFullResist|hitPartialResist) != 0 {
		r.Skip(4 * n)
	}
	state = r.U8()
	return attacker, target, damage, state, !r.Err
}
