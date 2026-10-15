package db_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

const (
	scratchDBPrefix      = "recetas_mig_"
	scratchDBSuffixBytes = 6
)

// domainTables is the BE-3 schema. Every one must exist after `up` and be
// gone after `down-to 0`.
var domainTables = []string{
	"locales",
	"regions",
	"region_translations",
	"region_regional_dishes",
	"dishes",
	"dish_translations",
	"variations",
	"variation_translations",
	"recipes",
	"recipe_translations",
	"recipe_ingredients",
	"recipe_ingredient_translations",
	"recipe_steps",
	"recipe_step_translations",
	"content_versions",
}

// scratchDB creates an empty database on the TEST_DATABASE_URL server and
// drops it when the test ends. Migrations run there, never in the shared
// recetas_test: a `down` would wipe it under tests in other packages.
func scratchDB(t *testing.T) *sql.DB {
	t.Helper()

	admin := testdb.New(t)
	name := scratchDBPrefix + randomSuffix(t)
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
	// Registered before db.Close below, so it runs after it.
	t.Cleanup(func() { dropDB(t, admin, name) })

	cfg := admin.Config().ConnConfig.Copy()
	cfg.Database = name
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("ping %s: %v", name, err)
	}
	return db
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	buf := make([]byte, scratchDBSuffixBytes)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random db name: %v", err)
	}
	return hex.EncodeToString(buf)
}

// dropDB drops the scratch database; FORCE (PG 13+) terminates any
// connection still open to it.
func dropDB(t *testing.T, admin *pgxpool.Pool, name string) {
	t.Helper()
	stmt := "DROP DATABASE IF EXISTS " + pgx.Identifier{name}.Sanitize() + " WITH (FORCE)"
	if _, err := admin.Exec(context.Background(), stmt); err != nil {
		t.Errorf("drop database %s: %v", name, err)
	}
}

func newProvider(t *testing.T, db *sql.DB) *goose.Provider {
	t.Helper()
	p, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("migrations"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func publicTables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	return queryStrings(t, db, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		  AND table_name <> 'goose_db_version'
		ORDER BY table_name`)
}

func queryStrings(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

var wantLocales = []string{"en", "es", "ru"}

func assertMigratedSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	tables := publicTables(t, db)
	for _, want := range domainTables {
		if !slices.Contains(tables, want) {
			t.Errorf("table %q: missing after up (got tables %v)", want, tables)
		}
	}
	assertNoLegacyDishColumns(t, db)
	if slices.Contains(tables, "locales") {
		assertLocalesSeeded(t, db)
	}
}

// assertNoLegacyDishColumns checks the iteration-1 shape (dishes.name,
// dishes.description) is gone: names are per-locale in dish_translations.
func assertNoLegacyDishColumns(t *testing.T, db *sql.DB) {
	t.Helper()
	oldCols := queryStrings(t, db, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'dishes'
		  AND column_name IN ('name', 'description')
		ORDER BY column_name`)
	if len(oldCols) != 0 {
		t.Errorf("dishes iteration-1 columns: got %v, want none", oldCols)
	}
}

func assertLocalesSeeded(t *testing.T, db *sql.DB) {
	t.Helper()
	got := queryStrings(t, db, `SELECT code FROM locales ORDER BY code`)
	if !slices.Equal(got, wantLocales) {
		t.Errorf("locales: got %v, want %v", got, wantLocales)
	}
}

func assertEmptySchema(t *testing.T, db *sql.DB) {
	t.Helper()

	if tables := publicTables(t, db); len(tables) != 0 {
		t.Errorf("tables after down-to 0: got %v, want none", tables)
	}
	funcs := queryStrings(t, db, `
		SELECT p.proname FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public'
		ORDER BY p.proname`)
	if len(funcs) != 0 {
		t.Errorf("functions after down-to 0: got %v, want none", funcs)
	}
}

func TestIntegrationMigrationsUpDownUp(t *testing.T) {
	db := scratchDB(t)
	p := newProvider(t, db)
	ctx := context.Background()

	if len(p.ListSources()) == 0 {
		t.Fatal("migrations: got 0 sources, want at least 1")
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("first up: got err %v, want nil", err)
	}
	t.Run("after first up", func(t *testing.T) { assertMigratedSchema(t, db) })

	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down-to 0: got err %v, want nil", err)
	}
	t.Run("after down-to 0", func(t *testing.T) { assertEmptySchema(t, db) })

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("second up: got err %v, want nil", err)
	}
	t.Run("after second up", func(t *testing.T) { assertMigratedSchema(t, db) })
}
