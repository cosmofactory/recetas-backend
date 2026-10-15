package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

const (
	insertRegionSQL       = `INSERT INTO regions (key, sort_order, tile_color) VALUES ($1, $2, $3)`
	insertDishSQL         = `INSERT INTO dishes (key, sort_order, course, season) VALUES ($1, $2, $3, $4)`
	insertVariationSQL    = `INSERT INTO variations (dish_id, region_id, recipe_key) VALUES ($1, $2, $3)`
	insertRecipeSQL       = `INSERT INTO recipes (variation_id, time_minutes, base_servings) VALUES ($1, $2, $3)`
	insertIngredientSQL   = `INSERT INTO recipe_ingredients (recipe_id, position, quantity, unit) VALUES ($1, $2, $3, $4)`
	insertStepSQL         = `INSERT INTO recipe_steps (recipe_id, position) VALUES ($1, $2)`
	insertRegionalDishSQL = `INSERT INTO region_regional_dishes (region_id, position, name) VALUES ($1, $2, $3)`

	// Ingredient positions used by the non-table subtests, clear of the
	// table cases' 1..n.
	scalePosition     = 100
	duplicatePosition = 200
)

type regionCase struct {
	name  string
	key   string
	order int
	color string
	want  string
}

func regionCases(t *testing.T, takenKey string, takenOrder int) []regionCase {
	return []regionCase{
		{"valid", uniqKey(t, "ast"), uniqOrder(t), "#C97B4A", codeOK},
		{"valid lowercase hex", uniqKey(t, "bas"), uniqOrder(t), "#c4552e", codeOK},
		{"valid digits in slug", uniqKey(t, "a1"), uniqOrder(t), "#D9A441", codeOK},
		{"slug uppercase", "Gal" + randHex(t, 3), uniqOrder(t), baselineTileColor, codeCheck},
		{"slug space", "ga l" + randHex(t, 3), uniqOrder(t), baselineTileColor, codeCheck},
		{"slug leading hyphen", "-gal" + randHex(t, 3), uniqOrder(t), baselineTileColor, codeCheck},
		{"slug trailing hyphen", "gal" + randHex(t, 3) + "-", uniqOrder(t), baselineTileColor, codeCheck},
		{"slug double hyphen", "ga--l" + randHex(t, 3), uniqOrder(t), baselineTileColor, codeCheck},
		{"slug underscore", "ga_l" + randHex(t, 3), uniqOrder(t), baselineTileColor, codeCheck},
		{"slug accented", "galá" + randHex(t, 3), uniqOrder(t), baselineTileColor, codeCheck},
		{"slug empty", "", uniqOrder(t), baselineTileColor, codeCheck},
		{"duplicate key", takenKey, uniqOrder(t), baselineTileColor, codeUnique},
		{"duplicate sort_order", uniqKey(t, "cat"), takenOrder, baselineTileColor, codeUnique},
		{"tile_color without hash", uniqKey(t, "cyl"), uniqOrder(t), "9A9A5E", codeCheck},
		{"tile_color five digits", uniqKey(t, "mad"), uniqOrder(t), "#9A9A5", codeCheck},
		{"tile_color seven digits", uniqKey(t, "ara"), uniqOrder(t), "#9A9A5E0", codeCheck},
		{"tile_color not hex", uniqKey(t, "val"), uniqOrder(t), "#9A9A5G", codeCheck},
		{"tile_color named", uniqKey(t, "ext"), uniqOrder(t), "olive", codeCheck},
	}
}

func TestIntegrationRegionConstraints(t *testing.T) {
	tx := testdb.Tx(t)
	takenKey, takenOrder := uniqKey(t, "gal"), uniqOrder(t)
	mustExec(t, tx, insertRegionSQL, takenKey, takenOrder, baselineTileColor)

	for _, tc := range regionCases(t, takenKey, takenOrder) {
		t.Run(tc.name, func(t *testing.T) {
			wantCode(t, try(t, tx, insertRegionSQL, tc.key, tc.order, tc.color), tc.want)
		})
	}
	t.Run("null tile_color", func(t *testing.T) {
		wantCode(t, try(t, tx, insertRegionSQL, uniqKey(t, "clm"), uniqOrder(t), nil), codeNotNull)
	})
}

