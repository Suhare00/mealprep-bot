package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suhare00/mealprep-bot/internal/config"
	"github.com/Suhare00/mealprep-bot/internal/postgres"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
	"github.com/Suhare00/mealprep-bot/internal/telegram"
)

const (
	startupTimeout = 5 * time.Second
	httpTimeout    = 40 * time.Second
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

	logger.Info("service starting", "environment", cfg.Environment)
	if err := run(ctx, cfg, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("service stopped with error", "error", sanitizeRuntimeError(err, cfg))
		os.Exit(1)
	}
	logger.Info("service stopped")
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("configure PostgreSQL pool: invalid connection configuration")
	}
	defer pool.Close()

	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := pool.Ping(startupCtx); err != nil {
		if startupCtx.Err() != nil {
			return startupCtx.Err()
		}
		return errors.New("connect to PostgreSQL: connection failed")
	}

	repository := postgres.NewRecipeRepository(pool)
	catalogue := recipecatalogue.NewCatalogue(repository)
	if _, err := catalogue.ListActive(startupCtx); err != nil {
		if startupCtx.Err() != nil {
			return startupCtx.Err()
		}
		return errors.New("verify recipe catalogue schema: query failed")
	}
	cancel()
	client := telegram.NewAPIClient(cfg.TelegramBotToken, &http.Client{Timeout: httpTimeout})
	bot := telegram.NewBot(client, catalogue, logger)

	logger.Info("Telegram long polling started")
	return bot.Run(ctx)
}

func sanitizeRuntimeError(err error, cfg config.Config) string {
	message := err.Error()
	secrets := []string{cfg.TelegramBotToken, cfg.DatabaseURL}
	if parsed, parseErr := url.Parse(cfg.DatabaseURL); parseErr == nil && parsed.User != nil {
		if password, ok := parsed.User.Password(); ok {
			secrets = append(secrets, password)
		}
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return message
}
