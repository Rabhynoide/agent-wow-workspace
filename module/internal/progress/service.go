// Package progress makes the character stronger: class trainer spells, talents and gear.
package progress

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/progressv1"
	statepb "wowagent/gen/statev1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/api"
	"wowagent/internal/dbc"
	"wowagent/internal/gear"
	"wowagent/internal/jobs"
	"wowagent/internal/moveio"
	"wowagent/internal/priest"
	"wowagent/internal/wire"
)

const (
	maxWait = 25
	// Talent points spent per call: each waits for the server, and a call must stay well under the
	// 30 s RPC timeout (auto calls again while points remain).
	maxTalentsPerCall = 10
)

type job = jobs.Job[*pb.TrainStatus]

type buyResult struct {
	spell uint32
	ok    bool
	why   uint32
}

type Service struct {
	pb.UnimplementedProgressServer
	s    modv1.SessionClient
	api  api.C
	jobs *jobs.Runner[*pb.TrainStatus]

	dbcDir   string
	dataOnce sync.Once
	data     *dbc.Data
	dataErr  error

	trainerList chan []trainerSpell
	bought      chan buyResult

	mu      sync.Mutex
	talents map[uint32]int    // talent id -> ranks learned (from SMSG_TALENTS_INFO)
	skip    map[uint32]uint32 // trainer spell -> level at which the trainer would not teach it
	items   map[uint32]*worldpb.ItemInfo
}

func Register(g grpc.ServiceRegistrar, s modv1.SessionClient) {
	dir := os.Getenv("DBC_DIR")
	if dir == "" {
		dir = "/dbc"
	}
	runner := jobs.NewRunner(&pb.TrainStatus{State: "idle"})
	runner.Decorate = func(j *job, st *pb.TrainStatus) { st.ElapsedS = uint32(time.Since(j.Start).Seconds()) }
	pb.RegisterProgressServer(g, &Service{s: s, api: api.C{S: s}, jobs: runner, dbcDir: dir,
		trainerList: make(chan []trainerSpell, 1), bought: make(chan buyResult, 8),
		talents: map[uint32]int{}, skip: map[uint32]uint32{}, items: map[uint32]*worldpb.ItemInfo{}})
}

// dbcData loads the client tables once (spell names, learn effects, talents).
func (s *Service) dbcData() (*dbc.Data, error) {
	s.dataOnce.Do(func() {
		s.data, s.dataErr = dbc.Load(s.dbcDir)
		if s.dataErr != nil {
			slog.Warn("dbc unavailable", "err", s.dataErr)
		}
	})
	return s.data, s.dataErr
}

func (s *Service) spellName(id uint32) string {
	if d, _ := s.dbcData(); d != nil {
		if sp := d.Spells[id]; sp != nil {
			if sp.Rank != "" {
				return sp.Name + " (" + sp.Rank + ")"
			}
			return sp.Name
		}
	}
	return strconv.Itoa(int(id))
}

// ---- packets ----------------------------------------------------------------

type trainerSpell struct {
	spell, cost, reqLevel uint32
	state                 uint8 // 0 available, 1 unavailable, 2 known
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	r := wire.NewReader(p.GetPayload())
	switch p.GetOpcode() {
	case uint32(opcode.SMSGTrainerList):
		r.U64()
		r.U32() // trainer type
		n := int(r.U32())
		var list []trainerSpell
		for i := 0; i < n && i < 1000 && !r.Err; i++ {
			ts := trainerSpell{spell: r.U32(), state: r.U8(), cost: r.U32()}
			r.Skip(8) // talent/profession point costs
			ts.reqLevel = uint32(r.U8())
			r.Skip(4 + 4 + 12) // skill line, skill rank, 3 required abilities
			list = append(list, ts)
		}
		if r.Err {
			slog.Warn("trainer list: truncated packet")
			return &emptypb.Empty{}, nil
		}
		select {
		case <-s.trainerList:
		default:
		}
		s.trainerList <- list
	case uint32(opcode.SMSGTrainerBuySucceeded), uint32(opcode.SMSGTrainerBuyFailed):
		r.U64()
		res := buyResult{spell: r.U32(), ok: p.GetOpcode() == uint32(opcode.SMSGTrainerBuySucceeded)}
		if !res.ok {
			res.why = r.U32()
		}
		select {
		case s.bought <- res:
		default:
		}
	case uint32(opcode.SMSGTalentsInfo):
		if r.U8() != 0 { // pet talents
			break
		}
		r.U32() // unspent points (also in the update fields)
		specs, active := int(r.U8()), int(r.U8())
		ranks := map[uint32]int{}
		for sp := 0; sp < specs && !r.Err; sp++ {
			n := int(r.U8())
			for i := 0; i < n && !r.Err; i++ {
				id, rank := r.U32(), int(r.U8())
				if sp == active {
					ranks[id] = rank + 1
				}
			}
			r.Skip(2 * int(r.U8())) // glyphs
		}
		if !r.Err {
			s.mu.Lock()
			s.talents = ranks
			s.mu.Unlock()
		}
	case uint32(opcode.SMSGInventoryChangeFailure):
		slog.Info("inventory change failed", "code", r.U8())
	}
	return &emptypb.Empty{}, nil
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	s.jobs.Stop()
	return &emptypb.Empty{}, nil
}

