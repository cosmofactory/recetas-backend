package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

const (
	tracesPath = "/v1/traces"
	logsPath   = "/v1/logs"
	authHeader = "Authorization"
)

// export is what both OTLP/HTTP exporters share.
type export struct {
	baseURL    string
	headers    map[string]string
	httpClient *http.Client
	resource   *resource.Resource
}

func newExport(cfg Config, opts []Option) export {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return export{
		baseURL:    strings.TrimRight(cfg.Endpoint, "/"),
		headers:    map[string]string{authHeader: cfg.Token},
		httpClient: o.httpClient,
		resource:   newResource(cfg),
	}
}

func newResource(cfg Config) *resource.Resource {
	return resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentNameKey.String(cfg.Environment),
	)
}

func newTracerProvider(ctx context.Context, ex export) (*sdktrace.TracerProvider, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(ex.baseURL + tracesPath),
		otlptracehttp.WithHeaders(ex.headers),
	}
	if ex.httpClient != nil {
		opts = append(opts, otlptracehttp.WithHTTPClient(ex.httpClient))
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}
	return sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(ex.resource)), nil
}

func newLoggerProvider(ctx context.Context, ex export) (*sdklog.LoggerProvider, error) {
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpointURL(ex.baseURL + logsPath),
		otlploghttp.WithHeaders(ex.headers),
	}
	if ex.httpClient != nil {
		opts = append(opts, otlploghttp.WithHTTPClient(ex.httpClient))
	}
	exporter, err := otlploghttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("log exporter: %w", err)
	}
	processor := sdklog.NewBatchProcessor(exporter)
	return sdklog.NewLoggerProvider(sdklog.WithProcessor(processor), sdklog.WithResource(ex.resource)), nil
}
