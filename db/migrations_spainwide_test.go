package db_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
)

// versionBeforeSpainWide is the last migration before BE-9's; the BE-9
// migration is the next one (D-ESP-1, D-ESP-4).
const versionBeforeSpainWide = 10

// populatedPreSpainWide is a scratch database at versionBeforeSpainWide with
// regional variations inserted out of region order:
//
//	regions  north (sort 1), south (sort 2)
//	tortilla [south, north]   inserted in that order
//	paella   [south]
type populatedPreSpainWide struct {
	db                                        *sql.DB
	provider                                  *goose.Provider
	tortilla, paella                          int64
	tortillaSouth, tortillaNorth, paellaSouth int64
}

func newPopulatedPreSpainWide(t *testing.T) populatedPreSpainWide {
	t.Helper()
	db := scratchDB(t)
	p := newProvider(t, db)
	if _, err := p.UpTo(context.Background(), versionBeforeSpainWide); err != nil {
		t.Fatalf("up to %d: %v", versionBeforeSpainWide, err)
	}
	north := sqlID(t, db, `INSERT INTO regions (key, sort_order, tile_color) VALUES ('north', 1, '#9A9A5E') RETURNING id`)
	south := sqlID(t, db, `INSERT INTO regions (key, sort_order, tile_color) VALUES ('south', 2, '#C97B4A') RETURNING id`)
	s := populatedPreSpainWide{db: db, provider: p}
	s.tortilla = sqlID(t, db, `INSERT INTO dishes (key, sort_order, course, season) VALUES ('tortilla', 1, 'tapas', 'all') RETURNING id`)
	s.paella = sqlID(t, db, `INSERT INTO dishes (key, sort_order, course, season) VALUES ('paella', 2, 'mains', 'all') RETURNING id`)
	s.tortillaSouth = preSpainWideVariation(t, db, regionalVariation{s.tortilla, south, "tortilla-south"})
	s.tortillaNorth = preSpainWideVariation(t, db, regionalVariation{s.tortilla, north, "tortilla-north"})
	s.paellaSouth = preSpainWideVariation(t, db, regionalVariation{s.paella, south, "paella-south"})
	return s
}

// regionalVariation is a variation row before BE-9.
type regionalVariation struct {
	dishID, regionID int64
	recipeKey        string
}

// preSpainWideVariation inserts a variation with its recipe, in the schema
// before BE-9 (no slug, no position).
func preSpainWideVariation(t *testing.T, db *sql.DB, v regionalVariation) int64 {
	t.Helper()
	id := sqlID(t, db, `INSERT INTO variations (dish_id, region_id, recipe_key) VALUES ($1, $2, $3) RETURNING id`,
		v.dishID, v.regionID, v.recipeKey)
	sqlExec(t, db, `INSERT INTO recipes (variation_id, time_minutes, base_servings) VALUES ($1, 25, 2)`, id)
	return id
}

func (s populatedPreSpainWide) upSpainWide(t *testing.T) {
	t.Helper()
	if _, err := s.provider.UpByOne(context.Background()); err != nil {
		t.Fatalf("up the BE-9 migration on a populated database: got err %v, want nil", err)
	}
}

func (s populatedPreSpainWide) dbVersion(t *testing.T) int64 {
	t.Helper()
	v, err := s.provider.GetDBVersion(context.Background())
	if err != nil {
		t.Fatalf("db version: %v", err)
	}
	return v
}

// assertBackfilledPositions checks every existing variation got a position by
// region order within its dish, and no slug.
func (s populatedPreSpainWide) assertBackfilledPositions(t *testing.T) {
	t.Helper()
	want := map[int64]int{s.tortillaNorth: 1, s.tortillaSouth: 2, s.paellaSouth: 1}
	for id, wantPos := range want {
		var pos int
		var slug sql.NullString
		row := s.db.QueryRowContext(context.Background(), `SELECT position, slug FROM variations WHERE id = $1`, id)
		if err := row.Scan(&pos, &slug); err != nil {
			t.Fatalf("read variation %d: %v", id, err)
		}
		if pos != wantPos {
			t.Errorf("variation %d position: got %d, want %d (region order within the dish)", id, pos, wantPos)
		}
		if slug.Valid {
			t.Errorf("variation %d slug: got %q, want NULL for a regional variation", id, slug.String)
		}
	}
}

