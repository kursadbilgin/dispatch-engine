package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ProviderError classifies provider call failures as transient/permanent.
type ProviderError struct {
	StatusCode int
	Message    string
	Transient  bool
	Cause      error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "<nil>"
	}

	parts := make([]string, 0, 4)
	parts = append(parts, "provider error")

	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if msg := strings.TrimSpace(e.Message); msg != "" {
		parts = append(parts, msg)
	}
	if e.Cause != nil {
		parts = append(parts, e.Cause.Error())
	}

	return strings.Join(parts, ": ")
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// IsTransient reports whether an error should be retried.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Transient
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	return false
}
