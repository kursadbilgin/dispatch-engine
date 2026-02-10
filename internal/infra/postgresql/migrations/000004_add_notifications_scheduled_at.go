package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func addNotificationsScheduledAtColumn() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000004_add_notifications_scheduled_at",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE notifications ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ`,
				`CREATE INDEX IF NOT EXISTS idx_notifications_scheduled_due ON notifications (scheduled_at) WHERE status = 'ACCEPTED' AND scheduled_at IS NOT NULL`,
			}
			for _, sql := range statements {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP INDEX IF EXISTS idx_notifications_scheduled_due`,
				`ALTER TABLE notifications DROP COLUMN IF EXISTS scheduled_at`,
			}
			for _, sql := range statements {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
