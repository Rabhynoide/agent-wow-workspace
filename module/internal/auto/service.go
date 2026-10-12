// Package auto chains the other modules to complete quests unattended.
package auto

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/autov1"
	combatpb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/api"
	"wowagent/internal/jobs"
	"wowagent/internal/wire"
)

const (
	// Death.SicknessLevel of the server (worldserver.conf): from this level the spirit healer
	// applies resurrection sickness (level-10 minutes up to 10 min of reduced stats).
	sicknessLevel = 11
	maxWait       = 25
	// Do not walk further than this (yards) to a class trainer between quests: the walk does not
	// avoid monsters yet.
	maxTrainerDist = 400
	// Monsters more than this many levels above the character, or elites, are not hunted: classes that
	// heal or shield themselves (priest, mage, warlock) get 2 levels, melee classes without a heal 1
	// (a level-5 warrior died twice on level 7-8 boars).
	dangerLevelsCaster = 2
	dangerLevelsMelee  = 1
	// Repair when an equipped item falls below this durability (percent).
	repairBelow = 60
)

type job = jobs.Job[*pb.AutoStatus]

type Service struct {
	pb.UnimplementedAutoServer
	sess   modv1.SessionClient
	corpse chan [4]float32
	api    api.C
	jobs   *jobs.Runner[*pb.AutoStatus]
	guard  atomic.Bool // defend when attacked between jobs
	items  sync.Map    // item entry -> *worldpb.ItemInfo (food checks)
	// money when a repair failed (not enough): no new repair trip before we earn a silver more
	repairFailedAt atomic.Uint32
	// money when a food trip bought nothing (too poor): no new trip before we earn a silver more
	foodFailedAt atomic.Uint32
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	runner := jobs.NewRunner(&pb.AutoStatus{State: "idle"})
	runner.Decorate = func(j *job, st *pb.AutoStatus) { st.ElapsedS = uint32(time.Since(j.Start).Seconds()) }
	svc := &Service{sess: s, corpse: make(chan [4]float32, 4), api: api.C{S: s}, jobs: runner}
	pb.RegisterAutoServer(g, svc)
	go svc.guardLoop()
}

// guardLoop defends the character when it is attacked while no auto job runs (a job that failed or
// finished leaves it standing among monsters: Claudio died that way). It is armed when an auto job
// ends on its own and disarmed by auto.stop and logout, so a stop by the user stays a stop.
func (s *Service) guardLoop() {
	const inCombat = 0x00080000 // UNIT_FLAG_IN_COMBAT
	ctx := context.Background()
	for {
		time.Sleep(2 * time.Second)
		if !s.guard.Load() || s.jobs.Cur() != nil {
			continue
		}
		b, err := s.api.Brief(ctx)
		if err != nil || b.Self == nil || b.Self.Dead || b.Self.UnitFlags&inCombat == 0 {
			continue
		}
		if st, err := s.api.CombatStatus(ctx, 0); err != nil || st.State == "running" {
			continue
		}
		slog.Info("attacked while idle: defending")
		_, _ = s.api.CombatDefend(ctx, 1)
	}
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	if p.GetOpcode() == uint32(opcode.MSGCorpseQuery) {
		r := wire.NewReader(p.GetPayload())
		if r.U8() == 1 {
			m, x, y, z := r.U32(), r.F32(), r.F32(), r.F32()
			if !r.Err {
				select {
				case s.corpse <- [4]float32{float32(m), x, y, z}:
				default:
				}
			}
		}
	}
	return &emptypb.Empty{}, nil
}

func (s *Service) Goto(ctx context.Context, r *pb.GotoRequest) (*pb.AutoStatus, error) {
	radius := r.Radius
	if radius <= 0 {
		radius = 3
	}
	return s.start(ctx, r.WaitS, &pb.AutoStatus{State: "running", Step: "walking"}, func(run *run) error {
		return run.walk(r.X, r.Y, r.Z, radius)
	}), nil
}

func (s *Service) Recover(ctx context.Context, r *pb.StatusRequest) (*pb.AutoStatus, error) {
	return s.start(ctx, r.WaitS, &pb.AutoStatus{State: "running", Step: "recovering corpse"}, func(r *run) error { return r.recover() }), nil
}

// start runs body as the current job and maps its error to the final state.
func (s *Service) start(ctx context.Context, waitS uint32, initial *pb.AutoStatus, body func(r *run) error) *pb.AutoStatus {
	s.guard.Store(false)
	s.jobs.Start(initial, func(j *job) {
		err := body(&run{s: s, j: j, ctx: j.Ctx})
		if j.Ctx.Err() == nil {
			s.guard.Store(true) // ended on its own: watch over the character until the next job
		}
		j.Update(func(st *pb.AutoStatus) {
			switch {
			case err == nil:
				st.State = "done"
			case j.Ctx.Err() != nil:
				st.State, st.Reason = "stopped", "cancelled"
			default:
				st.State, st.Reason = "failed", err.Error()
			}
			st.ElapsedS = uint32(time.Since(j.Start).Seconds())
		})
	})
	return s.jobs.Wait(ctx, waitS, 20, maxWait)
}

