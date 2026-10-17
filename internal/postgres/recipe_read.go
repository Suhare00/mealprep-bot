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

func loadEntry(ctx context.Context, tx pgx.Tx, id domain.RecipeID) (recipecatalogue.CatalogueEntry, error) {
	var (
		name, category, difficulty, budgetLevel, state string
		servings, storageDays                          int
		preparationSeconds, cookingSeconds             int64
		freezerFriendly                                bool
		reheatScore, batchCookingScore                 int
	)

	err := tx.QueryRow(ctx, `
		SELECT name, category, servings, preparation_seconds, cooking_seconds,
		       storage_days, freezer_friendly, difficulty, budget_level,
		       reheat_score, batch_cooking_score, state
		FROM recipes
		WHERE id = $1`, string(id)).Scan(
		&name, &category, &servings, &preparationSeconds, &cookingSeconds,
		&storageDays, &freezerFriendly, &difficulty, &budgetLevel,
		&reheatScore, &batchCookingScore, &state,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("%w: %s", recipecatalogue.ErrNotFound, id)
	}
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("select recipe %q: %w", id, err)
	}

	preparationDuration, err := secondsToDuration(preparationSeconds)
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("map preparation duration: %w", err)
	}
	cookingDuration, err := secondsToDuration(cookingSeconds)
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("map cooking duration: %w", err)
	}

	entry := recipecatalogue.CatalogueEntry{Recipe: domain.Recipe{
		ID:                  id,
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
	}}

	mealTypeRows, err := tx.Query(ctx,
		`SELECT meal_type FROM recipe_meal_types WHERE recipe_id = $1 ORDER BY meal_type`, string(id))
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("select meal types: %w", err)
	}
	entry.Recipe.MealTypes, err = pgx.CollectRows(mealTypeRows, func(row pgx.CollectableRow) (domain.MealType, error) {
		var value string
		if err := row.Scan(&value); err != nil {
			return "", err
		}
		return domain.MealType(value), nil
	})
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("scan meal types: %w", err)
	}

	tagRows, err := tx.Query(ctx,
		`SELECT tag FROM recipe_tags WHERE recipe_id = $1 ORDER BY tag`, string(id))
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("select tags: %w", err)
	}
	entry.Recipe.Tags, err = pgx.CollectRows(tagRows, func(row pgx.CollectableRow) (string, error) {
		var value string
		if err := row.Scan(&value); err != nil {
			return "", err
		}
		return value, nil
	})
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("scan tags: %w", err)
	}

	ingredientRows, err := tx.Query(ctx, `
		SELECT i.id, i.name, ri.quantity, ri.unit
		FROM recipe_ingredients ri
		JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE ri.recipe_id = $1
		ORDER BY i.id`, string(id))
	if err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("select ingredients: %w", err)
	}
	for ingredientRows.Next() {
		var ingredientID, ingredientName, unit string
		var quantity float64
		if err := ingredientRows.Scan(&ingredientID, &ingredientName, &quantity, &unit); err != nil {
			ingredientRows.Close()
			return recipecatalogue.CatalogueEntry{}, fmt.Errorf("scan ingredient: %w", err)
		}
		id := domain.IngredientID(ingredientID)
		entry.Ingredients = append(entry.Ingredients, domain.Ingredient{ID: id, Name: ingredientName})
		entry.Recipe.Ingredients = append(entry.Recipe.Ingredients, domain.RecipeIngredient{
			IngredientID: id,
			Quantity:     quantity,
			Unit:         unit,
		})
	}
	if err := ingredientRows.Err(); err != nil {
		ingredientRows.Close()
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("iterate ingredients: %w", err)
	}
	ingredientRows.Close()

	if err := validateEntry(entry); err != nil {
		return recipecatalogue.CatalogueEntry{}, fmt.Errorf("stored recipe %q is invalid: %w", id, err)
	}

	return entry, nil
}

func secondsToDuration(seconds int64) (time.Duration, error) {
	if seconds < 0 || seconds > math.MaxInt64/int64(time.Second) {
		return 0, fmt.Errorf("seconds value %d is outside time.Duration range", seconds)
	}
	return time.Duration(seconds) * time.Second, nil
}
