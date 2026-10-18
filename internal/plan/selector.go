package plan

import (
	"math/rand/v2"
	"sort"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

const variedCandidateLimit = 5

type randomChooser struct{}

func (randomChooser) IntN(n int) int { return rand.IntN(n) }

type candidate struct {
	entry recipecatalogue.CatalogueEntry
	score int
}

type combination struct {
	ids         IDs
	score       int
	recentCount int
}

func selectPlan(entries []recipecatalogue.CatalogueEntry, recent []domain.RecipeID, chooser Chooser) (IDs, error) {
	byMeal := map[domain.MealType][]candidate{}
	for _, mealType := range []domain.MealType{domain.MealTypeBreakfast, domain.MealTypeLunch, domain.MealTypeDinner} {
		byMeal[mealType] = eligible(entries, mealType)
		if len(byMeal[mealType]) == 0 {
			return IDs{}, &EligibilityError{MealType: mealType}
		}
	}

	recentSet := make(map[domain.RecipeID]struct{}, len(recent))
	for _, id := range recent {
		recentSet[id] = struct{}{}
	}
	var combinations []combination
	for _, breakfast := range byMeal[domain.MealTypeBreakfast] {
		for _, lunch := range byMeal[domain.MealTypeLunch] {
			if lunch.entry.Recipe.ID == breakfast.entry.Recipe.ID {
				continue
			}
			for _, dinner := range byMeal[domain.MealTypeDinner] {
				if dinner.entry.Recipe.ID == breakfast.entry.Recipe.ID || dinner.entry.Recipe.ID == lunch.entry.Recipe.ID {
					continue
				}
				ids := IDs{Breakfast: breakfast.entry.Recipe.ID, Lunch: lunch.entry.Recipe.ID, Dinner: dinner.entry.Recipe.ID}
				combinations = append(combinations, combination{
					ids: ids, score: breakfast.score + lunch.score + dinner.score,
					recentCount: countRecent(ids, recentSet),
				})
			}
		}
	}
	if len(combinations) == 0 {
		return IDs{}, ErrInsufficientRecipes
	}

	minRecent := combinations[0].recentCount
	for _, item := range combinations[1:] {
		minRecent = min(minRecent, item.recentCount)
	}
	filtered := combinations[:0]
	for _, item := range combinations {
		if item.recentCount == minRecent {
			filtered = append(filtered, item)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].score != filtered[j].score {
			return filtered[i].score > filtered[j].score
		}
		return idsKey(filtered[i].ids) < idsKey(filtered[j].ids)
	})
	limit := min(variedCandidateLimit, len(filtered))
	return filtered[chooseIndex(chooser, limit)].ids, nil
}

func selectReplacement(entries []recipecatalogue.CatalogueEntry, current IDs, mealType domain.MealType, recent []domain.RecipeID, chooser Chooser) (domain.RecipeID, error) {
	if mealType != domain.MealTypeBreakfast && mealType != domain.MealTypeLunch && mealType != domain.MealTypeDinner {
		return "", &EligibilityError{MealType: mealType}
	}
	excluded := make(map[domain.RecipeID]struct{}, 3)
	for _, id := range current.Slice() {
		excluded[id] = struct{}{}
	}
	candidates := eligible(entries, mealType)
	available := candidates[:0]
	for _, item := range candidates {
		if _, skip := excluded[item.entry.Recipe.ID]; !skip {
			available = append(available, item)
		}
	}
	if len(available) == 0 {
		return "", ErrNoAlternative
	}

	recentSet := make(map[domain.RecipeID]struct{}, len(recent))
	for _, id := range recent {
		recentSet[id] = struct{}{}
	}
	nonRecent := make([]candidate, 0, len(available))
	for _, item := range available {
		if _, seen := recentSet[item.entry.Recipe.ID]; !seen {
			nonRecent = append(nonRecent, item)
		}
	}
	if len(nonRecent) > 0 {
		available = nonRecent
	}
	sortCandidates(available)
	limit := min(variedCandidateLimit, len(available))
	return available[chooseIndex(chooser, limit)].entry.Recipe.ID, nil
}

func eligible(entries []recipecatalogue.CatalogueEntry, mealType domain.MealType) []candidate {
	result := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		recipe := entry.Recipe
		if recipe.State != domain.RecipeStateActive || recipe.StorageDays < Days || recipe.Servings != PortionsPerRecipe {
			continue
		}
		if recipe.Validate() != nil || !supports(recipe.MealTypes, mealType) {
			continue
		}
		result = append(result, candidate{entry: entry, score: int(recipe.BatchCookingScore)*2 + int(recipe.ReheatScore)})
	}
	sortCandidates(result)
	return result
}

func sortCandidates(values []candidate) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].score != values[j].score {
			return values[i].score > values[j].score
		}
		return values[i].entry.Recipe.ID < values[j].entry.Recipe.ID
	})
}

func supports(values []domain.MealType, target domain.MealType) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func countRecent(ids IDs, recent map[domain.RecipeID]struct{}) int {
	count := 0
	for _, id := range ids.Slice() {
		if _, ok := recent[id]; ok {
			count++
		}
	}
	return count
}

func idsKey(ids IDs) string {
	return string(ids.Breakfast) + "\x00" + string(ids.Lunch) + "\x00" + string(ids.Dinner)
}

func chooseIndex(chooser Chooser, n int) int {
	if n <= 1 {
		return 0
	}
	index := chooser.IntN(n)
	if index < 0 || index >= n {
		return 0
	}
	return index
}
