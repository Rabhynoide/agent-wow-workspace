// Command zygor2route turns a Zygor-format leveling guide (Lua file with RegisterGuide blocks) into
// route files readable by guidecheck: one JSON file per guide, steps flattened to accept / turnin /
// kill / collect / travel... actions with the ids of the quests, NPCs and monsters.
//
// The source files are third-party content kept in assets/ (not in git); so are the generated routes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type only struct {
	Race  string `json:"race"`
	Class string `json:"class"`
}

type step struct {
	N        int      `json:"n"`
	ZStep    int      `json:"zstep"`
	Action   string   `json:"action"`
	Quest    string   `json:"quest,omitempty"`
	QuestID  uint32   `json:"quest_id,omitempty"`
	Obj      string   `json:"objective,omitempty"` // quest log objective index ("1", "2"...)
	NPC      string   `json:"npc,omitempty"`
	NPCID    uint32   `json:"npc_id,omitempty"`
	Targets  string   `json:"targets,omitempty"`
	TargetID []uint32 `json:"target_ids,omitempty"`
	ItemID   uint32   `json:"item_id,omitempty"`
	Count    int      `json:"count,omitempty"`
	Place    string   `json:"place,omitempty"`
	Coords   string   `json:"coords,omitempty"`
	Level    string   `json:"level,omitempty"`
	Class    string   `json:"class,omitempty"`
	Only     []only   `json:"only,omitempty"`
	Note     string   `json:"note,omitempty"`
}

type guide struct {
	Source     string `json:"source"`
	Faction    string `json:"faction"`
	Race       string `json:"race"`
	Zone       string `json:"zone"`
	Levels     string `json:"levels"`
	StartLevel int    `json:"startlevel"`
	Next       string `json:"next,omitempty"`
	Steps      []step `json:"steps"`
}

var (
	reGuide  = regexp.MustCompile(`RegisterGuide\("([^"]*)"`)
	reTitle  = regexp.MustCompile(`\\\\([^\\]+?)\s*\((\d+)-(\d+)\)\s*$`)
	reIDTail = regexp.MustCompile(`^(.*?)##(\d+)$`)
	reQ      = regexp.MustCompile(`\|q\s+(\d+)(?:/(\d+))?`)
	reCount  = regexp.MustCompile(`^(\d+)\s+(.*)$`)
)

func main() {
	src := flag.String("src", "", "Zygor leveling guide .lua file")
	out := flag.String("out", "../assets/routes", "output directory")
	prefix := flag.String("prefix", "", "file name prefix (default: faction)")
	flag.Parse()
	b, err := os.ReadFile(*src)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.MkdirAll(*out, 0o755)
	text := strings.ReplaceAll(string(b), "\r", "")
	var cur *guide
	var ctx ctxT
	n, zs := 0, 0
	idx := 0
	stats := map[string]int{}
	flush := func() {
		if cur == nil {
			return
		}
		idx++
		name := fmt.Sprintf("%s_%02d_%s.json", pick(*prefix, strings.ToLower(cur.Faction)), idx, slug(cur.Zone))
		data, _ := json.MarshalIndent(cur, "", " ")
		os.WriteFile(filepath.Join(*out, name), data, 0o644)
		fmt.Printf("%s: %d steps\n", name, len(cur.Steps))
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "--") || line == "":
			continue
		case strings.Contains(line, "RegisterGuide("):
			flush()
			m := reGuide.FindStringSubmatch(line)
			name := ""
			if m != nil {
				name = m[1]
			}
			cur = &guide{Source: "zygor:" + filepath.Base(*src), Race: "any", Zone: lastPart(name)}
			if t := reTitle.FindStringSubmatch(name); t != nil {
				cur.Zone = strings.TrimSpace(t[1]) + " (" + t[2] + "-" + t[3] + ")"
				cur.Levels = t[2] + "-" + t[3]
			}
			n, zs, ctx = 0, 0, ctxT{}
			continue
		case line == "]])" || strings.HasPrefix(line, "]])"):
			flush()
			cur = nil
			continue
		case cur == nil:
			continue
		}
		fields := strings.SplitN(line, " ", 2)
		key, rest := fields[0], ""
		if len(fields) > 1 {
			rest = strings.TrimSpace(fields[1])
		}
		switch key {
		case "faction":
			cur.Faction = strings.Title(rest)
		case "defaultfor":
			cur.Race = strings.ReplaceAll(rest, "NightElf", "Night Elf")
		case "startlevel":
			cur.StartLevel, _ = strconv.Atoi(rest)
		case "next":
			cur.Next = lastPart(rest)
		case "type", "author":
		case "step":
			zs++
			ctx = ctxT{place: ctx.place}
		case "only":
			cur.attachOnly(zs, parseOnly(rest))
		default:
			emit(cur, &n, zs, &ctx, line, stats)
		}
	}
	flush()
	var unk []string
	for k, v := range stats {
		unk = append(unk, fmt.Sprintf("%s=%d", k, v))
	}
	fmt.Println("ignored directives:", strings.Join(unk, " "))
}

