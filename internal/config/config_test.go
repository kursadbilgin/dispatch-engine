package config

import (
	"testing"
	"time"
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
	if cfg.WorkerConcurrency != 16 {
		t.Errorf("WorkerConcurrency = %d, want 16", cfg.WorkerConcurrency)
	}
	if cfg.RetryScanInterval != 5*time.Second {
		t.Errorf("RetryScanInterval = %s, want 5s", cfg.RetryScanInterval)
	}
	if cfg.RetryScanLimit != 100 {
		t.Errorf("RetryScanLimit = %d, want 100", cfg.RetryScanLimit)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("API_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("RATE_LIMIT_PER_SEC", "250")
	t.Setenv("WORKER_CONCURRENCY", "8")
	t.Setenv("RETRY_SCAN_INTERVAL", "2s")
	t.Setenv("RETRY_SCAN_LIMIT", "250")

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
	if cfg.WorkerConcurrency != 8 {
		t.Errorf("WorkerConcurrency = %d, want 8", cfg.WorkerConcurrency)
	}
	if cfg.RetryScanInterval != 2*time.Second {
		t.Errorf("RetryScanInterval = %s, want 2s", cfg.RetryScanInterval)
	}
	if cfg.RetryScanLimit != 250 {
		t.Errorf("RetryScanLimit = %d, want 250", cfg.RetryScanLimit)
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
