package handler

import (
	"context"
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

const readinessTimeout = 2 * time.Second

func RegisterHealthRoutes(app fiber.Router, sqlDB *sql.DB, rdb *redis.Client) {
	app.Get("/livez", LivezHandler())
	app.Get("/readyz", ReadyzHandler(sqlDB, rdb))
}

func LivezHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ok",
		})
	}
}

func ReadyzHandler(sqlDB *sql.DB, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), readinessTimeout)
		defer cancel()

		pgErr := sqlDB.PingContext(ctx)
		redisErr := rdb.Ping(ctx).Err()

		pgStatus := "ok"
		if pgErr != nil {
			pgStatus = "down"
		}
		redisStatus := "ok"
		if redisErr != nil {
			redisStatus = "down"
		}

		status := "ready"
		statusCode := fiber.StatusOK
		if pgErr != nil || redisErr != nil {
			status = "not_ready"
			statusCode = fiber.StatusServiceUnavailable
		}

		return c.Status(statusCode).JSON(fiber.Map{
			"status": status,
			"checks": fiber.Map{
				"postgres": pgStatus,
				"redis":    redisStatus,
			},
		})
	}
}
