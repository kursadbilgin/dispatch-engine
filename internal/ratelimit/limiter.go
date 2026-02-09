package ratelimit

import "context"

// RateLimiter controls message throughput per channel.
type RateLimiter interface {
	Allow(ctx context.Context, channel string) (bool, error)
	Wait(ctx context.Context, channel string) error
}
