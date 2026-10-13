package combat

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/classdata"
	"wowagent/internal/priest"
	"wowagent/internal/warrior"
)

// Priority tables (Ovale model): the rotation of a class is a list of lines in
// journal/classes/<class>.yaml (priorities in a fight, buffs out of combat). The first line whose
// condition holds and whose spell passes canCast (cooldown, power, aura states, range, weapon) is
// cast. Conditions are small expressions:
//
//	hp < 40 and mana >= 12
//	target_aura_left("Rend") < 2 and target_dies_in > 8
//	dodged(4) or aura("Taste for Blood")
//
// Values are numbers (true = 1); operators: and, or, not, < <= > >= == !=, parentheses.

// env is what a condition can read during one step.
type env struct {
	ctx      context.Context
	s        *Service
	me       *statepb.SelfInfo
	t        *statepb.Obj // nil out of combat (buffs)
	dist     float32      // 3D distance to t
	known    map[uint32]bool
	started  time.Time
	diesIn   float64
	noEnemy  bool // spells on the enemy are skipped (priest: no line of sight, fighting in melee)
	visible  *bool
	attacked *[]*statepb.Obj
}

func (e *env) isVisible() bool {
	if e.visible == nil {
		v := e.t != nil && e.s.visible(e.ctx, e.t)
		e.visible = &v
	}
	return *e.visible
}

// attackers counts the units in combat that target us (within 30 yards).
func (e *env) attackers() int { return len(e.onMe()) }

// logGroupCast logs a spell cast while several units attack us (multi-target fights in the bench
// report).
func (e *env) logGroupCast(spell string) {
	if e.t != nil && len(e.onMe()) >= 2 {
		slog.Info("group cast", "spell", spell, "attackers", len(e.onMe()), "near", e.nearAttackers())
	}
}

// nearAttackers counts the attackers within reach of our area spells (Thunder Clap: 8 yards).
func (e *env) nearAttackers() int {
	n := 0
	for _, o := range e.onMe() {
		if o.Dist <= areaRange {
			n++
		}
	}
	return n
}

// onMe lists the units in combat that target us (within 30 yards), read once per step.
func (e *env) onMe() []*statepb.Obj {
	if e.attacked == nil {
		var out []*statepb.Obj
		r, err := e.s.api.Nearby(e.ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 30, Max: 12, AliveOnly: true})
		if err == nil {
			for _, o := range r.Objs {
				if o.Target == e.me.Guid && o.UnitFlags&unitInCombat != 0 {
					out = append(out, o)
				}
			}
		}
		e.attacked = &out
	}
	return *e.attacked
}

type expr func(*env) float64

// names resolves spell and aura names of a class.
type names struct {
	spells map[string][]uint32
	auras  map[string][]uint32 // procs and buffs that are not the spell itself
}

func classNames(class uint32) names {
	switch class {
	case warrior.ClassID:
		return names{warrior.SpellGroups, warrior.Auras}
	case priest.ClassID:
		return names{priest.SpellGroups, priest.Auras}
	}
	return names{}
}

