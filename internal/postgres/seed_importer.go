package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
	"github.com/Suhare00/mealprep-bot/internal/seed"
)

// ErrSeedConflict indicates that a stable seed ID already has different catalogue data.
var ErrSeedConflict = errors.New("seed conflict")

// SeedResult describes how much canonical data was inserted or already present.
type SeedResult struct {
	InsertedRecipes     int
	ExistingRecipes     int
	InsertedIngredients int
	ExistingIngredients int
}

// SeedImporter atomically imports validated catalogue seed data into PostgreSQL.
type SeedImporter struct {
	pool *pgxpool.Pool
}

// NewSeedImporter creates a PostgreSQL seed importer backed by pool.
func NewSeedImporter(pool *pgxpool.Pool) *SeedImporter {
	return &SeedImporter{pool: pool}
}

// Import validates and applies the complete dataset in one transaction.
func (i *SeedImporter) Import(ctx context.Context, dataset seed.Dataset) (SeedResult, error) {
	if err := dataset.Validate(); err != nil {
		return SeedResult{}, fmt.Errorf("validate seed dataset: %w", err)
	}

	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return SeedResult{}, fmt.Errorf("begin seed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result := SeedResult{}
	for _, ingredient := range dataset.Ingredients {
		inserted, err := ensureSeedIngredient(ctx, tx, ingredient)
		if err != nil {
			return SeedResult{}, err
		}
		if inserted {
			result.InsertedIngredients++
		} else {
			result.ExistingIngredients++
		}
	}

	for _, entry := range dataset.Entries {
		inserted, err := ensureSeedRecipe(ctx, tx, entry)
		if err != nil {
			return SeedResult{}, err
		}
		if inserted {
			result.InsertedRecipes++
		} else {
			result.ExistingRecipes++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return SeedResult{}, fmt.Errorf("commit seed transaction: %w", err)
	}
	return result, nil
}

func ensureSeedIngredient(ctx context.Context, tx pgx.Tx, ingredient domain.Ingredient) (bool, error) {
	result, err := tx.Exec(ctx,
		`INSERT INTO ingredients (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		string(ingredient.ID), ingredient.Name,
	)
	if err != nil {
		return false, fmt.Errorf("insert seed ingredient %q: %w", ingredient.ID, err)
	}
	if result.RowsAffected() == 1 {
		return true, nil
	}

	var storedName string
	if err := tx.QueryRow(ctx, `SELECT name FROM ingredients WHERE id = $1 FOR UPDATE`,
		string(ingredient.ID)).Scan(&storedName); err != nil {
		return false, fmt.Errorf("read existing seed ingredient %q: %w", ingredient.ID, err)
	}
	if storedName != ingredient.Name {
		return false, fmt.Errorf("%w: ingredient %q is %q in PostgreSQL, seed requires %q",
			ErrSeedConflict, ingredient.ID, storedName, ingredient.Name)
	}
	return false, nil
}

func ensureSeedRecipe(ctx context.Context, tx pgx.Tx, expected recipecatalogue.CatalogueEntry) (bool, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM recipes WHERE id = $1 FOR UPDATE`, string(expected.Recipe.ID)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := insertRecipe(ctx, tx, expected.Recipe); err != nil {
			return false, fmt.Errorf("insert seed recipe %q: %w", expected.Recipe.ID, err)
		}
		if err := insertOwnedRelations(ctx, tx, expected); err != nil {
			return false, fmt.Errorf("insert seed recipe %q relations: %w", expected.Recipe.ID, err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock existing seed recipe %q: %w", expected.Recipe.ID, err)
	}

	actual, err := loadEntryUnchecked(ctx, tx, expected.Recipe.ID)
	if err != nil {
		return false, fmt.Errorf("load existing seed recipe %q: %w", expected.Recipe.ID, err)
	}
	if !reflect.DeepEqual(normalizedEntry(actual), normalizedEntry(expected)) {
		return false, fmt.Errorf("%w: recipe %q differs from the canonical seed",
			ErrSeedConflict, expected.Recipe.ID)
	}
	return false, nil
}

func normalizedEntry(entry recipecatalogue.CatalogueEntry) recipecatalogue.CatalogueEntry {
	entry.Recipe.MealTypes = append([]domain.MealType(nil), entry.Recipe.MealTypes...)
	entry.Recipe.Tags = append([]string(nil), entry.Recipe.Tags...)
	entry.Recipe.Ingredients = append([]domain.RecipeIngredient(nil), entry.Recipe.Ingredients...)
	entry.Ingredients = append([]domain.Ingredient(nil), entry.Ingredients...)
	sort.Slice(entry.Recipe.MealTypes, func(i, j int) bool {
		return entry.Recipe.MealTypes[i] < entry.Recipe.MealTypes[j]
	})
	sort.Strings(entry.Recipe.Tags)
	sort.Slice(entry.Recipe.Ingredients, func(i, j int) bool {
		return entry.Recipe.Ingredients[i].IngredientID < entry.Recipe.Ingredients[j].IngredientID
	})
	sort.Slice(entry.Ingredients, func(i, j int) bool {
		return entry.Ingredients[i].ID < entry.Ingredients[j].ID
	})
	return entry
}
