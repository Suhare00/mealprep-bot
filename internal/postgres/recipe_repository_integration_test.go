//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

const testDatabaseURLEnv = "TEST_DATABASE_URL"

func TestMigrationsApplyAndReverse(t *testing.T) {
	pool := openTestPool(t)
	applyMigration(t, pool, "000001_recipe_catalogue.down.sql")
	applyMigration(t, pool, "000001_recipe_catalogue.up.sql")

	for _, table := range []string{
		"recipes", "ingredients", "recipe_meal_types", "recipe_tags", "recipe_ingredients",
	} {
		var exists bool
		if err := pool.QueryRow(t.Context(),
			`SELECT to_regclass($1) IS NOT NULL`, "public."+table,
		).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s was not created", table)
		}
	}

	applyMigration(t, pool, "000001_recipe_catalogue.down.sql")
	var exists bool
	if err := pool.QueryRow(t.Context(), `SELECT to_regclass('public.recipes') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("check reversed migration: %v", err)
	}
	if exists {
		t.Fatal("recipes table still exists after down migration")
	}
}

func TestRecipeRepositoryCreateGetRoundTrip(t *testing.T) {
	pool, repository := newTestRepository(t)
	entry := testEntry("recipe-round-trip")

	if err := repository.Create(t.Context(), entry); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repository.GetByID(t.Context(), entry.Recipe.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	assertEntriesEqual(t, got, entry)

	var quantityText string
	if err := pool.QueryRow(t.Context(), `
		SELECT quantity::text FROM recipe_ingredients
		WHERE recipe_id = $1 AND ingredient_id = $2`,
		string(entry.Recipe.ID), "ingredient-rice",
	).Scan(&quantityText); err != nil {
		t.Fatalf("read stored quantity: %v", err)
	}
	if quantityText != "1.25" {
		t.Fatalf("stored quantity = %q, want 1.25", quantityText)
	}
}

func TestRecipeRepositoryGetByIDUsesStableSnapshot(t *testing.T) {
	hook := &queryEndHook{match: "FROM recipes WHERE id = $1"}
	readerPool := openTestPoolWithTracer(t, hook)
	applyMigration(t, readerPool, "000001_recipe_catalogue.down.sql")
	applyMigration(t, readerPool, "000001_recipe_catalogue.up.sql")
	repository := NewRecipeRepository(readerPool)
	writerPool := openTestPool(t)

	original := testEntry("recipe-stable-snapshot")
	if err := repository.Create(t.Context(), original); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated := original
	updated.Recipe.Name = "Updated concurrently"
	updated.Recipe.Tags = []string{"concurrent"}
	hook.run = func() error {
		tx, err := writerPool.Begin(t.Context())
		if err != nil {
			return fmt.Errorf("begin concurrent update: %w", err)
		}
		defer func() { _ = tx.Rollback(t.Context()) }()
		if _, err := tx.Exec(t.Context(), `UPDATE recipes SET name = $2 WHERE id = $1`,
			string(updated.Recipe.ID), updated.Recipe.Name); err != nil {
			return fmt.Errorf("update recipe concurrently: %w", err)
		}
		if _, err := tx.Exec(t.Context(), `DELETE FROM recipe_tags WHERE recipe_id = $1`,
			string(updated.Recipe.ID)); err != nil {
			return fmt.Errorf("delete tags concurrently: %w", err)
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO recipe_tags (recipe_id, tag) VALUES ($1, $2)`,
			string(updated.Recipe.ID), updated.Recipe.Tags[0]); err != nil {
			return fmt.Errorf("insert tag concurrently: %w", err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			return fmt.Errorf("commit concurrent update: %w", err)
		}
		return nil
	}

	got, err := repository.GetByID(t.Context(), original.Recipe.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if hook.err != nil {
		t.Fatalf("concurrent update: %v", hook.err)
	}
	if !hook.fired.Load() {
		t.Fatal("concurrent update hook did not run")
	}
	assertEntriesEqual(t, got, original)

	fresh, err := repository.GetByID(t.Context(), original.Recipe.ID)
	if err != nil {
		t.Fatalf("second GetByID() error = %v", err)
	}
	assertEntriesEqual(t, fresh, updated)
}

