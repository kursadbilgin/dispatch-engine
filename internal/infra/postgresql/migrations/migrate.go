package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func Migrations() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		createNotificationsTable(),
		createNotificationsAttemptsTable(),
		createBatchesTable(),
		addNotificationsScheduledAtColumn(),
	}
}

func Run(db *gorm.DB) error {
	m := gormigrate.New(db, gormigrate.DefaultOptions, Migrations())

	return m.Migrate()
}
