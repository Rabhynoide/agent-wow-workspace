// Package api holds typed clients for the other modules.
package api

import (
	"context"
	"fmt"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	combatpb "wowagent/gen/combatv1"
	movepb "wowagent/gen/movev1"
	questpb "wowagent/gen/questv1"
	statepb "wowagent/gen/statev1"
	townpb "wowagent/gen/townv1"
	worldpb "wowagent/gen/worldv1"
	"wowagent/internal/invoke"
)

type C struct{ S modv1.SessionClient }

func (c C) Snapshot(ctx context.Context) (*statepb.SnapshotResponse, error) {
	var r statepb.SnapshotResponse
	err := invoke.Call(ctx, c.S, "state", "snapshot", &emptypb.Empty{}, &r)
	return &r, err
}

// Brief is the snapshot without the item list (self, quest log, free slots).
func (c C) Brief(ctx context.Context) (*statepb.BriefResponse, error) {
	var r statepb.BriefResponse
	err := invoke.Call(ctx, c.S, "state", "brief", &emptypb.Empty{}, &r)
	return &r, err
}

func (c C) Nearby(ctx context.Context, req *statepb.NearbyRequest) (*statepb.NearbyResponse, error) {
	var r statepb.NearbyResponse
	err := invoke.Call(ctx, c.S, "state", "nearby", req, &r)
	return &r, err
}

func (c C) Obj(ctx context.Context, guid uint64) (*statepb.Obj, error) {
	var r statepb.Obj
	err := invoke.Call(ctx, c.S, "state", "get_obj", &statepb.GetObjRequest{Guid: guid}, &r)
	return &r, err
}

func (c C) SetPos(ctx context.Context, x, y, z, o float32) error {
	return invoke.Call(ctx, c.S, "state", "set_pos", &statepb.SetPosRequest{X: x, Y: y, Z: z, O: o}, &emptypb.Empty{})
}

func (c C) Spells(ctx context.Context) (map[uint32]bool, error) {
	var r statepb.SpellsResponse
	if err := invoke.Call(ctx, c.S, "state", "spells", &emptypb.Empty{}, &r); err != nil {
		return nil, err
	}
	m := map[uint32]bool{}
	for _, id := range r.Spells {
		m[id] = true
	}
	return m, nil
}

func (c C) Goto(ctx context.Context, x, y, z, radius float32, waitS uint32) (*movepb.MoveStatus, error) {
	var r movepb.MoveStatus
	err := invoke.Call(ctx, c.S, "move", "goto", &movepb.GotoRequest{X: x, Y: y, Z: z, Radius: radius, WaitS: waitS}, &r)
	return &r, err
}

func (c C) MoveStatus(ctx context.Context, waitS uint32) (*movepb.MoveStatus, error) {
	var r movepb.MoveStatus
	err := invoke.Call(ctx, c.S, "move", "status", &movepb.StatusRequest{WaitS: waitS}, &r)
	return &r, err
}

func (c C) MoveStop(ctx context.Context) error {
	var r movepb.MoveStatus
	return invoke.Call(ctx, c.S, "move", "stop", &emptypb.Empty{}, &r)
}

// GotoWait walks to the point and blocks (polling) until arrival, failure or maxS seconds.
func (c C) GotoWait(ctx context.Context, x, y, z, radius float32, maxS int) (*movepb.MoveStatus, error) {
	st, err := c.Goto(ctx, x, y, z, radius, 10)
	for waited := 10; err == nil && st.State == "moving" && waited < maxS; waited += 10 {
		if ctx.Err() != nil {
			_ = c.MoveStop(context.Background())
			return st, ctx.Err()
		}
		st, err = c.MoveStatus(ctx, 10)
	}
	if err == nil && st.State == "moving" {
		_ = c.MoveStop(ctx)
		st.State, st.Reason = "failed", "timeout"
	}
	return st, err
}

// ---- world / quest / combat -------------------------------------------------

func (c C) QuestInfo(ctx context.Context, id uint32) (*worldpb.QuestInfo, error) {
	var r worldpb.QuestInfoResponse
	if err := invoke.Call(ctx, c.S, "world", "quest_info", &worldpb.QuestInfoRequest{Id: id}, &r); err != nil {
		return nil, err
	}
	if len(r.Quests) == 0 {
		return nil, fmt.Errorf("unknown quest %d", id)
	}
	return r.Quests[0], nil
}

