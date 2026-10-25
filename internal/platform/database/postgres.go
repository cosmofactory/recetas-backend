package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

const (
	maxConns        = 10
	minConns        = 1
	maxConnLifetime = time.Hour
	maxConnIdleTime = 30 * time.Minute
	pingTimeout     = 5 * time.Second
)

// sqlcNamePrefix starts the comment sqlc puts before every query.
const sqlcNamePrefix = "-- name: "

// ParseConfig returns the pool settings with query tracing on tp. Spans are
// named "query <Name>", "prepare <Name>" (pgx's statement cache) or "batch
// query <Name>"; db.operation.name is the bare name. Query arguments are
// never recorded.
func ParseConfig(url string, tp trace.TracerProvider) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse postgres url: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(
		otelpgx.WithTracerProvider(tp),
		otelpgx.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelpgx.WithSpanNameFunc(queryName),
		otelpgx.WithQuerySpanNamePrefix(),
	)
	return cfg, nil
}

func NewPool(ctx context.Context, url string, tp trace.TracerProvider) (*pgxpool.Pool, error) {
	cfg, err := ParseConfig(url, tp)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

// queryName is the sqlc query name, or the SQL operation (e.g. SELECT) for
// statements sqlc didn't generate.
func queryName(stmt string) string {
	rest, ok := strings.CutPrefix(strings.TrimSpace(stmt), sqlcNamePrefix)
	if fields := strings.Fields(rest); ok && len(fields) > 0 {
		return fields[0]
	}
	return otelpgx.SQLOperationName(context.Background(), stmt)
}
