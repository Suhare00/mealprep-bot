//go:build integration

package postgres

import (
	"errors"
	"testing"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

func TestCatalogueServiceReadsSeededActiveRecipes(t *testing.T) {
	pool, repository := newTestRepository(t)
	if _, err := NewSeedImporter(pool).Import(t.Context(), loadCanonicalDataset(t)); err != nil {
		t.Fatalf("import canonical seed: %v", err)
	}
	catalogue := recipecatalogue.NewCatalogue(repository)

	entries, err := catalogue.ListActive(t.Context())
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(entries) != 90 {
		t.Fatalf("ListActive() count = %d, want 90", len(entries))
	}
	if entries[0].Recipe.ID != "R001" || entries[len(entries)-1].Recipe.ID != "R090" {
		t.Fatalf("ListActive() boundaries = %q..%q, want R001..R090",
			entries[0].Recipe.ID, entries[len(entries)-1].Recipe.ID)
	}

	entry, err := catalogue.GetActiveByID(t.Context(), domain.RecipeID("R021"))
	if err != nil {
		t.Fatalf("GetActiveByID(R021) error = %v", err)
	}
	if len(entry.Recipe.MealTypes) != 2 || len(entry.Recipe.Ingredients) == 0 || len(entry.Ingredients) == 0 {
		t.Fatalf("GetActiveByID(R021) returned incomplete entry: %#v", entry)
	}

	if err := repository.Disable(t.Context(), "R021"); err != nil {
		t.Fatalf("Disable(R021) error = %v", err)
	}
	_, err = catalogue.GetActiveByID(t.Context(), "R021")
	if !errors.Is(err, recipecatalogue.ErrUnavailable) {
		t.Fatalf("GetActiveByID(disabled) error = %v, want ErrUnavailable", err)
	}
}