// recover releases the spirit if needed, runs to the corpse and resurrects.
func (r *run) recover() error {
	snap, err := r.s.api.Brief(r.ctx)
	if err != nil || snap.Self == nil {
		return fmt.Errorf("state unavailable")
	}
	if !snap.Self.Dead {
		return nil
	}
	send := func(op uint32, payload []byte) {
		_, _ = r.s.sess.SendPacket(r.ctx, &modv1.SendPacketRequest{Opcode: op, Payload: payload})
	}
	if !snap.Self.Ghost {
		r.step("releasing spirit")
		send(uint32(opcode.CMSGRepopRequest), []byte{0})
		if !jobs.Sleep(r.ctx, 3*time.Second) {
			return r.ctx.Err()
		}
	}
	// Back at the corpse (safe spot near it): only the death's 10 % durability. The spirit healer adds
	// 25 % more (gear broke in the 2026-10-11 benches) and resurrection sickness from sicknessLevel:
	// it is only the fallback when the corpse cannot be reclaimed, below that level.
	err = r.corpseRez(send)
	if err == nil || r.ctx.Err() != nil {
		return err
	}
	if snap.Self.Level < sicknessLevel {
		r.note("corpse not reclaimed (%v): spirit healer", err)
		if ok := r.healerRez(send); ok {
			r.restAfterRez()
			return nil
		}
	}
	return err
}

