package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Recipe describes a recipe catalogue entry and its ingredient relations.
type Recipe struct {
	ID                  RecipeID
	Name                string
	MealTypes           []MealType
	Category            string
	Servings            int
	PreparationDuration time.Duration
	CookingDuration     time.Duration
	StorageDays         int
	FreezerFriendly     bool
	Difficulty          Difficulty
	BudgetLevel         BudgetLevel
	ReheatScore         Score
	BatchCookingScore   Score
	Tags                []string
	State               RecipeState
	Ingredients         []RecipeIngredient
}

// Validate checks all recipe and recipe-ingredient invariants.
func (r Recipe) Validate() error {
	if blank(string(r.ID)) {
		return errors.New("recipe ID is required")
	}
	if blank(r.Name) {
		return errors.New("recipe name is required")
	}
	if len(r.MealTypes) == 0 {
		return errors.New("recipe must have at least one meal type")
	}
	seenMealTypes := make(map[MealType]struct{}, len(r.MealTypes))
	for index, mealType := range r.MealTypes {
		if !mealType.valid() {
			return fmt.Errorf("meal type at index %d is invalid: %q", index, mealType)
		}
		if _, duplicate := seenMealTypes[mealType]; duplicate {
			return fmt.Errorf("meal type %q is duplicated", mealType)
		}
		seenMealTypes[mealType] = struct{}{}
	}
	if blank(r.Category) {
		return errors.New("recipe category is required")
	}
	if r.Servings <= 0 {
		return errors.New("servings must be positive")
	}
	if r.PreparationDuration < 0 {
		return errors.New("preparation duration must be non-negative")
	}
	if r.CookingDuration < 0 {
		return errors.New("cooking duration must be non-negative")
	}
	if r.StorageDays <= 0 {
		return errors.New("storage days must be positive")
	}
	if !r.Difficulty.valid() {
		return fmt.Errorf("difficulty %q is invalid", r.Difficulty)
	}
	if !r.BudgetLevel.valid() {
		return fmt.Errorf("budget level %q is invalid", r.BudgetLevel)
	}
	if !r.ReheatScore.valid() {
		return fmt.Errorf("reheat score must be between %d and %d", MinScore, MaxScore)
	}
	if !r.BatchCookingScore.valid() {
		return fmt.Errorf("batch-cooking score must be between %d and %d", MinScore, MaxScore)
	}
	for index, tag := range r.Tags {
		if blank(tag) {
			return fmt.Errorf("tag at index %d is required", index)
		}
	}
	if !r.State.valid() {
		return fmt.Errorf("recipe state %q is invalid", r.State)
	}
	if len(r.Ingredients) == 0 {
		return errors.New("recipe must have at least one ingredient")
	}
	for index, ingredient := range r.Ingredients {
		if err := ingredient.Validate(); err != nil {
			return fmt.Errorf("ingredient at index %d: %w", index, err)
		}
	}

	return nil
}

func blank(value string) bool {
	return strings.TrimSpace(value) == ""
}
