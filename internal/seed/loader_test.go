package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
)

func TestLoadValidMinimalDataset(t *testing.T) {
	dataset, err := Load(writeFixture(t, validFixture()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(dataset.Entries) != 1 || len(dataset.Ingredients) != 1 {
		t.Fatalf("Load() counts = %d entries, %d ingredients; want 1, 1",
			len(dataset.Entries), len(dataset.Ingredients))
	}
	entry := dataset.Entries[0]
	if entry.Recipe.ID != "R001" || entry.Recipe.PreparationDuration != 60*time.Second {
		t.Fatalf("Load() entry = %#v", entry)
	}
	if len(entry.Recipe.MealTypes) != 1 || entry.Recipe.MealTypes[0] != domain.MealTypeBreakfast {
		t.Fatalf("Load() meal types = %v", entry.Recipe.MealTypes)
	}
}

func TestLoadCanonicalDataset(t *testing.T) {
	dataset, err := Load(filepath.Join("..", "..", "seed"))
	if err != nil {
		t.Fatalf("Load(canonical) error = %v", err)
	}
	if len(dataset.Entries) != 90 || len(dataset.Ingredients) != 84 {
		t.Fatalf("canonical counts = %d recipes, %d ingredients; want 90, 84",
			len(dataset.Entries), len(dataset.Ingredients))
	}

	var relations, mealTypes, tags int
	for _, entry := range dataset.Entries {
		relations += len(entry.Recipe.Ingredients)
		mealTypes += len(entry.Recipe.MealTypes)
		tags += len(entry.Recipe.Tags)
	}
	if relations != 466 || mealTypes != 155 || tags != 233 {
		t.Fatalf("canonical relation counts = %d ingredients, %d meal types, %d tags; want 466, 155, 233",
			relations, mealTypes, tags)
	}
}

func TestLoadRejectsMalformedFields(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		replace  string
		with     string
		wantText string
	}{
		{name: "integer", file: recipesFile, replace: ",6,60,", with: ",many,60,", wantText: "recipes.csv: row 2: field servings"},
		{name: "boolean", file: recipesFile, replace: ",false,easy,", with: ",sometimes,easy,", wantText: "recipes.csv: row 2: field freezer_friendly"},
		{name: "float", file: recipeIngredientsFile, replace: ",1.5,kg", with: ",many,kg", wantText: "recipe_ingredients.csv: row 2: field quantity"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixture()
			fixture[test.file] = strings.Replace(fixture[test.file], test.replace, test.with, 1)
			assertLoadErrorContains(t, fixture, test.wantText)
		})
	}
}

func TestLoadRejectsBadHeadersAndMalformedCSV(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]string)
		wantText string
	}{
		{
			name: "unexpected header",
			mutate: func(fixture map[string]string) {
				fixture[ingredientsFile] = strings.Replace(fixture[ingredientsFile], "ingredient_id,name", "id,name", 1)
			},
			wantText: "ingredients.csv: row 1: unexpected header",
		},
		{
			name: "malformed CSV",
			mutate: func(fixture map[string]string) {
				fixture[recipeTagsFile] = "recipe_id,tag\nR001,\"unterminated\n"
			},
			wantText: "recipe_tags.csv: row 2",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixture()
			test.mutate(fixture)
			assertLoadErrorContains(t, fixture, test.wantText)
		})
	}
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		row      string
		wantText string
	}{
		{name: "recipe", file: recipesFile, row: "R001,Duplicate,main,1,0,0,1,false,easy,low,1,1,active\n", wantText: "recipes.csv: row 3: field recipe_id"},
		{name: "ingredient", file: ingredientsFile, row: "I001,Duplicate\n", wantText: "ingredients.csv: row 3: field ingredient_id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixture()
			fixture[test.file] += test.row
			assertLoadErrorContains(t, fixture, test.wantText)
		})
	}
}

func TestLoadRejectsDuplicateRelations(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		row      string
		wantText string
	}{
		{name: "ingredient", file: recipeIngredientsFile, row: "R001,I001,2,kg\n", wantText: "recipe_ingredients.csv: row 3: recipe/ingredient relation"},
		{name: "meal type", file: recipeMealTypesFile, row: "R001,breakfast\n", wantText: "recipe_meal_types.csv: row 3: recipe/meal-type relation"},
		{name: "tag", file: recipeTagsFile, row: "R001,quick\n", wantText: "recipe_tags.csv: row 3: recipe/tag relation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixture()
			fixture[test.file] += test.row
			assertLoadErrorContains(t, fixture, test.wantText)
		})
	}
}

func TestLoadRejectsBrokenReferences(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		replace  string
		with     string
		wantText string
	}{
		{name: "recipe", file: recipeMealTypesFile, replace: "R001", with: "missing", wantText: "recipe_meal_types.csv: row 2: field recipe_id"},
		{name: "ingredient", file: recipeIngredientsFile, replace: "I001", with: "missing", wantText: "recipe_ingredients.csv: row 2: field ingredient_id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixture()
			fixture[test.file] = strings.Replace(fixture[test.file], test.replace, test.with, 1)
			assertLoadErrorContains(t, fixture, test.wantText)
		})
	}
}

