package catalogtest_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogtest"
	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

// fixtureIDs are the sorted ids Load returned, per entity.
type fixtureIDs struct {
	regions, dishes, variations, recipes []int64
}

func idsOf(f catalogtest.Fixture) fixtureIDs {
	return fixtureIDs{
		regions:    sortedIDs(f.RegionIDs),
		dishes:     sortedIDs(f.DishIDs),
		variations: sortedIDs(f.VariationIDs),
		recipes:    sortedIDs(f.RecipeIDs),
	}
}

func sortedIDs(m map[string]int64) []int64 {
	out := make([]int64, 0, len(m))
	for _, id := range m {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

type rowCount struct {
	table string
	query string
	args  []any
	want  int
}

// rowCounts: 4+5+5 ingredients and 5+4+5 steps across the three recipes;
// only fabada has a time note.
func rowCounts(f catalogtest.Fixture, ids fixtureIDs) []rowCount {
	tortillas := []int64{f.RecipeIDs["tortilla-betanzos"], f.RecipeIDs["tortilla-chorizo"]}
	return []rowCount{
		{"regions", `SELECT count(*) FROM regions WHERE id = ANY($1)`, []any{ids.regions}, 2},
		{"region_translations", `SELECT count(*) FROM region_translations WHERE region_id = ANY($1)`, []any{ids.regions}, 6},
		{"region_regional_dishes", `SELECT count(*) FROM region_regional_dishes WHERE region_id = ANY($1)`, []any{ids.regions}, 6},
		{"dishes", `SELECT count(*) FROM dishes WHERE id = ANY($1)`, []any{ids.dishes}, 2},
		{"dish_translations", `SELECT count(*) FROM dish_translations WHERE dish_id = ANY($1)`, []any{ids.dishes}, 6},
		{"variations", `SELECT count(*) FROM variations WHERE dish_id = ANY($1)`, []any{ids.dishes}, 3},
		{"variation_translations", `SELECT count(*) FROM variation_translations WHERE variation_id = ANY($1)`, []any{ids.variations}, 9},
		{"recipes", `SELECT count(*) FROM recipes WHERE variation_id = ANY($1)`, []any{ids.variations}, 3},
		{"recipe_ingredients", `SELECT count(*) FROM recipe_ingredients WHERE recipe_id = ANY($1)`, []any{ids.recipes}, 14},
		{"recipe_ingredient_translations", `SELECT count(*) FROM recipe_ingredient_translations WHERE recipe_id = ANY($1)`, []any{ids.recipes}, 42},
		{"recipe_steps", `SELECT count(*) FROM recipe_steps WHERE recipe_id = ANY($1)`, []any{ids.recipes}, 14},
		{"recipe_step_translations", `SELECT count(*) FROM recipe_step_translations WHERE recipe_id = ANY($1)`, []any{ids.recipes}, 42},
		{"recipe_translations", `SELECT count(*) FROM recipe_translations WHERE recipe_id = ANY($1)`, []any{ids.recipes}, 3},
		{"recipe_translations of fabada", `SELECT count(*) FROM recipe_translations WHERE recipe_id = $1`, []any{f.RecipeIDs["fabada-asturiana"]}, 3},
		{"recipe_translations of tortillas", `SELECT count(*) FROM recipe_translations WHERE recipe_id = ANY($1)`, []any{tortillas}, 0},
		{"content_versions", `SELECT count(*) FROM content_versions WHERE id = $1`, []any{f.ContentVersionID}, 1},
		{"counted ingredients (null unit)", `SELECT count(*) FROM recipe_ingredients WHERE recipe_id = ANY($1) AND unit IS NULL`, []any{ids.recipes}, 5},
	}
}

// localeCheck returns the parent rows whose locales are not exactly en, es, ru.
type localeCheck struct {
	table string
	query string
	ids   []int64
}

func localeChecks(ids fixtureIDs) []localeCheck {
	return []localeCheck{
		{"region_translations", `
			SELECT region_id::text FROM region_translations WHERE region_id = ANY($1)
			GROUP BY region_id HAVING array_agg(locale ORDER BY locale) <> ARRAY['en','es','ru']`, ids.regions},
		{"dish_translations", `
			SELECT dish_id::text FROM dish_translations WHERE dish_id = ANY($1)
			GROUP BY dish_id HAVING array_agg(locale ORDER BY locale) <> ARRAY['en','es','ru']`, ids.dishes},
		{"variation_translations", `
			SELECT variation_id::text FROM variation_translations WHERE variation_id = ANY($1)
			GROUP BY variation_id HAVING array_agg(locale ORDER BY locale) <> ARRAY['en','es','ru']`, ids.variations},
		{"recipe_ingredient_translations", `
			SELECT i.recipe_id || '/' || i.position FROM recipe_ingredients i
			LEFT JOIN recipe_ingredient_translations tr USING (recipe_id, position)
			WHERE i.recipe_id = ANY($1)
			GROUP BY i.recipe_id, i.position
			HAVING array_agg(tr.locale::text ORDER BY tr.locale) <> ARRAY['en','es','ru']`, ids.recipes},
		{"recipe_translations", `
			SELECT recipe_id::text FROM recipe_translations WHERE recipe_id = ANY($1)
			GROUP BY recipe_id HAVING array_agg(locale ORDER BY locale) <> ARRAY['en','es','ru']`, ids.recipes},
		{"recipe_step_translations", `
			SELECT s.recipe_id || '/' || s.position FROM recipe_steps s
			LEFT JOIN recipe_step_translations tr USING (recipe_id, position)
			WHERE s.recipe_id = ANY($1)
			GROUP BY s.recipe_id, s.position
			HAVING array_agg(tr.locale::text ORDER BY tr.locale) <> ARRAY['en','es','ru']`, ids.recipes},
	}
}

// storedValue is a single value read back as text.
type storedValue struct {
	name  string
	query string
	args  []any
	want  string
}

func storedValues(f catalogtest.Fixture) []storedValue {
	return []storedValue{
		{"gal tile_color", `SELECT tile_color FROM regions WHERE key = 'gal'`, nil, "#9A9A5E"},
		{"tortilla-betanzos servings/minutes", `
			SELECT r.base_servings || '/' || r.time_minutes FROM recipes r
			JOIN variations v ON v.id = r.variation_id WHERE v.recipe_key = 'tortilla-betanzos'`, nil, "2/25"},
		{"tortilla-chorizo salt quantity", `
			SELECT quantity::float8::text FROM recipe_ingredients
			WHERE recipe_id = $1 AND unit = 'tsp'`, []any{f.RecipeIDs["tortilla-chorizo"]}, "0.5"},
	}
}

// photoCheck reads one row's photo_thumb, photo_card, photo_hero.
type photoCheck struct {
	name  string
	query string
	id    int64
	want  catalogtest.Photos
}

func photoChecks(f catalogtest.Fixture) []photoCheck {
	const (
		dishPhotos      = `SELECT photo_thumb, photo_card, photo_hero FROM dishes WHERE id = $1`
		variationPhotos = `SELECT photo_thumb, photo_card, photo_hero FROM variations WHERE id = $1`
	)
	var checks []photoCheck
	for _, d := range f.Dishes {
		checks = append(checks, photoCheck{"dish " + d.Key, dishPhotos, f.DishIDs[d.Key], d.Photos})
		for _, v := range d.Variations {
			checks = append(checks, photoCheck{"variation " + v.RecipeKey, variationPhotos, f.VariationIDs[v.RecipeKey], v.Photos})
		}
	}
	return checks
}

func assertStoredPhotos(t *testing.T, tx pgx.Tx, c photoCheck) {
	t.Helper()
	var got catalogtest.Photos
	if err := tx.QueryRow(context.Background(), c.query, c.id).Scan(&got.Thumb, &got.Card, &got.Hero); err != nil {
		t.Fatalf("%s photos: %v", c.name, err)
	}
	assertPhoto(t, c.name+" photo_thumb", got.Thumb, c.want.Thumb)
	assertPhoto(t, c.name+" photo_card", got.Card, c.want.Card)
	assertPhoto(t, c.name+" photo_hero", got.Hero, c.want.Hero)
}

func assertPhoto(t *testing.T, what string, got, want *string) {
	t.Helper()
	if deref(got) != deref(want) {
		t.Errorf("%s: got %s, want %s", what, deref(got), deref(want))
	}
}

// deref renders a nullable key, with NULL distinct from any stored key.
func deref(s *string) string {
	if s == nil {
		return "NULL"
	}
	return "\"" + *s + "\""
}

var wantFabadaTimeNotes = []string{"en=+ soaking", "es=+ remojo", "ru=+ замачивание"}

func TestIntegrationLoadInsertsCompleteFixture(t *testing.T) {
	tx := testdb.Tx(t)
	f := catalogtest.Load(t, tx)
	ids := idsOf(f)
	requireIDCounts(t, ids)

	for _, c := range rowCounts(f, ids) {
		t.Run("rows "+c.table, func(t *testing.T) { assertRowCount(t, tx, c) })
	}
	for _, c := range localeChecks(ids) {
		t.Run("locales "+c.table, func(t *testing.T) { assertExactLocales(t, tx, c) })
	}
	t.Run("time note round-trips", func(t *testing.T) { assertFabadaTimeNotes(t, tx, f) })
	t.Run("photos round-trip", func(t *testing.T) {
		for _, c := range photoChecks(f) {
			assertStoredPhotos(t, tx, c)
		}
	})
	t.Run("values round-trip", func(t *testing.T) {
		for _, v := range storedValues(f) {
			assertStoredValue(t, tx, v)
		}
	})
}

func requireIDCounts(t *testing.T, ids fixtureIDs) {
	t.Helper()
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"region ids", len(ids.regions), 2},
		{"dish ids", len(ids.dishes), 2},
		{"variation ids", len(ids.variations), 3},
		{"recipe ids", len(ids.recipes), 3},
	} {
		if c.got != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}

func assertRowCount(t *testing.T, tx pgx.Tx, c rowCount) {
	t.Helper()
	var n int
	if err := tx.QueryRow(context.Background(), c.query, c.args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", c.query, err)
	}
	if n != c.want {
		t.Errorf("%s: got %d rows, want %d", c.table, n, c.want)
	}
}

func queryStrings(t *testing.T, tx pgx.Tx, query string, args ...any) []string {
	t.Helper()
	rows, err := tx.Query(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return out
}

func assertExactLocales(t *testing.T, tx pgx.Tx, c localeCheck) {
	t.Helper()
	if bad := queryStrings(t, tx, c.query, c.ids); len(bad) != 0 {
		t.Errorf("%s: rows %v do not have exactly %v", c.table, bad, catalogtest.Locales)
	}
}

func assertFabadaTimeNotes(t *testing.T, tx pgx.Tx, f catalogtest.Fixture) {
	t.Helper()
	got := queryStrings(t, tx, `
		SELECT locale || '=' || time_note FROM recipe_translations
		WHERE recipe_id = $1 ORDER BY locale`, f.RecipeIDs["fabada-asturiana"])
	if !slices.Equal(got, wantFabadaTimeNotes) {
		t.Errorf("fabada-asturiana time notes: got %v, want %v", got, wantFabadaTimeNotes)
	}
	if note := f.Dish(t, "tortilla").Variations[0].Recipe.TimeNote; note != nil {
		t.Errorf("tortilla-betanzos TimeNote: got %v, want nil", *note)
	}
}

func assertStoredValue(t *testing.T, tx pgx.Tx, v storedValue) {
	t.Helper()
	var got string
	if err := tx.QueryRow(context.Background(), v.query, v.args...).Scan(&got); err != nil {
		t.Fatalf("%s: %v", v.name, err)
	}
	if got != v.want {
		t.Errorf("%s: got %q, want %q", v.name, got, v.want)
	}
}
