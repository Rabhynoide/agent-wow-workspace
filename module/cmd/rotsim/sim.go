package main

import (
	"math"
	"math/rand"

	"wowagent/internal/warrior"
)

// policy is a warrior rotation, the choices the priority table of journal/classes/warrior.yaml makes.
// Victory Rush, Execute and Overpower are always first when usable (free or nearly, and procs that run
// out); Charge opens every pull it is ready for.
type policy struct {
	RendMin      float64 // Rend the target when it dies in more than this many seconds; < 0 never
	RendAll      bool    // also Rend the other attackers (no target switch for the swings)
	DiesIn       int     // the estimate: 0 the bot's (99 for the first 2 s on a target), 1 the learned damage per second until then
	TCOne        float64 // rage for Thunder Clap on a lone enemy; 0 never
	TCMulti      bool    // Thunder Clap when 2+ enemies are in reach
	TCFirst      bool    // Thunder Clap before Rend on a lone enemy
	TCMultiFirst bool    // Thunder Clap before Rend on 2+ enemies
	HS           float64 // rage for Heroic Strike; 0 never
	HSEvery      float64 // seconds between two Heroic Strikes queued (the table's every)
	Shout        int     // Battle Shout: 0 never, 1 before a pull when < 30 s left, 2 and in combat when it runs out, 3 and right after a fight with the rage left
	Demo         int     // attackers for Demoralizing Shout (when one lacks it); 0 never
	Blood        int     // Bloodrage: 0 never, 1 before a pull for the shout, 2 at every pull, 3 both 1 and in combat under 20 rage, 4 in combat only
	Cleave       float64 // rage for Cleave on 2+ enemies; 0 never
}

// scenario is a series of pulls of the same kind.
type scenario struct {
	level, foes, diff int
	pulls             int     // pulls in a chain
	downtime          float64 // seconds out of combat between two pulls (walking, looting)
	restBelow         float64 // health share under which we rest before a pull
	deathCost         float64 // seconds (tuning death_cost_s)
}

// char is our warrior at a level.
type char struct {
	level              int
	ap, crit, maxhp    float64
	armor, avoid       float64
	w                  weapon
	conv               float64 // rage conversion (Unit::RewardRage)
	regen              float64 // health per second out of combat (sitting: x1.5)
	expDPS             float64 // what the learned estimate expects
	shout, rend, hs    *spell
	tc, vr, op, blood  *spell
	demo, cleave, exe  *spell
	charge             *spell
	impRend, impTC     int // talent ranks (journal/classes/warrior.yaml talent_order)
	impHS, impCharge   int
	impOP              int
	mobCalib, chargeOK float64
}

// talent ranks at a level, from the talent order of journal/classes/warrior.yaml.
func talentsAt(l int) (rend, tc, hs, charge, deflection, op int) {
	rank := func(from, n int) int { return min(max(l-from+1, 0), n) }
	rend = rank(10, 2)
	if l < 25 {
		tc = rank(12, 3)
	} else if l >= 75 {
		tc = rank(75, 3)
	}
	return rend, tc, rank(15, 3), rank(18, 2), rank(20, 3), rank(23, 2)
}

func (g *gameData) char(level int, w weapon, gearStr, armor float64) *char {
	p := g.player[level]
	str := p.str + gearStr
	c := &char{level: level, w: w, armor: armor, mobCalib: 1.17, chargeOK: 1}
	c.ap = float64(3*level) + 2*str - 20
	c.crit = g.critBase + p.agi*g.critAgi[level-1]
	c.maxhp = p.hp + min(p.sta, 20) + max(p.sta-20, 0)*10
	c.conv = 0.0091107836*float64(level*level) + 3.225598133*float64(level) + 4.2652911
	spi := p.spi
	c.regen = (min(spi, 50)*g.octRegen[level-1] + max(spi-50, 0)*0.5) / 2 // every 2 s (Player::RegenerateHealth)
	var defl int
	c.impRend, c.impTC, c.impHS, c.impCharge, defl, c.impOP = talentsAt(level)
	// Avoidance measured in front (series of 2026-10-29: 45 of 254 blows: dodge, parry, block, miss).
	c.avoid = 0.17 + 0.01*float64(defl)
	c.shout, c.rend, c.hs = g.rank(warrior.BattleShout, level), g.rank(warrior.Rend, level), g.rank(warrior.HeroicStrike, level)
	c.tc, c.vr, c.op, c.blood = g.rank(warrior.ThunderClap, level), g.rank(warrior.VictoryRush, level), g.rank(warrior.Overpower, level), g.rank(warrior.Bloodrage, level)
	c.demo, c.cleave, c.exe = g.rank(warrior.DemoShout, level), g.rank(warrior.Cleave, level), g.rank(warrior.Execute, level)
	c.charge = g.rank(warrior.Charge, level)
	if v, ok := g.learnedUs[level]; ok {
		c.expDPS = v
	} else {
		c.expDPS = 2 * (w.min + w.max) / 2 / w.speed
	}
	return c
}

