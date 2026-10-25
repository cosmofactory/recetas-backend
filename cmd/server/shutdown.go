package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	// shutdownTimeout is the whole graceful-shutdown budget: draining the
	// server, then flushing telemetry with a fresh context of flushReserve.
	shutdownTimeout = 5 * time.Second
	flushReserve    = time.Second
	drainTimeout    = shutdownTimeout - flushReserve
)

// drainer is satisfied by *http.Server.
type drainer interface {
	Shutdown(ctx context.Context) error
	Close() error
}

// shutdownSteps are what stops, in this order: the server, the telemetry
// flush, the pool. The pool closes last so draining requests can still
// query and their spans are exported before the process exits.
type shutdownSteps struct {
	server    drainer
	flush     func(context.Context) error
	closePool func()
	logger    *slog.Logger
}

// gracefulShutdown runs every step even when an earlier one fails, and
// joins their errors.
func gracefulShutdown(s shutdownSteps) error {
	err := drain(s.server, s.logger)
	if err != nil {
		s.logger.Error("server exited with error", "err", err)
	}
	return errors.Join(err, flushAndClosePool(s))
}

// abort logs err before the flush, so the reason the server stops is
// exported too.
func abort(s shutdownSteps, err error) error {
	s.logger.Error("server exited with error", "err", err)
	return errors.Join(err, flushAndClosePool(s))
}

func flushAndClosePool(s shutdownSteps) error {
	err := flushTelemetry(s.flush)
	if err != nil {
		s.logger.Error("flush telemetry failed", "err", err)
	}
	s.closePool()
	return err
}

func drain(srv drainer, logger *slog.Logger) error {
	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		// Closing connections cancels in-flight request contexts, so their
		// queries abort and closing the pool doesn't block.
		return fmt.Errorf("shutdown: %w", errors.Join(err, srv.Close()))
	}
	logger.Info("server stopped")
	return nil
}

// flushTelemetry gives the exporters a fresh context, so a drain that used
// up its own timeout can't leave them without one.
func flushTelemetry(flush func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), flushReserve)
	defer cancel()
	if err := flush(ctx); err != nil {
		return fmt.Errorf("flush telemetry: %w", err)
	}
	return nil
}
