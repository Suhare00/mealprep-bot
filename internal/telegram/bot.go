package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	mealplan "github.com/Suhare00/mealprep-bot/internal/plan"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

const (
	longPollSeconds = 30
	initialBackoff  = time.Second
	maximumBackoff  = 30 * time.Second
)

// Catalogue is the application read use case required by Telegram.
type Catalogue interface {
	ListActive(ctx context.Context) ([]recipecatalogue.CatalogueEntry, error)
	GetActiveByID(ctx context.Context, id domain.RecipeID) (recipecatalogue.CatalogueEntry, error)
}

// Planner is the application use case required by the Telegram adapter.
type Planner interface {
	Current(ctx context.Context, chatID int64) (mealplan.Snapshot, error)
	Generate(ctx context.Context, chatID, expectedRevision int64) (mealplan.Snapshot, error)
	Replace(ctx context.Context, chatID int64, mealType domain.MealType, expectedRevision int64) (mealplan.Snapshot, error)
	ShoppingList(ctx context.Context, chatID int64) (mealplan.Snapshot, []mealplan.ShoppingItem, error)
	RecipeShoppingList(ctx context.Context, chatID int64, mealType domain.MealType) (mealplan.Snapshot, []mealplan.ShoppingItem, error)
}

// Bot dispatches Telegram updates to the catalogue use case.
type Bot struct {
	client    Client
	catalogue Catalogue
	planner   Planner
	logger    *slog.Logger
	wait      func(context.Context, time.Duration) error
}

// NewBot constructs the Telegram delivery adapter.
func NewBot(client Client, catalogue Catalogue, planner Planner, logger *slog.Logger) *Bot {
	return &Bot{client: client, catalogue: catalogue, planner: planner, logger: logger, wait: wait}
}

// Run verifies long-poll compatibility and consumes updates until cancellation.
// An update is acknowledged to Telegram only by the next getUpdates offset, after
// every earlier update in the returned batch has been handled successfully.
func (b *Bot) Run(ctx context.Context) error {
	webhook, err := b.client.GetWebhookInfo(ctx)
	if err != nil {
		return fmt.Errorf("check Telegram webhook: %w", err)
	}
	if webhook.URL != "" {
		return errors.New("telegram webhook is active; remove it before using long polling")
	}

	var offset int64
	backoff := initialBackoff
	for {
		updates, err := b.client.GetUpdates(ctx, offset, longPollSeconds)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if isPermanentPollingError(err) {
				return fmt.Errorf("telegram long polling conflict: another polling consumer may be using this bot token: %w", err)
			}
			b.logger.Warn("Telegram polling failed; retrying", "error", err, "backoff", backoff)
			if err := b.wait(ctx, backoff); err != nil {
				return nil
			}
			backoff = min(backoff*2, maximumBackoff)
			continue
		}
		backoff = initialBackoff

		for _, update := range updates {
			if err := b.HandleUpdate(ctx, update); err != nil {
				return fmt.Errorf("handle Telegram update %d: %w", update.UpdateID, err)
			}
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
		}
	}
}

// HandleUpdate routes one update. Unsupported and non-private updates are ignored safely.
func (b *Bot) HandleUpdate(ctx context.Context, update Update) error {
	if update.CallbackQuery != nil {
		return b.handleCallback(ctx, *update.CallbackQuery)
	}
	if update.Message != nil {
		return b.handleMessage(ctx, *update.Message)
	}
	return nil
}

func (b *Bot) handleMessage(ctx context.Context, message Message) error {
	if message.Chat.Type != "private" {
		return nil
	}

	command := commandName(message.Text)
	var response OutgoingMessage
	var err error
	switch command {
	case "/start":
		response = mainMenu()
	case "/help":
		response = helpMessage()
	case "/recipes":
		response, err = b.loadCatalogue(ctx, 0)
	case "/plan":
		response, err = b.loadPlan(ctx, message.Chat.ID)
	default:
		response = OutgoingMessage{
			Text:        "Неизвестная команда. Используйте /help или откройте главное меню.",
			ReplyMarkup: keyboard(buttonRow("Главное меню", "m")),
		}
	}
	if err != nil {
		return errors.Join(err, b.client.SendMessage(ctx, message.Chat.ID, processingErrorMessage()))
	}
	if err := b.client.SendMessage(ctx, message.Chat.ID, response); err != nil {
		return fmt.Errorf("send command response: %w", err)
	}
	return nil
}

func (b *Bot) handleCallback(ctx context.Context, callback CallbackQuery) error {
	if err := b.client.AnswerCallbackQuery(ctx, callback.ID); err != nil {
		if isExpiredCallbackAcknowledgement(err) {
			b.logger.Debug("Telegram callback acknowledgement expired")
		} else {
			b.logger.Warn("Telegram callback acknowledgement failed; continuing update processing", "error", err)
		}
	}

	if callback.Message != nil && callback.Message.Chat.Type == "private" {
		return b.routeCallback(ctx, callback.Data, *callback.Message)
	}
	return nil
}