func (c C) Spawns(ctx context.Context, entry, mapID uint32, x, y, radius float32, kind string) ([]*worldpb.Spawn, error) {
	var r worldpb.SpawnsResponse
	err := invoke.Call(ctx, c.S, "world", "spawns", &worldpb.SpawnsRequest{Entry: entry, Map: mapID, X: x, Y: y, Radius: radius, Max: 6, Kind: kind}, &r)
	return r.Spawns, err
}

func (c C) ItemSources(ctx context.Context, entry uint32) ([]*worldpb.ItemSource, error) {
	var r worldpb.ItemSourcesResponse
	err := invoke.Call(ctx, c.S, "world", "item_sources", &worldpb.ItemSourcesRequest{Entry: entry}, &r)
	return r.Sources, err
}

func (c C) Accept(ctx context.Context, npc, quest uint32) (*questpb.QuestResult, error) {
	var r questpb.QuestResult
	err := invoke.Call(ctx, c.S, "quest", "accept", &questpb.AcceptRequest{NpcEntry: npc, QuestId: quest}, &r)
	return &r, err
}

func (c C) TurnIn(ctx context.Context, npc, quest, reward uint32) (*questpb.QuestResult, error) {
	var r questpb.QuestResult
	err := invoke.Call(ctx, c.S, "quest", "turnin", &questpb.TurnInRequest{NpcEntry: npc, QuestId: quest, Reward: reward}, &r)
	return &r, err
}

func (c C) UseObject(ctx context.Context, entry uint32, loot bool) (*questpb.QuestResult, error) {
	var r questpb.QuestResult
	err := invoke.Call(ctx, c.S, "quest", "use", &questpb.UseRequest{Entry: entry, Loot: loot}, &r)
	return &r, err
}

func (c C) CombatKill(ctx context.Context, req *combatpb.KillRequest) (*combatpb.CombatStatus, error) {
	var r combatpb.CombatStatus
	err := invoke.Call(ctx, c.S, "combat", "kill", req, &r)
	return &r, err
}

func (c C) CombatRest(ctx context.Context) (*combatpb.CombatStatus, error) {
	var r combatpb.CombatStatus
	err := invoke.Call(ctx, c.S, "combat", "rest", &combatpb.RestRequest{WaitS: 20}, &r)
	return &r, err
}

func (c C) CombatStatus(ctx context.Context, waitS uint32) (*combatpb.CombatStatus, error) {
	var r combatpb.CombatStatus
	err := invoke.Call(ctx, c.S, "combat", "status", &combatpb.StatusRequest{WaitS: waitS}, &r)
	return &r, err
}

func (c C) CombatStop(ctx context.Context) error {
	var r combatpb.CombatStatus
	return invoke.Call(ctx, c.S, "combat", "stop", &emptypb.Empty{}, &r)
}

func (c C) FindVendor(ctx context.Context, mapID uint32, x, y float32) ([]*worldpb.Spawn, error) {
	var r worldpb.FindNpcResponse
	err := invoke.Call(ctx, c.S, "world", "find_npc", &worldpb.FindNpcRequest{MapFilter: true, Map: mapID, X: x, Y: y, Radius: 500, Max: 12, NpcFlags: 128}, &r)
	return r.Spawns, err
}

func (c C) ItemInfo(ctx context.Context, entry uint32) (*worldpb.ItemInfo, error) {
	var r worldpb.ItemInfoResponse
	if err := invoke.Call(ctx, c.S, "world", "item_info", &worldpb.ItemInfoRequest{Entry: entry}, &r); err != nil {
		return nil, err
	}
	if len(r.Items) == 0 {
		return nil, fmt.Errorf("unknown item %d", entry)
	}
	return r.Items[0], nil
}

func (c C) SellJunk(ctx context.Context) (*townpb.SellResult, error) {
	var r townpb.SellResult
	err := invoke.Call(ctx, c.S, "town", "sell_junk", &townpb.SellRequest{}, &r)
	return &r, err
}
