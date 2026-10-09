// Package auto chains the other modules to complete quests unattended.
package auto

import (
	"context"
	"fmt"
	"sync"
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
	"wowagent/internal/wire"
)

const maxWait = 25

type job struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	st     pb.AutoStatus
	start  time.Time
}

func (j *job) set(f func(*pb.AutoStatus)) {
	j.mu.Lock()
	f(&j.st)
	j.mu.Unlock()
}

func (j *job) snap() *pb.AutoStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := j.st
	if s.State == "running" {
		s.ElapsedS = uint32(time.Since(j.start).Seconds())
	}
	return &s
}

type Service struct {
	pb.UnimplementedAutoServer
	sess   modv1.SessionClient
	corpse chan [4]float32
	api    api.C
	mu     sync.Mutex
	cur    *job
	last   pb.AutoStatus
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	pb.RegisterAutoServer(g, &Service{sess: s, corpse: make(chan [4]float32, 4), api: api.C{S: s}, last: pb.AutoStatus{State: "idle"}})
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
	s.stopCurrent()
	jctx, cancel := context.WithCancel(context.Background())
	j := &job{cancel: cancel, done: make(chan struct{}), start: time.Now(), st: pb.AutoStatus{State: "running", Step: "recovering corpse"}}
	s.mu.Lock()
	s.cur = j
	s.mu.Unlock()
	go s.finish(jctx, j, func() error { return (&run{s: s, j: j, ctx: jctx}).recover() })
	return s.wait(ctx, r.WaitS, 20), nil
}

func (s *Service) finish(jctx context.Context, j *job, body func() error) {
	err := body()
	j.set(func(st *pb.AutoStatus) {
		switch {
		case err == nil:
			st.State = "done"
		case jctx.Err() != nil:
			st.State, st.Reason = "stopped", "cancelled"
		default:
			st.State, st.Reason = "failed", err.Error()
		}
		st.ElapsedS = uint32(time.Since(j.start).Seconds())
	})
	st := *j.snap()
	s.mu.Lock()
	s.last, s.cur = st, nil
	s.mu.Unlock()
	close(j.done)
}

// recover releases the spirit if needed, runs to the corpse and resurrects.
func (r *run) recover() error {
	snap, err := r.s.api.Snapshot(r.ctx)
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
		time.Sleep(3 * time.Second)
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
		time.Sleep(3 * time.Second)
		if s, err := r.s.api.Snapshot(r.ctx); err == nil && s.Self != nil && !s.Self.Dead {
			return nil
		}
	}
	return fmt.Errorf("could not resurrect at corpse")
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	s.stopCurrent()
	return &emptypb.Empty{}, nil
}

func (s *Service) stopCurrent() {
	s.mu.Lock()
	j := s.cur
	s.mu.Unlock()
	if j != nil {
		j.cancel()
		<-j.done
	}
}

func (s *Service) current() *pb.AutoStatus {
	s.mu.Lock()
	j, last := s.cur, s.last
	s.mu.Unlock()
	if j != nil {
		return j.snap()
	}
	return &last
}

func (s *Service) Stop(context.Context, *emptypb.Empty) (*pb.AutoStatus, error) {
	s.stopCurrent()
	return s.current(), nil
}

func (s *Service) wait(ctx context.Context, w, def uint32) *pb.AutoStatus {
	s.mu.Lock()
	j := s.cur
	s.mu.Unlock()
	if j != nil {
		if w == 0 {
			w = def
		}
		select {
		case <-j.done:
		case <-time.After(time.Duration(min(w, maxWait)) * time.Second):
		case <-ctx.Done():
		}
	}
	return s.current()
}

func (s *Service) Status(ctx context.Context, r *pb.StatusRequest) (*pb.AutoStatus, error) {
	return s.wait(ctx, r.WaitS, 0), nil
}

func (s *Service) Run(ctx context.Context, r *pb.RunRequest) (*pb.AutoStatus, error) {
	s.stopCurrent()
	jctx, cancel := context.WithCancel(context.Background())
	j := &job{cancel: cancel, done: make(chan struct{}), start: time.Now(), st: pb.AutoStatus{State: "running"}}
	s.mu.Lock()
	s.cur = j
	s.mu.Unlock()
	go func() {
		err := s.runQuests(jctx, j, r)
		j.set(func(st *pb.AutoStatus) {
			switch {
			case err == nil:
				st.State = "done"
			case jctx.Err() != nil:
				st.State, st.Reason = "stopped", "cancelled"
			default:
				st.State, st.Reason = "failed", err.Error()
			}
			st.ElapsedS = uint32(time.Since(j.start).Seconds())
		})
		st := *j.snap()
		s.mu.Lock()
		s.last, s.cur = st, nil
		s.mu.Unlock()
		close(j.done)
	}()
	return s.wait(ctx, r.WaitS, 20), nil
}

