package importer_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// betanzosRecipeSQL selects the id of the tortilla-betanzos recipe.
const betanzosRecipeSQL = `(SELECT r.id FROM recipes r JOIN variations v ON v.id = r.variation_id WHERE v.recipe_key = 'tortilla-betanzos')`

// extraBetanzosTipSQL gives tortilla-betanzos, which has no tips in the
// bundle, a complete tip in every locale.
const extraBetanzosTipSQL = `WITH tip AS (
	INSERT INTO recipe_tips (recipe_id, position) SELECT ` + betanzosRecipeSQL + `, 1 RETURNING recipe_id, position)
INSERT INTO recipe_tip_translations (recipe_id, position, locale, text)
SELECT tip.recipe_id, tip.position, l.code, 'Extra tip.' FROM tip CROSS JOIN locales l`

// fabadaRecipeSQL selects the id of the fabada-asturiana recipe, the one with tips.
const fabadaRecipeSQL = `(SELECT r.id FROM recipes r JOIN variations v ON v.id = r.variation_id WHERE v.recipe_key = 'fabada-asturiana')`

// A database edited behind the importer's back is repaired by re-importing the
// bundle it came from, even though the bundle's checksum is the latest one.
func TestApply_ReimportRepairsDrift(t *testing.T) {
	tests := []struct {
		name  string
		drift string
		want  map[string]importer.Counts
	}{
		{"dish translations deleted in one locale", "DELETE FROM dish_translations WHERE locale = 'ru'",
			map[string]importer.Counts{"dishes": {Updated: 2}}},
		{"region tile color hand-edited", "UPDATE regions SET tile_color = '#000000' WHERE key = 'gal'",
			map[string]importer.Counts{"regions": {Updated: 1}}},
		{"variation translation deleted",
			"DELETE FROM variation_translations WHERE locale = 'es' AND variation_id = (SELECT id FROM variations WHERE recipe_key = 'tortilla-chorizo')",
			map[string]importer.Counts{"variations": {Updated: 1}}},
		{"variation score hand-edited", "UPDATE variations SET score = 7 WHERE recipe_key = 'tortilla-chorizo'",
			map[string]importer.Counts{"variations": {Updated: 1}}},
		{"recipe step deleted", "DELETE FROM recipe_steps WHERE position = 2 AND recipe_id = " + betanzosRecipeSQL,
			map[string]importer.Counts{"recipes": {Updated: 1}}},
		{"recipe tip deleted", "DELETE FROM recipe_tips WHERE position = 2 AND recipe_id = " + fabadaRecipeSQL,
			map[string]importer.Counts{"recipes": {Updated: 1}}},
		{"recipe tip translation hand-edited", "UPDATE recipe_tip_translations SET text = 'Otro truco.' WHERE locale = 'es' AND position = 1 AND recipe_id = " + fabadaRecipeSQL,
			map[string]importer.Counts{"recipes": {Updated: 1}}},
		{"recipe tip added by hand", extraBetanzosTipSQL, map[string]importer.Counts{"recipes": {Updated: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := emptyTx(t)
			b := validBundle()
			first := mustApply(t, tx, b, realImport("first"))
			execSQL(t, tx, tt.drift)

			s := mustApply(t, tx, b, realImport("repair"))

			if s.NoChanges {
				t.Fatal("got NoChanges, want the drift repaired")
			}
			assertChanges(t, s, tt.want)
			if s.Version <= first.Version {
				t.Errorf("got version %d, want a new one after %d", s.Version, first.Version)
			}
			assertCatalogMatches(t, tx, b)
		})
	}
}

// Float noise in a quantity is rounded to the stored 3 decimals when parsed,
// so the recipe compares equal to what the database holds on the next import.
func TestApply_FloatNoiseInQuantityIsNotAPerpetualUpdate(t *testing.T) {
	tx := emptyTx(t)
	b := validBundle()
	b.Recipes[0].Ingredients[3].Quantity = noisyThreeTenths()
	mustApply(t, tx, parseString(t, bundleJSON(t, b)), realImport("first"))

	b.Regions[0].TileColor = "#000000"
	s := mustApply(t, tx, parseString(t, bundleJSON(t, b)), realImport("unrelated change"))

	assertChanges(t, s, map[string]importer.Counts{"regions": {Updated: 1}})
}

// noisyThreeTenths is 0.1 + 0.2 computed at run time: 0.30000000000000004.
func noisyThreeTenths() float64 {
	tenth := 0.1
	return tenth + 0.2
}

func bundleJSON(t *testing.T, b importer.Bundle) string {
	t.Helper()
	out, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("encode bundle: %v", err)
	}
	return string(out)
}

func execSQL(t *testing.T, tx pgx.Tx, sql string) {
	t.Helper()
	tag, err := tx.Exec(context.Background(), sql)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	if tag.RowsAffected() == 0 {
		t.Fatalf("%s: affected no rows, want the drift applied", sql)
	}
}
