// Package spantest records OpenTelemetry spans in memory for tests.
//
// It is imported only by _test.go files; production code receives the
// returned provider wherever it accepts a trace.TracerProvider.
package spantest

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// exceptionEvent is the event name span.RecordError writes.
const exceptionEvent = "exception"

// New returns a provider that samples every span and records it when it ends.
func New(t testing.TB) (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp, recorder
}

// StartServerSpan starts a server span on tp, standing in for the HTTP
// tracing middleware in tests of inner layers. End it before inspecting.
func StartServerSpan(ctx context.Context, tp trace.TracerProvider) (context.Context, trace.Span) {
	return tp.Tracer("spantest").Start(ctx, "test server span", trace.WithSpanKind(trace.SpanKindServer))
}

// OfKind returns the ended spans of the given kind.
func OfKind(rec *tracetest.SpanRecorder, kind trace.SpanKind) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.SpanKind() == kind {
			out = append(out, s)
		}
	}
	return out
}

// SingleServer fails unless exactly one server span ended, and returns it.
func SingleServer(t testing.TB, rec *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	spans := OfKind(rec, trace.SpanKindServer)
	if len(spans) != 1 {
		t.Fatalf("got %d server spans %v, want 1", len(spans), Names(rec.Ended()))
	}
	return spans[0]
}

// Named returns the first span called name.
func Named(spans []sdktrace.ReadOnlySpan, name string) (sdktrace.ReadOnlySpan, bool) {
	for _, s := range spans {
		if s.Name() == name {
			return s, true
		}
	}
	return nil, false
}

// Names lists span names, for failure messages.
func Names(spans []sdktrace.ReadOnlySpan) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.Name())
	}
	return out
}

// Attr returns the value of the attribute key on s.
func Attr(s sdktrace.ReadOnlySpan, key string) (attribute.Value, bool) {
	return find(s.Attributes(), key)
}

// AttrsContaining lists "key=value" for every attribute of every span whose
// key or value contains needle. Event attributes are included.
func AttrsContaining(spans []sdktrace.ReadOnlySpan, needle string) []string {
	var out []string
	for _, s := range spans {
		for _, kv := range allAttributes(s) {
			pair := string(kv.Key) + "=" + kv.Value.String()
			if strings.Contains(pair, needle) {
				out = append(out, s.Name()+": "+pair)
			}
		}
	}
	return out
}

// AssertErrorWithException fails unless s has status Error and an exception
// event whose message contains wantInMessage.
func AssertErrorWithException(t testing.TB, s sdktrace.ReadOnlySpan, wantInMessage string) {
	t.Helper()
	if got := s.Status().Code; got != codes.Error {
		t.Errorf("got span %q status %v, want %v", s.Name(), got, codes.Error)
	}
	msg, ok := ExceptionMessage(s)
	if !ok {
		t.Fatalf("got span %q events %v, want an %q event", s.Name(), eventNames(s), exceptionEvent)
	}
	if !strings.Contains(msg, wantInMessage) {
		t.Errorf("got exception.message %q, want it to contain %q", msg, wantInMessage)
	}
}

// AssertNoError fails if s has status Error or an exception event.
func AssertNoError(t testing.TB, s sdktrace.ReadOnlySpan) {
	t.Helper()
	if got := s.Status().Code; got == codes.Error {
		t.Errorf("got span %q status %v, want it not to be an error", s.Name(), got)
	}
	if _, ok := ExceptionMessage(s); ok {
		t.Errorf("got an %q event on span %q, want none", exceptionEvent, s.Name())
	}
}

// ExceptionMessage returns exception.message of the first exception event.
func ExceptionMessage(s sdktrace.ReadOnlySpan) (string, bool) {
	for _, e := range s.Events() {
		if e.Name != exceptionEvent {
			continue
		}
		v, _ := find(e.Attributes, "exception.message")
		return v.String(), true
	}
	return "", false
}

func allAttributes(s sdktrace.ReadOnlySpan) []attribute.KeyValue {
	out := append([]attribute.KeyValue(nil), s.Attributes()...)
	for _, e := range s.Events() {
		out = append(out, e.Attributes...)
	}
	return out
}

func find(attrs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, kv := range attrs {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func eventNames(s sdktrace.ReadOnlySpan) []string {
	out := make([]string, 0, len(s.Events()))
	for _, e := range s.Events() {
		out = append(out, e.Name)
	}
	return out
}
