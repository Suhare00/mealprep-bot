package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Suhare00/mealprep-bot/internal/config"
)

func main() {
	bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("service started", "environment", cfg.Environment)
	run(ctx)
	logger.Info("service stopped")
}

func run(ctx context.Context) {
	<-ctx.Done()
}
