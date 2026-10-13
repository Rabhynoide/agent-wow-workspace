// Package xpscore ranks monster types by experience per minute of a full cycle (user's model,
// 2026-10-12):
//
//	XP/min = XP_kill / (T_kill + T_rest + T_search + P_death × T_death)
//
// The monster side comes from the world database (level range, health and damage modifiers,
// attack speed, spawn density and respawn time). Our side is measured: the `fight` lines of
// logs/combat.log give, per class and level gap, the health lost and the duration of a fight
// against one monster (Table). Gaps without enough fights are extrapolated from the nearest
// measured one. The death risk of the model is corrected by the deaths measured against each
// monster type (all its fights, chained ones included).
package xpscore

import (
	"fmt"
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
	PDeath  float64 // model blended with the measured deaths (Table.DeathRisk)
	// Measured fights against this monster at its level gaps: fights, deaths, chained fights and
	// deaths in them.
	Fights, Deaths, Chained, ChainedDeaths int
	XPMin                                  float64
}

// Model constants. Measured or chosen on the 2026-10-12 benches (human warrior, level 5).
const (
	restSPerHealth = 0.36  // seconds of rest (eating) per percent of health
	runSpeed       = 7.0   // yards per second
	deathS         = 150.0 // run to the corpse, wait for a safe spot, rest
	addChance      = 0.35  // chance a nearby hostile spawn joins the fight
	// Extra loss per add, in single fights: won fights started at 90 %+ lost 2-2.5 times more with 2
	// attackers and 2.5-3 times with 3 (logs of 2026-10-12).
	addCost     = 1.5
	startHealth = 85.0 // average health when a fight starts (rest below 75 %, up to 90 %)
	deathSpread = 10.0
)

// Limits on a monster type (auto, level.sh) and on a pull (combat).
const (
	// MaxDeathRisk: above this death risk per kill, a monster type is not hunted.
	MaxDeathRisk = 0.1
	// MaxLoss: above this expected health loss (percent) for one fight, the monster is not fought on
	// purpose. The worst fights cost about twice the average (gap +1: 28 % on average, 56 % at worst),
	// so a 45 % average can still kill from 85 %.
	MaxLoss = 45.0
	// PullReserve: a pull is engaged only if its expected loss leaves this much health (percent).
	PullReserve = 35.0
)

// Single is one fight against one monster of level lv (no add): the expected health loss (percent)
// and the death risk (model blended with the deaths measured against that monster type).
func Single(m Mob, lv, level, class uint32, t *Table) (loss, pDeath float64) {
	speed := 1.0
	if m.AttackMs > 0 {
		speed = 2000 / float64(m.AttackMs)
	}
	gap := int(lv) - int(level)
	loss1, _ := t.Fight(class, gap)
	loss = loss1 * orOne(m.HealthMod) * orOne(m.DamageMod) * speed
	return loss, t.DeathRisk(class, m.Entry, gap, deathModel(loss))
}

func deathModel(loss float64) float64 { return 1 / (1 + math.Exp(-(loss-startHealth)/deathSpread)) }

// Risky explains why a monster type should not be fought on purpose at this level ("" when fine):
// one fight costs more than MaxLoss on average or its death risk is above MaxDeathRisk. The server
// picks a spawn's level in the range, so both are averaged over it (bench 2026-10-12: judging Defias
// Thugs 3-4 at their level 4 refused them at level 2, after 27 fights without a death).
func Risky(m Mob, level, class uint32, t *Table) string {
	hi := max(m.MaxLevel, m.MinLevel)
	loss, p, n := 0.0, 0.0, 0.0
	for lv := m.MinLevel; lv <= hi; lv++ {
		l, d := Single(m, lv, level, class, t)
		loss, p, n = loss+l, p+d, n+1
	}
	loss, p = loss/n, p/n
	if loss > MaxLoss || p > MaxDeathRisk {
		return fmt.Sprintf("expected loss %.0f %% of health, death risk %.0f %%", loss, 100*p)
	}
	return ""
}

// PullLoss is the expected health loss (percent) of a fight against monsters of these levels at once
// (the target and its adds), from the single fights: their average, times 1 + addCost per add.
func PullLoss(levels []uint32, level, class uint32, t *Table) float64 {
	if len(levels) == 0 {
		return 0
	}
	sum := 0.0
	for _, lv := range levels {
		l, _ := t.Fight(class, int(lv)-int(level))
		sum += l
	}
	return sum / float64(len(levels)) * (1 + addCost*float64(len(levels)-1))
}

// LossLeft is the expected health loss (percent) still to come in a fight against these attackers,
// each with the part of its health it has left (0-1): PullLoss with every single fight weighted by
// what remains of it.
func LossLeft(levels []uint32, left []float64, level, class uint32, t *Table) float64 {
	if len(levels) == 0 {
		return 0
	}
	sum := 0.0
	for i, lv := range levels {
		l, _ := t.Fight(class, int(lv)-int(level))
		sum += l * left[i]
	}
	return sum / float64(len(levels)) * (1 + addCost*float64(len(levels)-1))
}

// Of scores a monster type for a character of this class and level.
func Of(m Mob, level, class uint32, t *Table) Score {
	if m.MaxLevel < m.MinLevel {
		m.MaxLevel = m.MinLevel
	}
	hm, exp := orOne(m.HealthMod), orOne(m.ExpMod)
	var s Score
	n := 0.0
	for lv := m.MinLevel; lv <= m.MaxLevel; lv++ {
		gap := int(lv) - int(level)
		_, secs := t.Fight(class, gap)
		single, _ := Single(m, lv, level, class, t)
		kill := secs * hm
		adds := addChance * m.Neighbours
		loss := single * (1 + addCost*adds)
		s.XP += float64(BaseGain(level, lv)) * exp
		s.KillS += kill * (1 + adds)
		s.Loss += loss
		s.PDeath += t.DeathRisk(class, m.Entry, gap, deathModel(loss))
		c := t.Measured(class, m.Entry, gap)
		s.Fights += c.N
		s.Deaths += c.Deaths
		s.Chained += c.Chained
		s.ChainedDeaths += c.ChainedDeaths
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
