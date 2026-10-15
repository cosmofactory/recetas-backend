package httpx_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cosmosfactory/recetas-backend/internal/platform/httpx"
	"github.com/cosmosfactory/recetas-backend/internal/platform/logging"
)

func newLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return logging.New(&buf, slog.LevelDebug), &buf
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	out := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line %d is not a JSON object: %v (line=%q)", i, err, line)
		}
		out = append(out, rec)
	}
	return out
}

func linesWithLevel(t *testing.T, buf *bytes.Buffer, level string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, rec := range decodeLines(t, buf) {
		if rec["level"] == level {
			out = append(out, rec)
		}
	}
	return out
}

// accessLines returns the access-log records (msg "request").
func accessLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, rec := range decodeLines(t, buf) {
		if rec["msg"] == "request" {
			out = append(out, rec)
		}
	}
	return out
}

func singleAccessLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := accessLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d access log lines, want 1 (logs=%q)", len(lines), buf.String())
	}
	return lines[0]
}

// serve runs h and turns an escaped panic into a test failure instead of
// crashing the test binary.
func serve(t *testing.T, h http.Handler, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	defer func() {
		if v := recover(); v != nil {
			t.Fatalf("got panic %v escaping the middleware, want it recovered", v)
		}
	}()
	h.ServeHTTP(w, r)
}

// --- Chain ---

func TestChain_FirstMiddlewareIsOutermost(t *testing.T) {
	t.Parallel()
	var calls []string
	mw := func(name string) httpx.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, name+":before")
				next.ServeHTTP(w, r)
				calls = append(calls, name+":after")
			})
		}
	}
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls = append(calls, "handler")
	})

	httpx.Chain(h, mw("a"), mw("b"), mw("c")).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"a:before", "b:before", "c:before", "handler", "c:after", "b:after", "a:after"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Errorf("got call order %v, want %v", calls, want)
	}
}

func TestChain_NoMiddlewaresServesHandler(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	rec := httptest.NewRecorder()

	httpx.Chain(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusTeapot)
	}
}

// --- Recover ---

const panicText = "boom: secret-panic-detail"

func TestRecover_PanicReturnsGeneric500AndLogsStack(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
	}{
		{name: "string value", value: panicText},
		{name: "error value", value: errors.New(panicText)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger, logs := newLogger(t)
			h := httpx.Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic(tt.value)
			}))
			rec := httptest.NewRecorder()

			serve(t, h, rec, httptest.NewRequest(http.MethodGet, "/dishes", nil))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("got status %d, want %d", rec.Code, http.StatusInternalServerError)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("got Content-Type %q, want application/json", ct)
			}
			if got, want := strings.TrimSpace(rec.Body.String()), `{"error":"internal error"}`; got != want {
				t.Errorf("got body %q, want %q", got, want)
			}
			if strings.Contains(rec.Body.String(), "secret-panic-detail") {
				t.Errorf("got body %q, want it not to contain the panic value", rec.Body.String())
			}

			errs := linesWithLevel(t, logs, "ERROR")
			if len(errs) != 1 {
				t.Fatalf("got %d ERROR log lines, want 1 (logs=%q)", len(errs), logs.String())
			}
			line := errs[0]
			if got := line["msg"]; got != "panic recovered" {
				t.Errorf("got msg %v, want %q", got, "panic recovered")
			}
			raw, err := json.Marshal(line)
			if err != nil {
				t.Fatalf("re-marshal log line: %v", err)
			}
			if !strings.Contains(string(raw), "secret-panic-detail") {
				t.Errorf("got log line %s, want it to contain the panic value", raw)
			}
			stack, ok := line["stack"].(string)
			if !ok || strings.TrimSpace(stack) == "" {
				t.Errorf("got stack attr %v, want a non-empty string", line["stack"])
			}
		})
	}
}

