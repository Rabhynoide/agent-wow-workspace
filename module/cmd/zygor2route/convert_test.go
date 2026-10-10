package main

import "testing"

const sample = `ZygorGuidesViewer:RegisterGuide("Zygor's Alliance Leveling Guides\\Human (1-13)",[[
	author x
	faction alliance
	defaultfor Human
	next Zygor's Alliance Leveling Guides\\Main Guide (13-20)
	startlevel 1
	step //1
		goto Elwynn Forest,48.1,43.6
		.talk Deputy Willem##823
		..accept A Threat Within##783
	step //2
		only Human Priest
		goto 48.9,41.6
		.talk Marshal McBride##197
		..turnin A Threat Within##783
		..accept Hallowed Letter##3103
	step //3
		.from Diseased Young Wolf##299+
		.get 8 Diseased Wolf Pelt|q 33/1
		.kill 8 Kobold Vermin##6|q 7/1
		ding 2
	step //4
		'Fly to Sentinel Hill|goto Westfall,56.6,52.6|noway|c
		..accept Beached Sea Turtle##4725 //? or 4725,4727
		only Gnome,Human
]])
--TRIAL ZygorGuidesViewer:RegisterGuide("Trial",[[
	step //1
		..accept Ignored##1
]])
`

func TestConvert(t *testing.T) {
	gs, _ := convert(sample, "test.lua")
	if len(gs) != 1 {
		t.Fatalf("%d guides, want 1 (the commented trial must be skipped)", len(gs))
	}
	g := gs[0]
	if g.Faction != "Alliance" || g.Race != "Human" || g.Levels != "1-13" || g.Next != "Main Guide (13-20)" {
		t.Fatalf("header = %+v", g)
	}
	byN := map[int]step{}
	for _, s := range g.Steps {
		byN[s.N] = s
	}
	if s := byN[1]; s.Action != "accept" || s.QuestID != 783 || s.NPCID != 823 || s.Place != "Elwynn Forest" || s.Coords != "48.1,43.6" {
		t.Fatalf("step 1 = %+v", s)
	}
	// "only" before the actions applies to them.
	if s := byN[3]; s.Action != "accept" || s.QuestID != 3103 || s.Class != "Priest" || len(s.Only) != 1 {
		t.Fatalf("step 3 = %+v", s)
	}
	if s := byN[4]; s.Action != "collect" || s.Count != 8 || s.QuestID != 33 || len(s.TargetID) != 1 || s.TargetID[0] != 299 {
		t.Fatalf("step 4 = %+v", s)
	}
	if s := byN[5]; s.Action != "kill" || s.Count != 8 || s.QuestID != 7 || s.TargetID[0] != 6 {
		t.Fatalf("step 5 = %+v", s)
	}
	if s := byN[6]; s.Level != "2" {
		t.Fatalf("step 6 = %+v", s)
	}
	if s := byN[7]; s.Action != "travel" || s.Place != "Westfall" {
		t.Fatalf("step 7 = %+v", s)
	}
	// Trailing comment removed; "only" after the actions applies too.
	if s := byN[8]; s.QuestID != 4725 || s.Quest != "Beached Sea Turtle" || len(s.Only) != 2 {
		t.Fatalf("step 8 = %+v", s)
	}
}