type dishCase struct {
	name   string
	key    string
	order  int
	course string
	season string
	want   string
}

func dishCases(t *testing.T, takenKey string, takenOrder int) []dishCase {
	return []dishCase{
		{"course tapas", uniqKey(t, "d"), uniqOrder(t), "tapas", "summer", codeOK},
		{"course mains", uniqKey(t, "d"), uniqOrder(t), "mains", "winter", codeOK},
		{"course desserts", uniqKey(t, "d"), uniqOrder(t), "desserts", "easter", codeOK},
		{"season all", uniqKey(t, "d"), uniqOrder(t), "desserts", "all", codeOK},
		{"slug uppercase", "Tortilla" + randHex(t, 3), uniqOrder(t), "tapas", "all", codeCheck},
		{"slug space", "tortilla de" + randHex(t, 3), uniqOrder(t), "tapas", "all", codeCheck},
		{"slug underscore", "tortilla_" + randHex(t, 3), uniqOrder(t), "tapas", "all", codeCheck},
		{"duplicate key", takenKey, uniqOrder(t), "tapas", "all", codeUnique},
		{"duplicate sort_order", uniqKey(t, "d"), takenOrder, "tapas", "all", codeUnique},
		{"unknown course", uniqKey(t, "d"), uniqOrder(t), "starters", "all", codeCheck},
		{"course wrong case", uniqKey(t, "d"), uniqOrder(t), "Tapas", "all", codeCheck},
		{"unknown season", uniqKey(t, "d"), uniqOrder(t), "tapas", "spring", codeCheck},
		{"season empty", uniqKey(t, "d"), uniqOrder(t), "tapas", "", codeCheck},
	}
}

func TestIntegrationDishConstraints(t *testing.T) {
	tx := testdb.Tx(t)
	takenKey, takenOrder := uniqKey(t, "tortilla"), uniqOrder(t)
	mustExec(t, tx, insertDishSQL, takenKey, takenOrder, "tapas", "all")

	for _, tc := range dishCases(t, takenKey, takenOrder) {
		t.Run(tc.name, func(t *testing.T) {
			wantCode(t, try(t, tx, insertDishSQL, tc.key, tc.order, tc.course, tc.season), tc.want)
		})
	}
	t.Run("flags default to false", func(t *testing.T) { assertDishFlagsDefaultFalse(t, tx) })
	t.Run("null flag rejected", func(t *testing.T) {
		err := try(t, tx,
			`INSERT INTO dishes (key, sort_order, course, season, is_vegetarian) VALUES ($1, $2, 'tapas', 'all', NULL)`,
			uniqKey(t, "d"), uniqOrder(t))
		wantCode(t, err, codeNotNull)
	})
	t.Run("photo keys nullable", func(t *testing.T) { assertDishPhotosDefaultNull(t, tx) })
}

func assertDishFlagsDefaultFalse(t *testing.T, tx pgx.Tx) {
	t.Helper()
	id := insertDish(t, tx)
	var veg, seafood, pork, quick, slow bool
	err := tx.QueryRow(context.Background(), `
		SELECT is_vegetarian, is_seafood, has_pork, is_quick, is_slow_cooked
		FROM dishes WHERE id = $1`, id).Scan(&veg, &seafood, &pork, &quick, &slow)
	if err != nil {
		t.Fatalf("select flags: %v", err)
	}
	if veg || seafood || pork || quick || slow {
		t.Errorf("flags: got %v %v %v %v %v, want all false", veg, seafood, pork, quick, slow)
	}
}

func assertDishPhotosDefaultNull(t *testing.T, tx pgx.Tx) {
	t.Helper()
	id := insertDish(t, tx)
	var thumb, card, hero *string
	err := tx.QueryRow(context.Background(),
		`SELECT photo_thumb, photo_card, photo_hero FROM dishes WHERE id = $1`, id).Scan(&thumb, &card, &hero)
	if err != nil {
		t.Fatalf("select photos: %v", err)
	}
	if thumb != nil || card != nil || hero != nil {
		t.Errorf("photos: got %v %v %v, want all NULL", thumb, card, hero)
	}
}

