// Package api holds typed clients for the other modules.
package api

import (
	"context"
	"fmt"
	"time"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	combatpb "wowagent/gen/combatv1"
	movepb "wowagent/gen/movev1"
	progresspb "wowagent/gen/progressv1"
	questpb "wowagent/gen/questv1"
	statepb "wowagent/gen/statev1"
	townpb "wowagent/gen/townv1"
	travelpb "wowagent/gen/travelv1"
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

// LineOfSight asks move whether the static geometry leaves a clear view from us to the point.
func (c C) LineOfSight(ctx context.Context, x, y, z float32) (bool, error) {
	var r movepb.LosResult
	err := invoke.Call(ctx, c.S, "move", "los", &movepb.LosRequest{X: x, Y: y, Z: z}, &r)
	return r.Visible, err
}

// PathPoints is the walking path to (x, y, z), every 2 yards.
func (c C) PathPoints(ctx context.Context, x, y, z float32) (*movepb.PathResponse, error) {
	var r movepb.PathResponse
	err := invoke.Call(ctx, c.S, "move", "path", &movepb.PathRequest{X: x, Y: y, Z: z, WithPoints: true}, &r)
	return &r, err
}

// PathFrom asks move for the walk from (fx, fy, fz) to (x, y, z) on a map, with the areas crossed.
func (c C) PathFrom(ctx context.Context, mapID uint32, fx, fy, fz, x, y, z float32) (*movepb.PathResponse, error) {
	return c.PathFromSnap(ctx, mapID, fx, fy, fz, x, y, z, 0)
}

// PathFromSnap is PathFrom ending at the closest walkable point within snap yards of the destination.
func (c C) PathFromSnap(ctx context.Context, mapID uint32, fx, fy, fz, x, y, z, snap float32) (*movepb.PathResponse, error) {
	var r movepb.PathResponse
	err := invoke.Call(ctx, c.S, "move", "path", &movepb.PathRequest{X: x, Y: y, Z: z, FromSet: true, Map: mapID, FromX: fx, FromY: fy, FromZ: fz, WithAreas: true, Snap: snap}, &r)
	return &r, err
}

// PathFromSnapPoints is PathFromSnap with the path's points (the flight ways, combat/fleemap.go).
func (c C) PathFromSnapPoints(ctx context.Context, mapID uint32, fx, fy, fz, x, y, z, snap float32) (*movepb.PathResponse, error) {
	var r movepb.PathResponse
	err := invoke.Call(ctx, c.S, "move", "path", &movepb.PathRequest{X: x, Y: y, Z: z, FromSet: true, Map: mapID, FromX: fx, FromY: fy, FromZ: fz, Snap: snap, WithPoints: true}, &r)
	return &r, err
}

// Call invokes any module RPC.
func (c C) Call(ctx context.Context, module, method string, req, resp proto.Message) error {
	return invoke.Call(ctx, c.S, module, method, req, resp)
}

// ---- travel ----------------------------------------------------------------

// TravelPlan asks travel for the fastest journey to a point (walks, flights, portals, hearthstone).
func (c C) TravelPlan(ctx context.Context, mapID uint32, x, y, z float32) (*travelpb.PlanResponse, error) {
	var r travelpb.PlanResponse
	err := invoke.Call(ctx, c.S, "travel", "plan", &travelpb.PlanRequest{To: &travelpb.Point{Map: mapID, X: x, Y: y, Z: z}}, &r)
	return &r, err
}

// TravelDo runs a non-walk step: "fly" (nodes), "portal" (trigger, map), "hearth", "discover", "bind".
func (c C) TravelDo(ctx context.Context, kind string, nodes []uint32, trigger, mapID uint32) (*travelpb.TravelResult, error) {
	var r travelpb.TravelResult
	var err error
	switch kind {
	case "fly":
		err = invoke.Call(ctx, c.S, "travel", "fly", &travelpb.FlyRequest{Nodes: nodes}, &r)
	case "portal":
		err = invoke.Call(ctx, c.S, "travel", "trigger", &travelpb.TriggerRequest{Id: trigger, Map: mapID}, &r)
	case "hearth", "discover", "bind":
		err = invoke.Call(ctx, c.S, "travel", kind, &emptypb.Empty{}, &r)
	default:
		return nil, fmt.Errorf("unknown travel step %q", kind)
	}
	return &r, err
}

// Boat calls travel.board or travel.disembark (step): wait for the boat at dock, step aboard or onto pier.
func (c C) Boat(ctx context.Context, step string, transport uint32, dock, pier *travelpb.Point, maxWaitS uint32) (*travelpb.BoatStatus, error) {
	var r travelpb.BoatStatus
	err := invoke.Call(ctx, c.S, "travel", step, &travelpb.BoatRequest{Transport: transport, Dock: dock, Pier: pier, MaxWaitS: maxWaitS}, &r)
	return &r, err
}

// NearUnknownFlight finds a flight master within radius whose node we do not know.
func (c C) NearUnknownFlight(ctx context.Context, radius float32) (*travelpb.NearResponse, error) {
	var r travelpb.NearResponse
	err := invoke.Call(ctx, c.S, "travel", "near_unknown", &travelpb.NearRequest{Radius: radius}, &r)
	return &r, err
}

// SafePoint asks move for the point near (x, y, z) farthest from hostile monsters.
func (c C) SafePoint(ctx context.Context, x, y, z, maxDist float32, pathClear bool) (*movepb.SafePointResult, error) {
	var r movepb.SafePointResult
	err := invoke.Call(ctx, c.S, "move", "safe_point", &movepb.SafePointRequest{X: x, Y: y, Z: z, MaxDist: maxDist, PathClear: pathClear}, &r)
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

// Walk goes to the point and fails unless it arrives.
func (c C) Walk(ctx context.Context, x, y, z, radius float32) error {
	st, err := c.GotoWait(ctx, x, y, z, radius, 240)
	if err != nil {
		return err
	}
	if st.State != "arrived" {
		return fmt.Errorf("walk to (%.0f,%.0f): %s %s", x, y, st.State, st.Reason)
	}
	return nil
}

// ApproachNpc walks to the spawn point, then follows the NPC (entry) if it wandered off,
// until it is within 4 yards. It returns the NPC when visible.
func (c C) ApproachNpc(ctx context.Context, entry uint32, x, y, z float32) (*statepb.Obj, error) {
	if err := c.Walk(ctx, x, y, z, 3); err != nil {
		return nil, err
	}
	var o *statepb.Obj
	for i := 0; i < 4; i++ {
		vis, err := c.Nearby(ctx, &statepb.NearbyRequest{Kind: "unit", Entry: entry, Radius: 80, Max: 1})
		if err != nil || len(vis.Objs) == 0 {
			return o, nil
		}
		if o = vis.Objs[0]; o.Dist <= 4 {
			return o, nil
		}
		if err := c.Walk(ctx, o.X, o.Y, o.Z, 2.5); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return o, nil
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

// SpawnsAll is every creature spawn within radius (entry ignored), with movement type and wander radius.
func (c C) SpawnsAll(ctx context.Context, mapID uint32, x, y, radius float32) ([]*worldpb.Spawn, error) {
	var r worldpb.SpawnsResponse
	err := invoke.Call(ctx, c.S, "world", "spawns", &worldpb.SpawnsRequest{Map: mapID, X: x, Y: y, Radius: radius, Max: 1000, All: true}, &r)
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

func (c C) Explore(ctx context.Context, trigger uint32) (*questpb.QuestResult, error) {
	var r questpb.QuestResult
	err := invoke.Call(ctx, c.S, "quest", "explore", &questpb.ExploreRequest{Trigger: trigger}, &r)
	return &r, err
}

func (c C) UseItem(ctx context.Context, item, spell, target uint32, kind string) (*questpb.QuestResult, error) {
	var r questpb.QuestResult
	err := invoke.Call(ctx, c.S, "quest", "use_item", &questpb.UseItemRequest{ItemEntry: item, Spell: spell, TargetEntry: target, TargetKind: kind}, &r)
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

func (c C) CombatDefend(ctx context.Context, waitS uint32) (*combatpb.CombatStatus, error) {
	var r combatpb.CombatStatus
	err := invoke.Call(ctx, c.S, "combat", "defend", &combatpb.StatusRequest{WaitS: waitS}, &r)
	return &r, err
}

func (c C) CombatStop(ctx context.Context) error {
	var r combatpb.CombatStatus
	return invoke.Call(ctx, c.S, "combat", "stop", &emptypb.Empty{}, &r)
}

// FindVendor lists the nearest NPCs with any of the npc flags (128 vendor, 4096 repair).
func (c C) FindVendor(ctx context.Context, mapID uint32, x, y float32, flags uint32) ([]*worldpb.Spawn, error) {
	var r worldpb.FindNpcResponse
	err := invoke.Call(ctx, c.S, "world", "find_npc", &worldpb.FindNpcRequest{MapFilter: true, Map: mapID, X: x, Y: y, Radius: 500, Max: 12, NpcFlags: flags}, &r)
	return r.Spawns, err
}

// FindFoodVendor lists the vendors within 500 yards selling food or drink (spell category) usable at level.
func (c C) FindFoodVendor(ctx context.Context, mapID uint32, x, y float32, category, level uint32) ([]*worldpb.Spawn, error) {
	var r worldpb.FindNpcResponse
	err := invoke.Call(ctx, c.S, "world", "find_npc", &worldpb.FindNpcRequest{MapFilter: true, Map: mapID, X: x, Y: y, Radius: 500, Max: 6,
		NpcFlags: 128, SellsSpellCategory: category, SellsMaxLevel: level}, &r)
	return r.Spawns, err
}

func (c C) Restock(ctx context.Context, food, drink, potions uint32) (*townpb.RestockResult, error) {
	var r townpb.RestockResult
	err := invoke.Call(ctx, c.S, "town", "restock", &townpb.RestockRequest{Food: food, Drink: drink, Potions: potions}, &r)
	return &r, err
}

// Creature returns a creature template (world.creature).
func (c C) Creature(ctx context.Context, entry uint32) (*worldpb.CreatureInfo, error) {
	var r worldpb.CreatureInfo
	if err := invoke.Call(ctx, c.S, "world", "creature", &worldpb.CreatureInfoRequest{Entry: entry}, &r); err != nil {
		return nil, err
	}
	return &r, nil
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

func (c C) ClassTrainers(ctx context.Context, class, mapID uint32, x, y float32) ([]*worldpb.ClassTrainer, error) {
	var r worldpb.ClassTrainersResponse
	err := invoke.Call(ctx, c.S, "world", "class_trainers", &worldpb.ClassTrainersRequest{ClassId: class, Map: mapID, X: x, Y: y}, &r)
	return r.Trainers, err
}

// ---- progress ---------------------------------------------------------------

func (c C) TrainCheck(ctx context.Context) (*progresspb.TrainCheckResult, error) {
	var r progresspb.TrainCheckResult
	err := invoke.Call(ctx, c.S, "progress", "train_check", &emptypb.Empty{}, &r)
	return &r, err
}

func (c C) Train(ctx context.Context, waitS uint32) (*progresspb.TrainStatus, error) {
	var r progresspb.TrainStatus
	err := invoke.Call(ctx, c.S, "progress", "train", &progresspb.TrainRequest{WaitS: waitS}, &r)
	return &r, err
}

func (c C) TrainStatus(ctx context.Context, waitS uint32) (*progresspb.TrainStatus, error) {
	var r progresspb.TrainStatus
	err := invoke.Call(ctx, c.S, "progress", "status", &progresspb.StatusRequest{WaitS: waitS}, &r)
	return &r, err
}

func (c C) TrainStop(ctx context.Context) error {
	var r progresspb.TrainStatus
	return invoke.Call(ctx, c.S, "progress", "stop", &emptypb.Empty{}, &r)
}

func (c C) Talents(ctx context.Context) (*progresspb.TalentsResult, error) {
	var r progresspb.TalentsResult
	err := invoke.Call(ctx, c.S, "progress", "talents", &progresspb.TalentsRequest{}, &r)
	return &r, err
}

func (c C) Gear(ctx context.Context) (*progresspb.GearResult, error) {
	var r progresspb.GearResult
	err := invoke.Call(ctx, c.S, "progress", "gear", &progresspb.GearRequest{}, &r)
	return &r, err
}

// BestPurchase picks the best gear upgrade among items for sale (prices in copper, at most maxCopper).
func (c C) BestPurchase(ctx context.Context, items, prices []uint32, maxCopper uint32) (*progresspb.PickRewardResult, error) {
	var r progresspb.PickRewardResult
	err := invoke.Call(ctx, c.S, "progress", "pick_reward", &progresspb.PickRewardRequest{Items: items, Prices: prices, MaxCopper: maxCopper}, &r)
	return &r, err
}

// Vendors lists the vendors spawned within radius of (x, y) on a map, nearest first.
func (c C) Vendors(ctx context.Context, mapID uint32, x, y, radius float32) ([]*worldpb.Spawn, error) {
	var r worldpb.FindNpcResponse
	err := invoke.Call(ctx, c.S, "world", "find_npc", &worldpb.FindNpcRequest{MapFilter: true, Map: mapID, X: x, Y: y, Radius: radius, Max: 40, NpcFlags: 128}, &r)
	return r.Spawns, err
}

// VendorItems lists the weapons and armor a vendor (creature entry) sells.
func (c C) VendorItems(ctx context.Context, entry uint32) ([]*worldpb.VendorItem, error) {
	var r worldpb.VendorItemsResponse
	err := invoke.Call(ctx, c.S, "world", "vendor_items", &worldpb.VendorItemsRequest{Entry: entry}, &r)
	return r.Items, err
}

// BuyItem buys one of an item at the vendor within 5 yards (at most maxCopper; 0: all our money).
func (c C) BuyItem(ctx context.Context, item, maxCopper uint32) (*townpb.RestockResult, error) {
	var r townpb.RestockResult
	err := invoke.Call(ctx, c.S, "town", "restock", &townpb.RestockRequest{Item: item, MaxCopper: maxCopper}, &r)
	return &r, err
}

func (c C) PickReward(ctx context.Context, items []uint32) (*progresspb.PickRewardResult, error) {
	var r progresspb.PickRewardResult
	err := invoke.Call(ctx, c.S, "progress", "pick_reward", &progresspb.PickRewardRequest{Items: items}, &r)
	return &r, err
}

func (c C) Repair(ctx context.Context) (*townpb.RepairResult, error) {
	var r townpb.RepairResult
	err := invoke.Call(ctx, c.S, "town", "repair", &emptypb.Empty{}, &r)
	return &r, err
}

func (c C) SellJunk(ctx context.Context) (*townpb.SellResult, error) {
	var r townpb.SellResult
	err := invoke.Call(ctx, c.S, "town", "sell_junk", &townpb.SellRequest{}, &r)
	return &r, err
}

// MakeRoom asks town for a bag slot for an item about to be taken (entry 0: a quest reward).
func (c C) MakeRoom(ctx context.Context, entry, count uint32) (*townpb.RoomResult, error) {
	var r townpb.RoomResult
	err := invoke.Call(ctx, c.S, "town", "make_room", &townpb.RoomRequest{Entry: entry, Count: count}, &r)
	return &r, err
}

// Room is a loot.Room backed by town.make_room.
func (c C) Room(ctx context.Context, entry, count uint32) bool {
	r, err := c.MakeRoom(ctx, entry, count)
	return err == nil && r.Ok
}

func (c C) GameObject(ctx context.Context, entry uint32) (*worldpb.GameObjectInfo, error) {
	var r worldpb.GameObjectInfo
	err := invoke.Call(ctx, c.S, "world", "game_object", &worldpb.GameObjectRequest{Entry: entry}, &r)
	return &r, err
}

// Chests lists the chests within radius yards that any character may open now (quest.chests).
func (c C) Chests(ctx context.Context, radius float32) ([]*questpb.Chest, error) {
	var r questpb.ChestsResponse
	err := invoke.Call(ctx, c.S, "quest", "chests", &questpb.ChestsRequest{Radius: radius}, &r)
	return r.Chests, err
}

// OpenChest opens a chest (quest.use with its guid and opening spell) and takes the loot.
func (c C) OpenChest(ctx context.Context, ch *questpb.Chest) (*questpb.QuestResult, error) {
	var r questpb.QuestResult
	err := invoke.Call(ctx, c.S, "quest", "use", &questpb.UseRequest{Entry: ch.Entry, Guid: ch.Guid, OpenSpell: ch.OpenSpell, Loot: true}, &r)
	return &r, err
}

// SellPlan reports what sell_junk would sell (dry run), without a vendor.
func (c C) SellPlan(ctx context.Context) (*townpb.SellResult, error) {
	var r townpb.SellResult
	err := invoke.Call(ctx, c.S, "town", "sell_junk", &townpb.SellRequest{DryRun: true}, &r)
	return &r, err
}
