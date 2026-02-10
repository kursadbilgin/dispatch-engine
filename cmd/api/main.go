package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"os/signal"
	"syscall"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/kursadbilgin/dispatch-engine/internal/config"
	"github.com/kursadbilgin/dispatch-engine/internal/handler"
	"github.com/kursadbilgin/dispatch-engine/internal/infra/postgresql"
	"github.com/kursadbilgin/dispatch-engine/internal/infra/postgresql/migrations"
	infraredis "github.com/kursadbilgin/dispatch-engine/internal/infra/redis"
	"github.com/kursadbilgin/dispatch-engine/internal/observability"
	"github.com/kursadbilgin/dispatch-engine/internal/provider"
	"github.com/kursadbilgin/dispatch-engine/internal/queue"
	"github.com/kursadbilgin/dispatch-engine/internal/ratelimit"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
	"github.com/kursadbilgin/dispatch-engine/internal/service"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const shutdownTimeout = 15 * time.Second

type appDependencies struct {
	notificationRepo    repository.NotificationRepository
	batchRepo           repository.BatchRepository
	attemptRepo         repository.AttemptRepository
	publisher           queue.Publisher
	consumer            queue.Consumer
	notificationService *service.NotificationService
	workerService       *service.WorkerService
	retryScanner        *service.RetryScanner
	provider            provider.Provider
	rateLimiter         ratelimit.RateLimiter
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger, err := observability.NewLogger(cfg.LogLevel)
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer logger.Sync() //nolint:errcheck

	db, err := postgresql.NewPostgres(cfg.DatabaseDSN)
	if err != nil {
		logger.Fatal("postgres initialization failed", zap.Error(err))
	}

	if err := migrations.Run(db); err != nil {
		logger.Fatal("database migrations failed", zap.Error(err))
	}

	sqlDB, err := db.DB()
	if err != nil {
		logger.Fatal("postgres underlying db init failed", zap.Error(err))
	}

	rdb, err := infraredis.NewRedis(cfg.RedisURL)
	if err != nil {
		logger.Fatal("redis initialization failed", zap.Error(err))
	}

	rmq, err := queue.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		logger.Fatal("rabbitmq initialization failed", zap.Error(err))
	}

	app := fiber.New(fiber.Config{
		AppName:      "dispatch-engine",
		ErrorHandler: handler.ErrorHandler(logger),
	})

	metrics := observability.NewMetrics()

	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(cors.New())
	app.Use(metrics.HTTPMiddleware())

	app.Get("/metrics", adaptor.HTTPHandler(metrics.Handler()))

	handler.RegisterHealthRoutes(app, sqlDB, rdb)

	deps, err := wireDependencies(cfg, db, rdb, rmq, logger, metrics)
	if err != nil {
		logger.Fatal("dependency wiring failed", zap.Error(err))
	}

	if err := handler.RegisterNotificationRoutes(app, deps.notificationService); err != nil {
		logger.Fatal("notification route registration failed", zap.Error(err))
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	workerErrCh := make(chan error, 1)
	go func() {
		if err := deps.workerService.Start(signalCtx); err != nil {
			workerErrCh <- err
			return
		}
		workerErrCh <- nil
	}()

	retryScannerErrCh := make(chan error, 1)
	go func() {
		if err := deps.retryScanner.Start(signalCtx); err != nil {
			retryScannerErrCh <- err
			return
		}
		retryScannerErrCh <- nil
	}()

	listenErrCh := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf(":%d", cfg.APIPort)
		logger.Info("dispatch-engine api started", zap.Int("port", cfg.APIPort))
		if err := app.Listen(addr); err != nil {
			listenErrCh <- err
			return
		}
		listenErrCh <- nil
	}()

	select {
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	case err := <-listenErrCh:
		if err != nil {
			logger.Error("fiber listen failed", zap.Error(err))
		}
		stop()
	case err := <-workerErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("worker stopped with error", zap.Error(err))
		}
		stop()
	case err := <-retryScannerErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("retry scanner stopped with error", zap.Error(err))
		}
		stop()
	}

	stop()
	logger.Info("shutting down server")
	if err := app.ShutdownWithTimeout(shutdownTimeout); err != nil {
		logger.Error("server forced shutdown", zap.Error(err))
	}

	closeWithLog(logger, "rabbitmq", rmq.Close)
	closeWithLog(logger, "redis", rdb.Close)
	closeWithLog(logger, "postgres", sqlDB.Close)

	logger.Info("server stopped")
}

func wireDependencies(
	cfg *config.Config,
	db *gorm.DB,
	rdb *goredis.Client,
	rmq *queue.RabbitMQ,
	logger *zap.Logger,
	metrics *observability.Metrics,
) (*appDependencies, error) {
	notificationRepo := repository.NewGormNotificationRepo(db)
	batchRepo := repository.NewGormBatchRepo(db)
	attemptRepo := repository.NewGormAttemptRepo(db)

	publisher := queue.NewRabbitMQPublisher(rmq)
	consumer := queue.NewRabbitMQConsumer(rmq, 1, logger)

	notificationService, err := service.NewNotificationService(notificationRepo, batchRepo, publisher, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notification service: %w", err)
	}

	webhookProvider, err := provider.NewWebhookSiteProvider(cfg.WebhookSiteURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize provider client: %w", err)
	}

	redisRateLimiter, err := infraredis.NewRedisRateLimiter(rdb, cfg.RateLimitPerSec)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize rate limiter: %w", err)
	}

	workerService, err := service.NewWorkerService(
		notificationRepo,
		attemptRepo,
		consumer,
		webhookProvider,
		redisRateLimiter,
		cfg.WorkerConcurrency,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize worker service: %w", err)
	}
	workerService.SetMetrics(metrics)

	retryScanner, err := service.NewRetryScanner(
		notificationRepo,
		publisher,
		cfg.RetryScanInterval,
		cfg.RetryScanLimit,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize retry scanner: %w", err)
	}

	return &appDependencies{
		notificationRepo:    notificationRepo,
		batchRepo:           batchRepo,
		attemptRepo:         attemptRepo,
		publisher:           publisher,
		consumer:            consumer,
		notificationService: notificationService,
		workerService:       workerService,
		retryScanner:        retryScanner,
		provider:            webhookProvider,
		rateLimiter:         redisRateLimiter,
	}, nil
}

func closeWithLog(logger *zap.Logger, component string, closeFn func() error) {
	if closeFn == nil {
		return
	}
	if err := closeFn(); err != nil {
		logger.Error("failed to close component",
			zap.String("component", component),
			zap.Error(err),
		)
	}
}
