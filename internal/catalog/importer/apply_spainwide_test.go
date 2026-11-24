package importer_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// Spanish (Spain-wide) variations through import, storage and the read
// model (BE-9, D-ESP-1 to D-ESP-5). spainWideBundle's allioli has, in bundle
// order, esp allioli-de-morter, ast allioli-asturiano, esp allioli-de-codony.

var spainWideCreated = map[string]importer.Counts{
	"regions":    {Created: 3},
	"dishes":     {Created: 3},
	"variations": {Created: 7},
	"recipes":    {Created: 7},
}

// allioliDisplayOrder is allioli's variations as served: the regional one,
// then the Spanish ones in bundle order.
var allioliDisplayOrder = []string{"allioli-asturiano", morterRecipeKey, codonyRecipeKey}

// storedVariation is a variation row as the schema holds it.
type storedVariation struct {
	hasRegion bool
	slug      *string
	position  int
}

const storedVariationsSQL = `
	SELECT v.recipe_key, v.region_id IS NOT NULL, v.slug, v.position
	FROM variations v JOIN dishes d ON d.id = v.dish_id
	WHERE d.key = $1`

func storedVariations(t *testing.T, tx pgx.Tx, dishKey string) map[string]storedVariation {
	t.Helper()
	rows, err := tx.Query(context.Background(), storedVariationsSQL, dishKey)
	if err != nil {
		t.Fatalf("query variations of %s: %v", dishKey, err)
	}
	out := map[string]storedVariation{}
	var key string
	var v storedVariation
	_, err = pgx.ForEachRow(rows, []any{&key, &v.hasRegion, &v.slug, &v.position}, func() error {
		out[key] = v
		return nil
	})
	if err != nil {
		t.Fatalf("scan variations of %s: %v", dishKey, err)
	}
	return out
}

func importedSpainWide(t *testing.T) (pgx.Tx, *catalog.Service) {
	t.Helper()
	tx := emptyTx(t)
	mustApply(t, tx, spainWideBundle(), realImport("spain-wide"))
	return tx, newService(tx)
}

func mustDish(t *testing.T, svc *catalog.Service, key string) catalog.Dish {
	t.Helper()
	d, _, err := svc.Dish(context.Background(), key, "en")
	if err != nil {
		t.Fatalf("Dish(%s): %v", key, err)
	}
	return d
}

func variationRecipeKeys(vs []catalog.Variation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.RecipeKey)
	}
	return out
}

func TestApply_SpainWideBundleReadsBackInEveryLocale(t *testing.T) {
	tx := emptyTx(t)
	b := spainWideBundle()

	s := mustApply(t, tx, b, realImport("spain-wide"))

	assertChanges(t, s, spainWideCreated)
	assertCatalogMatches(t, tx, b)
}

// A Spanish variation is stored with no region and its slug; every variation
// gets its 1-based bundle position within the dish (D-ESP-1, D-ESP-4).
func TestApply_StoresSpainWideVariationsWithoutRegion(t *testing.T) {
	tx, _ := importedSpainWide(t)

	want := map[string]storedVariation{
		morterRecipeKey:     {hasRegion: false, slug: strp(morterSlug), position: 1},
		"allioli-asturiano": {hasRegion: true, slug: nil, position: 2},
		codonyRecipeKey:     {hasRegion: false, slug: strp(codonySlug), position: 3},
	}
	assertDeepEqual(t, "allioli variations", storedVariations(t, tx, "allioli"), want)
}

func TestApply_DishListsRegionalThenSpanishVariations(t *testing.T) {
	_, svc := importedSpainWide(t)

	allioli := mustDish(t, svc, "allioli")

	assertDeepEqual(t, "allioli recipe keys", variationRecipeKeys(allioli.Variations), allioliDisplayOrder)
	for _, v := range allioli.Variations[1:] {
		if v.RegionKey != spainWideKey || v.RegionName != "" || v.RegionRank != 0 || v.VariationSlug == "" {
			t.Errorf("%s: got regionKey %q, regionName %q, regionRank %d, slug %q; want esp, no name, no rank, a slug",
				v.RecipeKey, v.RegionKey, v.RegionName, v.RegionRank, v.VariationSlug)
		}
	}
	if ast := allioli.Variations[0]; ast.VariationSlug != "" || ast.RegionRank == 0 {
		t.Errorf("allioli-asturiano: got slug %q, regionRank %d; want no slug and a rank", ast.VariationSlug, ast.RegionRank)
	}
}

func TestApply_ReimportingSpainWideBundleMakesNoChanges(t *testing.T) {
	tx, _ := importedSpainWide(t)
	before := readState(t, tx)

	s := mustApply(t, tx, spainWideBundle(), realImport("again"))

	if !s.NoChanges {
		t.Errorf("got changes %+v, want NoChanges", s.Changes)
	}
	assertStateUnchanged(t, tx, before)
}

