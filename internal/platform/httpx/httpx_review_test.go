package httpx_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cosmosfactory/recetas-backend/internal/platform/httpx"
	"github.com/cosmosfactory/recetas-backend/internal/platform/logging"
)

// syncBuffer is a goroutine-safe log sink for tests that run handlers on an
// httptest.Server goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// snapshot returns a copy usable with the *bytes.Buffer helpers.
func (b *syncBuffer) snapshot() *bytes.Buffer {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.NewBufferString(b.buf.String())
}

func newSyncLogger(t *testing.T) (*slog.Logger, *syncBuffer) {
	t.Helper()
	sb := &syncBuffer{}
	return logging.New(sb, slog.LevelDebug), sb
}

// recoverFrom runs h and returns whatever value panicked out of it (nil if none).
func recoverFrom(h http.Handler, w http.ResponseWriter, r *http.Request) (v any) {
	defer func() { v = recover() }()
	h.ServeHTTP(w, r)
	return nil
}

// countingWriter records every WriteHeader call that reaches it.
type countingWriter struct {
	http.ResponseWriter
	codes []int
}

func (w *countingWriter) WriteHeader(code int) {
	w.codes = append(w.codes, code)
	w.ResponseWriter.WriteHeader(code)
}

// --- Recover: panic after the header was written ---

func TestRecover_PanicAfterHeaderWrittenAbortsConnection(t *testing.T) {
	t.Parallel()
	logger, logs := newSyncLogger(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"partial":`)
		_ = http.NewResponseController(w).Flush()
		panic(panicText)
	})
	srv := httptest.NewServer(httpx.Chain(h, httpx.AccessLog(logger), httpx.Recover(logger)))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/dishes")
	if err == nil {
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr == nil {
			t.Errorf("got complete body %q with nil read error, want the connection aborted (unexpected EOF / reset)", body)
		}
	}
	// Close waits for the handler goroutine, so all log lines are written.
	srv.Close()

	snap := logs.snapshot()
	errs := linesWithLevel(t, snap, "ERROR")
	if len(errs) != 1 {
		t.Fatalf("got %d ERROR log lines, want 1 (logs=%q)", len(errs), snap.String())
	}
	if got := errs[0]["msg"]; got != "panic recovered" {
		t.Errorf("got ERROR msg %v, want %q", got, "panic recovered")
	}
	line := singleAccessLine(t, snap)
	if got := line["aborted"]; got != true {
		t.Errorf("got access-log aborted %v, want true", got)
	}
	if got := line["status"]; got != float64(http.StatusOK) {
		t.Errorf("got access-log status %v, want %d", got, http.StatusOK)
	}
}

func TestRecover_PanicAfterHeaderWrittenRepanicsErrAbortHandler(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	h := httpx.Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"partial":`)
		panic(panicText)
	}))
	rec := httptest.NewRecorder()

	recovered := recoverFrom(h, rec, httptest.NewRequest(http.MethodGet, "/dishes", nil))

	if recovered != http.ErrAbortHandler {
		t.Errorf("got recovered value %v, want http.ErrAbortHandler", recovered)
	}
	errs := linesWithLevel(t, logs, "ERROR")
	if len(errs) != 1 {
		t.Fatalf("got %d ERROR log lines, want 1 (logs=%q)", len(errs), logs.String())
	}
	if got := errs[0]["msg"]; got != "panic recovered" {
		t.Errorf("got ERROR msg %v, want %q", got, "panic recovered")
	}
	if strings.Contains(rec.Body.String(), "internal error") {
		t.Errorf("got body %q, want no 500 error body appended to the partial response", rec.Body.String())
	}
}

func TestRecover_PanicAfterHeaderWrittenUnderAccessLogMarksAborted(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	h := httpx.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		panic(panicText)
	}), httpx.AccessLog(logger), httpx.Recover(logger))

	recovered := recoverFrom(h, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if recovered != http.ErrAbortHandler {
		t.Errorf("got recovered value %v, want http.ErrAbortHandler", recovered)
	}
	line := singleAccessLine(t, logs)
	if got := line["aborted"]; got != true {
		t.Errorf("got access-log aborted %v, want true", got)
	}
}

// --- Recover: headers set before the panic ---