type mob struct {
	hp, maxhp, armor  float64
	min, max, speed   float64
	ap                float64
	next              float64
	rendEnd, rendNext float64
	rendTick          float64
	tcEnd, demoEnd    float64
	level             int
}

// result sums a chain up.
type result struct {
	fight, rest, deaths, hpLost float64
	pulls                       int
	casts                       map[string]float64
}

func (r *result) perPull(deathCost float64) float64 {
	return (r.fight + r.rest + r.deaths*deathCost) / float64(r.pulls)
}

// sim runs one chain of pulls.
type sim struct {
	g   *gameData
	c   *char
	p   *policy
	sc  *scenario
	rng *rand.Rand
	res *result

	t, rage, hp      float64
	gcd, swing       float64
	hsQueued         bool
	cleaveQueued     bool
	hsMark           float64
	shoutEnd         float64
	vrEnd, opEnd     float64
	cds              map[uint32]float64 // ready at (absolute time, across pulls)
	bloodEnd         float64
	clock            float64 // absolute time across the chain
	targetHP0, tObs0 float64
	target           int
}

func newSim(g *gameData, c *char, p *policy, sc *scenario, seed int64) *sim {
	return &sim{g: g, c: c, p: p, sc: sc, rng: rand.New(rand.NewSource(seed)), res: &result{casts: map[string]float64{}},
		hp: c.maxhp, cds: map[uint32]float64{}, shoutEnd: -1, vrEnd: -1}
}

func (s *sim) ap(m *mob) float64 {
	ap := s.c.ap
	if s.clock < s.shoutEnd && s.c.shout != nil {
		ap += s.c.shout.value(0, s.c.level)
	}
	return ap
}

func (s *sim) ready(sp *spell) bool { return sp != nil && s.clock >= s.cds[sp.id] }

func (s *sim) use(sp *spell, name string, cost float64) {
	s.rage -= cost
	if sp.cd > 0 {
		s.cds[sp.id] = s.clock + sp.cd
	}
	s.res.casts[name]++
}

// roll is the attack table against a monster: 0 miss, 1 dodge, 2 parry, 3 glancing, 4 crit, 5 hit.
func (s *sim) roll(m *mob, white bool, extraCrit float64) int {
	sd := float64(5 * (m.level - s.c.level)) // defense over weapon skill
	miss := 0.05 + 0.001*sd
	dodge := max(0.05+0.001*sd, 0)
	parry := max(0.05+0.001*sd, 0)
	r := s.rng.Float64()
	if r < miss {
		return 0
	}
	if r -= miss; r < dodge {
		return 1
	}
	if r -= dodge; r < parry {
		return 2
	}
	r -= parry
	crit := s.c.crit + extraCrit - 0.0004*sd
	if white {
		if glance := 0.10 + 0.01*sd; sd > 0 && r < glance {
			return 3
		} else if sd > 0 {
			r -= glance
		}
		if r < crit {
			return 4
		}
		return 5
	}
	if s.rng.Float64() < crit {
		return 4
	}
	return 5
}

func (s *sim) armorCut(m *mob) float64 {
	return 1 - m.armor/(m.armor+85*float64(s.c.level)+400)
}

func (s *sim) weaponDmg(m *mob, normalized bool) float64 {
	sp := s.c.w.speed
	if normalized {
		sp = 3.3
	}
	return s.c.w.min + s.rng.Float64()*(s.c.w.max-s.c.w.min) + s.ap(m)/14*sp
}

func (s *sim) hurt(m *mob, dmg float64) {
	m.hp -= dmg
}

