// Package api serves the HTTP contract in api/openapi.yaml through the
// generated apigen strict-server interface.
package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"

	apispec "github.com/cosmofactory/recetas-backend/api"
	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
	"github.com/cosmofactory/recetas-backend/internal/platform/telemetry"
)

var specJSON = sync.OnceValues(func() ([]byte, error) {
	doc, err := openapi3.NewLoader().LoadFromData(apispec.OpenAPIYAML)
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}
	raw, err := doc.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal openapi spec: %w", err)
	}
	return raw, nil
})

// SpecJSON returns the embedded OpenAPI document converted to JSON. The
// result is computed once; callers must not modify it.
func SpecJSON() ([]byte, error) {
	return specJSON()
}

// Register attaches every catalog operation, GET /openapi.json and a "/"
// catch-all that answers unrouted paths with the contract 404. /healthz is
// documented in the spec but registered by the health package. Because "/"
// matches every method, a wrong method on a known path also gets 404, not 405.
// Catalog operations answer If-None-Match through httpx.ConditionalGET, since
// the strict interface can't see request headers.
func Register(mux *http.ServeMux, impl apigen.StrictServerInterface, logger *slog.Logger) {
	strict := apigen.NewStrictHandlerWithOptions(impl, nil, strictOptions(logger))
	apigen.HandlerWithOptions(strict, apigen.StdHTTPServerOptions{
		BaseRouter:  mux,
		Middlewares: []apigen.MiddlewareFunc{validateLang, httpx.ConditionalGET},
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", paramErrorMessage(err))
		},
	})
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		raw, err := SpecJSON()
		if err != nil {
			logger.ErrorContext(r.Context(), "openapi spec unavailable", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
	})
}

// strictOptions answers request errors with 400 and handler errors with a
// generic 500, logged and recorded on the request span.
func strictOptions(logger *slog.Logger) apigen.StrictHTTPServerOptions {
	return apigen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid request")
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.ErrorContext(r.Context(), "api handler failed", "route", r.Pattern, "err", err)
			telemetry.RecordError(r.Context(), err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		},
	}
}

// paramErrorMessage names the offending parameter without echoing the
// binder's error text, which can include client input.
func paramErrorMessage(err error) string {
	var (
		required *apigen.RequiredParamError
		header   *apigen.RequiredHeaderError
		format   *apigen.InvalidParamFormatError
		tooMany  *apigen.TooManyValuesForParamError
		unmarsh  *apigen.UnmarshalingParamError
		cookie   *apigen.UnescapedCookieParamError
	)
	var name string
	switch {
	case errors.As(err, &required):
		name = required.ParamName
	case errors.As(err, &header):
		name = header.ParamName
	case errors.As(err, &format):
		name = format.ParamName
	case errors.As(err, &tooMany):
		name = tooMany.ParamName
	case errors.As(err, &unmarsh):
		name = unmarsh.ParamName
	case errors.As(err, &cookie):
		name = cookie.ParamName
	}
	if name == "" {
		return "invalid request"
	}
	return "invalid parameter " + name
}

// validateLang enforces the Locale enum, which the generated wrapper binds
// without checking. It runs after parameter binding, so lang is a single value.
func validateLang(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.URL.Query()["lang"]; ok && (len(v) != 1 || !apigen.Locale(v[0]).Valid()) {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "lang must be one of en, es, ru")
			return
		}
		next.ServeHTTP(w, r)
	})
}
