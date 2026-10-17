// Package postgres implements PostgreSQL adapters for the application.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

// RecipeRepository persists complete recipe catalogue entries in PostgreSQL.
type RecipeRepository struct {
	pool *pgxpool.Pool
}

var _ recipecatalogue.Repository = (*RecipeRepository)(nil)

// NewRecipeRepository creates a PostgreSQL recipe repository backed by pool.
func NewRecipeRepository(pool *pgxpool.Pool) *RecipeRepository {
	return &RecipeRepository{pool: pool}
}

// Create stores a complete recipe atomically.
func (r *RecipeRepository) Create(ctx context.Context, entry recipecatalogue.CatalogueEntry) error {
	if err := validateEntry(entry); err != nil {
		return fmt.Errorf("validate recipe catalogue entry: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create recipe transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := insertRecipe(ctx, tx, entry.Recipe); err != nil {
		return fmt.Errorf("insert recipe: %w", err)
	}
	if err := insertRelations(ctx, tx, entry); err != nil {
		return fmt.Errorf("insert recipe relations: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create recipe transaction: %w", err)
	}

	return nil
}

// GetByID retrieves a recipe regardless of active or inactive state.
func (r *RecipeRepository) GetByID(ctx context.Context, id domain.RecipeID) (recipecatalogue.CatalogueEntry, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("begin get recipe transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	entry, err := loadEntry(ctx, tx, id)
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("commit get recipe transaction: %w", err)
	}

	return entry, nil
}

// Update replaces a complete recipe and its relation sets atomically.
func (r *RecipeRepository) Update(ctx context.Context, entry recipecatalogue.CatalogueEntry) error {
	if err := validateEntry(entry); err != nil {
		return fmt.Errorf("validate recipe catalogue entry: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin update recipe transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	found, err := updateRecipe(ctx, tx, entry.Recipe)
	if err != nil {
		return fmt.Errorf("update recipe: %w", err)
	}
	if !found {
		return fmt.Errorf("%w: %s", recipecatalogue.ErrNotFound, entry.Recipe.ID)
	}
	if err := clearRelations(ctx, tx, entry.Recipe.ID); err != nil {
		return fmt.Errorf("clear recipe relations: %w", err)
	}
	if err := insertRelations(ctx, tx, entry); err != nil {
		return fmt.Errorf("replace recipe relations: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit update recipe transaction: %w", err)
	}

	return nil
}

// ListActive returns all active recipes ordered by ID.
func (r *RecipeRepository) ListActive(ctx context.Context) ([]recipecatalogue.CatalogueEntry, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin list recipes transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT id FROM recipes WHERE state = 'active' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list active recipe IDs: %w", err)
	}
	ids, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.RecipeID, error) {
		var id string
		if err := row.Scan(&id); err != nil {
			return "", err
		}
		return domain.RecipeID(id), nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan active recipe IDs: %w", err)
	}

	entries := make([]recipecatalogue.CatalogueEntry, 0, len(ids))
	for _, id := range ids {
		entry, err := loadEntry(ctx, tx, id)
		if err != nil {
			return nil, fmt.Errorf("load active recipe %q: %w", id, err)
		}
		entries = append(entries, entry)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit list recipes transaction: %w", err)
	}

	return entries, nil
}

// Disable marks a recipe inactive without deleting its relations.
func (r *RecipeRepository) Disable(ctx context.Context, id domain.RecipeID) error {
	result, err := r.pool.Exec(ctx, `UPDATE recipes SET state = 'inactive' WHERE id = $1`, string(id))
	if err != nil {
		return fmt.Errorf("disable recipe %q: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", recipecatalogue.ErrNotFound, id)
	}

	return nil
}

func validateEntry(entry recipecatalogue.CatalogueEntry) error {
	if err := entry.Recipe.Validate(); err != nil {
		return err
	}
	if entry.Recipe.PreparationDuration%time.Second != 0 {
		return errors.New("preparation duration must use whole seconds")
	}
	if entry.Recipe.CookingDuration%time.Second != 0 {
		return errors.New("cooking duration must use whole seconds")
	}

	ingredients := make(map[domain.IngredientID]domain.Ingredient, len(entry.Ingredients))
	for index, ingredient := range entry.Ingredients {
		if err := ingredient.Validate(); err != nil {
			return fmt.Errorf("ingredient at index %d: %w", index, err)
		}
		if _, duplicate := ingredients[ingredient.ID]; duplicate {
			return fmt.Errorf("ingredient %q is duplicated in catalogue entry", ingredient.ID)
		}
		ingredients[ingredient.ID] = ingredient
	}

	referenced := make(map[domain.IngredientID]struct{}, len(entry.Recipe.Ingredients))
	for _, relation := range entry.Recipe.Ingredients {
		if _, ok := ingredients[relation.IngredientID]; !ok {
			return fmt.Errorf("ingredient %q has no catalogue record", relation.IngredientID)
		}
		if _, duplicate := referenced[relation.IngredientID]; duplicate {
			return fmt.Errorf("ingredient %q is duplicated in recipe relations", relation.IngredientID)
		}
		referenced[relation.IngredientID] = struct{}{}
	}
	for id := range ingredients {
		if _, ok := referenced[id]; !ok {
			return fmt.Errorf("ingredient %q is not referenced by the recipe", id)
		}
	}

	return nil
}