type variationCase struct {
	name      string
	dish      int64
	region    int64
	recipeKey string
	want      string
}

// variationBaseline is two dishes and two regions, with tortilla × gal
// already taken under takenKey.
type variationBaseline struct {
	tortilla, cocido, gal, ast int64
	takenKey                   string
}

func newVariationBaseline(t *testing.T, tx pgx.Tx) variationBaseline {
	t.Helper()
	b := variationBaseline{
		gal: insertRegion(t, tx), ast: insertRegion(t, tx),
		tortilla: insertDish(t, tx), cocido: insertDish(t, tx),
		takenKey: uniqKey(t, "tortilla-betanzos"),
	}
	mustExec(t, tx, insertVariationSQL, b.tortilla, b.gal, b.takenKey)
	return b
}

func variationCases(t *testing.T, b variationBaseline) []variationCase {
	tortilla, cocido, gal, ast, takenKey := b.tortilla, b.cocido, b.gal, b.ast, b.takenKey
	return []variationCase{
		{"same dish other region", tortilla, ast, uniqKey(t, "tortilla-chorizo"), codeOK},
		{"same region other dish", cocido, gal, uniqKey(t, "cocido-gallego"), codeOK},
		{"duplicate dish and region", tortilla, gal, uniqKey(t, "tortilla-other"), codeUnique},
		{"duplicate recipe_key", cocido, ast, takenKey, codeUnique},
		{"recipe_key free text", cocido, ast, "Betanzos-style runny", codeCheck},
		{"recipe_key uppercase", cocido, ast, "Fabada-" + randHex(t, 3), codeCheck},
		{"recipe_key trailing hyphen", cocido, ast, "fabada-" + randHex(t, 3) + "-", codeCheck},
		{"recipe_key empty", cocido, ast, "", codeCheck},
		{"unknown dish", unknownID, ast, uniqKey(t, "x"), codeForeignKey},
		{"unknown region", cocido, unknownID, uniqKey(t, "x"), codeForeignKey},
	}
}

func TestIntegrationVariationConstraints(t *testing.T) {
	tx := testdb.Tx(t)
	b := newVariationBaseline(t, tx)

	for _, tc := range variationCases(t, b) {
		t.Run(tc.name, func(t *testing.T) {
			wantCode(t, try(t, tx, insertVariationSQL, tc.dish, tc.region, tc.recipeKey), tc.want)
		})
	}
	t.Run("null recipe_key", func(t *testing.T) {
		wantCode(t, try(t, tx, insertVariationSQL, insertDish(t, tx), b.gal, nil), codeNotNull)
	})
}

// forbiddenVariationColumns would rank or single out a variation; all
// variations are equal and ordered by region.
var forbiddenVariationColumns = map[string]bool{
	"is_primary": true, "primary": true, "is_default": true, "is_featured": true,
	"is_best": true, "rank": true, "position": true, "sort_order": true, "rating": true,
}

func TestIntegrationVariationsHaveNoPrimaryOrRank(t *testing.T) {
	tx := testdb.Tx(t)
	rows, err := tx.Query(context.Background(), `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'variations'
		ORDER BY column_name`)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	cols, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(cols) == 0 {
		t.Fatal("variations: got no columns, want the table to exist")
	}
	for _, c := range cols {
		if forbiddenVariationColumns[c] {
			t.Errorf("variations.%s: got column, want none (all variations are equal, ordered by region)", c)
		}
	}
}

var recipeCases = []struct {
	name     string
	minutes  any
	servings any
	want     string
}{
	{"valid", 25, 2, codeOK},
	{"servings 1", 25, 1, codeOK},
	{"servings 20", 25, 20, codeOK},
	{"time 1 minute", 1, 4, codeOK},
	{"servings 0", 25, 0, codeCheck},
	{"servings 21", 25, 21, codeCheck},
	{"servings negative", 25, -2, codeCheck},
	{"time 0", 0, 2, codeCheck},
	{"time negative", -5, 2, codeCheck},
	{"time null", nil, 2, codeNotNull},
	{"servings null", 25, nil, codeNotNull},
}

