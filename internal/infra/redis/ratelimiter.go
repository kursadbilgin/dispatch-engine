package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	defaultLimitPerSec int64 = 100
	backoffStep              = 10 * time.Millisecond
	backoffMax               = 50 * time.Millisecond
	windowSeconds            = 1
)

// RedisRateLimiter is a distributed per-second rate limiter backed by Redis.
type RedisRateLimiter struct {
	client      *goredis.Client
	limitPerSec int64
	now         func() time.Time
	sleep       func(ctx context.Context, d time.Duration) error
}

func NewRedisRateLimiter(client *goredis.Client, limitPerSec int) (*RedisRateLimiter, error) {
	return newRedisRateLimiter(
		client,
		int64(limitPerSec),
		time.Now,
		sleepWithContext,
	)
}

func newRedisRateLimiter(
	client *goredis.Client,
	limitPerSec int64,
	nowFn func() time.Time,
	sleepFn func(ctx context.Context, d time.Duration) error,
) (*RedisRateLimiter, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if limitPerSec <= 0 {
		limitPerSec = defaultLimitPerSec
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	if sleepFn == nil {
		sleepFn = sleepWithContext
	}

	return &RedisRateLimiter{
		client:      client,
		limitPerSec: limitPerSec,
		now:         nowFn,
		sleep:       sleepFn,
	}, nil
}

func (r *RedisRateLimiter) Allow(ctx context.Context, channel string) (bool, error) {
	if r == nil || r.client == nil {
		return false, fmt.Errorf("rate limiter is not initialized")
	}

	normalizedChannel := strings.ToLower(strings.TrimSpace(channel))
	if normalizedChannel == "" {
		return false, fmt.Errorf("channel is required")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	key := fmt.Sprintf("ratelimit:%s:%d", normalizedChannel, r.now().UTC().Unix())
	window := time.Duration(windowSeconds) * time.Second

	var incrCmd *goredis.IntCmd
	_, err := r.client.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		incrCmd = pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, window)
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to evaluate rate limit: %w", err)
	}
	if incrCmd == nil {
		return false, fmt.Errorf("failed to evaluate rate limit: missing INCR result")
	}

	return incrCmd.Val() <= r.limitPerSec, nil
}

func (r *RedisRateLimiter) Wait(ctx context.Context, channel string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	backoff := backoffStep
	for {
		allowed, err := r.Allow(ctx, channel)
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}

		if err := r.sleep(ctx, backoff); err != nil {
			return err
		}

		backoff += backoffStep
		if backoff > backoffMax {
			backoff = backoffMax
		}
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
