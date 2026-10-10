// Package auto chains the other modules to complete quests unattended.
package auto

import (
	"context"
	"fmt"
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

const maxWait = 25

type job = jobs.Job[*pb.AutoStatus]

type Service struct {
	pb.UnimplementedAutoServer
	sess   modv1.SessionClient
	corpse chan [4]float32
	api    api.C
	jobs   *jobs.Runner[*pb.AutoStatus]
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	runner := jobs.NewRunner(&pb.AutoStatus{State: "idle"})
	runner.Decorate = func(j *job, st *pb.AutoStatus) { st.ElapsedS = uint32(time.Since(j.Start).Seconds()) }
	pb.RegisterAutoServer(g, &Service{sess: s, corpse: make(chan [4]float32, 4), api: api.C{S: s}, jobs: runner})
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

func (s *Service) Recover(ctx context.Context, r *pb.StatusRequest) (*pb.AutoStatus, error) {
	return s.start(ctx, r.WaitS, &pb.AutoStatus{State: "running", Step: "recovering corpse"}, func(r *run) error { return r.recover() }), nil
}

// start runs body as the current job and maps its error to the final state.
func (s *Service) start(ctx context.Context, waitS uint32, initial *pb.AutoStatus, body func(r *run) error) *pb.AutoStatus {
	s.jobs.Start(initial, func(j *job) {
		err := body(&run{s: s, j: j, ctx: j.Ctx})
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
	if err := r.walk(pos[1], pos[2], pos[3], 15); err != nil {
		return err
	}
	for i := 0; i < 20; i++ {
		w := &wire.Writer{}
		w.U64(0)
		send(uint32(opcode.CMSGReclaimCorpse), w.B)
		if !jobs.Sleep(r.ctx, 3*time.Second) {
			return r.ctx.Err()
		}
		if s, err := r.s.api.Brief(r.ctx); err == nil && s.Self != nil && !s.Self.Dead {
			return nil
		}
	}
	return fmt.Errorf("could not resurrect at corpse")
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	s.jobs.Stop()
	return &emptypb.Empty{}, nil
}

func (s *Service) Stop(context.Context, *emptypb.Empty) (*pb.AutoStatus, error) {
	s.jobs.Stop()
	return s.jobs.Current(), nil
}

func (s *Service) Status(ctx context.Context, r *pb.StatusRequest) (*pb.AutoStatus, error) {
	return s.jobs.Wait(ctx, r.WaitS, 0, maxWait), nil
}

func (s *Service) Run(ctx context.Context, r *pb.RunRequest) (*pb.AutoStatus, error) {
	return s.start(ctx, r.WaitS, &pb.AutoStatus{State: "running"}, func(run *run) error { return run.quests(r) }), nil
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
	for _, id := range req.Quests {
		r.j.Update(func(st *pb.AutoStatus) { st.Quest = id })
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if err = r.recover(); err != nil {
				return fmt.Errorf("recover: %w", err)
			}
			if err = r.doQuest(id, req.Rewards[id]); err == nil || r.ctx.Err() != nil {
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
	}
	return nil
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

func (r *run) walk(x, y, z, radius float32) error {
	st, err := r.s.api.GotoWait(r.ctx, x, y, z, radius, 240)
	if err != nil {
		return err
	}
	if st.State != "arrived" {
		return fmt.Errorf("walk to (%.0f,%.0f): %s %s", x, y, st.State, st.Reason)
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

func (r *run) doQuest(id uint32, reward uint32) error {
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
	me = snap.Self
	en := nearestNpc(info.Enders, me.Map, me.X, me.Y)
	if en == nil {
		return fmt.Errorf("no quest ender found on this map")
	}
	r.step("turn in '%s' at %s", info.Title, en.Name)
	if err := r.approachNpc(en.Entry, en.X, en.Y, en.Z); err != nil {
		return err
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
			if o.Kind == "kill" {
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
	return nil
}

func (r *run) hunt(entry uint32, name string, count, itemEntry, itemCount uint32) error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	me := snap.Self
	sp, err := r.s.api.Spawns(r.ctx, entry, me.Map, me.X, me.Y, 800, "creature")
	if err != nil || len(sp) == 0 {
		return fmt.Errorf("no spawn found for %s (%d)", name, entry)
	}
	r.step("hunt %s x%d", name, count)
	if sp[0].Dist > 30 {
		if err := r.walk(sp[0].X, sp[0].Y, sp[0].Z, 10); err != nil {
			return err
		}
	}
	return r.fightJob(&combatpb.KillRequest{Entry: entry, Count: count, ItemEntry: itemEntry, ItemCount: itemCount, Radius: 80, MaxS: 240, WaitS: 20})
}

// ensureSpace sells gray items when the backpack is nearly full.
func (r *run) ensureSpace() error {
	snap, err := r.snapshot()
	if err != nil {
		return err
	}
	if snap.FreeSlots >= 3 {
		return nil
	}
	r.step("bags nearly full: selling junk")
	me := snap.Self
	vendors, err := r.s.api.FindVendor(r.ctx, me.Map, me.X, me.Y)
	if err != nil {
		return err
	}
	arrived := false
	var lastErr error
	for _, v := range vendors {
		if len(v.Name) == 0 || v.Name[0] == '[' {
			continue
		}
		if lastErr = r.walk(v.X, v.Y, v.Z, 3); lastErr == nil {
			arrived = true
			break
		}
	}
	if !arrived {
		return fmt.Errorf("no reachable vendor to sell junk (%d candidates): %v", len(vendors), lastErr)
	}
	res, err := r.s.api.SellJunk(r.ctx)
	if err != nil || !res.Ok {
		return fmt.Errorf("sell junk: %v %s", err, res.GetReason())
	}
	if res.FreeSlots < 2 {
		return fmt.Errorf("bags still full after selling %d items", res.Sold)
	}
	return nil
}

func (r *run) fightJob(req *combatpb.KillRequest) error {
	for attempt := 0; ; attempt++ {
		if err := r.ensureSpace(); err != nil {
			return err
		}
		st, err := r.s.api.CombatKill(r.ctx, req)
		for err == nil && st.State == "running" {
			r.step("combat: kills=%d items=%d hp=%d%% mana=%d%%", st.Kills, st.Items, st.HpPct, st.ManaPct)
			if r.ctx.Err() != nil {
				_ = r.s.api.CombatStop(context.Background())
				return r.ctx.Err()
			}
			st, err = r.s.api.CombatStatus(r.ctx, 20)
		}
		if err != nil {
			return err
		}
		switch {
		case st.State == "failed" && st.Reason == "bags full" && attempt < 2:
			continue // sell junk and resume
		case st.State == "failed" && st.Reason != "time limit":
			return fmt.Errorf("combat: %s", st.Reason)
		}
		return nil
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
	for _, cand := range srcs {
		sp, err := r.s.api.Spawns(r.ctx, cand.Entry, me.Map, me.X, me.Y, 2000, cand.Kind)
		if err != nil || len(sp) == 0 {
			continue
		}
		if src == nil || sp[0].Dist < bestDist {
			src, bestDist = cand, sp[0].Dist
		}
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
		sp, err := r.s.api.Spawns(r.ctx, entry, me.Map, me.X, me.Y, 800, "gameobject")
		if err != nil || len(sp) == 0 {
			return fmt.Errorf("no spawn found for object %s (%d)", name, entry)
		}
		tx, ty, tz = sp[0].X, sp[0].Y, sp[0].Z
	}
	if err := r.walk(tx, ty, tz, 2.5); err != nil {
		return err
	}
	if res, err := r.s.api.UseObject(r.ctx, entry, loot); err != nil || !res.Ok {
		return fmt.Errorf("use %s: %v %s", name, err, res.GetReason())
	}
	jobs.Sleep(r.ctx, 2*time.Second)
	return nil
}
