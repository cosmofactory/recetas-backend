package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"github.com/cosmofactory/recetas-backend/internal/platform/database"
	"github.com/cosmofactory/recetas-backend/internal/platform/telemetry"
)

type AppEnv string

const (
	EnvLocal AppEnv = "local"
	EnvDemo  AppEnv = "demo"
	EnvProd  AppEnv = "prod"
)

type Config struct {
	AppEnv      AppEnv
	LogLevel    slog.Level
	DatabaseURL string
	HTTPAddr    string
	Pool        database.PoolSize
	Telemetry   telemetry.Config
}

const (
	defaultHTTPAddr    = ":8080"
	defaultServiceName = "recetas-backend"
	endpointVar        = "OTEL_EXPORTER_OTLP_ENDPOINT"
)

func Load() (Config, error) {
	_ = godotenv.Load()
	return FromEnv(os.Getenv)
}

func FromEnv(getenv func(string) string) (Config, error) {
	appEnv, err := parseAppEnv(getenv("APP_ENV"))
	if err != nil {
		return Config{}, err
	}

	level, err := parseLogLevel(getenv("LOG_LEVEL"))
	if err != nil {
		return Config{}, err
	}

	dbURL, pool, err := parseDatabase(getenv)
	if err != nil {
		return Config{}, err
	}

	tel, err := parseTelemetry(getenv, appEnv)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:      appEnv,
		LogLevel:    level,
		DatabaseURL: dbURL,
		HTTPAddr:    withDefault(getenv("HTTP_ADDR"), defaultHTTPAddr),
		Pool:        pool,
		Telemetry:   tel,
	}, nil
}

func parseDatabase(getenv func(string) string) (string, database.PoolSize, error) {
	dbURL := getenv("DATABASE_URL")
	if dbURL == "" {
		return "", database.PoolSize{}, errors.New("DATABASE_URL is required")
	}
	pool, err := parsePool(getenv)
	if err != nil {
		return "", database.PoolSize{}, err
	}
	return dbURL, pool, nil
}

// parseTelemetry reads nothing else when LOGFIRE_TOKEN is blank, so local
// development needs no telemetry settings.
func parseTelemetry(getenv func(string) string, appEnv AppEnv) (telemetry.Config, error) {
	token := strings.TrimSpace(getenv("LOGFIRE_TOKEN"))
	if token == "" {
		return telemetry.Config{}, nil
	}
	endpoint := strings.TrimSpace(getenv(endpointVar))
	if endpoint == "" {
		return telemetry.Config{}, errors.New(endpointVar + " is required when LOGFIRE_TOKEN is set")
	}
	if err := telemetry.ValidateEndpoint(endpoint); err != nil {
		return telemetry.Config{}, fmt.Errorf("%s %w", endpointVar, err)
	}
	return telemetry.Config{
		Token:       token,
		Endpoint:    endpoint,
		ServiceName: withDefault(strings.TrimSpace(getenv("OTEL_SERVICE_NAME")), defaultServiceName),
		Environment: string(appEnv),
	}, nil
}

func withDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func parseAppEnv(v string) (AppEnv, error) {
	switch e := AppEnv(v); e {
	case EnvLocal, EnvDemo, EnvProd:
		return e, nil
	case "":
		return "", errors.New("APP_ENV is required (local|demo|prod)")
	default:
		return "", fmt.Errorf("APP_ENV %q is invalid (want local|demo|prod)", v)
	}
}

func parseLogLevel(v string) (slog.Level, error) {
	switch v {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL %q is invalid (want debug|info|warn|error)", v)
	}
}

// String and LogValue never render the DSN or the telemetry token.
func (c Config) String() string {
	return fmt.Sprintf("Config{AppEnv: %s, LogLevel: %s, HTTPAddr: %s, DatabaseURL: %s, Pool: %+v, Telemetry: %s}",
		c.AppEnv, c.LogLevel, c.HTTPAddr, c.databaseURLState(), c.Pool, c.Telemetry)
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("app_env", string(c.AppEnv)),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("http_addr", c.HTTPAddr),
		slog.String("database_url", c.databaseURLState()),
		slog.Group("pool",
			slog.Int("max_conns", int(c.Pool.MaxConns)),
			slog.Int("min_conns", int(c.Pool.MinConns)),
		),
		slog.Any("telemetry", c.Telemetry),
	)
}

func (c Config) databaseURLState() string {
	if c.DatabaseURL == "" {
		return "unset"
	}
	return "set"
}