// ---- trainer ----------------------------------------------------------------

type plan struct {
	trainers     []*worldpb.ClassTrainer
	spells       []*worldpb.TrainerSpell // learnable now, cheapest first
	cost         uint32
	unaffordable uint32
}

// planTraining lists what the nearest class trainer should teach at this level (from the world DB).
func (s *Service) planTraining(ctx context.Context) (*statepb.SelfInfo, *plan, error) {
	b, err := s.api.Brief(ctx)
	if err != nil || b.Self == nil {
		return nil, nil, fmt.Errorf("state unavailable")
	}
	me := b.Self
	known, err := s.api.Spells(ctx)
	if err != nil {
		return nil, nil, err
	}
	trainers, err := s.api.ClassTrainers(ctx, me.ClassId, me.Map, me.X, me.Y)
	if err != nil {
		return nil, nil, err
	}
	p := &plan{trainers: trainers}
	if len(trainers) == 0 {
		return me, p, nil
	}
	d, _ := s.dbcData()
	s.mu.Lock()
	defer s.mu.Unlock()
	money := me.Money
	for _, ts := range trainers[0].Spells {
		if ts.ReqLevel > me.Level || s.skip[ts.Spell] == me.Level {
			continue
		}
		learned := []uint32{ts.Spell}
		if d != nil {
			if sp := d.Spells[ts.Spell]; sp != nil {
				learned = sp.Learned()
			}
			if !d.FitsRaceClass(ts.Spell, me.Race, me.ClassId) {
				continue
			}
		}
		all := true
		for _, id := range learned {
			all = all && known[id]
		}
		missing := false
		for _, a := range ts.ReqAbility {
			missing = missing || !known[a]
		}
		if all || missing {
			continue
		}
		if ts.Cost > money {
			p.unaffordable++
			continue
		}
		money -= ts.Cost
		p.cost += ts.Cost
		p.spells = append(p.spells, ts)
	}
	return me, p, nil
}

func (s *Service) TrainCheck(ctx context.Context, _ *emptypb.Empty) (*pb.TrainCheckResult, error) {
	_, p, err := s.planTraining(ctx)
	if err != nil {
		return &pb.TrainCheckResult{Reason: err.Error()}, nil
	}
	if len(p.trainers) == 0 {
		return &pb.TrainCheckResult{Reason: "no class trainer on this map"}, nil
	}
	t := p.trainers[0]
	res := &pb.TrainCheckResult{Count: uint32(len(p.spells)), Cost: p.cost, Trainer: t.Entry, TrainerName: t.Name, Dist: t.Dist, Unaffordable: p.unaffordable}
	for _, ts := range p.spells {
		res.Spells = append(res.Spells, s.spellName(ts.Spell))
	}
	return res, nil
}

func (s *Service) Train(ctx context.Context, r *pb.TrainRequest) (*pb.TrainStatus, error) {
	s.jobs.Start(&pb.TrainStatus{State: "running"}, func(j *job) {
		err := s.train(j)
		j.Update(func(st *pb.TrainStatus) {
			st.Doing = ""
			switch {
			case err == nil:
				st.State = "done"
			case j.Ctx.Err() != nil:
				st.State, st.Reason = "stopped", "cancelled"
			default:
				st.State, st.Reason = "failed", err.Error()
			}
		})
	})
	return s.jobs.Wait(ctx, r.WaitS, 20, maxWait), nil
}

