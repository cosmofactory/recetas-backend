// Package testdb provides Postgres fixtures for integration tests.
//
// Tests opt in through TEST_DATABASE_URL, never DATABASE_URL: `make test`
// loads .env, whose DATABASE_URL points at the dev database, and tests must
// not touch it implicitly. The target database must already be migrated.
package testdb

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvVar names the variable that points tests at a migrated database.
const EnvVar = "TEST_DATABASE_URL"

const pingTimeout = 5 * time.Second

// New returns a pool connected to TEST_DATABASE_URL and closes it when the
// test ends. It skips the test when the variable is unset and fails it when
// the database is configured but unreachable. In CI (CI set) an unset
// variable fails the test instead of skipping it.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv(EnvVar)
	if url == "" {
		// CI must run integration tests, never silently skip them.
		if os.Getenv("CI") != "" {
			t.Fatal(EnvVar + " not set in CI")
		}
		t.Skip(EnvVar + " not set")
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("testdb: create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("testdb: ping %s: %v", EnvVar, err)
	}
	return pool
}

// Tx begins a transaction on a fresh pool and rolls it back when the test
// ends, so nothing a test writes is ever committed.
func Tx(t testing.TB) pgx.Tx {
	t.Helper()

	pool := New(t)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("testdb: begin tx: %v", err)
	}
	// Registered after pool.Close, so it runs first. t.Context() is already
	// cancelled by the time cleanups run.
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("testdb: rollback: %v", err)
		}
	})
	return tx
}
