package combat

import (
	"log/slog"
	"sync"
	"time"

	"wowagent/internal/tuning"
)

// Adaptive pull size, the way a player levels (human session of 2026-10-14): take more monsters at
// once while the fights stay comfortable, fewer after a hard one. Engagements of 2026-10-13/14
// (levels 6-9, started at 90 %+ health): 12.9 s and 18 % health per kill alone, 9.8 s and 15 % with
// 2 monsters, 7.9 s and 16 % with 3 (Thunder Clap, rage from the hits, one approach for all).
//
// An engagement is a run of fights that follow each other within engageGap (the adds are fought
// after the target). It ends easy (won, lowest health at least easyMinHP, loss at most easyLoss) or
// hard (fled, died, or lowest health under hardMinHP); in between it changes nothing.

const (
	engageGap = 3 * time.Second
	easyMinHP = 40
	easyLoss  = 50
	hardMinHP = 20
)

type engagement struct {
	end     time.Time
	hpStart uint32
	hpMin   uint32
	most    int
	kills   int
	bad     string // "fled", "died": ended badly
}

type pullSizer struct {
	mu     sync.Mutex
	size   int // 0 until the first use: tuning PullSize
	easy   int // easy engagements of the full size in a row
	cur    *engagement
}

// cap is the most monsters a pull may bring at once; 0 when the adaptive pull size is off.
func (p *pullSizer) cap(t tuning.Tuning) int {
	if !t.PullAdapt {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sizeLocked(t)
}

func (p *pullSizer) sizeLocked(t tuning.Tuning) int {
	if p.size == 0 {
		p.size = max(1, t.PullSize)
	}
	return min(max(p.size, 1), max(t.PullSizeMax, 1))
}

// fightDone adds a fight (its fightRecord, result as logged) to the engagement running, and closes
// the previous one when this fight is a new engagement.
func (p *pullSizer) fightDone(t tuning.Tuning, r *fightRecord, result string) {
	if !t.PullAdapt {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	start := r.start
	if p.cur != nil && start.Sub(p.cur.end) > engageGap {
		p.closeLocked(t)
	}
	if p.cur == nil {
		p.cur = &engagement{hpStart: r.hpStart, hpMin: 100}
	}
	e := p.cur
	e.end = time.Now()
	e.hpMin = min(e.hpMin, r.hpMin)
	e.most = max(e.most, r.most)
	switch result {
	case "won":
		e.kills++
	case "died", "fled":
		e.bad = result
		p.closeLocked(t)
	}
}

// closeLocked judges the engagement that ended and moves the pull size.
func (p *pullSizer) closeLocked(t tuning.Tuning) {
	e := p.cur
	p.cur = nil
	if e == nil {
		return
	}
	size := p.sizeLocked(t)
	loss := int(e.hpStart) - int(e.hpMin)
	switch {
	case e.bad != "" || e.hpMin < hardMinHP:
		if e.most >= 2 || size > 1 {
			p.easy = 0
			if size > 1 {
				p.size = size - 1
				p.logLocked("down", e)
			}
		}
	case e.most >= size && e.hpMin >= easyMinHP && loss <= easyLoss:
		if p.easy++; p.easy >= max(t.PullUpAfter, 1) && size < t.PullSizeMax {
			p.size, p.easy = size+1, 0
			p.logLocked("up", e)
		}
	}
}

// gearUp counts a better item equipped as an easy engagement: the next fights should cost less.
func (p *pullSizer) gearUp(t tuning.Tuning) {
	if !t.PullAdapt {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	size := p.sizeLocked(t)
	if p.easy++; p.easy >= max(t.PullUpAfter, 1) && size < t.PullSizeMax {
		p.size, p.easy = size+1, 0
		p.logLocked("up after gear", nil)
	}
}

func (p *pullSizer) logLocked(why string, e *engagement) {
	args := []any{"size", p.size, "why", why}
	if e != nil {
		args = append(args, "attackers", e.most, "hp_start", e.hpStart, "hp_min", e.hpMin, "kills", e.kills, "bad", e.bad)
	}
	slog.Info("pull size", args...)
}