func TestRecover_DiscardsHeadersSetBeforePanic(t *testing.T) {
	t.Parallel()
	logger, _ := newLogger(t)
	h := httpx.Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "999")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		panic(panicText)
	}))
	rec := httptest.NewRecorder()

	serve(t, h, rec, httptest.NewRequest(http.MethodGet, "/dishes", nil))

	res := rec.Result()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", res.StatusCode, http.StatusInternalServerError)
	}
	for _, name := range []string{"Content-Length", "Cache-Control"} {
		if got := rec.Header().Values(name); len(got) != 0 {
			t.Errorf("got %s header %q on the 500, want it absent", name, got)
		}
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("got Content-Type %q, want application/json", ct)
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"error":"internal error"}`; got != want {
		t.Errorf("got body %q, want %q", got, want)
	}
}

// --- AccessLog: aborted ---

func TestAccessLog_ErrAbortHandlerLogsAbortedAndRepanics(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dishes", func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	})

	recovered := recoverFrom(httpx.AccessLog(logger)(mux), httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/dishes", nil))

	if recovered != http.ErrAbortHandler {
		t.Errorf("got recovered value %v, want http.ErrAbortHandler re-panicked", recovered)
	}
	line := singleAccessLine(t, logs)
	if got := line["level"]; got != "INFO" {
		t.Errorf("got level %v, want INFO", got)
	}
	if got := line["aborted"]; got != true {
		t.Errorf("got aborted %v, want true", got)
	}
	if got := line["route"]; got != "GET /dishes" {
		t.Errorf("got route %v, want %q", got, "GET /dishes")
	}
}

func TestAccessLog_CompletedRequestsHaveNoAbortedKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "normal response", handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "ok")
		}},
		{name: "panic recovered into 500", handler: func(http.ResponseWriter, *http.Request) {
			panic(panicText)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger, logs := newLogger(t)
			h := httpx.Chain(tt.handler, httpx.AccessLog(logger), httpx.Recover(logger))

			serve(t, h, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

			line := singleAccessLine(t, logs)
			if v, ok := line["aborted"]; ok {
				t.Errorf("got aborted key with value %v, want it absent", v)
			}
		})
	}
}

// --- AccessLog + Routed: route survives request cloning ---

type cloneKey struct{}

func cloner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), cloneKey{}, "v")))
	})
}

func TestAccessLog_RouteSurvivesRequestCloningViaRouted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		path      string
		wantRoute string
	}{
		{name: "matched", path: "/dishes/42", wantRoute: "GET /dishes/{id}"},
		{name: "unmatched", path: "/nope/1", wantRoute: "unmatched"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger, logs := newLogger(t)
			var sawValue any
			mux := http.NewServeMux()
			mux.HandleFunc("GET /dishes/{id}", func(w http.ResponseWriter, r *http.Request) {
				sawValue = r.Context().Value(cloneKey{})
				w.WriteHeader(http.StatusOK)
			})

			serve(t, httpx.Chain(httpx.Routed(mux), httpx.AccessLog(logger), cloner),
				httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.path, nil))

			line := singleAccessLine(t, logs)
			if got := line["route"]; got != tt.wantRoute {
				t.Errorf("got route %v, want %q", got, tt.wantRoute)
			}
			if tt.name == "matched" && sawValue != "v" {
				t.Errorf("got cloned context value %v in handler, want %q", sawValue, "v")
			}
		})
	}
}

func TestAccessLog_RoutedWithoutCloning(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dishes/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	rec := httptest.NewRecorder()

	serve(t, httpx.Chain(httpx.Routed(mux), httpx.AccessLog(logger)), rec, httptest.NewRequest(http.MethodGet, "/dishes/7", nil))

	if rec.Code != http.StatusAccepted {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusAccepted)
	}
	line := singleAccessLine(t, logs)
	if got := line["route"]; got != "GET /dishes/{id}" {
		t.Errorf("got route %v, want %q", got, "GET /dishes/{id}")
	}
}

func TestRouted_ServesMuxWithoutAccessLog(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	var gotID string
	mux.HandleFunc("GET /dishes/{id}", func(w http.ResponseWriter, r *http.Request) {
		gotID = r.PathValue("id")
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()

	serve(t, httpx.Routed(mux), rec, httptest.NewRequest(http.MethodGet, "/dishes/42", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if gotID != "42" {
		t.Errorf("got path value id %q, want %q", gotID, "42")
	}
}

// --- MaxBody outermost: 413 closes the connection ---

func TestMaxBody_Outermost413ClosesConnection(t *testing.T) {
	t.Parallel()
	logger, _ := newSyncLogger(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /dishes", func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = io.WriteString(w, `{"error":"request body too large"}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(httpx.Chain(httpx.Routed(mux),
		httpx.MaxBody(16), httpx.AccessLog(logger), httpx.Recover(logger)))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Post(srv.URL+"/dishes", "application/json", strings.NewReader(strings.Repeat("x", 100)))
	if err != nil {
		t.Fatalf("got request error %v, want nil", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
	if !resp.Close && !strings.EqualFold(resp.Header.Get("Connection"), "close") {
		t.Errorf("got resp.Close=false and Connection %q, want the connection closed after a 413", resp.Header.Get("Connection"))
	}
}

// --- Deadline ---

func TestDeadline_SetsContextDeadline(t *testing.T) {
	t.Parallel()
	const d = 5 * time.Second
	var (
		deadline time.Time
		ok       bool
	)
	h := httpx.Deadline(d)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	}))

	before := time.Now()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	after := time.Now()

	if !ok {
		t.Fatal("got no context deadline, want one")
	}
	if lo := before.Add(d); deadline.Before(lo) {
		t.Errorf("got deadline %v, want >= %v (start + %v)", deadline, lo, d)
	}
	if hi := after.Add(d); deadline.After(hi) {
		t.Errorf("got deadline %v, want <= %v (end + %v)", deadline, hi, d)
	}
}

