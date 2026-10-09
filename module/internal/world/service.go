// Package world answers read-only questions about the static world database.
package world

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "wowagent/gen/worldv1"
)

type Service struct {
	pb.UnimplementedWorldServer
	once sync.Once
	db   *sql.DB
	err  error
}

func Register(s grpc.ServiceRegistrar, _ modv1.SessionClient) {
	pb.RegisterWorldServer(s, &Service{})
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// conn opens the database lazily. The session is forced read-only.
func (s *Service) conn() (*sql.DB, error) {
	s.once.Do(func() {
		dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&timeout=5s&transaction_read_only=1",
			env("WORLD_DB_USER", "root"), env("WORLD_DB_PASSWORD", "password"),
			env("WORLD_DB_HOST", "192.168.100.7:3306"), env("WORLD_DB_NAME", "acore_world"))
		s.db, s.err = sql.Open("mysql", dsn)
		if s.db != nil {
			s.db.SetMaxOpenConns(4)
		}
	})
	return s.db, s.err
}

func (s *Service) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	db, err := s.conn()
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return rows, nil
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func limit(n uint32, def int) int {
	if n == 0 {
		return def
	}
	if n > 100 {
		return 100
	}
	return int(n)
}

func (s *Service) FindNpc(ctx context.Context, r *pb.FindNpcRequest) (*pb.FindNpcResponse, error) {
	where := []string{"1=1"}
	var args []any
	if r.Entry != 0 {
		where = append(where, "ct.entry=?")
		args = append(args, r.Entry)
	}
	if r.Name != "" {
		where = append(where, "ct.name LIKE ?")
		args = append(args, "%"+r.Name+"%")
	}
	if r.MapFilter {
		where = append(where, "c.map=?")
		args = append(args, r.Map)
	}
	if r.NpcFlags != 0 {
		where = append(where, "(ct.npcflag & ?)<>0")
		args = append(args, r.NpcFlags)
	}
	dist := "0"
	order := "ct.entry"
	if r.X != 0 || r.Y != 0 {
		dist = "SQRT(POW(c.position_x-?,2)+POW(c.position_y-?,2))"
		order = "dist"
	}
	q := "SELECT ct.entry, ct.name, ct.minlevel, ct.maxlevel, ct.npcflag, ct.faction, c.map, c.position_x, c.position_y, c.position_z, " +
		dist + " AS dist FROM creature_template ct JOIN creature c ON c.id=ct.entry WHERE " + strings.Join(where, " AND ")
	var full []any
	if dist != "0" {
		full = append(full, r.X, r.Y)
	}
	full = append(full, args...)
	if r.Radius > 0 && dist != "0" {
		q += " HAVING dist<=?"
		full = append(full, r.Radius)
	}
	q += " ORDER BY " + order + fmt.Sprintf(" LIMIT %d", limit(r.Max, 10))
	rows, err := s.query(ctx, q, full...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &pb.FindNpcResponse{}
	for rows.Next() {
		sp := &pb.Spawn{}
		if err := rows.Scan(&sp.Entry, &sp.Name, &sp.MinLevel, &sp.MaxLevel, &sp.NpcFlags, &sp.Faction, &sp.Map, &sp.X, &sp.Y, &sp.Z, &sp.Dist); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		out.Spawns = append(out.Spawns, sp)
	}
	return out, rows.Err()
}

func (s *Service) Spawns(ctx context.Context, r *pb.SpawnsRequest) (*pb.SpawnsResponse, error) {
	table, tpl, idcol := "creature", "creature_template", "id"
	nameExpr, lvl := "t.name", "t.minlevel, t.maxlevel, t.npcflag, t.faction"
	if r.Kind == "gameobject" {
		table, tpl = "gameobject", "gameobject_template"
		lvl = "0, 0, 0, 0"
	}
	q := fmt.Sprintf("SELECT t.entry, %s, %s, c.map, c.position_x, c.position_y, c.position_z, SQRT(POW(c.position_x-?,2)+POW(c.position_y-?,2)) AS dist "+
		"FROM %s c JOIN %s t ON t.entry=c.%s WHERE c.%s=? AND c.map=?", nameExpr, lvl, table, tpl, idcol, idcol)
	args := []any{r.X, r.Y, r.Entry, r.Map}
	if r.Radius > 0 {
		q += " HAVING dist<=?"
		args = append(args, r.Radius)
	}
	q += fmt.Sprintf(" ORDER BY dist LIMIT %d", limit(r.Max, 20))
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &pb.SpawnsResponse{}
	for rows.Next() {
		sp := &pb.Spawn{}
		if err := rows.Scan(&sp.Entry, &sp.Name, &sp.MinLevel, &sp.MaxLevel, &sp.NpcFlags, &sp.Faction, &sp.Map, &sp.X, &sp.Y, &sp.Z, &sp.Dist); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		out.Spawns = append(out.Spawns, sp)
	}
	return out, rows.Err()
}

func (s *Service) giverSpawns(ctx context.Context, kind, role string, quest uint32) []*pb.QuestNpc {
	var q string
	switch {
	case kind == "creature":
		q = "SELECT t.entry, t.name, c.map, c.position_x, c.position_y, c.position_z FROM creature_%s g JOIN creature_template t ON t.entry=g.id JOIN creature c ON c.id=t.entry WHERE g.quest=? LIMIT 4"
	default:
		q = "SELECT t.entry, t.name, c.map, c.position_x, c.position_y, c.position_z FROM gameobject_%s g JOIN gameobject_template t ON t.entry=g.id JOIN gameobject c ON c.id=t.entry WHERE g.quest=? LIMIT 4"
	}
	rows, err := s.query(ctx, fmt.Sprintf(q, role), quest)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*pb.QuestNpc
	for rows.Next() {
		n := &pb.QuestNpc{}
		if rows.Scan(&n.Entry, &n.Name, &n.Map, &n.X, &n.Y, &n.Z) == nil {
			out = append(out, n)
		}
	}
	return out
}

func (s *Service) nameOf(ctx context.Context, q string, id int64) string {
	rows, err := s.query(ctx, q, id)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var n string
	if rows.Next() {
		_ = rows.Scan(&n)
	}
	return n
}

func (s *Service) QuestInfo(ctx context.Context, r *pb.QuestInfoRequest) (*pb.QuestInfoResponse, error) {
	cols := "q.ID, q.LogTitle, q.QuestLevel, q.MinLevel, q.QuestSortID, COALESCE(a.PrevQuestID,0), q.RewardNextQuest, q.RewardXPDifficulty, q.RewardMoney, q.LogDescription"
	for i := 1; i <= 4; i++ {
		cols += fmt.Sprintf(", q.RequiredNpcOrGo%d, q.RequiredNpcOrGoCount%d", i, i)
	}
	for i := 1; i <= 6; i++ {
		cols += fmt.Sprintf(", q.RequiredItemId%d, q.RequiredItemCount%d", i, i)
	}
	q := "SELECT " + cols + " FROM quest_template q LEFT JOIN quest_template_addon a ON a.ID=q.ID WHERE "
	var args []any
	if r.Id != 0 {
		q += "q.ID=?"
		args = append(args, r.Id)
	} else if r.Title != "" {
		q += "q.LogTitle LIKE ?"
		args = append(args, "%"+r.Title+"%")
	} else {
		return nil, status.Error(codes.InvalidArgument, "id or title required")
	}
	q += " LIMIT 5"
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &pb.QuestInfoResponse{}
	for rows.Next() {
		qi := &pb.QuestInfo{}
		var npc [4]int64
		var npcN [4]uint32
		var item [6]uint32
		var itemN [6]uint32
		dest := []any{&qi.Id, &qi.Title, &qi.Level, &qi.MinLevel, &qi.ZoneOrSort, &qi.PrevQuest, &qi.NextQuest, &qi.RewardXpId, &qi.RewardMoney, &qi.ObjectivesText}
		for i := 0; i < 4; i++ {
			dest = append(dest, &npc[i], &npcN[i])
		}
		for i := 0; i < 6; i++ {
			dest = append(dest, &item[i], &itemN[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		out.Quests = append(out.Quests, qi)
		for i := 0; i < 4; i++ {
			switch {
			case npc[i] > 0:
				qi.Objectives = append(qi.Objectives, &pb.QuestObjective{Kind: "kill", Entry: uint32(npc[i]), Count: npcN[i]})
			case npc[i] < 0:
				qi.Objectives = append(qi.Objectives, &pb.QuestObjective{Kind: "gameobject", Entry: uint32(-npc[i]), Count: npcN[i]})
			}
		}
		for i := 0; i < 6; i++ {
			if item[i] != 0 {
				qi.Objectives = append(qi.Objectives, &pb.QuestObjective{Kind: "item", Entry: item[i], Count: itemN[i]})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	rows.Close()
	for _, qi := range out.Quests {
		for _, o := range qi.Objectives {
			switch o.Kind {
			case "kill":
				o.Name = s.nameOf(ctx, "SELECT name FROM creature_template WHERE entry=?", int64(o.Entry))
			case "gameobject":
				o.Name = s.nameOf(ctx, "SELECT name FROM gameobject_template WHERE entry=?", int64(o.Entry))
			case "item":
				o.Name = s.nameOf(ctx, "SELECT name FROM item_template WHERE entry=?", int64(o.Entry))
			}
		}
		qi.Starters = append(s.giverSpawns(ctx, "creature", "queststarter", qi.Id), s.giverSpawns(ctx, "gameobject", "queststarter", qi.Id)...)
		qi.Enders = append(s.giverSpawns(ctx, "creature", "questender", qi.Id), s.giverSpawns(ctx, "gameobject", "questender", qi.Id)...)
	}
	return out, nil
}

func (s *Service) QuestsForNpc(ctx context.Context, r *pb.QuestsForNpcRequest) (*pb.QuestsForNpcResponse, error) {
	out := &pb.QuestsForNpcResponse{}
	for _, role := range []string{"start", "end"} {
		rows, err := s.query(ctx, fmt.Sprintf("SELECT q.ID, q.LogTitle, q.QuestLevel, q.MinLevel FROM creature_quest%ser g JOIN quest_template q ON q.ID=g.quest WHERE g.id=? ORDER BY q.MinLevel, q.ID", role), r.Entry)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			qr := &pb.QuestRef{Role: role}
			if rows.Scan(&qr.Id, &qr.Title, &qr.Level, &qr.MinLevel) == nil {
				out.Quests = append(out.Quests, qr)
			}
		}
		rows.Close()
	}
	return out, nil
}

func (s *Service) ItemInfo(ctx context.Context, r *pb.ItemInfoRequest) (*pb.ItemInfoResponse, error) {
	q := "SELECT entry, name, Quality, ItemLevel, RequiredLevel, class, subclass, InventoryType, SellPrice, BuyPrice, stackable FROM item_template WHERE "
	var arg any
	if r.Entry != 0 {
		q += "entry=?"
		arg = r.Entry
	} else if r.Name != "" {
		q += "name LIKE ?"
		arg = "%" + r.Name + "%"
	} else {
		return nil, status.Error(codes.InvalidArgument, "entry or name required")
	}
	rows, err := s.query(ctx, q+" LIMIT 10", arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &pb.ItemInfoResponse{}
	for rows.Next() {
		it := &pb.ItemInfo{}
		if err := rows.Scan(&it.Entry, &it.Name, &it.Quality, &it.ItemLevel, &it.RequiredLevel, &it.Class, &it.Subclass, &it.InventoryType, &it.SellPrice, &it.BuyPrice, &it.Stackable); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		out.Items = append(out.Items, it)
	}
	return out, rows.Err()
}

func (s *Service) Creature(ctx context.Context, r *pb.CreatureInfoRequest) (*pb.CreatureInfo, error) {
	rows, err := s.query(ctx, "SELECT entry, name, minlevel, maxlevel, faction, npcflag, `rank`, type, skinloot, lootid FROM creature_template WHERE entry=?", r.Entry)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, status.Error(codes.NotFound, "no such creature")
	}
	c := &pb.CreatureInfo{}
	if err := rows.Scan(&c.Entry, &c.Name, &c.MinLevel, &c.MaxLevel, &c.Faction, &c.NpcFlags, &c.Rank, &c.Type, &c.SkinningLoot, &c.LootId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return c, nil
}

func (s *Service) ItemSources(ctx context.Context, r *pb.ItemSourcesRequest) (*pb.ItemSourcesResponse, error) {
	out := &pb.ItemSourcesResponse{}
	queries := []struct{ kind, q string }{
		{"creature", "SELECT ct.entry, ct.name, ct.minlevel, ct.maxlevel, ABS(l.Chance) FROM creature_loot_template l JOIN creature_template ct ON ct.lootid=l.Entry WHERE l.Item=? AND EXISTS (SELECT 1 FROM creature c WHERE c.id=ct.entry) ORDER BY ABS(l.Chance) DESC LIMIT 8"},
		{"gameobject", "SELECT g.entry, g.name, 0, 0, ABS(l.Chance) FROM gameobject_loot_template l JOIN gameobject_template g ON g.Data1=l.Entry AND g.type=3 WHERE l.Item=? AND EXISTS (SELECT 1 FROM gameobject o WHERE o.id=g.entry) ORDER BY ABS(l.Chance) DESC LIMIT 8"},
	}
	for _, qq := range queries {
		rows, err := s.query(ctx, qq.q, r.Entry)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			src := &pb.ItemSource{Kind: qq.kind}
			if rows.Scan(&src.Entry, &src.Name, &src.MinLevel, &src.MaxLevel, &src.Chance) == nil {
				out.Sources = append(out.Sources, src)
			}
		}
		rows.Close()
	}
	return out, nil
}
