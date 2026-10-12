package combat

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/dbc"
	"wowagent/internal/wire"
)

// "Can cast": before trying a spell, check what the client can know so that the server does not
// refuse it (out of range, not ready, no rage, wrong aura state): Spell.dbc data + the cooldowns
// the server sends (SMSG_SPELL_COOLDOWN, SMSG_CLEAR_COOLDOWN).

// spellBook holds the spell data (loaded once) and our running cooldowns.
type spellBook struct {
	once  sync.Once
	info  map[uint32]dbc.SpellInfo
	mu    sync.Mutex
	ready map[uint32]time.Time // spell -> end of its cooldown
	cats  map[uint32]time.Time // cooldown category -> end
	gcd   time.Time            // end of the global cooldown
	// weapon spells wait until then: main-hand weapon missing or broken (rechecked every minute), or a
	// short pause after an unexplained refusal. Being disarmed is read from our unit flags instead.
	noWeapon    time.Time
	weaponCheck bool // a weapon refusal (code 30) arrived: look at the main hand
}

func (b *spellBook) load() map[uint32]dbc.SpellInfo {
	b.once.Do(func() {
		dir := os.Getenv("DBC_DIR")
		if dir == "" {
			dir = "/dbc"
		}
		m, err := dbc.LoadSpellInfo(dir)
		if err != nil {
			slog.Warn("no spell data: casts are not checked before sending", "err", err)
		}
		b.info = m
	})
	return b.info
}

// onCooldown reads SMSG_SPELL_COOLDOWN: our GUID, flags, then (spell, cooldown ms) pairs.
func (b *spellBook) onCooldown(payload []byte, me uint64) {
	r := wire.NewReader(payload)
	if r.U64() != me {
		return
	}
	r.U8()
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	for r.Left() >= 8 {
		spell, ms := r.U32(), r.U32()
		if ms > 0 {
			b.ready[spell] = now.Add(time.Duration(ms) * time.Millisecond)
		}
	}
}

// onClear reads SMSG_CLEAR_COOLDOWN: spell, GUID.
func (b *spellBook) onClear(payload []byte) {
	r := wire.NewReader(payload)
	spell := r.U32()
	if r.Err {
		return
	}
	b.mu.Lock()
	delete(b.ready, spell)
	b.mu.Unlock()
}

// started records a cast the server accepted: the client counts the cooldowns itself (the server only
// sends the unusual ones), from Spell.dbc: the spell's, its category's and the global cooldown.
func (b *spellBook) started(spell uint32) {
	info, ok := b.load()[spell]
	if !ok {
		return
	}
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cats == nil {
		b.cats = map[uint32]time.Time{}
	}
	if cd := max(info.Recovery, info.CategoryRecovery); cd > 0 {
		b.ready[spell] = now.Add(time.Duration(cd) * time.Millisecond)
	}
	if info.Category != 0 && info.CategoryRecovery > 0 {
		b.cats[info.Category] = now.Add(time.Duration(info.CategoryRecovery) * time.Millisecond)
	}
	if info.GCD > 0 {
		b.gcd = now.Add(time.Duration(info.GCD) * time.Millisecond)
	}
}

// unitFlagDisarmed is set on us while disarmed (UNIT_FLAG_DISARMED): weapon spells are refused only
// for that time (a few seconds), not for a fixed delay.
const unitFlagDisarmed = 0x00200000

// weaponRefused records SMSG_CAST_FAILED 30 (EQUIPPED_ITEM_CLASS): the fight loop then looks at the
// main hand (checkWeapon); meanwhile weapon spells pause 3 s.
func (b *spellBook) weaponRefused() {
	b.mu.Lock()
	b.noWeapon = time.Now().Add(3 * time.Second)
	b.weaponCheck = true
	b.mu.Unlock()
}

// checkWeapon runs after a weapon refusal: a missing or broken main-hand weapon blocks weapon spells
// for a minute (then it is looked at again, e.g. after a repair); otherwise the 3 s pause stands.
func (s *Service) checkWeapon(ctx context.Context) {
	b := s.book
	b.mu.Lock()
	need := b.weaponCheck
	b.weaponCheck = false
	b.mu.Unlock()
	if !need {
		return
	}
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return
	}
	usable := false
	for _, it := range snap.Items {
		if it.Bag == 255 && it.BagSlot == mainHandSlot {
			usable = it.MaxDurability == 0 || it.Durability > 0
		}
	}
	if !usable {
		slog.Info("main-hand weapon missing or broken: weapon spells paused")
		b.mu.Lock()
		b.noWeapon = time.Now().Add(time.Minute)
		b.mu.Unlock()
	}
}

// mainHandSlot is EQUIPMENT_SLOT_MAINHAND.
const mainHandSlot = 15

func (b *spellBook) weaponless() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return time.Now().Before(b.noWeapon)
}

// cooling tells whether the spell is still on cooldown (its own, its category's, or the global one).
func (b *spellBook) cooling(spell uint32) bool {
	info := b.load()[spell]
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	if now.Before(b.ready[spell]) {
		return true
	}
	if info.Category != 0 && now.Before(b.cats[info.Category]) {
		return true
	}
	return info.GCD > 0 && now.Before(b.gcd)
}

// can is canCast without the reason.
func (s *Service) can(spell uint32, me *statepb.SelfInfo, t *statepb.Obj, dist float32) bool {
	ok, _ := s.book.canCast(spell, me, t, dist)
	return ok
}

// canCast checks a spell before sending it: cooldown, power, aura states and range (dist is the 3D
// distance to the target; t nil = self cast). An unknown spell is allowed (the server decides).
func (b *spellBook) canCast(spell uint32, me *statepb.SelfInfo, t *statepb.Obj, dist float32) (bool, string) {
	info, ok := b.load()[spell]
	if !ok {
		return true, ""
	}
	if b.cooling(spell) {
		return false, "cooldown"
	}
	if info.NeedsWeapon && (me.UnitFlags&unitFlagDisarmed != 0 || b.weaponless()) {
		return false, "no usable weapon"
	}
	if info.PowerType >= 0 && uint32(info.PowerType) == me.PowerType && info.Cost > me.Power {
		return false, "power"
	}
	if info.CasterAuraState > 0 && !auraState(me.AuraState, info.CasterAuraState) {
		return false, "caster aura state"
	}
	if t == nil {
		return true, ""
	}
	if info.TargetAuraState > 0 && !auraState(t.AuraState, info.TargetAuraState) {
		return false, "target aura state"
	}
	switch {
	case info.Melee:
		if dist > meleeRange+0.5 {
			return false, "out of melee range"
		}
	case info.MaxRange > 0:
		// Server ranges add the combat reach of both units (about 1.5 yards each).
		if dist > info.MaxRange+2 {
			return false, "out of range"
		}
		if info.MinRange > 0 && dist < info.MinRange+0.5 {
			return false, "too close"
		}
	}
	return true, ""
}
