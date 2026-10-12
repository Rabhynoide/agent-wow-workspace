package combat

import (
	"testing"
	"time"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/classdata"
	"wowagent/internal/priest"
	"wowagent/internal/warrior"
)

func TestCondition(t *testing.T) {
	n := classNames(warrior.ClassID)
	me := &statepb.SelfInfo{Hp: 30, MaxHp: 100, Power: 250, PowerType: 1, Level: 6,
		Auras: []*statepb.Aura{{Spell: 6673, LeftMs: 2500, Mine: true}, {Spell: 52437}}}
	wolf := &statepb.Obj{Hp: 15, MaxHp: 100, AuraState: 1 << 1}
	e := &env{me: me, t: wolf, diesIn: 99}
	for src, want := range map[string]float64{
		"":                             1,
		"hp < 40 and rage >= 25":       1,
		"hp < 40 and rage > 25":        0,
		"not (hp > 50) or level == 1":  1,
		`my_aura_left("Battle Shout")`: 2.5,
		`my_aura_left("Battle Shout") < 3 and target_hp < 20`: 1,
		`aura("Sudden Death")`:                                1,
		`target_aura_left("Rend") < 2`:                        1,
		"target_aura_state(2)":                                1,
		"target_aura_state(1)":                                0,
		"target_dies_in > 8 and dist <= 24":                   1,
	} {
		c, err := compile(src, n)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		got := 1.0
		if c != nil {
			got = c(e)
		}
		if got != want {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
	for _, bad := range []string{"hp <", `aura("Nope")`, "foo > 1", "hp = 3", "(hp > 1", `aura(hp)`} {
		if _, err := compile(bad, n); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}

func TestDiesIn(t *testing.T) {
	r := &rotation{}
	wolf := &statepb.Obj{Guid: 7, Hp: 100, MaxHp: 100}
	if d := r.observe(wolf); d != 99 {
		t.Fatalf("first sample %v", d)
	}
	r.t0 = time.Now().Add(-4 * time.Second)
	wolf.Hp = 60 // 10 % per second
	if d := r.observe(wolf); d < 5.5 || d > 6.5 {
		t.Fatalf("dies in %v, want 6", d)
	}
}

// The class files of the workspace compile: every spell, aura and condition is known.
func TestClassTables(t *testing.T) {
	for _, class := range []uint32{warrior.ClassID, priest.ClassID} {
		f, err := classdata.Load("../../../journal/classes", class)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.Priorities) == 0 || len(f.Buffs) == 0 {
			t.Fatalf("class %d: no priorities or buffs", class)
		}
		for _, ps := range [][]classdata.Priority{f.Priorities, f.Buffs} {
			if _, err := compileTable(ps, classNames(class)); err != nil {
				t.Errorf("class %d: %v", class, err)
			}
		}
	}
}

// step: the first castable line wins; every and off_gcd lines.
func TestStep(t *testing.T) {
	s := &Service{book: &spellBook{ready: map[uint32]time.Time{}}, failures: map[uint8]castFailure{}}
	s.book.once.Do(func() {})
	r, err := compileTable([]classdata.Priority{
		{Spell: "Execute", If: "target_hp < 20"},
		{Spell: "Rend", Every: 2},
	}, classNames(warrior.ClassID))
	if err != nil {
		t.Fatal(err)
	}
	r.prepare(map[uint32]bool{772: true, 5308: true})
	if r.lines[0].spell != 5308 || r.lines[1].spell != 772 {
		t.Fatalf("ranks %d %d", r.lines[0].spell, r.lines[1].spell)
	}
	me := &statepb.SelfInfo{Power: 300, PowerType: 1}
	e := &env{me: me, t: &statepb.Obj{Hp: 50, MaxHp: 100}}
	// Rend's mark set by a previous try: nothing to cast yet.
	r.lines[1].mark = time.Now()
	if cast, refused := s.step(t.Context(), r, e, 1); cast || refused {
		t.Fatal("cast during every")
	}
}
