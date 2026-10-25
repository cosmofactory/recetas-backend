package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/cosmofactory/recetas-backend/internal/api"
	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/config"
	"github.com/cosmofactory/recetas-backend/internal/platform/database"
	"github.com/cosmofactory/recetas-backend/internal/platform/health"
	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
	"github.com/cosmofactory/recetas-backend/internal/platform/telemetry"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		logging.New(os.Stdout, slog.LevelInfo).Error("load config failed", "err", err)
		os.Exit(1)
	}
	if err := run(cfg, os.Stdout); err != nil {
		os.Exit(1)
	}
}

// run logs its own failure, so the error reaches telemetry before the one
// flush, which runs after the server has drained (or never started).
func run(cfg config.Config, stdout io.Writer) error {
	tel, err := telemetry.Setup(context.Background(), telemetryConfig(cfg))
	if err != nil {
		logging.New(stdout, cfg.LogLevel).Error("telemetry setup failed", "err", err)
		return fmt.Errorf("telemetry: %w", err)
	}
	logger := newLogger(cfg, stdout, tel.LoggerProvider)
	logStartup(logger, cfg)

	return serve(cfg, tel.TracerProvider, shutdownSteps{flush: tel.Shutdown, closePool: func() {}, logger: logger})
}

func telemetryConfig(cfg config.Config) telemetry.Config {
	tc := cfg.Telemetry
	tc.ServiceVersion = version
	return tc
}

func newLogger(cfg config.Config, stdout io.Writer, lp otellog.LoggerProvider) *slog.Logger {
	if !cfg.Telemetry.Enabled() {
		return logging.New(stdout, cfg.LogLevel)
	}
	return logging.NewTee(stdout, cfg.LogLevel, lp)
}

// logStartup runs before the database is reached, so a boot that fails there
// still shows which build it was.
func logStartup(logger *slog.Logger, cfg config.Config) {
	logger.Info("starting", "version", version, "app_env", cfg.AppEnv)
	if !cfg.Telemetry.Enabled() {
		logger.Info("telemetry disabled")
	}
}

// serve fills in the server and pool steps as it opens them, so every exit
// flushes telemetry and then closes whatever pool is open.
func serve(cfg config.Config, tp trace.TracerProvider, steps shutdownSteps) error {
	if _, err := api.SpecJSON(); err != nil {
		return abort(steps, fmt.Errorf("openapi spec: %w", err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, database.Options{URL: cfg.DatabaseURL, TracerProvider: tp, Size: cfg.Pool})
	if err != nil {
		return abort(steps, fmt.Errorf("database: %w", err))
	}
	steps.closePool = pool.Close

	srv := httpx.NewServer(cfg.HTTPAddr, newHandler(pool, steps.logger, tp), steps.logger)
	steps.logger.Info("server starting", "addr", cfg.HTTPAddr, "log_level", cfg.LogLevel)
	if err := serveUntilDone(ctx, srv); err != nil {
		return abort(steps, fmt.Errorf("server: %w", err))
	}
	// Restore default signal handling so a second Ctrl-C/SIGTERM kills the process.
	stop()
	steps.server = srv
	return gracefulShutdown(steps)
}

func newHandler(pool *pgxpool.Pool, logger *slog.Logger, tp trace.TracerProvider) http.Handler {
	mux := http.NewServeMux()
	health.NewHandler(pool, logger).Register(mux)
	catalogHandler := catalog.NewHandler(catalog.NewService(catalog.NewPGRepository(pool)), logger)
	api.Register(mux, catalogHandler, logger)
	return httpx.Stack(mux, logger, tp)
}

// serveUntilDone returns when ctx is cancelled or the server fails to serve.
func serveUntilDone(ctx context.Context, srv *http.Server) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- listenAndServe(srv) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		return nil
	}
}

func listenAndServe(srv *http.Server) error {
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
