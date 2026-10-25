// roads reads the road and path textures of the terrain from the client archives and writes one road
// grid per map (internal/roads): go run ./cmd/roads [-list] [-maps 0,1,530,571]
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"wowagent/internal/mpq"
	"wowagent/internal/roads"
)

// Map directories of the client (Map.dbc internal names).
var dirs = map[uint32]string{0: "Azeroth", 1: "Kalimdor", 530: "Expansion01", 571: "Northrend"}

// cellCover: a 4.2-yard cell is road when road textures cover at least this much of it on average.
const cellCover = 128

func main() {
	client := flag.String("client", "../assets/client/Data", "client Data directory")
	out := flag.String("out", "../assets/roads", "output directory")
	maps := flag.String("maps", "0,1,530,571", "maps")
	list := flag.Bool("list", false, "list the terrain textures by coverage instead")
	flag.Parse()
	set, err := mpq.OpenClient(*client)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer set.Close()
	os.MkdirAll(*out, 0o755)
	cover := map[string]int{}
	for _, s := range strings.Split(*maps, ",") {
		id64, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
		id := uint32(id64)
		dir := dirs[id]
		if dir == "" {
			fmt.Println("unknown map", id)
			os.Exit(1)
		}
		w, err := set.Read(fmt.Sprintf(`World\Maps\%s\%s.wdt`, dir, dir))
		if err != nil {
			fmt.Println("wdt:", err)
			os.Exit(1)
		}
		tiles, big, _ := roads.WDT(w)
		rm := roads.New()
		nt, cells := 0, 0
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				if !tiles[y*64+x] {
					continue
				}
				b, err := set.Read(fmt.Sprintf(`World\Maps\%s\%s_%d_%d.adt`, dir, dir, x, y))
				if err != nil {
					slog.Warn("adt", "map", id, "x", x, "y", y, "err", err)
					continue
				}
				a, err := roads.ParseADT(b, big)
				if err != nil {
					slog.Warn("adt", "map", id, "x", x, "y", y, "err", err)
					continue
				}
				nt++
				road := make([]bool, len(a.Textures))
				for i, t := range a.Textures {
					road[i] = roads.IsRoad(t)
				}
				for _, c := range a.Chunks {
					if *list {
						for l, ti := range c.Tex {
							if int(ti) >= len(a.Textures) {
								continue
							}
							sum := 0
							if c.Alpha[l] != nil {
								for _, v := range c.Alpha[l] {
									sum += int(v)
								}
							} else {
								sum = 255 * 4096 / 2
							}
							cover[strings.ToLower(a.Textures[ti])] += sum / 255
						}
						continue
					}
					rc := roads.RoadCover(c, road)
					// ADT <map>_x_y is the .map grid (gx, gy) = (y, x) (the extractor writes it as
					// %03u%02u%02u with map, y, x); chunk rows (IY) run along gx like terrain's area cells.
					for r := 0; r < 8; r++ {
						for col := 0; col < 8; col++ {
							sum := 0
							for dr := 0; dr < 8; dr++ {
								for dc := 0; dc < 8; dc++ {
									sum += rc[(r*8+dr)*64+col*8+dc]
								}
							}
							if sum/64 >= cellCover {
								rm.Set(y, x, c.IY*8+r, c.IX*8+col)
								cells++
							}
						}
					}
				}
			}
		}
		if !*list {
			if err := rm.Write(roads.File(*out, id)); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		slog.Info("map", "id", id, "tiles", nt, "road cells", cells)
	}
	if *list {
		type tc struct {
			t string
			c int
		}
		var all []tc
		for t, c := range cover {
			all = append(all, tc{t, c})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].c > all[j].c })
		for _, e := range all {
			mark := " "
			if roads.IsRoad(e.t) {
				mark = "R"
			}
			fmt.Printf("%s %9d %s\n", mark, e.c, e.t)
		}
	}
}
