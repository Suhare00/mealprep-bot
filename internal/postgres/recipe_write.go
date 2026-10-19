package postgres

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

func insertRecipe(ctx context.Context, tx pgx.Tx, recipe domain.Recipe) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO recipes (
			id, name, category, servings, preparation_seconds, cooking_seconds,
			storage_days, freezer_friendly, difficulty, budget_level,
			reheat_score, batch_cooking_score, state
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		string(recipe.ID), recipe.Name, recipe.Category, recipe.Servings,
		int64(recipe.PreparationDuration.Seconds()), int64(recipe.CookingDuration.Seconds()),
		recipe.StorageDays, recipe.FreezerFriendly, string(recipe.Difficulty),
		string(recipe.BudgetLevel), int(recipe.ReheatScore), int(recipe.BatchCookingScore),
		string(recipe.State),
	)
	return err
}

func updateRecipe(ctx context.Context, tx pgx.Tx, recipe domain.Recipe) (bool, error) {
	result, err := tx.Exec(ctx, `
		UPDATE recipes SET
			name = $2,
			category = $3,
			servings = $4,
			preparation_seconds = $5,
			cooking_seconds = $6,
			storage_days = $7,
			freezer_friendly = $8,
			difficulty = $9,
			budget_level = $10,
			reheat_score = $11,
			batch_cooking_score = $12,
			state = $13
		WHERE id = $1`,
		string(recipe.ID), recipe.Name, recipe.Category, recipe.Servings,
		int64(recipe.PreparationDuration.Seconds()), int64(recipe.CookingDuration.Seconds()),
		recipe.StorageDays, recipe.FreezerFriendly, string(recipe.Difficulty),
		string(recipe.BudgetLevel), int(recipe.ReheatScore), int(recipe.BatchCookingScore),
		string(recipe.State),
	)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func clearRelations(ctx context.Context, tx pgx.Tx, recipeID domain.RecipeID) error {
	for _, table := range []string{
		"recipe_steps", "recipe_images", "recipe_cover_images", "recipe_media",
		"recipe_ingredients", "recipe_tags", "recipe_meal_types",
	} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE recipe_id = $1", string(recipeID)); err != nil {
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}
	return nil
}

func insertRelations(ctx context.Context, tx pgx.Tx, entry recipecatalogue.CatalogueEntry) error {
	ingredients := append([]domain.Ingredient(nil), entry.Ingredients...)
	sort.Slice(ingredients, func(i, j int) bool { return ingredients[i].ID < ingredients[j].ID })
	for _, ingredient := range ingredients {
		var storedName string
		err := tx.QueryRow(ctx, `
			INSERT INTO ingredients (id, name) VALUES ($1, $2)
			ON CONFLICT (id) DO UPDATE SET name = ingredients.name
			RETURNING name`, string(ingredient.ID), ingredient.Name).Scan(&storedName)
		if err != nil {
			return fmt.Errorf("ensure ingredient %q: %w", ingredient.ID, err)
		}
		if storedName != ingredient.Name {
			return fmt.Errorf("ingredient %q already exists with a different name", ingredient.ID)
		}
	}
	return insertOwnedRelations(ctx, tx, entry)
}

func insertOwnedRelations(ctx context.Context, tx pgx.Tx, entry recipecatalogue.CatalogueEntry) error {
	mealTypes := append([]domain.MealType(nil), entry.Recipe.MealTypes...)
	sort.Slice(mealTypes, func(i, j int) bool { return mealTypes[i] < mealTypes[j] })
	for _, mealType := range mealTypes {
		if _, err := tx.Exec(ctx,
			`INSERT INTO recipe_meal_types (recipe_id, meal_type) VALUES ($1, $2)`,
			string(entry.Recipe.ID), string(mealType),
		); err != nil {
			return fmt.Errorf("insert meal type %q: %w", mealType, err)
		}
	}

	tags := append([]string(nil), entry.Recipe.Tags...)
	sort.Strings(tags)
	for _, tag := range tags {
		if _, err := tx.Exec(ctx,
			`INSERT INTO recipe_tags (recipe_id, tag) VALUES ($1, $2)`,
			string(entry.Recipe.ID), tag,
		); err != nil {
			return fmt.Errorf("insert tag %q: %w", tag, err)
		}
	}

	relations := append([]domain.RecipeIngredient(nil), entry.Recipe.Ingredients...)
	sort.Slice(relations, func(i, j int) bool { return relations[i].IngredientID < relations[j].IngredientID })
	for _, relation := range relations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, unit)
			VALUES ($1, $2, $3, $4)`,
			string(entry.Recipe.ID), string(relation.IngredientID), relation.Quantity, relation.Unit,
		); err != nil {
			return fmt.Errorf("insert ingredient relation %q: %w", relation.IngredientID, err)
		}
	}

	mediaItems := append([]domain.RecipeMedia(nil), entry.Recipe.Media...)
	sort.Slice(mediaItems, func(i, j int) bool { return mediaItems[i].ID < mediaItems[j].ID })
	for _, media := range mediaItems {
		if _, err := tx.Exec(ctx, `
			INSERT INTO recipe_media (
				id, recipe_id, storage_key, mime_type, width, height, alt_text, caption
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			string(media.ID), string(entry.Recipe.ID), string(media.StorageKey), string(media.MIMEType),
			media.Width, media.Height, nullableText(media.AltText), nullableText(media.Caption),
		); err != nil {
			return fmt.Errorf("insert media %q: %w", media.ID, err)
		}
	}
	if entry.Recipe.CoverImageID != nil {
		if _, err := tx.Exec(ctx,
			`INSERT INTO recipe_cover_images (recipe_id, media_id) VALUES ($1, $2)`,
			string(entry.Recipe.ID), string(*entry.Recipe.CoverImageID),
		); err != nil {
			return fmt.Errorf("insert cover image: %w", err)
		}
	}
	images := append([]domain.RecipeImage(nil), entry.Recipe.Images...)
	sort.Slice(images, func(i, j int) bool { return images[i].Position < images[j].Position })
	for _, image := range images {
		if _, err := tx.Exec(ctx,
			`INSERT INTO recipe_images (recipe_id, media_id, position) VALUES ($1, $2, $3)`,
			string(entry.Recipe.ID), string(image.MediaID), image.Position,
		); err != nil {
			return fmt.Errorf("insert additional image at position %d: %w", image.Position, err)
		}
	}
	steps := append([]domain.RecipeStep(nil), entry.Recipe.Steps...)
	sort.Slice(steps, func(i, j int) bool { return steps[i].Position < steps[j].Position })
	for _, step := range steps {
		var imageID any
		if step.ImageID != nil {
			imageID = string(*step.ImageID)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recipe_steps (id, recipe_id, position, text, image_media_id)
			VALUES ($1, $2, $3, $4, $5)`,
			string(step.ID), string(entry.Recipe.ID), step.Position, step.Text, imageID,
		); err != nil {
			return fmt.Errorf("insert step at position %d: %w", step.Position, err)
		}
	}

	return nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