// corpseRez runs to the corpse, waits at the safest spot within the reclaim radius, and reclaims it.
func (r *run) corpseRez(send func(uint32, []byte)) error {
	for len(r.s.corpse) > 0 {
		<-r.s.corpse
	}
	send(uint32(opcode.MSGCorpseQuery), nil)
	var pos [4]float32
	select {
	case pos = <-r.s.corpse:
	case <-time.After(4 * time.Second):
		return fmt.Errorf("corpse position unknown")
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
	r.step("running to corpse (%.0f,%.0f)", pos[1], pos[2])
	if err := r.walk(pos[1], pos[2], pos[3], 30); err != nil {
		return err
	}
	// The corpse often lies in the camp that killed us: come back to life at the edge of the
	// reclaim radius (39 yards), as far as possible from the monsters (mod-playerbots does the same).
	for wait := 0; ; wait++ {
		sp, err := r.s.api.SafePoint(r.ctx, pos[1], pos[2], pos[3], 33, false)
		if err != nil || !sp.Ok {
			r.note("no safe point near the corpse: %v %s", err, sp.GetReason())
			if err := r.walk(pos[1], pos[2], pos[3], 15); err != nil {
				return err
			}
			break
		}
		if sp.Clearance < 25 && wait < 4 {
			r.step("monsters near the corpse (nearest %.0f yards): waiting for them to move", sp.Clearance)
			if !jobs.Sleep(r.ctx, 15*time.Second) {
				return r.ctx.Err()
			}
			continue
		}
		r.step("going to a safe spot near the corpse (%.0f yards from the nearest monster)", sp.Clearance)
		if err := r.walk(sp.X, sp.Y, sp.Z, 1.5); err != nil {
			return err
		}
		break
	}
	// The reclaim delay grows with repeated deaths (up to 2 minutes): keep trying for 4 minutes.
	for i := 0; i < 80; i++ {
		w := &wire.Writer{}
		w.U64(0)
		send(uint32(opcode.CMSGReclaimCorpse), w.B)
		if !jobs.Sleep(r.ctx, 3*time.Second) {
			return r.ctx.Err()
		}
		if s, err := r.s.api.Brief(r.ctx); err == nil && s.Self != nil && !s.Self.Dead {
			r.restAfterRez()
			return nil
		}
	}
	return fmt.Errorf("could not resurrect at corpse")
}

// restAfterRez: back with half health, rest (and eat) before anything else; leaving at once got the
// character killed again on the way (bench of 2026-10-11: four deaths in a row near Goldshire).
func (r *run) restAfterRez() {
	r.step("resting after resurrection")
	st, err := r.s.api.CombatRest(r.ctx)
	for err == nil && st.State == "running" {
		st, err = r.s.api.CombatStatus(r.ctx, 20)
	}
}

// healerRez walks to a spirit healer within 150 yards and resurrects there.
func (r *run) healerRez(send func(uint32, []byte)) bool {
	const spiritHealerFlag = 0x4000
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", NpcFlags: spiritHealerFlag, Radius: 150, Max: 1})
	if err != nil || len(vis.Objs) == 0 {
		return false
	}
	h := vis.Objs[0]
	r.step("going to the spirit healer (%.0f yards)", h.Dist)
	if h.Dist > 6 {
		if err := r.walk(h.X, h.Y, h.Z, 3); err != nil {
			return false
		}
	}
	for i := 0; i < 5; i++ {
		w := &wire.Writer{}
		w.U64(h.Guid)
		send(uint32(opcode.CMSGSpiritHealerActivate), w.B)
		if !jobs.Sleep(r.ctx, 2*time.Second) {
			return false
		}
		if b, err := r.s.api.Brief(r.ctx); err == nil && b.Self != nil && !b.Self.Dead {
			return true
		}
	}
	return false
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	s.guard.Store(false)
	s.jobs.Stop()
	return &emptypb.Empty{}, nil
}

func (s *Service) Stop(context.Context, *emptypb.Empty) (*pb.AutoStatus, error) {
	s.guard.Store(false)
	s.jobs.Stop()
	return s.jobs.Current(), nil
}

func (s *Service) Status(ctx context.Context, r *pb.StatusRequest) (*pb.AutoStatus, error) {
	return s.jobs.Wait(ctx, r.WaitS, 0, maxWait), nil
}

func (s *Service) Run(ctx context.Context, r *pb.RunRequest) (*pb.AutoStatus, error) {
	return s.start(ctx, r.WaitS, &pb.AutoStatus{State: "running"}, func(run *run) error { return run.quests(r) }), nil
}

// Grind hunts the given creatures in turn until the level is reached, with everything auto.run does
// around a fight: vendor visits (sell, repair), resurrection, trainer, talents, gear.
func (s *Service) Grind(ctx context.Context, r *pb.GrindRequest) (*pb.AutoStatus, error) {
	return s.start(ctx, r.WaitS, &pb.AutoStatus{State: "running"}, func(run *run) error { return run.grind(r) }), nil
}

func (r *run) grind(req *pb.GrindRequest) error {
	if len(req.Entries) == 0 {
		return fmt.Errorf("no creature entries to hunt")
	}
	limit := time.Duration(req.MaxS) * time.Second
	if limit == 0 {
		limit = 1500 * time.Second
	}
	deadline := time.Now().Add(limit)
	if err := r.recover(); err != nil {
		return fmt.Errorf("recover: %w", err)
	}
	r.progress()
	deaths, idle, failures, lastLevel := 0, 0, 0, uint32(0)
	for round := 0; time.Now().Before(deadline); round++ {
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		b, err := r.s.api.Brief(r.ctx)
		if err != nil || b.Self == nil {
			return fmt.Errorf("state unavailable")
		}
		if b.Self.Dead {
			if deaths++; deaths > 3 {
				return fmt.Errorf("stopping: died %d times", deaths)
			}
			if err := r.recover(); err != nil {
				return fmt.Errorf("recover: %w", err)
			}
			continue
		}
		if req.UntilLevel > 0 && b.Self.Level >= req.UntilLevel {
			return nil
		}
		if lastLevel != 0 && b.Self.Level > lastLevel {
			r.progress() // new level: gear, talents, trainer
		}
		lastLevel = b.Self.Level
		r.j.Update(func(st *pb.AutoStatus) { st.Level, st.Xp = b.Self.Level, b.Self.Xp })
		entry := req.Entries[round%len(req.Entries)]
		err = r.hunt(entry, fmt.Sprintf("creature %d", entry), 4, 0, 0)
		switch {
		case err == nil:
			idle = 0
		case strings.Contains(err.Error(), "dead"):
			idle = 0 // handled at the top of the loop
		case strings.Contains(err.Error(), "no live entry"), strings.Contains(err.Error(), "too dangerous"), strings.Contains(err.Error(), "no spawn found"):
			if idle++; idle >= len(req.Entries) {
				return fmt.Errorf("nothing to hunt right now (%v)", err)
			}
		case strings.HasPrefix(err.Error(), "combat:") || strings.HasPrefix(err.Error(), "walk to"):
			// A fight or a walk that went wrong (timeout, unreachable target): next creature. Stop only
			// when it keeps happening.
			r.note("round skipped: %v", err)
			if failures++; failures >= 5 {
				return fmt.Errorf("stopping after 5 failed rounds: %v", err)
			}
			continue
		default:
			return err
		}
		failures = 0
	}
	return fmt.Errorf("time limit")
}

// ---- quest logic -----------------------------------------------------------

type run struct {
	s   *Service
	j   *job
	ctx context.Context
}

func (r *run) step(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	r.j.Update(func(st *pb.AutoStatus) { st.Step = msg })
}

// snapshot reads the light state (self, quest log, free slots) and fails when dead.
func (r *run) snapshot() (*statepb.BriefResponse, error) {
	snap, err := r.s.api.Brief(r.ctx)
	if err != nil {
		return nil, err
	}
	if snap.Self == nil {
		return nil, fmt.Errorf("state unavailable")
	}
	if snap.Self.Dead {
		return nil, fmt.Errorf("character is dead")
	}
	r.j.Update(func(st *pb.AutoStatus) { st.Level, st.Xp = snap.Self.Level, snap.Self.Xp })
	return snap, nil
}

func (r *run) quests(req *pb.RunRequest) error {
	if !req.NoProgress {
		if err := r.recover(); err != nil {
			return fmt.Errorf("recover: %w", err)
		}
		r.progress()
	}
	for _, id := range req.Quests {
		r.j.Update(func(st *pb.AutoStatus) { st.Quest = id })
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if err = r.recover(); err != nil {
				return fmt.Errorf("recover: %w", err)
			}
			reward, chosen := req.Rewards[id]
			if err = r.doQuest(id, reward, chosen); err == nil || r.ctx.Err() != nil {
				break
			}
			if b, e := r.s.api.Brief(r.ctx); e != nil || b.Self == nil || !b.Self.Dead {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("quest %d: %w", id, err)
		}
		r.j.Update(func(st *pb.AutoStatus) { st.Done = append(st.Done, id) })
		if !req.NoProgress {
			r.progress()
		}
	}
	return nil
}

// progress equips upgrades, spends talent points and visits the class trainer when it has
// something to teach. Failures are reported in the step but never stop the quest chain.
func (r *run) progress() {
	if g, err := r.s.api.Gear(r.ctx); err == nil && len(g.Equipped) > 0 {
		r.note("equipped %v", g.Equipped)
	}
	// Talents are spent in batches (one call stays short); several calls after a long break.
	for i := 0; i < 8; i++ {
		t, err := r.s.api.Talents(r.ctx)
		if err != nil || len(t.Learned) == 0 {
			break
		}
		r.note("talents %v", t.Learned)
		if t.PointsLeft == 0 || !t.Ok {
			break
		}
	}
	chk, err := r.s.api.TrainCheck(r.ctx)
	if err != nil || chk.Count == 0 || chk.Dist > maxTrainerDist {
		return
	}
	r.step("training at %s: %v", chk.TrainerName, chk.Spells)
	st, err := r.s.api.Train(r.ctx, 20)
	for err == nil && st.State == "running" && r.ctx.Err() == nil {
		st, err = r.s.api.TrainStatus(r.ctx, 20)
	}
	if r.ctx.Err() != nil {
		_ = r.s.api.TrainStop(context.Background())
		return
	}
	if err == nil && len(st.Learned) > 0 {
		r.note("learned %v", st.Learned)
	} else if err == nil && st.State == "failed" {
		r.note("training failed: %s", st.Reason)
	}
}

// note records a progress event in the status (kept short: last 10).
func (r *run) note(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	r.j.Update(func(st *pb.AutoStatus) {
		st.Progress = append(st.Progress, msg)
		if len(st.Progress) > 10 {
			st.Progress = st.Progress[len(st.Progress)-10:]
		}
	})
}

func inLog(snap *statepb.BriefResponse, id uint32) *statepb.Quest {
	for _, q := range snap.Quests {
		if q.Id == id {
			return q
		}
	}
	return nil
}

func itemCount(items []*statepb.Item, entry uint32) uint32 {
	var n uint32
	for _, it := range items {
		if it.Entry == entry {
			n += it.Count
		}
	}
	return n
}

func nearestNpc(list []*worldpb.QuestNpc, mapID uint32, x, y float32) *worldpb.QuestNpc {
	var best *worldpb.QuestNpc
	var bd float32
	for _, n := range list {
		if n.Map != mapID {
			continue
		}
		d := (n.X-x)*(n.X-x) + (n.Y-y)*(n.Y-y)
		if best == nil || d < bd {
			best, bd = n, d
		}
	}
	return best
}

// walk goes to the point. A monster attacking on the way is fought back (combat.defend) before going
// on: the move alone ignores attacks and the character died walking with a monster at its back.
func (r *run) walk(x, y, z, radius float32) error {
	deadline := time.Now().Add(240 * time.Second)
	for fights := 0; ; {
		st, err := r.s.api.Goto(r.ctx, x, y, z, radius, 2)
		attacked := false
		for err == nil && st.State == "moving" {
			if attacked = r.inCombat(); attacked {
				break
			}
			if time.Now().After(deadline) {
				_ = r.s.api.MoveStop(r.ctx)
				return fmt.Errorf("walk to (%.0f,%.0f): timeout", x, y)
			}
			st, err = r.s.api.MoveStatus(r.ctx, 2)
		}
		if err != nil {
			return err
		}
		if !attacked {
			if st.State != "arrived" {
				return fmt.Errorf("walk to (%.0f,%.0f): %s %s", x, y, st.State, st.Reason)
			}
			return nil
		}
		_ = r.s.api.MoveStop(r.ctx)
		if fights++; fights > 20 {
			return fmt.Errorf("walk to (%.0f,%.0f): attacked %d times on the way", x, y, fights)
		}
		r.step("attacked on the way to (%.0f,%.0f): fighting back", x, y)
		if err := r.defend(); err != nil {
			return err
		}
		deadline = deadline.Add(60 * time.Second)
	}
}

// inCombat tells whether a monster is attacking us: we are flagged in combat (UNIT_FLAG_IN_COMBAT,
// which lingers a few seconds after a fight) and a unit in combat within 40 yards targets us.
func (r *run) inCombat() bool {
	const flag = 0x00080000
	b, err := r.s.api.Brief(r.ctx)
	if err != nil || b.Self == nil || b.Self.Dead || b.Self.UnitFlags&flag == 0 {
		return false
	}
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 40, Max: 12, AliveOnly: true})
	if err != nil {
		return false
	}
	for _, o := range vis.Objs {
		if o.Target == b.Self.Guid && o.UnitFlags&flag != 0 {
			return true
		}
	}
	return false
}

