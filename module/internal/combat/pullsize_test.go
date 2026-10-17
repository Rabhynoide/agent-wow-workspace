package combat

import (
	"testing"
	"time"

	"wowagent/internal/tuning"
)

func TestPullSizer(t *testing.T) {
	tu := tuning.Defaults()
	var p pullSizer
	if p.cap(tu) != 0 {
		t.Fatal("off by default")
	}
	tu.PullAdapt = true
	at := time.Now() // fightDone ends an engagement at time.Now(): later starts are new ones
	fight := func(most int, hpStart, hpMin uint32, result string) {
		at = at.Add(time.Minute) // a new engagement each time
		p.fightDone(tu, &fightRecord{start: at, most: most, hpStart: hpStart, hpMin: hpMin}, result)
	}
	if p.cap(tu) != 2 {
		t.Fatalf("start %d", p.cap(tu))
	}
	fight(2, 100, 60, "won")
	fight(2, 100, 55, "won")
	fight(1, 100, 90, "won") // closes the second easy one
	if p.cap(tu) != 3 {
		t.Fatalf("after two easy pairs %d", p.cap(tu))
	}
	fight(3, 100, 40, "fled")
	if p.cap(tu) != 2 {
		t.Fatalf("after a flight %d", p.cap(tu))
	}
	// adds of one engagement: fights within engageGap
	p.fightDone(tu, &fightRecord{start: at.Add(time.Hour), most: 2, hpStart: 100, hpMin: 70}, "won")
	p.fightDone(tu, &fightRecord{start: time.Now(), most: 1, hpStart: 70, hpMin: 10}, "won")
	at = at.Add(2 * time.Hour)
	fight(1, 100, 90, "won")
	if p.cap(tu) != 1 {
		t.Fatalf("after a hard engagement %d", p.cap(tu))
	}
}
