// Tests for internal/platform/telemetry. The production API they pin:
//
//	type Config struct {
//		Token          string // LOGFIRE_TOKEN; blank disables telemetry
//		Endpoint       string // OTLP/HTTP base URL (https); /v1/traces and /v1/logs are appended
//		ServiceName    string
//		ServiceVersion string // main.version
//		Environment    string // APP_ENV → deployment.environment.name
//	}
//	func (Config) Enabled() bool            // a non-blank token
//	func (Config) String() string           // never contains Token
//	func (Config) LogValue() slog.Value     // never contains Token
//
//	type Providers struct {
//		TracerProvider trace.TracerProvider  // go.opentelemetry.io/otel/trace
//		LoggerProvider log.LoggerProvider    // go.opentelemetry.io/otel/log
//		Shutdown       func(context.Context) error // flushes both batch processors
//	}
//
//	type Option func(*options)
//	func WithHTTPClient(c *http.Client) Option // exporters' client; tests trust an httptest TLS server
//	func Setup(ctx context.Context, cfg Config, opts ...Option) (Providers, error)
//
// Setup with a disabled Config returns no-op providers and a no-op Shutdown
// and touches no network; the caller logs "telemetry disabled". Enabled, it
// exports over OTLP/HTTP with "Authorization: <token>" through batch
// processors. Setup must not depend on (or require tests to reset) global
// OTel state.
package telemetry_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/cosmofactory/recetas-backend/internal/platform/telemetry"
)

const (
	testToken   = "pylf_v1_eu_TOKENs3cret" //nolint:gosec // fake token used to check redaction
	tracesPath  = "/v1/traces"
	logsPath    = "/v1/logs"
	testService = "recetas-backend"
	testVersion = "sha-abc1234"
	testEnv     = "demo"
	testSpan    = "test-span"
	// emitTimeout bounds how long ending a span or emitting a log may take
	// while the collector stalls; a synchronous exporter would block forever.
	emitTimeout = 2 * time.Second
)

func enabledConfig(endpoint string) telemetry.Config {
	return telemetry.Config{
		Token:          testToken,
		Endpoint:       endpoint,
		ServiceName:    testService,
		ServiceVersion: testVersion,
		Environment:    testEnv,
	}
}

func setup(t *testing.T, cfg telemetry.Config, c *collector) telemetry.Providers {
	t.Helper()
	p, err := telemetry.Setup(context.Background(), cfg, telemetry.WithHTTPClient(c.srv.Client()))
	if err != nil {
		t.Fatalf("got Setup error %v, want nil", err)
	}
	if p.TracerProvider == nil || p.LoggerProvider == nil || p.Shutdown == nil {
		t.Fatalf("got Providers %+v, want TracerProvider, LoggerProvider and Shutdown set", p)
	}
	return p
}

func shutdown(t *testing.T, p telemetry.Providers) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("got Shutdown error %v, want nil", err)
	}
}

func emitSpanAndLog(p telemetry.Providers) {
	ctx, span := p.TracerProvider.Tracer("test").Start(context.Background(), testSpan)
	span.End()
	var rec otellog.Record
	rec.SetBody(attribute.StringValue("test log"))
	rec.SetSeverity(otellog.SeverityInfo)
	p.LoggerProvider.Logger("test").Emit(ctx, rec)
}

func TestSetup_DisabledReturnsNoopProvidersAndMakesNoNetworkCalls(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		token string
	}{
		{name: "unset token", token: ""},
		{name: "blank token", token: "  \t "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newCollector(t)
			cfg := enabledConfig(c.srv.URL)
			cfg.Token = tt.token
			p := setup(t, cfg, c)

			assertNoopProviders(t, p)
			emitSpanAndLog(p)
			shutdown(t, p)

			if got := c.requests(); len(got) != 0 {
				t.Errorf("got %d requests to the collector %v, want 0", len(got), c.paths())
			}
		})
	}
}

func assertNoopProviders(t *testing.T, p telemetry.Providers) {
	t.Helper()
	_, span := p.TracerProvider.Tracer("test").Start(context.Background(), testSpan)
	defer span.End()
	if span.IsRecording() {
		t.Error("got a recording span, want a no-op tracer")
	}
	if span.SpanContext().IsValid() {
		t.Errorf("got valid span context %v, want an invalid one from a no-op tracer", span.SpanContext())
	}
	if p.LoggerProvider.Logger("test").Enabled(context.Background(), otellog.EnabledParameters{}) {
		t.Error("got an enabled logger, want a no-op logger provider")
	}
}

