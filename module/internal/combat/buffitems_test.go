package combat

import (
	"testing"

	statepb "wowagent/gen/statev1"
	"wowagent/internal/dbc"
)

func TestBuffItemHelps(t *testing.T) {
	warrior := &statepb.SelfInfo{ClassId: 1, PowerType: 1}
	priest := &statepb.SelfInfo{ClassId: 5, PowerType: 0}
	strength := dbc.SpellInfo{Auras: [3]uint32{auraModStat}, Misc: [3]int32{statStrength}}
	intellect := dbc.SpellInfo{Auras: [3]uint32{auraModStat}, Misc: [3]int32{statIntellect}}
	armor := dbc.SpellInfo{Auras: [3]uint32{auraModResistance}, Misc: [3]int32{resistanceArmorMask}}
	fire := dbc.SpellInfo{Auras: [3]uint32{auraModResistance}, Misc: [3]int32{4}}
	water := dbc.SpellInfo{Auras: [3]uint32{82}} // water breathing
	for _, c := range []struct {
		name string
		sp   dbc.SpellInfo
		me   *statepb.SelfInfo
		want bool
	}{
		{"strength/warrior", strength, warrior, true}, {"strength/priest", strength, priest, false},
		{"intellect/warrior", intellect, warrior, false}, {"intellect/priest", intellect, priest, true},
		{"armor", armor, warrior, true}, {"fire resistance", fire, warrior, false}, {"water breathing", water, priest, false},
	} {
		if got := helps(c.sp, c.me); got != c.want {
			t.Errorf("%s: helps = %v", c.name, got)
		}
	}
}
