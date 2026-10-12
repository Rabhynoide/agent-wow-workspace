// Package xpscore ranks monster types by experience per minute of a full cycle (user's model,
// 2026-10-12):
//
//	XP/min = XP_kill / (T_kill + T_rest + T_search + P_death × T_death)
//
// The monster side comes from the world database (level range, health and damage modifiers,
// attack speed, spawn density and respawn time). Our side is measured: the `fight` lines of
// logs/combat.log give, per class and level gap, the health lost and the duration of a fight
// against one monster (Table). Gaps without enough fights are extrapolated from the nearest
// measured one.
package xpscore

import (
	"math"
)

// GrayLevel is the highest monster level that gives no experience (Acore::XP::GetGrayLevel).
func GrayLevel(pl uint32) uint32 {
	switch {
	case pl <= 5:
		return 0
	case pl <= 39:
		return pl - 5 - pl/10
	case pl <= 59:
		return pl - 1 - pl/5
	}
	return pl - 9
}

// zeroDifference is Acore::XP::GetZeroDifference.
func zeroDifference(pl uint32) uint32 {
	for _, z := range []struct{ below, zd uint32 }{{8, 5}, {10, 6}, {12, 7}, {16, 8}, {20, 9}, {30, 11}, {40, 12}, {45, 13}, {50, 14}, {55, 15}, {60, 16}} {
		if pl < z.below {
			return z.zd
		}
	}
	return 17
}

// BaseGain is the experience of a kill (Acore::XP::BaseGain, classic content: base 45), before
// the creature's ExperienceModifier and the server rate.
func BaseGain(pl, mob uint32) uint32 {
	const nBaseExp = 45
	if mob >= pl {
		diff := min(mob-pl, 4)
		return ((pl*5+nBaseExp)*(20+diff)/10 + 1) / 2
	}
	if mob <= GrayLevel(pl) {
		return 0
	}
	zd := zeroDifference(pl)
	return (pl*5 + nBaseExp) * (zd + mob - pl) / zd
}

// Mob is a monster type around the grind area.
type Mob struct {
	Entry              uint32
	Name               string
	MinLevel, MaxLevel uint32
	HealthMod          float64 // creature_template.HealthModifier
	DamageMod          float64 // creature_template.DamageModifier
	AttackMs           uint32  // creature_template.BaseAttackTime
	ExpMod             float64 // creature_template.ExperienceModifier
	Spawns             int     // spawn points in the area
	AreaRadius         float64 // radius of that area (yards)
	RespawnS           float64 // average spawntimesecs
	Neighbours         float64 // other hostile spawn points near each of its spawn points (average)
}

// Score is the estimated cycle of one kill.
type Score struct {
	XP      float64 // per kill
	KillS   float64
	RestS   float64
	SearchS float64
	Loss    float64 // percent of our health per kill, adds included
	PDeath  float64
	XPMin   float64
}

// Model constants. Measured or chosen on the 2026-10-12 benches (human warrior, level 5).
const (
	restSPerHealth = 0.36  // seconds of rest (eating) per percent of health
	runSpeed       = 7.0   // yards per second
	deathS         = 150.0 // run to the corpse, wait for a safe spot, rest
	addChance      = 0.35  // chance a nearby hostile spawn joins the fight
	addCost        = 2.0   // extra loss per add, in single fights (2 attackers: about 3 times the loss)
	startHealth    = 85.0  // average health when a fight starts (rest below 75 %, up to 90 %)
	deathSpread    = 10.0
)

// Of scores a monster type for a character of this class and level.
func Of(m Mob, level, class uint32, t *Table) Score {
	if m.MaxLevel < m.MinLevel {
		m.MaxLevel = m.MinLevel
	}
	hm, dm, exp := orOne(m.HealthMod), orOne(m.DamageMod), orOne(m.ExpMod)
	speed := 1.0
	if m.AttackMs > 0 {
		speed = 2000 / float64(m.AttackMs)
	}
	var s Score
	n := 0.0
	for lv := m.MinLevel; lv <= m.MaxLevel; lv++ {
		gap := int(lv) - int(level)
		loss1, secs := t.Fight(class, gap)
		kill := secs * hm
		adds := addChance * m.Neighbours
		loss := loss1 * hm * dm * speed * (1 + addCost*adds)
		s.XP += float64(BaseGain(level, lv)) * exp
		s.KillS += kill * (1 + adds)
		s.Loss += loss
		s.PDeath += 1 / (1 + math.Exp(-(loss-startHealth)/deathSpread))
		n++
	}
	s.XP, s.KillS, s.Loss, s.PDeath = s.XP/n, s.KillS/n, s.Loss/n, s.PDeath/n
	s.RestS = s.Loss * restSPerHealth
	if m.Spawns > 0 {
		area := math.Pi * m.AreaRadius * m.AreaRadius
		s.SearchS = math.Sqrt(area/float64(m.Spawns)) / runSpeed
		// Killing faster than they respawn empties the area: one kill per respawn / spawns.
		s.SearchS = max(s.SearchS, m.RespawnS/float64(m.Spawns)-s.KillS-s.RestS)
	} else {
		s.SearchS = 60
	}
	cycle := s.KillS + s.RestS + s.SearchS + s.PDeath*deathS
	if cycle > 0 {
		s.XPMin = 60 * s.XP * (1 - s.PDeath) / cycle
	}
	return s
}

func orOne(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}
