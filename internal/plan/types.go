// Package plan implements the Telegram-independent three-day meal-planning use case.
package plan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

const (
	People            = 2
	Days              = 3
	PortionsPerRecipe = People * Days
	TotalPortions     = PortionsPerRecipe * 3
	RecentLimit       = 12
)

var (
	ErrNotFound            = errors.New("meal plan not found")
	ErrStaleRevision       = errors.New("meal plan revision is stale")
	ErrPlanUnavailable     = errors.New("saved meal plan contains unavailable recipes")
	ErrNoAlternative       = errors.New("no alternative recipe available")
	ErrInsufficientRecipes = errors.New("eligible recipes cannot form three distinct meal slots")
)

// EligibilityError reports a meal slot for which no complete plan can be built.
type EligibilityError struct {
	MealType domain.MealType
}

func (e *EligibilityError) Error() string {
	return fmt.Sprintf("no eligible %s recipe", e.MealType)
}

// IDs identifies the three distinct recipe batches in a plan.
type IDs struct {
	Breakfast domain.RecipeID
	Lunch     domain.RecipeID
	Dinner    domain.RecipeID
}

func (ids IDs) For(mealType domain.MealType) domain.RecipeID {
	switch mealType {
	case domain.MealTypeBreakfast:
		return ids.Breakfast
	case domain.MealTypeLunch:
		return ids.Lunch
	case domain.MealTypeDinner:
		return ids.Dinner
	default:
		return ""
	}
}

func (ids *IDs) Set(mealType domain.MealType, id domain.RecipeID) {
	switch mealType {
	case domain.MealTypeBreakfast:
		ids.Breakfast = id
	case domain.MealTypeLunch:
		ids.Lunch = id
	case domain.MealTypeDinner:
		ids.Dinner = id
	}
}

func (ids IDs) Slice() []domain.RecipeID {
	return []domain.RecipeID{ids.Breakfast, ids.Lunch, ids.Dinner}
}

// Stored is the persistence-neutral current plan plus bounded recent selections.
type Stored struct {
	IDs       IDs
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
	Recent    []domain.RecipeID
}

// Snapshot is a fully hydrated plan safe for display and shopping calculations.
type Snapshot struct {
	Breakfast recipecatalogue.CatalogueEntry
	Lunch     recipecatalogue.CatalogueEntry
	Dinner    recipecatalogue.CatalogueEntry
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s Snapshot) IDs() IDs {
	return IDs{
		Breakfast: s.Breakfast.Recipe.ID,
		Lunch:     s.Lunch.Recipe.ID,
		Dinner:    s.Dinner.Recipe.ID,
	}
}

func (s Snapshot) Entry(mealType domain.MealType) recipecatalogue.CatalogueEntry {
	switch mealType {
	case domain.MealTypeBreakfast:
		return s.Breakfast
	case domain.MealTypeLunch:
		return s.Lunch
	case domain.MealTypeDinner:
		return s.Dinner
	default:
		return recipecatalogue.CatalogueEntry{}
	}
}

// Repository persists the current plan and recent selections atomically.
type Repository interface {
	Load(ctx context.Context, chatID int64) (Stored, error)
	Save(ctx context.Context, chatID int64, expectedRevision int64, ids IDs, at time.Time) (Stored, error)
}

// Catalogue is the recipe read capability used by planning.
type Catalogue interface {
	ListActive(ctx context.Context) ([]recipecatalogue.CatalogueEntry, error)
}

// Chooser supplies controlled variation. Tests provide deterministic implementations.
type Chooser interface {
	IntN(n int) int
}

// ShoppingItem is one consolidated ingredient+unit requirement.
type ShoppingItem struct {
	IngredientID domain.IngredientID
	Name         string
	Quantity     float64
	Unit         string
}
