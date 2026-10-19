//go:build integration

package postgres

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
	"github.com/Suhare00/mealprep-bot/internal/seed"
)

func TestSeedImporterCanonicalDatasetAndIdempotency(t *testing.T) {
	pool, repository := newTestRepository(t)
	dataset := loadCanonicalDataset(t)
	importer := NewSeedImporter(pool)

	result, err := importer.Import(t.Context(), dataset)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.InsertedRecipes != 90 || result.ExistingRecipes != 0 ||
		result.InsertedIngredients != 84 || result.ExistingIngredients != 0 {
		t.Fatalf("first Import() result = %+v", result)
	}
	assertSeedCounts(t, pool, 90, 84, 466, 155, 233)

	for _, id := range []domain.RecipeID{"R001", "R021"} {
		got, err := repository.GetByID(t.Context(), id)
		if err != nil {
			t.Fatalf("GetByID(%s) error = %v", id, err)
		}
		assertEntriesEqual(t, got, seedEntry(t, dataset, id))
	}

	result, err = importer.Import(t.Context(), dataset)
	if err != nil {
		t.Fatalf("second Import() error = %v", err)
	}
	if result.InsertedRecipes != 0 || result.ExistingRecipes != 90 ||
		result.InsertedIngredients != 0 || result.ExistingIngredients != 84 {
		t.Fatalf("second Import() result = %+v", result)
	}
	assertSeedCounts(t, pool, 90, 84, 466, 155, 233)
}

