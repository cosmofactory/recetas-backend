package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The iteration-1 CRUD surface must stay gone: every method answers the
// contract 404 from the catch-all.
func TestRegister_LegacyDishRoutesAreContract404(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method string
		target string
		body   string
	}{
		{method: http.MethodPost, target: "/dishes", body: `{"name":"Tortilla","description":"x"}`},
		{method: http.MethodGet, target: "/dishes"},
		{method: http.MethodGet, target: "/dishes/1"},
		{method: http.MethodPut, target: "/dishes/1", body: `{"name":"Tortilla","description":"x"}`},
		{method: http.MethodDelete, target: "/dishes/1"},
		{method: http.MethodPost, target: "/v1/dishes", body: `{"name":"Tortilla","description":"x"}`},
		{method: http.MethodPut, target: "/v1/dishes/tortilla", body: `{"name":"Tortilla","description":"x"}`},
		{method: http.MethodDelete, target: "/v1/dishes/tortilla"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			t.Parallel()
			srv := newFullMux(t)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			srv.mux.ServeHTTP(rec, req)

			assertContractNotFound(t, srv, rec)
		})
	}
}

// catchAllPattern is what ServeMux reports for a request no route claims.
const catchAllPattern = "/"

var probedMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

// The reverse of TestRegister_EveryCatalogOperationIsWired: on every spec
// path, a method the spec doesn't list must fall through to the catch-all,
// so the mux serves nothing the contract doesn't describe.
func TestRegister_OnlySpecMethodsAreRoutedOnSpecPaths(t *testing.T) {
	t.Parallel()
	srv := newFullMux(t)
	for _, op := range specRoutes(t) {
		for _, method := range probedMethods {
			req := httptest.NewRequest(method, op.target, nil)
			_, pattern := srv.mux.Handler(req)
			if routedBySpec(op, method, pattern) {
				continue
			}
			t.Errorf("%s %s: got pattern %q, want %q (not in api/openapi.yaml)", method, op.target, pattern, catchAllPattern)
		}
	}
}

// routedBySpec allows the catch-all, the spec's own method and the HEAD that
// ServeMux adds to every GET pattern.
func routedBySpec(op specRoute, method, pattern string) bool {
	if pattern == catchAllPattern {
		return true
	}
	return op.methods[method] || (method == http.MethodHead && op.methods[http.MethodGet])
}

type specRoute struct {
	target  string
	methods map[string]bool
}

// specRoutes is every spec path (catalog, /healthz and /openapi.json) with
// valid path parameters and the methods the spec lists for it.
func specRoutes(t *testing.T) []specRoute {
	t.Helper()
	var routes []specRoute
	for path, item := range loadFileSpec(t).Paths.Map() {
		route := specRoute{target: pathParam.ReplaceAllStringFunc(path, validParam(t)), methods: map[string]bool{}}
		for method := range item.Operations() {
			route.methods[strings.ToUpper(method)] = true
		}
		routes = append(routes, route)
	}
	return routes
}

func validParam(t *testing.T) func(string) string {
	t.Helper()
	return func(m string) string {
		v, ok := validPathParams[m[1:len(m)-1]]
		if !ok {
			t.Fatalf("no valid value for path parameter %s in validPathParams", m)
		}
		return v
	}
}