// white is an auto attack, or the Heroic Strike or Cleave it is replaced with.
func (s *sim) white(mobs []*mob) {
	m := mobs[s.target]
	if s.hsQueued && s.c.hs != nil && s.rage >= s.c.hs.cost-float64(s.c.impHS) {
		s.hsQueued = false
		s.use(s.c.hs, "Heroic Strike", s.c.hs.cost-float64(s.c.impHS))
		s.yellow(m, s.weaponDmg(m, false)+s.c.hs.value(0, s.c.level), 0, true)
		return
	}
	s.hsQueued = false
	if s.cleaveQueued && s.c.cleave != nil && s.rage >= s.c.cleave.cost {
		s.cleaveQueued = false
		s.use(s.c.cleave, "Cleave", s.c.cleave.cost)
		n := 0
		for i := range mobs {
			k := (s.target + i) % len(mobs)
			if mobs[k].hp > 0 && n < 2 {
				s.yellow(mobs[k], s.weaponDmg(mobs[k], false)+s.c.cleave.value(0, s.c.level), 0, true)
				n++
			}
		}
		return
	}
	s.cleaveQueued = false
	o := s.roll(m, true, 0)
	if o <= 2 {
		if o == 1 {
			s.opEnd = s.clock + 5
		}
		return
	}
	d := s.weaponDmg(m, false) * s.armorCut(m)
	f := 3.5
	switch o {
	case 3:
		d *= 0.95 - 0.05*float64(m.level-s.c.level)
	case 4:
		d *= 2
		f = 7
	}
	d = math.Floor(d)
	s.hurt(m, d)
	add := (d/s.c.conv*7.5 + math.Floor(s.c.w.speed*f)) / 2
	s.rage = min(s.rage+math.Floor(add*10)/10, 100)
}

// yellow is a special attack: refunds 80 % of its cost when dodged or parried.
func (s *sim) yellow(m *mob, dmg, extraCrit float64, armor bool) int {
	o := s.roll(m, false, extraCrit)
	if o == 1 || o == 2 {
		if o == 1 {
			s.opEnd = s.clock + 5
		}
		return o
	}
	if o == 0 {
		return o
	}
	if armor {
		dmg *= s.armorCut(m)
	}
	if o == 4 {
		dmg *= 2
	}
	s.hurt(m, math.Floor(dmg))
	return o
}

func (s *sim) mobSwing(m *mob) {
	speed := m.speed
	if s.clock < m.tcEnd {
		speed /= 1 - 0.10 - []float64{0, 0.04, 0.07, 0.10}[s.c.impTC]
	}
	m.next += speed
	if s.rng.Float64() < s.c.avoid {
		return
	}
	ap := m.ap
	if s.clock < m.demoEnd && s.c.demo != nil {
		ap = max(ap+s.c.demo.value(0, s.c.level), 0)
	}
	bv := ap / 14 * m.speed
	d := (m.min + bv + s.rng.Float64()*(m.max-m.min)) * s.c.mobCalib
	if s.rng.Float64() < 0.05 {
		d *= 2
	}
	d *= 1 - s.c.armor/(s.c.armor+85*float64(m.level)+400)
	d = math.Floor(d)
	s.hp -= d
	s.res.hpLost += d
	s.rage = min(s.rage+math.Floor(d/s.c.conv*2.5*10)/10, 100)
}

func (s *sim) diesIn(m *mob, t float64) float64 {
	el := t - s.tObs0
	if el >= 2 && m.hp < s.targetHP0 {
		return m.hp / ((s.targetHP0 - m.hp) / el)
	}
	if s.p.DiesIn == 1 {
		return m.hp / s.c.expDPS
	}
	return 99
}

func (s *sim) alive(mobs []*mob) (n int) {
	for _, m := range mobs {
		if m.hp > 0 {
			n++
		}
	}
	return
}

func (s *sim) shoutCost() float64 { return s.c.shout.cost }

// outOfCombat: buffs before a pull (journal/classes/warrior.yaml buffs).
func (s *sim) beforePull() {
	if s.p.Shout >= 1 && s.c.shout != nil && s.shoutEnd-s.clock < 30 {
		if s.rage < s.shoutCost() && (s.p.Blood == 1 || s.p.Blood == 3) && s.ready(s.c.blood) {
			s.bloodrage()
		}
		if s.rage >= s.shoutCost() {
			s.use(s.c.shout, "Battle Shout", s.shoutCost())
			s.shoutEnd = s.clock + s.c.shout.dur
			s.clock += 1.5
			s.res.rest += 1.5
		}
	}
	if s.p.Blood == 2 && s.ready(s.c.blood) {
		s.bloodrage()
	}
}

