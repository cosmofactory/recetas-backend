package db_test

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Schema tests run against the shared, migrated recetas_test inside a
// rolled-back transaction. Keys and sort orders are random so they never
// collide with rows from concurrent tests (or block on their uncommitted
// unique keys).

const (
	codeOK         = ""
	codeUnique     = "23505"
	codeCheck      = "23514"
	codeForeignKey = "23503"
	codeNotNull    = "23502"
)

const (
	keySuffixBytes    = 5
	sortOrderBase     = 1_000_000
	sortOrderSpread   = 1_000_000_000
	unknownID         = -1
	missingPosition   = 99
	firstPosition     = 1
	baselineTileColor = "#9A9A5E"
)

// tree holds the ids of one region → dish → variation → recipe chain.
type tree struct {
	dish, region, variation, recipe int64
}

func randHex(t *testing.T, n int) string {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(buf)
}

// uniqKey returns a valid, unique slug such as "gal-1a2b3c4d5e".
func uniqKey(t *testing.T, prefix string) string {
	t.Helper()
	return prefix + "-" + randHex(t, keySuffixBytes)
}

// uniqOrder returns a sort order unlikely to collide with any other test's.
func uniqOrder(t *testing.T) int {
	t.Helper()
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return sortOrderBase + int(binary.BigEndian.Uint32(buf[:])%sortOrderSpread)
}

// try runs one statement inside a savepoint, so a rejected row leaves tx
// usable, and returns its error.
func try(t *testing.T, tx pgx.Tx, sql string, args ...any) error {
	t.Helper()
	ctx := context.Background()
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	if _, execErr := sp.Exec(ctx, sql, args...); execErr != nil {
		if err := sp.Rollback(ctx); err != nil {
			t.Fatalf("rollback savepoint: %v", err)
		}
		return execErr
	}
	if err := sp.Commit(ctx); err != nil {
		t.Fatalf("release savepoint: %v", err)
	}
	return nil
}

func wantCode(t *testing.T, err error, want string) {
	t.Helper()
	if want == codeOK {
		if err != nil {
			t.Errorf("got err %v, want row accepted", err)
		}
		return
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Errorf("got err %v, want pg error %s", err, want)
		return
	}
	if pgErr.Code != want {
		t.Errorf("got pg error %s (%s), want %s", pgErr.Code, pgErr.Message, want)
	}
}

func mustID(t *testing.T, tx pgx.Tx, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("insert %q: %v", sql, err)
	}
	return id
}

func mustExec(t *testing.T, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// queryOne returns the single value of a one-row query.
func queryOne[T any](t *testing.T, tx pgx.Tx, sql string, args ...any) T {
	t.Helper()
	var v T
	if err := tx.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	return v
}

func countRows(t *testing.T, tx pgx.Tx, sql string, args ...any) int {
	t.Helper()
	return queryOne[int](t, tx, sql, args...)
}

func insertRegion(t *testing.T, tx pgx.Tx) int64 {
	t.Helper()
	return mustID(t, tx,
		`INSERT INTO regions (key, sort_order, tile_color) VALUES ($1, $2, $3) RETURNING id`,
		uniqKey(t, "reg"), uniqOrder(t), baselineTileColor)
}

func insertDish(t *testing.T, tx pgx.Tx) int64 {
	t.Helper()
	return mustID(t, tx,
		`INSERT INTO dishes (key, sort_order, course, season) VALUES ($1, $2, 'tapas', 'all') RETURNING id`,
		uniqKey(t, "dish"), uniqOrder(t))
}

func insertVariation(t *testing.T, tx pgx.Tx, dishID, regionID int64) int64 {
	t.Helper()
	return mustID(t, tx,
		`INSERT INTO variations (dish_id, region_id, recipe_key) VALUES ($1, $2, $3) RETURNING id`,
		dishID, regionID, uniqKey(t, "rec"))
}

func insertRecipe(t *testing.T, tx pgx.Tx, variationID int64) int64 {
	t.Helper()
	return mustID(t, tx,
		`INSERT INTO recipes (variation_id, time_minutes, base_servings) VALUES ($1, 25, 2) RETURNING id`,
		variationID)
}

func insertFullRecipe(t *testing.T, tx pgx.Tx) tree {
	t.Helper()
	var tr tree
	tr.region = insertRegion(t, tx)
	tr.dish = insertDish(t, tx)
	tr.variation = insertVariation(t, tx, tr.dish, tr.region)
	tr.recipe = insertRecipe(t, tx, tr.variation)
	return tr
}
