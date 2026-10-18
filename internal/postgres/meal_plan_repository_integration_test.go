//go:build integration

package postgres

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	"github.com/Suhare00/mealprep-bot/internal/plan"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

func TestMealPlanMigrationsApplyAndReverseWithoutDroppingCatalogue(t *testing.T) {
	pool := openTestPool(t)
	resetPlanSchema(t, pool)
	applyMigration(t, pool, "000001_recipe_catalogue.up.sql")
	applyMigration(t, pool, "000002_meal_plans.up.sql")

	for _, table := range []string{"meal_plans", "meal_plan_recent_selections"} {
		var exists bool
		if err := pool.QueryRow(t.Context(), `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s was not created", table)
		}
	}

	applyMigration(t, pool, "000002_meal_plans.down.sql")
	var recipesExist bool
	if err := pool.QueryRow(t.Context(), `SELECT to_regclass('public.recipes') IS NOT NULL`).Scan(&recipesExist); err != nil {
		t.Fatal(err)
	}
	if !recipesExist {
		t.Fatal("reversing plan migration dropped recipe catalogue")
	}
	applyMigration(t, pool, "000001_recipe_catalogue.down.sql")
}

func TestMealPlanRepositoryPersistenceIsolationRevisionAndRollback(t *testing.T) {
	pool, recipeRepository, repository := newPlanTestRepository(t)
	for _, entry := range []recipecatalogue.CatalogueEntry{
		testPlanEntry("B1", domain.MealTypeBreakfast),
		testPlanEntry("B2", domain.MealTypeBreakfast),
		testPlanEntry("L1", domain.MealTypeLunch),
		testPlanEntry("D1", domain.MealTypeDinner),
	} {
		if err := recipeRepository.Create(t.Context(), entry); err != nil {
			t.Fatalf("create recipe %s: %v", entry.Recipe.ID, err)
		}
	}
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	firstIDs := plan.IDs{Breakfast: "B1", Lunch: "L1", Dinner: "D1"}
	first, err := repository.Save(t.Context(), 100, 0, firstIDs, at)
	if err != nil {
		t.Fatalf("Save(initial) error = %v", err)
	}
	if first.Revision != 1 {
		t.Fatalf("initial revision = %d", first.Revision)
	}

	restarted := NewMealPlanRepository(pool)
	loaded, err := restarted.Load(t.Context(), 100)
	if err != nil || loaded.IDs != firstIDs {
		t.Fatalf("Load(after restart) = %#v, %v", loaded, err)
	}
	if _, err := restarted.Load(t.Context(), 200); !errors.Is(err, plan.ErrNotFound) {
		t.Fatalf("Load(other chat) error = %v", err)
	}
	if _, err := restarted.Save(t.Context(), 100, 0, firstIDs, at); !errors.Is(err, plan.ErrStaleRevision) {
		t.Fatalf("Save(replay) error = %v", err)
	}

	secondIDs := plan.IDs{Breakfast: "B2", Lunch: "L1", Dinner: "D1"}
	updated, err := restarted.Save(t.Context(), 100, 1, secondIDs, at.Add(time.Minute))
	if err != nil || updated.Revision != 2 {
		t.Fatalf("Save(update) = %#v, %v", updated, err)
	}
	if _, err := restarted.Save(t.Context(), 100, 2,
		plan.IDs{Breakfast: "missing", Lunch: "L1", Dinner: "D1"}, at.Add(2*time.Minute)); err == nil {
		t.Fatal("Save(invalid FK) error = nil")
	}
	afterRollback, err := restarted.Load(t.Context(), 100)
	if err != nil || afterRollback.Revision != 2 || afterRollback.IDs != secondIDs {
		t.Fatalf("state after rollback = %#v, %v", afterRollback, err)
	}
	currentRevision := afterRollback.Revision
	for i := 0; i < 4; i++ {
		ids := firstIDs
		if i%2 == 0 {
			ids = secondIDs
		}
		saved, saveErr := restarted.Save(t.Context(), 100, currentRevision, ids, at.Add(time.Duration(3+i)*time.Minute))
		if saveErr != nil {
			t.Fatalf("Save(history %d) error = %v", i, saveErr)
		}
		currentRevision = saved.Revision
	}
	withHistory, err := restarted.Load(t.Context(), 100)
	if err != nil || len(withHistory.Recent) != plan.RecentLimit {
		t.Fatalf("bounded recent history = %d entries, %v", len(withHistory.Recent), err)
	}

	other, err := restarted.Save(t.Context(), 200, 0, firstIDs, at)
	if err != nil || other.Revision != 1 {
		t.Fatalf("Save(other chat) = %#v, %v", other, err)
	}
}

func TestMealPlanRepositoryConcurrentRevisionAllowsOneMutation(t *testing.T) {
	_, recipeRepository, repository := newPlanTestRepository(t)
	for _, entry := range []recipecatalogue.CatalogueEntry{
		testPlanEntry("B1", domain.MealTypeBreakfast), testPlanEntry("B2", domain.MealTypeBreakfast),
		testPlanEntry("L1", domain.MealTypeLunch), testPlanEntry("D1", domain.MealTypeDinner),
	} {
		if err := recipeRepository.Create(t.Context(), entry); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	initial := plan.IDs{Breakfast: "B1", Lunch: "L1", Dinner: "D1"}
	if _, err := repository.Save(t.Context(), 42, 0, initial, at); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errorsByCall := make([]error, 2)
	for i := range errorsByCall {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errorsByCall[index] = repository.Save(t.Context(), 42, 1,
				plan.IDs{Breakfast: "B2", Lunch: "L1", Dinner: "D1"}, at.Add(time.Minute))
		}(i)
	}
	wg.Wait()
	successes, stale := 0, 0
	for _, err := range errorsByCall {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, plan.ErrStaleRevision):
			stale++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent results success=%d stale=%d", successes, stale)
	}
}

func TestMealPlanServiceUsesCanonicalSeedAndSurvivesRestart(t *testing.T) {
	pool, recipeRepository, planRepository := newPlanTestRepository(t)
	if _, err := NewSeedImporter(pool).Import(t.Context(), loadCanonicalDataset(t)); err != nil {
		t.Fatalf("import canonical seed: %v", err)
	}
	catalogue := recipecatalogue.NewCatalogue(recipeRepository)
	service := plan.NewService(catalogue, planRepository, fixedChooser{}, func() time.Time {
		return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	})

	generated, err := service.Generate(t.Context(), 42, 0)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	assertCanonicalPlan(t, generated)
	itemsBefore, err := plan.BuildShoppingList(generated)
	if err != nil || len(itemsBefore) == 0 {
		t.Fatalf("shopping list = %#v, %v", itemsBefore, err)
	}

	restartedService := plan.NewService(catalogue, NewMealPlanRepository(pool), fixedChooser{}, time.Now)
	loaded, err := restartedService.Current(t.Context(), 42)
	if err != nil || loaded.IDs() != generated.IDs() {
		t.Fatalf("Current(after restart) = %#v, %v", loaded.IDs(), err)
	}
	replaced, err := restartedService.Replace(t.Context(), 42, domain.MealTypeBreakfast, loaded.Revision)
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if replaced.Breakfast.Recipe.ID == loaded.Breakfast.Recipe.ID || replaced.Lunch.Recipe.ID != loaded.Lunch.Recipe.ID || replaced.Dinner.Recipe.ID != loaded.Dinner.Recipe.ID {
		t.Fatalf("replacement changed incorrect slots: before=%#v after=%#v", loaded.IDs(), replaced.IDs())
	}
	_, itemsAfter, err := restartedService.ShoppingList(t.Context(), 42)
	if err != nil || len(itemsAfter) == 0 {
		t.Fatalf("ShoppingList(after replacement) = %#v, %v", itemsAfter, err)
	}
	if reflect.DeepEqual(itemsBefore, itemsAfter) {
		t.Fatal("shopping list did not change after replacing a recipe")
	}
	regenerated, err := restartedService.Generate(t.Context(), 42, replaced.Revision)
	if err != nil {
		t.Fatalf("Generate(regenerate) error = %v", err)
	}
	if regenerated.IDs() == replaced.IDs() {
		t.Fatalf("regeneration reused the complete recent plan: %#v", regenerated.IDs())
	}
	if err := recipeRepository.Disable(t.Context(), regenerated.Lunch.Recipe.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := restartedService.Current(t.Context(), 42); !errors.Is(err, plan.ErrPlanUnavailable) {
		t.Fatalf("Current(inactive reference) error = %v", err)
	}
}

func newPlanTestRepository(t *testing.T) (*pgxpool.Pool, *RecipeRepository, *MealPlanRepository) {
	t.Helper()
	pool := openTestPool(t)
	resetPlanSchema(t, pool)
	applyMigration(t, pool, "000001_recipe_catalogue.up.sql")
	applyMigration(t, pool, "000002_meal_plans.up.sql")
	t.Cleanup(func() { applyMigration(t, pool, "000002_meal_plans.down.sql") })
	return pool, NewRecipeRepository(pool), NewMealPlanRepository(pool)
}

func resetPlanSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	applyMigration(t, pool, "000002_meal_plans.down.sql")
	applyMigration(t, pool, "000001_recipe_catalogue.down.sql")
}

func testPlanEntry(id string, mealType domain.MealType) recipecatalogue.CatalogueEntry {
	entry := testEntry(id)
	entry.Recipe.MealTypes = []domain.MealType{mealType}
	entry.Recipe.Name = "Plan recipe " + id
	return entry
}

func assertCanonicalPlan(t *testing.T, snapshot plan.Snapshot) {
	t.Helper()
	ids := snapshot.IDs().Slice()
	if ids[0] == ids[1] || ids[0] == ids[2] || ids[1] == ids[2] {
		t.Fatalf("plan IDs are not distinct: %v", ids)
	}
	for mealType, entry := range map[domain.MealType]recipecatalogue.CatalogueEntry{
		domain.MealTypeBreakfast: snapshot.Breakfast,
		domain.MealTypeLunch:     snapshot.Lunch,
		domain.MealTypeDinner:    snapshot.Dinner,
	} {
		if entry.Recipe.Servings != plan.PortionsPerRecipe || entry.Recipe.StorageDays < plan.Days || entry.Recipe.State != domain.RecipeStateActive {
			t.Fatalf("ineligible %s recipe: %#v", mealType, entry.Recipe)
		}
	}
}

type fixedChooser struct{}

func (fixedChooser) IntN(int) int { return 0 }
