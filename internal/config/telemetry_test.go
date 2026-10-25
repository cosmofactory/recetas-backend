package config_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/config"
	"github.com/cosmofactory/recetas-backend/internal/platform/database"
	"github.com/cosmofactory/recetas-backend/internal/platform/telemetry"
)

// Config.Telemetry is a telemetry.Config. FromEnv fills it only when
// LOGFIRE_TOKEN is non-blank; otherwise it stays the zero value and no other
// telemetry variable is read or validated. ServiceVersion is left for main
// (main.version).

const (
	testToken    = "pylf_v1_eu_TOKENs3cret" //nolint:gosec // fake token used to check redaction
	testEndpoint = "https://logfire-eu.pydantic.dev"
)

func withTelemetry(overrides map[string]string) map[string]string {
	base := map[string]string{
		"LOGFIRE_TOKEN":               testToken,
		"OTEL_EXPORTER_OTLP_ENDPOINT": testEndpoint,
	}
	for k, v := range overrides {
		base[k] = v
	}
	return with(base)
}

func TestFromEnv_TelemetryDisabledWithoutToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "token unset", env: with(nil)},
		{name: "token blank", env: with(map[string]string{"LOGFIRE_TOKEN": "  \t "})},
		{name: "other telemetry vars ignored", env: with(map[string]string{
			"LOGFIRE_TOKEN":               " ",
			"OTEL_EXPORTER_OTLP_ENDPOINT": "http://not-https.example",
			"OTEL_SERVICE_NAME":           "ignored",
		})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.FromEnv(envFrom(tt.env))
			if err != nil {
				t.Fatalf("got error %v, want nil when telemetry is off", err)
			}
			if cfg.Telemetry != (telemetry.Config{}) {
				t.Errorf("got Telemetry %#v, want the zero value", cfg.Telemetry)
			}
			if cfg.Telemetry.Enabled() {
				t.Error("got Telemetry.Enabled()=true, want false")
			}
		})
	}
}

func TestFromEnv_TelemetryEnabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  map[string]string
		want telemetry.Config
	}{
		{
			name: "defaults service name, maps APP_ENV",
			env:  withTelemetry(map[string]string{"APP_ENV": "demo"}),
			want: telemetry.Config{Token: testToken, Endpoint: testEndpoint, ServiceName: "recetas-backend", Environment: "demo"},
		},
		{
			name: "explicit service name",
			env:  withTelemetry(map[string]string{"APP_ENV": "prod", "OTEL_SERVICE_NAME": "recetas-api"}),
			want: telemetry.Config{Token: testToken, Endpoint: testEndpoint, ServiceName: "recetas-api", Environment: "prod"},
		},
		{
			name: "token is trimmed",
			env:  withTelemetry(map[string]string{"LOGFIRE_TOKEN": "  " + testToken + "\n"}),
			want: telemetry.Config{Token: testToken, Endpoint: testEndpoint, ServiceName: "recetas-backend", Environment: "local"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.FromEnv(envFrom(tt.env))
			if err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if cfg.Telemetry != tt.want {
				t.Errorf("got Telemetry %#v, want %#v", cfg.Telemetry, tt.want)
			}
			if !cfg.Telemetry.Enabled() {
				t.Error("got Telemetry.Enabled()=false, want true")
			}
		})
	}
}

func TestFromEnv_TelemetryEndpointMustBeHTTPS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{name: "https", endpoint: testEndpoint},
		{name: "https trailing slash", endpoint: testEndpoint + "/"},
		{name: "https with port", endpoint: "https://otel.example:4318"},
		{name: "missing", endpoint: "", wantErr: true},
		{name: "http", endpoint: "http://logfire-eu.pydantic.dev", wantErr: true},
		{name: "no scheme", endpoint: "logfire-eu.pydantic.dev", wantErr: true},
		{name: "no host", endpoint: "https://", wantErr: true},
		{name: "not a url", endpoint: "::nope", wantErr: true},
		{name: "grpc scheme", endpoint: "grpc://logfire-eu.pydantic.dev", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := withTelemetry(nil)
			env["OTEL_EXPORTER_OTLP_ENDPOINT"] = tt.endpoint
			_, err := config.FromEnv(envFrom(env))
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("got error %v, want nil", err)
				}
				return
			}
			assertEndpointError(t, err)
		})
	}
}

func assertEndpointError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("got nil error, want one naming OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if !strings.Contains(err.Error(), "OTEL_EXPORTER_OTLP_ENDPOINT") {
		t.Errorf("got error %q, want it to name OTEL_EXPORTER_OTLP_ENDPOINT", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Errorf("got error %q, want it not to contain the token", err)
	}
}

func TestFromEnv_FullValidEnvWithTelemetry(t *testing.T) {
	t.Parallel()
	cfg, err := config.FromEnv(envFrom(map[string]string{
		"APP_ENV":                     "demo",
		"LOG_LEVEL":                   "debug",
		"DATABASE_URL":                testDSN,
		"HTTP_ADDR":                   ":9090",
		"LOGFIRE_TOKEN":               testToken,
		"OTEL_EXPORTER_OTLP_ENDPOINT": testEndpoint,
		"OTEL_SERVICE_NAME":           "recetas-api",
	}))
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	want := config.Config{
		AppEnv:      config.EnvDemo,
		LogLevel:    slog.LevelDebug,
		DatabaseURL: testDSN,
		HTTPAddr:    ":9090",
		Pool:        database.PoolSize{MaxConns: 10, MinConns: 1},
		Telemetry: telemetry.Config{
			Token:       testToken,
			Endpoint:    testEndpoint,
			ServiceName: "recetas-api",
			Environment: "demo",
		},
	}
	if cfg != want {
		t.Errorf("got %#v, want %#v", cfg, want)
	}
}

func TestConfig_NeverRendersTelemetryToken(t *testing.T) {
	t.Parallel()
	cfg, err := config.FromEnv(envFrom(withTelemetry(nil)))
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("config loaded", "config", cfg)

	renderings := map[string]string{
		"String":    cfg.String(),
		"%v":        fmt.Sprintf("%v", cfg),
		"%+v":       fmt.Sprintf("%+v", cfg),
		"slog JSON": logs.String(),
	}
	for name, got := range renderings {
		for _, secret := range []string{testToken, "s3cret"} {
			if strings.Contains(got, secret) {
				t.Errorf("got %s rendering %q, want it not to contain %q", name, got, secret)
			}
		}
	}
}
