package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		{
			ID: "000001_create_notifications",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.AutoMigrate(&domain.Notification{}); err != nil {
					return err
				}
				indexes := []string{
					`CREATE INDEX IF NOT EXISTS idx_notifications_status_channel_created ON notifications (status, channel, created_at)`,
					`CREATE INDEX IF NOT EXISTS idx_notifications_batch_id ON notifications (batch_id) WHERE batch_id IS NOT NULL`,
					`CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_idempotency_key ON notifications (idempotency_key) WHERE idempotency_key IS NOT NULL`,
					`CREATE INDEX IF NOT EXISTS idx_notifications_retry ON notifications (next_retry_at) WHERE status = 'QUEUED'`,
					`CREATE INDEX IF NOT EXISTS idx_notifications_correlation_id ON notifications (correlation_id)`,
				}
				for _, sql := range indexes {
					if err := tx.Exec(sql).Error; err != nil {
						return err
					}
				}
				return nil
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable(&domain.Notification{})
			},
		},
		{
			ID: "000002_create_notification_attempts",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.AutoMigrate(&domain.NotificationAttempt{}); err != nil {
					return err
				}
				return tx.Exec(`CREATE INDEX IF NOT EXISTS idx_attempts_notification_id ON notification_attempts (notification_id)`).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable(&domain.NotificationAttempt{})
			},
		},
		{
			ID: "000003_create_batches",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&domain.Batch{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable(&domain.Batch{})
			},
		},
	})

	return m.Migrate()
}
