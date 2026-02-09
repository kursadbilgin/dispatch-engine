package domain

import "time"

// NotificationAttempt records a single delivery attempt for a notification.
type NotificationAttempt struct {
	ID             string  `gorm:"type:uuid;primaryKey"`
	NotificationID string  `gorm:"type:uuid;not null"`
	AttemptNumber  int     `gorm:"not null"`
	StatusCode     *int    `gorm:"type:int"`
	ResponseBody   *string `gorm:"type:text"`
	Error          *string `gorm:"type:text"`
	CreatedAt      time.Time
}
