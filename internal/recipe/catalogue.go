package recipe

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Suhare00/mealprep-bot/internal/domain"
)

// ErrUnavailable means a recipe is missing or is not active.
var ErrUnavailable = errors.New("recipe unavailable")

// CatalogueReader contains the persistence operations needed by the read use case.
type CatalogueReader interface {
	ListActive(ctx context.Context) ([]CatalogueEntry, error)
	GetByID(ctx context.Context, id domain.RecipeID) (CatalogueEntry, error)
}

// Catalogue provides Telegram-independent read access to active recipes.
type Catalogue struct {
	reader CatalogueReader
}

// NewCatalogue creates an active-recipe catalogue service.
func NewCatalogue(reader CatalogueReader) *Catalogue {
	return &Catalogue{reader: reader}
}

// ListActive returns active recipes in stable ID order.
func (c *Catalogue) ListActive(ctx context.Context) ([]CatalogueEntry, error) {
	entries, err := c.reader.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active recipes: %w", err)
	}

	active := make([]CatalogueEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Recipe.State == domain.RecipeStateActive {
			active = append(active, entry)
		}
	}
	sort.SliceStable(active, func(i, j int) bool {
		return active[i].Recipe.ID < active[j].Recipe.ID
	})
	return active, nil
}

// GetActiveByID returns a recipe only while it is active.
func (c *Catalogue) GetActiveByID(ctx context.Context, id domain.RecipeID) (CatalogueEntry, error) {
	entry, err := c.reader.GetByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return CatalogueEntry{}, fmt.Errorf("%w: %s", ErrUnavailable, id)
	}
	if err != nil {
		return CatalogueEntry{}, fmt.Errorf("get recipe %q: %w", id, err)
	}
	if entry.Recipe.State != domain.RecipeStateActive {
		return CatalogueEntry{}, fmt.Errorf("%w: %s", ErrUnavailable, id)
	}
	return entry, nil
}
