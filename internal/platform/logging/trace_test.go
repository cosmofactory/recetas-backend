package logging_test

// The production API these tests pin:
//
//	func New(w io.Writer, level slog.Level) *slog.Logger
//	    unchanged JSON format; adds top-level trace_id and span_id (lowercase
//	    hex, as SpanContext.TraceID().String()) when ctx carries a valid span.
//	func NewTee(w io.Writer, level slog.Level, lp log.LoggerProvider) *slog.Logger
//	    New's stdout output plus the otelslog bridge to lp, at the same level.

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
	"github.com/cosmofactory/recetas-backend/internal/testutil/spantest"
)

// memoryLogExporter keeps every record the bridge exports.
type memoryLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *memoryLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range records {
		e.records = append(e.records, r.Clone())
	}
	return nil
}

func (e *memoryLogExporter) Shutdown(context.Context) error   { return nil }
func (e *memoryLogExporter) ForceFlush(context.Context) error { return nil }

func (e *memoryLogExporter) all() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

func newBridgeRecorder(t *testing.T) (*sdklog.LoggerProvider, *memoryLogExporter) {
	t.Helper()
	exp := &memoryLogExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })
	return lp, exp
}

func spanContext(t *testing.T) (context.Context, trace.SpanContext) {
	t.Helper()
	tp, _ := spantest.New(t)
	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	t.Cleanup(func() { span.End() })
	return ctx, span.SpanContext()
}

func singleLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	recs := decodeLines(t, buf)
	if len(recs) != 1 {
		t.Fatalf("got %d log lines, want 1 (output=%q)", len(recs), buf.String())
	}
	return recs[0]
}

func assertTraceIDs(t *testing.T, rec map[string]any, sc trace.SpanContext) {
	t.Helper()
	if got := rec["trace_id"]; got != sc.TraceID().String() {
		t.Errorf("got trace_id %v, want %q", got, sc.TraceID().String())
	}
	if got := rec["span_id"]; got != sc.SpanID().String() {
		t.Errorf("got span_id %v, want %q", got, sc.SpanID().String())
	}
}

func assertNoTraceIDs(t *testing.T, rec map[string]any) {
	t.Helper()
	for _, k := range []string{"trace_id", "span_id"} {
		if v, ok := rec[k]; ok {
			t.Errorf("got %s=%v, want no %s without a valid span", k, v, k)
		}
	}
}

func TestNew_AddsTraceAndSpanIDWhenSpanInContext(t *testing.T) {
	t.Parallel()
	ctx, sc := spanContext(t)
	tests := []struct {
		name string
		log  func(*slog.Logger)
	}{
		{name: "InfoContext", log: func(l *slog.Logger) { l.InfoContext(ctx, "m", "k", "v") }},
		{name: "ErrorContext", log: func(l *slog.Logger) { l.ErrorContext(ctx, "m", "k", "v") }},
		{name: "With attrs", log: func(l *slog.Logger) { l.With("k", "v").InfoContext(ctx, "m") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			tt.log(logging.New(&buf, slog.LevelDebug))

			rec := singleLine(t, &buf)
			assertTraceIDs(t, rec, sc)
			if rec["k"] != "v" || rec["msg"] != "m" {
				t.Errorf("got record %v, want msg %q and k=%q kept", rec, "m", "v")
			}
		})
	}
}

func TestNew_NoTraceIDsWithoutValidSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{name: "background", ctx: context.Background()},
		{name: "non-recording span", ctx: trace.ContextWithSpan(context.Background(), trace.SpanFromContext(context.Background()))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			logging.New(&buf, slog.LevelDebug).InfoContext(tt.ctx, "m")
			assertNoTraceIDs(t, singleLine(t, &buf))
		})
	}
}

func TestNewTee_WritesStdoutAndBridgesToOTel(t *testing.T) {
	t.Parallel()
	ctx, sc := spanContext(t)
	lp, exp := newBridgeRecorder(t)
	var buf bytes.Buffer

	logging.NewTee(&buf, slog.LevelInfo, lp).WarnContext(ctx, "dish lookup slow", "dish", "tortilla")

	rec := singleLine(t, &buf)
	if rec["level"] != "WARN" || rec["msg"] != "dish lookup slow" || rec["dish"] != "tortilla" {
		t.Errorf("got stdout record %v, want the unchanged JSON shape", rec)
	}
	assertTraceIDs(t, rec, sc)

	bridged := exp.all()
	if len(bridged) != 1 {
		t.Fatalf("got %d bridged records, want 1", len(bridged))
	}
	assertBridgedRecord(t, bridged[0], sc)
}

func assertBridgedRecord(t *testing.T, r sdklog.Record, sc trace.SpanContext) {
	t.Helper()
	if got := r.Body().AsString(); got != "dish lookup slow" {
		t.Errorf("got bridged body %q, want %q", got, "dish lookup slow")
	}
	if got := r.TraceID(); got != sc.TraceID() {
		t.Errorf("got bridged trace ID %v, want %v", got, sc.TraceID())
	}
	if got := r.SpanID(); got != sc.SpanID() {
		t.Errorf("got bridged span ID %v, want %v", got, sc.SpanID())
	}
	if got := bridgedAttr(r, "dish"); got != "tortilla" {
		t.Errorf("got bridged attr dish=%q, want %q", got, "tortilla")
	}
}

func bridgedAttr(r sdklog.Record, key string) string {
	var out string
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		if string(kv.Key) == key {
			out = kv.Value.String()
			return false
		}
		return true
	})
	return out
}

func TestNewTee_DropsRecordsBelowLevelOnBothSides(t *testing.T) {
	t.Parallel()
	lp, exp := newBridgeRecorder(t)
	var buf bytes.Buffer

	logging.NewTee(&buf, slog.LevelInfo, lp).Debug("probe ok")

	if buf.Len() != 0 {
		t.Errorf("got stdout %q, want nothing below INFO", buf.String())
	}
	if n := len(exp.all()); n != 0 {
		t.Errorf("got %d bridged records, want 0 below INFO", n)
	}
}

func TestNew_AddsRequestIDFromContext(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ctx := logging.WithRequestID(context.Background(), "req-42")
	logging.New(&buf, slog.LevelDebug).ErrorContext(ctx, "m")

	rec := singleLine(t, &buf)
	if rec["request_id"] != "req-42" {
		t.Errorf("got request_id %v, want %q", rec["request_id"], "req-42")
	}
	if got := logging.RequestIDFrom(context.Background()); got != "" {
		t.Errorf("got RequestIDFrom(background) %q, want empty", got)
	}
}
