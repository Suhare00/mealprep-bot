package telegram

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	mealplan "github.com/Suhare00/mealprep-bot/internal/plan"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

const (
	defaultPageSize  = 9
	shoppingPageSize = 12
	maxCallbackBytes = 64
	maxMessageRunes  = 1800
)

func mainMenu() OutgoingMessage {
	return OutgoingMessage{
		Text: "Добро пожаловать в MealPrep Bot!\n\nЗдесь можно посмотреть каталог рецептов и составить план питания на три дня.",
		ReplyMarkup: keyboard(
			buttonRow("Каталог рецептов", "c:0"),
			buttonRow("План питания", "p"),
		),
	}
}

func helpMessage() OutgoingMessage {
	return OutgoingMessage{
		Text:        "Доступные команды:\n/start — главное меню\n/recipes — каталог рецептов\n/plan — план питания на три дня\n/help — эта справка",
		ReplyMarkup: keyboard(buttonRow("Главное меню", "m")),
	}
}

func emptyPlanMessage() OutgoingMessage {
	return OutgoingMessage{
		Text: "Плана пока нет. Сгенерируйте три блюда для двух человек на три дня: завтрак, обед и ужин. " +
			"Каждое блюдо готовится один раз на 6 порций и повторяется все три дня.",
		ReplyMarkup: keyboard(buttonRow("Сгенерировать план", "pg:0"), buttonRow("Главное меню", "m")),
	}
}

func planMessage(snapshot mealplan.Snapshot, prefix string) OutgoingMessage {
	text := fmt.Sprintf("%sПлан питания на 3 дня\nДля 2 человек · 18 порций\n\n"+
		"Завтрак: %s — 6 порций\nОбед: %s — 6 порций\nУжин: %s — 6 порций\n\n"+
		"Эти три блюда повторяются все три дня по задумке batch-cooking. Сроки хранения — данные каталога, а не персональная гарантия безопасности.",
		prefix, snapshot.Breakfast.Recipe.Name, snapshot.Lunch.Recipe.Name, snapshot.Dinner.Recipe.Name)
	revision := snapshot.Revision
	return OutgoingMessage{Text: truncateText(text), ReplyMarkup: keyboard(
		buttonRow("Заменить завтрак", fmt.Sprintf("px:b:%d", revision)),
		buttonRow("Заменить обед", fmt.Sprintf("px:l:%d", revision)),
		buttonRow("Заменить ужин", fmt.Sprintf("px:d:%d", revision)),
		buttonRow("Список покупок", fmt.Sprintf("sm:%d", revision)),
		buttonRow("Сгенерировать заново", fmt.Sprintf("pg:%d", revision)),
		buttonRow("Главное меню", "m"),
	)}
}

func shoppingMenuMessage(snapshot mealplan.Snapshot) OutgoingMessage {
	revision := snapshot.Revision
	return OutgoingMessage{
		Text: truncateText(fmt.Sprintf("Список покупок\n\n"+
			"Общий список объединяет одинаковые ингредиенты всего плана.\n\n"+
			"По блюдам:\n• Завтрак — %s\n• Обед — %s\n• Ужин — %s",
			snapshot.Breakfast.Recipe.Name, snapshot.Lunch.Recipe.Name, snapshot.Dinner.Recipe.Name)),
		ReplyMarkup: keyboard(
			buttonRow("Общий список", fmt.Sprintf("sc:%d:0", revision)),
			buttonRow("Завтрак", fmt.Sprintf("sr:b:%d:0", revision)),
			buttonRow("Обед", fmt.Sprintf("sr:l:%d:0", revision)),
			buttonRow("Ужин", fmt.Sprintf("sr:d:%d:0", revision)),
			buttonRow("← Назад к плану", "p"),
			buttonRow("Главное меню", "m"),
		),
	}
}

func consolidatedShoppingListMessage(snapshot mealplan.Snapshot, items []mealplan.ShoppingItem, requestedPage int) OutgoingMessage {
	return shoppingListPage(
		"Общий список покупок",
		items,
		requestedPage,
		func(page int) string { return fmt.Sprintf("sc:%d:%d", snapshot.Revision, page) },
		fmt.Sprintf("sm:%d", snapshot.Revision),
	)
}

func recipeShoppingListMessage(snapshot mealplan.Snapshot, mealType domain.MealType, items []mealplan.ShoppingItem, requestedPage int) OutgoingMessage {
	entry := snapshot.Entry(mealType)
	title := fmt.Sprintf("%s: %s\n%d порций", mealTypeTitle(mealType), entry.Recipe.Name, entry.Recipe.Servings)
	return shoppingListPage(
		title,
		items,
		requestedPage,
		func(page int) string {
			return fmt.Sprintf("sr:%s:%d:%d", mealTypeCode(mealType), snapshot.Revision, page)
		},
		fmt.Sprintf("sm:%d", snapshot.Revision),
	)
}

func shoppingListPage(title string, items []mealplan.ShoppingItem, requestedPage int, pageCallback func(int) string, backCallback string) OutgoingMessage {
	pageCount := max(1, (len(items)+shoppingPageSize-1)/shoppingPageSize)
	page := min(max(requestedPage, 0), pageCount-1)
	start := min(page*shoppingPageSize, len(items))
	end := min(start+shoppingPageSize, len(items))
	var text strings.Builder
	fmt.Fprintf(&text, "%s\nСтраница %d из %d\n\n", title, page+1, pageCount)
	for _, item := range items[start:end] {
		fmt.Fprintf(&text, "• %s — %s %s\n", shortIngredientName(item.Name), formatQuantity(item.Quantity), item.Unit)
	}
	rows := make([][]InlineKeyboardButton, 0, 3)
	var navigation []InlineKeyboardButton
	if page > 0 {
		navigation = append(navigation, InlineKeyboardButton{Text: "← Назад", CallbackData: pageCallback(page - 1)})
	}
	if page+1 < pageCount {
		navigation = append(navigation, InlineKeyboardButton{Text: "Вперёд →", CallbackData: pageCallback(page + 1)})
	}
	if len(navigation) > 0 {
		rows = append(rows, navigation)
	}
	rows = append(rows, buttonRow("← К спискам", backCallback), buttonRow("← Назад к плану", "p"), buttonRow("Главное меню", "m"))
	return OutgoingMessage{
		Text:        truncateText(text.String()),
		ReplyMarkup: InlineKeyboardMarkup{InlineKeyboard: rows},
	}
}

