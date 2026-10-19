// Package config loads and validates runtime configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	envAppEnvironment   = "APP_ENV"
	envDatabaseURL      = "DATABASE_URL"
	envLogLevel         = "LOG_LEVEL"
	envMediaRoot        = "MEDIA_ROOT"
	envTelegramBotToken = "TELEGRAM_BOT_TOKEN"
)

// Config contains the settings required to start the service.
type Config struct {
	Environment      string
	DatabaseURL      string
	TelegramBotToken string
	LogLevel         slog.Level
	MediaRoot        string
}

// Load reads configuration from environment variables and fails on missing or invalid values.
func Load() (Config, error) {
	environment, err := required(envAppEnvironment)
	if err != nil {
		return Config{}, err
	}
	if err := validateEnvironment(environment); err != nil {
		return Config{}, err
	}

	databaseURL, err := LoadDatabaseURL()
	if err != nil {
		return Config{}, err
	}

	telegramBotToken, err := required(envTelegramBotToken)
	if err != nil {
		return Config{}, err
	}

	logLevel, err := parseLogLevel(os.Getenv(envLogLevel))
	if err != nil {
		return Config{}, err
	}
	mediaRoot, err := mediaRootPath(os.Getenv(envMediaRoot))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:      environment,
		DatabaseURL:      databaseURL,
		TelegramBotToken: telegramBotToken,
		LogLevel:         logLevel,
		MediaRoot:        mediaRoot,
	}, nil
}

func mediaRootPath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		value = filepath.Join("data", "media")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("%s is invalid: %w", envMediaRoot, err)
	}
	return filepath.Clean(absolute), nil
}

// LoadDatabaseURL reads and validates the PostgreSQL connection URL.
// It supports database-only tools without requiring unrelated service settings.
func LoadDatabaseURL() (string, error) {
	databaseURL, err := required(envDatabaseURL)
	if err != nil {
		return "", err
	}
	if err := validateDatabaseURL(databaseURL); err != nil {
		return "", err
	}
	return databaseURL, nil
}

func required(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return value, nil
}

func validateEnvironment(environment string) error {
	switch environment {
	case "development", "staging", "production":
		return nil
	default:
		return fmt.Errorf("%s must be development, staging, or production", envAppEnvironment)
	}
}

func validateDatabaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", envDatabaseURL, err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("%s must use the postgres or postgresql scheme", envDatabaseURL)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include a host", envDatabaseURL)
	}

	return nil
}

func parseLogLevel(value string) (slog.Level, error) {
	if strings.TrimSpace(value) == "" {
		return slog.LevelInfo, nil
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return 0, fmt.Errorf("%s is invalid: %w", envLogLevel, err)
	}
	if level < slog.LevelDebug || level > slog.LevelError {
		return 0, errors.New("LOG_LEVEL must be debug, info, warn, or error")
	}

	return level, nil
}
