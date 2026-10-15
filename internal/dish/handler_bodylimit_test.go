package dish_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/dish"
	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
)

// createBody returns a valid create-dish JSON object of exactly size bytes.
func createBody(t *testing.T, size int) string {
	t.Helper()
	const prefix = `{"name":"Tortilla","description":"`
	const suffix = `"}`
	n := size - len(prefix) - len(suffix)
	if n < 0 {
		t.Fatalf("size %d too small for a create body", size)
	}
	return prefix + strings.Repeat("a", n) + suffix
}

func TestHandler_CreateBodyLimit(t *testing.T) {
	t.Parallel()
	limit := int(httpx.MaxBodyBytes)
	tests := []struct {
		name       string
		size       int
		wantStatus int
	}{
		{name: "small body", size: 128, wantStatus: http.StatusCreated},
		{name: "exactly at limit", size: limit, wantStatus: http.StatusCreated},
		{name: "one byte over limit", size: limit + 1, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "100 KiB description", size: 100<<10 + 64, wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			created := false
			repo := &fakeRepo{createFn: func(_ context.Context, name, description string) (dish.Dish, error) {
				created = true
				return dish.Dish{ID: 1, Name: name, Description: description}, nil
			}}
			srv := newTestServer(t, repo)
			h := httpx.MaxBody(httpx.MaxBodyBytes)(srv.mux)
			r := httptest.NewRequest(http.MethodPost, "/dishes", strings.NewReader(createBody(t, tt.size)))
			r.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, r)

			if rec.Code != tt.wantStatus {
				t.Fatalf("got status %d, want %d (body=%.200q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if lines := errorLogLines(t, srv.logs); len(lines) != 0 {
				t.Errorf("got %d ERROR log lines, want 0 (logs=%q)", len(lines), srv.logs.String())
			}
			if tt.wantStatus == http.StatusCreated {
				if !created {
					t.Error("got repository Create not called, want it called")
				}
				return
			}
			if created {
				t.Error("got repository Create called, want it not called for an oversized body")
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("got Content-Type %q, want application/json", ct)
			}
			body := decodeErrorBody(t, rec)
			if len(body) != 1 || body["error"] != "request body too large" {
				t.Errorf("got body %v, want %v", body, map[string]string{"error": "request body too large"})
			}
		})
	}
}
