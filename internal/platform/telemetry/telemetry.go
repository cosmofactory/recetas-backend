package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/log"
	lognoop "go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Providers are the trace and log providers to inject. Shutdown flushes the
// batch processors; call it once, after the HTTP server has stopped.
type Providers struct {
	TracerProvider trace.TracerProvider
	LoggerProvider log.LoggerProvider
	Shutdown       func(context.Context) error
}

type options struct {
	httpClient *http.Client
}

type Option func(*options)

// WithHTTPClient sets the HTTP client both exporters use.
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) { o.httpClient = c }
}

// Setup returns no-op providers when cfg is disabled, without touching the
// network. Enabled, it exports through batch processors, so a slow or
// unreachable endpoint never blocks the caller.
func Setup(ctx context.Context, cfg Config, opts ...Option) (Providers, error) {
	if !cfg.Enabled() {
		return noopProviders(), nil
	}
	if err := ValidateEndpoint(cfg.Endpoint); err != nil {
		return Providers{}, fmt.Errorf("telemetry endpoint: %w", err)
	}
	ex := newExport(cfg, opts)
	tp, err := newTracerProvider(ctx, ex)
	if err != nil {
		return Providers{}, err
	}
	lp, err := newLoggerProvider(ctx, ex)
	if err != nil {
		return Providers{}, errors.Join(err, tp.Shutdown(ctx))
	}
	shutdown := func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), lp.Shutdown(ctx))
	}
	return Providers{TracerProvider: tp, LoggerProvider: lp, Shutdown: shutdown}, nil
}

func noopProviders() Providers {
	return Providers{
		TracerProvider: tracenoop.NewTracerProvider(),
		LoggerProvider: lognoop.NewLoggerProvider(),
		Shutdown:       func(context.Context) error { return nil },
	}
}
