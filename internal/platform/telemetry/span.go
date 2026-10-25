package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RecordError records err as an exception on the span in ctx and marks the
// span as failed. It is a no-op for a nil err or without a recording span.
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	if err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.RecordError(err, opts...)
	// The exception event carries the message; otelhttp overwrites the status
	// description on a 5xx anyway.
	span.SetStatus(codes.Error, "")
}
