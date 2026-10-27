package auto

import (
	"fmt"
	"slices"
	"strings"

	pb "wowagent/gen/autov1"
	worldpb "wowagent/gen/worldv1"
)

// Quests by hub (tuning quest_hub), the way speedrunners level: take every quest of the batch, work
// on their objectives one after the other, then turn them all in, instead of a walk to the giver, the
// objectives and a walk back for each quest (three trips to the Fargodeep mine for 47, 60 and 62).
// Monsters killed for one quest also count for the others in the log (kills, and quest items that
// drop only while the quest is taken). Rounds: a quest whose giver only offers it once another one is
// turned in (a chain inside the batch) is accepted in the next round. One quest failing no longer
// stops the batch: its failure is reported in AutoStatus.failed and the others go on.

const (
	hubTodo = iota
	hubTaken
	hubComplete
	hubDone
	hubFailed
	hubHeld // complete, turned in later (RunRequest.hold)
)

type hubQuest struct {
	id        uint32
	info      *worldpb.QuestInfo
	state     int
	acceptErr error // the last refusal to accept (a chain not open yet, a level too low)
	deaths    int
}

func (r *run) hubQuests(req *pb.RunRequest) error {
	var qs []*hubQuest
	var first *pb.QuestFailure
	fail := func(q *hubQuest, err error) {
		q.state = hubFailed
		f := &pb.QuestFailure{Quest: q.id, Reason: err.Error()}
		if first == nil {
			first = f
		}
		r.note("quest %d failed: %v", q.id, err)
		r.j.Update(func(st *pb.AutoStatus) { st.Failed = append(st.Failed, f) })
	}
	for _, id := range req.Quests {
		q := &hubQuest{id: id}
		info, err := r.s.api.QuestInfo(r.ctx, id)
		qs = append(qs, q)
		if err != nil {
			fail(q, err)
			continue
		}
		q.info = info
	}
	// retry tells whether a failed step should be done again: after a death (3 at most per quest,
	// as before), once the character is back up.
	retry := func(q *hubQuest, err error) (bool, error) {
		if r.ctx.Err() != nil {
			return false, r.ctx.Err()
		}
		b, e := r.s.api.Brief(r.ctx)
		if e != nil || b.Self == nil || !b.Self.Dead {
			return false, nil
		}
		if q.deaths++; q.deaths >= 3 {
			return false, nil
		}
		if e := r.recover(); e != nil {
			return false, fmt.Errorf("recover: %w", e)
		}
		return true, nil
	}
	for round := 0; round < len(qs)+1; round++ {
		moved := false
		// 1. Accept, nearest giver first.
		tried := map[uint32]bool{}
		for {
			if err := r.recover(); err != nil {
				return fmt.Errorf("recover: %w", err)
			}
			q := r.nearestHub(qs, hubTodo, tried, true)
			if q == nil {
				break
			}
			tried[q.id] = true
			r.j.Update(func(st *pb.AutoStatus) { st.Quest = q.id })
			if err := r.acceptQuest(q.id, q.info); err != nil {
				again, rerr := retry(q, err)
				switch {
				case rerr != nil:
					return rerr
				case again:
					delete(tried, q.id)
				case strings.HasPrefix(err.Error(), "accept failed"):
					q.acceptErr = err // the giver refused: maybe a chain opened by a turn-in to come
				default:
					fail(q, err) // the walk to the giver failed (danger, flight), as one by one
				}
				continue
			}
			q.state, q.acceptErr, moved = hubTaken, nil, true
		}
		// 2. Objectives, in the order of the batch (the route's order).
		for _, q := range qs {
			for q.state == hubTaken {
				if err := r.recover(); err != nil {
					return fmt.Errorf("recover: %w", err)
				}
				r.j.Update(func(st *pb.AutoStatus) { st.Quest = q.id })
				err := r.completeQuest(q.id, q.info)
				if err == nil {
					q.state, moved = hubComplete, true
					break
				}
				again, rerr := retry(q, err)
				if rerr != nil {
					return rerr
				}
				if !again {
					fail(q, err)
				}
			}
		}
		// 3. Turn in, nearest ender first, except the quests held for later (RunRequest.hold).
		for _, q := range qs {
			if q.state == hubComplete && slices.Contains(req.Hold, q.id) {
				q.state = hubHeld
				r.note("quest %d done, kept for the guide's turn-in later", q.id)
			}
		}
		for {
			if err := r.recover(); err != nil {
				return fmt.Errorf("recover: %w", err)
			}
			q := r.nearestHub(qs, hubComplete, nil, false)
			if q == nil {
				break
			}
			r.j.Update(func(st *pb.AutoStatus) { st.Quest = q.id })
			reward, chosen := req.Rewards[q.id]
			if err := r.turnInQuest(q.id, q.info, reward, chosen); err != nil {
				if again, rerr := retry(q, err); rerr != nil {
					return rerr
				} else if !again {
					fail(q, err)
				}
				continue
			}
			q.state, moved = hubDone, true
			r.j.Update(func(st *pb.AutoStatus) { st.Done = append(st.Done, q.id) })
			if !req.NoProgress {
				r.progress()
			}
		}
		if !moved {
			break
		}
	}
	// A quest still refused while another one of the batch failed is most likely its follow-up (61
	// after 47): left for a later batch, neither done nor failed (reported, it was skipped for 6 h).
	// Refused although all the others went through: a real failure.
	othersFailed := first != nil
	for _, q := range qs {
		switch {
		case q.state != hubTodo:
		case othersFailed:
			r.note("quest %d left for later: %v", q.id, q.acceptErr)
		case q.acceptErr != nil:
			fail(q, q.acceptErr)
		default:
			fail(q, fmt.Errorf("not accepted"))
		}
	}
	if first != nil {
		r.j.Update(func(st *pb.AutoStatus) { st.Quest = first.Quest })
		return fmt.Errorf("quest %d: %s", first.Quest, first.Reason)
	}
	return nil
}

// nearestHub returns the quest in this state whose giver (or ender) is the nearest on our map, skipping
// the ones in skip; quests without one on this map come last, in batch order.
func (r *run) nearestHub(qs []*hubQuest, state int, skip map[uint32]bool, giver bool) *hubQuest {
	snap, err := r.snapshot()
	if err != nil {
		return nil
	}
	me := snap.Self
	var best, other *hubQuest
	var bd float32
	for _, q := range qs {
		if q.state != state || skip[q.id] {
			continue
		}
		list := q.info.Enders
		if giver {
			list = q.info.Starters
		}
		n := nearestNpc(list, me.Map, me.X, me.Y)
		if n == nil {
			if other == nil {
				other = q
			}
			continue
		}
		if d := dist2(me.X, me.Y, n.X, n.Y); best == nil || d < bd {
			best, bd = q, d
		}
	}
	if best == nil {
		return other
	}
	return best
}