// Changing a slug is a removal plus an addition (D-ESP-2): a Spanish variation
// is identified by (dish, esp, slug), whatever its recipe key.
func TestApply_SpainWideSlugChangeIsRemoveAndAdd(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(b *importer.Bundle)
		removed string // the old public path
	}{
		{"slug and recipe key renamed", func(b *importer.Bundle) {
			b.Dishes[2].Variations[0] = spainWideVariation("allioli", "allioli-a-mano")
			b.Recipes[4].RecipeKey = spainWideRecipeKey("allioli", "allioli-a-mano")
		}, "/dishes/allioli/esp/" + morterSlug},
		{"slug renamed, recipe key kept", func(b *importer.Bundle) {
			b.Dishes[2].Variations[0].VariationSlug = "allioli-a-mano"
		}, "/dishes/allioli/esp/" + morterSlug},
		{"regional variation made spanish, recipe key kept", func(b *importer.Bundle) {
			b.Dishes[2].Variations[1].RegionKey = spainWideKey
			b.Dishes[2].Variations[1].VariationSlug = "allioli-asturiano"
		}, "/dishes/allioli/ast"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, _ := importedSpainWide(t)
			b := spainWideBundle()
			tt.edit(&b)

			s := mustApply(t, tx, b, allowRemovals(realImport("renamed"), tt.removed))

			assertChanges(t, s, replacedOne)
			assertCatalogMatches(t, tx, b)
		})
	}
}

func TestApply_RenamedSlugServesOnlyTheNewSlug(t *testing.T) {
	tx, _ := importedSpainWide(t)
	b := spainWideBundle()
	b.Dishes[2].Variations[0].VariationSlug = "allioli-a-mano"
	mustApply(t, tx, b, allowRemovals(realImport("renamed"), "/dishes/allioli/esp/"+morterSlug))
	svc := newService(tx)

	if _, _, err := svc.SpainWideRecipe(context.Background(), "allioli", morterSlug, "en"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("SpainWideRecipe(allioli, old slug): got %v, want ErrNotFound", err)
	}
	if _, _, err := svc.SpainWideRecipe(context.Background(), "allioli", "allioli-a-mano", "en"); err != nil {
		t.Errorf("SpainWideRecipe(allioli, new slug): got %v, want nil", err)
	}
}

// Reordering a dish's Spanish variations in the bundle rewrites their
// positions; each moved variation counts as updated, like a moved dish.
func TestApply_ReorderedSpanishVariationsFollowTheBundle(t *testing.T) {
	tx, _ := importedSpainWide(t)
	b := spainWideBundle()
	vs := b.Dishes[2].Variations
	vs[0], vs[2] = vs[2], vs[0]

	s := mustApply(t, tx, b, realImport("reordered"))

	assertChanges(t, s, map[string]importer.Counts{"variations": {Updated: 2}})
	got := variationRecipeKeys(mustDish(t, newService(tx), "allioli").Variations)
	assertDeepEqual(t, "allioli recipe keys", got, []string{"allioli-asturiano", codonyRecipeKey, morterRecipeKey})
	assertCatalogMatches(t, tx, b)
}

// Regional variations are listed by region order, so reordering only them in
// the bundle changes nothing served: no changes, nothing written, and the
// stored positions stay as they were.
func TestApply_RegionalOnlyReorderReportsNoChanges(t *testing.T) {
	tx, _ := importedSpainWide(t)
	before := readState(t, tx)
	b := spainWideBundle()
	vs := b.Dishes[0].Variations
	vs[0], vs[1] = vs[1], vs[0]

	s := mustApply(t, tx, b, realImport("regional reorder"))

	if !s.NoChanges {
		t.Errorf("got changes %+v, want NoChanges", s.Changes)
	}
	assertStateUnchanged(t, tx, before)
	if got := storedVariations(t, tx, "tortilla")["tortilla-betanzos"].position; got != 1 {
		t.Errorf("tortilla-betanzos position: got %d, want the stored 1", got)
	}
}

func TestApply_RemovedSpanishVariationDeletesOnlyItsTree(t *testing.T) {
	tx, _ := importedSpainWide(t)
	b := spainWideBundle()
	b.Dishes[2].Variations = b.Dishes[2].Variations[:2]
	b.Recipes = b.Recipes[:6]

	s := mustApply(t, tx, b, allowRemovals(realImport("without codony"), "/dishes/allioli/esp/"+codonySlug))

	assertChanges(t, s, map[string]importer.Counts{"variations": {Deleted: 1}, "recipes": {Deleted: 1}})
	assertCatalogMatches(t, tx, b)
}