func TestRecipeRepositoryUpdateSynchronizesCompleteEntry(t *testing.T) {
	_, repository := newTestRepository(t)
	entry := testEntry("recipe-update")
	if err := repository.Create(t.Context(), entry); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated := entry
	updated.Recipe.Name = "Updated breakfast bowl"
	updated.Recipe.Category = "breakfast"
	updated.Recipe.Servings = 2
	updated.Recipe.PreparationDuration = 5 * time.Minute
	updated.Recipe.CookingDuration = 10 * time.Minute
	updated.Recipe.StorageDays = 2
	updated.Recipe.FreezerFriendly = false
	updated.Recipe.Difficulty = domain.DifficultyMedium
	updated.Recipe.BudgetLevel = domain.BudgetLevelMedium
	updated.Recipe.ReheatScore = 3
	updated.Recipe.BatchCookingScore = 4
	updated.Recipe.MealTypes = []domain.MealType{domain.MealTypeBreakfast}
	updated.Recipe.Tags = []string{"quick"}
	updated.Recipe.Ingredients = []domain.RecipeIngredient{
		{IngredientID: "ingredient-rice", Quantity: 0.75, Unit: "kg"},
		{IngredientID: "ingredient-tofu", Quantity: 300, Unit: "g"},
	}
	updated.Ingredients = []domain.Ingredient{
		{ID: "ingredient-rice", Name: "Rice"},
		{ID: "ingredient-tofu", Name: "Tofu"},
	}

	if err := repository.Update(t.Context(), updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, err := repository.GetByID(t.Context(), entry.Recipe.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	assertEntriesEqual(t, got, updated)
}

func TestRecipeRepositoryDisablePreservesEntryAndListActiveFiltersIt(t *testing.T) {
	_, repository := newTestRepository(t)
	disabled := testEntry("recipe-disabled")
	active := testEntry("recipe-active")
	active.Recipe.Name = "Still active"

	for _, entry := range []recipecatalogue.CatalogueEntry{disabled, active} {
		if err := repository.Create(t.Context(), entry); err != nil {
			t.Fatalf("Create(%s) error = %v", entry.Recipe.ID, err)
		}
	}
	if err := repository.Disable(t.Context(), disabled.Recipe.ID); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	disabled.Recipe.State = domain.RecipeStateInactive
	got, err := repository.GetByID(t.Context(), disabled.Recipe.ID)
	if err != nil {
		t.Fatalf("GetByID(disabled) error = %v", err)
	}
	assertEntriesEqual(t, got, disabled)

	entries, err := repository.ListActive(t.Context())
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Recipe.ID != active.Recipe.ID {
		t.Fatalf("ListActive() IDs = %v, want [%s]", entryIDs(entries), active.Recipe.ID)
	}
}

func TestRecipeRepositoryListActiveBulkLoadsCompleteEntriesInOrder(t *testing.T) {
	counter := &selectCounter{}
	pool := openTestPoolWithTracer(t, counter)
	applyMigration(t, pool, "000001_recipe_catalogue.down.sql")
	applyMigration(t, pool, "000001_recipe_catalogue.up.sql")
	repository := NewRecipeRepository(pool)

	last := testEntry("recipe-z")
	last.Recipe.Name = "Last recipe"
	first := testEntry("recipe-a")
	first.Recipe.Name = "First recipe"
	first.Recipe.MealTypes = []domain.MealType{domain.MealTypeDinner}
	first.Recipe.Tags = []string{"quick"}
	first.Recipe.Ingredients[0].Quantity = 2.5

	for _, entry := range []recipecatalogue.CatalogueEntry{last, first} {
		if err := repository.Create(t.Context(), entry); err != nil {
			t.Fatalf("Create(%s) error = %v", entry.Recipe.ID, err)
		}
	}

	counter.count.Store(0)
	got, err := repository.ListActive(t.Context())
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if count := counter.count.Load(); count != 4 {
		t.Fatalf("ListActive() SELECT count = %d, want 4", count)
	}

	want := []recipecatalogue.CatalogueEntry{sortedEntry(first), sortedEntry(last)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListActive() mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestRecipeRepositoryMapsMissingRecipe(t *testing.T) {
	_, repository := newTestRepository(t)
	missingID := domain.RecipeID("missing")

	if _, err := repository.GetByID(t.Context(), missingID); !errors.Is(err, recipecatalogue.ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
	entry := testEntry(string(missingID))
	if err := repository.Update(t.Context(), entry); !errors.Is(err, recipecatalogue.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
	if err := repository.Disable(t.Context(), missingID); !errors.Is(err, recipecatalogue.ErrNotFound) {
		t.Fatalf("Disable() error = %v, want ErrNotFound", err)
	}
}

func TestRecipeRepositoryTransactionsRollBack(t *testing.T) {
	_, repository := newTestRepository(t)
	original := testEntry("recipe-original")
	if err := repository.Create(t.Context(), original); err != nil {
		t.Fatalf("seed original recipe: %v", err)
	}

	t.Run("create", func(t *testing.T) {
		conflicting := testEntry("recipe-conflicting-create")
		conflicting.Ingredients[0].Name = "Brown rice"
		if err := repository.Create(t.Context(), conflicting); err == nil {
			t.Fatal("Create() error = nil, want ingredient conflict")
		}
		if _, err := repository.GetByID(t.Context(), conflicting.Recipe.ID); !errors.Is(err, recipecatalogue.ErrNotFound) {
			t.Fatalf("GetByID() error = %v, want rolled-back recipe to be missing", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		conflicting := original
		conflicting.Recipe.Name = "Must roll back"
		conflicting.Ingredients = append([]domain.Ingredient(nil), original.Ingredients...)
		conflicting.Ingredients[0].Name = "Brown rice"
		if err := repository.Update(t.Context(), conflicting); err == nil {
			t.Fatal("Update() error = nil, want ingredient conflict")
		}

		got, err := repository.GetByID(t.Context(), original.Recipe.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		assertEntriesEqual(t, got, original)
	})
}

func TestRecipeSchemaConstraints(t *testing.T) {
	pool, _ := newTestRepository(t)
	ctx := t.Context()

	recipeArgs := func() []any {
		return []any{"direct-recipe", "Direct recipe", "main", 2, int64(0), int64(0), 1, false, "easy", "low", 1, 1, "active"}
	}
	insertRecipeSQL := `
		INSERT INTO recipes (
			id, name, category, servings, preparation_seconds, cooking_seconds,
			storage_days, freezer_friendly, difficulty, budget_level,
			reheat_score, batch_cooking_score, state
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	tests := []struct {
		name   string
		mutate func([]any)
	}{
		{name: "blank ID", mutate: func(args []any) { args[0] = " " }},
		{name: "non-positive servings", mutate: func(args []any) { args[3] = 0 }},
		{name: "negative duration", mutate: func(args []any) { args[4] = int64(-1) }},
		{name: "invalid difficulty", mutate: func(args []any) { args[8] = "hard" }},
		{name: "score below range", mutate: func(args []any) { args[10] = 0 }},
		{name: "invalid state", mutate: func(args []any) { args[12] = "deleted" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := recipeArgs()
			test.mutate(args)
			if _, err := pool.Exec(ctx, insertRecipeSQL, args...); err == nil {
				t.Fatal("constraint accepted invalid recipe row")
			}
		})
	}

	if _, err := pool.Exec(ctx, insertRecipeSQL, recipeArgs()...); err != nil {
		t.Fatalf("insert valid direct recipe: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ingredients (id, name) VALUES ($1, $2)`, "direct-ingredient", "Rice"); err != nil {
		t.Fatalf("insert valid direct ingredient: %v", err)
	}

	relationTests := []struct {
		name  string
		query string
		args  []any
	}{
		{name: "invalid meal type", query: `INSERT INTO recipe_meal_types (recipe_id, meal_type) VALUES ($1, $2)`, args: []any{"direct-recipe", "snack"}},
		{name: "blank tag", query: `INSERT INTO recipe_tags (recipe_id, tag) VALUES ($1, $2)`, args: []any{"direct-recipe", " "}},
		{name: "non-positive quantity", query: `INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, unit) VALUES ($1, $2, $3, $4)`, args: []any{"direct-recipe", "direct-ingredient", 0, "kg"}},
		{name: "missing ingredient reference", query: `INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, unit) VALUES ($1, $2, $3, $4)`, args: []any{"direct-recipe", "missing", 1, "kg"}},
	}
	for _, test := range relationTests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, test.query, test.args...); err == nil {
				t.Fatal("constraint accepted invalid relation row")
			}
		})
	}
}

func newTestRepository(t *testing.T) (*pgxpool.Pool, *RecipeRepository) {
	t.Helper()
	pool := openTestPool(t)
	applyMigration(t, pool, "000001_recipe_catalogue.down.sql")
	applyMigration(t, pool, "000001_recipe_catalogue.up.sql")
	return pool, NewRecipeRepository(pool)
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	return openTestPoolWithTracer(t, nil)
}

func openTestPoolWithTracer(t *testing.T, tracer pgx.QueryTracer) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv(testDatabaseURLEnv)
	if databaseURL == "" {
		t.Fatalf("%s is required for integration tests", testDatabaseURLEnv)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database configuration: %v", err)
	}
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	if err := pool.Ping(t.Context()); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type selectCounter struct {
	count atomic.Int64
}

func (c *selectCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(data.SQL)), "SELECT") {
		c.count.Add(1)
	}
	return ctx
}

func (*selectCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

type queryEndHook struct {
	match string
	run   func() error
	fired atomic.Bool
	err   error
}

type querySQLContextKey struct{}

func (*queryEndHook) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, querySQLContextKey{}, data.SQL)
}

func (h *queryEndHook) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	query, _ := ctx.Value(querySQLContextKey{}).(string)
	if strings.Contains(query, h.match) && h.fired.CompareAndSwap(false, true) {
		h.err = h.run()
	}
}

