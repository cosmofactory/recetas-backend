package httpx_test

// Tests for the production middleware chain. The API they pin:
//
//	func Stack(mux http.Handler, logger *slog.Logger, tp trace.TracerProvider) http.Handler
//
// Stack wraps mux, outermost first, in:
//
//	MaxBody(MaxBodyBytes) → tracing (otelhttp, server span on tp) → request ID
//	→ AccessLog → Recover → Deadline(HandlerTimeout) → Routed(mux)
//
// MaxBody stays outermost (a 413 must close the connection). Tracing uses tp
// and the W3C trace-context propagator explicitly, never the OTel globals.
// Server spans are named "<METHOD> <route pattern>" and carry http.route;
// /healthz gets no span. The request ID (X-Request-ID, honoured when it is
// 1–64 chars of [A-Za-z0-9._-], else 32 lowercase hex from crypto/rand) is
// echoed in the response, set as span attribute "request_id" and logged as
// "request_id" in the access line. A 2xx /healthz is access-logged at DEBUG.

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
	"github.com/cosmofactory/recetas-backend/internal/testutil/spantest"
)

const (
	requestIDHeader = "X-Request-ID"
	requestIDAttr   = "request_id"
	stackPanicText  = "kaboom in handler"
	healthzPath     = "/healthz"
)

var generatedRequestID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// stackServer is Stack over a mux of representative routes.
type stackServer struct {
	h     http.Handler
	spans *tracetest.SpanRecorder
	logs  *bytes.Buffer
}

func newStack(t *testing.T, level slog.Level) stackServer {
	t.Helper()
	return newStackWithHealth(t, level, http.StatusOK)
}

func newStackWithHealth(t *testing.T, level slog.Level, healthStatus int) stackServer {
	t.Helper()
	tp, spans := spantest.New(t)
	var logs bytes.Buffer
	logger := logging.New(&logs, level)
	return stackServer{h: httpx.Stack(routes(healthStatus), logger, tp), spans: spans, logs: &logs}
}