func (n names) aura(name string) []uint32 {
	if ids, ok := n.auras[name]; ok {
		return ids
	}
	return n.spells[name]
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// seconds of an aura: 0 when missing, a large number when it has no duration.
func auraSeconds(auras []*statepb.Aura, ids []uint32, mine bool) float64 {
	left, ok := auraLeft(auras, ids, mine)
	if !ok {
		return 0
	}
	if left == math.MaxUint32 {
		return 1e6
	}
	return float64(left) / 1000
}

var variables = map[string]expr{
	"hp":        func(e *env) float64 { return float64(pct(e.me.Hp, e.me.MaxHp)) },
	"mana":      func(e *env) float64 { return float64(pct(e.me.Power, e.me.MaxPower)) },
	"rage":      func(e *env) float64 { return float64(e.me.Power / 10) },
	"level":     func(e *env) float64 { return float64(e.me.Level) },
	"in_combat": func(e *env) float64 { return b2f(e.me.UnitFlags&inCombatFlag != 0) },
	"attackers": func(e *env) float64 { return float64(e.attackers()) },
	// Attackers within 8 yards: what Thunder Clap, Cleave and Sweeping Strikes reach.
	"near_attackers": func(e *env) float64 { return float64(e.nearAttackers()) },
	"fight_s":        func(e *env) float64 { return time.Since(e.started).Seconds() },
	"dist":           func(e *env) float64 { return float64(e.dist) },
	"visible":        func(e *env) float64 { return b2f(e.isVisible()) },
	"target_hp": func(e *env) float64 {
		return withTarget(e, func(t *statepb.Obj) float64 { return float64(pct(t.Hp, t.MaxHp)) })
	},
	"target_level": func(e *env) float64 { return withTarget(e, func(t *statepb.Obj) float64 { return float64(t.Level) }) },
	"target_on_me": func(e *env) float64 {
		return withTarget(e, func(t *statepb.Obj) float64 { return b2f(t.Target == e.me.Guid) })
	},
	// Seconds before the target dies at the damage rate seen so far (99 without enough data).
	"target_dies_in": func(e *env) float64 { return e.diesIn },
}

func withTarget(e *env, f func(*statepb.Obj) float64) float64 {
	if e.t == nil {
		return 0
	}
	return f(e.t)
}

// functions take one argument: a name (string) or a number.
var functions = map[string]func(n names, name string, num float64) (expr, error){
	"aura": func(n names, name string, _ float64) (expr, error) {
		ids, err := need(n.aura(name), name)
		return func(e *env) float64 { return b2f(hasAura(e.me.Auras, ids...)) }, err
	},
	"my_aura_left": func(n names, name string, _ float64) (expr, error) {
		ids, err := need(n.aura(name), name)
		return func(e *env) float64 { return auraSeconds(e.me.Auras, ids, true) }, err
	},
	"target_aura": func(n names, name string, _ float64) (expr, error) {
		ids, err := need(n.aura(name), name)
		return func(e *env) float64 {
			return withTarget(e, func(t *statepb.Obj) float64 { return b2f(hasAura(t.Auras, ids...)) })
		}, err
	},
	"target_aura_left": func(n names, name string, _ float64) (expr, error) {
		ids, err := need(n.aura(name), name)
		return func(e *env) float64 {
			return withTarget(e, func(t *statepb.Obj) float64 { return auraSeconds(t.Auras, ids, true) })
		}, err
	},
	"known": func(n names, name string, _ float64) (expr, error) {
		ids, err := need(n.spells[name], name)
		return func(e *env) float64 { return b2f(priest.Best(e.known, ids) != 0) }, err
	},
	"dodged": func(_ names, _ string, s float64) (expr, error) {
		d := time.Duration(s * float64(time.Second))
		return func(e *env) float64 { return b2f(e.t != nil && e.s.melee.dodged(e.t.Guid, d)) }, nil
	},
	"aura_state": func(_ names, _ string, v float64) (expr, error) {
		return func(e *env) float64 { return b2f(auraState(e.me.AuraState, uint32(v))) }, nil
	},
	"target_aura_state": func(_ names, _ string, v float64) (expr, error) {
		return func(e *env) float64 {
			return withTarget(e, func(t *statepb.Obj) float64 { return b2f(auraState(t.AuraState, uint32(v))) })
		}, nil
	},
}

func need(ids []uint32, name string) ([]uint32, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("unknown spell or aura %q", name)
	}
	return ids, nil
}

// ---- parser ----------------------------------------------------------------

type token struct {
	kind byte // 'n' number, 'i' identifier, 's' string, 'o' operator or parenthesis, 0 end
	text string
	num  float64
}

func tokenize(src string) ([]token, error) {
	var out []token
	for i := 0; i < len(src); {
		c := rune(src[i])
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '"':
			j := strings.IndexByte(src[i+1:], '"')
			if j < 0 {
				return nil, fmt.Errorf("unterminated string")
			}
			out = append(out, token{kind: 's', text: src[i+1 : i+1+j]})
			i += j + 2
		case unicode.IsDigit(c) || c == '.':
			j := i
			for j < len(src) && (unicode.IsDigit(rune(src[j])) || src[j] == '.') {
				j++
			}
			v, err := strconv.ParseFloat(src[i:j], 64)
			if err != nil {
				return nil, err
			}
			out = append(out, token{kind: 'n', num: v, text: src[i:j]})
			i = j
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(src) && (unicode.IsLetter(rune(src[j])) || unicode.IsDigit(rune(src[j])) || src[j] == '_') {
				j++
			}
			out = append(out, token{kind: 'i', text: src[i:j]})
			i = j
		default:
			op := string(c)
			if i+1 < len(src) && src[i+1] == '=' && strings.ContainsRune("<>=!", c) {
				op += "="
			}
			if !operators[op] {
				return nil, fmt.Errorf("unexpected %q", op)
			}
			out = append(out, token{kind: 'o', text: op})
			i += len(op)
		}
	}
	return append(out, token{}), nil
}

