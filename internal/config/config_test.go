package config

import (
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_DSN", "host=localhost user=test password=test dbname=test port=5432 sslmode=disable")
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("WEBHOOK_SITE_URL", "https://webhook.site/test-uuid")
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIPort != 8080 {
		t.Errorf("APIPort = %d, want 8080", cfg.APIPort)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %s, want info", cfg.LogLevel)
	}
	if cfg.RateLimitPerSec != 100 {
		t.Errorf("RateLimitPerSec = %d, want 100", cfg.RateLimitPerSec)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("API_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("RATE_LIMIT_PER_SEC", "250")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIPort != 9090 {
		t.Errorf("APIPort = %d, want 9090", cfg.APIPort)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
	}
	if cfg.RateLimitPerSec != 250 {
		t.Errorf("RateLimitPerSec = %d, want 250", cfg.RateLimitPerSec)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("DATABASE_DSN", "host=localhost")
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required env vars, got nil")
	}
}

func TestLoad_RequiredFields(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseDSN == "" {
		t.Error("DatabaseDSN should not be empty")
	}
	if cfg.RabbitMQURL == "" {
		t.Error("RabbitMQURL should not be empty")
	}
	if cfg.RedisURL == "" {
		t.Error("RedisURL should not be empty")
	}
	if cfg.WebhookSiteURL == "" {
		t.Error("WebhookSiteURL should not be empty")
	}
}
