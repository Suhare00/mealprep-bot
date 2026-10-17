package domain

import (
	"math"
	"strings"
	"testing"
)

func TestIngredientValidate(t *testing.T) {
	tests := []struct {
		name       string
		ingredient Ingredient
		wantError  string
	}{
		{
			name:       "valid",
			ingredient: Ingredient{ID: "ingredient-1", Name: "Carrot"},
		},
		{
			name:       "empty ID",
			ingredient: Ingredient{Name: "Carrot"},
			wantError:  "ingredient ID",
		},
		{
			name:       "whitespace ID",
			ingredient: Ingredient{ID: " \t ", Name: "Carrot"},
			wantError:  "ingredient ID",
		},
		{
			name:       "empty name",
			ingredient: Ingredient{ID: "ingredient-1"},
			wantError:  "ingredient name",
		},
		{
			name:       "whitespace name",
			ingredient: Ingredient{ID: "ingredient-1", Name: "  \n"},
			wantError:  "ingredient name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.ingredient.Validate()
			assertValidationResult(t, err, test.wantError)
		})
	}
}

func TestRecipeIngredientValidate(t *testing.T) {
	tests := []struct {
		name       string
		ingredient RecipeIngredient
		wantError  string
	}{
		{
			name:       "valid",
			ingredient: RecipeIngredient{IngredientID: "ingredient-1", Quantity: 1.5, Unit: "kg"},
		},
		{
			name:       "empty ingredient ID",
			ingredient: RecipeIngredient{Quantity: 1, Unit: "kg"},
			wantError:  "ingredient ID",
		},
		{
			name:       "whitespace ingredient ID",
			ingredient: RecipeIngredient{IngredientID: "  ", Quantity: 1, Unit: "kg"},
			wantError:  "ingredient ID",
		},
		{
			name:       "zero quantity",
			ingredient: RecipeIngredient{IngredientID: "ingredient-1", Unit: "kg"},
			wantError:  "quantity",
		},
		{
			name:       "negative quantity",
			ingredient: RecipeIngredient{IngredientID: "ingredient-1", Quantity: -1, Unit: "kg"},
			wantError:  "quantity",
		},
		{
			name:       "NaN quantity",
			ingredient: RecipeIngredient{IngredientID: "ingredient-1", Quantity: math.NaN(), Unit: "kg"},
			wantError:  "quantity",
		},
		{
			name:       "infinite quantity",
			ingredient: RecipeIngredient{IngredientID: "ingredient-1", Quantity: math.Inf(1), Unit: "kg"},
			wantError:  "quantity",
		},
		{
			name:       "empty unit",
			ingredient: RecipeIngredient{IngredientID: "ingredient-1", Quantity: 1},
			wantError:  "unit",
		},
		{
			name:       "whitespace unit",
			ingredient: RecipeIngredient{IngredientID: "ingredient-1", Quantity: 1, Unit: " \t"},
			wantError:  "unit",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.ingredient.Validate()
			assertValidationResult(t, err, test.wantError)
		})
	}
}

func assertValidationResult(t *testing.T, err error, wantError string) {
	t.Helper()

	if wantError == "" {
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		return
	}

	if err == nil {
		t.Fatalf("Validate() error = nil, want error containing %q", wantError)
	}
	if !strings.Contains(err.Error(), wantError) {
		t.Fatalf("Validate() error = %q, want error containing %q", err, wantError)
	}
}
