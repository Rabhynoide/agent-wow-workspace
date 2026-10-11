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
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/api"
	"wowagent/internal/jobs"
	"wowagent/internal/loot"
	"wowagent/internal/moveio"
	"wowagent/internal/priest"
	"wowagent/internal/warrior"
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

	jobs      *jobs.Runner[*pb.CombatStatus]
	mu        sync.Mutex
	castCount uint8
	// failures by cast count (our CMSG_CAST_SPELL counter): SMSG_CAST_FAILED of other casts
	// (server-side or item spells, count 0) must not be taken for ours.
	failures   map[uint8]castFailure
	lastShield time.Time // Weakened Soul: no new shield for 15 s
	lastRenew  time.Time
	lastBuff   time.Time // Fortitude / Inner Fire (30 min); Battle Shout (2 min)
	lastKill   time.Time // Victory Rush is usable for 20 s after a killing blow
	lastCharge time.Time // Charge has a 15 s cooldown across fights
	melee      *meleeLog
	abandoned  map[uint64]time.Time // targets given up (no damage dealt): not picked again for a while
	items      map[uint32]*worldpb.ItemInfo
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	guid, _ := strconv.ParseUint(os.Getenv("AGENT_WOW_CHARACTER_GUID"), 10, 64)
	runner := jobs.NewRunner(&pb.CombatStatus{State: "idle"})
	runner.Decorate = func(j *job, st *pb.CombatStatus) { st.ElapsedS = uint32(time.Since(j.Start).Seconds()) }
	pb.RegisterCombatServer(g, &Service{s: s, api: api.C{S: s}, loot: loot.NewTracker(), guid: guid, jobs: runner, failures: map[uint8]castFailure{},
		melee: newMeleeLog(), abandoned: map[uint64]time.Time{}, items: map[uint32]*worldpb.ItemInfo{}})
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	switch p.GetOpcode() {
	case uint32(opcode.SMSGLootResponse):
		s.loot.OnPacket(p.GetPayload())
	case uint32(opcode.SMSGAttackswingNotinrange):
		s.melee.refusedRange()
	case uint32(opcode.SMSGAttackswingBadfacing):
		s.melee.refusedFacing()
	case uint32(opcode.SMSGAttackerstateupdate):
		s.melee.attackerState(s.guid, p.GetPayload())
	case uint32(opcode.SMSGCastFailed):
		r := wire.NewReader(p.GetPayload())
		cnt, spell, res := r.U8(), r.U32(), r.U8()
		if cnt == 0 { // not one of our casts
			break
		}
		slog.Info("cast failed", "count", cnt, "spell", spell, "result", res)
		s.mu.Lock()
		s.failures[cnt] = castFailure{spell: spell, at: time.Now()}
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

type castFailure struct {
	spell uint32
	at    time.Time
}

// cast sends CMSG_CAST_SPELL and returns its cast count (never 0: the server uses 0 for its own casts).
func (s *Service) cast(ctx context.Context, spell uint32, target uint64) (uint8, error) {
	s.mu.Lock()
	if s.castCount++; s.castCount == 0 {
		s.castCount = 1
	}
	cnt := s.castCount
	delete(s.failures, cnt)
	s.mu.Unlock()
	w := &wire.Writer{}
	w.U8(cnt)
	w.U32(spell)
	w.U8(0)
	w.U32(0x2)
	w.PackedGUID(target)
	return cnt, moveio.Packet(ctx, s.s, uint32(opcode.CMSGCastSpell), w.B)
}

// face turns towards the target when the orientation differs by more than ~6 degrees.
func (s *Service) face(ctx context.Context, me *statepb.SelfInfo, tx, ty float32) {
	s.faceTo(ctx, me, tx, ty, false)
}

// faceTo turns towards the target; force sends the orientation even when ours looks right (the
// server said we were not facing it).
func (s *Service) faceTo(ctx context.Context, me *statepb.SelfInfo, tx, ty float32, force bool) {
	o := math.Atan2(float64(ty-me.Y), float64(tx-me.X))
	if o < 0 {
		o += 2 * math.Pi
	}
	d := math.Abs(o - float64(me.O))
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	if d < 0.1 && !force {
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

func (s *Service) Defend(ctx context.Context, r *pb.StatusRequest) (*pb.CombatStatus, error) {
	return s.start(ctx, r.WaitS, s.defend), nil
}

// defend fights the units attacking us, one after the other, until none has for 4 seconds.
func (s *Service) defend(ctx context.Context, j *job) (string, string) {
	var kills uint32
	quiet := time.Now()
	for time.Since(quiet) < 4*time.Second {
		if ctx.Err() != nil {
			return "stopped", "cancelled"
		}
		snap, err := s.refresh(ctx, j)
		if err != nil || snap.Self == nil {
			return "failed", "state unavailable"
		}
		if snap.Self.Dead {
			s.forgetBuffs()
			return "failed", "dead"
		}
		a := s.aggressor(ctx, snap.Self)
		if a == nil {
			if !sleep(ctx, time.Second) {
				return "stopped", "cancelled"
			}
			continue
		}
		j.Update(func(st *pb.CombatStatus) { st.Doing = fmt.Sprintf("defending vs %d", a.Entry) })
		dead, why := s.fight(ctx, j, a.Guid)
		if why != "" {
			return "failed", why
		}
		if dead {
			kills++
			j.Update(func(st *pb.CombatStatus) { st.Kills = kills })
		}
		quiet = time.Now()
	}
	return "done", ""
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
	defer s.stand(context.Background(), 0)
	startHP := uint32(0)
	for i := 0; i < maxS; i++ {
		snap, err := s.refresh(ctx, j)
		if err != nil || snap.Self == nil {
			return "failed", "state unavailable"
		}
		me := snap.Self
		if me.Dead {
			s.forgetBuffs()
			return "failed", "dead"
		}
		hp := pct(me.Hp, me.MaxHp)
		if i == 0 {
			startHP = hp
			if s.eatAndDrink(ctx, me) {
				j.Update(func(st *pb.CombatStatus) { st.Doing = "eating / drinking" })
			} else {
				s.stand(ctx, 1)
			}
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
		if s.wasAbandoned(o.Guid) {
			continue
		}
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
			s.forgetBuffs()
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
		// Classes without a heal need more health before the next pull.
		restBelow := uint32(55)
		if me.ClassId == warrior.ClassID {
			restBelow = 75
		}
		if pct(me.Hp, me.MaxHp) < restBelow || (me.PowerType == 0 && pct(me.Power, me.MaxPower) < 25) {
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
	cnt, err := s.cast(ctx, spell, target)
	if err != nil {
		return false
	}
	sleep(ctx, 400*time.Millisecond)
	return !s.failedCast(cnt, spell)
}

// failedCast tells whether the server rejected our cast number cnt of this spell.
func (s *Service) failedCast(cnt uint8, spell uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.failures[cnt]
	return ok && f.spell == spell
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

// forgetBuffs is called on death: buffs (and Weakened Soul) are gone.
func (s *Service) forgetBuffs() {
	s.mu.Lock()
	s.lastBuff, s.lastShield, s.lastRenew = time.Time{}, time.Time{}, time.Time{}
	s.mu.Unlock()
}

// buff refreshes Power Word: Fortitude and Inner Fire every 25 minutes (out of combat).
func (s *Service) buff(ctx context.Context, me *statepb.SelfInfo) {
	if me.ClassId == warrior.ClassID {
		s.warriorBuff(ctx, me)
		return
	}
	if s.since(&s.lastBuff) < 25*time.Minute || pct(me.Power, me.MaxPower) < 40 {
		return
	}
	known, err := s.api.Spells(ctx)
	if err != nil {
		return
	}
	ok := true
	for _, ranks := range [][]uint32{priest.Fortitude, priest.InnerFire} {
		if id := best(known, ranks); id != 0 {
			ok = s.tryCast(ctx, id, me.Guid) && ok
			sleep(ctx, 1200*time.Millisecond) // global cooldown
		}
	}
	if ok { // a failed buff is retried before the next pull
		s.mark(&s.lastBuff)
	}
}

// fight returns whether the target died; a non-empty reason aborts the job.
// Priority: heal when low, shield, renew, Shadow Word: Pain once, Mind Blast, Smite;
// out of mana: wand (Shoot) if one is equipped, else melee.
func (s *Service) fight(ctx context.Context, j *job, guid uint64) (bool, string) {
	if b, err := s.api.Brief(ctx); err == nil && b.Self != nil && b.Self.ClassId == warrior.ClassID {
		dead, why := s.fightWarrior(ctx, j, guid)
		if dead {
			s.mark(&s.lastKill)
		}
		return dead, why
	}
	var busyUntil, lastSwing, lastShoot, lastBlast, painAt, losAt, lastFace time.Time
	progress, lastHP := time.Now(), uint32(0)
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
			s.forgetBuffs()
			return false, "dead"
		}
		hp, mana := pct(me.Hp, me.MaxHp), pct(me.Power, me.MaxPower)
		if lastHP == 0 || t.Hp < lastHP || s.melee.lastHit(guid).After(progress) {
			progress = time.Now()
		}
		lastHP = t.Hp
		if time.Since(progress) > noProgress {
			s.abandon(guid, "no damage dealt", t.Entry)
			return false, ""
		}
		dist := float32(math.Hypot(float64(t.X-me.X), float64(t.Y-me.Y)))
		outOfMana := smite == 0 || mana < 8 || time.Now().Before(meleeUntil)
		want := float32(castRange - 2)
		tooFar, badFacing := s.melee.recent(1500 * time.Millisecond)
		if outOfMana && shoot == 0 {
			want = meleeRange - 1
			if tooFar { // the server refused our swings: closer
				want = 1.5
				dist = want + 3
			}
		}
		if dist > want+2 {
			st, err := s.api.Goto(ctx, t.X, t.Y, t.Z, want, 3)
			if err != nil || st.State == "failed" {
				return false, fmt.Sprintf("cannot approach target: %v %s", err, st.GetReason())
			}
			continue
		}
		// Ranged spells need a clear view: when a wall or a ceiling is in the way, walk halfway to the
		// target (the path goes around the obstacle) and check again, instead of casting into it.
		if dist > meleeRange+1 && time.Since(losAt) > 700*time.Millisecond {
			losAt = time.Now()
			if !s.visible(ctx, t) {
				closer := max(meleeRange-1, dist/2)
				slog.Info("no line of sight, moving closer", "target", t.Entry, "dist", dist, "to", closer)
				st, err := s.api.Goto(ctx, t.X, t.Y, t.Z, closer, 3)
				if err != nil || st.State == "failed" {
					return false, fmt.Sprintf("cannot approach target: %v %s", err, st.GetReason())
				}
				continue
			}
		}
		if !selected {
			s.packetU64(ctx, uint32(opcode.CMSGSetSelection), guid)
			selected = true
		}
		force := badFacing || time.Since(lastFace) > 2*time.Second
		s.faceTo(ctx, me, t.X, t.Y, force)
		if force {
			lastFace = time.Now()
		}
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
					_, _ = s.cast(ctx, shoot, guid)
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

// abandon remembers a target we could not damage, so pickTarget leaves it alone for 2 minutes.
func (s *Service) abandon(guid uint64, why string, entry uint32) {
	slog.Info("giving up target", "entry", entry, "why", why)
	s.mu.Lock()
	s.abandoned[guid] = time.Now()
	s.mu.Unlock()
}

func (s *Service) wasAbandoned(guid uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.abandoned[guid]) < 2*time.Minute
}

// noProgress is the time without any damage on the target: our hits (combat log) or its health dropping.
const noProgress = 25 * time.Second

// visible tells whether we see the target (move.los); unknown counts as visible.
func (s *Service) visible(ctx context.Context, t *statepb.Obj) bool {
	ok, err := s.api.LineOfSight(ctx, t.X, t.Y, t.Z)
	return err != nil || ok
}

// ---- warrior --------------------------------------------------------------

const (
	battleShoutTime = 110 * time.Second // the shout lasts 2 minutes
	inCombatFlag    = 0x00080000        // UNIT_FLAG_IN_COMBAT on ourselves
	chargeMin       = 8                 // Charge range: 8 to 25 yards
	chargeMax       = 24
)

// warriorBuff keeps Battle Shout up between fights; Bloodrage gives the 10 rage it costs.
func (s *Service) warriorBuff(ctx context.Context, me *statepb.SelfInfo) {
	if s.since(&s.lastBuff) < battleShoutTime {
		return
	}
	known, err := s.api.Spells(ctx)
	if err != nil {
		return
	}
	shout := best(known, warrior.BattleShout)
	if shout == 0 {
		return
	}
	if me.Power < 100 { // rage is stored x10; Battle Shout costs 10
		br := best(known, warrior.Bloodrage)
		if br == 0 || !s.tryCast(ctx, br, me.Guid) {
			return // no rage out of combat: the shout is cast in the fight, with the first rage
		}
		sleep(ctx, 1500*time.Millisecond)
	}
	if s.tryCast(ctx, shout, me.Guid) {
		s.mark(&s.lastBuff)
	}
}

// fightWarrior follows journal/classes/warrior.yaml (rotation_solo): Charge to open, auto-attack,
// Victory Rush after a kill, Execute on a low target, Overpower when it fires, Rend on targets that
// live long enough, Mortal Strike / Slam once known, Thunder Clap at low level, Heroic Strike with
// spare rage.
func (s *Service) fightWarrior(ctx context.Context, j *job, guid uint64) (bool, string) {
	known, err := s.api.Spells(ctx)
	if err != nil {
		return false, "spells unavailable"
	}
	charge, rend, hs := best(known, warrior.Charge), best(known, warrior.Rend), best(known, warrior.HeroicStrike)
	tc, vr, op := best(known, warrior.ThunderClap), best(known, warrior.VictoryRush), best(known, warrior.Overpower)
	exe, ms, slam := best(known, warrior.Execute), best(known, warrior.MortalStrike), best(known, warrior.Slam)
	var gcdUntil, lastSwing, lastFace, rendAt, tcAt, opTry, vrTry, msAt, hsAt time.Time
	progress, lastHP := time.Now(), uint32(0)
	shout := best(known, warrior.BattleShout)
	selected := false
	started := time.Now()
	for time.Since(started) < 120*time.Second {
		if ctx.Err() != nil {
			return false, "cancelled"
		}
		t, err := s.api.Obj(ctx, guid)
		if status.Code(err) == codes.NotFound {
			return false, "" // out of sight: not a kill, pick another target
		}
		if err != nil {
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
			s.forgetBuffs()
			return false, "dead"
		}
		rage := me.Power / 10
		targetPct := pct(t.Hp, t.MaxHp)
		dist := float32(math.Hypot(float64(t.X-me.X), float64(t.Y-me.Y)))
		dist3 := float32(math.Sqrt(float64(dist*dist + (t.Z-me.Z)*(t.Z-me.Z))))
		now := time.Now()
		// Give up a target we do not damage (unreachable, evading, wrong position) instead of
		// standing there for the whole fight timeout.
		if lastHP == 0 || t.Hp < lastHP || s.melee.lastHit(guid).After(progress) {
			progress = now
		}
		lastHP = t.Hp
		if now.Sub(progress) > noProgress {
			s.abandon(guid, "no damage dealt", t.Entry)
			return false, ""
		}
		// Open with Charge (out of combat only): the server moves us next to the target.
		if charge != 0 && dist >= chargeMin+1 && dist <= chargeMax && me.UnitFlags&inCombatFlag == 0 && t.Target != me.Guid &&
			s.since(&s.lastCharge) > 15*time.Second && s.visible(ctx, t) {
			s.packetU64(ctx, uint32(opcode.CMSGSetSelection), guid)
			selected = true
			s.face(ctx, me, t.X, t.Y)
			s.mark(&s.lastCharge)
			if s.tryCast(ctx, charge, guid) {
				sleep(ctx, 800*time.Millisecond) // the charge itself
				continue
			}
		}
		// The server refuses swings out of range: get closer than our own estimate says.
		tooFar, badFacing := s.melee.recent(1500 * time.Millisecond)
		if dist3 > meleeRange || tooFar {
			radius := float32(meleeRange - 2)
			if tooFar {
				radius = 1.5
			}
			st, err := s.api.Goto(ctx, t.X, t.Y, t.Z, radius, 3)
			if err != nil || st.State == "failed" {
				return false, fmt.Sprintf("cannot approach target: %v %s", err, st.GetReason())
			}
			continue
		}
		if !selected {
			s.packetU64(ctx, uint32(opcode.CMSGSetSelection), guid)
			selected = true
		}
		// Re-send our orientation when the server says we are not facing it, and every 2 s anyway.
		force := badFacing || now.Sub(lastFace) > 2*time.Second
		s.faceTo(ctx, me, t.X, t.Y, force)
		if force {
			lastFace = now
		}
		if now.Sub(lastSwing) > 3*time.Second {
			s.packetU64(ctx, uint32(opcode.CMSGAttackswing), guid)
			lastSwing = now
		}
		if now.After(gcdUntil) {
			var spell uint32
			var mark *time.Time
			reactive := false // only usable after an event (dodge, kill): failures are expected
			switch {
			case shout != 0 && rage >= 10 && s.since(&s.lastBuff) > battleShoutTime:
				spell = shout
				s.mark(&s.lastBuff)
			case vr != 0 && s.since(&s.lastKill) < 18*time.Second && now.Sub(vrTry) > 2*time.Second:
				spell, mark, reactive = vr, &vrTry, true
			case exe != 0 && targetPct < 20 && rage >= 15:
				spell = exe
			case op != 0 && rage >= 5 && s.melee.dodged(guid, 4*time.Second) && now.Sub(opTry) > time.Second:
				spell, mark, reactive = op, &opTry, true // only after a dodge (combat log)
			case rend != 0 && rage >= 10 && targetPct > 50 && now.Sub(rendAt) > 15*time.Second:
				spell, mark = rend, &rendAt
			case ms != 0 && rage >= 30 && now.Sub(msAt) > 6*time.Second:
				spell, mark = ms, &msAt
			case slam != 0 && me.Level >= 41 && rage >= 15:
				spell = slam
			case tc != 0 && me.Level < 20 && rage >= 20 && now.Sub(tcAt) > 6*time.Second:
				spell, mark = tc, &tcAt
			}
			if spell != 0 {
				if mark != nil {
					*mark = now
				}
				if s.tryCast(ctx, spell, guid) {
					gcdUntil = time.Now().Add(1100 * time.Millisecond) // 1.5 s global cooldown minus the check
					if reactive && spell == vr {
						s.mu.Lock()
						s.lastKill = time.Time{} // used
						s.mu.Unlock()
					}
				} else if !reactive && mark != nil {
					*mark = time.Time{}
				}
				continue
			}
		}
		// Heroic Strike replaces the next swing (no global cooldown): spend spare rage.
		spare := uint32(30)
		if me.Level >= 41 {
			spare = 60 // Slam and Mortal Strike need the rage
		}
		if hs != 0 && rage >= spare && time.Since(hsAt) > 3500*time.Millisecond { // one per swing (2-3.6 s)
			_, _ = s.cast(ctx, hs, guid)
			hsAt = time.Now()
		}
		if !sleep(ctx, 300*time.Millisecond) {
			return false, "cancelled"
		}
	}
	return false, "fight timeout"
}