func applyMigration(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	if _, err := pool.Exec(t.Context(), string(contents)); err != nil {
		t.Fatalf("apply migration %s: %v", name, err)
	}
}

func testEntry(id string) recipecatalogue.CatalogueEntry {
	return recipecatalogue.CatalogueEntry{
		Recipe: domain.Recipe{
			ID:                  domain.RecipeID(id),
			Name:                "Rice and vegetables",
			MealTypes:           []domain.MealType{domain.MealTypeLunch, domain.MealTypeDinner},
			Category:            "main",
			Servings:            6,
			PreparationDuration: 15 * time.Minute,
			CookingDuration:     30 * time.Minute,
			StorageDays:         3,
			FreezerFriendly:     true,
			Difficulty:          domain.DifficultyEasy,
			BudgetLevel:         domain.BudgetLevelLow,
			ReheatScore:         4,
			BatchCookingScore:   5,
			Tags:                []string{"vegetarian", "batch"},
			State:               domain.RecipeStateActive,
			Ingredients: []domain.RecipeIngredient{
				{IngredientID: "ingredient-rice", Quantity: 1.25, Unit: "kg"},
				{IngredientID: "ingredient-carrot", Quantity: 500, Unit: "g"},
			},
		},
		Ingredients: []domain.Ingredient{
			{ID: "ingredient-rice", Name: "Rice"},
			{ID: "ingredient-carrot", Name: "Carrot"},
		},
	}
}

