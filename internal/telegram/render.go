package telegram

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

const (
	defaultPageSize  = 9
	maxCallbackBytes = 64
	maxMessageRunes  = 1800
)

func mainMenu() OutgoingMessage {
	return OutgoingMessage{
		Text: "Добро пожаловать в MealPrep Bot!\n\nЗдесь можно посмотреть каталог рецептов. Планирование питания появится позже.",
		ReplyMarkup: keyboard(
			buttonRow("Каталог рецептов", "c:0"),
			buttonRow("План питания (скоро)", "p"),
		),
	}
}

func helpMessage() OutgoingMessage {
	return OutgoingMessage{
		Text:        "Доступные команды:\n/start — главное меню\n/recipes — каталог рецептов\n/help — эта справка",
		ReplyMarkup: keyboard(buttonRow("Главное меню", "m")),
	}
}

func planPlaceholder() OutgoingMessage {
	return OutgoingMessage{
		Text:        "План питания пока в разработке. Сейчас доступен каталог рецептов.",
		ReplyMarkup: keyboard(buttonRow("Каталог рецептов", "c:0"), buttonRow("Главное меню", "m")),
	}
}

func cataloguePage(entries []recipecatalogue.CatalogueEntry, requestedPage int) (OutgoingMessage, error) {
	if len(entries) == 0 {
		return OutgoingMessage{
			Text:        "Каталог пока пуст. Добавьте активные рецепты и попробуйте снова.",
			ReplyMarkup: keyboard(buttonRow("Главное меню", "m")),
		}, nil
	}

	pages := (len(entries) + defaultPageSize - 1) / defaultPageSize
	page := requestedPage
	if page < 0 {
		page = 0
	}
	if page >= pages {
		page = pages - 1
	}
	start := page * defaultPageSize
	end := min(start+defaultPageSize, len(entries))

	rows := make([][]InlineKeyboardButton, 0, end-start+2)
	for _, entry := range entries[start:end] {
		data := fmt.Sprintf("r:%d:%s", page, entry.Recipe.ID)
		if len(data) > maxCallbackBytes {
			return OutgoingMessage{}, fmt.Errorf("recipe %q cannot fit Telegram callback data", entry.Recipe.ID)
		}
		rows = append(rows, buttonRow(shortButtonText(entry.Recipe.Name), data))
	}
	var navigation []InlineKeyboardButton
	if page > 0 {
		navigation = append(navigation, InlineKeyboardButton{Text: "← Назад", CallbackData: fmt.Sprintf("c:%d", page-1)})
	}
	if page+1 < pages {
		navigation = append(navigation, InlineKeyboardButton{Text: "Вперёд →", CallbackData: fmt.Sprintf("c:%d", page+1)})
	}
	if len(navigation) > 0 {
		rows = append(rows, navigation)
	}
	rows = append(rows, buttonRow("Главное меню", "m"))

	return OutgoingMessage{
		Text:        fmt.Sprintf("Каталог рецептов\nСтраница %d из %d", page+1, pages),
		ReplyMarkup: InlineKeyboardMarkup{InlineKeyboard: rows},
	}, nil
}

func recipeDetail(entry recipecatalogue.CatalogueEntry, page int) OutgoingMessage {
	recipe := entry.Recipe
	ingredientNames := make(map[domain.IngredientID]string, len(entry.Ingredients))
	for _, ingredient := range entry.Ingredients {
		ingredientNames[ingredient.ID] = ingredient.Name
	}

	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\nПриёмы пищи: %s\nПорций: %d\nКатегория: %s\n",
		recipe.Name, mealTypesText(recipe.MealTypes), recipe.Servings, recipe.Category)
	fmt.Fprintf(&text, "Подготовка: %s\nПриготовление: %s\nХранение: %s\n",
		durationText(recipe.PreparationDuration), durationText(recipe.CookingDuration), daysText(recipe.StorageDays))
	fmt.Fprintf(&text, "Сложность: %s\nБюджет: %s\nЗаморозка: %s\n",
		difficultyText(recipe.Difficulty), budgetText(recipe.BudgetLevel), yesNo(recipe.FreezerFriendly))
	fmt.Fprintf(&text, "Оценка разогрева: %d/5\nДля готовки партиями: %d/5\n",
		recipe.ReheatScore, recipe.BatchCookingScore)
	if len(recipe.Tags) > 0 {
		fmt.Fprintf(&text, "Теги: %s\n", strings.Join(recipe.Tags, ", "))
	}
	text.WriteString("\nИнгредиенты:\n")
	for _, relation := range recipe.Ingredients {
		name := ingredientNames[relation.IngredientID]
		if name == "" {
			name = string(relation.IngredientID)
		}
		fmt.Fprintf(&text, "• %s — %s %s\n", name,
			strconv.FormatFloat(relation.Quantity, 'f', -1, 64), relation.Unit)
	}

	return OutgoingMessage{
		Text: truncateText(text.String()),
		ReplyMarkup: keyboard(
			buttonRow("← К каталогу", fmt.Sprintf("c:%d", max(page, 0))),
			buttonRow("Главное меню", "m"),
		),
	}
}

