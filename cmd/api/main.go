package main

import (
	"log"

	"github.com/kursadbilgin/dispatch-engine/internal/config"
	"github.com/kursadbilgin/dispatch-engine/internal/infra/postgresql"
	"github.com/kursadbilgin/dispatch-engine/internal/infra/postgresql/migrations"
	infraredis "github.com/kursadbilgin/dispatch-engine/internal/infra/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := postgresql.NewPostgres(cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}

	if err := migrations.Migrate(db); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("postgres underlying db: %v", err)
	}
	defer sqlDB.Close()

	rdb, err := infraredis.NewRedis(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	log.Printf("dispatch-engine api started on port %d", cfg.APIPort)

	_ = db
	_ = rdb
}