// A position edited by hand is repaired by re-importing, like any drift.
func TestApply_ReimportRepairsPositionDrift(t *testing.T) {
	tx, _ := importedSpainWide(t)
	execSQL(t, tx, "UPDATE variations SET position = 9 WHERE recipe_key = '"+codonyRecipeKey+"'")

	s := mustApply(t, tx, spainWideBundle(), realImport("repair"))

	assertChanges(t, s, map[string]importer.Counts{"variations": {Updated: 1}})
	if got := storedVariations(t, tx, "allioli")[codonyRecipeKey].position; got != 3 {
		t.Errorf("codony position: got %d, want 3", got)
	}
}

// A dish's score sums all its variations, Spanish ones included (D-ESP-5).
func TestApply_SpanishVariationScoreCountsInDishScore(t *testing.T) {
	tx := emptyTx(t)
	b := spainWideBundle()
	b.Dishes[0].Variations[0].Score = 10 // tortilla 10
	b.Dishes[2].Variations[0].Score = 6  // allioli 6 + 5 = 11, Spanish only
	b.Dishes[2].Variations[2].Score = 5

	mustApply(t, tx, b, realImport("scored"))

	dishes, _, err := newService(tx).Dishes(context.Background(), "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	assertDeepEqual(t, "dish keys and orders", dishKeyOrders(dishes), []string{"1 allioli", "2 tortilla", "3 cocido"})
	assertCatalogMatches(t, tx, b)
}

// esp is never a region: no region list holds it or a Spanish variation
// (D-ESP-5).
func TestApply_RegionListsLeaveOutSpanishVariations(t *testing.T) {
	_, svc := importedSpainWide(t)
	ctx := context.Background()

	regions, _, err := svc.Regions(ctx, "en")
	if err != nil {
		t.Fatalf("Regions: %v", err)
	}
	for _, r := range regions {
		assertNoSpanishVariationIn(t, svc, r.Key)
	}
	if slices.ContainsFunc(regions, func(r catalog.Region) bool { return r.Key == spainWideKey }) {
		t.Errorf("regions: got esp among %d regions, want none", len(regions))
	}
	if _, _, err := svc.Region(ctx, spainWideKey, "en"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("Region(esp): got %v, want ErrNotFound", err)
	}
}

func assertNoSpanishVariationIn(t *testing.T, svc *catalog.Service, regionKey string) {
	t.Helper()
	detail, _, err := svc.Region(context.Background(), regionKey, "en")
	if err != nil {
		t.Fatalf("Region(%s): %v", regionKey, err)
	}
	for _, v := range detail.Variations {
		if v.RegionKey != regionKey || v.VariationSlug != "" {
			t.Errorf("region %s: got variation %s (regionKey %q, slug %q), want only its own regional ones",
				regionKey, v.RecipeKey, v.RegionKey, v.VariationSlug)
		}
	}
}

func TestApply_SpainWideRecipeLookup(t *testing.T) {
	_, svc := importedSpainWide(t)

	rc, _, err := svc.SpainWideRecipe(context.Background(), "allioli", codonySlug, "ru")
	if err != nil {
		t.Fatalf("SpainWideRecipe(allioli, %s): %v", codonySlug, err)
	}
	if rc.RecipeKey != codonyRecipeKey || rc.RegionKey != spainWideKey || rc.RegionName != "" || rc.VariationSlug != codonySlug {
		t.Errorf("got recipe %s, regionKey %q, regionName %q, slug %q; want %s, esp, no name, %s",
			rc.RecipeKey, rc.RegionKey, rc.RegionName, rc.VariationSlug, codonyRecipeKey, codonySlug)
	}
}

func TestApply_SpainWideRecipeNotFound(t *testing.T) {
	_, svc := importedSpainWide(t)
	tests := []struct{ name, dish, slug string }{
		{"unknown slug", "allioli", "allioli-a-mano"},
		{"a regional variation's recipe key as slug", "allioli", "allioli-asturiano"},
		{"slug of another dish", "tortilla", morterSlug},
		{"unknown dish", "salmorejo", morterSlug},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := svc.SpainWideRecipe(context.Background(), tt.dish, tt.slug, "en")
			if !errors.Is(err, catalog.ErrNotFound) {
				t.Errorf("SpainWideRecipe(%s, %s): got %v, want ErrNotFound", tt.dish, tt.slug, err)
			}
		})
	}
}

// The regional lookup never finds a Spanish variation: esp is not a region.
func TestApply_RegionalRecipeLookupWithEspIsNotFound(t *testing.T) {
	_, svc := importedSpainWide(t)

	assertNoRecipe(t, svc, pairKey{"allioli", spainWideKey})
}
