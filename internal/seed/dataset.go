// Package seed parses and validates the canonical recipe seed dataset.
package seed

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

// Dataset is a complete, persistence-independent recipe catalogue seed.
type Dataset struct {
	Ingredients []domain.Ingredient
	Entries     []recipecatalogue.CatalogueEntry
}

// Validate verifies that a programmatically constructed dataset is complete and consistent.
func (d Dataset) Validate() error {
	if len(d.Ingredients) == 0 {
		return errors.New("dataset must contain at least one ingredient")
	}
	if len(d.Entries) == 0 {
		return errors.New("dataset must contain at least one recipe")
	}

	ingredients := make(map[domain.IngredientID]domain.Ingredient, len(d.Ingredients))
	for index, ingredient := range d.Ingredients {
		if err := ingredient.Validate(); err != nil {
			return fmt.Errorf("ingredient at index %d: %w", index, err)
		}
		if _, duplicate := ingredients[ingredient.ID]; duplicate {
			return fmt.Errorf("ingredient %q is duplicated", ingredient.ID)
		}
		ingredients[ingredient.ID] = ingredient
	}

	recipes := make(map[domain.RecipeID]struct{}, len(d.Entries))
	for index, entry := range d.Entries {
		if _, duplicate := recipes[entry.Recipe.ID]; duplicate {
			return fmt.Errorf("recipe %q is duplicated", entry.Recipe.ID)
		}
		recipes[entry.Recipe.ID] = struct{}{}
		if err := validateEntry(entry, ingredients); err != nil {
			return fmt.Errorf("recipe at index %d: %w", index, err)
		}
	}

	return nil
}

func validateEntry(entry recipecatalogue.CatalogueEntry, catalogue map[domain.IngredientID]domain.Ingredient) error {
	if err := entry.Recipe.Validate(); err != nil {
		return err
	}
	if entry.Recipe.PreparationDuration%time.Second != 0 {
		return errors.New("preparation duration must use whole seconds")
	}
	if entry.Recipe.CookingDuration%time.Second != 0 {
		return errors.New("cooking duration must use whole seconds")
	}
	seenTags := make(map[string]struct{}, len(entry.Recipe.Tags))
	for _, tag := range entry.Recipe.Tags {
		if _, duplicate := seenTags[tag]; duplicate {
			return fmt.Errorf("tag %q is duplicated", tag)
		}
		seenTags[tag] = struct{}{}
	}

	entryIngredients := make(map[domain.IngredientID]domain.Ingredient, len(entry.Ingredients))
	for _, ingredient := range entry.Ingredients {
		canonical, ok := catalogue[ingredient.ID]
		if !ok {
			return fmt.Errorf("ingredient %q is absent from the dataset catalogue", ingredient.ID)
		}
		if ingredient != canonical {
			return fmt.Errorf("ingredient %q differs from the dataset catalogue", ingredient.ID)
		}
		if _, duplicate := entryIngredients[ingredient.ID]; duplicate {
			return fmt.Errorf("ingredient %q is duplicated in catalogue entry", ingredient.ID)
		}
		entryIngredients[ingredient.ID] = ingredient
	}

	relations := make(map[domain.IngredientID]struct{}, len(entry.Recipe.Ingredients))
	for _, relation := range entry.Recipe.Ingredients {
		if _, ok := entryIngredients[relation.IngredientID]; !ok {
			return fmt.Errorf("ingredient %q has no catalogue record in recipe", relation.IngredientID)
		}
		if _, duplicate := relations[relation.IngredientID]; duplicate {
			return fmt.Errorf("ingredient %q is duplicated in recipe relations", relation.IngredientID)
		}
		relations[relation.IngredientID] = struct{}{}
	}
	if len(relations) != len(entryIngredients) {
		return errors.New("recipe ingredient catalogue and relation counts differ")
	}

	return nil
}

func normalize(dataset *Dataset) {
	sort.Slice(dataset.Ingredients, func(i, j int) bool {
		return dataset.Ingredients[i].ID < dataset.Ingredients[j].ID
	})
	sort.Slice(dataset.Entries, func(i, j int) bool {
		return dataset.Entries[i].Recipe.ID < dataset.Entries[j].Recipe.ID
	})
	for i := range dataset.Entries {
		entry := &dataset.Entries[i]
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
	}
}