func (s *Service) Status(ctx context.Context, r *pb.StatusRequest) (*pb.TrainStatus, error) {
	return s.jobs.Wait(ctx, r.WaitS, 0, maxWait), nil
}

func (s *Service) Stop(context.Context, *emptypb.Empty) (*pb.TrainStatus, error) {
	s.jobs.Stop()
	return s.jobs.Current(), nil
}

func (s *Service) train(j *job) error {
	ctx := j.Ctx
	me, p, err := s.planTraining(ctx)
	if err != nil {
		return err
	}
	if len(p.trainers) == 0 {
		return fmt.Errorf("no class trainer on this map")
	}
	var lastErr error
	for _, t := range p.trainers {
		j.Update(func(st *pb.TrainStatus) { st.Doing = fmt.Sprintf("going to %s (%.0f yards)", t.Name, t.Dist) })
		npc, err := s.api.ApproachNpc(ctx, t.Entry, t.X, t.Y, t.Z)
		if err == nil && (npc == nil || npc.Dist > 5) {
			err = fmt.Errorf("%s not reachable", t.Name)
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			continue
		}
		return s.buyAll(j, me, npc)
	}
	return lastErr
}

// buyAll opens the trainer window and buys every available spell the character can afford.
func (s *Service) buyAll(j *job, me *statepb.SelfInfo, npc *statepb.Obj) error {
	ctx := j.Ctx
	j.Update(func(st *pb.TrainStatus) { st.Doing = "reading trainer list" })
	select {
	case <-s.trainerList:
	default:
	}
	w := &wire.Writer{}
	w.U64(npc.Guid)
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGTrainerList), w.B); err != nil {
		return err
	}
	var list []trainerSpell
	select {
	case list = <-s.trainerList:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("no trainer list from %d", npc.Entry)
	case <-ctx.Done():
		return ctx.Err()
	}
	listed := map[uint32]bool{}
	var buy []trainerSpell
	s.mu.Lock()
	for _, ts := range list {
		listed[ts.spell] = true
		switch {
		case ts.state == 0:
			buy = append(buy, ts)
		case ts.state == 1 && ts.reqLevel <= me.Level:
			s.skip[ts.spell] = me.Level // missing rank/ability: not teachable at this level
		}
	}
	s.mu.Unlock()
	sort.Slice(buy, func(a, b int) bool { return buy[a].cost < buy[b].cost })
	money := me.Money
	for _, ts := range buy {
		if ts.cost > money {
			continue
		}
		j.Update(func(st *pb.TrainStatus) { st.Doing = "learning " + s.spellName(ts.spell) })
		for len(s.bought) > 0 {
			<-s.bought
		}
		bw := &wire.Writer{}
		bw.U64(npc.Guid)
		bw.U32(ts.spell)
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGTrainerBuySpell), bw.B); err != nil {
			return err
		}
		select {
		case res := <-s.bought:
			if res.ok {
				money -= ts.cost
				j.Update(func(st *pb.TrainStatus) {
					st.Learned = append(st.Learned, s.spellName(ts.spell))
					st.Spent += ts.cost
				})
			} else {
				slog.Info("trainer refused", "spell", ts.spell, "reason", res.why)
			}
		case <-time.After(3 * time.Second):
			slog.Info("no trainer answer", "spell", ts.spell)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// Spells of the DB list this trainer does not show (race/class filter): do not come back for them.
	if _, p, err := s.planTraining(ctx); err == nil {
		s.mu.Lock()
		for _, ts := range p.spells {
			if !listed[ts.Spell] {
				s.skip[ts.Spell] = me.Level
			}
		}
		s.mu.Unlock()
	}
	return nil
}

// ---- talents ----------------------------------------------------------------

// talentRank is the rank learned, from SMSG_TALENTS_INFO or the spell book (talents with spells).
func (s *Service) talentRank(t *dbc.Talent, known map[uint32]bool) int {
	s.mu.Lock()
	r := s.talents[t.ID]
	s.mu.Unlock()
	for i := t.MaxRank() - 1; i >= r; i-- {
		if known[t.Ranks[i]] {
			return i + 1
		}
	}
	return r
}

