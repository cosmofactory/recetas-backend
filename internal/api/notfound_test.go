package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cosmofactory/recetas-backend/internal/api"
	"github.com/cosmofactory/recetas-backend/internal/dish"
	"github.com/cosmofactory/recetas-backend/internal/platform/health"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
)

// fakeDishRepo is an in-memory dish.Repository for the legacy /dishes routes.
type fakeDishRepo struct {
	mu     sync.Mutex
	dishes []dish.Dish
}

func (r *fakeDishRepo) Create(_ context.Context, name, description string) (dish.Dish, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := dish.Dish{ID: int64(len(r.dishes) + 1), Name: name, Description: description, CreatedAt: now, UpdatedAt: now}
	r.dishes = append(r.dishes, d)
	return d, nil
}

func (r *fakeDishRepo) List(context.Context) ([]dish.Dish, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]dish.Dish{}, r.dishes...), nil
}

func (r *fakeDishRepo) Get(_ context.Context, id int64) (dish.Dish, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.dishes {
		if d.ID == id {
			return d, nil
		}
	}
	return dish.Dish{}, dish.ErrNotFound
}

func (r *fakeDishRepo) Count(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.dishes)), nil
}

// newFullMux registers every handler on one mux in the order cmd/server/main.go uses.
func newFullMux(t *testing.T) testServer {
	t.Helper()
	var logs bytes.Buffer
	logger := logging.New(&logs, slog.LevelDebug)
	repo := &fakeDishRepo{}
	if _, err := repo.Create(context.Background(), "Tortilla de patatas", "Huevos y patatas"); err != nil {
		t.Fatalf("seed fake repo: %v", err)
	}
	mux := http.NewServeMux()
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("got panic %v registering handlers, want none", r)
			}
		}()
		dish.NewHandler(dish.NewService(repo), logger).Register(mux)
		health.NewHandler(okPinger{}, logger).Register(mux)
		api.Register(mux, api.Stub{}, logger)
	}()
	return testServer{mux: mux, logs: &logs}
}

func assertContractNotFound(t *testing.T, srv testServer, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d (body=%.200q)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	body := decodeErrorBody(t, rec)
	if got, want := body.Error.Code, "not_found"; got != want {
		t.Errorf("got error code %q, want %q", got, want)
	}
	if got, want := body.Error.Message, "not found"; got != want {
		t.Errorf("got error message %q, want %q", got, want)
	}
	if lines := logLinesWithLevel(t, srv.logs, "ERROR"); len(lines) != 0 {
		t.Errorf("got %d ERROR log lines, want 0 (logs=%q)", len(lines), srv.logs.String())
	}
}

var unroutedPaths = []string{
	"/v1/nope",
	"/v1/regions/",
	"/v1/dishes/tortilla/variations",
	"/v1/dishes/tortilla/variations/gal",
	"/v1",
	"/v1/",
	"/totally/unknown",
	"/",
	"/healthz/extra",
}

func TestRegister_UnroutedPathIsContract404(t *testing.T) {
	t.Parallel()
	setups := []struct {
		name string
		new  func(t *testing.T) testServer
	}{
		{name: "api only", new: func(t *testing.T) testServer { return newTestServer(t, api.Stub{}) }},
		{name: "full mux", new: newFullMux},
	}
	for _, s := range setups {
		for _, path := range unroutedPaths {
			t.Run(s.name+" "+path, func(t *testing.T) {
				t.Parallel()
				srv := s.new(t)

				rec := srv.get(t, path)

				assertContractNotFound(t, srv, rec)
			})
		}
	}
}

