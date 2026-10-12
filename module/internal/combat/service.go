// Package combat hunts creatures, fights with the priority table of the class and loots.
package combat

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
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
	"wowagent/internal/xpscore"
)

const (
	castRange  = 26
	meleeRange = 4.5
	// defendMax bounds combat.defend (attackers keep coming, or one cannot be reached).
	defendMax = 3 * time.Minute
	// SpellCastResult 30 (EQUIPPED_ITEM_CLASS): the spell needs a usable weapon (missing or broken).
	castFailedNoWeapon = 30
	maxWait            = 25
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
	failures  map[uint8]castFailure
	rotations map[uint32]*classTables // class -> priority tables (journal/classes), nil = none
	fights    *xpscore.File           // learned fights (journal/learn): the pull capacity
	melee     *meleeLog
	abandoned map[uint64]time.Time // targets given up (no damage dealt): not picked again for a while
	crowded   map[uint64]time.Time // targets skipped for their adds: not pulled for a while, still fought back
	items     map[uint32]*worldpb.ItemInfo
	book      *spellBook
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	guid, _ := strconv.ParseUint(os.Getenv("AGENT_WOW_CHARACTER_GUID"), 10, 64)
	runner := jobs.NewRunner(&pb.CombatStatus{State: "idle"})
	runner.Decorate = func(j *job, st *pb.CombatStatus) { st.ElapsedS = uint32(time.Since(j.Start).Seconds()) }
	pb.RegisterCombatServer(g, &Service{s: s, api: api.C{S: s}, loot: loot.NewTracker(), guid: guid, jobs: runner, failures: map[uint8]castFailure{},
		rotations: map[uint32]*classTables{}, melee: newMeleeLog(), abandoned: map[uint64]time.Time{}, crowded: map[uint64]time.Time{}, items: map[uint32]*worldpb.ItemInfo{}, book: &spellBook{ready: map[uint32]time.Time{}},
		fights: xpscore.LearnedFile()})
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
	case uint32(opcode.SMSGSpellCooldown):
		s.book.onCooldown(p.GetPayload(), s.guid)
	case uint32(opcode.SMSGClearCooldown):
		s.book.onClear(p.GetPayload())
	case uint32(opcode.SMSGCastFailed):
		r := wire.NewReader(p.GetPayload())
		cnt, spell, res := r.U8(), r.U32(), r.U8()
		if cnt == 0 { // not one of our casts
			break
		}
		slog.Info("cast failed", "count", cnt, "spell", spell, "result", res)
		if res == castFailedNoWeapon {
			s.book.weaponRefused()
		}
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
	quiet, started := time.Now(), time.Now()
	for time.Since(quiet) < 4*time.Second {
		if time.Since(started) > defendMax {
			return "done", "defend time limit"
		}
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

func (s *Service) aggressor(ctx context.Context, me *statepb.SelfInfo) *statepb.Obj {
	r, err := s.api.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Radius: 30, Max: 12, AliveOnly: true})
	if err != nil {
		return nil
	}
	for _, o := range r.Objs {
		// Friendly NPCs may face/target us (gossip): only units in combat are attackers. A target given
		// up (unreachable: another level of a mine) is skipped, or defend fought it forever (bench of
		// 2026-10-12: 40 minutes against one kobold).
		if o.Target == me.Guid && o.UnitFlags&unitInCombat != 0 && !s.wasAbandoned(o.Guid) {
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
	if r.Guid != 0 && r.Count == 0 {
		r.Count = 1
	}
	var kills uint32
	noTarget, unreachable := 0, 0
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
		t, err := s.choosePull(ctx, j, snap.Self, r, radius)
		if err != nil {
			return "failed", err.Error()
		}
		if t == nil {
			if noTarget++; noTarget > 6 {
				return "failed", fmt.Sprintf("no live entry %d within %.0f yards (or only ones with too many monsters around)", r.Entry, radius)
			}
			if !sleep(ctx, 3*time.Second) {
				return "stopped", "cancelled"
			}
			continue
		}
		noTarget = 0
		j.Update(func(st *pb.CombatStatus) { st.Doing = fmt.Sprintf("fighting %d", t.Entry) })
		dead, why := s.fight(ctx, j, t.Guid)
		if strings.HasPrefix(why, "cannot approach target") {
			// On a roof, a ledge or across water: try another one.
			s.abandon(t.Guid, why, t.Entry)
			if unreachable++; unreachable > 4 {
				return "failed", why
			}
			continue
		}
		unreachable = 0
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
	if s.failedCast(cnt, spell) {
		return false
	}
	s.book.started(spell)
	return true
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

// forgetBuffs is called on death: buffs (and Weakened Soul) are gone, so are the waits between tries.
func (s *Service) forgetBuffs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.rotations {
		if t == nil {
			continue
		}
		for _, r := range []*rotation{t.fight, t.buffs} {
			for _, l := range r.lines {
				l.mark = time.Time{}
			}
		}
	}
}

// buff casts the buffs table of the class out of combat (Battle Shout, Fortitude, Inner Fire...),
// up to three spells in a row.
func (s *Service) buff(ctx context.Context, me *statepb.SelfInfo) {
	tab := s.tables(me.ClassId)
	if tab == nil || len(tab.buffs.lines) == 0 {
		return
	}
	known, err := s.api.Spells(ctx)
	if err != nil {
		return
	}
	tab.buffs.prepare(known)
	for i := 0; i < 3; i++ {
		e := &env{ctx: ctx, s: s, me: me, known: known, started: time.Now(), diesIn: 99}
		if cast, _ := s.step(ctx, tab.buffs, e, me.Guid); !cast {
			return
		}
		sleep(ctx, 1700*time.Millisecond) // global cooldown (Bloodrage: its rage comes in)
		b, err := s.api.Brief(ctx)
		if err != nil || b.Self == nil {
			return
		}
		me = b.Self
	}
}

// fight returns whether the target died; a non-empty reason aborts the job.
// Spells come from the priority table of the class (journal/classes/<class>.yaml); a caster out of
// mana uses its wand (Shoot) if one is equipped, else melee.
func (s *Service) fight(ctx context.Context, j *job, guid uint64) (bool, string) {
	rec := s.watchFight(ctx, guid)
	dead, why := s.fightClass(ctx, j, guid)
	rec.done(dead, why)
	return dead, why
}

func (s *Service) fightClass(ctx context.Context, j *job, guid uint64) (bool, string) {
	b, err := s.api.Brief(ctx)
	if err == nil && b.Self != nil && b.Self.ClassId == warrior.ClassID {
		return s.fightWarrior(ctx, j, guid)
	}
	var lastSwing, lastShoot, losAt, lastFace time.Time
	progress, lastHP := time.Now(), uint32(0)
	selected := false
	fails := 0
	meleeUntil := time.Time{}
	started := time.Now()
	known, err := s.api.Spells(ctx) // spells only change on level-up: read once per fight
	if err != nil {
		return false, "spells unavailable"
	}
	var rot *rotation
	if b != nil && b.Self != nil {
		rot = s.fightTable(b.Self.ClassId, known)
	}
	smite := best(known, priest.Smite)
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
		mana := pct(me.Power, me.MaxPower)
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
		if rot != nil {
			e := &env{ctx: ctx, s: s, me: me, t: t, dist: dist, known: known, started: started, diesIn: rot.observe(t),
				noEnemy: time.Now().Before(meleeUntil)}
			cast, refused := s.step(ctx, rot, e, guid)
			if cast {
				fails = 0
				continue
			}
			if refused {
				if fails++; fails >= 3 {
					meleeUntil = time.Now().Add(8 * time.Second) // e.g. line of sight: fight in melee for a while
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

// skipCrowded leaves a target alone for a while because of the monsters around it. Unlike abandon,
// it does not stop us from fighting it back when it attacks (bench of 2026-10-12 at the Fargodeep
// mine: crowded kobolds attacked and the character ate under their blows until it died).
func (s *Service) skipCrowded(guid uint64) {
	s.mu.Lock()
	s.crowded[guid] = time.Now()
	s.mu.Unlock()
}

func (s *Service) wasCrowded(guid uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.crowded[guid]) < 2*time.Minute
}

// noProgress is the time without any damage on the target: our hits (combat log) or its health dropping.
const noProgress = 25 * time.Second

// visible tells whether we see the target (move.los); unknown counts as visible.
func (s *Service) visible(ctx context.Context, t *statepb.Obj) bool {
	ok, err := s.api.LineOfSight(ctx, t.X, t.Y, t.Z)
	return err != nil || ok
}

// ---- warrior --------------------------------------------------------------

const inCombatFlag = 0x00080000 // UNIT_FLAG_IN_COMBAT on ourselves

// fightWarrior moves and swings; the spells come from the priority table of
// journal/classes/warrior.yaml (Charge to open, Victory Rush, Execute, Overpower, Rend, defensives,
// Heroic Strike with spare rage...).
func (s *Service) fightWarrior(ctx context.Context, j *job, guid uint64) (bool, string) {
	known, err := s.api.Spells(ctx)
	if err != nil {
		return false, "spells unavailable"
	}
	rot := s.fightTable(warrior.ClassID, known)
	var lastSwing, lastFace time.Time
	progress, lastHP := time.Now(), uint32(0)
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
		if !selected {
			s.packetU64(ctx, uint32(opcode.CMSGSetSelection), guid)
			selected = true
		}
		// Re-send our orientation when the server says we are not facing it, and every 2 s anyway.
		tooFar, badFacing := s.melee.recent(1500 * time.Millisecond)
		force := badFacing || now.Sub(lastFace) > 2*time.Second
		s.faceTo(ctx, me, t.X, t.Y, force)
		if force {
			lastFace = now
		}
		s.checkWeapon(ctx)
		// The table at any distance: Charge opens from afar, melee spells wait for the range check.
		if rot != nil {
			e := &env{ctx: ctx, s: s, me: me, t: t, dist: dist3, known: known, started: started, diesIn: rot.observe(t)}
			if cast, _ := s.step(ctx, rot, e, guid); cast {
				continue
			}
		}
		// The server refuses swings out of range: get closer than our own estimate says.
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
		if now.Sub(lastSwing) > 3*time.Second {
			s.packetU64(ctx, uint32(opcode.CMSGAttackswing), guid)
			lastSwing = now
		}
		if !sleep(ctx, 300*time.Millisecond) {
			return false, "cancelled"
		}
	}
	return false, "fight timeout"
}

// fightTable returns the fight table of a class with the ranks we know, or nil.
func (s *Service) fightTable(class uint32, known map[uint32]bool) *rotation {
	tab := s.tables(class)
	if tab == nil {
		return nil
	}
	tab.fight.prepare(known)
	tab.fight.target = 0
	return tab.fight
}
