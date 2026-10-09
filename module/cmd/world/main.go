package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"wowagent/internal/runtime"
	"wowagent/internal/world"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runtime.Run(ctx, world.Register); err != nil {
		slog.Error("module stopped", "error", err)
		os.Exit(1)
	}
}
