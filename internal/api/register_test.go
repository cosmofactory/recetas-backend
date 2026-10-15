package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api"
	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
	"github.com/cosmofactory/recetas-backend/internal/platform/health"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
)

// validPathParams holds a valid value for every path parameter the spec uses.
// A new path parameter fails TestRegister_EveryCatalogOperationIsWired until
// it is added here.
var validPathParams = map[string]string{
	"dishKey":   "tortilla",
	"regionKey": "gal",
}

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

type specOperation struct {
	id     string
	method string
	target string
}

func catalogOperations(t *testing.T) []specOperation {
	t.Helper()
	doc := loadFileSpec(t)
	var ops []specOperation
	for path, item := range doc.Paths.Map() {
		target := pathParam.ReplaceAllStringFunc(path, func(m string) string {
			name := m[1 : len(m)-1]
			v, ok := validPathParams[name]
			if !ok {
				t.Fatalf("path %s: no valid value for path parameter %q in validPathParams", path, name)
			}
			return v
		})
		for method, op := range item.Operations() {
			if !slices.Contains(op.Tags, "catalog") {
				continue
			}
			ops = append(ops, specOperation{id: op.OperationID, method: method, target: target})
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].id < ops[j].id })
	if len(ops) == 0 {
		t.Fatal("got 0 catalog operations in the spec, want at least one")
	}
	return ops
}

func TestRegister_EveryCatalogOperationIsWired(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, api.Stub{})
	for _, op := range catalogOperations(t) {
		t.Run(op.id, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, httptest.NewRequest(op.method, op.target+"?lang=ru", nil))

			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("%s %s: got status %d, want %d (body=%.200q)",
					op.method, op.target, rec.Code, http.StatusNotImplemented, rec.Body.String())
			}
			body := decodeErrorBody(t, rec)
			if got, want := body.Error.Code, "not_implemented"; got != want {
				t.Errorf("got error code %q, want %q", got, want)
			}
			if got, want := body.Error.Message, "not implemented"; got != want {
				t.Errorf("got error message %q, want %q", got, want)
			}
		})
	}
}

func TestRegister_DoesNotClaimHealthz(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		order func(mux *http.ServeMux, logger *slog.Logger)
	}{
		{
			name: "api then health",
			order: func(mux *http.ServeMux, logger *slog.Logger) {
				api.Register(mux, api.Stub{}, logger)
				health.NewHandler(okPinger{}, logger).Register(mux)
			},
		},
		{
			name: "health then api",
			order: func(mux *http.ServeMux, logger *slog.Logger) {
				health.NewHandler(okPinger{}, logger).Register(mux)
				api.Register(mux, api.Stub{}, logger)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			logger := logging.New(&logs, slog.LevelDebug)
			mux := http.NewServeMux()
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("got panic %v registering both handlers, want none", r)
					}
				}()
				tt.order(mux, logger)
			}()

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("GET /healthz: got status %d, want %d (body=%.200q)", rec.Code, http.StatusOK, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("GET /healthz: body is not JSON: %v", err)
			}
			if got, want := body["status"], "ok"; got != want {
				t.Errorf("GET /healthz: got status field %v, want %q", got, want)
			}
		})
	}
}

func TestRegister_LangValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantCode   string
	}{
		{name: "no lang is optional", target: "/v1/regions", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{name: "lang en", target: "/v1/regions?lang=en", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{name: "lang es", target: "/v1/regions?lang=es", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{name: "lang ru", target: "/v1/regions?lang=ru", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{name: "unknown lang", target: "/v1/regions?lang=fr", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{name: "lang is case-sensitive", target: "/v1/regions?lang=EN", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{name: "unknown lang on detail route", target: "/v1/dishes/tortilla?lang=fr", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{name: "unknown lang on recipe route", target: "/v1/dishes/tortilla/variations/gal/recipe?lang=de", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newTestServer(t, api.Stub{})

			rec := srv.get(t, tt.target)

			if rec.Code != tt.wantStatus {
				t.Fatalf("got status %d, want %d (body=%.200q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			body := decodeErrorBody(t, rec)
			if body.Error.Code != tt.wantCode {
				t.Errorf("got error code %q, want %q", body.Error.Code, tt.wantCode)
			}
			if body.Error.Message == "" {
				t.Error("got empty error message, want a non-empty one")
			}
			if lines := logLinesWithLevel(t, srv.logs, "ERROR"); len(lines) != 0 {
				t.Errorf("got %d ERROR log lines, want 0 (logs=%q)", len(lines), srv.logs.String())
			}
		})
	}
}

const internalErrText = "pg: connection refused: secret-internal-detail"

type failingRegions struct {
	api.Stub
}

func (failingRegions) ListRegions(context.Context, apigen.ListRegionsRequestObject) (apigen.ListRegionsResponseObject, error) {
	return nil, errors.New(internalErrText)
}

func TestRegister_ResponseErrorIsGeneric500AndLogged(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, failingRegions{})

	rec := srv.get(t, "/v1/regions?lang=es")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d (body=%.200q)", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-internal-detail") {
		t.Errorf("got body %q, want it not to contain the internal error text", rec.Body.String())
	}
	body := decodeErrorBody(t, rec)
	if got, want := body.Error.Code, "internal"; got != want {
		t.Errorf("got error code %q, want %q", got, want)
	}
	if got, want := body.Error.Message, "internal error"; got != want {
		t.Errorf("got error message %q, want %q", got, want)
	}

	lines := logLinesWithLevel(t, srv.logs, "ERROR")
	if len(lines) != 1 {
		t.Fatalf("got %d ERROR log lines, want 1 (logs=%q)", len(lines), srv.logs.String())
	}
	raw, err := json.Marshal(lines[0])
	if err != nil {
		t.Fatalf("re-marshal log line: %v", err)
	}
	if !strings.Contains(string(raw), "secret-internal-detail") {
		t.Errorf("got log line %s, want it to contain the internal error text", raw)
	}
	if _, ok := lines[0]["err"]; !ok {
		t.Errorf("got log line %s, want an err attr", raw)
	}
}
