package domain

// RecipeID uniquely identifies a recipe independently of its persistence representation.
type RecipeID string

// IngredientID uniquely identifies an ingredient independently of its persistence representation.
type IngredientID string

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
	DifficultyHard   Difficulty = "hard"
)

func (d Difficulty) valid() bool {
	switch d {
	case DifficultyEasy, DifficultyMedium, DifficultyHard:
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
