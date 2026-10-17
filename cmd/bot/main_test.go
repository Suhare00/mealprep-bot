package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Suhare00/mealprep-bot/internal/config"
)

func TestSanitizeRuntimeErrorRedactsCredentials(t *testing.T) {
	cfg := config.Config{
		DatabaseURL:      "postgres://mealprep:database-secret@localhost:5432/mealprep",
		TelegramBotToken: "123456:telegram-secret",
	}
	err := errors.New("request https://api.telegram.org/bot123456:telegram-secret/getUpdates failed; " +
		"database postgres://mealprep:database-secret@localhost:5432/mealprep password database-secret")

	got := sanitizeRuntimeError(err, cfg)
	for _, secret := range []string{cfg.TelegramBotToken, cfg.DatabaseURL, "database-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitizeRuntimeError() leaked %q in %q", secret, got)
		}
	}
}
