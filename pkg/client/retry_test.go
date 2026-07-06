package client

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryConfig_AllSuccess(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: time.Second}
	calls := 0
	err := cfg.Retry(context.Background(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryConfig_RetriesAndSucceeds(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: time.Second}
	calls := 0
	err := cfg.Retry(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("transient error")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryConfig_ExhaustsRetries(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: time.Second}
	calls := 0
	expectedErr := errors.New("persistent error")
	err := cfg.Retry(context.Background(), func() error {
		calls++
		return expectedErr
	})
	if err != expectedErr {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryConfig_ContextCancelled(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 5, BaseDelay: 50 * time.Millisecond, MaxDelay: time.Second}
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after first attempt
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	calls := 0
	err := cfg.Retry(ctx, func() error {
		calls++
		return errors.New("fail")
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Fatalf("expected 3 retries, got %d", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 500*time.Millisecond {
		t.Fatalf("expected 500ms base, got %v", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 10*time.Second {
		t.Fatalf("expected 10s max, got %v", cfg.MaxDelay)
	}
}