// defend fights whatever attacks us until things are calm.
func (r *run) defend() error {
	st, err := r.s.api.CombatDefend(r.ctx, 20)
	for err == nil && st.State == "running" {
		st, err = r.s.api.CombatStatus(r.ctx, 20)
	}
	if err != nil {
		return err
	}
	if b, e := r.s.api.Brief(r.ctx); e == nil && b.Self != nil && b.Self.Dead {
		return fmt.Errorf("character is dead")
	}
	return nil
}

// approachNpc walks to the spawn point, then follows the NPC if it wandered off.
func (r *run) approachNpc(entry uint32, x, y, z float32) error {
	if err := r.walk(x, y, z, 3); err != nil {
		return err
	}
	for i := 0; i < 4; i++ {
		vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Entry: entry, Radius: 80, Max: 1})
		if err != nil || len(vis.Objs) == 0 || vis.Objs[0].Dist <= 4 {
			return nil
		}
		o := vis.Objs[0]
		if err := r.walk(o.X, o.Y, o.Z, 2.5); err != nil {
			return err
		}
		if !jobs.Sleep(r.ctx, 500*time.Millisecond) {
			return r.ctx.Err()
		}
	}
	return nil
}

func (r *run) doQuest(id uint32, reward uint32, chosen bool) error {
	info, err := r.s.api.QuestInfo(r.ctx, id)
	if err != nil {
		return err
	}
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	if inLog(snap, id) == nil {
		if info.MinLevel > me.Level {
			return fmt.Errorf("needs level %d (have %d)", info.MinLevel, me.Level)
		}
		st := nearestNpc(info.Starters, me.Map, me.X, me.Y)
		if st == nil {
			return fmt.Errorf("no quest giver found on this map")
		}
		r.step("go to %s to accept '%s'", st.Name, info.Title)
		if err := r.approachNpc(st.Entry, st.X, st.Y, st.Z); err != nil {
			return err
		}
		if res, err := r.s.api.Accept(r.ctx, st.Entry, id); err != nil || !res.Ok {
			return fmt.Errorf("accept failed: %v %s", err, res.GetReason())
		}
	}
	for tries := 0; tries < 12; tries++ {
		snap, err = r.snapshot()
		if err != nil {
			return err
		}
		q := inLog(snap, id)
		if q == nil {
			return fmt.Errorf("quest vanished from log")
		}
		if q.State == 1 {
			break
		}
		if err := r.objectives(info, q); err != nil {
			return err
		}
	}
	snap, err = r.snapshot()
	if err != nil {
		return err
	}
	if q := inLog(snap, id); q != nil && q.State != 1 {
		return fmt.Errorf("objectives still incomplete (counts %v)", q.Counts)
	}
	// Item rewards need bag space, or the turn-in is refused.
	if err := r.ensureSpace(); err != nil {
		r.step("could not make bag space: %v", err)
	}
	if snap, err = r.snapshot(); err != nil {
		return err
	}
	me = snap.Self
	en := nearestNpc(info.Enders, me.Map, me.X, me.Y)
	if en == nil {
		return fmt.Errorf("no quest ender found on this map")
	}
	r.step("turn in '%s' at %s", info.Title, en.Name)
	if err := r.approachNpc(en.Entry, en.X, en.Y, en.Z); err != nil {
		return err
	}
	if !chosen && len(info.RewardChoices) > 1 {
		if pick, err := r.s.api.PickReward(r.ctx, info.RewardChoices); err == nil {
			reward = pick.Index
			r.note("reward %d for quest %d (%s)", info.RewardChoices[reward], id, pick.Reason)
		}
	}
	if res, err := r.s.api.TurnIn(r.ctx, en.Entry, id, reward); err != nil || !res.Ok {
		return fmt.Errorf("turn-in failed: %v %s", err, res.GetReason())
	}
	return nil
}

