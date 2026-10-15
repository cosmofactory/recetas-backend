package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api"
	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
)

var _ apigen.StrictServerInterface = api.Stub{}

// repoRoot walks up from the package directory to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the package directory")
		}
		dir = parent
	}
}

func specPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "api", "openapi.yaml")
}

type testServer struct {
	mux  *http.ServeMux
	logs *bytes.Buffer
}

func newTestServer(t *testing.T, impl apigen.StrictServerInterface) testServer {
	t.Helper()
	var logs bytes.Buffer
	logger := logging.New(&logs, slog.LevelDebug)
	mux := http.NewServeMux()
	api.Register(mux, impl, logger)
	return testServer{mux: mux, logs: &logs}
}

func (s testServer) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeErrorBody decodes the contract Error shape strictly: unknown fields
// fail, matching additionalProperties: false in the spec.
func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("got Content-Type %q, want application/json", ct)
	}
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	var body errorBody
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("body is not the contract Error shape: %v (body=%q)", err, rec.Body.String())
	}
	return body
}

func logLinesWithLevel(t *testing.T, logs *bytes.Buffer, level string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimRight(logs.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %v (line=%q)", err, line)
		}
		if rec["level"] == level {
			out = append(out, rec)
		}
	}
	return out
}

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }
