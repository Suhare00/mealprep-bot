package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIClientGetUpdatesUsesOffsetAndDecodesResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/getUpdates") {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		var body struct {
			Offset  int64 `json:"offset"`
			Timeout int   `json:"timeout"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Offset != 12 || body.Timeout != 30 {
			t.Fatalf("request body = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":12,"message":{"message_id":1,"chat":{"id":42,"type":"private"},"text":"/start"}}]}`))
	}))
	defer server.Close()

	updates, err := newAPIClient("test-token", server.URL, server.Client()).GetUpdates(t.Context(), 12, 30)
	if err != nil {
		t.Fatalf("GetUpdates() error = %v", err)
	}
	if len(updates) != 1 || updates[0].UpdateID != 12 || updates[0].Message.Text != "/start" {
		t.Fatalf("GetUpdates() = %#v", updates)
	}
}

func TestAPIClientUsesTelegramResponseStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"bad request"}`))
	}))
	defer server.Close()

	client := newAPIClient("test-token", server.URL, server.Client())
	err := client.SendMessage(t.Context(), 1, OutgoingMessage{Text: "hello"})
	if err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("SendMessage() error = %v", err)
	}
}

func TestAPIClientRedactsTokenFromNetworkErrors(t *testing.T) {
	const token = "123456:super-secret"
	client := newAPIClient(token, "https://api.telegram.invalid", &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return nil, errors.New("dial " + request.URL.String() + ": failed")
		}),
	})

	_, err := client.GetUpdates(t.Context(), 0, 30)
	if err == nil {
		t.Fatal("GetUpdates() error = nil")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("GetUpdates() leaked token: %v", err)
	}
}

func TestAPIClientPropagatesCancellationWithoutToken(t *testing.T) {
	const token = "123456:super-secret"
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := newAPIClient(token, "https://api.telegram.invalid", &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return nil, request.Context().Err()
		}),
	})

	_, err := client.GetUpdates(ctx, 0, 30)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUpdates() error = %v, want context.Canceled", err)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("GetUpdates() leaked token: %v", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
