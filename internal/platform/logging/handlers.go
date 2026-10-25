package logging

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

const (
	traceIDKey   = "trace_id"
	spanIDKey    = "span_id"
	requestIDKey = "request_id"
)

// contextHandler adds the span and request ID in context to each record, so
// stdout lines can be matched to traces and to one request.
type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String(traceIDKey, sc.TraceID().String()), slog.String(spanIDKey, sc.SpanID().String()))
	}
	if id := RequestIDFrom(ctx); id != "" {
		r.AddAttrs(slog.String(requestIDKey, id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

// teeHandler sends each record at or above level to every handler. The
// level is checked here because the OTel bridge accepts every level.
type teeHandler struct {
	level    slog.Level
	handlers []slog.Handler
}

func (h *teeHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, child := range h.handlers {
		if child.Enabled(ctx, r.Level) {
			errs = append(errs, child.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.derive(func(child slog.Handler) slog.Handler { return child.WithAttrs(attrs) })
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return h.derive(func(child slog.Handler) slog.Handler { return child.WithGroup(name) })
}

func (h *teeHandler) derive(apply func(slog.Handler) slog.Handler) slog.Handler {
	children := make([]slog.Handler, len(h.handlers))
	for i, child := range h.handlers {
		children[i] = apply(child)
	}
	return &teeHandler{level: h.level, handlers: children}
}