func (s populatedPreSpainWide) addSpainWideVariation(t *testing.T) int64 {
	t.Helper()
	return sqlID(t, s.db, `
		INSERT INTO variations (dish_id, region_id, slug, recipe_key, position)
		VALUES ($1, NULL, 'tortilla-de-diario', 'tortilla-esp-tortilla-de-diario', 3) RETURNING id`, s.tortilla)
}

func (s populatedPreSpainWide) variationCount(t *testing.T) int {
	t.Helper()
	return sqlInt(t, s.db, `SELECT count(*) FROM variations`)
}

func variationColumns(t *testing.T, db *sql.DB) []string {
	t.Helper()
	return queryStrings(t, db, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'variations' AND column_name IN ('slug', 'position')
		ORDER BY column_name`)
}

func TestIntegrationSpainWideMigrationUpBackfillsPositionByRegionOrder(t *testing.T) {
	s := newPopulatedPreSpainWide(t)

	s.upSpainWide(t)

	s.assertBackfilledPositions(t)
	if got := s.variationCount(t); got != 3 {
		t.Errorf("variations after up: got %d, want the 3 regional ones kept", got)
	}
	assertMigratedSchema(t, s.db)
}

// The Down migration refuses to drop Spanish variations: it fails and leaves
// the schema and the rows as they were.
func TestIntegrationSpainWideMigrationDownRefusesWhileSpanishVariationsExist(t *testing.T) {
	s := newPopulatedPreSpainWide(t)
	s.upSpainWide(t)
	upVersion := s.dbVersion(t)
	spanish := s.addSpainWideVariation(t)

	if _, err := s.provider.Down(context.Background()); err == nil {
		t.Fatal("down with a Spanish variation stored: got nil, want an error refusing to drop it")
	}

	if got := s.dbVersion(t); got != upVersion {
		t.Errorf("db version after the refused down: got %d, want %d", got, upVersion)
	}
	if n := sqlInt(t, s.db, `SELECT count(*) FROM variations WHERE id = $1 AND region_id IS NULL`, spanish); n != 1 {
		t.Errorf("spanish variation after the refused down: got %d rows, want 1", n)
	}
	if got := variationColumns(t, s.db); len(got) != 2 {
		t.Errorf("variations slug/position columns after the refused down: got %v, want both", got)
	}
}

// Without Spanish variations, down restores the pre-BE-9 schema and keeps
// every regional variation; up again backfills positions the same way.
func TestIntegrationSpainWideMigrationDownThenUpKeepsRegionalVariations(t *testing.T) {
	s := newPopulatedPreSpainWide(t)
	s.upSpainWide(t)
	spanish := s.addSpainWideVariation(t)
	sqlExec(t, s.db, `DELETE FROM variations WHERE id = $1`, spanish)

	if _, err := s.provider.Down(context.Background()); err != nil {
		t.Fatalf("down without Spanish variations: got err %v, want nil", err)
	}
	if got := s.dbVersion(t); got != versionBeforeSpainWide {
		t.Errorf("db version after down: got %d, want %d", got, versionBeforeSpainWide)
	}
	if got := variationColumns(t, s.db); len(got) != 0 {
		t.Errorf("variations slug/position columns after down: got %v, want none", got)
	}
	if got := sqlInt(t, s.db, `SELECT count(*) FROM variations WHERE region_id IS NOT NULL`); got != 3 {
		t.Errorf("regional variations after down: got %d, want 3", got)
	}

	s.upSpainWide(t)
	s.assertBackfilledPositions(t)
}

func sqlExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func sqlID(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&id); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return id
}

func sqlInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	return int(sqlID(t, db, query, args...))
}
