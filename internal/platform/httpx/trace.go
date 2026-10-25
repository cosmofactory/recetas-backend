package httpx

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/cosmofactory/recetas-backend/internal/platform/health"
)

const serverOperation = "http.server"

// otherMethod names spans for methods outside the standard set, which a
// client could otherwise use to create unbounded span names.
const otherMethod = "HTTP"

var standardMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true,
	http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	http.MethodConnect: true, http.MethodOptions: true, http.MethodTrace: true,
}

// Trace starts a server span per request on tp, continuing a W3C
// traceparent. Health probes get no span. Routed renames the span after
// routing, since the mux sets the pattern on a request clone.
func Trace(tp trace.TracerProvider) Middleware {
	return otelhttp.NewMiddleware(serverOperation,
		otelhttp.WithTracerProvider(tp),
		otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelhttp.WithPropagators(propagation.TraceContext{}),
		otelhttp.WithFilter(func(r *http.Request) bool { return !isHealthProbe(r) }),
	)
}

func isHealthProbe(r *http.Request) bool {
	return r.URL.Path == health.Path
}

// nameServerSpan names the span in ctx "<METHOD> <route>" and sets
// http.route, so neither carries the raw path.
func nameServerSpan(ctx context.Context, method, pattern string) {
	route := routeOf(pattern)
	if route == "" {
		return
	}
	if !standardMethods[method] {
		method = otherMethod
	}
	span := trace.SpanFromContext(ctx)
	span.SetName(method + " " + route)
	span.SetAttributes(semconv.HTTPRoute(route))
}

// routeOf strips the method and host from a ServeMux pattern.
func routeOf(pattern string) string {
	if i := strings.IndexByte(pattern, '/'); i >= 0 {
		return pattern[i:]
	}
	return ""
}