func TestDeadline_ContextExpiresWithDeadlineExceeded(t *testing.T) {
	t.Parallel()
	var ctxErr error
	timedOut := false
	h := httpx.Deadline(50 * time.Millisecond)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			ctxErr = r.Context().Err()
		case <-time.After(5 * time.Second):
			timedOut = true
		}
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if timedOut {
		t.Fatal("got context still live after 5s, want it done after 50ms")
	}
	if !errors.Is(ctxErr, context.DeadlineExceeded) {
		t.Errorf("got context error %v, want context.DeadlineExceeded", ctxErr)
	}
}

func TestDeadline_PreservesParentContextValues(t *testing.T) {
	t.Parallel()
	var got any
	h := httpx.Deadline(time.Second)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Context().Value(cloneKey{})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), cloneKey{}, "parent"))

	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "parent" {
		t.Errorf("got context value %v, want %q", got, "parent")
	}
}

func TestHandlerTimeout_Is12sAndBelowWriteTimeout(t *testing.T) {
	t.Parallel()
	if httpx.HandlerTimeout != 12*time.Second {
		t.Errorf("got HandlerTimeout %v, want %v", httpx.HandlerTimeout, 12*time.Second)
	}
	logger, _ := newLogger(t)
	srv := httpx.NewServer(":0", http.NewServeMux(), logger)
	if srv.WriteTimeout <= httpx.HandlerTimeout {
		t.Errorf("got WriteTimeout %v, want > HandlerTimeout %v", srv.WriteTimeout, httpx.HandlerTimeout)
	}
}

// --- responseWriter: duplicate WriteHeader ---

func TestAccessLog_SecondWriteHeaderIsNotPassedThrough(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		calls      []int
		wantPassed []int
		wantStatus int
		// ResponseRecorder treats a 1xx as the final status, so its Code is
		// only meaningful for rows without informational responses.
		checkRecorder bool
	}{
		{name: "duplicate final status", calls: []int{201, 500}, wantPassed: []int{201}, wantStatus: 201, checkRecorder: true},
		{name: "informational then final", calls: []int{103, 201}, wantPassed: []int{103, 201}, wantStatus: 201},
		{name: "informational then duplicate final", calls: []int{103, 201, 500}, wantPassed: []int{103, 201}, wantStatus: 201},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger, logs := newLogger(t)
			h := httpx.AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for _, c := range tt.calls {
					w.WriteHeader(c)
				}
			}))
			rec := httptest.NewRecorder()
			cw := &countingWriter{ResponseWriter: rec}

			serve(t, h, cw, httptest.NewRequest(http.MethodGet, "/", nil))

			if !equalInts(cw.codes, tt.wantPassed) {
				t.Errorf("got WriteHeader calls %v reaching the underlying writer, want %v", cw.codes, tt.wantPassed)
			}
			if tt.checkRecorder && rec.Code != tt.wantStatus {
				t.Errorf("got response status %d, want %d", rec.Code, tt.wantStatus)
			}
			line := singleAccessLine(t, logs)
			if got := line["status"]; got != float64(tt.wantStatus) {
				t.Errorf("got access-log status %v, want %d", got, tt.wantStatus)
			}
		})
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