func unavailableMessage(page int) OutgoingMessage {
	return OutgoingMessage{
		Text: "Рецепт больше недоступен. Возможно, он был отключён.",
		ReplyMarkup: keyboard(
			buttonRow("← К каталогу", fmt.Sprintf("c:%d", max(page, 0))),
			buttonRow("Главное меню", "m"),
		),
	}
}

func invalidActionMessage() OutgoingMessage {
	return OutgoingMessage{
		Text:        "Эта кнопка устарела или повреждена. Откройте меню заново.",
		ReplyMarkup: keyboard(buttonRow("Главное меню", "m")),
	}
}

func processingErrorMessage() OutgoingMessage {
	return OutgoingMessage{
		Text:        "Не удалось загрузить каталог. Попробуйте ещё раз позже.",
		ReplyMarkup: keyboard(buttonRow("Главное меню", "m")),
	}
}

func keyboard(rows ...[]InlineKeyboardButton) InlineKeyboardMarkup {
	return InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buttonRow(text, data string) []InlineKeyboardButton {
	return []InlineKeyboardButton{{Text: text, CallbackData: data}}
}

func shortButtonText(value string) string {
	const maxRunes = 48
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes-1]) + "…"
}

func truncateText(value string) string {
	if utf8.RuneCountInString(value) <= maxMessageRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxMessageRunes-1]) + "…"
}

func mealTypesText(values []domain.MealType) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		switch value {
		case domain.MealTypeBreakfast:
			result = append(result, "завтрак")
		case domain.MealTypeLunch:
			result = append(result, "обед")
		case domain.MealTypeDinner:
			result = append(result, "ужин")
		default:
			result = append(result, string(value))
		}
	}
	return strings.Join(result, ", ")
}

func durationText(value time.Duration) string {
	if value == 0 {
		return "0 мин"
	}
	hours := int(value / time.Hour)
	minutes := int(value % time.Hour / time.Minute)
	seconds := int(value % time.Minute / time.Second)
	parts := make([]string, 0, 3)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d ч", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d мин", minutes))
	}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%d сек", seconds))
	}
	return strings.Join(parts, " ")
}

func daysText(days int) string {
	return fmt.Sprintf("%d дн.", days)
}

func difficultyText(value domain.Difficulty) string {
	switch value {
	case domain.DifficultyEasy:
		return "легко"
	case domain.DifficultyMedium:
		return "средне"
	default:
		return string(value)
	}
}

func budgetText(value domain.BudgetLevel) string {
	switch value {
	case domain.BudgetLevelLow:
		return "низкий"
	case domain.BudgetLevelMedium:
		return "средний"
	case domain.BudgetLevelHigh:
		return "высокий"
	default:
		return string(value)
	}
}

func yesNo(value bool) string {
	if value {
		return "да"
	}
	return "нет"
}

var errInvalidCallback = errors.New("invalid callback data")

type callbackAction struct {
	kind string
	page int
	id   domain.RecipeID
}

func parseCallback(data string) (callbackAction, error) {
	if data == "m" || data == "p" {
		return callbackAction{kind: data}, nil
	}
	if data == "" || len(data) > maxCallbackBytes {
		return callbackAction{}, errInvalidCallback
	}
	parts := strings.Split(data, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return callbackAction{}, errInvalidCallback
	}
	if parts[0] != "c" && parts[0] != "r" {
		return callbackAction{}, errInvalidCallback
	}
	page, err := strconv.Atoi(parts[1])
	if err != nil || page < 0 {
		return callbackAction{}, errInvalidCallback
	}
	if parts[0] == "c" && len(parts) == 2 {
		return callbackAction{kind: "c", page: page}, nil
	}
	if parts[0] == "r" && len(parts) == 3 && strings.TrimSpace(parts[2]) != "" && !strings.Contains(parts[2], ":") {
		return callbackAction{kind: "r", page: page, id: domain.RecipeID(parts[2])}, nil
	}
	return callbackAction{}, errInvalidCallback
}
