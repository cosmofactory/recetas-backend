package catalog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	apispec "github.com/cosmofactory/recetas-backend/api"
	"github.com/cosmofactory/recetas-backend/internal/api"
	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
)

const wantCacheControl = "public, max-age=60, stale-while-revalidate=600"

// apiServer is the real /v1 stack: api.Register over the catalog handler.
type apiServer struct {
	mux  *http.ServeMux
	logs *bytes.Buffer
}

func newAPIServer(t *testing.T, repo catalog.Repository) apiServer {
	t.Helper()
	var logs bytes.Buffer
	logger := logging.New(&logs, slog.LevelDebug)
	mux := http.NewServeMux()
	api.Register(mux, catalog.NewHandler(catalog.NewService(repo), logger), logger)
	return apiServer{mux: mux, logs: &logs}
}

// get sends a GET with one If-None-Match header line per ifNoneMatch value
// and validates the response against the contract.
func (s apiServer) get(t *testing.T, target string, ifNoneMatch ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, v := range ifNoneMatch {
		req.Header.Add("If-None-Match", v)
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	assertMatchesContract(t, req, rec)
	return rec
}

func (s apiServer) errorLogLines(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(s.logs.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %v (line=%q)", err, line)
		}
		if entry["level"] == "ERROR" {
			out = append(out, entry)
		}
	}
	return out
}

// contractRouter routes requests to spec operations. Servers are dropped so
// httptest's example.com host matches.
var contractRouter = sync.OnceValues(func() (routers.Router, error) {
	doc, err := openapi3.NewLoader().LoadFromData(apispec.OpenAPIYAML)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	doc.Servers = nil
	return legacy.NewRouter(doc)
})

// assertMatchesContract validates status, Content-Type and body against
// api/openapi.yaml; undocumented statuses fail.
func assertMatchesContract(t *testing.T, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()
	router, err := contractRouter()
	if err != nil {
		t.Fatalf("contract router: %v", err)
	}
	route, params, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("contract: no operation for %s %s: %v", req.Method, req.URL, err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Options:                &openapi3filter.Options{IncludeResponseStatus: true},
	}
	input.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		t.Errorf("contract: %s %s → %d does not match api/openapi.yaml: %v (body=%.300q)",
			req.Method, req.URL, rec.Code, err, rec.Body.String())
	}
}

// decodeStrict decodes the body into T, failing on fields T doesn't declare.
func decodeStrict[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	var v T
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("body does not decode into %T: %v (body=%.300q)", v, err, rec.Body.String())
	}
	return v
}

// assertSameJSON compares values by their JSON encoding, which is what the
// client sees (nil and empty slices differ, as null and []).
func assertSameJSON(t *testing.T, got, want any) {
	t.Helper()
	g, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	w, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if !bytes.Equal(g, w) {
		t.Errorf("body mismatch:\ngot  %s\nwant %s", g, w)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("got status %d, want %d (body=%.300q)", rec.Code, want, rec.Body.String())
	}
}

func assertHeader(t *testing.T, rec *httptest.ResponseRecorder, name, want string) {
	t.Helper()
	if got := rec.Header().Get(name); got != want {
		t.Errorf("got %s %q, want %q", name, got, want)
	}
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	assertStatus(t, rec, wantStatus)
	assertHeader(t, rec, "Content-Type", "application/json")
	body := decodeStrict[errorBody](t, rec)
	if body.Error.Code != wantCode {
		t.Errorf("got error code %q, want %q", body.Error.Code, wantCode)
	}
	if body.Error.Message == "" {
		t.Error("got an empty error message, want a client-safe one")
	}
}
