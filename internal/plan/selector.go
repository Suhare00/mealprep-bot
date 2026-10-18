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

func selectPlan(entries []recipecatalogue.CatalogueEntry, recent []domain.RecipeID, chooser Chooser) (IDs, error) {
	mealTypes := []domain.MealType{domain.MealTypeBreakfast, domain.MealTypeLunch, domain.MealTypeDinner}
	byMeal := map[domain.MealType][]candidate{}
	for _, mealType := range mealTypes {
		byMeal[mealType] = eligible(entries, mealType)
		if len(byMeal[mealType]) == 0 {
			return IDs{}, &EligibilityError{MealType: mealType}
		}
	}

	recentSet := make(map[domain.RecipeID]struct{}, len(recent))
	for _, id := range recent {
		recentSet[id] = struct{}{}
	}

	selected := make(map[domain.RecipeID]struct{}, len(mealTypes))
	minimumRecent := -1
	for target := 0; target <= len(mealTypes); target++ {
		if canCompletePlan(byMeal, mealTypes, 0, selected, recentSet, 0, target) {
			minimumRecent = target
			break
		}
	}
	if minimumRecent < 0 {
		return IDs{}, ErrInsufficientRecipes
	}

	var ids IDs
	recentUsed := 0
	for index, mealType := range mealTypes {
		options := make([]candidate, 0, variedCandidateLimit)
		for _, item := range byMeal[mealType] {
			id := item.entry.Recipe.ID
			if _, duplicate := selected[id]; duplicate {
				continue
			}
			nextRecent := recentUsed
			if _, seen := recentSet[id]; seen {
				nextRecent++
			}
			selected[id] = struct{}{}
			possible := canCompletePlan(byMeal, mealTypes, index+1, selected, recentSet, nextRecent, minimumRecent)
			delete(selected, id)
			if possible {
				options = append(options, item)
			}
		}
		if len(options) == 0 {
			return IDs{}, ErrInsufficientRecipes
		}
		limit := min(variedCandidateLimit, len(options))
		chosen := options[chooseIndex(chooser, limit)].entry.Recipe.ID
		ids.Set(mealType, chosen)
		selected[chosen] = struct{}{}
		if _, seen := recentSet[chosen]; seen {
			recentUsed++
		}
	}
	return ids, nil
}

func canCompletePlan(
	byMeal map[domain.MealType][]candidate,
	mealTypes []domain.MealType,
	index int,
	selected map[domain.RecipeID]struct{},
	recent map[domain.RecipeID]struct{},
	recentUsed int,
	targetRecent int,
) bool {
	if recentUsed > targetRecent || recentUsed+len(mealTypes)-index < targetRecent {
		return false
	}
	if index == len(mealTypes) {
		return recentUsed == targetRecent
	}
	for _, item := range byMeal[mealTypes[index]] {
		id := item.entry.Recipe.ID
		if _, duplicate := selected[id]; duplicate {
			continue
		}
		nextRecent := recentUsed
		if _, seen := recent[id]; seen {
			nextRecent++
		}
		selected[id] = struct{}{}
		possible := canCompletePlan(byMeal, mealTypes, index+1, selected, recent, nextRecent, targetRecent)
		delete(selected, id)
		if possible {
			return true
		}
	}
	return false
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
