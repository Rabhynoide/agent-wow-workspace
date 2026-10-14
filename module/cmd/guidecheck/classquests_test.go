package main

import "testing"

const warriorYaml = `class_quests:
  - { level: 10, unlocks: "Defensive Stance", quests: { Human: "A Warrior's Training (1638)", Dwarf_Gnome: "Muren Stormpike (1679)", NightElf: "Elanaria (1684)" } }
  - { level: 30, unlocks: "Berserker Stance", quests: { all: "The Islander (1718)" }, note: "x" }
`

func TestClassQuestStarts(t *testing.T) {
	if got := classQuestStarts(warriorYaml, "human"); got[10] != 1638 || got[30] != 1718 {
		t.Errorf("human %v", got)
	}
	if got := classQuestStarts(warriorYaml, "gnome"); got[10] != 1679 {
		t.Errorf("gnome %v", got)
	}
	if got := classQuestStarts(warriorYaml, "night elf"); got[10] != 1684 {
		t.Errorf("night elf %v", got)
	}
}

func TestInsertAt(t *testing.T) {
	cache := map[uint32]*quest{1: {MinLevel: 8}, 2: {MinLevel: 10}, 3: {MinLevel: 11}}
	got := insertAt([]routeQuest{{id: 1}, {id: 2}, {id: 3}}, cache, 10, []uint32{1638, 1639})
	var ids []uint32
	for _, e := range got {
		ids = append(ids, e.id)
	}
	if len(ids) != 5 || ids[1] != 1638 || ids[2] != 1639 || ids[3] != 2 {
		t.Errorf("route %v", ids)
	}
}
