package main

import (
	"log"

	"github.com/kursadbilgin/dispatch-engine/internal/config"
	"github.com/kursadbilgin/dispatch-engine/internal/infra/postgresql"
	"github.com/kursadbilgin/dispatch-engine/internal/infra/postgresql/migrations"
	infraredis "github.com/kursadbilgin/dispatch-engine/internal/infra/redis"
	"github.com/kursadbilgin/dispatch-engine/internal/observability"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	logger, err := observability.NewLogger(cfg.LogLevel)
	if err != nil {
		log.Fatal("failed to initialize logger", zap.Error(err))
	}
	defer logger.Sync() //nolint:errcheck

	db, err := postgresql.NewPostgres(cfg.DatabaseDSN)
	if err != nil {
		logger.Fatal("postgres initialization failed", zap.Error(err))
	}

	if err := migrations.Migrate(db); err != nil {
		logger.Fatal("database migrations failed", zap.Error(err))
	}

	sqlDB, err := db.DB()
	if err != nil {
		logger.Fatal("postgres underlying db init failed", zap.Error(err))
	}
	defer sqlDB.Close()

	rdb, err := infraredis.NewRedis(cfg.RedisURL)
	if err != nil {
		logger.Fatal("redis initialization failed", zap.Error(err))
	}
	defer rdb.Close()

	logger.Info("dispatch-engine api started", zap.Int("port", cfg.APIPort))

	_ = db
	_ = rdb
}
