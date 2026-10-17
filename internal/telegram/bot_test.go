package telegram

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	recipecatalogue "github.com/Suhare00/mealprep-bot/internal/recipe"
)

func TestBotCommands(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		wantText string
		wantList bool
	}{
		{name: "start", command: "/start", wantText: "Добро пожаловать"},
		{name: "help", command: "/help@mealprep_bot", wantText: "Доступные команды"},
		{name: "recipes", command: "/recipes", wantText: "Каталог рецептов", wantList: true},
		{name: "unknown", command: "/unknown", wantText: "Неизвестная команда"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeClient{}
			catalogue := &fakeCatalogue{entries: catalogueEntries(2)}
			bot := newTestBot(client, catalogue)

			err := bot.HandleUpdate(t.Context(), Update{Message: &Message{
				Chat: Chat{ID: 42, Type: "private"}, Text: test.command,
			}})
			if err != nil {
				t.Fatalf("HandleUpdate() error = %v", err)
			}
			if len(client.sent) != 1 || !strings.Contains(client.sent[0].message.Text, test.wantText) {
				t.Fatalf("sent = %#v, want text %q", client.sent, test.wantText)
			}
			if catalogue.listCalls != boolInt(test.wantList) {
				t.Fatalf("ListActive() calls = %d, want %v", catalogue.listCalls, test.wantList)
			}
		})
	}
}

func TestBotCallbackRoutingAndAcknowledgement(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		wantText string
	}{
		{name: "menu", data: "m", wantText: "Добро пожаловать"},
		{name: "plan", data: "p", wantText: "в разработке"},
		{name: "catalogue", data: "c:0", wantText: "Каталог рецептов"},
		{name: "invalid", data: "r:not-a-page:R001", wantText: "устарела или повреждена"},
		{name: "oversized", data: strings.Repeat("x", 65), wantText: "устарела или повреждена"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeClient{}
			bot := newTestBot(client, &fakeCatalogue{entries: catalogueEntries(1)})
			update := callbackUpdate(test.data)

			if err := bot.HandleUpdate(t.Context(), update); err != nil {
				t.Fatalf("HandleUpdate() error = %v", err)
			}
			if len(client.edited) != 1 || !strings.Contains(client.edited[0].message.Text, test.wantText) {
				t.Fatalf("edited = %#v, want text %q", client.edited, test.wantText)
			}
			if len(client.answered) != 1 || client.answered[0] != "callback-1" {
				t.Fatalf("answered callbacks = %v", client.answered)
			}
		})
	}
}

func TestBotRecipeDetailAndUnavailableRecipe(t *testing.T) {
	tests := []struct {
		name      string
		entry     recipecatalogue.CatalogueEntry
		err       error
		wantTexts []string
	}{
		{
			name:  "detail",
			entry: detailedEntry(),
			wantTexts: []string{
				"Тушёные овощи", "обед, ужин", "Рис — 1.25 кг", "Морковь — 250 г", "1 ч 30 мин",
			},
		},
		{
			name:      "missing or disabled",
			err:       recipecatalogue.ErrUnavailable,
			wantTexts: []string{"больше недоступен"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeClient{}
			bot := newTestBot(client, &fakeCatalogue{entry: test.entry, getErr: test.err})

			if err := bot.HandleUpdate(t.Context(), callbackUpdate("r:2:R001")); err != nil {
				t.Fatalf("HandleUpdate() error = %v", err)
			}
			if len(client.answered) != 1 {
				t.Fatalf("answered callbacks = %v", client.answered)
			}
			for _, want := range test.wantTexts {
				if !strings.Contains(client.edited[0].message.Text, want) {
					t.Fatalf("detail text = %q, want %q", client.edited[0].message.Text, want)
				}
			}
		})
	}
}

func TestBotCallbackIsAnsweredOnProcessingErrors(t *testing.T) {
	processingErr := errors.New("database failed")
	client := &fakeClient{}
	bot := newTestBot(client, &fakeCatalogue{listErr: processingErr})

	err := bot.HandleUpdate(t.Context(), callbackUpdate("c:0"))
	if !errors.Is(err, processingErr) {
		t.Fatalf("HandleUpdate() error = %v, want processing error", err)
	}
	if len(client.answered) != 1 {
		t.Fatalf("answered callbacks = %v", client.answered)
	}
	if len(client.edited) != 1 || !strings.Contains(client.edited[0].message.Text, "Попробуйте") {
		t.Fatalf("edited = %#v", client.edited)
	}
}

