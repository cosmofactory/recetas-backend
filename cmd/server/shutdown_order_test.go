package main

// The shutdown seam these tests pin:
//
//	type drainer interface { // satisfied by *http.Server
//	    Shutdown(ctx context.Context) error
//	    Close() error
//	}
//	type shutdownSteps struct {
//	    server    drainer
//	    flush     func(context.Context) error // telemetry.Providers.Shutdown
//	    closePool func()                      // (*pgxpool.Pool).Close
//	    logger    *slog.Logger
//	}
//	func gracefulShutdown(s shutdownSteps) error
//	    drains the server with a drainTimeout context (force-closing it when
//	    the drain fails), then flushes telemetry through flushTelemetry (a
//	    fresh flushReserve context), then closes the pool. Every step runs
//	    even when an earlier one fails; the result joins their errors.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"
)

const (
	stepDrain      = "drain"
	stepForceClose = "force close"
	stepFlush      = "flush"
	stepClosePool  = "close pool"
)

// shutdownRecorder fakes the server, the flusher and the pool, appending
// each call to steps so the test can check their order.
type shutdownRecorder struct {
	steps    []string
	drainErr error
	flushErr error

	drainDeadline time.Time
	drainReturned time.Time
	flushDeadline time.Time
	flushExpired  bool
}

func (r *shutdownRecorder) Shutdown(ctx context.Context) error {
	r.steps = append(r.steps, stepDrain)
	r.drainDeadline, _ = ctx.Deadline()
	r.drainReturned = time.Now()
	return r.drainErr
}

func (r *shutdownRecorder) Close() error {
	r.steps = append(r.steps, stepForceClose)
	return nil
}

func (r *shutdownRecorder) flush(ctx context.Context) error {
	r.steps = append(r.steps, stepFlush)
	r.flushDeadline, _ = ctx.Deadline()
	r.flushExpired = ctx.Err() != nil
	return r.flushErr
}

func (r *shutdownRecorder) closePool() {
	r.steps = append(r.steps, stepClosePool)
}

func (r *shutdownRecorder) shutdown() error {
	return gracefulShutdown(shutdownSteps{
		server:    r,
		flush:     r.flush,
		closePool: r.closePool,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

var (
	errDrain = errors.New("drain failed")
	errFlush = errors.New("export failed")
)

func TestGracefulShutdown_StepOrder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		drainErr  error
		flushErr  error
		wantSteps []string
		wantErrs  []error
	}{
		{
			name:      "clean drain",
			wantSteps: []string{stepDrain, stepFlush, stepClosePool},
		},
		{
			name:      "drain fails: force close, then still flush and close the pool",
			drainErr:  errDrain,
			wantSteps: []string{stepDrain, stepForceClose, stepFlush, stepClosePool},
			wantErrs:  []error{errDrain},
		},
		{
			name:      "drain times out: force close, then still flush and close the pool",
			drainErr:  context.DeadlineExceeded,
			wantSteps: []string{stepDrain, stepForceClose, stepFlush, stepClosePool},
			wantErrs:  []error{context.DeadlineExceeded},
		},
		{
			name:      "flush fails: the pool is still closed",
			flushErr:  errFlush,
			wantSteps: []string{stepDrain, stepFlush, stepClosePool},
			wantErrs:  []error{errFlush},
		},
		{
			name:      "both fail: every step runs and both errors are returned",
			drainErr:  errDrain,
			flushErr:  errFlush,
			wantSteps: []string{stepDrain, stepForceClose, stepFlush, stepClosePool},
			wantErrs:  []error{errDrain, errFlush},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &shutdownRecorder{drainErr: tt.drainErr, flushErr: tt.flushErr}

			err := rec.shutdown()

			if !slices.Equal(rec.steps, tt.wantSteps) {
				t.Errorf("got steps %q, want %q", rec.steps, tt.wantSteps)
			}
			assertJoinedErrors(t, err, tt.wantErrs)
		})
	}
}

func assertJoinedErrors(t *testing.T, err error, want []error) {
	t.Helper()
	if len(want) == 0 {
		if err != nil {
			t.Errorf("got error %v, want nil", err)
		}
		return
	}
	for _, w := range want {
		if !errors.Is(err, w) {
			t.Errorf("got error %v, want it to wrap %v", err, w)
		}
	}
}

func TestGracefulShutdown_DrainGetsTheDrainTimeout(t *testing.T) {
	t.Parallel()
	rec := &shutdownRecorder{}
	start := time.Now()

	if err := rec.shutdown(); err != nil {
		t.Fatalf("got error %v, want nil", err)
	}

	if rec.drainDeadline.IsZero() {
		t.Fatal("got a drain context without a deadline, want one of drainTimeout")
	}
	if rec.drainDeadline.Before(start.Add(drainTimeout)) || rec.drainDeadline.After(rec.drainReturned.Add(drainTimeout)) {
		t.Errorf("got drain deadline %v after start, want %v", rec.drainDeadline.Sub(start), drainTimeout)
	}
}

// Consistent with TestFlushTelemetry_GetsAFreshContextOfTheReserve: a drain
// that used up its timeout must not leave the flush without a budget.
func TestGracefulShutdown_FlushGetsItsOwnBudgetAfterAFailedDrain(t *testing.T) {
	t.Parallel()
	rec := &shutdownRecorder{drainErr: context.DeadlineExceeded}

	_ = rec.shutdown()
	end := time.Now()

	if rec.flushExpired {
		t.Error("got an expired flush context, want a live one")
	}
	if rec.flushDeadline.Before(rec.drainReturned.Add(flushReserve)) || rec.flushDeadline.After(end.Add(flushReserve)) {
		t.Errorf("got flush deadline %v after the drain, want %v from the flush",
			rec.flushDeadline.Sub(rec.drainReturned), flushReserve)
	}
}
