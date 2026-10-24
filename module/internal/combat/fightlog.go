package combat

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	statepb "wowagent/gen/statev1"
)

// fightRecord samples a fight once a second and logs one line at its end: our health (start, lowest,
// end), the most attackers at once and who they were. Without it a death could not be explained
// (bench c8edda2: "combat: dead" and nothing else).
type fightRecord struct {
	stop     context.CancelFunc
	wg       sync.WaitGroup
	start    time.Time
	target   uint64
	entry    uint32
	level    uint32
	myLevel  uint32
	class    uint32
	maxHP    uint32
	hpStart  uint32
	sampled  bool // hpStart, myLevel, class, maxHP are set
	hpMin    uint32
	hpEnd    uint32
	most     int
	attacker map[uint32]bool // entries that targeted us
	taken    [3]sideStats    // melee.taken at the start (blows by side, melee.go)
}

func (s *Service) watchFight(ctx context.Context, guid uint64) *fightRecord {
	r := &fightRecord{start: time.Now(), target: guid, hpMin: 100, attacker: map[uint32]bool{}, taken: s.melee.takenSoFar()}
	if t, err := s.api.Obj(ctx, guid); err == nil {
		r.entry, r.level = t.Entry, t.Level
	}
	var sub context.Context
	sub, r.stop = context.WithCancel(ctx)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			s.sampleFight(sub, r)
			if !sleep(sub, time.Second) {
				return
			}
		}
	}()
	return r
}

func (s *Service) sampleFight(ctx context.Context, r *fightRecord) {
	b, err := s.api.Brief(ctx)
	if err != nil || b.Self == nil {
		return
	}
	me := b.Self
	hp := pct(me.Hp, me.MaxHp)
	if me.Dead {
		hp = 0
	}
	if !r.sampled { // the first sample that worked (a failed Brief left hp_start at 0)
		r.sampled = true
		r.hpStart, r.myLevel, r.class, r.maxHP = hp, me.Level, me.ClassId, me.MaxHp
	}
	r.hpMin, r.hpEnd = min(r.hpMin, hp), hp
	n, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 40, Max: 12, AliveOnly: true})
	if err != nil {
		return
	}
	count := 0
	var foes []uint64
	for _, o := range n.Objs {
		if o.Target == me.Guid && o.UnitFlags&unitInCombat != 0 {
			count++
			r.attacker[o.Entry] = true
			s.melee.saw(o.Guid, inFront(me, o))
			foes = append(foes, o.Guid)
		}
	}
	r.most = max(r.most, count)
	s.encounterSaw(foes, hp)
}

func (r *fightRecord) done(s *Service, dead bool, why string) string {
	r.stop()
	r.wg.Wait()
	var entries []uint32
	for e := range r.attacker {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i] < entries[j] })
	result := "left"
	switch {
	case why == "dead":
		result = "died"
	case dead:
		result = "won"
	case why != "":
		result = why
	}
	slog.Info("fight", "result", result, "target", r.entry, "target_level", r.level, "level", r.myLevel, "class", r.class, "max_hp", r.maxHP,
		"hp_start", r.hpStart, "hp_min", r.hpMin, "hp_end", r.hpEnd, "attackers_max", r.most,
		"attackers", entries, "s", int(time.Since(r.start).Seconds()))
	// Blows taken in this fight by side ("front" = blows, damage, dodged, parried, blocked, missed).
	now := s.melee.takenSoFar()
	var side [3][6]uint32
	for i := range side {
		a, b := now[i], r.taken[i]
		side[i] = [6]uint32{a.Blows - b.Blows, a.Damage - b.Damage, a.Dodge - b.Dodge, a.Parry - b.Parry, a.Block - b.Block, a.Miss - b.Miss}
	}
	if side[0][0]+side[1][0]+side[2][0] > 0 {
		slog.Info("blows taken", "target", r.entry, "attackers_max", r.most, "front", side[sideFront], "back", side[sideBack], "unknown", side[sideUnknown])
	}
	return result
}