func mealTypeTitle(value domain.MealType) string {
	switch value {
	case domain.MealTypeBreakfast:
		return "Завтрак"
	case domain.MealTypeLunch:
		return "Обед"
	case domain.MealTypeDinner:
		return "Ужин"
	default:
		return string(value)
	}
}

func mealTypeCode(value domain.MealType) string {
	switch value {
	case domain.MealTypeBreakfast:
		return "b"
	case domain.MealTypeLunch:
		return "l"
	case domain.MealTypeDinner:
		return "d"
	default:
		return ""
	}
}

func shortIngredientName(value string) string {
	const maxRunes = 80
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes-1]) + "…"
}

func unavailablePlanMessage(revision int64) OutgoingMessage {
	return OutgoingMessage{
		Text:        "Один из рецептов сохранённого плана больше недоступен или не подходит для трёх дней. Сгенерируйте план заново.",
		ReplyMarkup: keyboard(buttonRow("Сгенерировать заново", fmt.Sprintf("pg:%d", revision)), buttonRow("Главное меню", "m")),
	}
}

func insufficientPlanMessage(mealType domain.MealType) OutgoingMessage {
	return OutgoingMessage{
		Text:        fmt.Sprintf("Не удалось составить полный план: недостаточно подходящих рецептов для приёма пищи «%s».", mealTypeText(mealType)),
		ReplyMarkup: keyboard(buttonRow("Главное меню", "m")),
	}
}

func insufficientDistinctPlanMessage() OutgoingMessage {
	return OutgoingMessage{
		Text:        "Не удалось составить полный план из трёх разных подходящих рецептов.",
		ReplyMarkup: keyboard(buttonRow("Главное меню", "m")),
	}
}

func planProcessingErrorMessage() OutgoingMessage {
	return OutgoingMessage{Text: "Не удалось обновить план. Попробуйте ещё раз позже.", ReplyMarkup: keyboard(buttonRow("План питания", "p"), buttonRow("Главное меню", "m"))}
}

func mealTypeText(value domain.MealType) string {
	switch value {
	case domain.MealTypeBreakfast:
		return "завтрак"
	case domain.MealTypeLunch:
		return "обед"
	case domain.MealTypeDinner:
		return "ужин"
	default:
		return string(value)
	}
}

func formatQuantity(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
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
	kind     string
	page     int
	id       domain.RecipeID
	revision int64
	mealType domain.MealType
}

func parseCallback(data string) (callbackAction, error) {
	if data == "m" || data == "p" {
		return callbackAction{kind: data}, nil
	}
	if data == "" || len(data) > maxCallbackBytes {
		return callbackAction{}, errInvalidCallback
	}
	parts := strings.Split(data, ":")
	if parts[0] == "pg" && len(parts) == 2 {
		revision, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil && revision >= 0 {
			return callbackAction{kind: "pg", revision: revision}, nil
		}
		return callbackAction{}, errInvalidCallback
	}
	if parts[0] == "sm" && len(parts) == 2 {
		revision, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil && revision > 0 {
			return callbackAction{kind: "sm", revision: revision}, nil
		}
		return callbackAction{}, errInvalidCallback
	}
	if (parts[0] == "sc" || parts[0] == "ps") && (len(parts) == 2 || len(parts) == 3) {
		revision, revisionErr := strconv.ParseInt(parts[1], 10, 64)
		page := 0
		var pageErr error
		if len(parts) == 3 {
			page, pageErr = strconv.Atoi(parts[2])
		}
		if revisionErr == nil && revision > 0 && pageErr == nil && page >= 0 {
			return callbackAction{kind: "sc", revision: revision, page: page}, nil
		}
		return callbackAction{}, errInvalidCallback
	}
	if parts[0] == "sr" && len(parts) == 4 {
		mealTypes := map[string]domain.MealType{"b": domain.MealTypeBreakfast, "l": domain.MealTypeLunch, "d": domain.MealTypeDinner}
		mealType, mealOK := mealTypes[parts[1]]
		revision, revisionErr := strconv.ParseInt(parts[2], 10, 64)
		page, pageErr := strconv.Atoi(parts[3])
		if mealOK && revisionErr == nil && revision > 0 && pageErr == nil && page >= 0 {
			return callbackAction{kind: "sr", revision: revision, mealType: mealType, page: page}, nil
		}
		return callbackAction{}, errInvalidCallback
	}
	if parts[0] == "px" && len(parts) == 3 {
		revision, err := strconv.ParseInt(parts[2], 10, 64)
		mealTypes := map[string]domain.MealType{"b": domain.MealTypeBreakfast, "l": domain.MealTypeLunch, "d": domain.MealTypeDinner}
		mealType, ok := mealTypes[parts[1]]
		if err == nil && revision > 0 && ok {
			return callbackAction{kind: "px", revision: revision, mealType: mealType}, nil
		}
		return callbackAction{}, errInvalidCallback
	}
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
