package observability

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsWorkerCollectors(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()

	metrics.IncNotificationSent("SMS")
	metrics.IncNotificationFailed("sms", "permanent_error")
	metrics.ObserveNotificationSendDuration("sms", 120*time.Millisecond)
	metrics.IncWorkerInFlight("sms")
	metrics.DecWorkerInFlight("sms")
	metrics.IncRetryScheduled("sms")

	if got := testutil.ToFloat64(metrics.notificationsSentTotal.WithLabelValues("sms")); got != 1 {
		t.Fatalf("notifications_sent_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.notificationsFailedTotal.WithLabelValues("sms", "permanent_error")); got != 1 {
		t.Fatalf("notifications_failed_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.retryScheduledTotal.WithLabelValues("sms")); got != 1 {
		t.Fatalf("retry_scheduled_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.workerInflight.WithLabelValues("sms")); got != 0 {
		t.Fatalf("worker_inflight = %v, want 0", got)
	}
}

func TestMetricsHTTPMiddlewareRecordsRequest(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()
	app := fiber.New()
	app.Use(metrics.HTTPMiddleware())
	app.Get("/livez", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/livez", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if got := testutil.ToFloat64(metrics.httpRequestsTotal.WithLabelValues("GET", "/livez", "200")); got != 1 {
		t.Fatalf("http_requests_total = %v, want 1", got)
	}
}

func TestMetricsHTTPMiddlewareRecordsErrorStatus(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()
	app := fiber.New()
	app.Use(metrics.HTTPMiddleware())
	app.Get("/boom", func(c *fiber.Ctx) error {
		return errors.New("boom")
	})

	req := httptest.NewRequest("GET", "/boom", nil)
	_, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}

	if got := testutil.ToFloat64(metrics.httpRequestsTotal.WithLabelValues("GET", "/boom", "500")); got != 1 {
		t.Fatalf("http_requests_total = %v, want 1", got)
	}
}
