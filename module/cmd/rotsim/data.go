package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"wowagent/internal/dbc"
	"wowagent/internal/mpq"
	"wowagent/internal/warrior"
)

// spell is one rank of a spell, as the client knows it (Spell.dbc, SpellDuration.dbc).
type spell struct {
	id                    uint32
	level, base, maxLevel int
	cost                  float64    // rage
	points                [3]float64 // EffectBasePoints + 1 (+ per level), by effect
	perLevel              [3]float64
	dur, amp, cd          float64 // seconds
}

// value is an effect's amount at our level (base points grow per level up to the spell's maximum level).
func (s *spell) value(e, level int) float64 {
	l := level
	if s.maxLevel > 0 && l > s.maxLevel {
		l = s.maxLevel
	}
	return s.points[e] + s.perLevel[e]*float64(max(l-s.base, 0))
}

// gameData is what the simulation reads from the client and the world database.
type gameData struct {
	spells    map[uint32]*spell
	critBase  float64   // warrior melee crit chance with no agility
	critAgi   []float64 // crit chance per point of agility, by level - 1
	octRegen  []float64 // out of combat health regeneration per point of spirit (the first 50), by level - 1
	player    map[int]playerStats
	mobs      map[int]mobStats
	learnedUs map[int]float64 // our damage per second by level (journal/learn/dps.json)
}

type playerStats struct{ hp, str, agi, sta, spi float64 }
type mobStats struct{ hp, armor, ap, dmg float64 }

func loadGame(client, learnDir string, db *sql.DB) (*gameData, error) {
	set, err := mpq.OpenClient(client)
	if err != nil {
		return nil, err
	}
	defer set.Close()
	read := func(name string) (*dbc.File, error) {
		b, err := set.Read(`DBFilesClient\` + name + `.dbc`)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return dbc.Parse(name, b)
	}
	g := &gameData{spells: map[uint32]*spell{}, player: map[int]playerStats{}, mobs: map[int]mobStats{}, learnedUs: map[int]float64{}}
	durs := map[uint32]float64{}
	f, err := read("SpellDuration")
	if err != nil {
		return nil, err
	}
	for i := 0; i < f.Records; i++ {
		durs[f.U32(i, 0)] = float64(int32(f.U32(i, 1))) / 1000
	}
	want := map[uint32]bool{}
	for _, ids := range warrior.SpellGroups {
		for _, id := range ids {
			want[id] = true
		}
	}
	if f, err = read("Spell"); err != nil {
		return nil, err
	}
	// Spell.dbc 3.3.5a fields: 29 recovery, 30 category recovery, 37 max level, 38 base level,
	// 39 spell level, 40 duration index, 42 cost, 80-82 base points, 77-79 points per level,
	// 98-100 amplitude.
	for r := 0; r < f.Records; r++ {
		id := f.U32(r, 0)
		if !want[id] {
			continue
		}
		s := &spell{id: id, level: int(f.U32(r, 39)), base: int(f.U32(r, 38)), maxLevel: int(f.U32(r, 37)),
			cost: float64(f.U32(r, 42)) / 10, dur: durs[f.U32(r, 40)],
			cd: float64(max(f.U32(r, 29), f.U32(r, 30))) / 1000}
		for e := 0; e < 3; e++ {
			s.points[e] = float64(int32(f.U32(r, 80+e))) + 1
			s.perLevel[e] = float64(f.F32(r, 77+e))
			if e == 0 {
				s.amp = float64(f.U32(r, 98)) / 1000
			}
		}
		g.spells[id] = s
	}
	for _, t := range []struct {
		name string
		dst  *[]float64
	}{{"gtChanceToMeleeCrit", &g.critAgi}, {"gtOCTRegenHP", &g.octRegen}} {
		f, err := read(t.name)
		if err != nil {
			return nil, err
		}
		for lvl := 0; lvl < 100; lvl++ { // warrior: class 1, the first 100 rows
			*t.dst = append(*t.dst, float64(f.F32(lvl, 0)))
		}
	}
	if f, err = read("gtChanceToMeleeCritBase"); err != nil {
		return nil, err
	}
	g.critBase = float64(f.F32(0, 0))

	rows, err := db.Query("select Level, BaseHP, Strength, Agility, Stamina, Spirit from player_class_stats where Class = 1")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l int
		var p playerStats
		if err := rows.Scan(&l, &p.hp, &p.str, &p.agi, &p.sta, &p.spi); err != nil {
			return nil, err
		}
		g.player[l] = p
	}
	rows.Close()
	// Monsters: a warrior-class monster with no modifier (the ones met from 5 to 12 have 1, Forest
	// Spider 0.9 damage, Young Forest Bear 1.25 health).
	if rows, err = db.Query("select level, basehp0, basearmor, attackpower, damage_base from creature_classlevelstats where class = 1"); err != nil {
		return nil, err
	}
	for rows.Next() {
		var l int
		var m mobStats
		if err := rows.Scan(&l, &m.hp, &m.armor, &m.ap, &m.dmg); err != nil {
			return nil, err
		}
		g.mobs[l] = m
	}
	rows.Close()
	if b, err := os.ReadFile(learnDir + "/dps.json"); err == nil {
		var t struct {
			Ours map[string]struct{ DPS float64 } `json:"ours"`
		}
		if json.Unmarshal(b, &t) == nil {
			for k, v := range t.Ours {
				if c, l, ok := strings.Cut(k, ":"); ok && c == "1" {
					n, _ := strconv.Atoi(l)
					g.learnedUs[n] = v.DPS
				}
			}
		}
	}
	return g, nil
}

// rank is the highest rank of a spell known at a level (trained as soon as possible), or nil.
func (g *gameData) rank(ids []uint32, level int) *spell {
	for _, id := range ids { // highest first
		if s := g.spells[id]; s != nil && s.level <= level {
			return s
		}
	}
	return nil
}

// weaponOf reads a weapon's damage and speed from the world database.
func weaponOf(db *sql.DB, entry int) (weapon, error) {
	var w weapon
	err := db.QueryRow("select name, dmg_min1, dmg_max1, delay from item_template where entry = ?", entry).Scan(&w.name, &w.min, &w.max, &w.speed)
	w.speed /= 1000
	return w, err
}

type weapon struct {
	name            string
	min, max, speed float64
}
