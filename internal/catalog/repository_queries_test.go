package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
)

// extraEntities is how many regions and dishes growCatalog adds; enough that
// a per-row query would blow every budget.
const extraEntities = 8

// growCatalog adds regions with regional dishes, and dishes with a variation
// in every fixture region, all translated in every locale.
func growCatalog(t *testing.T, p pgFixture) {
	t.Helper()
	for i := range extraEntities {
		addRegion(t, p, i)
		addDish(t, p, i)
	}
}

func addRegion(t *testing.T, p pgFixture, i int) {
	t.Helper()
	var id int64
	err := p.tx.QueryRow(context.Background(),
		`INSERT INTO regions (key, sort_order, tile_color) VALUES ($1, $2, '#123456') RETURNING id`,
		fmt.Sprintf("extra-region-%d", i), 5000+i).Scan(&id)
	if err != nil {
		t.Fatalf("insert region: %v", err)
	}
	p.exec(t, `INSERT INTO region_translations (region_id, locale, name, map_label, description)
		SELECT $1::bigint, code, 'n', 'm', 'd' FROM locales`, id)
	p.exec(t, `INSERT INTO region_regional_dishes (region_id, position, name) VALUES ($1, 1, 'a'), ($1, 2, 'b')`, id)
}

func addDish(t *testing.T, p pgFixture, i int) {
	t.Helper()
	var id int64
	key := fmt.Sprintf("extra-dish-%d", i)
	err := p.tx.QueryRow(context.Background(),
		`INSERT INTO dishes (key, sort_order, course, season) VALUES ($1, $2, 'mains', 'all') RETURNING id`,
		key, 5000+i).Scan(&id)
	if err != nil {
		t.Fatalf("insert dish: %v", err)
	}
	p.exec(t, `INSERT INTO dish_translations (dish_id, locale, name, description)
		SELECT $1::bigint, code, 'n', 'd' FROM locales`, id)
	p.exec(t, `INSERT INTO variations (dish_id, region_id, recipe_key)
		SELECT $1::bigint, r.id, $2::text || '-' || r.key FROM regions r WHERE r.key IN ('gal', 'ast')`, id, key)
	p.exec(t, `INSERT INTO variation_translations (variation_id, locale, name, description)
		SELECT v.id, l.code, 'n', 'd' FROM variations v CROSS JOIN locales l WHERE v.dish_id = $1::bigint`, id)
}

// statementsFor resets the counter, runs call and returns how many
// statements it sent.
func statementsFor(t *testing.T, p pgFixture, call func(ctx context.Context) error) int {
	t.Helper()
	p.counter.queries = 0
	if err := call(context.Background()); err != nil {
		t.Fatalf("call failed: %v", err)
	}
	return p.counter.queries
}

func TestPGRepository_QueryCountIsBoundedRegardlessOfSize(t *testing.T) {
	cases := []struct {
		name string
		max  int
		call func(repo catalog.Repository) func(ctx context.Context) error
	}{
		{"Regions", 3, func(r catalog.Repository) func(context.Context) error {
			return func(ctx context.Context) error { _, err := r.Regions(ctx, "ru"); return err }
		}},
		{"Dishes", 6, func(r catalog.Repository) func(context.Context) error {
			return func(ctx context.Context) error { _, err := r.Dishes(ctx, "ru"); return err }
		}},
		{"Recipe", 4, func(r catalog.Repository) func(context.Context) error {
			return func(ctx context.Context) error { _, err := r.Recipe(ctx, "cocido", "ast", "ru"); return err }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := loadPGFixture(t)
			small := statementsFor(t, p, tc.call(p.repo))
			growCatalog(t, p)
			large := statementsFor(t, p, tc.call(p.repo))

			if large > tc.max {
				t.Errorf("got %d statements, want at most %d", large, tc.max)
			}
			if large != small {
				t.Errorf("got %d statements with %d more rows, %d before; want the same", large, extraEntities, small)
			}
		})
	}
}

