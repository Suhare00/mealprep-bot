package domain

import (
	"testing"
	"time"
)

func TestRecipeValidateAcceptsValidRecipes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Recipe)
	}{
		{
			name: "typical active recipe",
			mutate: func(*Recipe) {
			},
		},
		{
			name: "numeric lower boundaries",
			mutate: func(recipe *Recipe) {
				recipe.Servings = 1
				recipe.PreparationDuration = 0
				recipe.CookingDuration = 0
				recipe.StorageDays = 1
				recipe.ReheatScore = MinScore
				recipe.BatchCookingScore = MinScore
				recipe.Ingredients[0].Quantity = 0.001
			},
		},
		{
			name: "score upper boundaries and inactive state",
			mutate: func(recipe *Recipe) {
				recipe.ReheatScore = MaxScore
				recipe.BatchCookingScore = MaxScore
				recipe.State = RecipeStateInactive
				recipe.Tags = nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe := validRecipe()
			test.mutate(&recipe)

			if err := recipe.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestRecipeValidateAcceptsEveryEnumValue(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Recipe)
	}{
		{name: "easy", mutate: func(recipe *Recipe) { recipe.Difficulty = DifficultyEasy }},
		{name: "medium difficulty", mutate: func(recipe *Recipe) { recipe.Difficulty = DifficultyMedium }},
		{name: "low budget", mutate: func(recipe *Recipe) { recipe.BudgetLevel = BudgetLevelLow }},
		{name: "medium budget", mutate: func(recipe *Recipe) { recipe.BudgetLevel = BudgetLevelMedium }},
		{name: "high budget", mutate: func(recipe *Recipe) { recipe.BudgetLevel = BudgetLevelHigh }},
		{name: "active", mutate: func(recipe *Recipe) { recipe.State = RecipeStateActive }},
		{name: "inactive", mutate: func(recipe *Recipe) { recipe.State = RecipeStateInactive }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe := validRecipe()
			test.mutate(&recipe)

			if err := recipe.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestRecipeValidateAcceptsMealTypeCombinations(t *testing.T) {
	tests := []struct {
		name      string
		mealTypes []MealType
	}{
		{name: "breakfast only", mealTypes: []MealType{MealTypeBreakfast}},
		{name: "lunch only", mealTypes: []MealType{MealTypeLunch}},
		{name: "dinner only", mealTypes: []MealType{MealTypeDinner}},
		{name: "lunch and dinner", mealTypes: []MealType{MealTypeLunch, MealTypeDinner}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe := validRecipe()
			recipe.MealTypes = test.mealTypes

			if err := recipe.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestRecipeValidateRejectsInvalidRecipes(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Recipe)
		wantError string
	}{
		{name: "empty ID", mutate: func(recipe *Recipe) { recipe.ID = "" }, wantError: "recipe ID"},
		{name: "whitespace ID", mutate: func(recipe *Recipe) { recipe.ID = " \t" }, wantError: "recipe ID"},
		{name: "empty name", mutate: func(recipe *Recipe) { recipe.Name = "" }, wantError: "recipe name"},
		{name: "whitespace name", mutate: func(recipe *Recipe) { recipe.Name = " \n" }, wantError: "recipe name"},
		{name: "nil meal types", mutate: func(recipe *Recipe) { recipe.MealTypes = nil }, wantError: "at least one meal type"},
		{name: "empty meal types", mutate: func(recipe *Recipe) { recipe.MealTypes = []MealType{} }, wantError: "at least one meal type"},
		{name: "invalid meal type", mutate: func(recipe *Recipe) { recipe.MealTypes = []MealType{MealTypeLunch, "snack"} }, wantError: "meal type at index 1"},
		{name: "duplicate meal type", mutate: func(recipe *Recipe) { recipe.MealTypes = []MealType{MealTypeLunch, MealTypeLunch} }, wantError: "duplicated"},
		{name: "empty category", mutate: func(recipe *Recipe) { recipe.Category = "" }, wantError: "category"},
		{name: "whitespace category", mutate: func(recipe *Recipe) { recipe.Category = "  " }, wantError: "category"},
		{name: "zero servings", mutate: func(recipe *Recipe) { recipe.Servings = 0 }, wantError: "servings"},
		{name: "negative servings", mutate: func(recipe *Recipe) { recipe.Servings = -1 }, wantError: "servings"},
		{name: "negative preparation duration", mutate: func(recipe *Recipe) { recipe.PreparationDuration = -time.Nanosecond }, wantError: "preparation duration"},
		{name: "negative cooking duration", mutate: func(recipe *Recipe) { recipe.CookingDuration = -time.Nanosecond }, wantError: "cooking duration"},
		{name: "zero storage days", mutate: func(recipe *Recipe) { recipe.StorageDays = 0 }, wantError: "storage days"},
		{name: "negative storage days", mutate: func(recipe *Recipe) { recipe.StorageDays = -1 }, wantError: "storage days"},
		{name: "empty difficulty", mutate: func(recipe *Recipe) { recipe.Difficulty = "" }, wantError: "difficulty"},
		{name: "unknown difficulty", mutate: func(recipe *Recipe) { recipe.Difficulty = "expert" }, wantError: "difficulty"},
		{name: "empty budget level", mutate: func(recipe *Recipe) { recipe.BudgetLevel = "" }, wantError: "budget level"},
		{name: "unknown budget level", mutate: func(recipe *Recipe) { recipe.BudgetLevel = "luxury" }, wantError: "budget level"},
		{name: "reheat score below minimum", mutate: func(recipe *Recipe) { recipe.ReheatScore = MinScore - 1 }, wantError: "reheat score"},
		{name: "reheat score above maximum", mutate: func(recipe *Recipe) { recipe.ReheatScore = MaxScore + 1 }, wantError: "reheat score"},
		{name: "batch score below minimum", mutate: func(recipe *Recipe) { recipe.BatchCookingScore = MinScore - 1 }, wantError: "batch-cooking score"},
		{name: "batch score above maximum", mutate: func(recipe *Recipe) { recipe.BatchCookingScore = MaxScore + 1 }, wantError: "batch-cooking score"},
		{name: "empty tag", mutate: func(recipe *Recipe) { recipe.Tags = []string{""} }, wantError: "tag"},
		{name: "whitespace tag", mutate: func(recipe *Recipe) { recipe.Tags = []string{"quick", " \t"} }, wantError: "tag"},
		{name: "empty state", mutate: func(recipe *Recipe) { recipe.State = "" }, wantError: "recipe state"},
		{name: "unknown state", mutate: func(recipe *Recipe) { recipe.State = "archived" }, wantError: "recipe state"},
		{name: "nil ingredients", mutate: func(recipe *Recipe) { recipe.Ingredients = nil }, wantError: "at least one ingredient"},
		{name: "empty ingredients", mutate: func(recipe *Recipe) { recipe.Ingredients = []RecipeIngredient{} }, wantError: "at least one ingredient"},
		{name: "invalid ingredient relation", mutate: func(recipe *Recipe) { recipe.Ingredients[0].Quantity = 0 }, wantError: "ingredient at index 0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe := validRecipe()
			test.mutate(&recipe)

			assertValidationResult(t, recipe.Validate(), test.wantError)
		})
	}
}

func validRecipe() Recipe {
	return Recipe{
		ID:                  "recipe-1",
		Name:                "Vegetable stew",
		MealTypes:           []MealType{MealTypeDinner},
		Category:            "stew",
		Servings:            6,
		PreparationDuration: 15 * time.Minute,
		CookingDuration:     45 * time.Minute,
		StorageDays:         3,
		FreezerFriendly:     true,
		Difficulty:          DifficultyEasy,
		BudgetLevel:         BudgetLevelLow,
		ReheatScore:         4,
		BatchCookingScore:   5,
		Tags:                []string{"vegetarian", "batch"},
		State:               RecipeStateActive,
		Ingredients: []RecipeIngredient{
			{IngredientID: "ingredient-1", Quantity: 500, Unit: "g"},
		},
	}
}
