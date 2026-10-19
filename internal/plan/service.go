package plan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

// Service generates and reads complete persisted plans.
type Service struct {
	catalogue  Catalogue
	repository Repository
	chooser    Chooser
	now        func() time.Time
}

func NewService(catalogue Catalogue, repository Repository, chooser Chooser, now func() time.Time) *Service {
	if chooser == nil {
		chooser = randomChooser{}
	}
	if now == nil {
		now = time.Now
	}
	return &Service{catalogue: catalogue, repository: repository, chooser: chooser, now: now}
}

func (s *Service) Current(ctx context.Context, chatID int64) (Snapshot, error) {
	stored, err := s.repository.Load(ctx, chatID)
	if err != nil {
		return Snapshot{}, err
	}
	entries, err := s.catalogue.ListActive(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list recipes for current plan: %w", err)
	}
	return hydrate(stored, entries)
}

func (s *Service) Generate(ctx context.Context, chatID, expectedRevision int64) (Snapshot, error) {
	stored, err := s.loadForMutation(ctx, chatID, expectedRevision)
	if err != nil {
		return Snapshot{}, err
	}
	entries, err := s.catalogue.ListActive(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list recipes for plan generation: %w", err)
	}
	ids, err := selectPlan(entries, stored.Recent, s.chooser)
	if err != nil {
		return Snapshot{}, err
	}
	saved, err := s.repository.Save(ctx, chatID, expectedRevision, ids, s.now().UTC())
	if err != nil {
		return Snapshot{}, fmt.Errorf("save generated plan: %w", err)
	}
	return hydrate(saved, entries)
}

func (s *Service) Replace(ctx context.Context, chatID int64, mealType domain.MealType, expectedRevision int64) (Snapshot, error) {
	stored, err := s.loadForMutation(ctx, chatID, expectedRevision)
	if err != nil {
		return Snapshot{}, err
	}
	if stored.Revision == 0 {
		return Snapshot{}, ErrNotFound
	}
	entries, err := s.catalogue.ListActive(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list recipes for replacement: %w", err)
	}
	if _, err := hydrate(stored, entries); err != nil {
		return Snapshot{}, err
	}
	replacement, err := selectReplacement(entries, stored.IDs, mealType, stored.Recent, s.chooser)
	if err != nil {
		return Snapshot{}, err
	}
	ids := stored.IDs
	ids.Set(mealType, replacement)
	saved, err := s.repository.Save(ctx, chatID, expectedRevision, ids, s.now().UTC())
	if err != nil {
		return Snapshot{}, fmt.Errorf("save replacement: %w", err)
	}
	return hydrate(saved, entries)
}

func (s *Service) ShoppingList(ctx context.Context, chatID int64) (Snapshot, []ShoppingItem, error) {
	snapshot, err := s.Current(ctx, chatID)
	if err != nil {
		return Snapshot{}, nil, err
	}
	items, err := BuildShoppingList(snapshot)
	if err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, items, nil
}

// RecipeShoppingList returns the saved plan and the uncombined ingredient
// quantities for one meal slot.
func (s *Service) RecipeShoppingList(ctx context.Context, chatID int64, mealType domain.MealType) (Snapshot, []ShoppingItem, error) {
	if mealType != domain.MealTypeBreakfast && mealType != domain.MealTypeLunch && mealType != domain.MealTypeDinner {
		return Snapshot{}, nil, &EligibilityError{MealType: mealType}
	}
	snapshot, err := s.Current(ctx, chatID)
	if err != nil {
		return Snapshot{}, nil, err
	}
	items, err := BuildRecipeShoppingList(snapshot.Entry(mealType))
	if err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, items, nil
}

func (s *Service) loadForMutation(ctx context.Context, chatID, expectedRevision int64) (Stored, error) {
	stored, err := s.repository.Load(ctx, chatID)
	if errors.Is(err, ErrNotFound) {
		stored = Stored{}
	} else if err != nil {
		return Stored{}, fmt.Errorf("load meal plan state: %w", err)
	}
	if stored.Revision != expectedRevision {
		return Stored{}, ErrStaleRevision
	}
	return stored, nil
}

func hydrate(stored Stored, entries []recipecatalogue.CatalogueEntry) (Snapshot, error) {
	snapshot := Snapshot{Revision: stored.Revision, CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt}
	byID := make(map[domain.RecipeID]recipecatalogue.CatalogueEntry, len(entries))
	for _, entry := range entries {
		byID[entry.Recipe.ID] = entry
	}
	breakfast, breakfastOK := byID[stored.IDs.Breakfast]
	lunch, lunchOK := byID[stored.IDs.Lunch]
	dinner, dinnerOK := byID[stored.IDs.Dinner]
	if !breakfastOK || !lunchOK || !dinnerOK ||
		!isEligibleFor(breakfast, domain.MealTypeBreakfast) ||
		!isEligibleFor(lunch, domain.MealTypeLunch) ||
		!isEligibleFor(dinner, domain.MealTypeDinner) {
		return snapshot, ErrPlanUnavailable
	}
	snapshot = Snapshot{
		Breakfast: breakfast, Lunch: lunch, Dinner: dinner,
		Revision: stored.Revision, CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt,
	}
	return snapshot, nil
}

func isEligibleFor(entry recipecatalogue.CatalogueEntry, mealType domain.MealType) bool {
	return len(eligible([]recipecatalogue.CatalogueEntry{entry}, mealType)) == 1
}
