package main

// Seams in main.go these tests pin:
//
//	func run(cfg config.Config, stdout io.Writer) error
//	    sets up telemetry (cfg.Telemetry plus ServiceVersion = version), builds
//	    the logger on stdout, writes a first "starting" line with version and
//	    app_env, logs "telemetry disabled" once at INFO when cfg.Telemetry is
//	    off, and only then connects to Postgres.
//	func newHandler(pool *pgxpool.Pool, logger *slog.Logger, tp trace.TracerProvider) http.Handler
//	    registers health and the API on a mux and wraps it in httpx.Stack.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/cosmofactory/recetas-backend/internal/config"
	"github.com/cosmofactory/recetas-backend/internal/platform/database"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
	"github.com/cosmofactory/recetas-backend/internal/testutil/spantest"
)

// contentVersionQuery is the span of the sqlc query every catalog read
// starts with.
const contentVersionQuery = "query LatestContentVersion"

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %v (line=%q)", err, line)
		}
		out = append(out, rec)
	}
	return out
}

func countMsg(lines []map[string]any, msg, level string) int {
	n := 0
	for _, l := range lines {
		if l["msg"] == msg && l["level"] == level {
			n++
		}
	}
	return n
}

func TestRun_LogsStartingBeforeConnectingToDatabase(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "malformed url", url: "postgres://%zz"},
		{name: "unreachable", url: "postgres://u:p@127.0.0.1:1/recetas?sslmode=disable&connect_timeout=2"}, //nolint:gosec // fake DSN, refused on loopback
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			cfg := config.Config{AppEnv: config.EnvDemo, LogLevel: slog.LevelInfo, DatabaseURL: tt.url, HTTPAddr: "127.0.0.1:0"}

			if err := run(cfg, &stdout); err == nil {
				t.Fatal("got nil error from run with a broken database, want one")
			}
			assertStartupLines(t, logLines(t, &stdout))
		})
	}
}

func assertStartupLines(t *testing.T, lines []map[string]any) {
	t.Helper()
	if len(lines) == 0 {
		t.Fatal("got no log lines, want a first \"starting\" line")
	}
	first := lines[0]
	if first["msg"] != "starting" || first["level"] != "INFO" {
		t.Errorf("got first line %v, want msg \"starting\" at INFO", first)
	}
	if first["version"] != version || first["app_env"] != string(config.EnvDemo) {
		t.Errorf("got version=%v app_env=%v, want %q %q", first["version"], first["app_env"], version, config.EnvDemo)
	}
	if n := countMsg(lines, "telemetry disabled", "INFO"); n != 1 {
		t.Errorf("got %d \"telemetry disabled\" INFO lines, want 1 (lines=%v)", n, lines)
	}
}

// --- integration: one request, one trace with a DB child span ---

func newTracedHandler(t *testing.T, tp trace.TracerProvider) http.Handler {
	t.Helper()
	url := testdb.New(t).Config().ConnConfig.ConnString()
	pool, err := database.NewPool(context.Background(), url, tp)
	if err != nil {
		t.Fatalf("got NewPool error %v, want nil", err)
	}
	t.Cleanup(pool.Close)
	return newHandler(pool, logging.New(io.Discard, slog.LevelDebug), tp)
}

func TestNewHandler_RepositoryRequestHasDBChildSpanUnderServerSpan(t *testing.T) {
	tp, spans := spantest.New(t)
	h := newTracedHandler(t, tp)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/regions?lang=en", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 (body=%.200q)", rec.Code, rec.Body.String())
	}

	server := spantest.SingleServer(t, spans)
	if server.Name() != "GET /v1/regions" {
		t.Errorf("got server span %q, want %q", server.Name(), "GET /v1/regions")
	}
	query, ok := spantest.Named(spans.Ended(), contentVersionQuery)
	if !ok {
		t.Fatalf("got spans %v, want a DB span named %q", spantest.Names(spans.Ended()), contentVersionQuery)
	}
	assertChildOf(t, query, server.SpanContext())
}

func assertChildOf(t *testing.T, child interface {
	Parent() trace.SpanContext
	SpanKind() trace.SpanKind
}, parent trace.SpanContext) {
	t.Helper()
	if child.Parent().TraceID() != parent.TraceID() {
		t.Errorf("got DB span trace %s, want the server span's %s", child.Parent().TraceID(), parent.TraceID())
	}
	if child.Parent().SpanID() != parent.SpanID() {
		t.Errorf("got DB span parent %s, want the server span %s", child.Parent().SpanID(), parent.SpanID())
	}
	if child.SpanKind() != trace.SpanKindClient {
		t.Errorf("got DB span kind %v, want %v", child.SpanKind(), trace.SpanKindClient)
	}
}

func TestNewHandler_HealthzWithDatabaseCreatesNoSpan(t *testing.T) {
	tp, spans := spantest.New(t)
	h := newTracedHandler(t, tp)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if got := spans.Ended(); len(got) != 0 {
		t.Errorf("got spans %v for /healthz, want none (its ping is not traced either)", spantest.Names(got))
	}
}
