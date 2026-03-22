package helpers

import (
	"context"
	"fmt"
	"time"
)

type Condition func() bool

func Wait(ctx context.Context, condition Condition, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

func WaitFor(ctx context.Context, condition Condition, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return Wait(ctx, condition, 10*time.Millisecond)
}

func WaitOrFail(condition Condition, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return Wait(ctx, condition, 10*time.Millisecond)
}

type EventCondition func() (interface{}, bool)

func WaitForEvent(ctx context.Context, condition EventCondition, timeout time.Duration) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for event")
		case <-ticker.C:
			if val, ok := condition(); ok {
				return val, nil
			}
		}
	}
}

type ErrorCondition func() error

func WaitForNoError(ctx context.Context, fn ErrorCondition, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("timeout, last error: %w", lastErr)
			}
			return ctx.Err()
		case <-ticker.C:
			if err := fn(); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
	}
}

func Retry(fn func() error, maxAttempts int, interval time.Duration) error {
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(interval)
		}
	}
	return fmt.Errorf("retry failed after %d attempts: %w", maxAttempts, lastErr)
}

func Eventually(condition Condition, waitFor time.Duration, tick time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), waitFor)
	defer cancel()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if condition() {
				return true
			}
		}
	}
}

func Consistently(condition Condition, duration time.Duration, tick time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return true
		case <-ticker.C:
			if !condition() {
				return false
			}
		}
	}
}