func TestSetup_DisabledIgnoresInvalidEndpoint(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"", "http://insecure.example", "::not a url"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			p, err := telemetry.Setup(context.Background(), telemetry.Config{Endpoint: endpoint})
			if err != nil {
				t.Fatalf("got Setup error %v, want nil when telemetry is disabled", err)
			}
			shutdown(t, p)
		})
	}
}

func TestSetup_EnabledRejectsNonHTTPSEndpointWithoutLeakingToken(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"", "http://logfire.example", "logfire.example", "https://"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			_, err := telemetry.Setup(context.Background(), enabledConfig(endpoint))
			if err == nil {
				t.Fatalf("got nil error for endpoint %q, want an https URL to be required", endpoint)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Errorf("got error %q, want it not to contain the token", err)
			}
		})
	}
}

func TestSetup_EnabledExportsTracesWithAuthAndResource(t *testing.T) {
	t.Parallel()
	for _, suffix := range []string{"", "/"} {
		t.Run("endpoint suffix "+suffix, func(t *testing.T) {
			t.Parallel()
			c := newCollector(t)
			p := setup(t, enabledConfig(c.srv.URL+suffix), c)

			emitSpanAndLog(p)
			shutdown(t, p)

			traces := c.requestsTo(tracesPath)
			if len(traces) == 0 {
				t.Fatalf("got requests to %v, want at least one to %s", c.paths(), tracesPath)
			}
			assertAuthorization(t, traces[0])
			assertTraceExport(t, decodeTraces(t, traces[0]))
		})
	}
}

func assertAuthorization(t *testing.T, r exportRequest) {
	t.Helper()
	if r.authorization != testToken {
		t.Errorf("got Authorization %q on %s, want the bare token %q", r.authorization, r.path, testToken)
	}
}

func assertTraceExport(t *testing.T, msg *coltracepb.ExportTraceServiceRequest) {
	t.Helper()
	attrs := resourceAttrs(msg)
	want := map[string]string{
		"service.name":                testService,
		"service.version":             testVersion,
		"deployment.environment.name": testEnv,
	}
	for k, v := range want {
		if attrs[k] != v {
			t.Errorf("got resource attribute %s=%q, want %q (all=%v)", k, attrs[k], v, attrs)
		}
	}
	if names := spanNames(msg); !slices.Contains(names, testSpan) {
		t.Errorf("got exported spans %v, want %q among them", names, testSpan)
	}
}

func TestSetup_EnabledExportsLogsWithAuth(t *testing.T) {
	t.Parallel()
	c := newCollector(t)
	p := setup(t, enabledConfig(c.srv.URL), c)

	emitSpanAndLog(p)
	shutdown(t, p)

	logs := c.requestsTo(logsPath)
	if len(logs) == 0 {
		t.Fatalf("got requests to %v, want at least one to %s", c.paths(), logsPath)
	}
	assertAuthorization(t, logs[0])
	if !containsBytes(logs[0].body, "test log") {
		t.Errorf("got log export body without the emitted record %q", "test log")
	}
}

// Batch processors export in the background: a stalled or unreachable
// collector must not slow down the request that ends a span or logs.
func TestSetup_EnabledExportDoesNotBlockTheCaller(t *testing.T) {
	t.Parallel()
	c := newHeldCollector(t)
	p := setup(t, enabledConfig(c.srv.URL), c)

	done := make(chan struct{})
	go func() {
		defer close(done)
		emitSpanAndLog(p)
	}()
	select {
	case <-done:
	case <-time.After(emitTimeout):
		t.Fatalf("ending a span and emitting a log blocked for %v on a stalled collector, want batched export", emitTimeout)
	}
	c.unhold()
	shutdown(t, p)
}

func TestConfig_NeverRendersToken(t *testing.T) {
	t.Parallel()
	cfg := enabledConfig("https://logfire-eu.pydantic.dev")
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("config", "telemetry", cfg)

	renderings := map[string]string{
		"String":    cfg.String(),
		"%v":        fmt.Sprintf("%v", cfg),
		"%+v":       fmt.Sprintf("%+v", cfg),
		"slog JSON": logs.String(),
	}
	for name, got := range renderings {
		if strings.Contains(got, testToken) {
			t.Errorf("got %s rendering %q, want it not to contain the token", name, got)
		}
		if !strings.Contains(got, testService) {
			t.Errorf("got %s rendering %q, want it to still show the service name %q", name, got, testService)
		}
	}
}

func TestConfig_Enabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		token string
		want  bool
	}{
		{token: "", want: false},
		{token: "   ", want: false},
		{token: testToken, want: true},
	}
	for _, tt := range tests {
		if got := (telemetry.Config{Token: tt.token}).Enabled(); got != tt.want {
			t.Errorf("got Enabled()=%v for token %q, want %v", got, tt.token, tt.want)
		}
	}
}