func TestBotPropagatesTelegramAPIErrors(t *testing.T) {
	apiErr := errors.New("telegram unavailable")

	t.Run("send command response", func(t *testing.T) {
		client := &fakeClient{sendErr: apiErr}
		err := newTestBot(client, &fakeCatalogue{}).HandleUpdate(t.Context(), Update{Message: &Message{
			Chat: Chat{ID: 42, Type: "private"}, Text: "/start",
		}})
		if !errors.Is(err, apiErr) {
			t.Fatalf("HandleUpdate() error = %v, want API error", err)
		}
	})

	t.Run("answer callback", func(t *testing.T) {
		client := &fakeClient{answerErr: apiErr}
		err := newTestBot(client, &fakeCatalogue{}).HandleUpdate(t.Context(), callbackUpdate("m"))
		if !errors.Is(err, apiErr) {
			t.Fatalf("HandleUpdate() error = %v, want API error", err)
		}
		if len(client.edited) != 1 {
			t.Fatalf("callback route did not run after acknowledgement failure: %#v", client.edited)
		}
	})
}

func TestBotIgnoresUnsupportedAndNonPrivateUpdates(t *testing.T) {
	client := &fakeClient{}
	bot := newTestBot(client, &fakeCatalogue{})

	updates := []Update{
		{},
		{Message: &Message{Chat: Chat{ID: -1, Type: "group"}, Text: "/start"}},
		{CallbackQuery: &CallbackQuery{ID: "group-callback", Message: &Message{Chat: Chat{ID: -1, Type: "group"}}, Data: "m"}},
	}
	for _, update := range updates {
		if err := bot.HandleUpdate(t.Context(), update); err != nil {
			t.Fatalf("HandleUpdate() error = %v", err)
		}
	}
	if len(client.sent) != 0 || len(client.edited) != 0 {
		t.Fatalf("non-private update produced output: sent=%v edited=%v", client.sent, client.edited)
	}
	if len(client.answered) != 1 || client.answered[0] != "group-callback" {
		t.Fatalf("callback acknowledgement = %v", client.answered)
	}
}

func TestBotRunUsesOffsetsAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	client := &fakeClient{}
	client.getUpdates = func(ctx context.Context, offset int64, _ int) ([]Update, error) {
		client.offsets = append(client.offsets, offset)
		if len(client.offsets) == 1 {
			return []Update{{UpdateID: 7}}, nil
		}
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	}

	if err := newTestBot(client, &fakeCatalogue{}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(client.offsets) != 2 || client.offsets[0] != 0 || client.offsets[1] != 8 {
		t.Fatalf("getUpdates offsets = %v, want [0 8]", client.offsets)
	}
}

func TestBotRunRejectsActiveWebhook(t *testing.T) {
	client := &fakeClient{webhook: WebhookInfo{URL: "https://example.com/webhook"}}
	err := newTestBot(client, &fakeCatalogue{}).Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "webhook is active") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestCataloguePaginationBoundariesAndEmptyState(t *testing.T) {
	entries := catalogueEntries(20)
	tests := []struct {
		name      string
		page      int
		entries   []recipecatalogue.CatalogueEntry
		wantPage  string
		wantFirst string
	}{
		{name: "first", page: 0, entries: entries, wantPage: "Страница 1 из 3", wantFirst: "Recipe 01"},
		{name: "middle", page: 1, entries: entries, wantPage: "Страница 2 из 3", wantFirst: "Recipe 10"},
		{name: "last", page: 2, entries: entries, wantPage: "Страница 3 из 3", wantFirst: "Recipe 19"},
		{name: "out of range clamps", page: 99, entries: entries, wantPage: "Страница 3 из 3", wantFirst: "Recipe 19"},
		{name: "empty", page: 0, entries: nil, wantPage: "Каталог пока пуст"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, err := cataloguePage(test.entries, test.page)
			if err != nil {
				t.Fatalf("cataloguePage() error = %v", err)
			}
			if !strings.Contains(message.Text, test.wantPage) {
				t.Fatalf("text = %q, want %q", message.Text, test.wantPage)
			}
			if test.wantFirst != "" && message.ReplyMarkup.InlineKeyboard[0][0].Text != test.wantFirst {
				t.Fatalf("first button = %q, want %q", message.ReplyMarkup.InlineKeyboard[0][0].Text, test.wantFirst)
			}
		})
	}
}

