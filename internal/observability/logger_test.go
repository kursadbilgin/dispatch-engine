package observability

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLogger_LevelMapping(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		level        string
		debugEnabled bool
	}{
		{name: "debug level", level: "debug", debugEnabled: true},
		{name: "info level", level: "info", debugEnabled: false},
		{name: "empty level defaults to info", level: "", debugEnabled: false},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger, err := NewLogger(tc.level)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if logger == nil {
				t.Fatal("logger should not be nil")
			}

			if got := logger.Core().Enabled(zapcore.DebugLevel); got != tc.debugEnabled {
				t.Fatalf("debug enabled=%v, want=%v", got, tc.debugEnabled)
			}
		})
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	t.Parallel()

	logger, err := NewLogger("not-a-level")
	if err == nil {
		t.Fatal("expected error for invalid level")
	}
	if logger != nil {
		t.Fatal("expected nil logger for invalid level")
	}
}

func TestCorrelationID_ContextHelpers(t *testing.T) {
	t.Parallel()

	ctx := WithCorrelationID(context.Background(), "cid-123")
	correlationID, ok := CorrelationIDFromContext(ctx)
	if !ok {
		t.Fatal("expected correlation id to exist")
	}
	if correlationID != "cid-123" {
		t.Fatalf("correlation id=%q, want=%q", correlationID, "cid-123")
	}
}

func TestCorrelationID_ContextHelpersNilContext(t *testing.T) {
	t.Parallel()

	ctx := WithCorrelationID(context.TODO(), "cid-456")
	correlationID, ok := CorrelationIDFromContext(ctx)
	if !ok {
		t.Fatal("expected correlation id to exist")
	}
	if correlationID != "cid-456" {
		t.Fatalf("correlation id=%q, want=%q", correlationID, "cid-456")
	}
}

func TestCorrelationID_MissingValue(t *testing.T) {
	t.Parallel()

	_, ok := CorrelationIDFromContext(context.Background())
	if ok {
		t.Fatal("expected correlation id to be missing")
	}
}

func TestWithContextLogger(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.InfoLevel)
	baseLogger := zap.New(core)

	ctx := WithCorrelationID(context.Background(), "cid-789")
	loggerWithContext := WithContextLogger(baseLogger, ctx)
	loggerWithContext.Info("message with correlation")

	entries := recorded.All()
	if len(entries) != 1 {
		t.Fatalf("entries=%d, want=1", len(entries))
	}

	if got := entries[0].ContextMap()["correlationId"]; got != "cid-789" {
		t.Fatalf("correlationId=%v, want=%q", got, "cid-789")
	}
}

func TestWithContextLogger_NoCorrelationID(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.InfoLevel)
	baseLogger := zap.New(core)

	loggerWithContext := WithContextLogger(baseLogger, context.Background())
	loggerWithContext.Info("message without correlation")

	entries := recorded.All()
	if len(entries) != 1 {
		t.Fatalf("entries=%d, want=1", len(entries))
	}

	if _, ok := entries[0].ContextMap()["correlationId"]; ok {
		t.Fatal("expected correlationId field to be absent")
	}
}

func TestWithContextLogger_NilLogger(t *testing.T) {
	t.Parallel()

	if got := WithContextLogger(nil, context.Background()); got != nil {
		t.Fatal("expected nil logger")
	}
}
