package observability

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type correlationIDKey struct{}

func NewLogger(level string) (*zap.Logger, error) {
	parsedLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(parsedLevel)
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.DisableStacktrace = true

	logger, err := cfg.Build(zap.AddCaller())
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}

func parseLevel(level string) (zapcore.Level, error) {
	var parsed zapcore.Level
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" {
		normalized = "info"
	}

	if err := parsed.UnmarshalText([]byte(normalized)); err != nil {
		return 0, fmt.Errorf("invalid log level %q: %w", level, err)
	}

	return parsed, nil
}

func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithValue(ctx, correlationIDKey{}, correlationID)
}

func CorrelationIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	correlationID, ok := ctx.Value(correlationIDKey{}).(string)
	if !ok || correlationID == "" {
		return "", false
	}

	return correlationID, true
}

func WithContextLogger(logger *zap.Logger, ctx context.Context) *zap.Logger {
	if logger == nil {
		return nil
	}

	correlationID, ok := CorrelationIDFromContext(ctx)
	if !ok {
		return logger
	}

	return logger.With(zap.String("correlationId", correlationID))
}
