package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	MaxImageDimension = 10_000
	MaxImagePixels    = 40_000_000
)

// RecipeMedia describes an image stored outside PostgreSQL.
type RecipeMedia struct {
	ID         MediaID
	StorageKey StorageKey
	MIMEType   MediaType
	Width      int
	Height     int
	AltText    string
	Caption    string
}

// Validate checks persisted recipe media metadata.
func (m RecipeMedia) Validate() error {
	if blank(string(m.ID)) {
		return errors.New("media ID is required")
	}
	if err := m.StorageKey.Validate(); err != nil {
		return err
	}
	if !m.MIMEType.valid() {
		return fmt.Errorf("media MIME type %q is invalid", m.MIMEType)
	}
	if m.Width <= 0 || m.Height <= 0 {
		return errors.New("media dimensions must be positive")
	}
	if m.Width > MaxImageDimension || m.Height > MaxImageDimension || int64(m.Width)*int64(m.Height) > MaxImagePixels {
		return errors.New("media dimensions exceed supported limits")
	}
	if m.AltText != "" && blank(m.AltText) {
		return errors.New("media alt text must not be whitespace-only")
	}
	if m.Caption != "" && blank(m.Caption) {
		return errors.New("media caption must not be whitespace-only")
	}
	return nil
}

// Validate rejects path-like or malformed storage keys. Keys are opaque file names,
// not paths, so callers cannot escape the configured media root.
func (k StorageKey) Validate() error {
	value := string(k)
	if value == "" || strings.TrimSpace(value) != value || len(value) > 255 || value == "." || value == ".." {
		return errors.New("media storage key is invalid")
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_' || character == '.' {
			continue
		}
		return errors.New("media storage key is invalid")
	}
	if strings.Contains(value, "..") || strings.HasPrefix(value, ".") {
		return errors.New("media storage key is invalid")
	}
	return nil
}

// RecipeImage places media in the ordered additional-image collection.
type RecipeImage struct {
	MediaID  MediaID
	Position int
}

// RecipeStep is one item in the ordered preparation instructions.
type RecipeStep struct {
	ID       RecipeStepID
	Position int
	Text     string
	ImageID  *MediaID
}

func validateRecipeEnrichment(recipe Recipe) error {
	mediaByID := make(map[MediaID]struct{}, len(recipe.Media))
	keys := make(map[StorageKey]struct{}, len(recipe.Media))
	for index, media := range recipe.Media {
		if err := media.Validate(); err != nil {
			return fmt.Errorf("media at index %d: %w", index, err)
		}
		if _, duplicate := mediaByID[media.ID]; duplicate {
			return fmt.Errorf("media ID %q is duplicated", media.ID)
		}
		if _, duplicate := keys[media.StorageKey]; duplicate {
			return fmt.Errorf("media storage key %q is duplicated", media.StorageKey)
		}
		mediaByID[media.ID] = struct{}{}
		keys[media.StorageKey] = struct{}{}
	}

	referenced := make(map[MediaID]string, len(recipe.Media))
	validateReference := func(id MediaID, owner string) error {
		if blank(string(id)) {
			return fmt.Errorf("%s media reference is required", owner)
		}
		if _, exists := mediaByID[id]; !exists {
			return fmt.Errorf("%s references unknown media %q", owner, id)
		}
		if previous, duplicate := referenced[id]; duplicate {
			return fmt.Errorf("media %q is referenced by both %s and %s", id, previous, owner)
		}
		referenced[id] = owner
		return nil
	}

	if recipe.CoverImageID != nil {
		if err := validateReference(*recipe.CoverImageID, "cover image"); err != nil {
			return err
		}
	}
	for index, image := range recipe.Images {
		if image.Position != index+1 {
			return fmt.Errorf("additional image at index %d must have position %d", index, index+1)
		}
		if err := validateReference(image.MediaID, fmt.Sprintf("additional image at position %d", image.Position)); err != nil {
			return err
		}
	}

	stepIDs := make(map[RecipeStepID]struct{}, len(recipe.Steps))
	for index, step := range recipe.Steps {
		if blank(string(step.ID)) {
			return fmt.Errorf("step at index %d: step ID is required", index)
		}
		if _, duplicate := stepIDs[step.ID]; duplicate {
			return fmt.Errorf("step ID %q is duplicated", step.ID)
		}
		stepIDs[step.ID] = struct{}{}
		if step.Position != index+1 {
			return fmt.Errorf("step at index %d must have position %d", index, index+1)
		}
		if blank(step.Text) {
			return fmt.Errorf("step at position %d text is required", step.Position)
		}
		if step.ImageID != nil {
			if err := validateReference(*step.ImageID, fmt.Sprintf("step at position %d", step.Position)); err != nil {
				return err
			}
		}
	}
	if len(referenced) != len(mediaByID) {
		for id := range mediaByID {
			if _, ok := referenced[id]; !ok {
				return fmt.Errorf("media %q is not referenced by the recipe", id)
			}
		}
	}
	return nil
}