func routes(healthStatus int) *http.ServeMux {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{}`) }
	mux.HandleFunc("GET /v1/regions", ok)
	mux.HandleFunc("GET /v1/dishes/{dishKey}", ok)
	mux.HandleFunc("GET /v1/dishes/{dishKey}/variations/{regionKey}/recipe", ok)
	mux.HandleFunc("GET "+healthzPath, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(healthStatus) })
	mux.HandleFunc("GET /fail", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	})
	mux.HandleFunc("GET /missing", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
	})
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic(stackPanicText) })
	mux.HandleFunc("POST /dishes", readBodyOr413)
	return mux
}

func readBodyOr413(w http.ResponseWriter, r *http.Request) {
	_, err := io.ReadAll(r.Body)
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body too large")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s stackServer) do(t *testing.T, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	serve(t, s.h, rec, r)
	return rec
}

func (s stackServer) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, httptest.NewRequest(http.MethodGet, target, nil))
}

func (s stackServer) getWithHeader(t *testing.T, target, name, value string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set(name, value)
	return s.do(t, req)
}

func (s stackServer) serverSpan(t *testing.T) sdktrace.ReadOnlySpan {
	t.Helper()
	return spantest.SingleServer(t, s.spans)
}

// --- server span naming ---

func TestStack_ServerSpanNamedByRoutePattern(t *testing.T) {
	t.Parallel()
	tests := []struct {
		target    string
		wantName  string
		wantRoute string
	}{
		{"/v1/regions?lang=ru", "GET /v1/regions", "/v1/regions"},
		{"/v1/dishes/tortilla?lang=es", "GET /v1/dishes/{dishKey}", "/v1/dishes/{dishKey}"},
		{"/v1/dishes/tortilla/variations/gal/recipe", "GET /v1/dishes/{dishKey}/variations/{regionKey}/recipe", "/v1/dishes/{dishKey}/variations/{regionKey}/recipe"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			t.Parallel()
			s := newStack(t, slog.LevelDebug)
			s.get(t, tt.target)

			span := s.serverSpan(t)
			if span.Name() != tt.wantName {
				t.Errorf("got span name %q, want %q", span.Name(), tt.wantName)
			}
			route, ok := spantest.Attr(span, "http.route")
			if !ok || route.AsString() != tt.wantRoute {
				t.Errorf("got http.route %q (present=%v), want %q", route.String(), ok, tt.wantRoute)
			}
		})
	}
}

func TestStack_UnmatchedPathSpanNameHasNoRawPath(t *testing.T) {
	t.Parallel()
	s := newStack(t, slog.LevelDebug)
	s.get(t, "/no/such/secretpath")

	if name := s.serverSpan(t).Name(); strings.Contains(name, "secretpath") {
		t.Errorf("got span name %q, want it not to contain the raw path", name)
	}
}

func TestStack_HealthzCreatesNoSpan(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			s := newStackWithHealth(t, slog.LevelDebug, status)
			s.get(t, healthzPath)

			if spans := s.spans.Ended(); len(spans) != 0 {
				t.Errorf("got spans %v for %s, want none", spantest.Names(spans), healthzPath)
			}
		})
	}
}

// --- trace propagation ---

func TestStack_HonoursIncomingTraceparent(t *testing.T) {
	t.Parallel()
	const (
		traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
		spanID  = "00f067aa0ba902b7"
	)
	s := newStack(t, slog.LevelDebug)
	s.getWithHeader(t, "/v1/regions", "traceparent", "00-"+traceID+"-"+spanID+"-01")

	span := s.serverSpan(t)
	parent := span.Parent()
	if !parent.IsRemote() {
		t.Error("got a local parent, want the remote span from traceparent")
	}
	if parent.TraceID().String() != traceID || parent.SpanID().String() != spanID {
		t.Errorf("got parent %s/%s, want %s/%s", parent.TraceID(), parent.SpanID(), traceID, spanID)
	}
	if got := span.SpanContext().TraceID().String(); got != traceID {
		t.Errorf("got server span trace ID %s, want %s", got, traceID)
	}
}

func TestStack_WithoutTraceparentStartsANewTrace(t *testing.T) {
	t.Parallel()
	s := newStack(t, slog.LevelDebug)
	s.get(t, "/v1/regions")

	span := s.serverSpan(t)
	if span.Parent().IsValid() {
		t.Errorf("got parent %v, want a root span", span.Parent())
	}
	if span.SpanKind() != trace.SpanKindServer {
		t.Errorf("got span kind %v, want %v", span.SpanKind(), trace.SpanKindServer)
	}
}

// --- attribute hygiene ---

func TestStack_SpanAttributesCarryNoQueryOrSecrets(t *testing.T) {
	t.Parallel()
	s := newStack(t, slog.LevelDebug)
	req := httptest.NewRequest(http.MethodGet, "/v1/dishes/tortilla?lang=ru&token=qs-s3cret", nil)
	req.Header.Set("Authorization", "Bearer hdr-s3cret")
	req.Header.Set("Cookie", "lang=cookie-s3cret")
	s.do(t, req)

	spans := s.spans.Ended()
	for _, needle := range []string{"qs-s3cret", "hdr-s3cret", "cookie-s3cret", "lang=ru", "url.query"} {
		if leaks := spantest.AttrsContaining(spans, needle); len(leaks) != 0 {
			t.Errorf("got span attributes %v, want none containing %q", leaks, needle)
		}
	}
}

// --- request ID ---

func TestStack_RequestIDHonouredOnlyWhenValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		incoming string
		honoured bool
	}{
		{name: "hex", incoming: "0af7651916cd43dd8448eb211c80319c", honoured: true},
		{name: "uuid", incoming: "2b0c7c5e-9d2a-4c1f-8f0e-6a1b2c3d4e5f", honoured: true},
		{name: "all allowed punctuation", incoming: "Caddy.req_42-a", honoured: true},
		{name: "single char", incoming: "a", honoured: true},
		{name: "64 chars", incoming: strings.Repeat("x", 64), honoured: true},
		{name: "65 chars", incoming: strings.Repeat("x", 65)},
		{name: "space", incoming: "abc def"},
		{name: "slash", incoming: "abc/def"},
		{name: "colon", incoming: "abc:def"},
		{name: "html", incoming: "<script>"},
		{name: "non-ascii", incoming: "año"},
		{name: "quote", incoming: `abc"def`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newStack(t, slog.LevelDebug)
			rec := s.getWithHeader(t, "/v1/regions", requestIDHeader, tt.incoming)

			got := rec.Header().Get(requestIDHeader)
			if tt.honoured && got != tt.incoming {
				t.Errorf("got %s %q, want the incoming %q echoed", requestIDHeader, got, tt.incoming)
			}
			if !tt.honoured && !generatedRequestID.MatchString(got) {
				t.Errorf("got %s %q for invalid %q, want a generated 32-char lowercase hex ID", requestIDHeader, got, tt.incoming)
			}
		})
	}
}

func TestStack_MissingRequestIDIsGeneratedPerRequest(t *testing.T) {
	t.Parallel()
	s := newStack(t, slog.LevelDebug)
	first := s.get(t, "/v1/regions").Header().Get(requestIDHeader)
	second := s.get(t, "/v1/regions").Header().Get(requestIDHeader)

	for _, id := range []string{first, second} {
		if !generatedRequestID.MatchString(id) {
			t.Errorf("got %s %q, want 32 lowercase hex characters", requestIDHeader, id)
		}
	}
	if first == second {
		t.Errorf("got the same generated request ID %q twice, want a fresh one per request", first)
	}
}

