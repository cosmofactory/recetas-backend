// Command import loads a content bundle into the database. See
// docs/content-import.md.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/cosmofactory/recetas-backend/internal/platform/database"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// main reads only DATABASE_URL (from .env too, when present); APP_ENV and the
// server's other settings don't apply to an import.
func main() {
	_ = godotenv.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	db := &lazyPool{url: os.Getenv("DATABASE_URL")}
	code := run(ctx, os.Args[1:], stdio{in: os.Stdin, out: os.Stdout, err: os.Stderr}, db)
	db.close()
	stop()
	os.Exit(code)
}

// lazyPool connects on the first Begin, so an invalid bundle never needs the
// database.
type lazyPool struct {
	url  string
	pool *pgxpool.Pool
}

func (p *lazyPool) Begin(ctx context.Context) (pgx.Tx, error) {
	if p.pool == nil {
		if p.url == "" {
			return nil, fmt.Errorf("DATABASE_URL is required")
		}
		pool, err := database.NewPool(ctx, p.url)
		if err != nil {
			return nil, err
		}
		p.pool = pool
	}
	return p.pool.Begin(ctx)
}

func (p *lazyPool) close() {
	if p.pool != nil {
		p.pool.Close()
	}
}
