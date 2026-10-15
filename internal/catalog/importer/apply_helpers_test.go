package importer_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

var locales = []catalog.Locale{"en", "es", "ru"}

// emptyTx is a rolled-back transaction in which the catalog and the content
// versions start empty, whatever the shared test database holds.
func emptyTx(t *testing.T) pgx.Tx {
	t.Helper()
	tx := testdb.Tx(t)
	for _, sql := range []string{"DELETE FROM dishes", "DELETE FROM regions", "DELETE FROM content_versions"} {
		if _, err := tx.Exec(context.Background(), sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	return tx
}

func mustApply(t *testing.T, db importer.Beginner, b importer.Bundle, opts importer.Options) importer.Summary {
	t.Helper()
	s, err := importer.Apply(context.Background(), db, b, opts)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return s
}

func realImport(source string) importer.Options { return importer.Options{Source: source} }

func dryRun(source string) importer.Options { return importer.Options{Source: source, DryRun: true} }

// assertChanges compares every entity's counts; entities missing from want
// must be all zero.
func assertChanges(t *testing.T, s importer.Summary, want map[string]importer.Counts) {
	t.Helper()
	for _, e := range entities {
		if got := s.Changes[e]; got != want[e] {
			t.Errorf("%s: got %+v, want %+v", e, got, want[e])
		}
	}
}

func assertDeepEqual(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\ngot  %+v\nwant %+v", what, got, want)
	}
}

const tableCountsSQL = `
SELECT 'regions', count(*) FROM regions
UNION ALL SELECT 'region_translations', count(*) FROM region_translations
UNION ALL SELECT 'region_regional_dishes', count(*) FROM region_regional_dishes
UNION ALL SELECT 'dishes', count(*) FROM dishes
UNION ALL SELECT 'dish_translations', count(*) FROM dish_translations
UNION ALL SELECT 'variations', count(*) FROM variations
UNION ALL SELECT 'variation_translations', count(*) FROM variation_translations
UNION ALL SELECT 'recipes', count(*) FROM recipes
UNION ALL SELECT 'recipe_translations', count(*) FROM recipe_translations
UNION ALL SELECT 'recipe_ingredients', count(*) FROM recipe_ingredients
UNION ALL SELECT 'recipe_ingredient_translations', count(*) FROM recipe_ingredient_translations
UNION ALL SELECT 'recipe_steps', count(*) FROM recipe_steps
UNION ALL SELECT 'recipe_step_translations', count(*) FROM recipe_step_translations
UNION ALL SELECT 'content_versions', count(*) FROM content_versions`

// rowIDsSQL names every catalog row by its natural key, so tests can tell an
// update in place from a delete and re-insert.
const rowIDsSQL = `
SELECT 'region:' || key, id FROM regions
UNION ALL SELECT 'dish:' || key, id FROM dishes
UNION ALL SELECT 'variation:' || recipe_key, id FROM variations
UNION ALL SELECT 'recipe:' || v.recipe_key, r.id FROM recipes r JOIN variations v ON v.id = r.variation_id`

func tableCounts(t *testing.T, tx pgx.Tx) map[string]int64 {
	t.Helper()
	return queryMap(t, tx, tableCountsSQL)
}

func rowIDs(t *testing.T, tx pgx.Tx) map[string]int64 {
	t.Helper()
	return queryMap(t, tx, rowIDsSQL)
}

func queryMap(t *testing.T, tx pgx.Tx, sql string) map[string]int64 {
	t.Helper()
	rows, err := tx.Query(context.Background(), sql)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	out := map[string]int64{}
	var name string
	var n int64
	_, err = pgx.ForEachRow(rows, []any{&name, &n}, func() error {
		out[name] = n
		return nil
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

// dbState is everything a failed or dry-run import must leave untouched.
type dbState struct {
	counts  map[string]int64
	ids     map[string]int64
	regions []catalog.Region
	dishes  []catalog.Dish
	version int64
}

func readState(t *testing.T, tx pgx.Tx) dbState {
	t.Helper()
	ctx := context.Background()
	svc := newService(tx)
	regions, version, err := svc.Regions(ctx, "ru")
	if err != nil {
		t.Fatalf("Regions: %v", err)
	}
	dishes, _, err := svc.Dishes(ctx, "es")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	return dbState{counts: tableCounts(t, tx), ids: rowIDs(t, tx), regions: regions, dishes: dishes, version: version}
}

func assertStateUnchanged(t *testing.T, tx pgx.Tx, before dbState) {
	t.Helper()
	after := readState(t, tx)
	assertDeepEqual(t, "table counts", after.counts, before.counts)
	assertDeepEqual(t, "row ids", after.ids, before.ids)
	assertDeepEqual(t, "regions", after.regions, before.regions)
	assertDeepEqual(t, "dishes", after.dishes, before.dishes)
	if after.version != before.version {
		t.Errorf("content version: got %d, want %d", after.version, before.version)
	}
}

// newService reads through a fresh Service, so nothing is memoised.
func newService(tx pgx.Tx) *catalog.Service {
	return catalog.NewService(catalog.NewPGRepository(tx))
}

// assertCatalogMatches reads every list and recipe back in every locale and
// compares it with what the bundle describes.
func assertCatalogMatches(t *testing.T, tx pgx.Tx, b importer.Bundle) {
	t.Helper()
	ctx := context.Background()
	svc := newService(tx)
	idx := indexBundle(b)
	for _, l := range locales {
		regions, _, err := svc.Regions(ctx, l)
		if err != nil {
			t.Fatalf("Regions(%s): %v", l, err)
		}
		assertDeepEqual(t, "regions "+string(l), regions, idx.regions(l))
		dishes, _, err := svc.Dishes(ctx, l)
		if err != nil {
			t.Fatalf("Dishes(%s): %v", l, err)
		}
		assertDeepEqual(t, "dishes "+string(l), dishes, idx.dishes(l))
		assertRecipesMatch(t, svc, idx, l)
	}
}

func assertRecipesMatch(t *testing.T, svc *catalog.Service, idx bundleIndex, l catalog.Locale) {
	t.Helper()
	for _, p := range idx.pairs() {
		got, _, err := svc.Recipe(context.Background(), p.dish.Key, p.variation.RegionKey, l)
		if err != nil {
			t.Fatalf("Recipe(%s/%s, %s): %v", p.dish.Key, p.variation.RegionKey, l, err)
		}
		assertDeepEqual(t, "recipe "+p.dish.Key+"/"+p.variation.RegionKey+" "+string(l), got, idx.recipe(p, l))
	}
}

// bundleIndex derives the catalog read model a bundle should produce.
type bundleIndex struct {
	b           importer.Bundle
	regionByKey map[string]importer.Region
	regionOrder map[string]int
	recipeByKey map[string]importer.Recipe
}

type dishVariation struct {
	dish      importer.Dish
	variation importer.Variation
}

func indexBundle(b importer.Bundle) bundleIndex {
	idx := bundleIndex{b: b, regionByKey: map[string]importer.Region{}, regionOrder: map[string]int{}, recipeByKey: map[string]importer.Recipe{}}
	for i, r := range b.Regions {
		idx.regionByKey[r.Key] = r
		idx.regionOrder[r.Key] = i + 1
	}
	for _, rc := range b.Recipes {
		idx.recipeByKey[rc.RecipeKey] = rc
	}
	return idx
}

func pick(text importer.Text, l catalog.Locale) string {
	switch l {
	case "es":
		return text.Es
	case "ru":
		return text.Ru
	default:
		return text.En
	}
}

func (idx bundleIndex) regions(l catalog.Locale) []catalog.Region {
	out := make([]catalog.Region, 0, len(idx.b.Regions))
	for i, r := range idx.b.Regions {
		out = append(out, catalog.Region{
			Key:            r.Key,
			Order:          i + 1,
			Name:           pick(r.Name, l),
			NameEs:         r.Name.Es,
			MapLabel:       pick(r.MapLabel, l),
			TileColor:      r.TileColor,
			Description:    pick(r.Description, l),
			RegionalDishes: r.RegionalDishes,
		})
	}
	return out
}

func (idx bundleIndex) dishes(l catalog.Locale) []catalog.Dish {
	out := make([]catalog.Dish, 0, len(idx.b.Dishes))
	for i, d := range idx.b.Dishes {
		out = append(out, catalog.Dish{
			Key:         d.Key,
			Order:       i + 1,
			Name:        pick(d.Name, l),
			NameEs:      d.Name.Es,
			Description: pick(d.Description, l),
			Flags:       flagsOf(d.Flags),
			Course:      d.Course,
			Season:      d.Season,
			Photos:      photosOf(d.Photos),
			Variations:  idx.variations(d, l),
		})
	}
	return out
}

// variations lists the dish's variations in region order.
func (idx bundleIndex) variations(d importer.Dish, l catalog.Locale) []catalog.Variation {
	out := make([]catalog.Variation, 0, len(d.Variations))
	for _, v := range d.Variations {
		out = append(out, catalog.Variation{
			RegionKey:   v.RegionKey,
			RegionName:  pick(idx.regionByKey[v.RegionKey].Name, l),
			RecipeKey:   v.RecipeKey,
			Name:        pick(v.Name, l),
			NameEs:      v.Name.Es,
			Description: pick(v.Description, l),
			Photos:      photosOf(v.Photos),
		})
	}
	slices.SortFunc(out, func(a, b catalog.Variation) int {
		return idx.regionOrder[a.RegionKey] - idx.regionOrder[b.RegionKey]
	})
	return out
}

func (idx bundleIndex) pairs() []dishVariation {
	var out []dishVariation
	for _, d := range idx.b.Dishes {
		for _, v := range d.Variations {
			out = append(out, dishVariation{dish: d, variation: v})
		}
	}
	return out
}

func (idx bundleIndex) recipe(p dishVariation, l catalog.Locale) catalog.Recipe {
	rc := idx.recipeByKey[p.variation.RecipeKey]
	out := catalog.Recipe{
		DishKey:      p.dish.Key,
		DishName:     pick(p.dish.Name, l),
		RegionKey:    p.variation.RegionKey,
		RegionName:   pick(idx.regionByKey[p.variation.RegionKey].Name, l),
		RecipeKey:    rc.RecipeKey,
		Name:         pick(p.variation.Name, l),
		NameEs:       p.variation.Name.Es,
		Description:  pick(p.variation.Description, l),
		TimeMinutes:  rc.TimeMinutes,
		BaseServings: rc.BaseServings,
		Photos:       photosOf(p.variation.Photos),
	}
	if rc.TimeNote != nil {
		note := pick(*rc.TimeNote, l)
		out.TimeNote = &note
	}
	for _, ing := range rc.Ingredients {
		out.Ingredients = append(out.Ingredients, catalog.Ingredient{Quantity: ing.Quantity, Unit: ing.Unit, Name: pick(ing.Name, l)})
	}
	for _, s := range rc.Steps {
		out.Steps = append(out.Steps, pick(s, l))
	}
	return out
}

func flagsOf(f importer.Flags) catalog.Flags {
	return catalog.Flags{Vegetarian: f.Vegetarian, Seafood: f.Seafood, Pork: f.Pork, Quick: f.Quick, SlowCooked: f.SlowCooked}
}

func photosOf(p *importer.Photos) catalog.Photos {
	if p == nil {
		return catalog.Photos{}
	}
	return catalog.Photos{Thumb: p.Thumb, Card: p.Card, Hero: p.Hero}
}

// recipeTreeRows counts the rows a variation and its recipe occupy, by table.
func recipeTreeRows(rc importer.Recipe) map[string]int64 {
	rows := map[string]int64{
		"variations":                     1,
		"variation_translations":         3,
		"recipes":                        1,
		"recipe_ingredients":             int64(len(rc.Ingredients)),
		"recipe_ingredient_translations": 3 * int64(len(rc.Ingredients)),
		"recipe_steps":                   int64(len(rc.Steps)),
		"recipe_step_translations":       3 * int64(len(rc.Steps)),
	}
	if rc.TimeNote != nil {
		rows["recipe_translations"] = 3
	}
	return rows
}
