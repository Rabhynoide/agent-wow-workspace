package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"wowagent/internal/quest"
	"wowagent/internal/runtime"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runtime.Run(ctx, quest.Register); err != nil {
		slog.Error("module stopped", "error", err)
		os.Exit(1)
	}
}
