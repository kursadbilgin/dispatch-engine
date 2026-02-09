package queue

import (
	"fmt"
	"strings"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
)

// NotificationMessage is the broker payload for notification processing.
type NotificationMessage struct {
	NotificationID string          `json:"notificationId"`
	CorrelationID  string          `json:"correlationId,omitempty"`
	Channel        domain.Channel  `json:"channel"`
	Priority       domain.Priority `json:"priority"`
}

func (m NotificationMessage) Validate() error {
	if strings.TrimSpace(m.NotificationID) == "" {
		return fmt.Errorf("notificationId is required")
	}
	if !m.Channel.IsValid() {
		return fmt.Errorf("invalid channel %q", m.Channel)
	}
	if !m.Priority.IsValid() {
		return fmt.Errorf("invalid priority %q", m.Priority)
	}
	return nil
}
