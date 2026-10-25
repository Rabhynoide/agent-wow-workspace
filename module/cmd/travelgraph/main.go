// travelgraph computes the travel graph read by the travel module (internal/travel/graph.go):
// go run ./cmd/travelgraph [-maps 0,1,369] [-out ../assets/travel/graph.json]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wowagent/internal/travel"
)

func main() {
	assets := flag.String("assets", "../assets", "assets directory (dbc, mmaps, maps, vmaps)")
	out := flag.String("out", "../assets/travel/graph.json", "graph file")
	maps := flag.String("maps", "0,1,369,530,571", "maps to cover (open world)")
	workers := flag.Int("workers", 3, "parallel planners")
	flag.Parse()
	var ms []uint32
	for _, s := range strings.Split(*maps, ",") {
		v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
		if err != nil {
			fmt.Println("bad map:", s)
			os.Exit(1)
		}
		ms = append(ms, uint32(v))
	}
	start := time.Now()
	g, err := travel.Build(context.Background(), travel.BuildOpts{Assets: *assets, Maps: ms, Workers: *workers})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	b, _ := json.Marshal(g)
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	slog.Info("written", "file", *out, "bytes", len(b), "s", int(time.Since(start).Seconds()))
}
