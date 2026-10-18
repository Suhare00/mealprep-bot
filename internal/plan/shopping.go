package plan

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

func BuildShoppingList(snapshot Snapshot) ([]ShoppingItem, error) {
	type key struct {
		id   domain.IngredientID
		unit string
	}
	items := make(map[key]ShoppingItem)
	for _, entry := range []recipecatalogue.CatalogueEntry{snapshot.Breakfast, snapshot.Lunch, snapshot.Dinner} {
		names := make(map[domain.IngredientID]string, len(entry.Ingredients))
		for _, ingredient := range entry.Ingredients {
			names[ingredient.ID] = ingredient.Name
		}
		for _, relation := range entry.Recipe.Ingredients {
			name := strings.TrimSpace(names[relation.IngredientID])
			unit := strings.TrimSpace(relation.Unit)
			if relation.Quantity <= 0 || name == "" || unit == "" {
				return nil, fmt.Errorf("invalid shopping ingredient %q", relation.IngredientID)
			}
			itemKey := key{id: relation.IngredientID, unit: unit}
			item := items[itemKey]
			if item.Name != "" && item.Name != name {
				return nil, errors.New("conflicting ingredient names in plan")
			}
			item.IngredientID = relation.IngredientID
			item.Name = name
			item.Unit = unit
			item.Quantity += relation.Quantity
			items[itemKey] = item
		}
	}
	result := make([]ShoppingItem, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := strings.ToLower(result[i].Name), strings.ToLower(result[j].Name)
		if left != right {
			return left < right
		}
		if result[i].IngredientID != result[j].IngredientID {
			return result[i].IngredientID < result[j].IngredientID
		}
		return result[i].Unit < result[j].Unit
	})
	return result, nil
}
