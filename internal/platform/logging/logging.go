package logging

import (
	"io"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log"
)

// instrumentationScope names the logger the OTel bridge emits through.
const instrumentationScope = "github.com/cosmofactory/recetas-backend"

// New writes JSON lines to w, with trace_id and span_id when the context
// carries a valid span, and request_id when it carries one.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(newStdoutHandler(w, level))
}

// NewTee writes what New writes and also bridges every record to lp.
func NewTee(w io.Writer, level slog.Level, lp log.LoggerProvider) *slog.Logger {
	bridge := otelslog.NewHandler(instrumentationScope, otelslog.WithLoggerProvider(lp))
	return slog.New(&teeHandler{level: level, handlers: []slog.Handler{newStdoutHandler(w, level), bridge}})
}

func newStdoutHandler(w io.Writer, level slog.Level) slog.Handler {
	return contextHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})}
}
