package domain

import "time"

// NotificationAttempt records a single delivery attempt for a notification.
type NotificationAttempt struct {
	ID             string
	NotificationID string
	AttemptNumber  int
	StatusCode     *int
	ResponseBody   *string
	Error          *string
	CreatedAt      time.Time
}