// objectives works on the first unfinished objective once.
func (r *run) objectives(info *worldpb.QuestInfo, q *statepb.Quest) error {
	var items []*statepb.Item
	for _, o := range info.Objectives {
		switch o.Kind {
		case "kill", "gameobject":
			var have uint32
			if int(o.Counter) < len(q.Counts) {
				have = q.Counts[o.Counter]
			}
			if have >= o.Count {
				continue
			}
			switch {
			case o.UseItem != 0:
				return r.useItemOn(o, q)
			case o.Kind == "kill" && len(o.CreditFrom) > 0:
				return r.huntAny(o.CreditFrom, o.Name, o.Count-have)
			case o.Kind == "kill":
				return r.hunt(o.Entry, o.Name, o.Count-have, 0, 0)
			}
			return r.useObject(o.Entry, o.Name, false)
		case "item":
			if items == nil {
				snap, err := r.s.api.Snapshot(r.ctx)
				if err != nil {
					return err
				}
				items = snap.Items
			}
			have := itemCount(items, o.Entry)
			if have >= o.Count {
				continue
			}
			return r.collect(o)
		}
	}
	// Counted objectives are done but the quest is not complete: an area remains to explore.
	return r.explore(info)
}

// explore walks into the nearest area trigger of the quest on this map and reports it to the server.
func (r *run) explore(info *worldpb.QuestInfo) error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	var best *worldpb.QuestObjective
	var bestD float32
	for _, o := range info.Objectives {
		if o.Kind != "explore" || o.Map != me.Map {
			continue
		}
		if d := dist2(me.X, me.Y, o.X, o.Y); best == nil || d < bestD {
			best, bestD = o, d
		}
	}
	if best == nil {
		if info.SpecialFlags&0x2 != 0 {
			return fmt.Errorf("quest needs an event auto cannot trigger (escort, script)")
		}
		return nil
	}
	r.step("explore %s (%.0f,%.0f)", best.Name, best.X, best.Y)
	// Go well inside: the server checks the distance to the trigger center against its radius.
	if err := r.clearWalk(best.X, best.Y, best.Z, min(best.Radius/2, 5)); err != nil {
		return err
	}
	if res, err := r.s.api.Explore(r.ctx, best.Entry); err != nil || !res.Ok {
		return fmt.Errorf("explore: %v %s", err, res.GetReason())
	}
	jobs.Sleep(r.ctx, 1500*time.Millisecond)
	return nil
}

