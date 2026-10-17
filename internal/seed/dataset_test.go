package seed

import (
	"strings"
	"testing"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

func TestDatasetValidateRejectsSubSecondRecipeDurations(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*domain.Recipe)
		wantError string
	}{
		{
			name: "preparation duration",
			mutate: func(recipe *domain.Recipe) {
				recipe.PreparationDuration = 1500 * time.Millisecond
			},
			wantError: "preparation duration must use whole seconds",
		},
		{
			name: "cooking duration",
			mutate: func(recipe *domain.Recipe) {
				recipe.CookingDuration = 1500 * time.Millisecond
			},
			wantError: "cooking duration must use whole seconds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataset := validDataset()
			test.mutate(&dataset.Entries[0].Recipe)

			err := dataset.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want text %q", test.wantError)
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %q, want text %q", err, test.wantError)
			}
		})
	}
}

func validDataset() Dataset {
	ingredient := domain.Ingredient{ID: "I001", Name: "Rice"}
	return Dataset{
		Ingredients: []domain.Ingredient{ingredient},
		Entries: []recipecatalogue.CatalogueEntry{
			{
				Recipe: domain.Recipe{
					ID:                  "R001",
					Name:                "Recipe",
					MealTypes:           []domain.MealType{domain.MealTypeDinner},
					Category:            "main",
					Servings:            2,
					PreparationDuration: time.Minute,
					CookingDuration:     2 * time.Minute,
					StorageDays:         3,
					Difficulty:          domain.DifficultyEasy,
					BudgetLevel:         domain.BudgetLevelLow,
					ReheatScore:         domain.MinScore,
					BatchCookingScore:   domain.MinScore,
					State:               domain.RecipeStateActive,
					Ingredients: []domain.RecipeIngredient{
						{IngredientID: ingredient.ID, Quantity: 1, Unit: "kg"},
					},
				},
				Ingredients: []domain.Ingredient{ingredient},
			},
		},
	}
}
