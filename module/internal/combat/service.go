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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/combatv1"
	statepb "wowagent/gen/statev1"
	"wowagent/internal/api"
	"wowagent/internal/jobs"
	"wowagent/internal/loot"
	"wowagent/internal/moveio"
	"wowagent/internal/priest"
	"wowagent/internal/wire"
)

const (
	castRange  = 26
	meleeRange = 4.5
	maxWait    = 25
	// UNIT_FLAG_IN_COMBAT: only units in combat can be attacking us.
	unitInCombat = 0x00080000
)

type job = jobs.Job[*pb.CombatStatus]

type Service struct {
	pb.UnimplementedCombatServer
	s    modv1.SessionClient
	api  api.C
	loot *loot.Tracker
	guid uint64

	jobs       *jobs.Runner[*pb.CombatStatus]
	mu         sync.Mutex
	castCount  uint8
	castFailed time.Time
	lastShield time.Time // Weakened Soul: no new shield for 15 s
	lastRenew  time.Time
	lastBuff   time.Time // Fortitude / Inner Fire (30 min)
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	guid, _ := strconv.ParseUint(os.Getenv("AGENT_WOW_CHARACTER_GUID"), 10, 64)
	runner := jobs.NewRunner(&pb.CombatStatus{State: "idle"})
	runner.Decorate = func(j *job, st *pb.CombatStatus) { st.ElapsedS = uint32(time.Since(j.Start).Seconds()) }
	pb.RegisterCombatServer(g, &Service{s: s, api: api.C{S: s}, loot: loot.NewTracker(), guid: guid, jobs: runner})
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
	s.jobs.Stop()
	return &emptypb.Empty{}, nil
}

func (s *Service) Stop(context.Context, *emptypb.Empty) (*pb.CombatStatus, error) {
	s.jobs.Stop()
	return s.jobs.Current(), nil
}

func (s *Service) Status(ctx context.Context, r *pb.StatusRequest) (*pb.CombatStatus, error) {
	return s.jobs.Wait(ctx, r.WaitS, 0, maxWait), nil
}

func (s *Service) start(ctx context.Context, waitS uint32, body func(ctx context.Context, j *job) (string, string)) *pb.CombatStatus {
	s.jobs.Start(&pb.CombatStatus{State: "running"}, func(j *job) {
		state, reason := body(j.Ctx, j)
		if j.Ctx.Err() != nil && state == "failed" {
			state, reason = "stopped", "cancelled"
		}
		j.Update(func(st *pb.CombatStatus) {
			st.State, st.Reason, st.Doing = state, reason, ""
			st.ElapsedS = uint32(time.Since(j.Start).Seconds())
		})
	})
	return s.jobs.Wait(ctx, waitS, 20, maxWait)
}

// ---- helpers -------------------------------------------------------------

func pct(a, b uint32) uint32 {
	if b == 0 {
		return 100
	}
	return a * 100 / b
}

var best = priest.Best