func TestRegister_CatchAllDoesNotShadowRoutes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
		check      func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "healthz", method: http.MethodGet, target: "/healthz", wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var body map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("body is not JSON: %v", err)
				}
				if got, want := body["status"], "ok"; got != want {
					t.Errorf("got status field %v, want %q", got, want)
				}
			},
		},
		{
			name: "openapi.json", method: http.MethodGet, target: "/openapi.json", wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) { assertSameSpec(t, rec.Body.Bytes()) },
		},
		{name: "list regions", method: http.MethodGet, target: "/v1/regions?lang=en", wantStatus: http.StatusNotImplemented, check: notImplemented},
		{name: "get region", method: http.MethodGet, target: "/v1/regions/gal?lang=es", wantStatus: http.StatusNotImplemented, check: notImplemented},
		{name: "list dishes", method: http.MethodGet, target: "/v1/dishes?lang=ru", wantStatus: http.StatusNotImplemented, check: notImplemented},
		{name: "get dish", method: http.MethodGet, target: "/v1/dishes/tortilla", wantStatus: http.StatusNotImplemented, check: notImplemented},
		{name: "get recipe", method: http.MethodGet, target: "/v1/dishes/tortilla/variations/gal/recipe", wantStatus: http.StatusNotImplemented, check: notImplemented},
		{name: "lang validation still runs", method: http.MethodGet, target: "/v1/regions?lang=fr", wantStatus: http.StatusBadRequest},
		{
			name: "legacy list dishes", method: http.MethodGet, target: "/dishes", wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var got []dish.Dish
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("body is not a JSON array of dishes: %v", err)
				}
				if len(got) != 1 {
					t.Errorf("got %d dishes, want 1", len(got))
				}
			},
		},
		{
			name: "legacy get dish", method: http.MethodGet, target: "/dishes/1", wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var got dish.Dish
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("body is not a JSON dish: %v", err)
				}
				if got.ID != 1 {
					t.Errorf("got id %d, want 1", got.ID)
				}
			},
		},
		{name: "legacy invalid id is still 400", method: http.MethodGet, target: "/dishes/abc", wantStatus: http.StatusBadRequest},
		{name: "legacy missing dish keeps legacy 404", method: http.MethodGet, target: "/dishes/999", wantStatus: http.StatusNotFound,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("body is not the legacy error shape: %v (body=%q)", err, rec.Body.String())
				}
				if got, want := body["error"], "dish not found"; got != want {
					t.Errorf("got error %q, want %q", got, want)
				}
			},
		},
		{
			name: "legacy count", method: http.MethodGet, target: "/dishes/total/count", wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var body map[string]int64
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("body is not JSON: %v", err)
				}
				if got, want := body["count"], int64(1); got != want {
					t.Errorf("got count %d, want %d", got, want)
				}
			},
		},
		{name: "legacy create", method: http.MethodPost, target: "/dishes", body: `{"name":"Pulpo a feira"}`, wantStatus: http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newFullMux(t)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))

			srv.mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("%s %s: got status %d, want %d (body=%.200q)", tt.method, tt.target, rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.check != nil {
				tt.check(t, rec)
			}
		})
	}
}

func notImplemented(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	body := decodeErrorBody(t, rec)
	if got, want := body.Error.Code, "not_implemented"; got != want {
		t.Errorf("got error code %q, want %q", got, want)
	}
}

// With a "/" catch-all on the mux, Go's ServeMux never answers 405 for these:
// a request whose method matches no pattern for its path still matches "/"
// (which accepts every method), so the catch-all handles it and returns 404.
// Only without a catch-all does ServeMux reply 405 with an Allow header. The
// test accepts either status but requires the contract Error shape on a 404.
func TestRegister_WrongMethodOnExistingPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method string
		target string
	}{
		{method: http.MethodPost, target: "/v1/regions"},
		{method: http.MethodDelete, target: "/v1/dishes/tortilla"},
		{method: http.MethodPut, target: "/v1/dishes/tortilla/variations/gal/recipe"},
		{method: http.MethodPost, target: "/openapi.json"},
		{method: http.MethodPost, target: "/healthz"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			t.Parallel()
			srv := newFullMux(t)
			rec := httptest.NewRecorder()

			srv.mux.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))

			switch rec.Code {
			case http.StatusNotFound:
				assertContractNotFound(t, srv, rec)
			case http.StatusMethodNotAllowed:
			default:
				t.Fatalf("got status %d, want %d or %d (body=%.200q)",
					rec.Code, http.StatusNotFound, http.StatusMethodNotAllowed, rec.Body.String())
			}
		})
	}
}
