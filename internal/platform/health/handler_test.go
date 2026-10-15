package health_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cosmosfactory/recetas-backend/internal/platform/health"
	"github.com/cosmosfactory/recetas-backend/internal/platform/logging"
)

const internalErrText = "dial tcp 10.0.0.5:5432: connect: connection refused secret-detail"

type fakePinger struct {
	pingFn func(ctx context.Context) error

	mu          sync.Mutex
	calls       int
	deadline    time.Time
	hasDeadline bool
	ctxErr      error
}

func (f *fakePinger) Ping(ctx context.Context) error {
	f.mu.Lock()
	f.calls++
	f.deadline, f.hasDeadline = ctx.Deadline()
	f.mu.Unlock()

	var err error
	if f.pingFn != nil {
		err = f.pingFn(ctx)
	}

	f.mu.Lock()
	f.ctxErr = ctx.Err()
	f.mu.Unlock()
	return err
}

var _ health.Pinger = (*fakePinger)(nil)

func blockUntilDone(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

type testServer struct {
	mux  *http.ServeMux
	logs *bytes.Buffer
}

func newServer(t *testing.T, h func(logger *slog.Logger) *health.Handler) testServer {
	t.Helper()
	var logs bytes.Buffer
	mux := http.NewServeMux()
	h(logging.New(&logs, slog.LevelDebug)).Register(mux)
	return testServer{mux: mux, logs: &logs}
}

func newDefaultServer(t *testing.T, db health.Pinger) testServer {
	t.Helper()
	return newServer(t, func(l *slog.Logger) *health.Handler { return health.NewHandler(db, l) })
}

func newTimeoutServer(t *testing.T, db health.Pinger, timeout time.Duration) testServer {
	t.Helper()
	return newServer(t, func(l *slog.Logger) *health.Handler { return health.NewHandlerWithTimeout(db, l, timeout) })
}

func (s testServer) get(ctx context.Context, t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, r)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not a JSON object: %v (body=%q)", err, w.Body.String())
	}
	return got
}

func decodeLogs(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	var out []map[string]any
	for i, line := range strings.Split(raw, "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line %d is not JSON: %v (line=%q)", i, err, line)
		}
		out = append(out, rec)
	}
	return out
}

func assertHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if got, want := w.Header().Get("Content-Type"), "application/json"; !strings.HasPrefix(got, want) {
		t.Errorf("Content-Type: got %q, want prefix %q", got, want)
	}
	if got, want := w.Header().Get("Cache-Control"), "no-store"; got != want {
		t.Errorf("Cache-Control: got %q, want %q", got, want)
	}
}

func TestDBTimeout_IsOneSecond(t *testing.T) {
	t.Parallel()
	if got, want := health.DBTimeout, time.Second; got != want {
		t.Fatalf("DBTimeout: got %v, want %v", got, want)
	}
}

func TestHealthz_Healthy(t *testing.T) {
	t.Parallel()
	db := &fakePinger{pingFn: func(context.Context) error { return nil }}
	s := newDefaultServer(t, db)

	w := s.get(context.Background(), t)

	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status: got %d, want %d (body=%q)", got, want, w.Body.String())
	}
	assertHeaders(t, w)
	if got, want := decodeBody(t, w), map[string]any{"status": "ok"}; !reflect.DeepEqual(got, want) {
		t.Errorf("body: got %v, want %v", got, want)
	}
	if got, want := db.calls, 1; got != want {
		t.Errorf("ping calls: got %d, want %d", got, want)
	}
	for _, rec := range decodeLogs(t, s.logs) {
		if lvl := rec["level"]; lvl == "WARN" || lvl == "ERROR" {
			t.Errorf("got %v log on healthy check (%v), want none", lvl, rec)
		}
	}
}

func TestHealthz_PingFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		err     error
		wantDB  string
		errText string
	}{
		{
			name:    "generic error is unavailable",
			err:     errors.New(internalErrText),
			wantDB:  "unavailable",
			errText: internalErrText,
		},
		{
			name:    "wrapped deadline exceeded is timeout",
			err:     fmt.Errorf("%s: %w", internalErrText, context.DeadlineExceeded),
			wantDB:  "timeout",
			errText: internalErrText,
		},
		{
			name:    "bare deadline exceeded is timeout",
			err:     context.DeadlineExceeded,
			wantDB:  "timeout",
			errText: context.DeadlineExceeded.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := &fakePinger{pingFn: func(context.Context) error { return tt.err }}
			s := newDefaultServer(t, db)

			w := s.get(context.Background(), t)

			if got, want := w.Code, http.StatusServiceUnavailable; got != want {
				t.Fatalf("status: got %d, want %d (body=%q)", got, want, w.Body.String())
			}
			assertHeaders(t, w)
			want := map[string]any{
				"status": "degraded",
				"checks": map[string]any{"db": tt.wantDB},
			}
			if got := decodeBody(t, w); !reflect.DeepEqual(got, want) {
				t.Errorf("body: got %v, want %v", got, want)
			}
			if body := w.Body.String(); strings.Contains(body, tt.errText) || strings.Contains(body, "secret-detail") {
				t.Errorf("body leaks internal error text: got %q", body)
			}

			var warns int
			for _, rec := range decodeLogs(t, s.logs) {
				switch rec["level"] {
				case "ERROR":
					t.Errorf("got ERROR log %v, want WARN only", rec)
				case "WARN":
					warns++
					if got, want := rec["msg"], "health check failed"; got != want {
						t.Errorf("WARN msg: got %v, want %q", got, want)
					}
					errAttr, _ := rec["err"].(string)
					if !strings.Contains(errAttr, tt.errText) {
						t.Errorf("WARN err attr: got %q, want it to contain %q", errAttr, tt.errText)
					}
				}
			}
			if got, want := warns, 1; got != want {
				t.Errorf("WARN log lines: got %d, want %d (logs=%q)", got, want, s.logs.String())
			}
		})
	}
}

