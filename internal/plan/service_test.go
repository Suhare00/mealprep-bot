package plan

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
	"github.com/Suhare00/mealprep-bot/internal/seed"
)

func TestEligibilityRules(t *testing.T) {
	valid := planEntry("B1", domain.MealTypeBreakfast)
	tests := []struct {
		name   string
		mutate func(*recipecatalogue.CatalogueEntry)
		want   bool
	}{
		{name: "valid", want: true},
		{name: "wrong meal type", mutate: func(e *recipecatalogue.CatalogueEntry) { e.Recipe.MealTypes = []domain.MealType{domain.MealTypeLunch} }},
		{name: "inactive", mutate: func(e *recipecatalogue.CatalogueEntry) { e.Recipe.State = domain.RecipeStateInactive }},
		{name: "short storage", mutate: func(e *recipecatalogue.CatalogueEntry) { e.Recipe.StorageDays = 2 }},
		{name: "wrong yield", mutate: func(e *recipecatalogue.CatalogueEntry) { e.Recipe.Servings = 4 }},
		{name: "invalid domain recipe", mutate: func(e *recipecatalogue.CatalogueEntry) { e.Recipe.Name = " " }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry := valid
			if test.mutate != nil {
				test.mutate(&entry)
			}
			if got := len(eligible([]recipecatalogue.CatalogueEntry{entry}, domain.MealTypeBreakfast)) == 1; got != test.want {
				t.Fatalf("eligible = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGenerateCreatesDistinctCompletePlan(t *testing.T) {
	entries := []recipecatalogue.CatalogueEntry{
		planEntry("B1", domain.MealTypeBreakfast),
		planEntry("L1", domain.MealTypeLunch),
		planEntry("D1", domain.MealTypeDinner),
		planEntry("LD", domain.MealTypeLunch, domain.MealTypeDinner),
	}
	repository := newFakeRepository()
	service := newTestService(entries, repository, &sequenceChooser{})

	snapshot, err := service.Generate(t.Context(), 42, 0)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	ids := snapshot.IDs().Slice()
	if ids[0] == ids[1] || ids[0] == ids[2] || ids[1] == ids[2] {
		t.Fatalf("plan IDs are not distinct: %v", ids)
	}
	if snapshot.Revision != 1 || PortionsPerRecipe != 6 || TotalPortions != 18 {
		t.Fatalf("revision/portion contract = %d/%d/%d", snapshot.Revision, PortionsPerRecipe, TotalPortions)
	}
}

func TestGenerateReportsMissingMealType(t *testing.T) {
	entries := []recipecatalogue.CatalogueEntry{
		planEntry("B1", domain.MealTypeBreakfast),
		planEntry("L1", domain.MealTypeLunch),
	}
	_, err := newTestService(entries, newFakeRepository(), &sequenceChooser{}).Generate(t.Context(), 42, 0)
	var eligibilityErr *EligibilityError
	if !errors.As(err, &eligibilityErr) || eligibilityErr.MealType != domain.MealTypeDinner {
		t.Fatalf("Generate() error = %v, want dinner EligibilityError", err)
	}
}

func TestSelectionAvoidsRecentAndFallsBack(t *testing.T) {
	entries := []recipecatalogue.CatalogueEntry{
		planEntry("B1", domain.MealTypeBreakfast), planEntry("B2", domain.MealTypeBreakfast),
		planEntry("L1", domain.MealTypeLunch), planEntry("L2", domain.MealTypeLunch),
		planEntry("D1", domain.MealTypeDinner), planEntry("D2", domain.MealTypeDinner),
	}
	ids, err := selectPlan(entries, []domain.RecipeID{"B1", "L1", "D1"}, &sequenceChooser{})
	if err != nil {
		t.Fatalf("selectPlan() error = %v", err)
	}
	if ids != (IDs{Breakfast: "B2", Lunch: "L2", Dinner: "D2"}) {
		t.Fatalf("selectPlan() = %#v, want non-recent recipes", ids)
	}

	allRecent := []domain.RecipeID{"B1", "B2", "L1", "L2", "D1", "D2"}
	if _, err := selectPlan(entries, allRecent, &sequenceChooser{}); err != nil {
		t.Fatalf("selectPlan(all recent) error = %v", err)
	}
}

func TestGenerationVariesWithControlledChooser(t *testing.T) {
	entries := []recipecatalogue.CatalogueEntry{
		planEntry("B1", domain.MealTypeBreakfast), planEntry("B2", domain.MealTypeBreakfast),
		planEntry("L1", domain.MealTypeLunch), planEntry("L2", domain.MealTypeLunch),
		planEntry("D1", domain.MealTypeDinner), planEntry("D2", domain.MealTypeDinner),
	}
	first, err := selectPlan(entries, nil, &sequenceChooser{values: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := selectPlan(entries, nil, &sequenceChooser{values: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("controlled choices produced identical plans: %#v", first)
	}
}

func TestCanonicalSeedVariesEveryMealSlot(t *testing.T) {
	dataset, err := seed.Load(filepath.Join("..", "..", "seed"))
	if err != nil {
		t.Fatalf("load canonical seed: %v", err)
	}

	breakfasts := make(map[domain.RecipeID]struct{})
	lunches := make(map[domain.RecipeID]struct{})
	dinners := make(map[domain.RecipeID]struct{})
	for choice := range variedCandidateLimit {
		ids, selectErr := selectPlan(dataset.Entries, nil, &sequenceChooser{values: []int{choice, choice, choice}})
		if selectErr != nil {
			t.Fatalf("select canonical plan for choice %d: %v", choice, selectErr)
		}
		if ids.Breakfast == ids.Lunch || ids.Breakfast == ids.Dinner || ids.Lunch == ids.Dinner {
			t.Fatalf("choice %d produced duplicate recipes: %#v", choice, ids)
		}
		breakfasts[ids.Breakfast] = struct{}{}
		lunches[ids.Lunch] = struct{}{}
		dinners[ids.Dinner] = struct{}{}
	}

	if len(breakfasts) < 2 || len(lunches) < 2 || len(dinners) < 2 {
		t.Fatalf("canonical diversity breakfast=%v lunch=%v dinner=%v", breakfasts, lunches, dinners)
	}
}

func TestReplacePreservesOtherSlotsAndRejectsReplay(t *testing.T) {
	entries := []recipecatalogue.CatalogueEntry{
		planEntry("B1", domain.MealTypeBreakfast), planEntry("B2", domain.MealTypeBreakfast),
		planEntry("L1", domain.MealTypeLunch), planEntry("D1", domain.MealTypeDinner),
	}
	repository := newFakeRepository()
	service := newTestService(entries, repository, &sequenceChooser{})
	generated, err := service.Generate(t.Context(), 42, 0)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := service.Replace(t.Context(), 42, domain.MealTypeBreakfast, generated.Revision)
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if replaced.Breakfast.Recipe.ID == generated.Breakfast.Recipe.ID ||
		replaced.Lunch.Recipe.ID != generated.Lunch.Recipe.ID ||
		replaced.Dinner.Recipe.ID != generated.Dinner.Recipe.ID {
		t.Fatalf("replacement changed wrong slots: before=%#v after=%#v", generated.IDs(), replaced.IDs())
	}
	if _, err := service.Replace(t.Context(), 42, domain.MealTypeBreakfast, generated.Revision); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("replayed Replace() error = %v, want ErrStaleRevision", err)
	}
}

func TestReplaceReturnsNoAlternative(t *testing.T) {
	entries := []recipecatalogue.CatalogueEntry{
		planEntry("B1", domain.MealTypeBreakfast),
		planEntry("L1", domain.MealTypeLunch),
		planEntry("D1", domain.MealTypeDinner),
	}
	repository := newFakeRepository()
	service := newTestService(entries, repository, &sequenceChooser{})
	generated, err := service.Generate(t.Context(), 42, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Replace(t.Context(), 42, domain.MealTypeBreakfast, generated.Revision); !errors.Is(err, ErrNoAlternative) {
		t.Fatalf("Replace() error = %v, want ErrNoAlternative", err)
	}
}

func TestCurrentRejectsInactiveSavedRecipe(t *testing.T) {
	entries := []recipecatalogue.CatalogueEntry{
		planEntry("B1", domain.MealTypeBreakfast),
		planEntry("L1", domain.MealTypeLunch),
		planEntry("D1", domain.MealTypeDinner),
	}
	repository := newFakeRepository()
	repository.states[42] = Stored{IDs: IDs{Breakfast: "B1", Lunch: "L1", Dinner: "D1"}, Revision: 1}
	entries[2].Recipe.State = domain.RecipeStateInactive

	_, err := newTestService(entries, repository, &sequenceChooser{}).Current(t.Context(), 42)
	if !errors.Is(err, ErrPlanUnavailable) {
		t.Fatalf("Current() error = %v, want ErrPlanUnavailable", err)
	}
}

func TestShoppingListAggregatesOnlyMatchingIngredientAndUnit(t *testing.T) {
	breakfast := planEntry("B1", domain.MealTypeBreakfast)
	lunch := planEntry("L1", domain.MealTypeLunch)
	dinner := planEntry("D1", domain.MealTypeDinner)
	breakfast.Recipe.Ingredients = []domain.RecipeIngredient{{IngredientID: "rice", Quantity: 100, Unit: "г"}}
	breakfast.Ingredients = []domain.Ingredient{{ID: "rice", Name: "Рис"}}
	lunch.Recipe.Ingredients = []domain.RecipeIngredient{{IngredientID: "rice", Quantity: 250, Unit: "г"}}
	lunch.Ingredients = []domain.Ingredient{{ID: "rice", Name: "Рис"}}
	dinner.Recipe.Ingredients = []domain.RecipeIngredient{{IngredientID: "rice", Quantity: 1, Unit: "кг"}}
	dinner.Ingredients = []domain.Ingredient{{ID: "rice", Name: "Рис"}}

	items, err := BuildShoppingList(Snapshot{Breakfast: breakfast, Lunch: lunch, Dinner: dinner})
	if err != nil {
		t.Fatalf("BuildShoppingList() error = %v", err)
	}
	want := []ShoppingItem{
		{IngredientID: "rice", Name: "Рис", Quantity: 350, Unit: "г"},
		{IngredientID: "rice", Name: "Рис", Quantity: 1, Unit: "кг"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("shopping list = %#v, want %#v", items, want)
	}
}

func TestShoppingListRejectsInvalidQuantity(t *testing.T) {
	entry := planEntry("B1", domain.MealTypeBreakfast)
	entry.Recipe.Ingredients[0].Quantity = 0
	_, err := BuildShoppingList(Snapshot{Breakfast: entry, Lunch: planEntry("L1", domain.MealTypeLunch), Dinner: planEntry("D1", domain.MealTypeDinner)})
	if err == nil {
		t.Fatal("BuildShoppingList() error = nil")
	}
}

func newTestService(entries []recipecatalogue.CatalogueEntry, repository Repository, chooser Chooser) *Service {
	return NewService(&fakePlanCatalogue{entries: entries}, repository, chooser, func() time.Time {
		return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	})
}

func planEntry(id string, mealTypes ...domain.MealType) recipecatalogue.CatalogueEntry {
	ingredientID := domain.IngredientID("ingredient-" + id)
	return recipecatalogue.CatalogueEntry{
		Recipe: domain.Recipe{
			ID: domain.RecipeID(id), Name: "Recipe " + id, MealTypes: mealTypes, Category: "meal",
			Servings: PortionsPerRecipe, PreparationDuration: 10 * time.Minute, CookingDuration: 20 * time.Minute,
			StorageDays: Days, Difficulty: domain.DifficultyEasy, BudgetLevel: domain.BudgetLevelLow,
			ReheatScore: 4, BatchCookingScore: 5, State: domain.RecipeStateActive,
			Ingredients: []domain.RecipeIngredient{{IngredientID: ingredientID, Quantity: 1, Unit: "шт"}},
		},
		Ingredients: []domain.Ingredient{{ID: ingredientID, Name: "Ingredient " + id}},
	}
}

type fakePlanCatalogue struct {
	entries []recipecatalogue.CatalogueEntry
	err     error
}

func (f *fakePlanCatalogue) ListActive(context.Context) ([]recipecatalogue.CatalogueEntry, error) {
	return append([]recipecatalogue.CatalogueEntry(nil), f.entries...), f.err
}

type fakeRepository struct {
	states map[int64]Stored
}

func newFakeRepository() *fakeRepository { return &fakeRepository{states: make(map[int64]Stored)} }

func (f *fakeRepository) Load(_ context.Context, chatID int64) (Stored, error) {
	state, ok := f.states[chatID]
	if !ok {
		return Stored{}, ErrNotFound
	}
	state.Recent = append([]domain.RecipeID(nil), state.Recent...)
	return state, nil
}

func (f *fakeRepository) Save(_ context.Context, chatID, expectedRevision int64, ids IDs, at time.Time) (Stored, error) {
	current, exists := f.states[chatID]
	if (!exists && expectedRevision != 0) || (exists && current.Revision != expectedRevision) {
		return Stored{}, ErrStaleRevision
	}
	createdAt := current.CreatedAt
	if !exists {
		createdAt = at
	}
	recent := append(ids.Slice(), current.Recent...)
	if len(recent) > RecentLimit {
		recent = recent[:RecentLimit]
	}
	state := Stored{IDs: ids, Revision: expectedRevision + 1, CreatedAt: createdAt, UpdatedAt: at, Recent: recent}
	f.states[chatID] = state
	return state, nil
}

type sequenceChooser struct {
	values []int
	index  int
}

func (s *sequenceChooser) IntN(n int) int {
	if len(s.values) == 0 {
		return 0
	}
	value := s.values[s.index%len(s.values)]
	s.index++
	return value % n
}
