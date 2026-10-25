package auto

import (
	"fmt"
	"log/slog"
	"time"

	travelpb "wowagent/gen/travelv1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/jobs"
)

// Journeys (travel module): a walk farther than travelMin, or to another map, asks travel for the
// fastest way (flights, area-trigger portals such as the Deeprun Tram, hearthstone, walks that do not
// cross areas far above our level). At level 10 the route went to Loch Modan and the character walked
// there through the Burning Steppes and died (bench 8 -> 12 of 2026-10-24).
const travelMin = 500

const unitTaxiFlight = 0x00100000

// npcOn returns the nearest NPC of the list on our map, else the nearest elsewhere (reached by travel).
func npcOn(list []*worldpb.QuestNpc, mapID uint32, x, y float32) *worldpb.QuestNpc {
	if n := nearestNpc(list, mapID, x, y); n != nil {
		return n
	}
	for _, n := range list {
		return n
	}
	return nil
}

// goTo travels to a point of any map when it is far or on another map; the caller walks the end.
func (r *run) goTo(mapID uint32, x, y, z float32) error {
	if r.travelling {
		return nil
	}
	b, err := r.s.api.Brief(r.ctx)
	if err != nil || b.Self == nil {
		return err
	}
	me := b.Self
	if me.Dead || me.Ghost {
		return nil // corpse runs walk (a ghost cannot fly; r.snapshot fails when dead: recover broke)
	}
	if me.Map == mapID && dist2(me.X, me.Y, x, y) < travelMin { // dist2: plain distance
		return nil
	}
	// Planning checks walks for a while and may need several calls (busy). No safe way: no walk
	// straight there either (it went through the Searing Gorge and died 3 times, 2026-10-25).
	var plan *travelpb.PlanResponse
	for try := 0; ; try++ {
		plan, err = r.s.api.TravelPlan(r.ctx, mapID, x, y, z)
		if err == nil && !plan.Busy {
			break
		}
		if try >= 8 {
			return fmt.Errorf("too dangerous for now: no journey planned to (%.0f,%.0f) on map %d: %v %s", x, y, mapID, err, plan.GetReason())
		}
		r.step("planning the journey to (%.0f,%.0f) map %d", x, y, mapID)
	}
	if !plan.Ok {
		return fmt.Errorf("too dangerous for now: no safe way to (%.0f,%.0f) on map %d: %s", x, y, mapID, plan.Reason)
	}
	legs := plan.Legs
	if n := len(legs); n > 0 && legs[n-1].Kind == "walk" && legs[n-1].To.Map == mapID {
		legs = legs[:n-1] // the caller walks the end
	}
	if len(legs) == 0 || (len(legs) == 1 && legs[0].Kind == "walk") {
		return nil
	}
	r.travelling = true
	defer func() { r.travelling = false }()
	desc := ""
	for _, l := range legs {
		if l.Kind != "walk" {
			desc += fmt.Sprintf(" %s %s;", l.Kind, l.Name)
		}
	}
	r.step("travel to (%.0f,%.0f) map %d in %.0f s (walking: %.0f s):%s", x, y, mapID, plan.Seconds, plan.WalkSeconds, desc)
	slog.Info("travel", "to", fmt.Sprintf("%d:(%.0f,%.0f)", mapID, x, y), "s", int(plan.Seconds), "walk_s", int(plan.WalkSeconds), "legs", desc)
	for _, l := range legs {
		switch l.Kind {
		case "walk":
			if err := r.walk(l.To.X, l.To.Y, l.To.Z, 1.5); err != nil {
				return err
			}
		case "fly":
			if err := r.approachNpc(l.NpcEntry, l.From.X, l.From.Y, l.From.Z); err != nil {
				return err
			}
			res, err := r.s.api.TravelDo(r.ctx, "fly", l.Nodes, 0, 0)
			if err != nil || !res.Ok {
				return fmt.Errorf("flight %s: %v %s", l.Name, err, res.GetReason())
			}
			r.step("flying %s (%.0f s)", l.Name, res.Seconds)
			if err := r.waitLanding(res.Seconds); err != nil {
				return err
			}
		case "portal":
			res, err := r.s.api.TravelDo(r.ctx, "portal", nil, l.Trigger, l.To.Map)
			if err != nil || !res.Ok {
				return fmt.Errorf("portal %s: %v %s", l.Name, err, res.GetReason())
			}
		case "boat":
			if err := r.rideBoat(l.Transport, l.Name, l.Dock, l.LandDock, l.To); err != nil {
				return err
			}
		case "hearth":
			r.restBelow(walkRestBelow)
			res, err := r.s.api.TravelDo(r.ctx, "hearth", nil, 0, 0)
			if err != nil || !res.Ok {
				return fmt.Errorf("hearthstone: %v %s", err, res.GetReason())
			}
		}
	}
	return nil
}