func TestRecover_ErrAbortHandlerIsRepanicked(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	h := httpx.Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	rec := httptest.NewRecorder()

	recovered := func() (v any) {
		defer func() { v = recover() }()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		return nil
	}()

	if recovered != http.ErrAbortHandler {
		t.Errorf("got recovered value %v, want http.ErrAbortHandler", recovered)
	}
	if strings.Contains(rec.Body.String(), "internal error") {
		t.Errorf("got body %q, want no 500 error body", rec.Body.String())
	}
	if errs := linesWithLevel(t, logs, "ERROR"); len(errs) != 0 {
		t.Errorf("got %d ERROR log lines, want 0 (logs=%q)", len(errs), logs.String())
	}
}

func TestRecover_NonPanickingRequestPassesThrough(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	h := httpx.Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "created")
	}))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/dishes", nil))

	if rec.Code != http.StatusCreated {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("X-Test"); got != "yes" {
		t.Errorf("got X-Test header %q, want %q", got, "yes")
	}
	if got := rec.Body.String(); got != "created" {
		t.Errorf("got body %q, want %q", got, "created")
	}
	if errs := linesWithLevel(t, logs, "ERROR"); len(errs) != 0 {
		t.Errorf("got %d ERROR log lines, want 0 (logs=%q)", len(errs), logs.String())
	}
}

// --- AccessLog ---

func TestAccessLog_LogsRoutePatternNotPathOrQuery(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dishes/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "hello")
	})
	rec := httptest.NewRecorder()

	httpx.AccessLog(logger)(mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dishes/42?secret=x", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got response status %d, want %d", rec.Code, http.StatusAccepted)
	}
	if got := rec.Body.String(); got != "hello" {
		t.Errorf("got response body %q, want %q", got, "hello")
	}
	line := singleAccessLine(t, logs)
	if got := line["level"]; got != "INFO" {
		t.Errorf("got level %v, want INFO", got)
	}
	if got := line["method"]; got != http.MethodGet {
		t.Errorf("got method %v, want %q", got, http.MethodGet)
	}
	if got := line["route"]; got != "GET /dishes/{id}" {
		t.Errorf("got route %v, want %q", got, "GET /dishes/{id}")
	}
	if got := line["status"]; got != float64(http.StatusAccepted) {
		t.Errorf("got status %v (%T), want %d", got, got, http.StatusAccepted)
	}
	if got := line["bytes"]; got != float64(len("hello")) {
		t.Errorf("got bytes %v (%T), want %d", got, got, len("hello"))
	}
	d, ok := line["duration_ms"].(float64)
	if !ok {
		t.Errorf("got duration_ms %v (%T), want a number", line["duration_ms"], line["duration_ms"])
	} else if d < 0 {
		t.Errorf("got duration_ms %v, want >= 0", d)
	}

	raw := logs.String()
	for _, leak := range []string{"secret", "?", "/dishes/42"} {
		if strings.Contains(raw, leak) {
			t.Errorf("got log %q, want it not to contain %q", raw, leak)
		}
	}
}

func TestAccessLog_UnmatchedRoute(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dishes", func(http.ResponseWriter, *http.Request) {})

	httpx.AccessLog(logger)(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope/123", nil))

	line := singleAccessLine(t, logs)
	if got := line["route"]; got != "unmatched" {
		t.Errorf("got route %v, want %q", got, "unmatched")
	}
	if got := line["status"]; got != float64(http.StatusNotFound) {
		t.Errorf("got status %v, want %d", got, http.StatusNotFound)
	}
	if strings.Contains(logs.String(), "/nope/123") {
		t.Errorf("got log %q, want it not to contain the raw path", logs.String())
	}
}

func TestAccessLog_ImplicitStatusIs200(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dishes", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "abc")
		_, _ = io.WriteString(w, "de")
	})

	httpx.AccessLog(logger)(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/dishes", nil))

	line := singleAccessLine(t, logs)
	if got := line["status"]; got != float64(http.StatusOK) {
		t.Errorf("got status %v, want %d", got, http.StatusOK)
	}
	if got := line["bytes"]; got != float64(5) {
		t.Errorf("got bytes %v, want %d", got, 5)
	}
}

