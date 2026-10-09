// Package combat hunts creatures, fights with priest spells and loots.
package combat

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/api"
	"wowagent/internal/loot"
	"wowagent/internal/moveio"
	"wowagent/internal/wire"
)

// Highest rank first.
var (
	smite      = []uint32{25364, 25363, 10934, 10933, 6060, 1004, 984, 598, 591, 585}
	lesserHeal = []uint32{2053, 2052, 2050}
	healSpells = []uint32{25314, 10965, 10964, 10963, 6064, 6063, 2055, 2054}
)

const (
	castRange  = 26
	meleeRange = 4.5
	maxWait    = 25
)

type job struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	st     pb.CombatStatus
	start  time.Time
}

func (j *job) snap() *pb.CombatStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := j.st
	if s.State == "running" {
		s.ElapsedS = uint32(time.Since(j.start).Seconds())
	}
	return &s
}

func (j *job) update(f func(*pb.CombatStatus)) {
	j.mu.Lock()
	f(&j.st)
	j.mu.Unlock()
}

type Service struct {
	pb.UnimplementedCombatServer
	s    modv1.SessionClient
	api  api.C
	loot *loot.Tracker
	guid uint64

	mu         sync.Mutex
	cur        *job
	last       pb.CombatStatus
	castCount  uint8
	castFailed time.Time
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	guid, _ := strconv.ParseUint(os.Getenv("AGENT_WOW_CHARACTER_GUID"), 10, 64)
	pb.RegisterCombatServer(g, &Service{s: s, api: api.C{S: s}, loot: loot.NewTracker(), guid: guid, last: pb.CombatStatus{State: "idle"}})
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	switch p.GetOpcode() {
	case uint32(opcode.SMSGLootResponse):
		s.loot.OnPacket(p.GetPayload())
	case uint32(opcode.SMSGCastFailed):
		r := wire.NewReader(p.GetPayload())
		cnt, spell, res := r.U8(), r.U32(), r.U8()
		slog.Info("cast failed", "count", cnt, "spell", spell, "result", res)
		s.mu.Lock()
		s.castFailed = time.Now()
		s.mu.Unlock()
	}
	return &emptypb.Empty{}, nil
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

func (s *Service) current() *pb.CombatStatus {
	s.mu.Lock()
	j, last := s.cur, s.last
	s.mu.Unlock()
	if j != nil {
		return j.snap()
	}
	return &last
}

func (s *Service) Stop(context.Context, *emptypb.Empty) (*pb.CombatStatus, error) {
	s.stopCurrent()
	return s.current(), nil
}

func (s *Service) wait(ctx context.Context, waitS uint32, def uint32) *pb.CombatStatus {
	s.mu.Lock()
	j := s.cur
	s.mu.Unlock()
	if j != nil {
		w := waitS
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

func (s *Service) Status(ctx context.Context, r *pb.StatusRequest) (*pb.CombatStatus, error) {
	return s.wait(ctx, r.WaitS, 0), nil
}

func (s *Service) start(ctx context.Context, waitS uint32, body func(ctx context.Context, j *job) (string, string)) *pb.CombatStatus {
	s.stopCurrent()
	jctx, cancel := context.WithCancel(context.Background())
	j := &job{cancel: cancel, done: make(chan struct{}), start: time.Now(), st: pb.CombatStatus{State: "running"}}
	s.mu.Lock()
	s.cur = j
	s.mu.Unlock()
	go func() {
		state, reason := body(jctx, j)
		if jctx.Err() != nil && state == "failed" {
			state, reason = "stopped", "cancelled"
		}
		j.update(func(st *pb.CombatStatus) {
			st.State, st.Reason, st.Doing = state, reason, ""
			st.ElapsedS = uint32(time.Since(j.start).Seconds())
		})
		st := *j.snap()
		s.mu.Lock()
		s.last, s.cur = st, nil
		s.mu.Unlock()
		close(j.done)
	}()
	return s.wait(ctx, waitS, 20)
}

// ---- helpers -------------------------------------------------------------

func pct(a, b uint32) uint32 {
	if b == 0 {
		return 100
	}
	return a * 100 / b
}

func best(known map[uint32]bool, ranks []uint32) uint32 {
	for _, id := range ranks {
		if known[id] {
			return id
		}
	}
	return 0
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

func (s *Service) cast(ctx context.Context, spell uint32, target uint64) error {
	s.mu.Lock()
	s.castCount++
	cnt := s.castCount
	s.mu.Unlock()
	w := &wire.Writer{}
	w.U8(cnt)
	w.U32(spell)
	w.U8(0)
	w.U32(0x2)
	w.PackedGUID(target)
	return moveio.Packet(ctx, s.s, uint32(opcode.CMSGCastSpell), w.B)
}

func (s *Service) face(ctx context.Context, me *statepb.SelfInfo, tx, ty float32) float32 {
	o := math.Atan2(float64(ty-me.Y), float64(tx-me.X))
	if o < 0 {
		o += 2 * math.Pi
	}
	_ = moveio.Send(ctx, s.s, s.guid, uint32(opcode.MSGMoveSetFacing), 0, me.X, me.Y, me.Z, float32(o))
	_ = s.api.SetPos(ctx, me.X, me.Y, me.Z, float32(o))
	return float32(o)
}

func (s *Service) packetU64(ctx context.Context, op uint32, v uint64) {
	w := &wire.Writer{}
	w.U64(v)
	_ = moveio.Packet(ctx, s.s, op, w.B)
}

func (s *Service) stand(ctx context.Context, state uint32) {
	w := &wire.Writer{}
	w.U32(state)
	_ = moveio.Packet(ctx, s.s, uint32(opcode.CMSGStandstatechange), w.B)
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// ---- jobs ----------------------------------------------------------------

func (s *Service) Kill(ctx context.Context, r *pb.KillRequest) (*pb.CombatStatus, error) {
	return s.start(ctx, r.WaitS, func(ctx context.Context, j *job) (string, string) { return s.kill(ctx, j, r) }), nil
}

func (s *Service) Rest(ctx context.Context, r *pb.RestRequest) (*pb.CombatStatus, error) {
	return s.start(ctx, r.WaitS, func(ctx context.Context, j *job) (string, string) {
		return s.rest(ctx, j, 95, 85, 120)
	}), nil
}

func (s *Service) refresh(ctx context.Context, j *job) (*statepb.SnapshotResponse, error) {
	snap, err := s.api.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	if me := snap.Self; me != nil {
		j.update(func(st *pb.CombatStatus) {
			st.HpPct = pct(me.Hp, me.MaxHp)
			st.ManaPct = pct(me.Power, me.MaxPower)
			st.Coin = uint64(me.Money)
		})
	}
	return snap, nil
}

func (s *Service) rest(ctx context.Context, j *job, hpTarget, manaTarget uint32, maxS int) (string, string) {
	j.update(func(st *pb.CombatStatus) { st.Doing = "resting" })
	s.stand(ctx, 1)
	defer s.stand(context.Background(), 0)
	startHP := uint32(0)
	for i := 0; i < maxS; i++ {
		snap, err := s.refresh(ctx, j)
		if err != nil || snap.Self == nil {
			return "failed", "state unavailable"
		}
		me := snap.Self
		if me.Dead {
			return "failed", "dead"
		}
		hp := pct(me.Hp, me.MaxHp)
		if i == 0 {
			startHP = hp
		}
		if hp+5 < startHP {
			return "done", "attacked while resting"
		}
		startHP = max(startHP, hp)
		if hp >= hpTarget && (me.PowerType != 0 || pct(me.Power, me.MaxPower) >= manaTarget) {
			return "done", ""
		}
		if !sleep(ctx, time.Second) {
			return "stopped", "cancelled"
		}
	}
	return "done", "rest timeout"
}

func (s *Service) pickTarget(ctx context.Context, snap *statepb.SnapshotResponse, entry uint32, radius float32) (*statepb.Obj, error) {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Entry: entry, Radius: radius, Max: 8, AliveOnly: true})
	if err != nil {
		return nil, err
	}
	var fallback *statepb.Obj
	for _, o := range r.Objs {
		if o.Target == 0 || o.Target == snap.Self.Guid {
			return o, nil
		}
		if fallback == nil {
			fallback = o
		}
	}
	return fallback, nil
}

func (s *Service) aggressor(ctx context.Context, me *statepb.SelfInfo) *statepb.Obj {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 30, Max: 6, AliveOnly: true})
	if err != nil {
		return nil
	}
	for _, o := range r.Objs {
		if o.Target == me.Guid {
			return o
		}
	}
	return nil
}

func (s *Service) kill(ctx context.Context, j *job, r *pb.KillRequest) (string, string) {
	radius := r.Radius
	if radius <= 0 {
		radius = 80
	}
	maxS := time.Duration(r.MaxS) * time.Second
	if maxS == 0 {
		maxS = 600 * time.Second
	}
	deadline := time.Now().Add(maxS)
	var kills uint32
	noTarget := 0
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "stopped", "cancelled"
		}
		snap, err := s.refresh(ctx, j)
		if err != nil || snap.Self == nil {
			return "failed", "state unavailable"
		}
		if snap.Self.Dead {
			return "failed", "dead"
		}
		items := uint32(0)
		if r.ItemEntry != 0 {
			items = itemCount(snap, r.ItemEntry)
		}
		j.update(func(st *pb.CombatStatus) { st.Kills, st.Items = kills, items })
		if (r.Count > 0 && kills >= r.Count && (r.ItemEntry == 0 || items >= r.ItemCount)) ||
			(r.Count == 0 && r.ItemEntry != 0 && items >= r.ItemCount) {
			return "done", ""
		}
		if a := s.aggressor(ctx, snap.Self); a != nil {
			j.update(func(st *pb.CombatStatus) { st.Doing = fmt.Sprintf("defending vs %d", a.Entry) })
			dead, why := s.fight(ctx, j, a.Guid)
			if why != "" {
				return "failed", why
			}
			if dead {
				if a.Entry == r.Entry {
					kills++
				}
				if !r.NoLoot {
					s.lootCorpse(ctx, j, a.Guid)
				}
			}
			continue
		}
		if !r.NoLoot && api.FreeSlots(snap) < 1 {
			return "failed", "bags full"
		}
		me := snap.Self
		if pct(me.Hp, me.MaxHp) < 55 || (me.PowerType == 0 && pct(me.Power, me.MaxPower) < 25) {
			if st, why := s.rest(ctx, j, 90, 70, 90); st != "done" {
				return st, why
			}
			continue
		}
		t, err := s.pickTarget(ctx, snap, r.Entry, radius)
		if err != nil {
			return "failed", err.Error()
		}
		if t == nil {
			if noTarget++; noTarget > 6 {
				return "failed", fmt.Sprintf("no live entry %d within %.0f yards", r.Entry, radius)
			}
			if !sleep(ctx, 3*time.Second) {
				return "stopped", "cancelled"
			}
			continue
		}
		noTarget = 0
		j.update(func(st *pb.CombatStatus) { st.Doing = fmt.Sprintf("fighting %d", t.Entry) })
		dead, why := s.fight(ctx, j, t.Guid)
		if why != "" {
			return "failed", why
		}
		if dead {
			kills++
			if !r.NoLoot {
				s.lootCorpse(ctx, j, t.Guid)
			}
		}
	}
	return "failed", "time limit"
}