func TestRecipeDetailUsesSafePlainTextAndTelegramSize(t *testing.T) {
	entry := detailedEntry()
	entry.Recipe.Name = "<b>не HTML</b> " + strings.Repeat("🧪", 3000)
	message := recipeDetail(entry, 0)

	if !strings.Contains(message.Text, "<b>не HTML</b>") {
		t.Fatalf("plain text content was changed: %q", message.Text[:40])
	}
	if utf8.RuneCountInString(message.Text) > maxMessageRunes {
		t.Fatalf("message rune count = %d, max %d", utf8.RuneCountInString(message.Text), maxMessageRunes)
	}
}

func newTestBot(client Client, catalogue Catalogue) *Bot {
	return NewBot(client, catalogue, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func callbackUpdate(data string) Update {
	return Update{CallbackQuery: &CallbackQuery{
		ID:      "callback-1",
		Message: &Message{MessageID: 8, Chat: Chat{ID: 42, Type: "private"}},
		Data:    data,
	}}
}

func catalogueEntries(count int) []recipecatalogue.CatalogueEntry {
	entries := make([]recipecatalogue.CatalogueEntry, count)
	for i := range entries {
		entries[i].Recipe.ID = domain.RecipeID(strings.ReplaceAll(time.Date(2000, 1, i+1, 0, 0, 0, 0, time.UTC).Format("02"), " ", "0"))
		entries[i].Recipe.Name = "Recipe " + time.Date(2000, 1, i+1, 0, 0, 0, 0, time.UTC).Format("02")
		entries[i].Recipe.State = domain.RecipeStateActive
	}
	return entries
}

func detailedEntry() recipecatalogue.CatalogueEntry {
	return recipecatalogue.CatalogueEntry{
		Recipe: domain.Recipe{
			ID: "R001", Name: "Тушёные овощи", Category: "Основное", Servings: 4,
			MealTypes:           []domain.MealType{domain.MealTypeLunch, domain.MealTypeDinner},
			PreparationDuration: 15 * time.Minute, CookingDuration: 90 * time.Minute,
			StorageDays: 3, FreezerFriendly: true, Difficulty: domain.DifficultyEasy,
			BudgetLevel: domain.BudgetLevelLow, ReheatScore: 4, BatchCookingScore: 5,
			Tags: []string{"овощи", "партия"}, State: domain.RecipeStateActive,
			Ingredients: []domain.RecipeIngredient{
				{IngredientID: "I001", Quantity: 1.25, Unit: "кг"},
				{IngredientID: "I002", Quantity: 250, Unit: "г"},
			},
		},
		Ingredients: []domain.Ingredient{{ID: "I001", Name: "Рис"}, {ID: "I002", Name: "Морковь"}},
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type sentMessage struct {
	chatID    int64
	messageID int
	message   OutgoingMessage
}

type fakeClient struct {
	mu         sync.Mutex
	webhook    WebhookInfo
	webhookErr error
	getUpdates func(context.Context, int64, int) ([]Update, error)
	sendErr    error
	editErr    error
	answerErr  error
	sent       []sentMessage
	edited     []sentMessage
	answered   []string
	offsets    []int64
}

func (f *fakeClient) GetWebhookInfo(context.Context) (WebhookInfo, error) {
	return f.webhook, f.webhookErr
}

func (f *fakeClient) GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	if f.getUpdates != nil {
		return f.getUpdates(ctx, offset, timeout)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *fakeClient) SendMessage(_ context.Context, chatID int64, message OutgoingMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMessage{chatID: chatID, message: message})
	return f.sendErr
}

func (f *fakeClient) EditMessage(_ context.Context, chatID int64, messageID int, message OutgoingMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edited = append(f.edited, sentMessage{chatID: chatID, messageID: messageID, message: message})
	return f.editErr
}

func (f *fakeClient) AnswerCallbackQuery(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = append(f.answered, id)
	return f.answerErr
}

type fakeCatalogue struct {
	entries   []recipecatalogue.CatalogueEntry
	entry     recipecatalogue.CatalogueEntry
	listErr   error
	getErr    error
	listCalls int
}

func (f *fakeCatalogue) ListActive(context.Context) ([]recipecatalogue.CatalogueEntry, error) {
	f.listCalls++
	return f.entries, f.listErr
}

func (f *fakeCatalogue) GetActiveByID(context.Context, domain.RecipeID) (recipecatalogue.CatalogueEntry, error) {
	return f.entry, f.getErr
}