func dist2(ax, ay, bx, by float32) float32 {
	return float32(math.Hypot(float64(ax-bx), float64(ay-by)))
}

// huntAny hunts the nearest safe creature among entries (creatures giving the same kill credit).
func (r *run) huntAny(entries []uint32, name string, count uint32) error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	var best *worldpb.Spawn
	var risky []string
	for _, e := range entries {
		sp, err := r.s.api.Spawns(r.ctx, e, me.Map, me.X, me.Y, 2000, "creature")
		if err != nil || len(sp) == 0 {
			continue
		}
		if why := tooDangerous(sp[0], me.Level, me.ClassId); why != "" {
			risky = append(risky, why)
			continue
		}
		if best == nil || sp[0].Dist < best.Dist {
			best = sp[0]
		}
	}
	switch {
	case best == nil && len(risky) > 0:
		return fmt.Errorf("too dangerous for now: %s", strings.Join(risky, "; "))
	case best == nil:
		return fmt.Errorf("no creature giving the credit for %s spawns near here", name)
	}
	r.note("%s credited by killing %s (%d)", name, best.Name, best.Entry)
	return r.hunt(best.Entry, best.Name, count, 0, 0)
}

// useItemOn uses the quest item on the nearest target of a spell-credited objective, then waits for
// the quest counter to move.
func (r *run) useItemOn(o *worldpb.QuestObjective, q *statepb.Quest) error {
	if len(o.UseOn) == 0 {
		return fmt.Errorf("use item %d for %s: no known target (scripted)", o.UseItem, o.Name)
	}
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	var best *worldpb.Spawn
	for _, e := range o.UseOn {
		sp, err := r.s.api.Spawns(r.ctx, e, me.Map, me.X, me.Y, 2000, o.UseOnKind)
		if err == nil && len(sp) > 0 && (best == nil || sp[0].Dist < best.Dist) {
			best = sp[0]
		}
	}
	if best == nil {
		return fmt.Errorf("use item %d for %s: no target spawned on this map", o.UseItem, o.Name)
	}
	r.step("use item %d on %s", o.UseItem, best.Name)
	if err := r.walk(best.X, best.Y, best.Z, 4); err != nil {
		return err
	}
	var before uint32
	if int(o.Counter) < len(q.Counts) {
		before = q.Counts[o.Counter]
	}
	if res, err := r.s.api.UseItem(r.ctx, o.UseItem, o.UseSpell, best.Entry, o.UseOnKind); err != nil || !res.Ok {
		return fmt.Errorf("use item %d on %s: %v %s", o.UseItem, best.Name, err, res.GetReason())
	}
	for i := 0; i < 20; i++ { // cast time and credit, up to 10 s
		jobs.Sleep(r.ctx, 500*time.Millisecond)
		snap, err := r.snapshot()
		if err != nil {
			return err
		}
		if cur := inLog(snap, q.Id); cur == nil || cur.State == 1 || (int(o.Counter) < len(cur.Counts) && cur.Counts[o.Counter] > before) {
			return nil
		}
	}
	return fmt.Errorf("used item %d on %s but got no credit", o.UseItem, best.Name)
}

// tooDangerous explains why a creature should not be hunted at this level ("" when it is fine).
func tooDangerous(sp *worldpb.Spawn, level, class uint32) string {
	margin := uint32(dangerLevelsMelee)
	switch class {
	case 5, 8, 9: // priest, mage, warlock
		margin = dangerLevelsCaster
	}
	switch {
	case sp.Rank == 1 || sp.Rank == 2 || sp.Rank == 3:
		return fmt.Sprintf("%s is an elite (rank %d)", sp.Name, sp.Rank)
	case sp.MinLevel > level+margin:
		return fmt.Sprintf("%s is level %d-%d (character %d)", sp.Name, sp.MinLevel, sp.MaxLevel, level)
	}
	return ""
}

func (r *run) hunt(entry uint32, name string, count, itemEntry, itemCount uint32) error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	sp, err := r.s.api.Spawns(r.ctx, entry, me.Map, me.X, me.Y, 2000, "creature")
	if err != nil || len(sp) == 0 {
		return fmt.Errorf("no spawn found for %s (%d)", name, entry)
	}
	if why := tooDangerous(sp[0], me.Level, me.ClassId); why != "" {
		return fmt.Errorf("too dangerous for now: %s", why)
	}
	r.step("hunt %s x%d", name, count)
	// Spawn points are sorted by distance. When every monster around one is dead (they respawn slowly),
	// go to the next point instead of waiting there; kills already made count.
	var err2 error
	for i := 0; i < len(sp) && i < 5; i++ {
		if i > 0 || sp[0].Dist > 30 {
			if err := r.walk(sp[i].X, sp[i].Y, sp[i].Z, 10); err != nil {
				// A spawn point off the navmesh (water, cliff) or unreachable: try the next one.
				r.step("cannot reach a %s spawn point (%v), trying another", name, err)
				err2 = err
				if r.ctx.Err() != nil {
					return r.ctx.Err()
				}
				continue
			}
		}
		req := &combatpb.KillRequest{Entry: entry, Count: count, ItemEntry: itemEntry, ItemCount: itemCount, Radius: 60, MaxS: 240, WaitS: 20}
		var kills uint32
		kills, err2 = r.fightJob(req)
		if err2 == nil || !strings.Contains(err2.Error(), "no live entry") {
			return err2
		}
		if count > 0 {
			if kills >= count {
				return nil
			}
			count -= kills
		}
		r.step("no %s left here, trying another spawn point", name)
	}
	return err2
}

