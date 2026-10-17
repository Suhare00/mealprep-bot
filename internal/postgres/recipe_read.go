package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

const recipeColumns = `
	id, name, category, servings, preparation_seconds, cooking_seconds,
	storage_days, freezer_friendly, difficulty, budget_level,
	reheat_score, batch_cooking_score, state`

type rowScanner interface {
	Scan(dest ...any) error
}

func loadEntry(ctx context.Context, tx pgx.Tx, id domain.RecipeID) (recipecatalogue.CatalogueEntry, error) {
	entry, err := scanEntry(tx.QueryRow(ctx,
		`SELECT `+recipeColumns+` FROM recipes WHERE id = $1`, string(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("%w: %s", recipecatalogue.ErrNotFound, id)
	}
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("select recipe %q: %w", id, err)
	}

	entries := []recipecatalogue.CatalogueEntry{entry}
	if err := loadRelations(ctx, tx, entries); err != nil {
		return recipecatalogue.CatalogueEntry{}, err
	}
	if err := validateStoredEntry(entries[0]); err != nil {
		return recipecatalogue.CatalogueEntry{}, err
	}

	return entries[0], nil
}

func loadActiveEntries(ctx context.Context, tx pgx.Tx) ([]recipecatalogue.CatalogueEntry, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+recipeColumns+` FROM recipes WHERE state = 'active' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select active recipes: %w", err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (recipecatalogue.CatalogueEntry, error) {
		return scanEntry(row)
	})
	if err != nil {
		return nil, fmt.Errorf("scan active recipes: %w", err)
	}
	if err := loadRelations(ctx, tx, entries); err != nil {
		return nil, err
	}
	for i := range entries {
		if err := validateStoredEntry(entries[i]); err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func scanEntry(row rowScanner) (recipecatalogue.CatalogueEntry, error) {
	var (
		id, name, category, difficulty, budgetLevel, state string
		servings, storageDays                              int
		preparationSeconds, cookingSeconds                 int64
		freezerFriendly                                    bool
		reheatScore, batchCookingScore                     int
	)

	if err := row.Scan(
		&id, &name, &category, &servings, &preparationSeconds, &cookingSeconds,
		&storageDays, &freezerFriendly, &difficulty, &budgetLevel,
		&reheatScore, &batchCookingScore, &state,
	); err != nil {
		return recipecatalogue.CatalogueEntry{}, err
	}

	preparationDuration, err := secondsToDuration(preparationSeconds)
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("map preparation duration: %w", err)
	}
	cookingDuration, err := secondsToDuration(cookingSeconds)
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("map cooking duration: %w", err)
	}

	return recipecatalogue.CatalogueEntry{Recipe: domain.Recipe{
		ID:                  domain.RecipeID(id),
		Name:                name,
		Category:            category,
		Servings:            servings,
		PreparationDuration: preparationDuration,
		CookingDuration:     cookingDuration,
		StorageDays:         storageDays,
		FreezerFriendly:     freezerFriendly,
		Difficulty:          domain.Difficulty(difficulty),
		BudgetLevel:         domain.BudgetLevel(budgetLevel),
		ReheatScore:         domain.Score(reheatScore),
		BatchCookingScore:   domain.Score(batchCookingScore),
		State:               domain.RecipeState(state),
	}}, nil
}

func loadRelations(ctx context.Context, tx pgx.Tx, entries []recipecatalogue.CatalogueEntry) error {
	if len(entries) == 0 {
		return nil
	}

	ids := make([]string, len(entries))
	entryIndexes := make(map[domain.RecipeID]int, len(entries))
	for i := range entries {
		ids[i] = string(entries[i].Recipe.ID)
		entryIndexes[entries[i].Recipe.ID] = i
	}

	mealTypeRows, err := tx.Query(ctx, `
		SELECT recipe_id, meal_type
		FROM recipe_meal_types
		WHERE recipe_id = ANY($1::text[])
		ORDER BY recipe_id, meal_type`, ids)
	if err != nil {
		return fmt.Errorf("select meal types: %w", err)
	}
	for mealTypeRows.Next() {
		var recipeID, mealType string
		if err := mealTypeRows.Scan(&recipeID, &mealType); err != nil {
			mealTypeRows.Close()
			return fmt.Errorf("scan meal type: %w", err)
		}
		index := entryIndexes[domain.RecipeID(recipeID)]
		entries[index].Recipe.MealTypes = append(entries[index].Recipe.MealTypes, domain.MealType(mealType))
	}
	if err := mealTypeRows.Err(); err != nil {
		mealTypeRows.Close()
		return fmt.Errorf("iterate meal types: %w", err)
	}
	mealTypeRows.Close()

	tagRows, err := tx.Query(ctx, `
		SELECT recipe_id, tag
		FROM recipe_tags
		WHERE recipe_id = ANY($1::text[])
		ORDER BY recipe_id, tag`, ids)
	if err != nil {
		return fmt.Errorf("select tags: %w", err)
	}
	for tagRows.Next() {
		var recipeID, tag string
		if err := tagRows.Scan(&recipeID, &tag); err != nil {
			tagRows.Close()
			return fmt.Errorf("scan tag: %w", err)
		}
		index := entryIndexes[domain.RecipeID(recipeID)]
		entries[index].Recipe.Tags = append(entries[index].Recipe.Tags, tag)
	}
	if err := tagRows.Err(); err != nil {
		tagRows.Close()
		return fmt.Errorf("iterate tags: %w", err)
	}
	tagRows.Close()

	ingredientRows, err := tx.Query(ctx, `
		SELECT ri.recipe_id, i.id, i.name, ri.quantity, ri.unit
		FROM recipe_ingredients ri
		JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE ri.recipe_id = ANY($1::text[])
		ORDER BY ri.recipe_id, i.id`, ids)
	if err != nil {
		return fmt.Errorf("select ingredients: %w", err)
	}
	for ingredientRows.Next() {
		var recipeID, ingredientID, ingredientName, unit string
		var quantity float64
		if err := ingredientRows.Scan(&recipeID, &ingredientID, &ingredientName, &quantity, &unit); err != nil {
			ingredientRows.Close()
			return fmt.Errorf("scan ingredient: %w", err)
		}
		index := entryIndexes[domain.RecipeID(recipeID)]
		id := domain.IngredientID(ingredientID)
		entries[index].Ingredients = append(entries[index].Ingredients, domain.Ingredient{ID: id, Name: ingredientName})
		entries[index].Recipe.Ingredients = append(entries[index].Recipe.Ingredients, domain.RecipeIngredient{
			IngredientID: id,
			Quantity:     quantity,
			Unit:         unit,
		})
	}
	if err := ingredientRows.Err(); err != nil {
		ingredientRows.Close()
		return fmt.Errorf("iterate ingredients: %w", err)
	}
	ingredientRows.Close()

	return nil
}

func validateStoredEntry(entry recipecatalogue.CatalogueEntry) error {
	if err := validateEntry(entry); err != nil {
		return fmt.Errorf("stored recipe %q is invalid: %w", entry.Recipe.ID, err)
	}
	return nil
}

func secondsToDuration(seconds int64) (time.Duration, error) {
	if seconds < 0 || seconds > math.MaxInt64/int64(time.Second) {
		return 0, fmt.Errorf("seconds value %d is outside time.Duration range", seconds)
	}
	return time.Duration(seconds) * time.Second, nil
}