func (s *sim) bloodrage() {
	s.use(s.c.blood, "Bloodrage", 0)
	s.rage = min(s.rage+s.c.blood.value(0, s.c.level)/10, 100)
	s.bloodEnd = s.clock + 10
}

// pull fights one pack; it returns false when we died.
func (s *sim) pull() bool {
	sc := s.sc
	mobs := make([]*mob, sc.foes)
	ms := s.g.mobs[s.c.level+sc.diff]
	for i := range mobs {
		mobs[i] = &mob{hp: ms.hp, maxhp: ms.hp, armor: ms.armor, min: ms.dmg, max: ms.dmg * 1.5, speed: 2, ap: ms.ap,
			level: s.c.level + sc.diff, rendEnd: -1, tcEnd: -1, demoEnd: -1}
		mobs[i].next = s.rng.Float64() * 1.0
	}
	if s.c.charge != nil && s.ready(s.c.charge) && s.rng.Float64() < s.c.chargeOK {
		s.use(s.c.charge, "Charge", 0)
		s.rage = min(s.rage+s.c.charge.value(1, s.c.level)/10+5*float64(s.c.impCharge), 100)
		for _, m := range mobs[:1] {
			m.next += 1 // stunned
		}
	}
	const dt = 0.05
	const react = 0.25 // the bot's tick after a cast
	s.target, s.targetHP0, s.tObs0 = 0, mobs[0].hp, 0
	s.swing, s.gcd, s.hsQueued, s.cleaveQueued, s.hsMark = 0, 0, false, false, -99
	inFight := true
	t := 0.0
	for ; inFight; t += dt {
		s.clock += dt
		if s.clock < s.bloodEnd {
			s.rage = min(s.rage+dt, 100) // 1 rage per second for 10 s
		}
		// bleeds
		for _, m := range mobs {
			if m.hp > 0 && m.rendEnd > 0 && t >= m.rendNext && m.rendNext <= m.rendEnd+1e-6 {
				m.hp -= math.Floor(m.rendTick)
				m.rendNext += s.c.rend.amp
			}
		}
		for _, m := range mobs {
			if m.hp > 0 && t >= m.next {
				s.mobSwing(m)
			}
		}
		if s.hp <= 0 {
			return false
		}
		if t >= s.swing {
			s.white(mobs)
			s.swing = t + s.c.w.speed
		}
		// target dead: Victory Rush proc, next target
		if mobs[s.target].hp <= 0 {
			if s.c.vr != nil {
				s.vrEnd = s.clock + 20
			}
			if s.alive(mobs) == 0 {
				break
			}
			for i, m := range mobs {
				if m.hp > 0 {
					s.target, s.targetHP0, s.tObs0 = i, m.hp, t
					break
				}
			}
		}
		s.decide(mobs, t, react)
	}
	s.res.fight += t
	return true
}