// Food and drink kept in the bags (20 of each; below 5 a vendor within 500 yards is visited).
const (
	foodWanted, drinkWanted     = 20, 20
	suppliesLow                 = 5
	minMoneyForFood             = 100 // copper
	categoryFood, categoryDrink = 11, 59
)

// supplies counts the food and drink in the bags usable at our level.
func (r *run) supplies(me *statepb.SelfInfo) (food, drink uint32) {
	snap, err := r.s.api.Snapshot(r.ctx)
	if err != nil {
		return foodWanted, drinkWanted // unknown: do not run to a vendor for it
	}
	for _, it := range snap.Items {
		if it.Bag == 0 {
			continue
		}
		var in *worldpb.ItemInfo
		if v, ok := r.s.items.Load(it.Entry); ok {
			in = v.(*worldpb.ItemInfo)
		} else if in, err = r.s.api.ItemInfo(r.ctx, it.Entry); err == nil {
			r.s.items.Store(it.Entry, in)
		}
		if in == nil || in.UseSpell == 0 || in.RequiredLevel > me.Level {
			continue
		}
		switch in.SpellCategory {
		case categoryFood:
			food += it.Count
		case categoryDrink:
			drink += it.Count
		}
	}
	return food, drink
}

// ensureSpace visits the nearest vendor when the bags are nearly full or the gear is worn out:
// it sells what the rules allow and repairs when the vendor can.
func (r *run) ensureSpace() error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	food, drink := r.supplies(me)
	wantDrink := uint32(0)
	if me.PowerType == 0 {
		wantDrink = drinkWanted
	}
	needSell, needRepair := snap.FreeSlots < 3, snap.DurabilityPct < repairBelow
	if failed := r.s.repairFailedAt.Load(); failed > 0 && me.Money < failed+100 {
		needRepair = false
	}
	needFood := (food < suppliesLow || (wantDrink > 0 && drink < suppliesLow)) && me.Money >= minMoneyForFood &&
		snap.DurabilityPct >= repairBelow // repairs first: a broken weapon is worse than no food
	if failed := r.s.foodFailedAt.Load(); failed > 0 && me.Money < failed+100 {
		needFood = false
	}
	if !needSell && !needRepair && !needFood {
		return nil
	}
	const vendorFlag, repairFlag = 128, 4096
	var vendors []*worldpb.Spawn
	if needSell || needRepair {
		flags := uint32(vendorFlag)
		if needRepair {
			flags = repairFlag
		}
		r.step("visiting a vendor (free slots %d, durability %d%%)", snap.FreeSlots, snap.DurabilityPct)
		vendors, err = r.s.api.FindVendor(r.ctx, me.Map, me.X, me.Y, flags)
	} else {
		category := uint32(categoryFood)
		if food >= suppliesLow {
			category = categoryDrink
		}
		r.step("buying food and drink (food %d, drink %d)", food, drink)
		vendors, err = r.s.api.FindFoodVendor(r.ctx, me.Map, me.X, me.Y, category, me.Level)
		if err == nil && len(vendors) == 0 {
			r.note("no food vendor within 500 yards")
			return nil
		}
	}
	if err != nil {
		return err
	}
	// The server only accepts a sale within 5.5 yards: follow the vendor if it moved.
	var npc *statepb.Obj
	var lastErr error
	for _, v := range vendors {
		if len(v.Name) == 0 || v.Name[0] == '[' {
			continue
		}
		var o *statepb.Obj
		err := r.approachNpc(v.Entry, v.X, v.Y, v.Z)
		if err == nil {
			if vis, e := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Entry: v.Entry, Radius: 8, Max: 1}); e == nil && len(vis.Objs) > 0 {
				o = vis.Objs[0]
			}
		}
		switch {
		case err != nil:
			lastErr = err
		case o == nil || o.Dist > 5:
			lastErr = fmt.Errorf("%s not found at its spawn", v.Name)
		default:
			npc = o
		}
		if npc != nil {
			break
		}
	}
	if npc == nil {
		if !needSell && !needRepair {
			r.note("food vendor unreachable: %v", lastErr)
			return nil
		}
		return fmt.Errorf("no reachable vendor (%d candidates): %v", len(vendors), lastErr)
	}
	if needSell || npc.NpcFlags&vendorFlag != 0 {
		res, err := r.s.api.SellJunk(r.ctx)
		if err != nil || !res.Ok {
			return fmt.Errorf("sell: %v %s", err, res.GetReason())
		}
		if res.Sold > 0 {
			r.note("sold %d stacks for %d copper (%d free slots)", res.Sold, res.Copper, res.FreeSlots)
		}
		if needSell && res.FreeSlots < 2 {
			return fmt.Errorf("bags still full after selling %d items: nothing else may be sold (see town.sell_junk with dry_run and show_kept)", res.Sold)
		}
	}
	if npc.NpcFlags&vendorFlag != 0 && snap.DurabilityPct >= repairBelow && (food < foodWanted/2 || (wantDrink > 0 && drink < wantDrink/2)) {
		if res, err := r.s.api.Restock(r.ctx, foodWanted, wantDrink); err == nil && res.Ok && len(res.Bought) > 0 {
			r.s.foodFailedAt.Store(0)
			var what []string
			for _, b := range res.Bought {
				what = append(what, fmt.Sprintf("%d %s", b.Count, b.Name))
			}
			r.note("bought %s for %d copper", strings.Join(what, ", "), res.Copper)
		} else {
			r.note("restock: nothing bought %v %s", err, res.GetReason())
			r.s.foodFailedAt.Store(max(me.Money, 1))
		}
	}
	if npc.NpcFlags&repairFlag != 0 && snap.DurabilityPct < 100 {
		if res, err := r.s.api.Repair(r.ctx); err == nil && res.Ok {
			r.s.repairFailedAt.Store(0)
			r.note("repaired for %d copper (durability %d%%)", res.Copper, res.DurabilityPct)
		} else {
			// Not enough money: play on with worn gear, the next sales pay for it.
			r.note("could not repair: %v %s", err, res.GetReason())
			r.s.repairFailedAt.Store(max(me.Money, 1))
		}
	}
	return nil
}

