package domain

import (
	"fmt"
	"strings"
	"time"
)

// Status represents the lifecycle state of a notification.
type Status string

const (
	StatusAccepted Status = "ACCEPTED"
	StatusQueued   Status = "QUEUED"
	StatusSending  Status = "SENDING"
	StatusSent     Status = "SENT"
	StatusFailed   Status = "FAILED"
	StatusCanceled Status = "CANCELED"
)

func (s Status) String() string { return string(s) }

func (s Status) IsValid() bool {
	switch s {
	case StatusAccepted, StatusQueued, StatusSending, StatusSent, StatusFailed, StatusCanceled:
		return true
	}
	return false
}

func ParseStatusFromString(s string) (Status, error) {
	st := Status(strings.ToUpper(strings.TrimSpace(s)))
	if !st.IsValid() {
		return "", fmt.Errorf("%w: invalid status %q", ErrValidation, s)
	}
	return st, nil
}

// Channel represents the delivery channel.
type Channel string

const (
	ChannelSMS   Channel = "SMS"
	ChannelEmail Channel = "EMAIL"
	ChannelPush  Channel = "PUSH"
)

func (c Channel) String() string { return string(c) }

func (c Channel) IsValid() bool {
	switch c {
	case ChannelSMS, ChannelEmail, ChannelPush:
		return true
	}
	return false
}

func ParseChannelFromString(s string) (Channel, error) {
	ch := Channel(strings.ToUpper(strings.TrimSpace(s)))
	if !ch.IsValid() {
		return "", fmt.Errorf("%w: invalid channel %q", ErrValidation, s)
	}
	return ch, nil
}

// Priority represents the message priority level.
type Priority string

const (
	PriorityHigh   Priority = "HIGH"
	PriorityNormal Priority = "NORMAL"
	PriorityLow    Priority = "LOW"
)

func (p Priority) String() string { return string(p) }

func (p Priority) IsValid() bool {
	switch p {
	case PriorityHigh, PriorityNormal, PriorityLow:
		return true
	}
	return false
}

func ParsePriorityFromString(s string) (Priority, error) {
	pr := Priority(strings.ToUpper(strings.TrimSpace(s)))
	if !pr.IsValid() {
		return "", fmt.Errorf("%w: invalid priority %q", ErrValidation, s)
	}
	return pr, nil
}

// Content limits per channel (in characters).
const (
	MaxSMSContent   = 160
	MaxPushContent  = 240
	MaxEmailContent = 10000
)

// Notification is the core domain entity representing a message to be delivered.
type Notification struct {
	ID                string   `gorm:"type:uuid;primaryKey"`
	CorrelationID     string   `gorm:"type:varchar(36);not null"`
	IdempotencyKey    *string  `gorm:"type:varchar(255)"`
	BatchID           *string  `gorm:"type:uuid"`
	Channel           Channel  `gorm:"type:varchar(10);not null"`
	Priority          Priority `gorm:"type:varchar(10);not null"`
	Recipient         string   `gorm:"type:varchar(255);not null"`
	Content           string   `gorm:"type:text;not null"`
	Status            Status   `gorm:"type:varchar(20);not null"`
	ProviderMessageID *string  `gorm:"type:varchar(255)"`
	AttemptCount      int      `gorm:"not null;default:0"`
	MaxRetries        int      `gorm:"not null;default:5"`
	NextRetryAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (n *Notification) Validate() error {
	if n.Recipient == "" {
		return fmt.Errorf("%w: recipient is required", ErrValidation)
	}
	if n.Content == "" {
		return fmt.Errorf("%w: content is required", ErrValidation)
	}
	if !n.Channel.IsValid() {
		return fmt.Errorf("%w: invalid channel %q", ErrValidation, n.Channel)
	}
	if !n.Priority.IsValid() {
		return fmt.Errorf("%w: invalid priority %q", ErrValidation, n.Priority)
	}

	contentLen := len([]rune(n.Content))
	switch n.Channel {
	case ChannelSMS:
		if contentLen > MaxSMSContent {
			return fmt.Errorf("%w: SMS content exceeds %d characters (got %d)", ErrValidation, MaxSMSContent, contentLen)
		}
	case ChannelPush:
		if contentLen > MaxPushContent {
			return fmt.Errorf("%w: push content exceeds %d characters (got %d)", ErrValidation, MaxPushContent, contentLen)
		}
	case ChannelEmail:
		if contentLen > MaxEmailContent {
			return fmt.Errorf("%w: email content exceeds %d characters (got %d)", ErrValidation, MaxEmailContent, contentLen)
		}
	}

	return nil
}