func (s *sim) decide(mobs []*mob, t, react float64) {
	c, p := s.c, s.p
	m := mobs[s.target]
	n := s.alive(mobs)
	// off the global cooldown
	if (p.Blood == 3 || p.Blood == 4) && s.rage < 20 && s.ready(c.blood) {
		s.bloodrage()
	}
	if p.Cleave > 0 && c.cleave != nil && n >= 2 && s.rage >= p.Cleave {
		s.cleaveQueued = true
	} else if p.HS > 0 && c.hs != nil && s.rage >= p.HS && !s.hsQueued && t-s.hsMark >= p.HSEvery {
		s.hsQueued, s.hsMark = true, t
	}
	if t < s.gcd {
		return
	}
	gcd := func(name string, sp *spell, cost float64) {
		s.use(sp, name, cost)
		s.gcd = t + 1.5 + react
	}
	if c.vr != nil && s.clock < s.vrEnd {
		s.vrEnd = -1
		gcd("Victory Rush", c.vr, 0)
		s.yellow(m, c.vr.value(0, c.level)/100*s.ap(m), 0, true)
		return
	}
	if c.exe != nil && m.hp < 0.2*m.maxhp && s.rage >= c.exe.cost {
		extra := min(s.rage-c.exe.cost, 15)
		gcd("Execute", c.exe, c.exe.cost+extra)
		s.yellow(m, c.exe.value(0, c.level)+3*extra+0.2*s.ap(m), 0, true)
		return
	}
	if c.op != nil && s.clock < s.opEnd && s.rage >= c.op.cost && s.ready(c.op) {
		s.opEnd = -1
		gcd("Overpower", c.op, c.op.cost)
		o := s.rng.Float64() < 0.05 // can only miss
		if !o {
			d := s.weaponDmg(m, true) * s.armorCut(m)
			if s.rng.Float64() < c.crit+0.25*float64(c.impOP) {
				d *= 2
			}
			s.hurt(m, math.Floor(d))
		}
		return
	}
	if p.Shout >= 2 && c.shout != nil && s.clock >= s.shoutEnd && s.rage >= c.shout.cost {
		gcd("Battle Shout", c.shout, c.shout.cost)
		s.shoutEnd = s.clock + c.shout.dur
		return
	}
	if p.Demo > 0 && c.demo != nil && n >= p.Demo && s.rage >= c.demo.cost {
		for _, o := range mobs {
			if o.hp > 0 && s.clock >= o.demoEnd {
				gcd("Demoralizing Shout", c.demo, c.demo.cost)
				for _, o := range mobs {
					o.demoEnd = s.clock + c.demo.dur
				}
				return
			}
		}
	}
	tcCost := 0.0
	if c.tc != nil {
		tcCost = c.tc.cost - []float64{0, 1, 2, 4}[c.impTC]
	}
	tc := func() bool {
		if c.tc == nil || !s.ready(c.tc) || s.rage < tcCost {
			return false
		}
		if !(p.TCMulti && n >= 2) && !(p.TCOne > 0 && n == 1 && s.rage >= p.TCOne) {
			return false
		}
		gcd("Thunder Clap", c.tc, tcCost)
		for _, o := range mobs {
			if o.hp > 0 && s.rng.Float64() >= 0.05 { // can only miss
				d := (c.tc.value(0, c.level) + 0.12*s.ap(o)) * (1 + 0.1*float64(c.impTC)) * s.armorCut(o)
				if s.rng.Float64() < c.crit {
					d *= 2
				}
				s.hurt(o, math.Floor(d))
				o.tcEnd = s.clock + c.tc.dur
			}
		}
		return true
	}
	rend := func() bool {
		if p.RendMin < 0 || c.rend == nil || s.rage < c.rend.cost {
			return false
		}
		try := func(o *mob, life float64) bool {
			if o.hp <= 0 || (o.rendEnd > 0 && o.rendEnd-t > 2) || o.hp < 0.25*o.maxhp || life <= p.RendMin {
				return false
			}
			gcd("Rend", c.rend, c.rend.cost)
			r := s.yellow(o, 0, -1, false) // no crit
			if r == 0 || r == 1 || r == 2 {
				if r != 0 {
					s.rage += 0.8 * c.rend.cost
				}
				return true
			}
			w := s.c.w
			o.rendTick = (c.rend.value(0, c.level) + 0.2*((w.min+w.max)/2+s.ap(o)*w.speed/14)) * (1 + 0.1*float64(c.impRend))
			o.rendEnd, o.rendNext = t+c.rend.dur, t+c.rend.amp
			return true
		}
		if try(m, s.diesIn(m, t)) {
			return true
		}
		if p.RendAll {
			for _, o := range mobs {
				if o != m && try(o, o.hp/c.expDPS+s.diesIn(m, t)) {
					return true
				}
			}
		}
		return false
	}
	if (n == 1 && p.TCFirst) || (n >= 2 && p.TCMultiFirst) {
		if tc() || rend() {
			return
		}
	} else if rend() || tc() {
		return
	}
}

// chain runs the pulls of a scenario.
func (s *sim) chain() *result {
	for i := 0; i < s.sc.pulls; i++ {
		// walking to the next pull: rage decays (1 per second), health comes back
		down := s.sc.downtime
		if i == 0 {
			down = 60
		}
		if s.p.Shout == 3 && s.c.shout != nil && s.shoutEnd-s.clock < 60 && s.rage >= s.shoutCost() {
			s.use(s.c.shout, "Battle Shout", s.shoutCost())
			s.shoutEnd = s.clock + s.c.shout.dur
		}
		s.clock += down
		s.rage = max(s.rage-down, 0)
		s.hp = min(s.hp+s.c.regen*down, s.c.maxhp)
		if s.hp < s.sc.restBelow*s.c.maxhp {
			need := (s.c.maxhp - s.hp) / (s.c.regen * 1.5)
			s.res.rest += need
			s.clock += need
			s.hp = s.c.maxhp
		}
		s.beforePull()
		s.res.pulls++
		if !s.pull() {
			s.res.deaths++
			s.hp, s.rage = s.c.maxhp, 0
		}
	}
	return s.res
}