func TestSeedImporterConflictsRollBack(t *testing.T) {
	dataset := loadCanonicalDataset(t)

	t.Run("recipe", func(t *testing.T) {
		pool, repository := newTestRepository(t)
		conflicting := testEntry("R001")
		conflicting.Recipe.Name = "User-edited recipe"
		if err := repository.Create(t.Context(), conflicting); err != nil {
			t.Fatalf("create conflicting recipe: %v", err)
		}

		_, err := NewSeedImporter(pool).Import(t.Context(), dataset)
		if !errors.Is(err, ErrSeedConflict) {
			t.Fatalf("Import() error = %v, want ErrSeedConflict", err)
		}
		assertSeedCounts(t, pool, 1, 2, 2, 2, 2)
		got, err := repository.GetByID(t.Context(), "R001")
		if err != nil {
			t.Fatalf("GetByID(R001) error = %v", err)
		}
		assertEntriesEqual(t, got, conflicting)
	})

	t.Run("ingredient", func(t *testing.T) {
		pool, _ := newTestRepository(t)
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO ingredients (id, name) VALUES ($1, $2)`, "I001", "Something Else"); err != nil {
			t.Fatalf("insert conflicting ingredient: %v", err)
		}

		_, err := NewSeedImporter(pool).Import(t.Context(), dataset)
		if !errors.Is(err, ErrSeedConflict) {
			t.Fatalf("Import() error = %v, want ErrSeedConflict", err)
		}
		assertSeedCounts(t, pool, 0, 1, 0, 0, 0)
	})
}

func TestSeedImporterPreservesUnrelatedCatalogueData(t *testing.T) {
	pool, repository := newTestRepository(t)
	unrelated := testEntry("user-recipe-123")
	unrelated.Recipe.Name = "User recipe"
	if err := repository.Create(t.Context(), unrelated); err != nil {
		t.Fatalf("create unrelated recipe: %v", err)
	}

	if _, err := NewSeedImporter(pool).Import(t.Context(), loadCanonicalDataset(t)); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	assertSeedCounts(t, pool, 91, 86, 468, 157, 235)
	got, err := repository.GetByID(t.Context(), unrelated.Recipe.ID)
	if err != nil {
		t.Fatalf("GetByID(unrelated) error = %v", err)
	}
	assertEntriesEqual(t, got, unrelated)
}

func TestSeedImporterPreservesCanonicalRecipeEnrichment(t *testing.T) {
	pool, repository := newTestRepository(t)
	dataset := loadCanonicalDataset(t)
	importer := NewSeedImporter(pool)
	if _, err := importer.Import(t.Context(), dataset); err != nil {
		t.Fatalf("first Import() error = %v", err)
	}

	enriched, err := repository.GetByID(t.Context(), "R001")
	if err != nil {
		t.Fatal(err)
	}
	enrichEntry(&enriched)
	if err := repository.Update(t.Context(), enriched); err != nil {
		t.Fatalf("enrich canonical recipe: %v", err)
	}

	result, err := importer.Import(t.Context(), dataset)
	if err != nil {
		t.Fatalf("repeat Import() error = %v", err)
	}
	if result.ExistingRecipes != 90 || result.InsertedRecipes != 0 {
		t.Fatalf("repeat Import() result = %+v", result)
	}
	got, err := repository.GetByID(t.Context(), "R001")
	if err != nil {
		t.Fatal(err)
	}
	assertEntriesEqual(t, got, enriched)
	assertSeedCounts(t, pool, 90, 84, 466, 155, 233)
}

func TestSeedWorkflowRejectsMalformedInputBeforeDatabaseMutation(t *testing.T) {
	pool, _ := newTestRepository(t)
	directory := writeMalformedSeedFixture(t)

	if _, err := seed.Load(directory); err == nil {
		t.Fatal("Load() error = nil, want malformed boolean error")
	}
	assertSeedCounts(t, pool, 0, 0, 0, 0, 0)
}

func TestSeedImporterRejectsSubSecondDurationsBeforeDatabaseMutation(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*domain.Recipe)
		wantError string
	}{
		{
			name: "preparation duration",
			mutate: func(recipe *domain.Recipe) {
				recipe.PreparationDuration = 1500 * time.Millisecond
			},
			wantError: "preparation duration must use whole seconds",
		},
		{
			name: "cooking duration",
			mutate: func(recipe *domain.Recipe) {
				recipe.CookingDuration = 1500 * time.Millisecond
			},
			wantError: "cooking duration must use whole seconds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool, _ := newTestRepository(t)
			dataset := loadCanonicalDataset(t)
			test.mutate(&dataset.Entries[0].Recipe)

			_, err := NewSeedImporter(pool).Import(t.Context(), dataset)
			if err == nil {
				t.Fatalf("Import() error = nil, want text %q", test.wantError)
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Import() error = %q, want text %q", err, test.wantError)
			}
			assertSeedCounts(t, pool, 0, 0, 0, 0, 0)
		})
	}
}

func TestSeedImporterPersistenceFailureRollsBackEverything(t *testing.T) {
	pool, _ := newTestRepository(t)
	if _, err := pool.Exec(t.Context(),
		`ALTER TABLE recipes ADD CONSTRAINT reject_seed_recipe CHECK (id <> 'R045')`); err != nil {
		t.Fatalf("add induced failure constraint: %v", err)
	}

	if _, err := NewSeedImporter(pool).Import(t.Context(), loadCanonicalDataset(t)); err == nil {
		t.Fatal("Import() error = nil, want induced persistence failure")
	}
	assertSeedCounts(t, pool, 0, 0, 0, 0, 0)
}

func loadCanonicalDataset(t *testing.T) seed.Dataset {
	t.Helper()
	dataset, err := seed.Load(filepath.Join("..", "..", "seed"))
	if err != nil {
		t.Fatalf("load canonical seed: %v", err)
	}
	return dataset
}

func seedEntry(t *testing.T, dataset seed.Dataset, id domain.RecipeID) recipecatalogue.CatalogueEntry {
	t.Helper()
	for _, entry := range dataset.Entries {
		if entry.Recipe.ID == id {
			return entry
		}
	}
	t.Fatalf("seed recipe %s not found", id)
	return recipecatalogue.CatalogueEntry{}
}

func assertSeedCounts(
	t *testing.T,
	pool *pgxpool.Pool,
	recipes, ingredients, recipeIngredients, mealTypes, tags int,
) {
	t.Helper()
	var gotRecipes, gotIngredients, gotRecipeIngredients, gotMealTypes, gotTags int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM recipes),
			(SELECT count(*) FROM ingredients),
			(SELECT count(*) FROM recipe_ingredients),
			(SELECT count(*) FROM recipe_meal_types),
			(SELECT count(*) FROM recipe_tags)`,
	).Scan(&gotRecipes, &gotIngredients, &gotRecipeIngredients, &gotMealTypes, &gotTags); err != nil {
		t.Fatalf("count seed tables: %v", err)
	}
	got := []int{gotRecipes, gotIngredients, gotRecipeIngredients, gotMealTypes, gotTags}
	want := []int{recipes, ingredients, recipeIngredients, mealTypes, tags}
	labels := []string{"recipes", "ingredients", "recipe_ingredients", "recipe_meal_types", "recipe_tags"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s count = %d, want %d", labels[index], got[index], want[index])
		}
	}
}

func writeMalformedSeedFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	files := map[string]string{
		"recipes.csv": "recipe_id,name,category,servings,preparation_seconds,cooking_seconds,storage_days,freezer_friendly,difficulty,budget_level,reheat_score,batch_cooking_score,state\n" +
			"R001,Recipe,main,6,0,0,3,not-a-bool,easy,low,5,5,active\n",
		"ingredients.csv":        "ingredient_id,name\nI001,Rice\n",
		"recipe_ingredients.csv": "recipe_id,ingredient_id,quantity,unit\nR001,I001,1,kg\n",
		"recipe_meal_types.csv":  "recipe_id,meal_type\nR001,breakfast\n",
		"recipe_tags.csv":        "recipe_id,tag\nR001,quick\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("write malformed fixture %s: %v", name, err)
		}
	}
	return directory
}