func assertEntriesEqual(t *testing.T, got, want recipecatalogue.CatalogueEntry) {
	t.Helper()
	got = sortedEntry(got)
	want = sortedEntry(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalogue entry mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func sortedEntry(entry recipecatalogue.CatalogueEntry) recipecatalogue.CatalogueEntry {
	entry.Recipe.MealTypes = append([]domain.MealType(nil), entry.Recipe.MealTypes...)
	entry.Recipe.Tags = append([]string(nil), entry.Recipe.Tags...)
	entry.Recipe.Ingredients = append([]domain.RecipeIngredient(nil), entry.Recipe.Ingredients...)
	entry.Ingredients = append([]domain.Ingredient(nil), entry.Ingredients...)
	sort.Slice(entry.Recipe.MealTypes, func(i, j int) bool { return entry.Recipe.MealTypes[i] < entry.Recipe.MealTypes[j] })
	sort.Strings(entry.Recipe.Tags)
	sort.Slice(entry.Recipe.Ingredients, func(i, j int) bool {
		return entry.Recipe.Ingredients[i].IngredientID < entry.Recipe.Ingredients[j].IngredientID
	})
	sort.Slice(entry.Ingredients, func(i, j int) bool { return entry.Ingredients[i].ID < entry.Ingredients[j].ID })
	return entry
}

func entryIDs(entries []recipecatalogue.CatalogueEntry) []domain.RecipeID {
	ids := make([]domain.RecipeID, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.Recipe.ID)
	}
	return ids
}