var operators = map[string]bool{"(": true, ")": true, ",": true, "<": true, "<=": true, ">": true, ">=": true, "==": true, "!=": true}

type parser struct {
	toks []token
	pos  int
	n    names
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) word(w string) bool {
	if t := p.peek(); t.kind == 'i' && t.text == w {
		p.pos++
		return true
	}
	return false
}

func (p *parser) op(o string) bool {
	if t := p.peek(); t.kind == 'o' && t.text == o {
		p.pos++
		return true
	}
	return false
}

// compile turns a condition into a function; an empty condition always holds.
func compile(src string, n names) (expr, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	toks, err := tokenize(src)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", src, err)
	}
	p := &parser{toks: toks, n: n}
	e, err := p.or()
	if err == nil && p.peek().kind != 0 {
		err = fmt.Errorf("unexpected %q", p.peek().text)
	}
	if err != nil {
		return nil, fmt.Errorf("%q: %w", src, err)
	}
	return e, nil
}

func (p *parser) or() (expr, error) {
	l, err := p.and()
	for err == nil && p.word("or") {
		var r expr
		if r, err = p.and(); err == nil {
			a, b := l, r
			l = func(e *env) float64 { return b2f(a(e) != 0 || b(e) != 0) }
		}
	}
	return l, err
}

func (p *parser) and() (expr, error) {
	l, err := p.not()
	for err == nil && p.word("and") {
		var r expr
		if r, err = p.not(); err == nil {
			a, b := l, r
			l = func(e *env) float64 { return b2f(a(e) != 0 && b(e) != 0) }
		}
	}
	return l, err
}

func (p *parser) not() (expr, error) {
	if p.word("not") {
		x, err := p.not()
		if err != nil {
			return nil, err
		}
		return func(e *env) float64 { return b2f(x(e) == 0) }, nil
	}
	return p.compare()
}

func (p *parser) compare() (expr, error) {
	l, err := p.primary()
	if err != nil {
		return nil, err
	}
	t := p.peek()
	cmp := comparisons[t.text]
	if t.kind != 'o' || cmp == nil {
		return l, nil
	}
	p.next()
	r, err := p.primary()
	if err != nil {
		return nil, err
	}
	return func(e *env) float64 { return b2f(cmp(l(e), r(e))) }, nil
}

var comparisons = map[string]func(a, b float64) bool{
	"<": func(a, b float64) bool { return a < b }, "<=": func(a, b float64) bool { return a <= b },
	">": func(a, b float64) bool { return a > b }, ">=": func(a, b float64) bool { return a >= b },
	"==": func(a, b float64) bool { return a == b }, "!=": func(a, b float64) bool { return a != b },
}

func (p *parser) primary() (expr, error) {
	t := p.next()
	switch t.kind {
	case 'n':
		v := t.num
		return func(*env) float64 { return v }, nil
	case 'o':
		if t.text == "(" {
			x, err := p.or()
			if err != nil {
				return nil, err
			}
			if !p.op(")") {
				return nil, fmt.Errorf("missing )")
			}
			return x, nil
		}
	case 'i':
		if f, ok := functions[t.text]; ok {
			if !p.op("(") {
				return nil, fmt.Errorf("%s needs an argument", t.text)
			}
			a := p.next()
			if a.kind != 's' && a.kind != 'n' {
				return nil, fmt.Errorf("%s: bad argument %q", t.text, a.text)
			}
			if !p.op(")") {
				return nil, fmt.Errorf("%s: missing )", t.text)
			}
			return f(p.n, a.text, a.num)
		}
		if v, ok := variables[t.text]; ok {
			return v, nil
		}
		return nil, fmt.Errorf("unknown name %q", t.text)
	}
	return nil, fmt.Errorf("unexpected %q", t.text)
}

// ---- tables ----------------------------------------------------------------

