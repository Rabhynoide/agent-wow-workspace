package combat

import (
	"math"
	"sync"
	"time"

	statepb "wowagent/gen/statev1"

	"wowagent/internal/wire"
)

// Melee feedback from the server: refused swings (out of range, bad facing) and the outcome of every
// attack (SMSG_ATTACKERSTATEUPDATE): whether our hits land and whether the target dodged (Overpower).

const (
	victimDodge  = 2 // VictimState VICTIMSTATE_DODGE
	victimParry  = 3
	victimBlocks = 5

	hitMiss  = 0x10
	hitBlock = 0x2000

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
	side       map[uint64]sideSeen  // attacker -> where it stood last sample (fightlog)
	taken      [3]sideStats         // blows aimed at us: in front, behind, side unknown
}

// Blows taken by side (measure of 2026-10-24): a player hit from behind cannot dodge, parry or block
// (Unit::RollMeleeOutcomeAgainst), and the bot only faces its current target, so in a fight against
// several monsters some stand in its back. These counts tell how many blows come from behind and how
// many the front ones avoid, before teaching the bot to keep them in front.
type sideStats struct {
	Blows, Damage, Dodge, Parry, Block, Miss uint32
}

type sideSeen struct {
	front bool
	at    time.Time
}

const (
	sideFront = iota
	sideBack
	sideUnknown
)

// inFront tells whether o stands in our front half (HasInArc(M_PI)).
func inFront(me *statepb.SelfInfo, o *statepb.Obj) bool {
	a := math.Atan2(float64(o.Y-me.Y), float64(o.X-me.X)) - float64(me.O)
	a = math.Mod(a+3*math.Pi, 2*math.Pi) - math.Pi
	return math.Abs(a) <= math.Pi/2
}

func newMeleeLog() *meleeLog {
	return &meleeLog{hit: map[uint64]time.Time{}, dodge: map[uint64]time.Time{}, side: map[uint64]sideSeen{}}
}

// saw records the side of an attacker (fight samples, once a second).
func (m *meleeLog) saw(attacker uint64, front bool) {
	m.mu.Lock()
	m.side[attacker] = sideSeen{front, time.Now()}
	m.mu.Unlock()
}

// takenSoFar returns the blows taken since the start, by side.
func (m *meleeLog) takenSoFar() [3]sideStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.taken
}

// takeBlow counts one blow aimed at us, by the side its attacker stood on (unknown when not seen in
// the last 2 s).
func (m *meleeLog) takeBlow(attacker uint64, hitInfo, damage uint32, state uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()
	side := sideUnknown
	if seen, ok := m.side[attacker]; ok && time.Since(seen.at) < 2*time.Second {
		side = sideBack
		if seen.front {
			side = sideFront
		}
	}
	t := &m.taken[side]
	t.Blows++
	t.Damage += damage
	switch {
	case hitInfo&hitMiss != 0:
		t.Miss++
	case state == victimDodge:
		t.Dodge++
	case state == victimParry:
		t.Parry++
	case state == victimBlocks || hitInfo&hitBlock != 0:
		t.Block++
	}
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
	hitInfo, attacker, target, damage, state, ok := parseAttackerState(payload)
	if ok && target == self && attacker != self {
		m.takeBlow(attacker, hitInfo, damage, state)
		return
	}
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
func parseAttackerState(payload []byte) (hitInfo uint32, attacker, target uint64, damage uint32, state uint8, ok bool) {
	r := wire.NewReader(payload)
	hitInfo = r.U32()
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
	return hitInfo, attacker, target, damage, state, !r.Err
}
