package config_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/cosmosfactory/recetas-backend/internal/config"
)

const testDSN = "postgres://u:s3cret@h:5432/db" //nolint:gosec // fake DSN used to check redaction

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func validEnv() map[string]string {
	return map[string]string{
		"APP_ENV":      "local",
		"DATABASE_URL": testDSN,
	}
}

func with(overrides map[string]string) map[string]string {
	m := validEnv()
	for k, v := range overrides {
		if v == "" {
			delete(m, k)
			continue
		}
		m[k] = v
	}
	return m
}

func TestFromEnv_AppEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		want    config.AppEnv
		wantErr bool
	}{
		{name: "local", value: "local", want: config.EnvLocal},
		{name: "demo", value: "demo", want: config.EnvDemo},
		{name: "prod", value: "prod", want: config.EnvProd},
		{name: "missing", value: "", wantErr: true},
		{name: "unknown", value: "staging", wantErr: true},
		{name: "uppercase", value: "PROD", wantErr: true},
		{name: "mixed case", value: "Local", wantErr: true},
		{name: "surrounding whitespace", value: " prod ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.FromEnv(envFrom(with(map[string]string{"APP_ENV": tt.value})))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("got nil error, want error naming APP_ENV (cfg=%v)", cfg)
				}
				if !strings.Contains(err.Error(), "APP_ENV") {
					t.Errorf("got error %q, want it to name APP_ENV", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if cfg.AppEnv != tt.want {
				t.Errorf("got AppEnv %q, want %q", cfg.AppEnv, tt.want)
			}
		})
	}
}

func TestFromEnv_AppEnvConstants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		got  config.AppEnv
		want string
	}{
		{config.EnvLocal, "local"},
		{config.EnvDemo, "demo"},
		{config.EnvProd, "prod"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestFromEnv_LogLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		want    slog.Level
		wantErr bool
	}{
		{name: "default when empty", value: "", want: slog.LevelInfo},
		{name: "debug", value: "debug", want: slog.LevelDebug},
		{name: "info", value: "info", want: slog.LevelInfo},
		{name: "warn", value: "warn", want: slog.LevelWarn},
		{name: "error", value: "error", want: slog.LevelError},
		{name: "unknown", value: "verbose", wantErr: true},
		{name: "warning spelled out", value: "warning", wantErr: true},
		{name: "numeric", value: "4", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.FromEnv(envFrom(with(map[string]string{"LOG_LEVEL": tt.value})))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("got nil error, want error naming LOG_LEVEL (cfg=%v)", cfg)
				}
				if !strings.Contains(err.Error(), "LOG_LEVEL") {
					t.Errorf("got error %q, want it to name LOG_LEVEL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if cfg.LogLevel != tt.want {
				t.Errorf("got LogLevel %v, want %v", cfg.LogLevel, tt.want)
			}
		})
	}
}

func TestFromEnv_DatabaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "set", value: testDSN},
		{name: "missing", value: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.FromEnv(envFrom(with(map[string]string{"DATABASE_URL": tt.value})))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("got nil error, want error naming DATABASE_URL")
				}
				if !strings.Contains(err.Error(), "DATABASE_URL") {
					t.Errorf("got error %q, want it to name DATABASE_URL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if cfg.DatabaseURL != tt.value {
				t.Errorf("got DatabaseURL %q, want %q", cfg.DatabaseURL, tt.value)
			}
		})
	}
}

func TestFromEnv_HTTPAddr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "default when empty", value: "", want: ":8080"},
		{name: "explicit", value: "127.0.0.1:9000", want: "127.0.0.1:9000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.FromEnv(envFrom(with(map[string]string{"HTTP_ADDR": tt.value})))
			if err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if cfg.HTTPAddr != tt.want {
				t.Errorf("got HTTPAddr %q, want %q", cfg.HTTPAddr, tt.want)
			}
		})
	}
}

func TestFromEnv_FullValidEnv(t *testing.T) {
	t.Parallel()
	cfg, err := config.FromEnv(envFrom(map[string]string{
		"APP_ENV":      "prod",
		"LOG_LEVEL":    "warn",
		"DATABASE_URL": testDSN,
		"HTTP_ADDR":    ":9090",
	}))
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	want := config.Config{
		AppEnv:      config.EnvProd,
		LogLevel:    slog.LevelWarn,
		DatabaseURL: testDSN,
		HTTPAddr:    ":9090",
	}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestConfigString_RedactsDatabaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  config.Config
	}{
		{
			name: "prod",
			cfg: config.Config{
				AppEnv:      config.EnvProd,
				LogLevel:    slog.LevelInfo,
				DatabaseURL: testDSN,
				HTTPAddr:    ":8080",
			},
		},
		{
			name: "local custom addr",
			cfg: config.Config{
				AppEnv:      config.EnvLocal,
				LogLevel:    slog.LevelDebug,
				DatabaseURL: testDSN,
				HTTPAddr:    "127.0.0.1:9000",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := tt.cfg.String()
			for _, secret := range []string{testDSN, "s3cret", "u:s3cret"} {
				if strings.Contains(s, secret) {
					t.Errorf("got String() %q, want it not to contain %q", s, secret)
				}
			}
			if !strings.Contains(s, string(tt.cfg.AppEnv)) {
				t.Errorf("got String() %q, want it to contain app env %q", s, tt.cfg.AppEnv)
			}
			if !strings.Contains(s, tt.cfg.HTTPAddr) {
				t.Errorf("got String() %q, want it to contain HTTP addr %q", s, tt.cfg.HTTPAddr)
			}
		})
	}
}
