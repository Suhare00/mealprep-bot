package domain

// RecipeID uniquely identifies a recipe independently of its persistence representation.
type RecipeID string

// IngredientID uniquely identifies an ingredient independently of its persistence representation.
type IngredientID string

// RecipeStepID uniquely identifies one preparation step.
type RecipeStepID string

// MediaID uniquely identifies recipe media metadata independently of storage.
type MediaID string

// StorageKey is an opaque, single-segment key assigned by a media store.
type StorageKey string

// MediaType is the validated MIME type of a stored recipe image.
type MediaType string

const (
	MediaTypeJPEG MediaType = "image/jpeg"
	MediaTypePNG  MediaType = "image/png"
)

func (m MediaType) valid() bool {
	switch m {
	case MediaTypeJPEG, MediaTypePNG:
		return true
	default:
		return false
	}
}

// MealType identifies when a recipe is intended to be served.
type MealType string

const (
	MealTypeBreakfast MealType = "breakfast"
	MealTypeLunch     MealType = "lunch"
	MealTypeDinner    MealType = "dinner"
)

func (m MealType) valid() bool {
	switch m {
	case MealTypeBreakfast, MealTypeLunch, MealTypeDinner:
		return true
	default:
		return false
	}
}

// Difficulty describes the effort required to prepare a recipe.
type Difficulty string

const (
	DifficultyEasy   Difficulty = "easy"
	DifficultyMedium Difficulty = "medium"
)

func (d Difficulty) valid() bool {
	switch d {
	case DifficultyEasy, DifficultyMedium:
		return true
	default:
		return false
	}
}

// BudgetLevel describes the relative cost of a recipe.
type BudgetLevel string

const (
	BudgetLevelLow    BudgetLevel = "low"
	BudgetLevelMedium BudgetLevel = "medium"
	BudgetLevelHigh   BudgetLevel = "high"
)

func (b BudgetLevel) valid() bool {
	switch b {
	case BudgetLevelLow, BudgetLevelMedium, BudgetLevelHigh:
		return true
	default:
		return false
	}
}

// RecipeState represents whether a recipe is available or soft-deleted.
type RecipeState string

const (
	RecipeStateActive   RecipeState = "active"
	RecipeStateInactive RecipeState = "inactive"
)

func (s RecipeState) valid() bool {
	switch s {
	case RecipeStateActive, RecipeStateInactive:
		return true
	default:
		return false
	}
}

// Score is an inclusive rating from 1 to 5.
type Score int

const (
	MinScore Score = 1
	MaxScore Score = 5
)

func (s Score) valid() bool {
	return s >= MinScore && s <= MaxScore
}