// fight returns whether the target died; a non-empty reason aborts the job.
func (s *Service) fight(ctx context.Context, j *job, guid uint64) (bool, string) {
	var lastCast, lastSwing time.Time
	selected := false
	fails := 0
	meleeUntil := time.Time{}
	started := time.Now()
	for time.Since(started) < 120*time.Second {
		if ctx.Err() != nil {
			return false, "cancelled"
		}
		t, err := s.api.Obj(ctx, guid)
		if err != nil {
			return true, "" // despawned
		}
		if t.Hp == 0 && t.MaxHp > 0 {
			return true, ""
		}
		snap, err := s.refresh(ctx, j)
		if err != nil || snap.Self == nil {
			return false, "state unavailable"
		}
		me := snap.Self
		if me.Dead {
			return false, "dead"
		}
		known, _ := s.api.Spells(ctx)
		spell := best(known, smite)
		mana := pct(me.Power, me.MaxPower)
		dist := float32(math.Hypot(float64(t.X-me.X), float64(t.Y-me.Y)))
		useMelee := spell == 0 || mana < 8 || time.Now().Before(meleeUntil)
		want := float32(castRange - 2)
		if useMelee {
			want = meleeRange - 1
		}
		if dist > want+2 {
			st, err := s.api.Goto(ctx, t.X, t.Y, t.Z, want, 3)
			if err != nil || st.State == "failed" {
				return false, fmt.Sprintf("cannot approach target: %v %s", err, st.GetReason())
			}
			continue
		}
		if !selected {
			s.packetU64(ctx, uint32(opcode.CMSGSetSelection), guid)
			selected = true
		}
		s.face(ctx, me, t.X, t.Y)
		if pct(me.Hp, me.MaxHp) < 40 && mana > 30 {
			if h := best(known, lesserHeal); h != 0 && time.Since(lastCast) > 2500*time.Millisecond {
				_ = s.cast(ctx, h, me.Guid)
				lastCast = time.Now()
				sleep(ctx, 2200*time.Millisecond)
				continue
			}
		}
		if useMelee {
			if time.Since(lastSwing) > 3*time.Second {
				s.packetU64(ctx, uint32(opcode.CMSGAttackswing), guid)
				lastSwing = time.Now()
			}
		} else if time.Since(lastCast) > 2300*time.Millisecond {
			before := me.Power
			_ = s.cast(ctx, spell, guid)
			lastCast = time.Now()
			sleep(ctx, 400*time.Millisecond)
			s.mu.Lock()
			failed := time.Since(s.castFailed) < 800*time.Millisecond
			s.mu.Unlock()
			if failed {
				if fails++; fails >= 3 {
					meleeUntil = time.Now().Add(8 * time.Second)
					fails = 0
				}
				lastCast = time.Now().Add(-1500 * time.Millisecond)
			} else {
				fails = 0
			}
			_ = before
		}
		if !sleep(ctx, 300*time.Millisecond) {
			return false, "cancelled"
		}
	}
	return false, "fight timeout"
}

func (s *Service) lootCorpse(ctx context.Context, j *job, guid uint64) {
	var t *statepb.Obj
	for i := 0; i < 10; i++ {
		var err error
		if t, err = s.api.Obj(ctx, guid); err != nil {
			return
		}
		if t.DynFlags&1 != 0 {
			break
		}
		sleep(ctx, 200*time.Millisecond)
	}
	if t.DynFlags&1 == 0 {
		slog.Info("corpse not lootable", "entry", t.Entry, "dynFlags", t.DynFlags)
		return
	}
	j.update(func(st *pb.CombatStatus) { st.Doing = "looting" })
	if st, err := s.api.GotoWait(ctx, t.X, t.Y, t.Z, 2.5, 20); err != nil || st.State == "failed" {
		slog.Info("cannot reach corpse", "entry", t.Entry, "err", err)
		return
	}
	for i := 0; i < 2; i++ {
		res, err := s.loot.Take(ctx, s.s, guid, true)
		if err == nil {
			slog.Info("looted", "entry", t.Entry, "gold", res.Gold, "items", res.Items)
			return
		}
		slog.Info("loot failed", "entry", t.Entry, "err", err)
		sleep(ctx, 500*time.Millisecond)
	}
}
