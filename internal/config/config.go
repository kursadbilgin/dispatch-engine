package config

import (
	"fmt"

	"github.com/Netflix/go-env"
)

type Config struct {
	DatabaseDSN       string `env:"DATABASE_DSN,required=true"`
	RabbitMQURL       string `env:"RABBITMQ_URL,required=true"`
	RedisURL          string `env:"REDIS_URL,required=true"`
	WebhookSiteURL    string `env:"WEBHOOK_SITE_URL,required=true"`
	RateLimitPerSec   int    `env:"RATE_LIMIT_PER_SEC,default=100"`
	WorkerConcurrency int    `env:"WORKER_CONCURRENCY,default=16"`
	APIPort           int    `env:"API_PORT,default=8080"`
	LogLevel          string `env:"LOG_LEVEL,default=info"`
}

func Load() (*Config, error) {
	var cfg Config
	_, err := env.UnmarshalFromEnviron(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return &cfg, nil
}
