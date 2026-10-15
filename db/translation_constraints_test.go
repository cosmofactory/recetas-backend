package db_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

// translationTable inserts one row for locale into a translation table.
type translationTable struct {
	table string
	sql   string
	args  func(tr tree, locale string) []any
}

var translationTables = []translationTable{
	{"region_translations",
		`INSERT INTO region_translations (region_id, locale, name, map_label, description) VALUES ($1, $2, 'Galicia', 'Galicia', 'Atlantic seafood.')`,
		func(tr tree, l string) []any { return []any{tr.region, l} }},
	{"dish_translations",
		`INSERT INTO dish_translations (dish_id, locale, name, description) VALUES ($1, $2, 'Tortilla de patatas', 'One dish.')`,
		func(tr tree, l string) []any { return []any{tr.dish, l} }},
	{"variation_translations",
		`INSERT INTO variation_translations (variation_id, locale, name, description) VALUES ($1, $2, 'Betanzos-style runny', 'Barely-set centre')`,
		func(tr tree, l string) []any { return []any{tr.variation, l} }},
	{"recipe_ingredient_translations",
		`INSERT INTO recipe_ingredient_translations (recipe_id, position, locale, name) VALUES ($1, 1, $2, 'Eggs')`,
		func(tr tree, l string) []any { return []any{tr.recipe, l} }},
	{"recipe_translations",
		`INSERT INTO recipe_translations (recipe_id, locale, time_note) VALUES ($1, $2, '+ soaking')`,
		func(tr tree, l string) []any { return []any{tr.recipe, l} }},
	{"recipe_step_translations",
		`INSERT INTO recipe_step_translations (recipe_id, position, locale, text) VALUES ($1, 1, $2, 'Peel the potatoes.')`,
		func(tr tree, l string) []any { return []any{tr.recipe, l} }},
}

// localeCases run in order against one parent row: the duplicate relies on
// the earlier "en".
var localeCases = []struct {
	name, locale, want string
}{
	{"en accepted", "en", codeOK},
	{"es accepted", "es", codeOK},
	{"ru accepted", "ru", codeOK},
	{"duplicate locale", "en", codeUnique},
	{"unknown locale fr", "fr", codeForeignKey},
	{"locale wrong case", "EN", codeForeignKey},
}

// translationRejections are single statements that must fail; args may
// insert fresh parent rows.
var translationRejections = []struct {
	name string
	sql  string
	args func(t *testing.T, tx pgx.Tx, tr tree) []any
	want string
}{
	{"ingredient translation for missing ingredient",
		`INSERT INTO recipe_ingredient_translations (recipe_id, position, locale, name) VALUES ($1, $2, 'en', 'Eggs')`,
		func(_ *testing.T, _ pgx.Tx, tr tree) []any { return []any{tr.recipe, missingPosition} },
		codeForeignKey},
	{"step translation for missing step",
		`INSERT INTO recipe_step_translations (recipe_id, position, locale, text) VALUES ($1, $2, 'en', 'Peel.')`,
		func(_ *testing.T, _ pgx.Tx, tr tree) []any { return []any{tr.recipe, missingPosition} },
		codeForeignKey},
	{"null time_note",
		`INSERT INTO recipe_translations (recipe_id, locale, time_note) VALUES ($1, 'en', NULL)`,
		func(t *testing.T, tx pgx.Tx, _ tree) []any { return []any{insertFullRecipe(t, tx).recipe} },
		codeNotNull},
	{"time_note for unknown recipe",
		`INSERT INTO recipe_translations (recipe_id, locale, time_note) VALUES ($1, 'en', '+ soaking')`,
		func(_ *testing.T, _ pgx.Tx, _ tree) []any { return []any{unknownID} },
		codeForeignKey},
	{"null translated name",
		`INSERT INTO dish_translations (dish_id, locale, name, description) VALUES ($1, 'en', NULL, 'One dish.')`,
		func(t *testing.T, tx pgx.Tx, _ tree) []any { return []any{insertDish(t, tx)} },
		codeNotNull},
}

func TestIntegrationTranslationLocaleConstraints(t *testing.T) {
	tx := testdb.Tx(t)
	tr := insertFullRecipe(t, tx)
	mustExec(t, tx, insertIngredientSQL, tr.recipe, firstPosition, 4, nil)
	mustExec(t, tx, insertStepSQL, tr.recipe, firstPosition)

	for _, tt := range translationTables {
		t.Run(tt.table, func(t *testing.T) { runLocaleCases(t, tx, tr, tt) })
	}
	for _, rc := range translationRejections {
		t.Run(rc.name, func(t *testing.T) {
			wantCode(t, try(t, tx, rc.sql, rc.args(t, tx, tr)...), rc.want)
		})
	}
	t.Run("recipe without time_note rows is valid", func(t *testing.T) {
		bare := insertFullRecipe(t, tx).recipe
		if n := countRows(t, tx, `SELECT count(*) FROM recipe_translations WHERE recipe_id = $1`, bare); n != 0 {
			t.Errorf("recipe_translations for a bare recipe: got %d rows, want 0", n)
		}
	})
}

func runLocaleCases(t *testing.T, tx pgx.Tx, tr tree, tt translationTable) {
	t.Helper()
	for _, c := range localeCases {
		t.Run(c.name, func(t *testing.T) {
			wantCode(t, try(t, tx, tt.sql, tt.args(tr, c.locale)...), c.want)
		})
	}
}