// fightJob runs combat.kill until it ends and returns the kills made (over the bags-full retries too).
func (r *run) fightJob(req *combatpb.KillRequest) (uint32, error) {
	var total uint32
	for attempt := 0; ; attempt++ {
		if err := r.ensureSpace(); err != nil {
			return total, err
		}
		st, err := r.s.api.CombatKill(r.ctx, req)
		for err == nil && st.State == "running" {
			r.step("combat: kills=%d items=%d hp=%d%% mana=%d%%", total+st.Kills, st.Items, st.HpPct, st.ManaPct)
			if r.ctx.Err() != nil {
				_ = r.s.api.CombatStop(context.Background())
				return total + st.Kills, r.ctx.Err()
			}
			st, err = r.s.api.CombatStatus(r.ctx, 20)
		}
		if err != nil {
			return total, err
		}
		total += st.Kills
		switch {
		case st.State == "failed" && st.Reason == "bags full" && attempt < 2:
			if req.Count > 0 {
				req.Count -= min(req.Count, st.Kills)
			}
			continue // sell and resume
		case st.State == "failed" && st.Reason != "time limit":
			return total, fmt.Errorf("combat: %s", st.Reason)
		}
		return total, nil
	}
}

// collect gets a quest item from its nearest source on this map (best drop chance first on ties).
func (r *run) collect(o *worldpb.QuestObjective) error {
	srcs, err := r.s.api.ItemSources(r.ctx, o.Entry)
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		return fmt.Errorf("no known source for item %s (%d)", o.Name, o.Entry)
	}
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	var src *worldpb.ItemSource
	var bestDist float32
	var risky []string
	for _, cand := range srcs {
		sp, err := r.s.api.Spawns(r.ctx, cand.Entry, me.Map, me.X, me.Y, 2000, cand.Kind)
		if err != nil || len(sp) == 0 {
			continue
		}
		if why := tooDangerous(sp[0], me.Level, me.ClassId); why != "" {
			risky = append(risky, why)
			continue
		}
		if src == nil || sp[0].Dist < bestDist {
			src, bestDist = cand, sp[0].Dist
		}
	}
	if src == nil && len(risky) > 0 {
		return fmt.Errorf("too dangerous for now: %s", strings.Join(risky, "; "))
	}
	if src == nil {
		return fmt.Errorf("no source of %s (%d) spawns on this map", o.Name, o.Entry)
	}
	if src.Kind == "creature" {
		return r.hunt(src.Entry, src.Name, 0, o.Entry, o.Count)
	}
	return r.useObject(src.Entry, src.Name, true)
}

func (r *run) useObject(entry uint32, name string, loot bool) error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	r.step("use %s", name)
	tx, ty, tz := float32(0), float32(0), float32(0)
	if vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "gameobject", Entry: entry, Radius: 80, Max: 1}); err == nil && len(vis.Objs) > 0 {
		o := vis.Objs[0]
		tx, ty, tz = o.X, o.Y, o.Z
	} else {
		sp, err := r.s.api.Spawns(r.ctx, entry, me.Map, me.X, me.Y, 2000, "gameobject")
		if err != nil || len(sp) == 0 {
			return fmt.Errorf("no spawn found for object %s (%d)", name, entry)
		}
		tx, ty, tz = sp[0].X, sp[0].Y, sp[0].Z
	}
	if err := r.clearWalk(tx, ty, tz, 2.5); err != nil {
		return err
	}
	if res, err := r.s.api.UseObject(r.ctx, entry, loot); err != nil || !res.Ok {
		return fmt.Errorf("use %s: %v %s", name, err, res.GetReason())
	}
	jobs.Sleep(r.ctx, 2*time.Second)
	return nil
}
