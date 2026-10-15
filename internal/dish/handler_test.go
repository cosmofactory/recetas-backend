package dish_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cosmosfactory/recetas-backend/internal/dish"
)

const internalErrText = "pg: connection refused to 10.0.0.5 secret-detail"

type fakeRepo struct {
	createFn func(ctx context.Context, name, description string) (dish.Dish, error)
	listFn   func(ctx context.Context) ([]dish.Dish, error)
	getFn    func(ctx context.Context, id int64) (dish.Dish, error)
	countFn  func(ctx context.Context) (int64, error)
}

func (f *fakeRepo) Create(ctx context.Context, name, description string) (dish.Dish, error) {
	if f.createFn == nil {
		return dish.Dish{}, errors.New("fakeRepo: Create not configured")
	}
	return f.createFn(ctx, name, description)
}

func (f *fakeRepo) List(ctx context.Context) ([]dish.Dish, error) {
	if f.listFn == nil {
		return nil, errors.New("fakeRepo: List not configured")
	}
	return f.listFn(ctx)
}

func (f *fakeRepo) Get(ctx context.Context, id int64) (dish.Dish, error) {
	if f.getFn == nil {
		return dish.Dish{}, errors.New("fakeRepo: Get not configured")
	}
	return f.getFn(ctx, id)
}

func (f *fakeRepo) Count(ctx context.Context) (int64, error) {
	if f.countFn == nil {
		return 0, errors.New("fakeRepo: Count not configured")
	}
	return f.countFn(ctx)
}

var _ dish.Repository = (*fakeRepo)(nil)

type testServer struct {
	mux  *http.ServeMux
	logs *bytes.Buffer
}

func newTestServer(t *testing.T, repo dish.Repository) testServer {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := dish.NewHandler(dish.NewService(repo), logger)
	mux := http.NewServeMux()
	h.Register(mux)
	return testServer{mux: mux, logs: &logs}
}

func (s testServer) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)
	return rec
}

func errorLogLines(t *testing.T, logs *bytes.Buffer) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(strings.TrimRight(logs.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %v (line=%q)", err, line)
		}
		if rec["level"] == "ERROR" {
			out = append(out, line)
		}
	}
	return out
}

func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not a JSON object of strings: %v (body=%q)", err, rec.Body.String())
	}
	return body
}

