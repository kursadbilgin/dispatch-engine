package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics stores Prometheus collectors used by API and worker flows.
type Metrics struct {
	registry *prometheus.Registry

	httpRequestsTotal        *prometheus.CounterVec
	httpRequestDuration      *prometheus.HistogramVec
	notificationsSentTotal   *prometheus.CounterVec
	notificationsFailedTotal *prometheus.CounterVec
	notificationSendDuration *prometheus.HistogramVec
	workerInflight           *prometheus.GaugeVec
	retryScheduledTotal      *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry: registry,
		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "dispatch_engine",
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests processed by method, path, and status.",
			},
			[]string{"method", "path", "status"},
		),
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "dispatch_engine",
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds by method and path.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		notificationsSentTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "dispatch_engine",
				Name:      "notifications_sent_total",
				Help:      "Total number of notifications sent successfully.",
			},
			[]string{"channel"},
		),
		notificationsFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "dispatch_engine",
				Name:      "notifications_failed_total",
				Help:      "Total number of notifications that ended in failed state.",
			},
			[]string{"channel", "reason"},
		),
		notificationSendDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "dispatch_engine",
				Name:      "notification_send_duration_seconds",
				Help:      "Provider send duration in seconds grouped by channel.",
				Buckets:   prometheus.ExponentialBuckets(0.01, 2, 12),
			},
			[]string{"channel"},
		),
		workerInflight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "dispatch_engine",
				Name:      "worker_inflight",
				Help:      "Current number of in-flight worker operations grouped by channel.",
			},
			[]string{"channel"},
		),
		retryScheduledTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "dispatch_engine",
				Name:      "retry_scheduled_total",
				Help:      "Total number of notifications scheduled for retry.",
			},
			[]string{"channel"},
		),
	}

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.httpRequestsTotal,
		m.httpRequestDuration,
		m.notificationsSentTotal,
		m.notificationsFailedTotal,
		m.notificationSendDuration,
		m.workerInflight,
		m.retryScheduledTotal,
	)

	return m
}

func (m *Metrics) Handler() http.Handler {
	if m == nil || m.registry == nil {
		return promhttp.Handler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) HTTPMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		path := routePath(c)
		// Avoid self-scrape noise for request counters.
		if path == "/metrics" {
			return err
		}

		m.recordHTTPRequest(c.Method(), path, statusFromResult(c, err), time.Since(start))
		return err
	}
}

func (m *Metrics) IncNotificationSent(channel string) {
	if m == nil {
		return
	}
	m.notificationsSentTotal.WithLabelValues(normalizeChannel(channel)).Inc()
}

func (m *Metrics) IncNotificationFailed(channel string, reason string) {
	if m == nil {
		return
	}
	reasonLabel := strings.TrimSpace(strings.ToLower(reason))
	if reasonLabel == "" {
		reasonLabel = "unknown"
	}
	m.notificationsFailedTotal.WithLabelValues(normalizeChannel(channel), reasonLabel).Inc()
}

func (m *Metrics) ObserveNotificationSendDuration(channel string, duration time.Duration) {
	if m == nil {
		return
	}
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	m.notificationSendDuration.WithLabelValues(normalizeChannel(channel)).Observe(seconds)
}

func (m *Metrics) IncWorkerInFlight(channel string) {
	if m == nil {
		return
	}
	m.workerInflight.WithLabelValues(normalizeChannel(channel)).Inc()
}

func (m *Metrics) DecWorkerInFlight(channel string) {
	if m == nil {
		return
	}
	m.workerInflight.WithLabelValues(normalizeChannel(channel)).Dec()
}

func (m *Metrics) IncRetryScheduled(channel string) {
	if m == nil {
		return
	}
	m.retryScheduledTotal.WithLabelValues(normalizeChannel(channel)).Inc()
}

func (m *Metrics) recordHTTPRequest(method string, path string, status int, duration time.Duration) {
	if m == nil {
		return
	}

	methodLabel := strings.ToUpper(strings.TrimSpace(method))
	if methodLabel == "" {
		methodLabel = "UNKNOWN"
	}
	pathLabel := strings.TrimSpace(path)
	if pathLabel == "" {
		pathLabel = "unmatched"
	}

	m.httpRequestsTotal.WithLabelValues(methodLabel, pathLabel, strconv.Itoa(status)).Inc()
	m.httpRequestDuration.WithLabelValues(methodLabel, pathLabel).Observe(duration.Seconds())
}

func routePath(c *fiber.Ctx) string {
	if c == nil {
		return "unmatched"
	}

	if route := c.Route(); route != nil {
		if path := strings.TrimSpace(route.Path); path != "" {
			return path
		}
	}
	return "unmatched"
}

func statusFromResult(c *fiber.Ctx, err error) int {
	if err != nil {
		if fiberErr, ok := err.(*fiber.Error); ok {
			return fiberErr.Code
		}
		return fiber.StatusInternalServerError
	}

	if c == nil {
		return fiber.StatusOK
	}

	status := c.Response().StatusCode()
	if status == 0 {
		return fiber.StatusOK
	}
	return status
}

func normalizeChannel(channel string) string {
	normalized := strings.ToLower(strings.TrimSpace(channel))
	if normalized == "" {
		return "unknown"
	}
	return normalized
}
