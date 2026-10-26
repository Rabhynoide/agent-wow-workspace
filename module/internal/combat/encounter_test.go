package combat

import (
	"testing"
	"time"

	statepb "wowagent/gen/statev1"
)

// One encounter across the fights of a pack: decisions and monsters add up until it ends; a stale one
// is closed by the next fight.
func TestEncounter(t *testing.T) {
	s := &Service{}
	me := &statepb.SelfInfo{Hp: 80, MaxHp: 100}
	s.encounterBegin(me)
	first := s.enc
	s.encounterDecision(&fleeDecision{key: "3:10", stayed: true})
	s.encounterSaw([]*statepb.Obj{{Guid: 1}, {Guid: 2}, {Guid: 3}}, 60)
	s.encounterBegin(me) // the next kobold of the pack: same encounter
	s.encounterSaw([]*statepb.Obj{{Guid: 2}, {Guid: 3}}, 40)
	if s.enc != first || len(s.enc.foes) != 3 || len(s.enc.decisions) != 1 || s.enc.hpMin != 40 {
		t.Fatalf("encounter %+v", s.enc)
	}
	s.enc.lastFoe = time.Now().Add(-time.Minute)
	s.encounterBegin(me)
	if s.enc == first || len(s.enc.decisions) != 0 {
		t.Fatal("a stale encounter must end at the next fight")
	}
	s.encounterEnd("won")
	if s.enc != nil {
		t.Fatal("ended")
	}
}
