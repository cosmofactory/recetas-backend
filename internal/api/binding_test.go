package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api"
)

func TestRegister_ParamBindingErrorHasFixedMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		target      string
		wantMessage string
	}{
		{name: "repeated lang on list", target: "/v1/regions?lang=en&lang=ru", wantMessage: "invalid parameter lang"},
		{name: "repeated identical lang", target: "/v1/regions?lang=en&lang=en", wantMessage: "invalid parameter lang"},
		{name: "repeated lang on dishes", target: "/v1/dishes?lang=es&lang=ru", wantMessage: "invalid parameter lang"},
		{name: "repeated lang on detail", target: "/v1/regions/gal?lang=en&lang=ru", wantMessage: "invalid parameter lang"},
		{name: "repeated lang on recipe", target: "/v1/dishes/tortilla/variations/gal/recipe?lang=en&lang=ru", wantMessage: "invalid parameter lang"},
		{name: "unknown lang keeps enum message", target: "/v1/regions?lang=fr", wantMessage: "lang must be one of en, es, ru"},
		{name: "uppercase lang keeps enum message", target: "/v1/dishes/tortilla?lang=EN", wantMessage: "lang must be one of en, es, ru"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newTestServer(t, api.Stub{})

			rec := srv.get(t, tt.target)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got status %d, want %d (body=%.200q)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			body := decodeErrorBody(t, rec)
			if got, want := body.Error.Code, "bad_request"; got != want {
				t.Errorf("got error code %q, want %q", got, want)
			}
			if got := body.Error.Message; got != tt.wantMessage {
				t.Errorf("got error message %q, want %q", got, tt.wantMessage)
			}
			if lines := logLinesWithLevel(t, srv.logs, "ERROR"); len(lines) != 0 {
				t.Errorf("got %d ERROR log lines, want 0 (logs=%q)", len(lines), srv.logs.String())
			}
		})
	}
}

// If-None-Match is not a declared parameter: the handler reads it from the
// request headers itself, so repeated headers must not fail binding.
func TestRegister_RepeatedIfNoneMatchIsNotABindingError(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, api.Stub{})
	req := httptest.NewRequest(http.MethodGet, "/v1/regions?lang=en", nil)
	req.Header.Add("If-None-Match", `"a"`)
	req.Header.Add("If-None-Match", `"b"`)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("got status %d, want %d (body=%.200q)", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "If-None-Match") {
		t.Errorf("got body %q, want no mention of If-None-Match", rec.Body.String())
	}
}
