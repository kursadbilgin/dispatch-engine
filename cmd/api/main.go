package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/kursadbilgin/dispatch-engine/internal/config"
	"github.com/kursadbilgin/dispatch-engine/internal/infra/postgresql"
	"github.com/kursadbilgin/dispatch-engine/internal/infra/postgresql/migrations"
	infraredis "github.com/kursadbilgin/dispatch-engine/internal/infra/redis"
	"github.com/kursadbilgin/dispatch-engine/internal/observability"
	"github.com/kursadbilgin/dispatch-engine/internal/transport"
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

	app := fiber.New(fiber.Config{
		AppName:      "dispatch-engine",
		ErrorHandler: transport.ErrorHandler(logger),
	})

	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(cors.New())

	transport.RegisterHealthRoutes(app, sqlDB, rdb)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.APIPort)
		logger.Info("dispatch-engine api started", zap.Int("port", cfg.APIPort))
		if err := app.Listen(addr); err != nil {
			logger.Fatal("fiber listen failed", zap.Error(err))
		}
	}()

	<-quit
	logger.Info("shutting down server")
	if err := app.ShutdownWithTimeout(15e9); err != nil {
		logger.Error("server forced shutdown", zap.Error(err))
	}
	logger.Info("server stopped")
}
