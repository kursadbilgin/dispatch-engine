package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
	"gorm.io/gorm"
)

func createBatchesTable() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000003_create_batches",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&repository.BatchModel{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&repository.BatchModel{})
		},
	}
}
