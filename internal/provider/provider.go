package provider

import (
	"context"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
)

// Provider is the outbound notification delivery port.
type Provider interface {
	Send(ctx context.Context, notification domain.Notification) (*ProviderResponse, error)
}

// ProviderResponse stores provider call metadata for audit and persistence.
type ProviderResponse struct {
	StatusCode int
	Body       string
	MessageID  string
}
