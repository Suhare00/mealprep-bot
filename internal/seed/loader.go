package seed

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

const (
	recipesFile           = "recipes.csv"
	ingredientsFile       = "ingredients.csv"
	recipeIngredientsFile = "recipe_ingredients.csv"
	recipeMealTypesFile   = "recipe_meal_types.csv"
	recipeTagsFile        = "recipe_tags.csv"
)

var (
	recipeHeader = []string{
		"recipe_id", "name", "category", "servings", "preparation_seconds", "cooking_seconds",
		"storage_days", "freezer_friendly", "difficulty", "budget_level", "reheat_score",
		"batch_cooking_score", "state",
	}
	ingredientHeader       = []string{"ingredient_id", "name"}
	recipeIngredientHeader = []string{"recipe_id", "ingredient_id", "quantity", "unit"}
	recipeMealTypeHeader   = []string{"recipe_id", "meal_type"}
	recipeTagHeader        = []string{"recipe_id", "tag"}
)

type recipeRecord struct {
	entry recipecatalogue.CatalogueEntry
	row   int
}

// Load reads and validates all persisted seed files in directory.
// provenance.csv is intentionally not read because it is documentation-only.
func Load(directory string) (Dataset, error) {
	recipes, err := loadRecipes(directory)
	if err != nil {
		return Dataset{}, err
	}
	ingredients, err := loadIngredients(directory)
	if err != nil {
		return Dataset{}, err
	}
	if err := loadMealTypes(directory, recipes); err != nil {
		return Dataset{}, err
	}
	if err := loadTags(directory, recipes); err != nil {
		return Dataset{}, err
	}
	if err := loadRecipeIngredients(directory, recipes, ingredients); err != nil {
		return Dataset{}, err
	}

	dataset := Dataset{
		Ingredients: make([]domain.Ingredient, 0, len(ingredients)),
		Entries:     make([]recipecatalogue.CatalogueEntry, 0, len(recipes)),
	}
	for _, ingredient := range ingredients {
		dataset.Ingredients = append(dataset.Ingredients, ingredient)
	}
	for _, record := range recipes {
		if err := record.entry.Recipe.Validate(); err != nil {
			return Dataset{}, fmt.Errorf("%s: row %d: validate recipe %q: %w",
				recipesFile, record.row, record.entry.Recipe.ID, err)
		}
		dataset.Entries = append(dataset.Entries, record.entry)
	}
	normalize(&dataset)
	if err := dataset.Validate(); err != nil {
		return Dataset{}, fmt.Errorf("validate seed dataset: %w", err)
	}

	return dataset, nil
}

func loadRecipes(directory string) (map[domain.RecipeID]*recipeRecord, error) {
	rows, err := readCSV(directory, recipesFile, recipeHeader)
	if err != nil {
		return nil, err
	}
	recipes := make(map[domain.RecipeID]*recipeRecord, len(rows))
	for _, row := range rows {
		id := domain.RecipeID(row.fields[0])
		if blank(row.fields[0]) {
			return nil, fieldError(recipesFile, row.number, "recipe_id", row.fields[0], "must not be blank")
		}
		if _, duplicate := recipes[id]; duplicate {
			return nil, fieldError(recipesFile, row.number, "recipe_id", row.fields[0], "is duplicated")
		}

		servings, err := parseInt(recipesFile, row, 3, "servings")
		if err != nil {
			return nil, err
		}
		preparation, err := parseDuration(recipesFile, row, 4, "preparation_seconds")
		if err != nil {
			return nil, err
		}
		cooking, err := parseDuration(recipesFile, row, 5, "cooking_seconds")
		if err != nil {
			return nil, err
		}
		storageDays, err := parseInt(recipesFile, row, 6, "storage_days")
		if err != nil {
			return nil, err
		}
		freezerFriendly, err := parseBool(recipesFile, row, 7, "freezer_friendly")
		if err != nil {
			return nil, err
		}
		reheatScore, err := parseInt(recipesFile, row, 10, "reheat_score")
		if err != nil {
			return nil, err
		}
		batchCookingScore, err := parseInt(recipesFile, row, 11, "batch_cooking_score")
		if err != nil {
			return nil, err
		}

		recipes[id] = &recipeRecord{
			row: row.number,
			entry: recipecatalogue.CatalogueEntry{Recipe: domain.Recipe{
				ID:                  id,
				Name:                row.fields[1],
				Category:            row.fields[2],
				Servings:            servings,
				PreparationDuration: preparation,
				CookingDuration:     cooking,
				StorageDays:         storageDays,
				FreezerFriendly:     freezerFriendly,
				Difficulty:          domain.Difficulty(row.fields[8]),
				BudgetLevel:         domain.BudgetLevel(row.fields[9]),
				ReheatScore:         domain.Score(reheatScore),
				BatchCookingScore:   domain.Score(batchCookingScore),
				State:               domain.RecipeState(row.fields[12]),
			}},
		}
	}
	return recipes, nil
}

