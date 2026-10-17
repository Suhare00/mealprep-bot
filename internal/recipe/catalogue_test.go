package recipe

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Suhare00/mealprep-bot/internal/domain"
)

func TestCatalogueListActiveSortsAndFilters(t *testing.T) {
	reader := &fakeCatalogueReader{entries: []CatalogueEntry{
		{Recipe: domain.Recipe{ID: "R003", State: domain.RecipeStateActive}},
		{Recipe: domain.Recipe{ID: "R002", State: domain.RecipeStateInactive}},
		{Recipe: domain.Recipe{ID: "R001", State: domain.RecipeStateActive}},
	}}

	entries, err := NewCatalogue(reader).ListActive(t.Context())
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	got := []domain.RecipeID{entries[0].Recipe.ID, entries[1].Recipe.ID}
	want := []domain.RecipeID{"R001", "R003"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListActive() IDs = %v, want %v", got, want)
	}
}

func TestCatalogueGetActiveByID(t *testing.T) {
	infrastructureError := errors.New("database unavailable")
	tests := []struct {
		name      string
		entry     CatalogueEntry
		err       error
		wantError error
	}{
		{name: "active", entry: CatalogueEntry{Recipe: domain.Recipe{ID: "R001", State: domain.RecipeStateActive}}},
		{name: "inactive", entry: CatalogueEntry{Recipe: domain.Recipe{ID: "R001", State: domain.RecipeStateInactive}}, wantError: ErrUnavailable},
		{name: "missing", err: ErrNotFound, wantError: ErrUnavailable},
		{name: "infrastructure", err: infrastructureError, wantError: infrastructureError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &fakeCatalogueReader{entry: test.entry, err: test.err}
			got, err := NewCatalogue(reader).GetActiveByID(t.Context(), "R001")
			if !errors.Is(err, test.wantError) {
				t.Fatalf("GetActiveByID() error = %v, want %v", err, test.wantError)
			}
			if test.wantError == nil && got.Recipe.ID != "R001" {
				t.Fatalf("GetActiveByID() ID = %q", got.Recipe.ID)
			}
		})
	}
}

func TestCataloguePropagatesContext(t *testing.T) {
	reader := &fakeCatalogueReader{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reader.listErr = ctx.Err()

	_, err := NewCatalogue(reader).ListActive(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListActive() error = %v, want context.Canceled", err)
	}
}

type fakeCatalogueReader struct {
	entries []CatalogueEntry
	entry   CatalogueEntry
	listErr error
	err     error
}

func (f *fakeCatalogueReader) ListActive(context.Context) ([]CatalogueEntry, error) {
	return f.entries, f.listErr
}

func (f *fakeCatalogueReader) GetByID(context.Context, domain.RecipeID) (CatalogueEntry, error) {
	return f.entry, f.err
}