type line struct {
	p     classdata.Priority
	cond  expr
	ranks []uint32
	self  bool
	spell uint32 // best known rank, set at each fight
	mark  time.Time
}

// rotation is a compiled table. Its marks (every) last across fights, like the cooldowns.
type rotation struct {
	lines []*line
	busy  time.Time
	// target_dies_in: first health sample of the current target
	target uint64
	t0     time.Time
	hp0    float64
}

// compileTable checks and compiles a table.
func compileTable(ps []classdata.Priority, n names) (*rotation, error) {
	r := &rotation{}
	for i, p := range ps {
		ranks := n.spells[p.Spell]
		if len(ranks) == 0 {
			return nil, fmt.Errorf("line %d: unknown spell %q", i+1, p.Spell)
		}
		if p.Target != "" && p.Target != "self" {
			return nil, fmt.Errorf("line %d: target %q (self or empty)", i+1, p.Target)
		}
		c, err := compile(p.If, n)
		if err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", i+1, p.Spell, err)
		}
		r.lines = append(r.lines, &line{p: p, cond: c, ranks: ranks, self: p.Target == "self"})
	}
	return r, nil
}

// classTables are the two tables of a class.
type classTables struct {
	fight, buffs *rotation
}

func classesDir() string {
	if d := os.Getenv("CLASSES_DIR"); d != "" {
		return d
	}
	return "/classes"
}

// tables returns the compiled tables of a class (read once); nil when the class has none.
func (s *Service) tables(class uint32) *classTables {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.rotations[class]; ok {
		return t
	}
	var t *classTables
	f, err := classdata.Load(classesDir(), class)
	if err == nil {
		t = &classTables{}
		n := classNames(class)
		if t.fight, err = compileTable(f.Priorities, n); err == nil {
			t.buffs, err = compileTable(f.Buffs, n)
		}
	}
	if err != nil {
		slog.Warn("no priority table: auto-attack only", "class", class, "err", err)
		t = nil
	}
	s.rotations[class] = t
	return t
}

// prepare picks the best known rank of each line for a new fight.
func (r *rotation) prepare(known map[uint32]bool) {
	for _, l := range r.lines {
		l.spell = priest.Best(known, l.ranks)
	}
}

// observe updates the damage rate behind target_dies_in.
func (r *rotation) observe(t *statepb.Obj) float64 {
	hp := float64(pct(t.Hp, t.MaxHp))
	if r.target != t.Guid || r.t0.IsZero() {
		r.target, r.t0, r.hp0 = t.Guid, time.Now(), hp
		return 99
	}
	el := time.Since(r.t0).Seconds()
	if el < 2 || hp >= r.hp0 {
		return 99
	}
	return hp / ((r.hp0 - hp) / el)
}

// step casts the first line that passes. It reports whether a spell was cast (the caller then
// waits for the next tick) and whether the server refused one aimed at the enemy.
func (s *Service) step(ctx context.Context, r *rotation, e *env, guid uint64) (cast, refused bool) {
	now := time.Now()
	for _, l := range r.lines {
		if l.spell == 0 || (!l.p.OffGCD && now.Before(r.busy)) || (l.p.Every > 0 && now.Sub(l.mark).Seconds() < l.p.Every) {
			continue
		}
		var t *statepb.Obj
		target := e.me.Guid
		if !l.self {
			if e.t == nil || e.noEnemy {
				continue
			}
			t, target = e.t, guid
		}
		if l.cond != nil && l.cond(e) == 0 {
			continue
		}
		if ok, _ := s.book.canCast(l.spell, e.me, t, e.dist); !ok {
			continue
		}
		l.mark = now
		if l.p.OffGCD {
			_, _ = s.cast(ctx, l.spell, target)
			e.logGroupCast(l.p.Spell)
			continue
		}
		r.busy = now.Add(time.Duration((max(l.p.Cast, 1.5) + 0.1) * float64(time.Second)))
		if s.tryCast(ctx, l.spell, target) {
			e.logGroupCast(l.p.Spell)
			if l.p.After > 0 {
				sleep(ctx, time.Duration(l.p.After*float64(time.Second)))
			}
			return true, false
		}
		r.busy = time.Now()
		if !l.p.Reactive {
			l.mark = time.Time{}
		}
		return false, !l.self && !l.p.Reactive
	}
	return false, false
}
