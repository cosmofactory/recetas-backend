package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

// probe counts the rows of one table that belong to a tree.
type probe struct {
	table string
	query string
	id    func(tr tree) int64
}

func byDish(tr tree) int64      { return tr.dish }
func byRegion(tr tree) int64    { return tr.region }
func byVariation(tr tree) int64 { return tr.variation }
func byRecipe(tr tree) int64    { return tr.recipe }

var (
	variationProbes = []probe{
		{"variations", `SELECT count(*) FROM variations WHERE id = $1`, byVariation},
		{"variation_translations", `SELECT count(*) FROM variation_translations WHERE variation_id = $1`, byVariation},
	}
	ingredientAndStepTranslationProbes = []probe{
		{"recipe_ingredient_translations", `SELECT count(*) FROM recipe_ingredient_translations WHERE recipe_id = $1`, byRecipe},
		{"recipe_step_translations", `SELECT count(*) FROM recipe_step_translations WHERE recipe_id = $1`, byRecipe},
	}
	recipeProbes = append([]probe{
		{"recipes", `SELECT count(*) FROM recipes WHERE id = $1`, byRecipe},
		{"recipe_ingredients", `SELECT count(*) FROM recipe_ingredients WHERE recipe_id = $1`, byRecipe},
		{"recipe_steps", `SELECT count(*) FROM recipe_steps WHERE recipe_id = $1`, byRecipe},
		{"recipe_translations", `SELECT count(*) FROM recipe_translations WHERE recipe_id = $1`, byRecipe},
	}, ingredientAndStepTranslationProbes...)
	variationTreeProbes = append(append([]probe{}, variationProbes...), recipeProbes...)
	dishProbes          = []probe{
		{"dishes", `SELECT count(*) FROM dishes WHERE id = $1`, byDish},
		{"dish_translations", `SELECT count(*) FROM dish_translations WHERE dish_id = $1`, byDish},
	}
	regionProbes = []probe{
		{"regions", `SELECT count(*) FROM regions WHERE id = $1`, byRegion},
		{"region_translations", `SELECT count(*) FROM region_translations WHERE region_id = $1`, byRegion},
		{"region_regional_dishes", `SELECT count(*) FROM region_regional_dishes WHERE region_id = $1`, byRegion},
	}
)

const (
	rowsGone = 0
	rowsKept = 1
)

type cascadeCase struct {
	name    string
	deletes []string // each takes the tree id chosen by target
	target  func(tr tree) int64
	gone    []probe
	kept    []probe
}

var cascadeCases = []cascadeCase{
	{"deleting a dish removes its tree and keeps the region",
		[]string{`DELETE FROM dishes WHERE id = $1`}, byDish,
		append(append([]probe{}, dishProbes...), variationTreeProbes...), regionProbes},
	{"deleting a region removes its variations and keeps the dish",
		[]string{`DELETE FROM regions WHERE id = $1`}, byRegion,
		append(append([]probe{}, regionProbes...), variationTreeProbes...), dishProbes},
	{"deleting a recipe removes its time notes, ingredients and steps",
		[]string{`DELETE FROM recipes WHERE id = $1`}, byRecipe,
		recipeProbes, variationProbes},
	{"deleting an ingredient or step removes its translations",
		[]string{`DELETE FROM recipe_ingredients WHERE recipe_id = $1`, `DELETE FROM recipe_steps WHERE recipe_id = $1`}, byRecipe,
		ingredientAndStepTranslationProbes, nil},
}

func TestIntegrationDeleteCascades(t *testing.T) {
	for _, c := range cascadeCases {
		t.Run(c.name, func(t *testing.T) { runCascadeCase(t, c) })
	}
}

func runCascadeCase(t *testing.T, c cascadeCase) {
	t.Helper()
	tx := testdb.Tx(t)
	tr := insertCascadeTree(t, tx)
	for _, del := range c.deletes {
		mustExec(t, tx, del, c.target(tr))
	}
	assertProbeCounts(t, tx, tr, probeSet{c.gone, rowsGone})
	assertProbeCounts(t, tx, tr, probeSet{c.kept, rowsKept})
}

