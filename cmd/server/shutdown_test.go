package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdownBudget_DrainPlusFlushFitsTheTotal(t *testing.T) {
	t.Parallel()
	if flushReserve <= 0 || drainTimeout <= 0 {
		t.Fatalf("got drain %v and flush %v, want both positive", drainTimeout, flushReserve)
	}
	if drainTimeout+flushReserve > shutdownTimeout {
		t.Errorf("got drain %v + flush %v, want at most %v", drainTimeout, flushReserve, shutdownTimeout)
	}
}

// The flush must get its own live deadline even after the drain used up its
// timeout, or the providers shut down without exporting.
func TestFlushTelemetry_GetsAFreshContextOfTheReserve(t *testing.T) {
	t.Parallel()
	var (
		deadline time.Time
		expired  bool
	)
	start := time.Now()
	err := flushTelemetry(func(ctx context.Context) error {
		deadline, _ = ctx.Deadline()
		expired = ctx.Err() != nil
		return nil
	})
	end := time.Now()
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	if expired {
		t.Error("got an expired flush context, want a live one")
	}
	if deadline.Before(start.Add(flushReserve)) || deadline.After(end.Add(flushReserve)) {
		t.Errorf("got flush deadline %v after start, want %v from the call", deadline.Sub(start), flushReserve)
	}
}

func TestFlushTelemetry_WrapsTheProviderError(t *testing.T) {
	t.Parallel()
	errExport := errors.New("export failed")
	err := flushTelemetry(func(context.Context) error { return errExport })
	if !errors.Is(err, errExport) {
		t.Errorf("got %v, want it to wrap %v", err, errExport)
	}
}