func loadIngredients(directory string) (map[domain.IngredientID]domain.Ingredient, error) {
	rows, err := readCSV(directory, ingredientsFile, ingredientHeader)
	if err != nil {
		return nil, err
	}
	ingredients := make(map[domain.IngredientID]domain.Ingredient, len(rows))
	for _, row := range rows {
		ingredient := domain.Ingredient{ID: domain.IngredientID(row.fields[0]), Name: row.fields[1]}
		if err := ingredient.Validate(); err != nil {
			return nil, fmt.Errorf("%s: row %d: %w", ingredientsFile, row.number, err)
		}
		if _, duplicate := ingredients[ingredient.ID]; duplicate {
			return nil, fieldError(ingredientsFile, row.number, "ingredient_id", row.fields[0], "is duplicated")
		}
		ingredients[ingredient.ID] = ingredient
	}
	return ingredients, nil
}

func loadMealTypes(directory string, recipes map[domain.RecipeID]*recipeRecord) error {
	rows, err := readCSV(directory, recipeMealTypesFile, recipeMealTypeHeader)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		record, err := referencedRecipe(recipeMealTypesFile, row, recipes)
		if err != nil {
			return err
		}
		mealType := domain.MealType(row.fields[1])
		if !validMealType(mealType) {
			return fieldError(recipeMealTypesFile, row.number, "meal_type", row.fields[1], "is invalid")
		}
		key := row.fields[0] + "\x00" + row.fields[1]
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s: row %d: recipe/meal-type relation (%q, %q) is duplicated",
				recipeMealTypesFile, row.number, row.fields[0], row.fields[1])
		}
		seen[key] = struct{}{}
		record.entry.Recipe.MealTypes = append(record.entry.Recipe.MealTypes, mealType)
	}
	return nil
}

func loadTags(directory string, recipes map[domain.RecipeID]*recipeRecord) error {
	rows, err := readCSV(directory, recipeTagsFile, recipeTagHeader)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		record, err := referencedRecipe(recipeTagsFile, row, recipes)
		if err != nil {
			return err
		}
		if blank(row.fields[1]) {
			return fieldError(recipeTagsFile, row.number, "tag", row.fields[1], "must not be blank")
		}
		key := row.fields[0] + "\x00" + row.fields[1]
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s: row %d: recipe/tag relation (%q, %q) is duplicated",
				recipeTagsFile, row.number, row.fields[0], row.fields[1])
		}
		seen[key] = struct{}{}
		record.entry.Recipe.Tags = append(record.entry.Recipe.Tags, row.fields[1])
	}
	return nil
}