func TestHealthz_PingContextHasDeadline(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		timeout time.Duration
		newSrv  func(t *testing.T, db health.Pinger) testServer
	}{
		{
			name:    "NewHandler uses DBTimeout",
			timeout: health.DBTimeout,
			newSrv:  newDefaultServer,
		},
		{
			name:    "NewHandlerWithTimeout uses the given timeout",
			timeout: 50 * time.Millisecond,
			newSrv: func(t *testing.T, db health.Pinger) testServer {
				t.Helper()
				return newTimeoutServer(t, db, 50*time.Millisecond)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := &fakePinger{pingFn: func(context.Context) error { return nil }}
			s := tt.newSrv(t, db)

			before := time.Now()
			w := s.get(context.Background(), t)
			after := time.Now()

			if got, want := w.Code, http.StatusOK; got != want {
				t.Fatalf("status: got %d, want %d", got, want)
			}
			if !db.hasDeadline {
				t.Fatalf("ping ctx deadline: got none, want one ≤ %v from now", tt.timeout)
			}
			if latest := after.Add(tt.timeout); db.deadline.After(latest) {
				t.Errorf("ping ctx deadline: got %v, want ≤ %v", db.deadline, latest)
			}
			if earliest := before.Add(tt.timeout / 2); db.deadline.Before(earliest) {
				t.Errorf("ping ctx deadline: got %v, want ≥ %v (timeout too short)", db.deadline, earliest)
			}
		})
	}
}

func TestHealthz_SlowPingTimesOut(t *testing.T) {
	t.Parallel()
	const timeout = 50 * time.Millisecond
	db := &fakePinger{pingFn: blockUntilDone}
	s := newTimeoutServer(t, db, timeout)

	start := time.Now()
	w := s.get(context.Background(), t)
	elapsed := time.Since(start)

	if got, want := w.Code, http.StatusServiceUnavailable; got != want {
		t.Fatalf("status: got %d, want %d (body=%q)", got, want, w.Body.String())
	}
	assertHeaders(t, w)
	want := map[string]any{
		"status": "degraded",
		"checks": map[string]any{"db": "timeout"},
	}
	if got := decodeBody(t, w); !reflect.DeepEqual(got, want) {
		t.Errorf("body: got %v, want %v", got, want)
	}
	if minElapsed := timeout - 10*time.Millisecond; elapsed < minElapsed {
		t.Errorf("elapsed: got %v, want ≥ %v", elapsed, minElapsed)
	}
	if maxElapsed := timeout + 500*time.Millisecond; elapsed >= maxElapsed {
		t.Errorf("elapsed: got %v, want < %v", elapsed, maxElapsed)
	}
	if !errors.Is(db.ctxErr, context.DeadlineExceeded) {
		t.Errorf("ping ctx err: got %v, want %v", db.ctxErr, context.DeadlineExceeded)
	}
}

func TestHealthz_RequestCancellationPropagatesToPing(t *testing.T) {
	t.Parallel()
	db := &fakePinger{pingFn: blockUntilDone}
	s := newTimeoutServer(t, db, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.get(ctx, t)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler still running 5s after the request context was cancelled, want it to return promptly")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if got, want := db.calls, 1; got != want {
		t.Fatalf("ping calls: got %d, want %d", got, want)
	}
	if !errors.Is(db.ctxErr, context.Canceled) {
		t.Errorf("ping ctx err: got %v, want %v", db.ctxErr, context.Canceled)
	}
}

func TestHealthz_NonGETMethodNotAllowed(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			db := &fakePinger{pingFn: func(context.Context) error { return nil }}
			s := newDefaultServer(t, db)

			r := httptest.NewRequest(method, "/healthz", nil)
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, r)

			if got, want := w.Code, http.StatusMethodNotAllowed; got != want {
				t.Errorf("status: got %d, want %d", got, want)
			}
			if got := db.calls; got != 0 {
				t.Errorf("ping calls: got %d, want 0", got)
			}
		})
	}
}
