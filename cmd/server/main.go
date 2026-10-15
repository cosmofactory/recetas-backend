package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cosmofactory/recetas-backend/internal/api"
	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/config"
	"github.com/cosmofactory/recetas-backend/internal/platform/database"
	"github.com/cosmofactory/recetas-backend/internal/platform/health"
	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const shutdownTimeout = 5 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		logging.New(os.Stdout, slog.LevelInfo).Error("load config failed", "err", err)
		os.Exit(1)
	}

	logger := logging.New(os.Stdout, cfg.LogLevel)
	if err := run(cfg, logger); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	if _, err := api.SpecJSON(); err != nil {
		return fmt.Errorf("openapi spec: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	srv := httpx.NewServer(cfg.HTTPAddr, newHandler(pool, logger), logger)
	logger.Info("server starting", "version", version, "app_env", cfg.AppEnv, "addr", cfg.HTTPAddr, "log_level", cfg.LogLevel)
	if err := serveUntilDone(ctx, srv); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	// Restore default signal handling so a second Ctrl-C/SIGTERM kills the process.
	stop()
	return shutdown(srv, logger)
}

func newHandler(pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	health.NewHandler(pool, logger).Register(mux)
	catalogHandler := catalog.NewHandler(catalog.NewService(catalog.NewPGRepository(pool)), logger)
	api.Register(mux, catalogHandler, logger)

	// MaxBody is outermost so MaxBytesReader sees net/http's own ResponseWriter and a 413 closes the connection.
	return httpx.Chain(httpx.Routed(mux),
		httpx.MaxBody(httpx.MaxBodyBytes),
		httpx.AccessLog(logger),
		httpx.Recover(logger),
		httpx.Deadline(httpx.HandlerTimeout),
	)
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

func shutdown(srv *http.Server, logger *slog.Logger) error {
	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		// Closing connections cancels in-flight request contexts, so their
		// queries abort and the deferred pool.Close doesn't block.
		return fmt.Errorf("shutdown: %w", errors.Join(err, srv.Close()))
	}
	logger.Info("server stopped")
	return nil
}
