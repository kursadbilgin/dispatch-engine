package repository

import (
	"time"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
)

// NotificationModel is the persistence model for the notifications table.
type NotificationModel struct {
	ID                string          `gorm:"type:uuid;primaryKey"`
	CorrelationID     string          `gorm:"type:varchar(36);not null"`
	IdempotencyKey    *string         `gorm:"type:varchar(255)"`
	BatchID           *string         `gorm:"type:uuid"`
	Channel           domain.Channel  `gorm:"type:varchar(10);not null"`
	Priority          domain.Priority `gorm:"type:varchar(10);not null"`
	Recipient         string          `gorm:"type:varchar(255);not null"`
	Content           string          `gorm:"type:text;not null"`
	Status            domain.Status   `gorm:"type:varchar(20);not null"`
	ProviderMessageID *string         `gorm:"type:varchar(255)"`
	AttemptCount      int             `gorm:"not null;default:0"`
	MaxRetries        int             `gorm:"not null;default:5"`
	ScheduledAt       *time.Time      `gorm:"type:timestamptz"`
	NextRetryAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (NotificationModel) TableName() string {
	return "notifications"
}

// NotificationAttemptModel is the persistence model for notification_attempts.
type NotificationAttemptModel struct {
	ID             string  `gorm:"type:uuid;primaryKey"`
	NotificationID string  `gorm:"type:uuid;not null"`
	AttemptNumber  int     `gorm:"not null"`
	StatusCode     *int    `gorm:"type:int"`
	ResponseBody   *string `gorm:"type:text"`
	Error          *string `gorm:"type:text"`
	CreatedAt      time.Time
}

func (NotificationAttemptModel) TableName() string {
	return "notification_attempts"
}

// BatchModel is the persistence model for batches.
type BatchModel struct {
	ID         string             `gorm:"type:uuid;primaryKey"`
	TotalCount int                `gorm:"not null"`
	Status     domain.BatchStatus `gorm:"type:varchar(20);not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (BatchModel) TableName() string {
	return "batches"
}

func notificationModelFromDomain(n *domain.Notification) *NotificationModel {
	if n == nil {
		return nil
	}

	return &NotificationModel{
		ID:                n.ID,
		CorrelationID:     n.CorrelationID,
		IdempotencyKey:    n.IdempotencyKey,
		BatchID:           n.BatchID,
		Channel:           n.Channel,
		Priority:          n.Priority,
		Recipient:         n.Recipient,
		Content:           n.Content,
		Status:            n.Status,
		ProviderMessageID: n.ProviderMessageID,
		AttemptCount:      n.AttemptCount,
		MaxRetries:        n.MaxRetries,
		ScheduledAt:       n.ScheduledAt,
		NextRetryAt:       n.NextRetryAt,
		CreatedAt:         n.CreatedAt,
		UpdatedAt:         n.UpdatedAt,
	}
}

func notificationModelToDomain(m *NotificationModel) *domain.Notification {
	if m == nil {
		return nil
	}

	return &domain.Notification{
		ID:                m.ID,
		CorrelationID:     m.CorrelationID,
		IdempotencyKey:    m.IdempotencyKey,
		BatchID:           m.BatchID,
		Channel:           m.Channel,
		Priority:          m.Priority,
		Recipient:         m.Recipient,
		Content:           m.Content,
		Status:            m.Status,
		ProviderMessageID: m.ProviderMessageID,
		AttemptCount:      m.AttemptCount,
		MaxRetries:        m.MaxRetries,
		ScheduledAt:       m.ScheduledAt,
		NextRetryAt:       m.NextRetryAt,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func attemptModelFromDomain(a *domain.NotificationAttempt) *NotificationAttemptModel {
	if a == nil {
		return nil
	}

	return &NotificationAttemptModel{
		ID:             a.ID,
		NotificationID: a.NotificationID,
		AttemptNumber:  a.AttemptNumber,
		StatusCode:     a.StatusCode,
		ResponseBody:   a.ResponseBody,
		Error:          a.Error,
		CreatedAt:      a.CreatedAt,
	}
}

func attemptModelToDomain(m *NotificationAttemptModel) *domain.NotificationAttempt {
	if m == nil {
		return nil
	}

	return &domain.NotificationAttempt{
		ID:             m.ID,
		NotificationID: m.NotificationID,
		AttemptNumber:  m.AttemptNumber,
		StatusCode:     m.StatusCode,
		ResponseBody:   m.ResponseBody,
		Error:          m.Error,
		CreatedAt:      m.CreatedAt,
	}
}

func batchModelFromDomain(b *domain.Batch) *BatchModel {
	if b == nil {
		return nil
	}

	return &BatchModel{
		ID:         b.ID,
		TotalCount: b.TotalCount,
		Status:     b.Status,
		CreatedAt:  b.CreatedAt,
		UpdatedAt:  b.UpdatedAt,
	}
}

func batchModelToDomain(m *BatchModel) *domain.Batch {
	if m == nil {
		return nil
	}

	return &domain.Batch{
		ID:         m.ID,
		TotalCount: m.TotalCount,
		Status:     m.Status,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}