// insertCascadeTree inserts one row in every table of a region → dish →
// variation → recipe chain.
func insertCascadeTree(t *testing.T, tx pgx.Tx) tree {
	t.Helper()
	tr := insertFullRecipe(t, tx)
	mustExec(t, tx, `INSERT INTO region_translations (region_id, locale, name, map_label, description) VALUES ($1, 'en', 'Galicia', 'Galicia', 'Seafood.')`, tr.region)
	mustExec(t, tx, `INSERT INTO region_regional_dishes (region_id, position, name) VALUES ($1, 1, 'Pulpo á feira')`, tr.region)
	mustExec(t, tx, `INSERT INTO dish_translations (dish_id, locale, name, description) VALUES ($1, 'en', 'Tortilla', 'Eggs.')`, tr.dish)
	mustExec(t, tx, `INSERT INTO variation_translations (variation_id, locale, name, description) VALUES ($1, 'en', 'Runny', 'Soft.')`, tr.variation)
	mustExec(t, tx, `INSERT INTO recipe_ingredients (recipe_id, position, quantity, unit) VALUES ($1, 1, 4, NULL)`, tr.recipe)
	mustExec(t, tx, `INSERT INTO recipe_ingredient_translations (recipe_id, position, locale, name) VALUES ($1, 1, 'en', 'Eggs')`, tr.recipe)
	mustExec(t, tx, `INSERT INTO recipe_steps (recipe_id, position) VALUES ($1, 1)`, tr.recipe)
	mustExec(t, tx, `INSERT INTO recipe_step_translations (recipe_id, position, locale, text) VALUES ($1, 1, 'en', 'Beat.')`, tr.recipe)
	mustExec(t, tx, `INSERT INTO recipe_translations (recipe_id, locale, time_note) VALUES ($1, 'en', '+ soaking')`, tr.recipe)
	return tr
}

// probeSet expects every probe to count want rows.
type probeSet struct {
	probes []probe
	want   int
}

func assertProbeCounts(t *testing.T, tx pgx.Tx, tr tree, set probeSet) {
	t.Helper()
	for _, p := range set.probes {
		if n := countRows(t, tx, p.query, p.id(tr)); n != set.want {
			t.Errorf("%s: got %d rows, want %d", p.table, n, set.want)
		}
	}
}

// minForeignKeys: variations → dishes, regions; translations → parents and
// locales; ingredients, steps and time notes → recipes; and so on.
const minForeignKeys = 15

// foreignKeysSQL lists every FK in public with its ON DELETE action and
// whether an index's leading columns are exactly the FK columns.
const foreignKeysSQL = `
	SELECT c.conrelid::regclass::text,
	       c.conname,
	       c.confrelid::regclass::text,
	       c.confdeltype::text,
	       EXISTS (
	           SELECT 1 FROM pg_index i
	           WHERE i.indrelid = c.conrelid
	             AND (string_to_array(i.indkey::text, ' ')::int2[])[1:cardinality(c.conkey)] @> c.conkey
	             AND (string_to_array(i.indkey::text, ' ')::int2[])[1:cardinality(c.conkey)] <@ c.conkey
	       )
	FROM pg_constraint c
	JOIN pg_namespace n ON n.oid = c.connamespace
	WHERE c.contype = 'f' AND n.nspname = 'public'
	ORDER BY 1, 2`

// pg_constraint.confdeltype codes.
const (
	onDeleteCascade  = "c"
	onDeleteRestrict = "r"
	onDeleteNoAction = "a"
)

type foreignKey struct {
	table, name, ref, onDelete string
	indexed                    bool
}

// TestIntegrationForeignKeysCascadeAndAreIndexed checks every FK in the
// schema: ON DELETE CASCADE (except references to the locales reference
// table, where RESTRICT is also acceptable), and an index whose leading
// columns are the FK columns.
func TestIntegrationForeignKeysCascadeAndAreIndexed(t *testing.T) {
	fks := listForeignKeys(t, testdb.Tx(t))
	if len(fks) < minForeignKeys {
		t.Errorf("foreign keys: got %d, want at least %d (got %v)", len(fks), minForeignKeys, fks)
	}
	for _, f := range fks {
		checkForeignKey(t, f)
	}
}

func listForeignKeys(t *testing.T, tx pgx.Tx) []foreignKey {
	t.Helper()
	rows, err := tx.Query(context.Background(), foreignKeysSQL)
	if err != nil {
		t.Fatalf("query fks: %v", err)
	}
	fks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (foreignKey, error) {
		var f foreignKey
		err := row.Scan(&f.table, &f.name, &f.ref, &f.onDelete, &f.indexed)
		return f, err
	})
	if err != nil {
		t.Fatalf("collect fks: %v", err)
	}
	return fks
}

func checkForeignKey(t *testing.T, f foreignKey) {
	t.Helper()
	restrictLocale := f.ref == "locales" && (f.onDelete == onDeleteRestrict || f.onDelete == onDeleteNoAction)
	if f.onDelete != onDeleteCascade && !restrictLocale {
		t.Errorf("%s.%s → %s: got ON DELETE %q, want CASCADE", f.table, f.name, f.ref, f.onDelete)
	}
	if !f.indexed {
		t.Errorf("%s.%s → %s: got no index leading with the FK columns, want one", f.table, f.name, f.ref)
	}
}
