//go:build integration

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunSeedsCanonicalDatasetAndIsIdempotent(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	t.Cleanup(pool.Close)

	for _, name := range []string{
		"000003_recipe_steps_media.down.sql",
		"000001_recipe_catalogue.down.sql",
		"000001_recipe_catalogue.up.sql",
		"000003_recipe_steps_media.up.sql",
	} {
		contents, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(t.Context(), string(contents)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}

	t.Setenv("DATABASE_URL", databaseURL)
	tests := []struct {
		name       string
		wantOutput string
	}{
		{
			name:       "initial import",
			wantOutput: "seed import complete: recipes inserted=90 existing=0; ingredients inserted=84 existing=0\n",
		},
		{
			name:       "idempotent import",
			wantOutput: "seed import complete: recipes inserted=0 existing=90; ingredients inserted=0 existing=84\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := run(t.Context(), []string{"-dir", filepath.Join("..", "..", "seed")}, &stdout, &stderr); err != nil {
				t.Fatalf("run() error = %v; stderr = %q", err, stderr.String())
			}
			if stdout.String() != test.wantOutput {
				t.Fatalf("run() output = %q, want %q", stdout.String(), test.wantOutput)
			}
		})
	}
}