// waitLanding waits for the end of a flight (taxi flag off).
func (r *run) waitLanding(secs float32) error {
	deadline := time.Now().Add(time.Duration(secs*1.5+90) * time.Second)
	for time.Now().Before(deadline) {
		if !jobs.Sleep(r.ctx, 2*time.Second) {
			return r.ctx.Err()
		}
		b, err := r.s.api.Brief(r.ctx)
		if err == nil && b.Self != nil && b.Self.UnitFlags&unitTaxiFlight == 0 {
			jobs.Sleep(r.ctx, time.Second)
			return nil
		}
	}
	return fmt.Errorf("still flying after %.0f s", secs*1.5+90)
}

// flightOnTheWay: a flight master of an unknown node within 40 yards (checked every 5 s on walks).
func (r *run) flightOnTheWay() (uint32, *worldpb.QuestNpc) {
	if r.detour || time.Since(r.lastFlightCheck) < 5*time.Second {
		return 0, nil
	}
	r.lastFlightCheck = time.Now()
	n, err := r.s.api.NearUnknownFlight(r.ctx, 40)
	if err != nil || !n.Found || r.triedNodes[n.Node] {
		return 0, nil
	}
	return n.Node, &worldpb.QuestNpc{Entry: n.NpcEntry, Name: n.Name, Map: n.At.Map, X: n.At.X, Y: n.At.Y, Z: n.At.Z}
}

// learnFlight talks to a flight master to learn its node.
func (r *run) learnFlight(node uint32, npc *worldpb.QuestNpc) {
	if r.triedNodes == nil {
		r.triedNodes = map[uint32]bool{}
	}
	r.triedNodes[node] = true
	r.detour = true
	defer func() { r.detour = false }()
	r.step("learning the flight path %s", npc.Name)
	if err := r.approachNpc(npc.Entry, npc.X, npc.Y, npc.Z); err != nil {
		slog.Info("flight path not reached", "node", npc.Name, "err", err)
		return
	}
	res, err := r.s.api.TravelDo(r.ctx, "discover", nil, 0, 0)
	slog.Info("flight path", "node", npc.Name, "ok", res.GetOk(), "reason", res.GetReason(), "err", err)
}

// rideBoat waits on the pier for the boat (fighting back if attacked), boards it, and steps off at
// the other dock.
func (r *run) rideBoat(entry uint32, name string, dock, land, pier *travelpb.Point) error {
	deadline := time.Now().Add(10 * time.Minute)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("boat %s: not boarded in 10 minutes", name)
		}
		if r.inCombat() {
			if _, err := r.defend(); err != nil {
				return err
			}
		}
		st, err := r.s.api.Boat(r.ctx, "board", entry, dock, nil, 20)
		if err != nil {
			return fmt.Errorf("boat %s: %v", name, err)
		}
		if st.State == "aboard" {
			break
		}
		if st.State != "waiting" {
			return fmt.Errorf("boat %s: %s", name, st.Reason)
		}
		r.step("waiting for the boat %s (%d s)", name, st.WaitS)
	}
	r.step("on the boat %s", name)
	for deadline = time.Now().Add(10 * time.Minute); time.Now().Before(deadline); {
		st, err := r.s.api.Boat(r.ctx, "disembark", entry, land, pier, 20)
		if err != nil {
			return fmt.Errorf("boat %s: %v", name, err)
		}
		switch st.State {
		case "ashore":
			return nil
		case "riding":
			continue
		default:
			return fmt.Errorf("boat %s: %s", name, st.Reason)
		}
	}
	return fmt.Errorf("boat %s: still aboard after 10 minutes", name)
}
