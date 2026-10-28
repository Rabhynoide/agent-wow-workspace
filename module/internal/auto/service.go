// Package auto chains the other modules to complete quests unattended.
package auto

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sort"
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
	questpb "wowagent/gen/questv1"
	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/api"
	"wowagent/internal/jobs"
	"wowagent/internal/tuning"
	"wowagent/internal/wire"
	"wowagent/internal/xpscore"
)

const (
	// Death.SicknessLevel of the server (worldserver.conf): from this level the spirit healer
	// applies resurrection sickness (level-10 minutes up to 10 min of reduced stats).
	sicknessLevel = 11
	// The server reclaims a corpse within 39 yards (CORPSE_RECLAIM_RADIUS): keep a margin.
	reclaimDist = 37
	maxWait     = 25
	// Do not walk further than this (yards) to a class trainer between quests: the walk does not
	// avoid monsters yet.
	maxTrainerDist = 400
	// Repair when an equipped item falls below this durability (percent).
	repairBelow = 60
)

type job = jobs.Job[*pb.AutoStatus]

type Service struct {
	pb.UnimplementedAutoServer
	sess      modv1.SessionClient
	corpse    chan [4]float32
	api       api.C
	jobs      *jobs.Runner[*pb.AutoStatus]
	guard     atomic.Bool // defend when attacked between jobs
	items     sync.Map    // item entry -> *worldpb.ItemInfo (food checks)
	detection sync.Map    // creature entry -> detection range (clear.go aggroOf)
	// money when a repair failed (not enough): no new repair trip before we earn a silver more
	repairFailedAt atomic.Uint32
	// money when a food trip bought nothing (too poor): no new trip before we earn a silver more
	foodFailedAt atomic.Uint32
	// lastVendor: where the last vendor we reached stands (map, x, y), searched around when none is
	// within 500 yards of us (lastvendor.go).
	lastVendor atomic.Pointer[[3]float32]
	// potionFailedAt: money when the last potion trip bought nothing (no new trip before 1 silver more).
	potionFailedAt atomic.Uint32
	// learned fights (journal/learn/fights.json): the danger of a monster type
	fights *xpscore.File
	// caution and play knobs (journal/learn/tuning.json): quest_hub
	tune *tuning.Source

	mu sync.Mutex
	// chests we went to (guid -> when): not tried again for chestRetry
	chestTried map[uint64]time.Time
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	runner := jobs.NewRunner(&pb.AutoStatus{State: "idle"})
	runner.Decorate = func(j *job, st *pb.AutoStatus) { st.ElapsedS = uint32(time.Since(j.Start).Seconds()) }
	svc := &Service{sess: s, corpse: make(chan [4]float32, 4), api: api.C{S: s}, jobs: runner, fights: xpscore.LearnedFile(), tune: tuning.NewSource(""),
		chestTried: map[uint64]time.Time{}}
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
		if r.OtherMap {
			if err := run.goTo(r.Map, r.X, r.Y, r.Z); err != nil {
				return err
			}
		}
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
		err := body(&run{s: s, j: j, ctx: j.Ctx, gearSeen: map[[3]uint32]bool{}})
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
	r.j.Update(func(st *pb.AutoStatus) { st.Deaths++ })
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
	err = r.corpseRez(send, snap.Self.Level < sicknessLevel)
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
// healer: the spirit healer is free (no resurrection sickness yet): when monsters still stand near
// the corpse after the wait, give up the corpse instead of coming back among them (Fargodeep,
// 2026-10-27: back at 33-42 % of health with kobolds 13-17 yards away, killed again twice in a row).
func (r *run) corpseRez(send func(uint32, []byte), healer bool) error {
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
		// The corpse can lie on a scrap of navmesh cut from the rest (a rock of the Gol'Bolar quarry:
		// "no walkable path", the run of 2026-10-25 stopped there): reclaiming only needs us within
		// reclaimDist, so go to the closest reachable point.
		near, ok := r.reachableNear(pos, reclaimDist-2)
		if !ok {
			return err
		}
		r.note("corpse out of reach (%v): reclaiming from (%.0f,%.0f)", err, near[0], near[1])
		if err := r.walk(near[0], near[1], near[2], 1.5); err != nil {
			return err
		}
		return r.reclaim(send)
	}
	// The corpse often lies in the camp that killed us: come back to life at the edge of the
	// reclaim radius (39 yards), as far as possible from the monsters (mod-playerbots does the same).
	for wait := 0; ; wait++ {
		sp, err := r.s.api.SafePoint(r.ctx, pos[1], pos[2], pos[3], reclaimDist, false)
		if err != nil || !sp.Ok {
			r.note("no safe point near the corpse: %v %s", err, sp.GetReason())
			if err := r.walk(pos[1], pos[2], pos[3], 15); err != nil {
				return err
			}
			break
		}
		// 2 minutes: resurrecting with kobolds 10-14 yards away after 1 minute killed at once
		// (Fargodeep, 2026-10-26).
		if sp.Clearance < 25 && wait >= 8 && healer {
			return fmt.Errorf("monsters still %.0f yards from the corpse", sp.Clearance)
		}
		if sp.Clearance < 25 && wait < 8 {
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
	return r.reclaim(send)
}

// reachableNear is the end of a walk from us towards pos stopping at the closest walkable point
// within reach yards of it (move.path snap), false when there is none.
func (r *run) reachableNear(pos [4]float32, reach float32) ([3]float32, bool) {
	b, err := r.s.api.Brief(r.ctx)
	if err != nil || b.Self == nil {
		return [3]float32{}, false
	}
	me := b.Self
	res, err := r.s.api.PathFromSnap(r.ctx, me.Map, me.X, me.Y, me.Z, pos[1], pos[2], pos[3], reach)
	if err != nil || res.Error != "" || res.End == nil {
		return [3]float32{}, false
	}
	if dist2(res.End.X, res.End.Y, pos[1], pos[2]) > reach {
		return [3]float32{}, false
	}
	return [3]float32{res.End.X, res.End.Y, res.End.Z}, true
}

// reclaim asks for the corpse until we are alive.
func (r *run) reclaim(send func(uint32, []byte)) error {
	// The reclaim delay grows with repeated deaths (up to 2 minutes): keep trying for 4 minutes.
	for i := 0; i < 80; i++ {
		w := &wire.Writer{}
		w.U64(0)
		send(uint32(opcode.CMSGReclaimCorpse), w.B)
		if !jobs.Sleep(r.ctx, 3*time.Second) {
			return r.ctx.Err()
		}
		if s, err := r.s.api.Brief(r.ctx); err == nil && s.Self != nil && !s.Self.Dead {
			r.awayFromMonsters(s.Self)
			r.restAfterRez()
			return nil
		}
	}
	return fmt.Errorf("could not resurrect at corpse")
}

// awayFromMonsters walks to a safer spot (move.safe_point, 35 yards) when a hostile monster stands
// within 20 yards: resting next to them right after resurrecting got us killed again.
func (r *run) awayFromMonsters(me *statepb.SelfInfo) {
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 20, Max: 1, AliveOnly: true, HostileOnly: true})
	if err != nil || len(vis.Objs) == 0 {
		return
	}
	sp, err := r.s.api.SafePoint(r.ctx, me.X, me.Y, me.Z, 35, false)
	if err != nil || !sp.Ok || sp.Clearance <= vis.Objs[0].Dist+5 {
		return
	}
	r.step("monster %.0f yards away after resurrecting: moving to a safer spot", vis.Objs[0].Dist)
	_ = r.walk(sp.X, sp.Y, sp.Z, 1.5)
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

// spiritHealer is the creature entry of the graveyard spirit healers.
const spiritHealer = 6491

// healerRez walks to the nearest spirit healer and resurrects there. The ghost waits at the corpse by
// then and the server only shows units within ~100 yards: the healer is found among the spawns of
// the database (the run of 2026-10-26 stopped dead near the leper gnomes of Dun Morogh, with no healer
// within 150 yards of the corpse).
func (r *run) healerRez(send func(uint32, []byte)) bool {
	const spiritHealerFlag = 0x4000
	vis, err := r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", NpcFlags: spiritHealerFlag, Radius: 150, Max: 1})
	if err != nil || len(vis.Objs) == 0 {
		b, err := r.s.api.Brief(r.ctx)
		if err != nil || b.Self == nil {
			return false
		}
		sp, err := r.s.api.Spawns(r.ctx, spiritHealer, b.Self.Map, b.Self.X, b.Self.Y, 2000, "creature")
		if err != nil || len(sp) == 0 {
			return false
		}
		r.step("going to the graveyard's spirit healer (%.0f yards)", sp[0].Dist)
		if err := r.walk(sp[0].X, sp[0].Y, sp[0].Z, 5); err != nil {
			return false
		}
		vis, err = r.s.api.Nearby(r.ctx, &statepb.NearbyRequest{Kind: "unit", NpcFlags: spiritHealerFlag, Radius: 30, Max: 1})
		if err != nil || len(vis.Objs) == 0 {
			return false
		}
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
		err = r.huntAt(entry, fmt.Sprintf("creature %d", entry), 4, 0, 0, true)
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
	// detour: on the way to a chest or a vendor (no other detour from there)
	detour bool
	// lastShop: last vendor visit or check on the way (shop); lastChestCheck: last chest lookup
	lastShop, lastChestCheck, lastVendorCheck time.Time
	lastEngageCheck                           time.Time // engage.go
	lastGearCheck                             time.Time // gearshop.go
	gearSeen                                  map[[3]uint32]bool
	weaponWant, weaponVendor                  uint32          // copper kept aside for a weapon upgrade seen, and its vendor (gearshop.go)
	owned                                     map[uint32]bool // item entries we carry or wear, at the last gear look
	// travelling: inside a journey (travel.go), whose walks do not plan again; lastFlightCheck and
	// triedNodes: flight masters of unknown nodes looked for and visited on the way
	travelling      bool
	lastFlightCheck time.Time
	triedNodes      map[uint32]bool
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
	if r.s.tune.Get().QuestHub {
		return r.hubQuests(req)
	}
	for _, id := range req.Quests {
		r.j.Update(func(st *pb.AutoStatus) { st.Quest = id })
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if err = r.recover(); err != nil {
				return fmt.Errorf("recover: %w", err)
			}
			reward, chosen := req.Rewards[id]
			if err = r.doQuest(id, reward, chosen, slices.Contains(req.Hold, id)); err == nil || r.ctx.Err() != nil {
				break
			}
			if b, e := r.s.api.Brief(r.ctx); e != nil || b.Self == nil || !b.Self.Dead {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("quest %d: %w", id, err)
		}
		if !slices.Contains(req.Hold, id) {
			r.j.Update(func(st *pb.AutoStatus) { st.Done = append(st.Done, id) })
		}
		if !req.NoProgress {
			r.progress()
		}
	}
	return nil
}

// gearUp equips the upgrades (and bags) among our bags.
func (r *run) gearUp() {
	if g, err := r.s.api.Gear(r.ctx); err == nil && len(g.Equipped) > 0 {
		r.note("equipped %v", g.Equipped)
		r.step("equipped %v", g.Equipped)
	}
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
	defer r.shop(false) // at the trainer or wherever progress runs: a vendor near is visited
	chk, err := r.s.api.TrainCheck(r.ctx)
	if err != nil || chk.Count == 0 || chk.Dist > maxTrainerDist {
		return
	}
	r.step("training at %s: %v", chk.TrainerName, chk.Spells)
	// Our own walk, which fights back: train's approach does not, and a snapshot started at the
	// Fargodeep mine died on the way to Goldshire (bench of 2026-10-13).
	if err := r.walk(chk.X, chk.Y, chk.Z, 5); err != nil {
		r.note("training: %v", err)
		return
	}
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
	limit := 240 * time.Second
	if b, err := r.s.api.Brief(r.ctx); err == nil && b.Self != nil {
		if err := r.goTo(b.Self.Map, x, y, z); err != nil {
			return err
		}
		// Long walks (Deeprun Tram tunnel: 2 600 yards) get the time to walk them.
		if b, err = r.s.api.Brief(r.ctx); err == nil && b.Self != nil {
			limit = max(limit, time.Duration(dist2(b.Self.X, b.Self.Y, x, y)/4)*time.Second)
		}
	}
	deadline := time.Now().Add(limit)
	engaged := 0 // monsters engaged on the way (engage.go)
	for fights := 0; ; {
		// No new fight below walkRestBelow: a monster met on the way would be a chained fight
		// (4 of the 5 deaths of the 2026-10-12 bench started at 12-41 %).
		r.restBelow(walkRestBelow)
		st, err := r.s.api.Goto(r.ctx, x, y, z, radius, 2)
		attacked := false
		var chest *questpb.Chest
		var vendor, prey *statepb.Obj
		var gear *gearBuy
		for err == nil && st.State == "moving" {
			if attacked = r.inCombat(); attacked {
				break
			}
			if engaged < engageMaxPerWalk {
				if prey = r.engageOnTheWay(x, y); prey != nil {
					break
				}
			}
			if chest = r.chestInSight(); chest != nil {
				break
			}
			if vendor = r.vendorOnTheWay(); vendor != nil {
				break
			}
			if gear = r.gearOnTheWay(); gear != nil {
				break
			}
			if node, fm := r.flightOnTheWay(); fm != nil {
				_ = r.s.api.MoveStop(r.ctx)
				r.learnFlight(node, fm)
				deadline = deadline.Add(60 * time.Second)
				st, err = r.s.api.Goto(r.ctx, x, y, z, radius, 2)
				continue
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
		if prey != nil && !attacked {
			_ = r.s.api.MoveStop(r.ctx)
			engaged++
			r.step("engaging monster %d (level %d) on the way, %.0f yards away", prey.Entry, prey.Level, prey.Dist)
			if _, err := r.fightJob(&combatpb.KillRequest{Guid: prey.Guid, Radius: 60, MaxS: 60, WaitS: 20}); err != nil &&
				!strings.Contains(err.Error(), "no live entry") {
				r.note("engaging on the way: %v", err)
			}
			deadline = deadline.Add(60 * time.Second)
			continue
		}
		if gear != nil && !attacked {
			_ = r.s.api.MoveStop(r.ctx)
			r.buyGear(gear)
			deadline = deadline.Add(60 * time.Second)
			continue
		}
		if chest != nil || vendor != nil {
			_ = r.s.api.MoveStop(r.ctx)
			if chest != nil {
				r.openChest(chest)
			} else {
				r.visit(vendor)
			}
			deadline = deadline.Add(60 * time.Second)
			continue
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
		fled, err := r.defend()
		if err != nil {
			return err
		}
		if fled > 0 {
			// Going on led back into the monsters we ran from (Narg the Taskmaster and kobolds: three
			// flights then a death, 6 → 10 bench of 2026-10-13). "too dangerous" postpones the quest.
			return fmt.Errorf("walk to (%.0f,%.0f): too dangerous for now: fled from monsters on the way", x, y)
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

// defend fights whatever attacks us until things are calm, and tells how many times we fled.
func (r *run) defend() (uint32, error) {
	st, err := r.s.api.CombatDefend(r.ctx, 20)
	for err == nil && st.State == "running" {
		st, err = r.s.api.CombatStatus(r.ctx, 20)
	}
	if err != nil {
		return 0, err
	}
	if b, e := r.s.api.Brief(r.ctx); e == nil && b.Self != nil && b.Self.Dead {
		return st.Fled, fmt.Errorf("character is dead")
	}
	return st.Fled, nil
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

// doQuest accepts the quest, does its objectives and turns it in; hold: not turned in (the guide
// does it later, RunRequest.hold).
func (r *run) doQuest(id uint32, reward uint32, chosen, hold bool) error {
	info, err := r.s.api.QuestInfo(r.ctx, id)
	if err != nil {
		return err
	}
	if err := r.acceptQuest(id, info); err != nil {
		return err
	}
	if err := r.completeQuest(id, info); err != nil {
		return err
	}
	if hold {
		r.note("quest %d done, kept for the guide's turn-in later", id)
		return nil
	}
	return r.turnInQuest(id, info, reward, chosen)
}

// acceptQuest goes to the nearest giver and accepts the quest, unless it is already in the log.
func (r *run) acceptQuest(id uint32, info *worldpb.QuestInfo) error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	if inLog(snap, id) != nil {
		return nil
	}
	if info.MinLevel > me.Level {
		return fmt.Errorf("needs level %d (have %d)", info.MinLevel, me.Level)
	}
	st := npcOn(info.Starters, me.Map, me.X, me.Y)
	if st == nil {
		return fmt.Errorf("no quest giver found")
	}
	r.step("go to %s to accept '%s'", st.Name, info.Title)
	if err := r.goTo(st.Map, st.X, st.Y, st.Z); err != nil {
		return err
	}
	if err := r.approachNpc(st.Entry, st.X, st.Y, st.Z); err != nil {
		return err
	}
	if res, err := r.s.api.Accept(r.ctx, st.Entry, id); err != nil || !res.Ok {
		return fmt.Errorf("accept failed: %v %s", err, res.GetReason())
	}
	r.shop(false) // quest givers stand in town
	return nil
}

// completeQuest works on the objectives of a quest in the log until it is complete.
func (r *run) completeQuest(id uint32, info *worldpb.QuestInfo) error {
	for tries := 0; tries < 12; tries++ {
		snap, err := r.snapshot()
		if err != nil {
			return err
		}
		q := inLog(snap, id)
		if q == nil {
			return fmt.Errorf("quest vanished from log")
		}
		if q.State == 1 {
			return nil
		}
		if err := r.objectives(info, q); err != nil {
			return err
		}
	}
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	if q := inLog(snap, id); q != nil && q.State != 1 {
		return fmt.Errorf("objectives still incomplete (counts %v)", q.Counts)
	}
	return nil
}

// turnInQuest goes to the nearest ender of a complete quest and turns it in.
func (r *run) turnInQuest(id uint32, info *worldpb.QuestInfo, reward uint32, chosen bool) error {
	if err := r.ensureRepair(); err != nil {
		r.step("could not repair: %v", err)
	}
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	en := npcOn(info.Enders, me.Map, me.X, me.Y)
	if en == nil {
		return fmt.Errorf("no quest ender found")
	}
	r.step("turn in '%s' at %s", info.Title, en.Name)
	if err := r.goTo(en.Map, en.X, en.Y, en.Z); err != nil {
		return err
	}
	if err := r.approachNpc(en.Entry, en.X, en.Y, en.Z); err != nil {
		return err
	}
	// Item rewards need bag space, or the turn-in is refused: sell at a vendor near the ender, else
	// throw away the cheapest junk.
	r.shop(true)
	if n := len(info.RewardItems) + min(len(info.RewardChoices), 1); n > 0 {
		r.rewardRoom(n)
	}
	// Back from the vendor: the server takes the quest within 5.5 yards of the ender (Benchurodky
	// skipped four turn-ins 8-14 yards away, 2026-10-23).
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
		if why := r.s.tooDangerous(sp[0], me.Level, me.ClassId); why != "" {
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

// tooDangerous explains why a creature should not be hunted at this level ("" when it is fine): elites,
// and monsters whose fight costs too much health or kills too often (xpscore.Risky, from the learned
// fights; it replaces the fixed "level + 1" margin).
func (s *Service) tooDangerous(sp *worldpb.Spawn, level, class uint32) string {
	if sp.Rank == 1 || sp.Rank == 2 || sp.Rank == 3 {
		return fmt.Sprintf("%s is an elite (rank %d)", sp.Name, sp.Rank)
	}
	if why := xpscore.Risky(spawnMob(sp), level, class, s.fights.Get()); why != "" {
		return fmt.Sprintf("%s is level %d-%d (character %d): %s", sp.Name, sp.MinLevel, sp.MaxLevel, level, why)
	}
	return ""
}

func spawnMob(sp *worldpb.Spawn) xpscore.Mob {
	return xpscore.Mob{Entry: sp.Entry, Name: sp.Name, MinLevel: sp.MinLevel, MaxLevel: sp.MaxLevel,
		HealthMod: float64(sp.HealthMod), DamageMod: float64(sp.DamageMod), AttackMs: sp.AttackMs}
}

// campSize: a spawn point with this many others of the same creature within campRadius is a camp
// (the kobolds at the Fargodeep mine entrance: the grind went there and died three times in a row).
const (
	campSize   = 3
	campRadius = 20
)

// neighbours counts the other spawn points within campRadius of p.
func neighbours(p *worldpb.Spawn, all []*worldpb.Spawn) int {
	n := 0
	for _, o := range all {
		if o != p && math.Hypot(float64(o.X-p.X), float64(o.Y-p.Y)) < campRadius {
			n++
		}
	}
	return n
}

// byIsolation sorts spawn points by distance plus 40 yards per neighbour: an isolated monster a bit
// further away beats the edge of a camp.
func byIsolation(sp []*worldpb.Spawn) []*worldpb.Spawn {
	score := make(map[*worldpb.Spawn]float32, len(sp))
	for _, p := range sp {
		score[p] = p.Dist + 40*float32(neighbours(p, sp))
	}
	out := append([]*worldpb.Spawn(nil), sp...)
	sort.SliceStable(out, func(i, j int) bool { return score[out[i]] < score[out[j]] })
	return out
}

func (r *run) hunt(entry uint32, name string, count, itemEntry, itemCount uint32) error {
	return r.huntAt(entry, name, count, itemEntry, itemCount, false)
}

// huntAt is hunt; when grinding, a creature that only lives in camps is skipped (a quest needs it).
func (r *run) huntAt(entry uint32, name string, count, itemEntry, itemCount uint32, grinding bool) error {
	defer r.gearUp() // the loot of the round: progress only came at quests and levels (100 min of grind in mail-less gear, 2026-10-13)
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	sp, err := r.s.api.Spawns(r.ctx, entry, me.Map, me.X, me.Y, 2000, "creature")
	if err != nil || len(sp) == 0 {
		return fmt.Errorf("no spawn found for %s (%d)", name, entry)
	}
	if why := r.s.tooDangerous(sp[0], me.Level, me.ClassId); why != "" {
		return fmt.Errorf("too dangerous for now: %s", why)
	}
	sp = byIsolation(sp)
	if grinding && neighbours(sp[0], sp) >= campSize {
		return fmt.Errorf("too dangerous for now: every %s spawn point is in a camp", name)
	}
	r.step("hunt %s x%d", name, count)
	// Spawn points are sorted by distance. When every monster around one is dead (they respawn slowly),
	// go to the next point instead of waiting there; kills already made count.
	var err2 error
	tried, avoided := 0, 0
	// Spawn points we fled on the way to: the others close to them lie behind the same monsters (the
	// four Kobold Miner points of the Fargodeep mine cost a flight each, twice: 10 flights in 10 min,
	// bench of 2026-10-13).
	var fledTo [][2]float32
	for i := 0; i < len(sp) && tried < 5; i++ {
		// Re-read each time: a flight during this hunt adds a zone.
		if b, err := r.s.api.Brief(r.ctx); err == nil && b.Self != nil {
			if z := fledOn(r.fledZones(), b.Self.Map, b.Self.X, b.Self.Y, sp[i].X, sp[i].Y); z != nil {
				if avoided++; avoided == 1 {
					slog.Info("spawn point near a fled zone", "entry", entry, "zone", [2]int{int(z.X), int(z.Y)}, "left_s", z.LeftS)
					r.step("skipping %s spawn points near where we fled from (%d s left)", name, z.LeftS)
				}
				continue
			}
		}
		if behindFled(fledTo, sp[i].X, sp[i].Y) {
			continue
		}
		tried++
		if tried > 1 || sp[i].Dist > 30 {
			if err := r.walk(sp[i].X, sp[i].Y, sp[i].Z, 10); err != nil {
				// A spawn point off the navmesh (water, cliff) or unreachable: try the next one.
				r.step("cannot reach a %s spawn point (%v), trying another", name, err)
				err2 = err
				if strings.Contains(err.Error(), "too dangerous") {
					fledTo = append(fledTo, [2]float32{sp[i].X, sp[i].Y})
				}
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
	if tried == 0 && avoided > 0 {
		return fmt.Errorf("no live entry away from fled places: the %d nearest %s spawn points are near places we fled from", avoided, name)
	}
	return err2
}

// fledZones asks combat for the places we fled from a short while ago (combat/zones.go).
func (r *run) fledZones() []*combatpb.Zone {
	st, err := r.s.api.CombatStatus(r.ctx, 0)
	if err != nil {
		return nil
	}
	return st.Avoid
}

// fledOn returns the fled zone that a spawn point (x, y) lies in, or that the straight way to it from
// (fx, fy) crosses, nil when none. Starting inside a zone is fine as long as the way leads out of it
// (right after a flight we are often still close). The walk follows the navmesh, not a straight line:
// a bend can still go through a zone.
func fledOn(zs []*combatpb.Zone, m uint32, fx, fy, x, y float32) *combatpb.Zone {
	for _, z := range zs {
		if z.Map != m {
			continue
		}
		if math.Hypot(float64(x-z.X), float64(y-z.Y)) < float64(z.Radius) {
			return z
		}
		dx, dy := float64(x-fx), float64(y-fy)
		l2 := dx*dx + dy*dy
		if l2 == 0 {
			continue
		}
		t := (float64(z.X-fx)*dx + float64(z.Y-fy)*dy) / l2
		if t <= 0 {
			continue // the way leads away from the zone
		}
		t = math.Min(t, 1)
		if math.Hypot(float64(fx)+t*dx-float64(z.X), float64(fy)+t*dy-float64(z.Y)) < float64(z.Radius) {
			return z
		}
	}
	return nil
}

// Food and drink kept in the bags (20 of each; below 5 a vendor within 500 yards is visited).
const (
	foodWanted, drinkWanted     = 20, 20
	suppliesLow                 = 5
	minMoneyForFood             = 100 // copper
	categoryFood, categoryDrink = 11, 59
	// Healing potions for critical health (combat/potion.go): bought at the vendor visited for food or
	// to sell (they share category 4 with mana potions: town buys the healing ones). No trip for them.
	potionsWanted      = 3
	categoryPotion     = 4
	minMoneyForPotions = 500 // copper: 3 Lesser Healing Potions cost 3 silver
)

// supplies counts the food, drink and potions in the bags usable at our level.
func (r *run) supplies(me *statepb.SelfInfo) (food, drink, potions uint32) {
	snap, err := r.s.api.Snapshot(r.ctx)
	if err != nil {
		return foodWanted, drinkWanted, potionsWanted // unknown: do not run to a vendor for it
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
		case categoryPotion:
			potions += it.Count
		}
	}
	return food, drink, potions
}

// fightJob runs combat.kill until it ends and returns the kills made (over the chest detours too: a
// chest in sight stops the hunt, the fight under way ends, the chest is opened, the hunt resumes).
func (r *run) fightJob(req *combatpb.KillRequest) (uint32, error) {
	var total uint32
	for chests := 0; ; {
		if err := r.ensureRepair(); err != nil {
			return total, err
		}
		st, err := r.s.api.CombatKill(r.ctx, req)
		var chest *questpb.Chest
		for err == nil && st.State == "running" {
			r.step("combat: kills=%d items=%d hp=%d%% mana=%d%%", total+st.Kills, st.Items, st.HpPct, st.ManaPct)
			if r.ctx.Err() != nil {
				_ = r.s.api.CombatStop(context.Background())
				return total + st.Kills, r.ctx.Err()
			}
			if chests < 5 {
				if chest = r.chestInSight(); chest != nil {
					_ = r.s.api.CombatStop(r.ctx)
					st, err = r.s.api.CombatStatus(r.ctx, 0)
					break
				}
			}
			st, err = r.s.api.CombatStatus(r.ctx, 5)
		}
		if err != nil {
			return total, err
		}
		total += st.Kills
		switch {
		case chest != nil:
			chests++
			if req.Count > 0 {
				if req.Count -= min(req.Count, st.Kills); req.Count == 0 {
					r.openChest(chest)
					return total, nil
				}
			}
			r.openChest(chest)
			continue // resume the hunt
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
		if why := r.s.tooDangerous(sp[0], me.Level, me.ClassId); why != "" {
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

// behindFled tells whether a spawn point lies within fledReach of one we fled on the way to.
func behindFled(fledTo [][2]float32, x, y float32) bool {
	for _, p := range fledTo {
		if math.Hypot(float64(p[0]-x), float64(p[1]-y)) < fledReach {
			return true
		}
	}
	return false
}

// fledReach: yards around a spawn point we fled on the way to.
const fledReach = 80