func TestIntegrationRecipeConstraints(t *testing.T) {
	tx := testdb.Tx(t)
	newVariation := func(t *testing.T) int64 {
		t.Helper()
		return insertVariation(t, tx, insertDish(t, tx), insertRegion(t, tx))
	}

	for _, tc := range recipeCases {
		t.Run(tc.name, func(t *testing.T) {
			wantCode(t, try(t, tx, insertRecipeSQL, newVariation(t), tc.minutes, tc.servings), tc.want)
		})
	}
	t.Run("second recipe for a variation", func(t *testing.T) {
		v := newVariation(t)
		mustExec(t, tx, insertRecipeSQL, v, 25, 2)
		wantCode(t, try(t, tx, insertRecipeSQL, v, 30, 4), codeUnique)
	})
	t.Run("unknown variation", func(t *testing.T) {
		wantCode(t, try(t, tx, insertRecipeSQL, unknownID, 25, 2), codeForeignKey)
	})
}

var ingredientCases = []struct {
	name     string
	quantity any
	unit     any
	want     string
}{
	{"g", 400, "g", codeOK},
	{"ml", 250, "ml", codeOK},
	{"tsp", 1, "tsp", codeOK},
	{"null unit is a counted item", 4, nil, codeOK},
	{"fractional quantity", 0.5, "tsp", codeOK},
	{"quantity 0", 0, "g", codeCheck},
	{"quantity negative", -1, "g", codeCheck},
	{"quantity null", nil, "g", codeNotNull},
	{"unit kg", 1, "kg", codeCheck},
	{"unit cup", 1, "cup", codeCheck},
	{"unit L", 1, "L", codeCheck},
	{"unit empty", 1, "", codeCheck},
	{"unit wrong case", 1, "G", codeCheck},
}

func TestIntegrationIngredientConstraints(t *testing.T) {
	tx := testdb.Tx(t)
	recipeID := insertFullRecipe(t, tx).recipe

	for i, tc := range ingredientCases {
		t.Run(tc.name, func(t *testing.T) {
			wantCode(t, try(t, tx, insertIngredientSQL, recipeID, firstPosition+i, tc.quantity, tc.unit), tc.want)
		})
	}
	t.Run("quantity keeps three decimals", func(t *testing.T) { assertQuantityScale(t, tx, recipeID) })
	t.Run("duplicate position", func(t *testing.T) {
		mustExec(t, tx, insertIngredientSQL, recipeID, duplicatePosition, 1, nil)
		wantCode(t, try(t, tx, insertIngredientSQL, recipeID, duplicatePosition, 2, nil), codeUnique)
	})
	t.Run("duplicate step position", func(t *testing.T) {
		mustExec(t, tx, insertStepSQL, recipeID, firstPosition)
		wantCode(t, try(t, tx, insertStepSQL, recipeID, firstPosition), codeUnique)
	})
	t.Run("unknown recipe", func(t *testing.T) {
		wantCode(t, try(t, tx, insertIngredientSQL, unknownID, firstPosition, 1, nil), codeForeignKey)
	})
}

// assertQuantityScale checks numeric(10,3): 1.25 reads back as 1.250.
func assertQuantityScale(t *testing.T, tx pgx.Tx, recipeID int64) {
	t.Helper()
	mustExec(t, tx, insertIngredientSQL, recipeID, scalePosition, "1.25", "tsp")
	got := queryOne[string](t, tx,
		`SELECT quantity::text FROM recipe_ingredients WHERE recipe_id = $1 AND position = $2`, recipeID, scalePosition)
	if got != "1.250" {
		t.Errorf("quantity: got %s, want 1.250 (numeric(10,3))", got)
	}
}

func TestIntegrationRegionalDishConstraints(t *testing.T) {
	tx := testdb.Tx(t)
	regionID := insertRegion(t, tx)

	wantCode(t, try(t, tx, insertRegionalDishSQL, regionID, firstPosition, "Pulpo á feira"), codeOK)
	wantCode(t, try(t, tx, insertRegionalDishSQL, regionID, firstPosition, "Empanada gallega"), codeUnique)
	wantCode(t, try(t, tx, insertRegionalDishSQL, regionID, firstPosition+1, nil), codeNotNull)
	wantCode(t, try(t, tx, insertRegionalDishSQL, unknownID, firstPosition, "Cachopo"), codeForeignKey)
}
