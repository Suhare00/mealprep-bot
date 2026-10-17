package domain

import (
	"errors"
	"math"
)

// Ingredient is a catalogue ingredient without recipe-specific quantity information.
type Ingredient struct {
	ID   IngredientID
	Name string
}

// Validate checks the ingredient invariants.
func (i Ingredient) Validate() error {
	if blank(string(i.ID)) {
		return errors.New("ingredient ID is required")
	}
	if blank(i.Name) {
		return errors.New("ingredient name is required")
	}

	return nil
}

// RecipeIngredient is the relation between a recipe and an ingredient.
// Quantity and Unit describe the amount used by the recipe, not the ingredient itself.
type RecipeIngredient struct {
	IngredientID IngredientID
	Quantity     float64
	Unit         string
}

// Validate checks the recipe-ingredient relation invariants.
func (i RecipeIngredient) Validate() error {
	if blank(string(i.IngredientID)) {
		return errors.New("ingredient ID is required")
	}
	if math.IsNaN(i.Quantity) || math.IsInf(i.Quantity, 0) || i.Quantity <= 0 {
		return errors.New("ingredient quantity must be a positive finite number")
	}
	if blank(i.Unit) {
		return errors.New("ingredient unit is required")
	}

	return nil
}