// ---- quest logic -----------------------------------------------------------

type run struct {
	s   *Service
	j   *job
	ctx context.Context
}

func (r *run) step(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	r.j.set(func(st *pb.AutoStatus) { st.Step = msg })
}

func (r *run) snapshot() (*statepb.SnapshotResponse, error) {
	snap, err := r.s.api.Snapshot(r.ctx)
	if err != nil {
		return nil, err
	}
	if snap.Self == nil {
		return nil, fmt.Errorf("state unavailable")
	}
	if snap.Self.Dead {
		return nil, fmt.Errorf("character is dead")
	}
	r.j.set(func(st *pb.AutoStatus) { st.Level, st.Xp = snap.Self.Level, snap.Self.Xp })
	return snap, nil
}

func (s *Service) runQuests(ctx context.Context, j *job, req *pb.RunRequest) error {
	r := &run{s: s, j: j, ctx: ctx}
	for _, id := range req.Quests {
		j.set(func(st *pb.AutoStatus) { st.Quest = id })
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if err = r.recover(); err != nil {
				return fmt.Errorf("recover: %w", err)
			}
			if err = r.doQuest(id, req.Rewards[id]); err == nil || ctx.Err() != nil {
				break
			}
			if snap, e := s.api.Snapshot(ctx); e != nil || snap.Self == nil || !snap.Self.Dead {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("quest %d: %w", id, err)
		}
		j.set(func(st *pb.AutoStatus) { st.Done = append(st.Done, id) })
	}
	return nil
}

func inLog(snap *statepb.SnapshotResponse, id uint32) *statepb.Quest {
	for _, q := range snap.Quests {
		if q.Id == id {
			return q
		}
	}
	return nil
}

func itemCount(snap *statepb.SnapshotResponse, entry uint32) uint32 {
	var n uint32
	for _, it := range snap.Items {
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
		if err := r.walk(st.X, st.Y, st.Z, 3); err != nil {
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
		if err := r.objectives(info, snap, q); err != nil {
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
	if err := r.walk(en.X, en.Y, en.Z, 3); err != nil {
		return err
	}
	if res, err := r.s.api.TurnIn(r.ctx, en.Entry, id, reward); err != nil || !res.Ok {
		return fmt.Errorf("turn-in failed: %v %s", err, res.GetReason())
	}
	return nil
}

// objectives works on the first unfinished objective once.
func (r *run) objectives(info *worldpb.QuestInfo, snap *statepb.SnapshotResponse, q *statepb.Quest) error {
	killIdx := 0
	for _, o := range info.Objectives {
		switch o.Kind {
		case "kill", "gameobject":
			var have uint32
			if killIdx < len(q.Counts) {
				have = q.Counts[killIdx]
			}
			killIdx++
			if have >= o.Count {
				continue
			}
			if o.Kind == "kill" {
				return r.hunt(o.Entry, o.Name, o.Count-have, 0, 0)
			}
			return r.useObject(o.Entry, o.Name, false)
		case "item":
			have := itemCount(snap, o.Entry)
			if have >= o.Count {
				continue
			}
			return r.collect(o, have)
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
	if api.FreeSlots(snap) >= 3 {
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
	if st.State == "failed" && st.Reason == "bags full" {
		return r.fightJob(req)
	}
	if st.State == "failed" && st.Reason != "time limit" {
		return fmt.Errorf("combat: %s", st.Reason)
	}
	return nil
}

func (r *run) collect(o *worldpb.QuestObjective, have uint32) error {
	srcs, err := r.s.api.ItemSources(r.ctx, o.Entry)
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		return fmt.Errorf("no known source for item %s (%d)", o.Name, o.Entry)
	}
	src := srcs[0]
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
	sp, err := r.s.api.Spawns(r.ctx, entry, me.Map, me.X, me.Y, 800, "gameobject")
	if err != nil || len(sp) == 0 {
		return fmt.Errorf("no spawn found for object %s (%d)", name, entry)
	}
	r.step("use %s", name)
	if err := r.walk(sp[0].X, sp[0].Y, sp[0].Z, 3); err != nil {
		return err
	}
	if res, err := r.s.api.UseObject(r.ctx, entry, loot); err != nil || !res.Ok {
		return fmt.Errorf("use %s: %v %s", name, err, res.GetReason())
	}
	time.Sleep(2 * time.Second)
	return nil
}
