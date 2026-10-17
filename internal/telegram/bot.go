package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Suhare00/mealprep-bot/internal/domain"
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

// Bot dispatches Telegram updates to the catalogue use case.
type Bot struct {
	client    Client
	catalogue Catalogue
	logger    *slog.Logger
}

// NewBot constructs the Telegram delivery adapter.
func NewBot(client Client, catalogue Catalogue, logger *slog.Logger) *Bot {
	return &Bot{client: client, catalogue: catalogue, logger: logger}
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
			b.logger.Warn("Telegram polling failed; retrying", "error", err, "backoff", backoff)
			if err := wait(ctx, backoff); err != nil {
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
	ackErr := b.client.AnswerCallbackQuery(ctx, callback.ID)
	if ackErr != nil {
		ackErr = fmt.Errorf("answer callback query: %w", ackErr)
	}

	var processErr error
	if callback.Message != nil && callback.Message.Chat.Type == "private" {
		processErr = b.routeCallback(ctx, callback.Data, *callback.Message)
	}
	return errors.Join(processErr, ackErr)
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
		response = planPlaceholder()
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