func (s *Service) Talents(ctx context.Context, r *pb.TalentsRequest) (*pb.TalentsResult, error) {
	b, err := s.api.Brief(ctx)
	if err != nil || b.Self == nil {
		return &pb.TalentsResult{Reason: "state unavailable"}, nil
	}
	me := b.Self
	res := &pb.TalentsResult{Ok: true, PointsLeft: me.TalentPoints}
	if me.TalentPoints == 0 {
		return res, nil
	}
	if me.ClassId != priest.ClassID {
		return &pb.TalentsResult{Reason: "no talent build for this class", PointsLeft: me.TalentPoints}, nil
	}
	d, err := s.dbcData()
	if err != nil {
		return &pb.TalentsResult{Reason: "dbc: " + err.Error(), PointsLeft: me.TalentPoints}, nil
	}
	known, err := s.api.Spells(ctx)
	if err != nil {
		return &pb.TalentsResult{Reason: err.Error(), PointsLeft: me.TalentPoints}, nil
	}
	planned := map[uint32]int{}
	for _, step := range priest.Build {
		t := d.TalentBySpellName(priest.ClassID, step.Name)
		if t == nil {
			return &pb.TalentsResult{Reason: "unknown talent " + step.Name, PointsLeft: res.PointsLeft}, nil
		}
		for {
			rank := max(s.talentRank(t, known), planned[t.ID])
			if rank >= step.Rank || res.PointsLeft == 0 || len(res.Learned) >= maxTalentsPerCall {
				break
			}
			if !r.DryRun {
				if err := s.learnTalent(ctx, t, rank); err != nil {
					res.Ok, res.Reason = false, err.Error()
					return res, nil
				}
				known, _ = s.api.Spells(ctx)
			}
			planned[t.ID] = rank + 1
			res.PointsLeft--
			res.Learned = append(res.Learned, fmt.Sprintf("%s %d", step.Name, rank+1))
		}
		if res.PointsLeft == 0 || len(res.Learned) >= maxTalentsPerCall {
			break
		}
	}
	return res, nil
}

// learnTalent asks for the next rank (0-based index = current rank) and waits for the server.
func (s *Service) learnTalent(ctx context.Context, t *dbc.Talent, rank int) error {
	w := &wire.Writer{}
	w.U32(t.ID)
	w.U32(uint32(rank))
	if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGLearnTalent), w.B); err != nil {
		return err
	}
	for i := 0; i < 20; i++ {
		if !jobs.Sleep(ctx, 100*time.Millisecond) {
			return ctx.Err()
		}
		s.mu.Lock()
		got := s.talents[t.ID]
		s.mu.Unlock()
		if got > rank {
			return nil
		}
		if known, err := s.api.Spells(ctx); err == nil && known[t.Ranks[rank]] {
			return nil
		}
	}
	return fmt.Errorf("server refused talent %d rank %d", t.ID, rank+1)
}

// ---- gear -------------------------------------------------------------------

func (s *Service) itemInfo(ctx context.Context, entry uint32) *worldpb.ItemInfo {
	s.mu.Lock()
	it, ok := s.items[entry]
	s.mu.Unlock()
	if ok {
		return it
	}
	it, err := s.api.ItemInfo(ctx, entry)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	s.items[entry] = it
	s.mu.Unlock()
	return it
}

type inventory struct {
	me       *statepb.SelfInfo
	known    map[uint32]bool
	equipped map[int]*worldpb.ItemInfo
	bags     []*statepb.Item
}

func (s *Service) inventory(ctx context.Context) (*inventory, error) {
	snap, err := s.api.Snapshot(ctx)
	if err != nil || snap.Self == nil {
		return nil, fmt.Errorf("state unavailable")
	}
	known, err := s.api.Spells(ctx)
	if err != nil {
		return nil, err
	}
	inv := &inventory{me: snap.Self, known: known, equipped: map[int]*worldpb.ItemInfo{}}
	for _, it := range snap.Items {
		switch {
		case it.Bag == 255 && it.BagSlot <= 18:
			inv.equipped[int(it.BagSlot)] = s.itemInfo(ctx, it.Entry)
		case it.Bag == 255 && it.BagSlot >= 23 && it.BagSlot <= 38, it.Bag >= 19 && it.Bag <= 22:
			inv.bags = append(inv.bags, it)
		}
	}
	return inv, nil
}

