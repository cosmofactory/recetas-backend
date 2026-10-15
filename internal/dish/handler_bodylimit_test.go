package dish_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/dish"
	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
)

type contractErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type contractError struct {
	Error contractErrorDetail `json:"error"`
}

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
			dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
			dec.DisallowUnknownFields()
			var body contractError
			if err := dec.Decode(&body); err != nil {
				t.Fatalf("body is not the contract Error shape: %v (body=%q)", err, rec.Body.String())
			}
			want := contractError{Error: contractErrorDetail{Code: "payload_too_large", Message: "request body too large"}}
			if body != want {
				t.Errorf("got body %+v, want %+v", body, want)
			}
		})
	}
}