func TestHandler_RepositoryErrorReturnsGeneric500AndLogsOnce(t *testing.T) {
	t.Parallel()
	repoErr := errors.New(internalErrText)
	tests := []struct {
		name   string
		repo   *fakeRepo
		method string
		target string
		body   string
	}{
		{
			name:   "list",
			repo:   &fakeRepo{listFn: func(context.Context) ([]dish.Dish, error) { return nil, repoErr }},
			method: http.MethodGet,
			target: "/dishes",
		},
		{
			name:   "get",
			repo:   &fakeRepo{getFn: func(context.Context, int64) (dish.Dish, error) { return dish.Dish{}, repoErr }},
			method: http.MethodGet,
			target: "/dishes/1",
		},
		{
			name:   "count",
			repo:   &fakeRepo{countFn: func(context.Context) (int64, error) { return 0, repoErr }},
			method: http.MethodGet,
			target: "/dishes/total/count",
		},
		{
			name: "create",
			repo: &fakeRepo{createFn: func(context.Context, string, string) (dish.Dish, error) {
				return dish.Dish{}, repoErr
			}},
			method: http.MethodPost,
			target: "/dishes",
			body:   `{"name":"Tortilla","description":"Potato omelette"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newTestServer(t, tt.repo)
			rec := srv.do(t, tt.method, tt.target, tt.body)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("got status %d, want %d (body=%q)", rec.Code, http.StatusInternalServerError, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("got Content-Type %q, want application/json", ct)
			}
			body := decodeErrorBody(t, rec)
			want := map[string]string{"error": "internal error"}
			if len(body) != len(want) || body["error"] != want["error"] {
				t.Errorf("got body %v, want %v", body, want)
			}
			for _, leak := range []string{internalErrText, "secret-detail", "10.0.0.5"} {
				if strings.Contains(rec.Body.String(), leak) {
					t.Errorf("got body %q, want it not to contain %q", rec.Body.String(), leak)
				}
			}

			lines := errorLogLines(t, srv.logs)
			if len(lines) != 1 {
				t.Fatalf("got %d ERROR log lines, want 1 (logs=%q)", len(lines), srv.logs.String())
			}
			if !strings.Contains(lines[0], internalErrText) {
				t.Errorf("got ERROR log line %q, want it to contain %q", lines[0], internalErrText)
			}
		})
	}
}

func TestHandler_ClientErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		repo       *fakeRepo
		method     string
		target     string
		body       string
		wantStatus int
	}{
		{
			name: "get unknown id",
			repo: &fakeRepo{getFn: func(context.Context, int64) (dish.Dish, error) {
				return dish.Dish{}, dish.ErrNotFound
			}},
			method:     http.MethodGet,
			target:     "/dishes/999",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "get non-numeric id",
			repo:       &fakeRepo{},
			method:     http.MethodGet,
			target:     "/dishes/abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "get id overflowing int64",
			repo:       &fakeRepo{},
			method:     http.MethodGet,
			target:     "/dishes/99999999999999999999",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create malformed json",
			repo:       &fakeRepo{},
			method:     http.MethodPost,
			target:     "/dishes",
			body:       `{"name":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create non-object json",
			repo:       &fakeRepo{},
			method:     http.MethodPost,
			target:     "/dishes",
			body:       `"tortilla"`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create missing name",
			repo:       &fakeRepo{},
			method:     http.MethodPost,
			target:     "/dishes",
			body:       `{"description":"Potato omelette"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create empty name",
			repo:       &fakeRepo{},
			method:     http.MethodPost,
			target:     "/dishes",
			body:       `{"name":"","description":"Potato omelette"}`,
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newTestServer(t, tt.repo)
			rec := srv.do(t, tt.method, tt.target, tt.body)

			if rec.Code != tt.wantStatus {
				t.Fatalf("got status %d, want %d (body=%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			body := decodeErrorBody(t, rec)
			if body["error"] == "" {
				t.Errorf("got body %v, want non-empty \"error\" field", body)
			}
			if lines := errorLogLines(t, srv.logs); len(lines) != 0 {
				t.Errorf("got %d ERROR log lines, want 0 for a client error (logs=%q)", len(lines), srv.logs.String())
			}
		})
	}
}

func TestHandler_ListReturnsDishes(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	dishes := []dish.Dish{
		{ID: 1, Name: "Tortilla de patatas", Description: "Potato and egg omelette", CreatedAt: ts, UpdatedAt: ts},
		{ID: 2, Name: "Pulpo á feira", Description: "Galician-style octopus", CreatedAt: ts, UpdatedAt: ts},
	}
	srv := newTestServer(t, &fakeRepo{listFn: func(context.Context) ([]dish.Dish, error) { return dishes, nil }})

	rec := srv.do(t, http.MethodGet, "/dishes", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("got Content-Type %q, want application/json", ct)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not a JSON array: %v (body=%q)", err, rec.Body.String())
	}
	if len(got) != len(dishes) {
		t.Fatalf("got %d dishes, want %d", len(got), len(dishes))
	}
	for i, want := range dishes {
		if got[i]["id"] != float64(want.ID) {
			t.Errorf("dish %d: got id %v, want %d", i, got[i]["id"], want.ID)
		}
		if got[i]["name"] != want.Name {
			t.Errorf("dish %d: got name %v, want %q", i, got[i]["name"], want.Name)
		}
		if got[i]["description"] != want.Description {
			t.Errorf("dish %d: got description %v, want %q", i, got[i]["description"], want.Description)
		}
	}
	if lines := errorLogLines(t, srv.logs); len(lines) != 0 {
		t.Errorf("got %d ERROR log lines, want 0 (logs=%q)", len(lines), srv.logs.String())
	}
}

func TestHandler_ListEmptyReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &fakeRepo{listFn: func(context.Context) ([]dish.Dish, error) { return []dish.Dish{}, nil }})

	rec := srv.do(t, http.MethodGet, "/dishes", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("got body %q, want %q", got, "[]")
	}
}