func TestLoadRejectsIncompleteRecipes(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		content  string
		wantText string
	}{
		{name: "meal types", file: recipeMealTypesFile, content: "recipe_id,meal_type\n", wantText: "recipes.csv: row 2: validate recipe \"R001\": recipe must have at least one meal type"},
		{name: "ingredients", file: recipeIngredientsFile, content: "recipe_id,ingredient_id,quantity,unit\n", wantText: "recipes.csv: row 2: validate recipe \"R001\": recipe must have at least one ingredient"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixture()
			fixture[test.file] = test.content
			assertLoadErrorContains(t, fixture, test.wantText)
		})
	}
}

func TestLoadRejectsInvalidEnumsAndDomainValues(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		replace  string
		with     string
		wantText string
	}{
		{name: "meal type", file: recipeMealTypesFile, replace: "breakfast", with: "Завтрак", wantText: "recipe_meal_types.csv: row 2: field meal_type"},
		{name: "difficulty", file: recipesFile, replace: ",easy,low,", with: ",Легко,low,", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "budget", file: recipesFile, replace: ",low,5,", with: ",₽,5,", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "state", file: recipesFile, replace: ",active\n", with: ",deleted\n", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "score", file: recipesFile, replace: ",5,5,active", with: ",0,5,active", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "batch score", file: recipesFile, replace: ",5,5,active", with: ",5,6,active", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "servings", file: recipesFile, replace: ",6,60,", with: ",0,60,", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "negative duration", file: recipesFile, replace: ",60,120,", with: ",-1,120,", wantText: "recipes.csv: row 2: field preparation_seconds"},
		{name: "negative cooking duration", file: recipesFile, replace: ",60,120,", with: ",60,-1,", wantText: "recipes.csv: row 2: field cooking_seconds"},
		{name: "storage days", file: recipesFile, replace: ",120,3,false", with: ",120,0,false", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "blank recipe ID", file: recipesFile, replace: "R001,Recipe", with: " ,Recipe", wantText: "recipes.csv: row 2: field recipe_id"},
		{name: "blank recipe name", file: recipesFile, replace: "R001,Recipe,main", with: "R001, ,main", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "blank category", file: recipesFile, replace: ",main,6,", with: ", ,6,", wantText: "recipes.csv: row 2: validate recipe"},
		{name: "blank ingredient ID", file: ingredientsFile, replace: "I001,Rice", with: " ,Rice", wantText: "ingredients.csv: row 2: ingredient ID"},
		{name: "blank ingredient name", file: ingredientsFile, replace: "I001,Rice", with: "I001, ", wantText: "ingredients.csv: row 2: ingredient name"},
		{name: "quantity", file: recipeIngredientsFile, replace: ",1.5,kg", with: ",0,kg", wantText: "recipe_ingredients.csv: row 2: ingredient quantity"},
		{name: "blank unit", file: recipeIngredientsFile, replace: ",1.5,kg", with: ",1.5, ", wantText: "recipe_ingredients.csv: row 2: ingredient unit"},
		{name: "blank tag", file: recipeTagsFile, replace: ",quick", with: ", ", wantText: "recipe_tags.csv: row 2: field tag"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixture()
			fixture[test.file] = strings.Replace(fixture[test.file], test.replace, test.with, 1)
			assertLoadErrorContains(t, fixture, test.wantText)
		})
	}
}

func validFixture() map[string]string {
	return map[string]string{
		recipesFile: "recipe_id,name,category,servings,preparation_seconds,cooking_seconds,storage_days,freezer_friendly,difficulty,budget_level,reheat_score,batch_cooking_score,state\n" +
			"R001,Recipe,main,6,60,120,3,false,easy,low,5,5,active\n",
		ingredientsFile:       "ingredient_id,name\nI001,Rice\n",
		recipeIngredientsFile: "recipe_id,ingredient_id,quantity,unit\nR001,I001,1.5,kg\n",
		recipeMealTypesFile:   "recipe_id,meal_type\nR001,breakfast\n",
		recipeTagsFile:        "recipe_id,tag\nR001,quick\n",
	}
}

func writeFixture(t *testing.T, fixture map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	for filename, contents := range fixture {
		if err := os.WriteFile(filepath.Join(directory, filename), []byte(contents), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", filename, err)
		}
	}
	return directory
}

func assertLoadErrorContains(t *testing.T, fixture map[string]string, want string) {
	t.Helper()
	_, err := Load(writeFixture(t, fixture))
	if err == nil {
		t.Fatalf("Load() error = nil, want text %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Load() error = %q, want text %q", err, want)
	}
}
