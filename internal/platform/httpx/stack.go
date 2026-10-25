package httpx

import (
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

// Stack wraps mux in the production middleware chain. MaxBody is outermost
// so MaxBytesReader sees net/http's own ResponseWriter and a 413 closes the
// connection; Trace comes next so every later log line carries trace_id.
func Stack(mux http.Handler, logger *slog.Logger, tp trace.TracerProvider) http.Handler {
	return Chain(Routed(mux),
		MaxBody(MaxBodyBytes),
		Trace(tp),
		RequestID,
		AccessLog(logger),
		Recover(logger),
		Deadline(HandlerTimeout),
	)
}