type ctxT struct {
	place, coords string
	npc           string
	npcID         uint32
	from          []uint32
	fromName      string
}

func pick(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func lastPart(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\`)
	if i := strings.LastIndex(s, `\`); i >= 0 {
		return s[i+1:]
	}
	return s
}

func slug(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func parseOnly(rest string) []only {
	var out []only
	for _, part := range strings.Split(rest, ",") {
		f := strings.Fields(part)
		switch len(f) {
		case 2:
			out = append(out, only{Race: strings.ReplaceAll(f[0], "NightElf", "Night Elf"), Class: f[1]})
		case 1:
			out = append(out, only{Race: f[0]}) // race or class alone: kept as given
		}
	}
	return out
}

// attachOnly restricts every step of the current Zygor step to the given race/class pairs.
func (g *guide) attachOnly(zs int, o []only) {
	classes := map[string]bool{}
	for _, p := range o {
		classes[p.Class] = true
	}
	for i := len(g.Steps) - 1; i >= 0 && g.Steps[i].ZStep == zs; i-- {
		g.Steps[i].Only = o
		if len(classes) == 1 && o[0].Class != "" {
			g.Steps[i].Class = o[0].Class
		}
	}
}

func splitID(s string) (string, uint32) {
	if m := reIDTail.FindStringSubmatch(s); m != nil {
		id, _ := strconv.Atoi(m[2])
		return strings.TrimSpace(m[1]), uint32(id)
	}
	return strings.TrimSpace(s), 0
}

// body strips the |flags of a directive and returns the flags too.
func body(s string) (string, string) {
	if i := strings.Index(s, "//"); i >= 0 { // trailing comment
		s = s[:i]
	}
	if i := strings.Index(s, "|"); i >= 0 {
		return strings.TrimSpace(s[:i]), s[i:]
	}
	return strings.TrimSpace(s), ""
}

func questRef(flags string) (uint32, string) {
	if m := reQ.FindStringSubmatch(flags); m != nil {
		id, _ := strconv.Atoi(m[1])
		return uint32(id), m[2]
	}
	return 0, ""
}

func add(g *guide, n *int, zs int, c *ctxT, s step) {
	*n++
	s.N, s.ZStep = *n, zs
	s.Place, s.Coords = c.place, c.coords
	g.Steps = append(g.Steps, s)
}

func emit(g *guide, n *int, zs int, c *ctxT, line string, stats map[string]int) {
	switch {
	case strings.HasPrefix(line, "goto "):
		parseGoto(c, strings.TrimSpace(strings.TrimPrefix(line, "goto ")))
	case strings.HasPrefix(line, ".talk "):
		b, _ := body(strings.TrimPrefix(line, ".talk "))
		c.npc, c.npcID = splitID(b)
	case strings.HasPrefix(line, "..accept "), strings.HasPrefix(line, "..turnin "):
		act := "accept"
		if strings.HasPrefix(line, "..turnin ") {
			act = "turnin"
		}
		b, _ := body(line[strings.Index(line, " ")+1:])
		title, id := splitID(b)
		add(g, n, zs, c, step{Action: act, Quest: title, QuestID: id, NPC: c.npc, NPCID: c.npcID})
	case strings.HasPrefix(line, "..fpath "), strings.HasPrefix(line, "fpath "):
		add(g, n, zs, c, step{Action: "flight_path", Place: "", Note: "flight path " + strings.TrimSpace(line[strings.Index(line, " ")+1:]), NPC: c.npc, NPCID: c.npcID})
	case strings.HasPrefix(line, "home "):
		add(g, n, zs, c, step{Action: "set_hearth", Note: strings.TrimPrefix(line, "home "), NPC: c.npc, NPCID: c.npcID})
	case strings.HasPrefix(line, "ding "):
		add(g, n, zs, c, step{Action: "note", Level: strings.TrimSpace(strings.TrimPrefix(line, "ding ")), Note: "reach this level"})
	case strings.HasPrefix(line, ".from "):
		b, _ := body(strings.TrimPrefix(line, ".from "))
		c.from, c.fromName = nil, ""
		for _, part := range strings.Split(b, ",") {
			name, id := splitID(strings.TrimSuffix(strings.TrimSpace(part), "+"))
			name = strings.TrimSuffix(name, "+")
			if id != 0 {
				c.from = append(c.from, id)
			}
			if c.fromName == "" {
				c.fromName = name
			}
		}
	case strings.HasPrefix(line, ".kill "):
		kind(g, n, zs, c, "kill", strings.TrimPrefix(line, ".kill "))
	case strings.HasPrefix(line, ".get "), strings.HasPrefix(line, ".collect "), strings.HasPrefix(line, "..get "), strings.HasPrefix(line, "..collect "):
		kind(g, n, zs, c, "collect", line[strings.Index(line, " ")+1:])
	case strings.HasPrefix(line, ".goal "):
		kind(g, n, zs, c, "complete", strings.TrimPrefix(line, ".goal "))
	case strings.HasPrefix(line, ".buy "), strings.HasPrefix(line, "..buy "):
		kind(g, n, zs, c, "buy", line[strings.Index(line, " ")+1:])
	case strings.HasPrefix(line, ".use "), strings.HasPrefix(line, "use "):
		b, _ := body(line[strings.Index(line, " ")+1:])
		name, id := splitID(b)
		add(g, n, zs, c, step{Action: "use_object", Targets: name, ItemID: id})
	case strings.HasPrefix(line, ".'"), strings.HasPrefix(line, "'"), strings.HasPrefix(line, "..'"):
		instruction(g, n, zs, c, strings.TrimLeft(line, ".'"))
	default:
		stats[strings.Fields(line)[0]]++
	}
}

func parseGoto(c *ctxT, s string) {
	s, _ = body(s)
	parts := strings.Split(s, ",")
	if len(parts) >= 3 {
		c.place = strings.TrimSpace(parts[0])
		c.coords = strings.TrimSpace(parts[1]) + "," + strings.TrimSpace(parts[2])
	} else if len(parts) == 2 {
		c.coords = strings.TrimSpace(parts[0]) + "," + strings.TrimSpace(parts[1])
	}
}

// kind parses "N Name##id|q questid/objective" for kill/collect/goal/buy.
func kind(g *guide, n *int, zs int, c *ctxT, action, rest string) {
	b, flags := body(rest)
	count := 0
	if m := reCount.FindStringSubmatch(b); m != nil {
		count, _ = strconv.Atoi(m[1])
		b = m[2]
	}
	name, id := splitID(b)
	qid, obj := questRef(flags)
	s := step{Action: action, Targets: name, Count: count, QuestID: qid, Obj: obj}
	if action == "collect" {
		s.ItemID = id
		s.TargetID = c.from
		if c.fromName != "" {
			s.NPC = c.fromName + " (drops)"
		}
		c.from, c.fromName = nil, ""
	} else if id != 0 {
		s.TargetID = []uint32{id}
	}
	if strings.Contains(flags, "|future") {
		s.Note = "future"
	}
	add(g, n, zs, c, s)
}

// instruction handles free text lines like "Fly to X|goto ...|noway|c" and "Click the Y|goal ...|q 12/1".
func instruction(g *guide, n *int, zs int, c *ctxT, line string) {
	text, flags := body(line)
	if i := strings.Index(flags, "|goto"); i >= 0 {
		g2 := flags[i+len("|goto"):]
		if j := strings.Index(g2, "|"); j >= 0 {
			g2 = g2[:j]
		}
		parseGoto(c, strings.TrimSpace(g2))
	}
	low := strings.ToLower(text)
	act := "note"
	switch {
	case strings.HasPrefix(low, "fly to"), strings.HasPrefix(low, "ride"), strings.HasPrefix(low, "go "), strings.HasPrefix(low, "run "), strings.HasPrefix(low, "follow"):
		act = "travel"
	case strings.HasPrefix(low, "hearth"):
		act = "hearth"
	case strings.Contains(flags, "|use "):
		act = "use_object"
	case strings.Contains(flags, "|goal"), strings.HasPrefix(low, "click"), strings.HasPrefix(low, "kill"), strings.HasPrefix(low, "burn"), strings.HasPrefix(low, "destroy"):
		act = "complete"
	}
	qid, obj := questRef(flags)
	s := step{Action: act, Note: text, QuestID: qid, Obj: obj}
	if i := strings.Index(flags, "|use "); i >= 0 {
		u := flags[i+5:]
		if j := strings.Index(u, "|"); j >= 0 {
			u = u[:j]
		}
		name, id := splitID(u)
		s.Targets, s.ItemID = name, id
	}
	add(g, n, zs, c, s)
}
