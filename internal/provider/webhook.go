package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/kursadbilgin/dispatch-engine/internal/domain"
)

const defaultWebhookTimeout = 10 * time.Second

type webhookRequest struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

// WebhookSiteProvider sends notifications to webhook.site-compatible endpoints.
type WebhookSiteProvider struct {
	client   *resty.Client
	endpoint string
}

func NewWebhookSiteProvider(endpoint string) (*WebhookSiteProvider, error) {
	client := resty.New()
	client.SetTimeout(defaultWebhookTimeout)
	client.SetRetryCount(0)

	return NewWebhookSiteProviderWithClient(endpoint, client)
}

func NewWebhookSiteProviderWithClient(endpoint string, client *resty.Client) (*WebhookSiteProvider, error) {
	trimmedEndpoint := strings.TrimSpace(endpoint)
	if trimmedEndpoint == "" {
		return nil, fmt.Errorf("webhook endpoint is required")
	}
	if _, err := url.ParseRequestURI(trimmedEndpoint); err != nil {
		return nil, fmt.Errorf("invalid webhook endpoint: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("resty client is required")
	}

	if client.GetClient().Timeout == 0 {
		client.SetTimeout(defaultWebhookTimeout)
	}
	client.SetRetryCount(0)

	return &WebhookSiteProvider{
		client:   client,
		endpoint: trimmedEndpoint,
	}, nil
}

func (p *WebhookSiteProvider) Send(ctx context.Context, notification domain.Notification) (*ProviderResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("provider is not initialized")
	}
	if err := notification.Validate(); err != nil {
		return nil, fmt.Errorf("invalid notification: %w", err)
	}

	reqBody := webhookRequest{
		To:      notification.Recipient,
		Channel: strings.ToLower(notification.Channel.String()),
		Content: notification.Content,
	}

	response, err := p.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		Post(p.endpoint)
	if err != nil {
		return nil, &ProviderError{
			Message:   "provider request failed",
			Transient: !errors.Is(err, context.Canceled),
			Cause:     err,
		}
	}
	if response == nil {
		return nil, &ProviderError{
			Message:   "provider returned empty response",
			Transient: true,
		}
	}

	statusCode := response.StatusCode()
	responseBody := strings.TrimSpace(response.String())

	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return &ProviderResponse{
			StatusCode: statusCode,
			Body:       responseBody,
			MessageID:  providerMessageID(response),
		}, nil
	}

	return nil, &ProviderError{
		StatusCode: statusCode,
		Message:    providerErrorMessage(statusCode, responseBody),
		Transient:  isTransientHTTPStatus(statusCode),
	}
}

func isTransientHTTPStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || (statusCode >= http.StatusInternalServerError && statusCode <= 599)
}

func providerErrorMessage(statusCode int, body string) string {
	base := fmt.Sprintf("provider returned status %d", statusCode)
	if body == "" {
		return base
	}
	return fmt.Sprintf("%s: %s", base, body)
}

func providerMessageID(response *resty.Response) string {
	if response == nil {
		return ""
	}

	for _, key := range []string{"X-Request-ID", "X-Request-Id", "X-Correlation-ID", "X-Correlation-Id"} {
		if value := strings.TrimSpace(response.Header().Get(key)); value != "" {
			return value
		}
	}

	return ""
}
