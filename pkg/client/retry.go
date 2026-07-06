package client

import (
	"context"
	"log"
	"math"
	"time"
)

// RetryConfig configures the retry behavior.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   10 * time.Second,
	}
}

// Retry executes the given function with exponential backoff on error.
// Returns the last error if all attempts fail.
func (cfg RetryConfig) Retry(ctx context.Context, fn func() error) error {
	var lastErr error
	for i := 0; i < cfg.MaxRetries; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if i < cfg.MaxRetries-1 {
			delay := time.Duration(math.Min(float64(cfg.BaseDelay)*math.Pow(2, float64(i)), float64(cfg.MaxDelay)))
			log.Printf("[retry] attempt %d/%d failed: %v, retrying in %v", i+1, cfg.MaxRetries, lastErr, delay)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return lastErr
}