func (b *Bot) routeCallback(ctx context.Context, data string, message Message) error {
	action, err := parseCallback(data)
	if err != nil {
		return b.edit(ctx, message, invalidActionMessage())
	}

	var response OutgoingMessage
	switch action.kind {
	case "m":
		response = mainMenu()
	case "p":
		response, err = b.loadPlan(ctx, message.Chat.ID)
	case "pg", "px", "sm", "sc", "sr":
		return b.handlePlanAction(ctx, action, message)
	case "c":
		response, err = b.loadCatalogue(ctx, action.page)
	case "r":
		var entry recipecatalogue.CatalogueEntry
		entry, err = b.catalogue.GetActiveByID(ctx, action.id)
		if errors.Is(err, recipecatalogue.ErrUnavailable) {
			return b.edit(ctx, message, unavailableMessage(action.page))
		}
		if err == nil {
			response = recipeDetail(entry, action.page)
		}
	}
	if err != nil {
		return errors.Join(err, b.edit(ctx, message, processingErrorMessage()))
	}
	return b.edit(ctx, message, response)
}

func (b *Bot) loadPlan(ctx context.Context, chatID int64) (OutgoingMessage, error) {
	snapshot, err := b.planner.Current(ctx, chatID)
	switch {
	case err == nil:
		return planMessage(snapshot, ""), nil
	case errors.Is(err, mealplan.ErrNotFound):
		return emptyPlanMessage(), nil
	case errors.Is(err, mealplan.ErrPlanUnavailable):
		return unavailablePlanMessage(snapshot.Revision), nil
	default:
		return OutgoingMessage{}, fmt.Errorf("load current meal plan: %w", err)
	}
}

func (b *Bot) handlePlanAction(ctx context.Context, action callbackAction, message Message) error {
	var snapshot mealplan.Snapshot
	var items []mealplan.ShoppingItem
	var err error
	switch action.kind {
	case "pg":
		snapshot, err = b.planner.Generate(ctx, message.Chat.ID, action.revision)
	case "px":
		snapshot, err = b.planner.Replace(ctx, message.Chat.ID, action.mealType, action.revision)
	case "sm":
		snapshot, err = b.planner.Current(ctx, message.Chat.ID)
		if err == nil && snapshot.Revision != action.revision {
			return b.edit(ctx, message, staleShoppingPlanMessage(snapshot))
		}
		if err == nil {
			return b.edit(ctx, message, shoppingMenuMessage(snapshot))
		}
	case "sc":
		snapshot, items, err = b.planner.ShoppingList(ctx, message.Chat.ID)
		if err == nil && snapshot.Revision != action.revision {
			return b.edit(ctx, message, staleShoppingPlanMessage(snapshot))
		}
		if err == nil {
			return b.edit(ctx, message, consolidatedShoppingListMessage(snapshot, items, action.page))
		}
	case "sr":
		snapshot, items, err = b.planner.RecipeShoppingList(ctx, message.Chat.ID, action.mealType)
		if err == nil && snapshot.Revision != action.revision {
			return b.edit(ctx, message, staleShoppingPlanMessage(snapshot))
		}
		if err == nil {
			return b.edit(ctx, message, recipeShoppingListMessage(snapshot, action.mealType, items, action.page))
		}
	}
	if err == nil {
		return b.edit(ctx, message, planMessage(snapshot, ""))
	}

	switch {
	case errors.Is(err, mealplan.ErrStaleRevision):
		latest, loadErr := b.loadPlan(ctx, message.Chat.ID)
		if loadErr != nil {
			return errors.Join(err, loadErr)
		}
		latest.Text = "Эта кнопка относится к старой версии плана. Показан актуальный план.\n\n" + latest.Text
		return b.edit(ctx, message, latest)
	case errors.Is(err, mealplan.ErrNoAlternative):
		latest, loadErr := b.loadPlan(ctx, message.Chat.ID)
		if loadErr != nil {
			return errors.Join(err, loadErr)
		}
		latest.Text = "Для этого приёма пищи сейчас нет другого подходящего рецепта.\n\n" + latest.Text
		return b.edit(ctx, message, latest)
	case errors.Is(err, mealplan.ErrPlanUnavailable):
		return b.edit(ctx, message, unavailablePlanMessage(action.revision))
	case errors.Is(err, mealplan.ErrInsufficientRecipes):
		return b.edit(ctx, message, insufficientDistinctPlanMessage())
	default:
		var eligibilityErr *mealplan.EligibilityError
		if errors.As(err, &eligibilityErr) {
			return b.edit(ctx, message, insufficientPlanMessage(eligibilityErr.MealType))
		}
		return errors.Join(err, b.edit(ctx, message, planProcessingErrorMessage()))
	}
}

func staleShoppingPlanMessage(snapshot mealplan.Snapshot) OutgoingMessage {
	return planMessage(snapshot, "План уже обновился. Показана актуальная версия.\n\n")
}

func (b *Bot) loadCatalogue(ctx context.Context, page int) (OutgoingMessage, error) {
	entries, err := b.catalogue.ListActive(ctx)
	if err != nil {
		return OutgoingMessage{}, fmt.Errorf("load active catalogue: %w", err)
	}
	response, err := cataloguePage(entries, page)
	if err != nil {
		return OutgoingMessage{}, fmt.Errorf("render active catalogue: %w", err)
	}
	return response, nil
}

func (b *Bot) edit(ctx context.Context, message Message, response OutgoingMessage) error {
	err := b.client.EditMessage(ctx, message.Chat.ID, message.MessageID, response)
	if IsMessageNotModified(err) {
		return nil
	}
	if isRecoverableMessageEditError(err) && message.Chat.Type == "private" {
		if sendErr := b.client.SendMessage(ctx, message.Chat.ID, response); sendErr != nil {
			return fmt.Errorf("send navigation fallback: %w", sendErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("edit navigation message: %w", err)
	}
	return nil
}

func commandName(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	command := strings.ToLower(fields[0])
	if index := strings.IndexByte(command, '@'); index >= 0 {
		command = command[:index]
	}
	return command
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