type upgrade struct {
	item *statepb.Item
	info *worldpb.ItemInfo
	slot int
	gain float32
}

// bestUpgrade picks the bag item that improves the score the most.
func (s *Service) bestUpgrade(ctx context.Context, inv *inventory) *upgrade {
	var best *upgrade
	for _, it := range inv.bags {
		info := s.itemInfo(ctx, it.Entry)
		if !gear.Usable(info, inv.me.ClassId, inv.me.Race, inv.me.Level, inv.known) {
			continue
		}
		slot, g := gear.Gain(info, inv.equipped)
		if slot >= 0 && g > 0.05 && (best == nil || g > best.gain) {
			best = &upgrade{it, info, slot, g}
		}
	}
	return best
}

func (s *Service) Gear(ctx context.Context, r *pb.GearRequest) (*pb.GearResult, error) {
	res := &pb.GearResult{Ok: true}
	for i := 0; i < 8; i++ {
		inv, err := s.inventory(ctx)
		if err != nil {
			return &pb.GearResult{Reason: err.Error(), Equipped: res.Equipped}, nil
		}
		up := s.bestUpgrade(ctx, inv)
		if up == nil {
			break
		}
		res.Equipped = append(res.Equipped, fmt.Sprintf("%s (+%.1f)", up.info.Name, up.gain))
		if r.DryRun {
			break // the next upgrade depends on this one
		}
		if err := s.equip(ctx, up); err != nil {
			res.Ok, res.Reason = false, err.Error()
			break
		}
	}
	return res, nil
}

// equip moves the item to its slot (explicit slot for rings/trinkets) and waits for the server.
func (s *Service) equip(ctx context.Context, up *upgrade) error {
	if len(gear.Slots(up.info.InventoryType)) > 1 {
		if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGSwapItem), []byte{255, byte(up.slot), byte(up.item.Bag), byte(up.item.BagSlot)}); err != nil {
			return err
		}
	} else if err := moveio.Packet(ctx, s.s, uint32(opcode.CMSGAutoequipItem), []byte{byte(up.item.Bag), byte(up.item.BagSlot)}); err != nil {
		return err
	}
	for i := 0; i < 20; i++ {
		if !jobs.Sleep(ctx, 100*time.Millisecond) {
			return ctx.Err()
		}
		snap, err := s.api.Snapshot(ctx)
		if err != nil {
			continue
		}
		for _, it := range snap.Items {
			if it.Guid == up.item.Guid && it.Bag == 255 && it.BagSlot <= 18 {
				return nil
			}
		}
	}
	return fmt.Errorf("server did not equip %s", up.info.Name)
}

// PickReward chooses the quest reward that upgrades the gear most, else the most valuable one.
func (s *Service) PickReward(ctx context.Context, r *pb.PickRewardRequest) (*pb.PickRewardResult, error) {
	if len(r.Items) == 0 {
		return &pb.PickRewardResult{Reason: "no choice"}, nil
	}
	inv, err := s.inventory(ctx)
	if err != nil {
		return &pb.PickRewardResult{Reason: err.Error()}, nil
	}
	best, bestGain, bestPrice := -1, float32(0), uint32(0)
	cheap := 0
	for i, entry := range r.Items {
		info := s.itemInfo(ctx, entry)
		if info == nil {
			continue
		}
		if info.SellPrice > bestPrice {
			cheap, bestPrice = i, info.SellPrice
		}
		if !gear.Usable(info, inv.me.ClassId, inv.me.Race, inv.me.Level, inv.known) {
			continue
		}
		if _, g := gear.Gain(info, inv.equipped); g > bestGain {
			best, bestGain = i, g
		}
	}
	if best >= 0 {
		return &pb.PickRewardResult{Index: uint32(best), Gain: bestGain, Reason: "upgrade"}, nil
	}
	return &pb.PickRewardResult{Index: uint32(cheap), Reason: "highest sell price"}, nil
}
