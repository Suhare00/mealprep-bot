package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suhare00/mealprep-bot/internal/config"
	"github.com/Suhare00/mealprep-bot/internal/postgres"
	"github.com/Suhare00/mealprep-bot/internal/seed"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "seed import failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	flags.SetOutput(stderr)
	directory := flags.String("dir", "./seed", "directory containing canonical seed CSV files")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	dataset, err := seed.Load(*directory)
	if err != nil {
		return fmt.Errorf("load seed dataset: %w", err)
	}
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return fmt.Errorf("load database configuration: %w", err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}

	result, err := postgres.NewSeedImporter(pool).Import(ctx, dataset)
	if err != nil {
		return fmt.Errorf("import seed dataset: %w", err)
	}
	if _, err := fmt.Fprintf(stdout,
		"seed import complete: recipes inserted=%d existing=%d; ingredients inserted=%d existing=%d\n",
		result.InsertedRecipes, result.ExistingRecipes,
		result.InsertedIngredients, result.ExistingIngredients,
	); err != nil {
		return fmt.Errorf("write seed summary: %w", err)
	}
	return nil
}
