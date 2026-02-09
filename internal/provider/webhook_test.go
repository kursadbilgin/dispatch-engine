package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/kursadbilgin/dispatch-engine/internal/domain"
)

func TestWebhookSiteProviderSendSuccess(t *testing.T) {
	t.Parallel()

	var gotBody webhookRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.Header().Set("X-Request-ID", "provider-msg-1")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	p, err := NewWebhookSiteProvider(server.URL)
	if err != nil {
		t.Fatalf("NewWebhookSiteProvider() error = %v", err)
	}

	notification := domain.Notification{
		Channel:   domain.ChannelSMS,
		Priority:  domain.PriorityNormal,
		Recipient: "+905551112233",
		Content:   "hello",
	}

	resp, err := p.Send(context.Background(), notification)
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	if resp.MessageID != "provider-msg-1" {
		t.Fatalf("MessageID = %q, want %q", resp.MessageID, "provider-msg-1")
	}

	if gotBody.To != notification.Recipient {
		t.Fatalf("request.to = %q, want %q", gotBody.To, notification.Recipient)
	}
	if gotBody.Channel != "sms" {
		t.Fatalf("request.channel = %q, want %q", gotBody.Channel, "sms")
	}
	if gotBody.Content != notification.Content {
		t.Fatalf("request.content = %q, want %q", gotBody.Content, notification.Content)
	}
}

func TestWebhookSiteProviderSendStatusClassification(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		statusCode    int
		wantTransient bool
	}{
		{name: "too many requests is transient", statusCode: http.StatusTooManyRequests, wantTransient: true},
		{name: "bad request is permanent", statusCode: http.StatusBadRequest, wantTransient: false},
		{name: "internal server error is transient", statusCode: http.StatusInternalServerError, wantTransient: true},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte("provider failed"))
			}))
			defer server.Close()

			p, err := NewWebhookSiteProvider(server.URL)
			if err != nil {
				t.Fatalf("NewWebhookSiteProvider() error = %v", err)
			}

			_, err = p.Send(context.Background(), domain.Notification{
				Channel:   domain.ChannelSMS,
				Priority:  domain.PriorityNormal,
				Recipient: "+905551112233",
				Content:   "hello",
			})
			if err == nil {
				t.Fatal("expected error")
			}

			if got := IsTransient(err); got != tc.wantTransient {
				t.Fatalf("IsTransient() = %v, want %v", got, tc.wantTransient)
			}

			var providerErr *ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.StatusCode != tc.statusCode {
				t.Fatalf("ProviderError.StatusCode = %d, want %d", providerErr.StatusCode, tc.statusCode)
			}
		})
	}
}

func TestWebhookSiteProviderSendTimeoutIsTransient(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := resty.New()
	client.SetTimeout(30 * time.Millisecond)

	p, err := NewWebhookSiteProviderWithClient(server.URL, client)
	if err != nil {
		t.Fatalf("NewWebhookSiteProviderWithClient() error = %v", err)
	}

	_, err = p.Send(context.Background(), domain.Notification{
		Channel:   domain.ChannelSMS,
		Priority:  domain.PriorityNormal,
		Recipient: "+905551112233",
		Content:   "hello",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !IsTransient(err) {
		t.Fatalf("IsTransient() = false, want true (err=%v)", err)
	}
}
