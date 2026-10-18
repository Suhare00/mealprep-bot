package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	defaultAPIBaseURL = "https://api.telegram.org"
	maxResponseBytes  = 1 << 20
)

// Client is the Telegram API surface used by the bot.
type Client interface {
	GetWebhookInfo(ctx context.Context) (WebhookInfo, error)
	GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error)
	SendMessage(ctx context.Context, chatID int64, message OutgoingMessage) error
	EditMessage(ctx context.Context, chatID int64, messageID int, message OutgoingMessage) error
	AnswerCallbackQuery(ctx context.Context, callbackQueryID string) error
}

// WebhookInfo reports whether Telegram currently delivers updates to a webhook.
type WebhookInfo struct {
	URL string `json:"url"`
}

// APIClient is a focused Telegram Bot API HTTP adapter.
type APIClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates a Telegram client. The HTTP timeout must exceed the long-poll wait.
func NewAPIClient(token string, httpClient *http.Client) *APIClient {
	return newAPIClient(token, defaultAPIBaseURL, httpClient)
}

func newAPIClient(token, baseURL string, httpClient *http.Client) *APIClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &APIClient{
		token:      token,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *APIClient) GetWebhookInfo(ctx context.Context) (WebhookInfo, error) {
	return call[WebhookInfo](ctx, c, "getWebhookInfo", struct{}{})
}

func (c *APIClient) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	request := struct {
		Offset         int64    `json:"offset"`
		Timeout        int      `json:"timeout"`
		AllowedUpdates []string `json:"allowed_updates"`
	}{
		Offset:         offset,
		Timeout:        timeoutSeconds,
		AllowedUpdates: []string{"message", "callback_query"},
	}
	return call[[]Update](ctx, c, "getUpdates", request)
}

func (c *APIClient) SendMessage(ctx context.Context, chatID int64, message OutgoingMessage) error {
	request := struct {
		ChatID      int64                `json:"chat_id"`
		Text        string               `json:"text"`
		ReplyMarkup InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	}{ChatID: chatID, Text: message.Text, ReplyMarkup: message.ReplyMarkup}
	_, err := call[Message](ctx, c, "sendMessage", request)
	return err
}

func (c *APIClient) EditMessage(ctx context.Context, chatID int64, messageID int, message OutgoingMessage) error {
	request := struct {
		ChatID      int64                `json:"chat_id"`
		MessageID   int                  `json:"message_id"`
		Text        string               `json:"text"`
		ReplyMarkup InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	}{ChatID: chatID, MessageID: messageID, Text: message.Text, ReplyMarkup: message.ReplyMarkup}
	_, err := call[Message](ctx, c, "editMessageText", request)
	return err
}

func (c *APIClient) AnswerCallbackQuery(ctx context.Context, callbackQueryID string) error {
	request := struct {
		CallbackQueryID string `json:"callback_query_id"`
	}{CallbackQueryID: callbackQueryID}
	_, err := call[bool](ctx, c, "answerCallbackQuery", request)
	return err
}

type apiResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
}

type apiError struct {
	method      string
	code        int
	description string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("Telegram %s failed (%d): %s", e.method, e.code, e.description)
}

// IsMessageNotModified reports the harmless Telegram edit error for identical content.
func IsMessageNotModified(err error) bool {
	var target *apiError
	return errors.As(err, &target) && strings.Contains(strings.ToLower(target.description), "message is not modified")
}

func isRecoverableMessageEditError(err error) bool {
	var target *apiError
	if !errors.As(err, &target) || target.method != "editMessageText" || target.code != http.StatusBadRequest {
		return false
	}

	description := strings.ToLower(target.description)
	for _, text := range []string{
		"message to edit not found",
		"message can't be edited",
		"message can not be edited",
		"message cannot be edited",
		"message is too old",
	} {
		if strings.Contains(description, text) {
			return true
		}
	}
	return false
}

func isPermanentPollingError(err error) bool {
	var target *apiError
	return errors.As(err, &target) && target.method == "getUpdates" && target.code == http.StatusConflict
}

func isExpiredCallbackAcknowledgement(err error) bool {
	var target *apiError
	if !errors.As(err, &target) || target.method != "answerCallbackQuery" || target.code != http.StatusBadRequest {
		return false
	}

	description := strings.ToLower(target.description)
	return strings.Contains(description, "query is too old") ||
		strings.Contains(description, "query id is invalid") ||
		strings.Contains(description, "response timeout expired")
}

func call[T any](ctx context.Context, client *APIClient, method string, payload any) (T, error) {
	var zero T
	body, err := json.Marshal(payload)
	if err != nil {
		return zero, fmt.Errorf("encode Telegram %s request: %w", method, err)
	}
	url := client.baseURL + "/bot" + client.token + "/" + method
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("create Telegram %s request: %s", method, redact(client.token, err.Error()))
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return zero, fmt.Errorf("telegram %s request cancelled: %w", method, ctx.Err())
		}
		return zero, fmt.Errorf("telegram %s request failed: %s", method, redact(client.token, err.Error()))
	}
	defer func() { _ = response.Body.Close() }()

	var decoded apiResponse[T]
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(&decoded); err != nil {
		return zero, fmt.Errorf("decode Telegram %s response: %w", method, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || !decoded.OK {
		description := redact(client.token, decoded.Description)
		if description == "" {
			description = http.StatusText(response.StatusCode)
		}
		code := decoded.ErrorCode
		if code == 0 {
			code = response.StatusCode
		}
		return zero, &apiError{method: method, code: code, description: description}
	}
	return decoded.Result, nil
}

func redact(secret, value string) string {
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[REDACTED]")
}