func loadRecipeIngredients(
	directory string,
	recipes map[domain.RecipeID]*recipeRecord,
	ingredients map[domain.IngredientID]domain.Ingredient,
) error {
	rows, err := readCSV(directory, recipeIngredientsFile, recipeIngredientHeader)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		record, err := referencedRecipe(recipeIngredientsFile, row, recipes)
		if err != nil {
			return err
		}
		if blank(row.fields[1]) {
			return fieldError(recipeIngredientsFile, row.number, "ingredient_id", row.fields[1], "must not be blank")
		}
		ingredientID := domain.IngredientID(row.fields[1])
		ingredient, ok := ingredients[ingredientID]
		if !ok {
			return fieldError(recipeIngredientsFile, row.number, "ingredient_id", row.fields[1], "references a missing ingredient")
		}
		quantity, err := parseFloat(recipeIngredientsFile, row, 2, "quantity")
		if err != nil {
			return err
		}
		relation := domain.RecipeIngredient{IngredientID: ingredientID, Quantity: quantity, Unit: row.fields[3]}
		if err := relation.Validate(); err != nil {
			return fmt.Errorf("%s: row %d: %w", recipeIngredientsFile, row.number, err)
		}
		key := row.fields[0] + "\x00" + row.fields[1]
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s: row %d: recipe/ingredient relation (%q, %q) is duplicated",
				recipeIngredientsFile, row.number, row.fields[0], row.fields[1])
		}
		seen[key] = struct{}{}
		record.entry.Recipe.Ingredients = append(record.entry.Recipe.Ingredients, relation)
		record.entry.Ingredients = append(record.entry.Ingredients, ingredient)
	}
	return nil
}

func referencedRecipe(filename string, row csvRow, recipes map[domain.RecipeID]*recipeRecord) (*recipeRecord, error) {
	if blank(row.fields[0]) {
		return nil, fieldError(filename, row.number, "recipe_id", row.fields[0], "must not be blank")
	}
	record, ok := recipes[domain.RecipeID(row.fields[0])]
	if !ok {
		return nil, fieldError(filename, row.number, "recipe_id", row.fields[0], "references a missing recipe")
	}
	return record, nil
}

func parseInt(filename string, row csvRow, index int, field string) (int, error) {
	value, err := strconv.Atoi(row.fields[index])
	if err != nil {
		return 0, fieldError(filename, row.number, field, row.fields[index], "must be an integer")
	}
	return value, nil
}

func parseFloat(filename string, row csvRow, index int, field string) (float64, error) {
	value, err := strconv.ParseFloat(row.fields[index], 64)
	if err != nil {
		return 0, fieldError(filename, row.number, field, row.fields[index], "must be a number")
	}
	return value, nil
}

func parseBool(filename string, row csvRow, index int, field string) (bool, error) {
	if row.fields[index] != "true" && row.fields[index] != "false" {
		return false, fieldError(filename, row.number, field, row.fields[index], "must be true or false")
	}
	value, err := strconv.ParseBool(row.fields[index])
	if err != nil {
		return false, fieldError(filename, row.number, field, row.fields[index], "must be true or false")
	}
	return value, nil
}

func parseDuration(filename string, row csvRow, index int, field string) (time.Duration, error) {
	seconds, err := strconv.ParseInt(row.fields[index], 10, 64)
	if err != nil {
		return 0, fieldError(filename, row.number, field, row.fields[index], "must be an integer")
	}
	if seconds < 0 || seconds > math.MaxInt64/int64(time.Second) {
		return 0, fieldError(filename, row.number, field, row.fields[index], "is outside the supported duration range")
	}
	return time.Duration(seconds) * time.Second, nil
}

func fieldError(filename string, row int, field, value, problem string) error {
	return fmt.Errorf("%s: row %d: field %s value %q %s", filename, row, field, value, problem)
}

func blank(value string) bool {
	return strings.TrimSpace(value) == ""
}

func validMealType(value domain.MealType) bool {
	switch value {
	case domain.MealTypeBreakfast, domain.MealTypeLunch, domain.MealTypeDinner:
		return true
	default:
		return false
	}
}
