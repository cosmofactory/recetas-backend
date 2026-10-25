package database_test

// The production API these tests pin:
//
//	func ParseConfig(url string, tp trace.TracerProvider) (*pgxpool.Config, error)
//	    pool settings plus ConnConfig.Tracer = an *otelpgx.Tracer on tp that
//	    names spans "query <Query>" / "prepare <Query>" from the sqlc
//	    "-- name: <Query>" comment, sets db.operation.name to the bare
//	    <Query>, and never records query arguments.
//	func NewPool(ctx context.Context, url string, tp trace.TracerProvider) (*pgxpool.Pool, error)
//	    ParseConfig, connect, ping (as today).

import (
	"context"
	"strings"
	"testing"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/cosmofactory/recetas-backend/internal/platform/database"
	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
	"github.com/cosmofactory/recetas-backend/internal/testutil/spantest"
)

const (
	offlineURL = "postgres://u:p@127.0.0.1:5432/recetas?sslmode=disable" //nolint:gosec // never dialled
	// probeQuery mimics a sqlc-generated statement.
	probeQuery  = "-- name: ProbeArgumentCanary :one\nSELECT $1::text"
	probeName   = "ProbeArgumentCanary"
	querySpan   = "query " + probeName
	prepareSpan = "prepare " + probeName
	probeCanary = "arg-canary-7f3e9b"
)

func TestParseConfig_InstallsOtelpgxTracer(t *testing.T) {
	t.Parallel()
	tp, _ := spantest.New(t)
	cfg, err := database.ParseConfig(offlineURL, tp)
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	if _, ok := cfg.ConnConfig.Tracer.(*otelpgx.Tracer); !ok {
		t.Errorf("got ConnConfig.Tracer %T, want *otelpgx.Tracer", cfg.ConnConfig.Tracer)
	}
}

func TestParseConfig_MalformedURLIsAnError(t *testing.T) {
	t.Parallel()
	tp, _ := spantest.New(t)
	if _, err := database.ParseConfig("postgres://%zz", tp); err == nil {
		t.Error("got nil error for a malformed URL, want one")
	}
}

// tracedPool connects to the test database through NewPool, so the pool
// carries the production tracer on tp.
func tracedPool(t *testing.T, tp trace.TracerProvider) *pgxpool.Pool {
	t.Helper()
	url := testdb.New(t).Config().ConnConfig.ConnString()
	pool, err := database.NewPool(context.Background(), url, tp)
	if err != nil {
		t.Fatalf("got NewPool error %v, want nil", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestNewPool_QuerySpanIsNamedBySqlcQueryWithoutArguments(t *testing.T) {
	tp, spans := spantest.New(t)
	pool := tracedPool(t, tp)
	ctx, parent := spantest.StartServerSpan(context.Background(), tp)

	var got string
	if err := pool.QueryRow(ctx, probeQuery, probeCanary).Scan(&got); err != nil {
		t.Fatalf("probe query: %v", err)
	}
	parent.End()

	query := namedSpan(t, spans.Ended(), querySpan)
	if query.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Errorf("got query span parent %s, want the request span %s", query.Parent().SpanID(), parent.SpanContext().SpanID())
	}
	if op, _ := spantest.Attr(query, "db.operation.name"); op.AsString() != probeName {
		t.Errorf("got db.operation.name %q, want the bare query name %q", op.AsString(), probeName)
	}
	// A fresh pool has an empty statement cache, so pgx prepares first.
	prepare := namedSpan(t, spans.Ended(), prepareSpan)
	if prepare.Parent().SpanID() != query.SpanContext().SpanID() {
		t.Errorf("got prepare span parent %s, want the query span %s", prepare.Parent().SpanID(), query.SpanContext().SpanID())
	}
	if leaks := spantest.AttrsContaining(spans.Ended(), probeCanary); len(leaks) != 0 {
		t.Errorf("got span attributes %v, want query arguments not recorded", leaks)
	}
	if strings.Contains(query.Name(), probeCanary) {
		t.Errorf("got span name %q, want no arguments in it", query.Name())
	}
}

func namedSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	s, ok := spantest.Named(spans, name)
	if !ok {
		t.Fatalf("got spans %v, want one named %q", spantest.Names(spans), name)
	}
	return s
}
