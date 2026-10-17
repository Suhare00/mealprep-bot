// Package recipe defines the recipe catalogue persistence boundary.
package recipe

import (
	"context"
	"errors"

	"github.com/Suhare00/mealprep-bot/internal/domain"
)

// ErrNotFound indicates that a requested recipe does not exist.
var ErrNotFound = errors.New("recipe not found")

// CatalogueEntry contains a recipe and the ingredient catalogue records it references.
type CatalogueEntry struct {
	Recipe      domain.Recipe
	Ingredients []domain.Ingredient
}

// Repository stores and retrieves complete recipe catalogue entries.
type Repository interface {
	Create(ctx context.Context, entry CatalogueEntry) error
	GetByID(ctx context.Context, id domain.RecipeID) (CatalogueEntry, error)
	Update(ctx context.Context, entry CatalogueEntry) error
	ListActive(ctx context.Context) ([]CatalogueEntry, error)
	Disable(ctx context.Context, id domain.RecipeID) error
}
