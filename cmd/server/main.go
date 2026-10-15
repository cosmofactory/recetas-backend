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

	"github.com/cosmosfactory/recetas-backend/internal/config"
	"github.com/cosmosfactory/recetas-backend/internal/dish"
	"github.com/cosmosfactory/recetas-backend/internal/dish/dishdb"
	"github.com/cosmosfactory/recetas-backend/internal/platform/database"
	"github.com/cosmosfactory/recetas-backend/internal/platform/logging"
)

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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	queries := dishdb.New(pool)
	repo := dish.NewPGRepository(queries)
	svc := dish.NewService(repo)
	handler := dish.NewHandler(svc, logger)

	mux := http.NewServeMux()
	handler.Register(mux)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "app_env", cfg.AppEnv, "addr", cfg.HTTPAddr, "log_level", cfg.LogLevel)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}
	// Restore default signal handling so a second Ctrl-C/SIGTERM kills the process.
	stop()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Closing connections cancels in-flight request contexts, so their
		// queries abort and the deferred pool.Close doesn't block.
		_ = srv.Close()
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}
