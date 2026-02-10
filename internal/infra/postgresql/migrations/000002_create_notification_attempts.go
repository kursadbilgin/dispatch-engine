package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
	"gorm.io/gorm"
)

func createNotificationsAttemptsTable() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000002_create_notification_attempts",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.AutoMigrate(&repository.NotificationAttemptModel{}); err != nil {
				return err
			}
			return tx.Exec(`CREATE INDEX IF NOT EXISTS idx_attempts_notification_id ON notification_attempts (notification_id)`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&repository.NotificationAttemptModel{})
		},
	}
}