func TestAccessLog_RecordsStatus500WhenComposedWithRecover(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dishes", func(http.ResponseWriter, *http.Request) {
		panic(panicText)
	})
	rec := httptest.NewRecorder()

	serve(t, httpx.Chain(mux, httpx.AccessLog(logger), httpx.Recover(logger)), rec, httptest.NewRequest(http.MethodGet, "/dishes", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got response status %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	line := singleAccessLine(t, logs)
	if got := line["status"]; got != float64(http.StatusInternalServerError) {
		t.Errorf("got access-log status %v, want %d", got, http.StatusInternalServerError)
	}
}

func TestAccessLog_WrapperSupportsResponseControllerFlush(t *testing.T) {
	t.Parallel()
	logger, _ := newLogger(t)
	var flushErr error
	called := false
	h := httpx.AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = io.WriteString(w, "chunk")
		flushErr = http.NewResponseController(w).Flush()
	}))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Fatal("handler was not called")
	}
	if flushErr != nil {
		t.Errorf("got Flush error %v, want nil", flushErr)
	}
	if !rec.Flushed {
		t.Errorf("got recorder Flushed=false, want true")
	}
}

// --- MaxBody ---

func TestMaxBody_LimitsRequestBody(t *testing.T) {
	t.Parallel()
	const limit int64 = 16
	tests := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{name: "empty", size: 0},
		{name: "under limit", size: limit - 1},
		{name: "exactly limit", size: limit},
		{name: "one over limit", size: limit + 1, wantErr: true},
		{name: "far over limit", size: limit * 100, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var (
				readErr error
				read    []byte
			)
			h := httpx.MaxBody(limit)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				read, readErr = io.ReadAll(r.Body)
			}))
			body := strings.Repeat("x", int(tt.size))

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))

			if !tt.wantErr {
				if readErr != nil {
					t.Fatalf("got read error %v, want nil", readErr)
				}
				if int64(len(read)) != tt.size {
					t.Errorf("got %d bytes read, want %d", len(read), tt.size)
				}
				return
			}
			var mbe *http.MaxBytesError
			if !errors.As(readErr, &mbe) {
				t.Fatalf("got read error %v (%T), want *http.MaxBytesError", readErr, readErr)
			}
			if mbe.Limit != limit {
				t.Errorf("got MaxBytesError.Limit %d, want %d", mbe.Limit, limit)
			}
		})
	}
}

func TestMaxBodyBytes_Is64KiB(t *testing.T) {
	t.Parallel()
	if httpx.MaxBodyBytes != 64<<10 {
		t.Errorf("got MaxBodyBytes %d, want %d", httpx.MaxBodyBytes, 64<<10)
	}
}

// --- NewServer ---

func TestNewServer_SetsHardenedDefaults(t *testing.T) {
	t.Parallel()
	logger, _ := newLogger(t)
	mux := http.NewServeMux()

	srv := httpx.NewServer(":9999", mux, logger)

	if srv == nil {
		t.Fatal("got nil server, want non-nil")
	}
	if srv.Addr != ":9999" {
		t.Errorf("got Addr %q, want %q", srv.Addr, ":9999")
	}
	if got, ok := srv.Handler.(*http.ServeMux); !ok || got != mux {
		t.Errorf("got Handler %v, want the given mux", srv.Handler)
	}
	durations := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "ReadHeaderTimeout", got: srv.ReadHeaderTimeout, want: 5 * time.Second},
		{name: "ReadTimeout", got: srv.ReadTimeout, want: 10 * time.Second},
		{name: "WriteTimeout", got: srv.WriteTimeout, want: 15 * time.Second},
		{name: "IdleTimeout", got: srv.IdleTimeout, want: 60 * time.Second},
	}
	for _, d := range durations {
		if d.got != d.want {
			t.Errorf("got %s %v, want %v", d.name, d.got, d.want)
		}
	}
	if srv.MaxHeaderBytes != 32<<10 {
		t.Errorf("got MaxHeaderBytes %d, want %d", srv.MaxHeaderBytes, 32<<10)
	}
	if srv.ErrorLog == nil {
		t.Error("got nil ErrorLog, want non-nil")
	}
}
