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

// The default pool size (DB_MAX_CONNS/DB_MIN_CONNS when unset). Ten
// connections serve millisecond-scale catalog reads at well above the
// target load and leave most of Postgres' default max_connections (100)
// to the import CLI and operators.
const (
	DefaultMaxConns = 10
	DefaultMinConns = 1
)

const (
	maxConnLifetime = time.Hour
	maxConnIdleTime = 30 * time.Minute
	pingTimeout     = 5 * time.Second
)

// sqlcNamePrefix starts the comment sqlc puts before every query.
const sqlcNamePrefix = "-- name: "

// PoolSize bounds the number of open connections.
type PoolSize struct {
	MaxConns int32
	MinConns int32
}

func DefaultPoolSize() PoolSize {
	return PoolSize{MaxConns: DefaultMaxConns, MinConns: DefaultMinConns}
}

// Options configures a pool. Size wins over any pool_* parameters in URL; a
// zero Size means DefaultPoolSize.
type Options struct {
	URL            string
	TracerProvider trace.TracerProvider
	Size           PoolSize
}

// ParseConfig returns the pool settings with query tracing on the tracer
// provider. Spans are
// named "query <Name>", "prepare <Name>" (pgx's statement cache) or "batch
// query <Name>"; db.operation.name is the bare name. Query arguments are
// never recorded.
func ParseConfig(opts Options) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres url: %w", err)
	}
	size := opts.Size
	if size == (PoolSize{}) {
		size = DefaultPoolSize()
	}
	cfg.MaxConns = size.MaxConns
	cfg.MinConns = size.MinConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(
		otelpgx.WithTracerProvider(opts.TracerProvider),
		otelpgx.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelpgx.WithSpanNameFunc(queryName),
		otelpgx.WithQuerySpanNamePrefix(),
	)
	return cfg, nil
}

func NewPool(ctx context.Context, opts Options) (*pgxpool.Pool, error) {
	cfg, err := ParseConfig(opts)
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
