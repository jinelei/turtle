package monitor_test

import (
	"context"
	"errors"
	"testing"

	"turtle/internal/monitor"
)

func TestRetrySucceedsAfterTransientFailures(t *testing.T) {
	tries := 0
	err := monitor.Retry(context.Background(), func() error {
		tries++
		if tries < 2 {
			return &monitor.Error{Kind: monitor.KindTimeout}
		}
		return nil
	})
	if err != nil || tries != 2 {
		t.Fatalf("err=%v tries=%d", err, tries)
	}
}

func TestRetryGivesUpAfterAttempts(t *testing.T) {
	tries := 0
	err := monitor.Retry(context.Background(), func() error {
		tries++
		return &monitor.Error{Kind: monitor.KindTimeout}
	})
	if err == nil || tries != 3 {
		t.Fatalf("err=%v tries=%d", err, tries)
	}
}

func TestRetryDoesNotRetryNonTransient(t *testing.T) {
	tries := 0
	want := &monitor.Error{Kind: monitor.KindUnsupportedCode}
	err := monitor.Retry(context.Background(), func() error {
		tries++
		return want
	})
	if !errors.Is(err, want) || tries != 1 {
		t.Fatalf("err=%v tries=%d", err, tries)
	}
}

func TestRetryRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tries := 0
	_ = monitor.Retry(ctx, func() error {
		tries++
		return &monitor.Error{Kind: monitor.KindTimeout}
	})
	if tries != 1 {
		t.Fatalf("expected no retries after cancel, got %d", tries)
	}
}
