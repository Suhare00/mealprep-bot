package config

import (
	"log/slog"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Environment != "development" {
		t.Errorf("Environment = %q, want development", cfg.Environment)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.DatabaseURL == "" || cfg.TelegramBotToken == "" {
		t.Error("required sensitive settings were not loaded")
	}
	if cfg.MediaRoot == "" {
		t.Error("MediaRoot was not resolved")
	}
}

func TestLoadRejectsMissingRequiredSetting(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv(envTelegramBotToken, "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), envTelegramBotToken) {
		t.Fatalf("Load() error = %v, want error mentioning %s", err, envTelegramBotToken)
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv(envAppEnvironment, "local")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), envAppEnvironment) {
		t.Fatalf("Load() error = %v, want error mentioning %s", err, envAppEnvironment)
	}
}

func TestLoadRejectsInvalidDatabaseURL(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv(envDatabaseURL, "http://localhost/database")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), envDatabaseURL) {
		t.Fatalf("Load() error = %v, want error mentioning %s", err, envDatabaseURL)
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv(envLogLevel, "verbose")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), envLogLevel) {
		t.Fatalf("Load() error = %v, want error mentioning %s", err, envLogLevel)
	}
}

func TestLoadDatabaseURLDoesNotRequireServiceConfiguration(t *testing.T) {
	t.Setenv(envDatabaseURL, "postgres://mealprep:secret@localhost:5432/mealprep")
	t.Setenv(envAppEnvironment, "")
	t.Setenv(envTelegramBotToken, "")

	got, err := LoadDatabaseURL()
	if err != nil {
		t.Fatalf("LoadDatabaseURL() error = %v", err)
	}
	if got != "postgres://mealprep:secret@localhost:5432/mealprep" {
		t.Fatalf("LoadDatabaseURL() = %q", got)
	}
}

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(envAppEnvironment, "development")
	t.Setenv(envDatabaseURL, "postgres://mealprep:password@localhost:5432/mealprep?sslmode=disable")
	t.Setenv(envLogLevel, "info")
	t.Setenv(envTelegramBotToken, "test-token")
}
