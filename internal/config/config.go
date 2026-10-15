package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
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
}

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

	dbURL := getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	addr := getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	return Config{
		AppEnv:      appEnv,
		LogLevel:    level,
		DatabaseURL: dbURL,
		HTTPAddr:    addr,
	}, nil
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

func (c Config) String() string {
	dsn := "unset"
	if c.DatabaseURL != "" {
		dsn = "set"
	}
	return fmt.Sprintf("Config{AppEnv: %s, LogLevel: %s, HTTPAddr: %s, DatabaseURL: %s}", c.AppEnv, c.LogLevel, c.HTTPAddr, dsn)
}