func itemCount(items []*statepb.Item, entry uint32) uint32 {
	var n uint32
	for _, it := range items {
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

// face turns towards the target when the orientation differs by more than ~6 degrees.
func (s *Service) face(ctx context.Context, me *statepb.SelfInfo, tx, ty float32) {
	o := math.Atan2(float64(ty-me.Y), float64(tx-me.X))
	if o < 0 {
		o += 2 * math.Pi
	}
	d := math.Abs(o - float64(me.O))
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	if d < 0.1 {
		return
	}
	_ = moveio.Send(ctx, s.s, s.guid, uint32(opcode.MSGMoveSetFacing), 0, me.X, me.Y, me.Z, float32(o))
	_ = s.api.SetPos(ctx, me.X, me.Y, me.Z, float32(o))
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

var sleep = jobs.Sleep

// ---- jobs ----------------------------------------------------------------

func (s *Service) Kill(ctx context.Context, r *pb.KillRequest) (*pb.CombatStatus, error) {
	return s.start(ctx, r.WaitS, func(ctx context.Context, j *job) (string, string) { return s.kill(ctx, j, r) }), nil
}

func (s *Service) Rest(ctx context.Context, r *pb.RestRequest) (*pb.CombatStatus, error) {
	return s.start(ctx, r.WaitS, func(ctx context.Context, j *job) (string, string) {
		return s.rest(ctx, j, 95, 85, 120)
	}), nil
}

// refresh reads the light state (no item list) and updates the job's vitals.
func (s *Service) refresh(ctx context.Context, j *job) (*statepb.BriefResponse, error) {
	b, err := s.api.Brief(ctx)
	if err != nil {
		return nil, err
	}
	if me := b.Self; me != nil {
		j.Update(func(st *pb.CombatStatus) {
			st.HpPct = pct(me.Hp, me.MaxHp)
			st.ManaPct = pct(me.Power, me.MaxPower)
			st.Coin = uint64(me.Money)
		})
	}
	return b, nil
}

func (s *Service) rest(ctx context.Context, j *job, hpTarget, manaTarget uint32, maxS int) (string, string) {
	j.Update(func(st *pb.CombatStatus) { st.Doing = "resting" })
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

func (s *Service) pickTarget(ctx context.Context, me *statepb.SelfInfo, entry uint32, radius float32) (*statepb.Obj, error) {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Entry: entry, Radius: radius, Max: 8, AliveOnly: true})
	if err != nil {
		return nil, err
	}
	var fallback *statepb.Obj
	for _, o := range r.Objs {
		if o.Target == 0 || o.Target == me.Guid {
			return o, nil
		}
		if fallback == nil {
			fallback = o
		}
	}
	return fallback, nil
}

func (s *Service) aggressor(ctx context.Context, me *statepb.SelfInfo) *statepb.Obj {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 30, Max: 12, AliveOnly: true})
	if err != nil {
		return nil
	}
	for _, o := range r.Objs {
		// Friendly NPCs may face/target us (gossip): only units in combat are attackers.
		if o.Target == me.Guid && o.UnitFlags&unitInCombat != 0 {
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
			full, err := s.api.Snapshot(ctx)
			if err != nil {
				return "failed", "state unavailable"
			}
			items = itemCount(full.Items, r.ItemEntry)
		}
		j.Update(func(st *pb.CombatStatus) { st.Kills, st.Items = kills, items })
		if (r.Count > 0 && kills >= r.Count && (r.ItemEntry == 0 || items >= r.ItemCount)) ||
			(r.Count == 0 && r.ItemEntry != 0 && items >= r.ItemCount) {
			return "done", ""
		}
		if a := s.aggressor(ctx, snap.Self); a != nil {
			j.Update(func(st *pb.CombatStatus) { st.Doing = fmt.Sprintf("defending vs %d", a.Entry) })
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
		if !r.NoLoot && snap.FreeSlots < 1 {
			return "failed", "bags full"
		}
		me := snap.Self
		if pct(me.Hp, me.MaxHp) < 55 || (me.PowerType == 0 && pct(me.Power, me.MaxPower) < 25) {
			if st, why := s.rest(ctx, j, 90, 70, 90); st != "done" {
				return st, why
			}
			continue
		}
		s.buff(ctx, me)
		t, err := s.pickTarget(ctx, snap.Self, r.Entry, radius)
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
		j.Update(func(st *pb.CombatStatus) { st.Doing = fmt.Sprintf("fighting %d", t.Entry) })
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

// tryCast casts a spell and reports whether the server did not reject it right away.
func (s *Service) tryCast(ctx context.Context, spell uint32, target uint64) bool {
	_ = s.cast(ctx, spell, target)
	sleep(ctx, 400*time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.castFailed) >= 800*time.Millisecond
}

// since returns the time elapsed since a shared timestamp.
func (s *Service) since(t *time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(*t)
}

func (s *Service) mark(t *time.Time) {
	s.mu.Lock()
	*t = time.Now()
	s.mu.Unlock()
}

// buff refreshes Power Word: Fortitude and Inner Fire every 25 minutes (out of combat).
func (s *Service) buff(ctx context.Context, me *statepb.SelfInfo) {
	if s.since(&s.lastBuff) < 25*time.Minute || pct(me.Power, me.MaxPower) < 40 {
		return
	}
	known, err := s.api.Spells(ctx)
	if err != nil {
		return
	}
	for _, ranks := range [][]uint32{priest.Fortitude, priest.InnerFire} {
		if id := best(known, ranks); id != 0 {
			s.tryCast(ctx, id, me.Guid)
			sleep(ctx, 1200*time.Millisecond) // global cooldown
		}
	}
	s.mark(&s.lastBuff)
}

// fight returns whether the target died; a non-empty reason aborts the job.
// Priority: heal when low, shield, renew, Shadow Word: Pain once, Mind Blast, Smite;
// out of mana: wand (Shoot) if one is equipped, else melee.
func (s *Service) fight(ctx context.Context, j *job, guid uint64) (bool, string) {
	var busyUntil, lastSwing, lastShoot, lastBlast, painAt time.Time
	selected := false
	fails := 0
	meleeUntil := time.Time{}
	started := time.Now()
	known, err := s.api.Spells(ctx) // spells only change on level-up: read once per fight
	if err != nil {
		return false, "spells unavailable"
	}
	smite := best(known, priest.Smite)
	heal := best(known, priest.Heal)
	if heal == 0 {
		heal = best(known, priest.LesserHeal)
	}
	shield, renew := best(known, priest.Shield), best(known, priest.Renew)
	pain, blast := best(known, priest.Pain), best(known, priest.MindBlast)
	shoot := uint32(0)
	if snap, err := s.api.Snapshot(ctx); err == nil {
		for _, it := range snap.Items {
			if it.Bag == 255 && it.BagSlot == 17 { // ranged slot: priests can only use wands
				shoot = best(known, priest.Shoot)
			}
		}
	}
	for time.Since(started) < 120*time.Second {
		if ctx.Err() != nil {
			return false, "cancelled"
		}
		t, err := s.api.Obj(ctx, guid)
		if status.Code(err) == codes.NotFound {
			return false, "" // out of sight: not a kill, pick another target
		}
		if err != nil { // transient RPC error: retry
			if !sleep(ctx, 300*time.Millisecond) {
				return false, "cancelled"
			}
			continue
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
			s.mu.Lock()
			s.lastBuff = time.Time{} // buffs are lost on death
			s.mu.Unlock()
			return false, "dead"
		}
		hp, mana := pct(me.Hp, me.MaxHp), pct(me.Power, me.MaxPower)
		dist := float32(math.Hypot(float64(t.X-me.X), float64(t.Y-me.Y)))
		outOfMana := smite == 0 || mana < 8 || time.Now().Before(meleeUntil)
		want := float32(castRange - 2)
		if outOfMana && shoot == 0 {
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
		if time.Now().After(busyUntil) {
			// spell, target, time the cast keeps us busy (cast time or global cooldown)
			var spell uint32
			var target uint64
			busy := 1600 * time.Millisecond
			switch {
			case heal != 0 && hp < 40 && mana >= 12:
				spell, target, busy = heal, me.Guid, 3100*time.Millisecond
			case shield != 0 && hp < 80 && mana >= 15 && s.since(&s.lastShield) > 15*time.Second:
				spell, target = shield, me.Guid
				s.mark(&s.lastShield)
			case renew != 0 && hp < 65 && mana >= 15 && s.since(&s.lastRenew) > 15*time.Second:
				spell, target = renew, me.Guid
				s.mark(&s.lastRenew)
			case outOfMana:
			case pain != 0 && mana >= 20 && pct(t.Hp, t.MaxHp) > 30 && time.Since(painAt) > 18*time.Second:
				spell, target, painAt = pain, guid, time.Now()
			case blast != 0 && mana >= 30 && time.Since(lastBlast) > 8*time.Second:
				spell, target, lastBlast = blast, guid, time.Now()
			default:
				spell, target, busy = smite, guid, 2600*time.Millisecond
			}
			if spell != 0 {
				busyUntil = time.Now().Add(busy)
				if !s.tryCast(ctx, spell, target) {
					busyUntil = time.Now()
					if target == guid {
						if fails++; fails >= 3 {
							meleeUntil = time.Now().Add(8 * time.Second) // e.g. line of sight: fight in melee for a while
							fails = 0
						}
					}
				} else {
					fails = 0
				}
				continue
			}
		}
		if outOfMana {
			if shoot != 0 {
				if time.Since(lastShoot) > 5*time.Second {
					_ = s.cast(ctx, shoot, guid)
					lastShoot = time.Now()
				}
			} else if time.Since(lastSwing) > 3*time.Second {
				s.packetU64(ctx, uint32(opcode.CMSGAttackswing), guid)
				lastSwing = time.Now()
			}
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
	j.Update(func(st *pb.CombatStatus) { st.Doing = "looting" })
	if st, err := s.api.GotoWait(ctx, t.X, t.Y, t.Z, 2.5, 20); err != nil || st.State == "failed" {
		slog.Info("cannot reach corpse", "entry", t.Entry, "err", err)
		return
	}
	for i := 0; i < 2; i++ {
		res, err := s.loot.Take(ctx, s.s, guid, true, loot.DefaultWait)
		if err == nil {
			slog.Info("looted", "entry", t.Entry, "gold", res.Gold, "items", res.Items)
			return
		}
		slog.Info("loot failed", "entry", t.Entry, "err", err)
		sleep(ctx, 500*time.Millisecond)
	}
}
