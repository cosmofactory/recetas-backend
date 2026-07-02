
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/cosmosfactory/recetas-backend/internal/config"
	"github.com/cosmosfactory/recetas-backend/internal/dish"
	"github.com/cosmosfactory/recetas-backend/internal/dish/dishdb"
	"github.com/cosmosfactory/recetas-backend/internal/platform/database"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
			log.Fatalf("config: %v", err)
	}

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
			log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	queries := dishdb.New(pool)
	repo := dish.NewPGRepository(queries)
	svc := dish.NewService(repo)
	handler := dish.NewHandler(svc)

	mux := http.NewServeMux()
	handler.Register(mux)

	srv := &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
			log.Printf("listening on %s", cfg.HTTPAddr)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Fatalf("server: %v", err)
			}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("shutdown: %v", err)
	}
	log.Println("bye")
}