func TestStack_RequestIDOnResponseSpanAndAccessLog(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		incoming string
	}{
		{name: "incoming", incoming: "caddy-0123456789"},
		{name: "generated", incoming: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newStack(t, slog.LevelDebug)
			id := s.getWithHeader(t, "/v1/regions", requestIDHeader, tt.incoming).Header().Get(requestIDHeader)
			if id == "" {
				t.Fatalf("got no %s response header, want one", requestIDHeader)
			}

			attr, ok := spantest.Attr(s.serverSpan(t), requestIDAttr)
			if !ok || attr.AsString() != id {
				t.Errorf("got span %s=%q (present=%v), want %q", requestIDAttr, attr.String(), ok, id)
			}
			if got := singleAccessLine(t, s.logs)[requestIDAttr]; got != id {
				t.Errorf("got access log %s=%v, want %q", requestIDAttr, got, id)
			}
		})
	}
}

// --- access log ---

func TestStack_AccessLogCarriesServerSpanTraceIDs(t *testing.T) {
	t.Parallel()
	s := newStack(t, slog.LevelDebug)
	s.get(t, "/v1/dishes/tortilla")

	sc := s.serverSpan(t).SpanContext()
	line := singleAccessLine(t, s.logs)
	if line["trace_id"] != sc.TraceID().String() || line["span_id"] != sc.SpanID().String() {
		t.Errorf("got access log trace_id=%v span_id=%v, want %s %s", line["trace_id"], line["span_id"], sc.TraceID(), sc.SpanID())
	}
	if line["route"] != "GET /v1/dishes/{dishKey}" {
		t.Errorf("got access log route %v, want the pattern", line["route"])
	}
}

func TestStack_AccessLogLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		target       string
		healthStatus int
		wantLevel    string
	}{
		{name: "healthy healthz is debug", target: healthzPath, healthStatus: http.StatusOK, wantLevel: "DEBUG"},
		{name: "failing healthz keeps info", target: healthzPath, healthStatus: http.StatusServiceUnavailable, wantLevel: "INFO"},
		{name: "api 200 is info", target: "/v1/regions", healthStatus: http.StatusOK, wantLevel: "INFO"},
		{name: "api 500 is info", target: "/fail", healthStatus: http.StatusOK, wantLevel: "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newStackWithHealth(t, slog.LevelDebug, tt.healthStatus)
			s.get(t, tt.target)

			if got := singleAccessLine(t, s.logs)["level"]; got != tt.wantLevel {
				t.Errorf("got access log level %v, want %s", got, tt.wantLevel)
			}
		})
	}
}

func TestStack_HealthyHealthzIsSilentAtInfo(t *testing.T) {
	t.Parallel()
	s := newStack(t, slog.LevelInfo)
	s.get(t, healthzPath)

	if lines := accessLines(t, s.logs); len(lines) != 0 {
		t.Errorf("got access lines %v at INFO, want a healthy probe not logged", lines)
	}
}

// --- errors ---

func TestStack_ServerSpanStatusFollowsResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		target    string
		wantError bool
	}{
		{target: "/v1/regions"},
		{target: "/missing"},
		{target: "/fail", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			t.Parallel()
			s := newStack(t, slog.LevelDebug)
			s.get(t, tt.target)

			got := s.serverSpan(t).Status().Code
			if tt.wantError != (got == codes.Error) {
				t.Errorf("got span status %v, want error=%v", got, tt.wantError)
			}
		})
	}
}

func TestStack_RecoveredPanicRecordsExceptionOnServerSpan(t *testing.T) {
	t.Parallel()
	s := newStack(t, slog.LevelDebug)
	rec := s.get(t, "/boom")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), stackPanicText) {
		t.Errorf("got body %q, want the panic value kept out of the response", rec.Body.String())
	}
	spantest.AssertErrorWithException(t, s.serverSpan(t), stackPanicText)
}

// --- order ---

func TestStack_MaxBodyStaysOutermostSoA413ClosesTheConnection(t *testing.T) {
	t.Parallel()
	s := newStack(t, slog.LevelDebug)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)

	body := strings.NewReader(strings.Repeat("x", int(httpx.MaxBodyBytes)+1))
	resp, err := srv.Client().Post(srv.URL+"/dishes", "application/json", body)
	if err != nil {
		t.Fatalf("got request error %v, want nil", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
	if !resp.Close && !strings.EqualFold(resp.Header.Get("Connection"), "close") {
		t.Errorf("got Connection %q, want the connection closed after a 413", resp.Header.Get("Connection"))
	}
}