// missingCase deletes one translation row and expects the read for locale to
// fail naming the entity key.
type missingCase struct {
	name    string
	delete  string // $1 is the id from idOf, $2 the locale
	idOf    func(p pgFixture) int64
	locale  catalog.Locale
	read    func(ctx context.Context, repo catalog.Repository, l catalog.Locale) error
	wantKey string
}

func readRegions(ctx context.Context, r catalog.Repository, l catalog.Locale) error {
	_, err := r.Regions(ctx, l)
	return err
}

func readDishes(ctx context.Context, r catalog.Repository, l catalog.Locale) error {
	_, err := r.Dishes(ctx, l)
	return err
}

func readFabada(ctx context.Context, r catalog.Repository, l catalog.Locale) error {
	_, err := r.Recipe(ctx, "cocido", "ast", l)
	return err
}

func missingCases() []missingCase {
	region := func(p pgFixture) int64 { return p.f.RegionIDs["gal"] }
	dish := func(p pgFixture) int64 { return p.f.DishIDs["tortilla"] }
	variation := func(p pgFixture) int64 { return p.f.VariationIDs["tortilla-betanzos"] }
	recipe := func(p pgFixture) int64 { return p.f.RecipeIDs["fabada-asturiana"] }
	return []missingCase{
		{"region ru", `DELETE FROM region_translations WHERE region_id = $1 AND locale = $2`, region, "ru", readRegions, "gal"},
		{"dish ru", `DELETE FROM dish_translations WHERE dish_id = $1 AND locale = $2`, dish, "ru", readDishes, "tortilla"},
		{"variation ru", `DELETE FROM variation_translations WHERE variation_id = $1 AND locale = $2`, variation, "ru", readDishes, "tortilla-betanzos"},
		{"region name in variation summaries", `DELETE FROM region_translations WHERE region_id = $1 AND locale = $2`, region, "ru", readDishes, "gal"},
		{"recipe ingredient ru", `DELETE FROM recipe_ingredient_translations WHERE recipe_id = $1 AND position = 2 AND locale = $2`, recipe, "ru", readFabada, "fabada-asturiana"},
		{"recipe step ru", `DELETE FROM recipe_step_translations WHERE recipe_id = $1 AND position = 3 AND locale = $2`, recipe, "ru", readFabada, "fabada-asturiana"},
		{"recipe time note ru", `DELETE FROM recipe_translations WHERE recipe_id = $1 AND locale = $2`, recipe, "ru", readFabada, "fabada-asturiana"},
	}
}

func TestPGRepository_MissingTranslationIsAnError(t *testing.T) {
	for _, tc := range missingCases() {
		t.Run(tc.name, func(t *testing.T) {
			p := loadPGFixture(t)
			p.exec(t, tc.delete, tc.idOf(p), string(tc.locale))

			err := tc.read(context.Background(), p.repo, tc.locale)

			assertMissingTranslation(t, err, tc.wantKey, tc.locale)
			if other := tc.read(context.Background(), p.repo, "en"); other != nil {
				t.Errorf("read in en: got %v, want nil (only %s is missing)", other, tc.locale)
			}
		})
	}
}

// The Spanish row also feeds nameEs, so every locale needs it.
func TestPGRepository_MissingSpanishNameFailsEveryLocale(t *testing.T) {
	for _, l := range locales {
		t.Run(string(l), func(t *testing.T) {
			p := loadPGFixture(t)
			p.exec(t, `DELETE FROM dish_translations WHERE dish_id = $1 AND locale = 'es'`, p.f.DishIDs["cocido"])

			_, err := p.repo.Dishes(context.Background(), l)

			assertMissingTranslation(t, err, "cocido", "es")
		})
	}
}

func assertMissingTranslation(t *testing.T, err error, wantKey string, wantLocale catalog.Locale) {
	t.Helper()
	var missing *catalog.MissingTranslationError
	if !errors.As(err, &missing) {
		t.Fatalf("got error %v, want a *MissingTranslationError", err)
	}
	if missing.Key != wantKey || missing.Locale != wantLocale || missing.Entity == "" {
		t.Errorf("got missing translation %+v, want key %q, locale %q and an entity", *missing, wantKey, wantLocale)
	}
}
