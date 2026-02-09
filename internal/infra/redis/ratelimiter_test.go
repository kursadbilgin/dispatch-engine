package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestRedisRateLimiterAllow(t *testing.T) {
	t.Parallel()

	rdb := newTestRedisClient(t)

	now := time.Unix(1_700_000_000, 0)
	limiter, err := newRedisRateLimiter(
		rdb,
		2,
		func() time.Time { return now },
		sleepWithContext,
	)
	if err != nil {
		t.Fatalf("newRedisRateLimiter() error = %v", err)
	}

	allowed, err := limiter.Allow(context.Background(), "sms")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !allowed {
		t.Fatal("first call should be allowed")
	}

	allowed, err = limiter.Allow(context.Background(), "sms")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !allowed {
		t.Fatal("second call should be allowed")
	}

	allowed, err = limiter.Allow(context.Background(), "sms")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if allowed {
		t.Fatal("third call should be rejected by rate limit")
	}

	now = now.Add(time.Second)
	allowed, err = limiter.Allow(context.Background(), "sms")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !allowed {
		t.Fatal("new second window should allow call")
	}
}

func TestRedisRateLimiterAllowPerChannel(t *testing.T) {
	t.Parallel()

	rdb := newTestRedisClient(t)

	now := time.Unix(1_700_000_100, 0)
	limiter, err := newRedisRateLimiter(
		rdb,
		1,
		func() time.Time { return now },
		sleepWithContext,
	)
	if err != nil {
		t.Fatalf("newRedisRateLimiter() error = %v", err)
	}

	allowed, err := limiter.Allow(context.Background(), "sms")
	if err != nil {
		t.Fatalf("Allow(sms) error = %v", err)
	}
	if !allowed {
		t.Fatal("sms should be allowed on first request")
	}

	allowed, err = limiter.Allow(context.Background(), "email")
	if err != nil {
		t.Fatalf("Allow(email) error = %v", err)
	}
	if !allowed {
		t.Fatal("email should be allowed on first request")
	}

	allowed, err = limiter.Allow(context.Background(), "sms")
	if err != nil {
		t.Fatalf("Allow(sms) error = %v", err)
	}
	if allowed {
		t.Fatal("sms second request should be rejected")
	}
}

func TestRedisRateLimiterWait(t *testing.T) {
	t.Parallel()

	rdb := newTestRedisClient(t)

	now := time.Unix(1_700_000_200, 0)
	sleepCalls := 0
	limiter, err := newRedisRateLimiter(
		rdb,
		1,
		func() time.Time { return now },
		func(ctx context.Context, d time.Duration) error {
			sleepCalls++
			if sleepCalls == 1 {
				now = now.Add(time.Second)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("newRedisRateLimiter() error = %v", err)
	}

	allowed, err := limiter.Allow(context.Background(), "push")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !allowed {
		t.Fatal("expected first call to be allowed")
	}

	if err := limiter.Wait(context.Background(), "push"); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if sleepCalls == 0 {
		t.Fatal("expected Wait() to sleep at least once")
	}
}

func TestRedisRateLimiterWaitContextDeadline(t *testing.T) {
	t.Parallel()

	rdb := newTestRedisClient(t)

	now := time.Unix(1_700_000_300, 0)
	limiter, err := newRedisRateLimiter(
		rdb,
		1,
		func() time.Time { return now },
		sleepWithContext,
	)
	if err != nil {
		t.Fatalf("newRedisRateLimiter() error = %v", err)
	}

	allowed, err := limiter.Allow(context.Background(), "sms")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !allowed {
		t.Fatal("expected first call to be allowed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err = limiter.Wait(ctx, "sms")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func newTestRedisClient(t *testing.T) *goredis.Client {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	return rdb
}
