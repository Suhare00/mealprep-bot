package domain

import (
	"strings"
	"testing"
)

func TestRecipeValidateAcceptsOptionalOrderedEnrichment(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Recipe)
	}{
		{name: "no steps or media", mutate: func(*Recipe) {}},
		{name: "Russian steps and all image placements", mutate: addValidEnrichment},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe := validRecipe()
			test.mutate(&recipe)
			if err := recipe.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestRecipeValidateRejectsInvalidEnrichment(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Recipe)
		wantError string
	}{
		{name: "blank step id", mutate: func(r *Recipe) { addValidEnrichment(r); r.Steps[0].ID = " " }, wantError: "step ID"},
		{name: "duplicate step id", mutate: func(r *Recipe) { addValidEnrichment(r); r.Steps[1].ID = r.Steps[0].ID }, wantError: "duplicated"},
		{name: "zero step position", mutate: func(r *Recipe) { addValidEnrichment(r); r.Steps[0].Position = 0 }, wantError: "position 1"},
		{name: "step gap", mutate: func(r *Recipe) { addValidEnrichment(r); r.Steps[1].Position = 3 }, wantError: "position 2"},
		{name: "whitespace step text", mutate: func(r *Recipe) { addValidEnrichment(r); r.Steps[0].Text = " \t" }, wantError: "text is required"},
		{name: "unknown step image", mutate: func(r *Recipe) { addValidEnrichment(r); r.Steps[0].ImageID = mediaID("unknown") }, wantError: "unknown media"},
		{name: "blank media id", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[0].ID = "" }, wantError: "media ID"},
		{name: "duplicate media id", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[1].ID = r.Media[0].ID }, wantError: "media ID"},
		{name: "duplicate storage key", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[1].StorageKey = r.Media[0].StorageKey }, wantError: "storage key"},
		{name: "path traversal key", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[0].StorageKey = "../photo.jpg" }, wantError: "storage key"},
		{name: "nested key", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[0].StorageKey = "folder/photo.jpg" }, wantError: "storage key"},
		{name: "invalid MIME", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[0].MIMEType = "image/gif" }, wantError: "MIME"},
		{name: "zero width", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[0].Width = 0 }, wantError: "dimensions"},
		{name: "too many pixels", mutate: func(r *Recipe) {
			addValidEnrichment(r)
			r.Media[0].Width = MaxImageDimension
			r.Media[0].Height = MaxImageDimension
		}, wantError: "limits"},
		{name: "whitespace alt text", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[0].AltText = " \n" }, wantError: "alt text"},
		{name: "whitespace caption", mutate: func(r *Recipe) { addValidEnrichment(r); r.Media[0].Caption = " \n" }, wantError: "caption"},
		{name: "unknown cover", mutate: func(r *Recipe) { addValidEnrichment(r); r.CoverImageID = mediaID("unknown") }, wantError: "unknown media"},
		{name: "gallery position gap", mutate: func(r *Recipe) { addValidEnrichment(r); r.Images[0].Position = 2 }, wantError: "position 1"},
		{name: "reused media", mutate: func(r *Recipe) { addValidEnrichment(r); r.Images[0].MediaID = *r.CoverImageID }, wantError: "referenced by both"},
		{name: "unreferenced media", mutate: func(r *Recipe) { addValidEnrichment(r); r.CoverImageID = nil }, wantError: "not referenced"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe := validRecipe()
			test.mutate(&recipe)
			err := recipe.Validate()
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want text %q", err, test.wantError)
			}
		})
	}
}

func addValidEnrichment(recipe *Recipe) {
	recipe.Media = []RecipeMedia{
		{ID: "media-cover", StorageKey: "a1-cover.jpg", MIMEType: MediaTypeJPEG, Width: 1200, Height: 800, AltText: "Готовое блюдо"},
		{ID: "media-extra", StorageKey: "b2-extra.png", MIMEType: MediaTypePNG, Width: 800, Height: 600, Caption: "Подача"},
		{ID: "media-step", StorageKey: "c3-step.jpg", MIMEType: MediaTypeJPEG, Width: 640, Height: 480},
	}
	recipe.CoverImageID = mediaID("media-cover")
	recipe.Images = []RecipeImage{{MediaID: "media-extra", Position: 1}}
	recipe.Steps = []RecipeStep{
		{ID: "step-1", Position: 1, Text: "Нарежьте овощи.", ImageID: mediaID("media-step")},
		{ID: "step-2", Position: 2, Text: "Тушите до готовности."},
	}
}

func mediaID(value MediaID) *MediaID {
	return &value
}
