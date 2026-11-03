package importer_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// setScore sets the score of dishKey's variation in regionKey.
func setScore(t *testing.T, b *importer.Bundle, p pairKey, score float64) {
	t.Helper()
	di := slices.IndexFunc(b.Dishes, func(d importer.Dish) bool { return d.Key == p.dish })
	if di < 0 {
		t.Fatalf("bundle has no dish %s", p.dish)
	}
	variations := b.Dishes[di].Variations
	vi := slices.IndexFunc(variations, func(v importer.Variation) bool { return v.RegionKey == p.region })
	if vi < 0 {
		t.Fatalf("bundle has no variation %s/%s", p.dish, p.region)
	}
	variations[vi].Score = score
}

// scoredBundle is validBundle with scores that reorder it: cocido
// (mad 1 + ast 20 = 21) overtakes tortilla (gal 5 + ast 2 = 7), and in
// Asturias fabada (20) ranks above tortilla con chorizo (2).
func scoredBundle(t *testing.T) importer.Bundle {
	t.Helper()
	b := validBundle()
	setScore(t, &b, pairKey{"tortilla", "gal"}, 5)
	setScore(t, &b, pairKey{"tortilla", "ast"}, 2)
	setScore(t, &b, pairKey{"cocido", "mad"}, 1)
	setScore(t, &b, pairKey{"cocido", "ast"}, 20)
	return b
}

func storedScores(t *testing.T, tx pgx.Tx) map[string]float64 {
	t.Helper()
	rows, err := tx.Query(context.Background(), "SELECT recipe_key, score FROM variations")
	if err != nil {
		t.Fatalf("query scores: %v", err)
	}
	out := map[string]float64{}
	var key string
	var score float64
	_, err = pgx.ForEachRow(rows, []any{&key, &score}, func() error {
		out[key] = score
		return nil
	})
	if err != nil {
		t.Fatalf("scan scores: %v", err)
	}
	return out
}

func TestApply_StoresScores(t *testing.T) {
	tx := emptyTx(t)
	mustApply(t, tx, scoredBundle(t), realImport("scored"))

	want := map[string]float64{
		"tortilla-betanzos": 5, "tortilla-chorizo": 2, "cocido-madrileno": 1, "fabada-asturiana": 20,
	}
	assertDeepEqual(t, "stored scores", storedScores(t, tx), want)
}

func TestApply_MissingScoreIsStoredAsZero(t *testing.T) {
	tx := emptyTx(t)
	mustApply(t, tx, validBundle(), realImport("unscored"))

	scores := storedScores(t, tx)
	requireLen(t, "variations", len(scores), 4)
	for key, score := range scores {
		if score != 0 {
			t.Errorf("%s: got score %v, want 0", key, score)
		}
	}
}

func TestApply_ChangedScoreUpdatesOnlyThatVariation(t *testing.T) {
	tests := []struct {
		name  string
		base  func(t *testing.T) importer.Bundle
		score float64
	}{
		{"from no score", func(*testing.T) importer.Bundle { return validBundle() }, 3.5},
		{"to another score", scoredBundle, 7},
		{"back to zero", scoredBundle, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := emptyTx(t)
			mustApply(t, tx, tt.base(t), realImport("base"))
			idsBefore := rowIDs(t, tx)

			b := tt.base(t)
			setScore(t, &b, pairKey{"tortilla", "ast"}, tt.score)
			s := mustApply(t, tx, b, realImport("rescored"))

			assertChanges(t, s, map[string]importer.Counts{"variations": {Updated: 1}})
			assertDeepEqual(t, "row ids", rowIDs(t, tx), idsBefore)
			assertCatalogMatches(t, tx, b)
		})
	}
}

func TestApply_ReimportingAScoredBundleMakesNoChanges(t *testing.T) {
	tx := emptyTx(t)
	first := mustApply(t, tx, scoredBundle(t), realImport("first"))
	before := readState(t, tx)

	again := mustApply(t, tx, scoredBundle(t), realImport("second"))

	if !again.NoChanges {
		t.Error("got changes, want NoChanges")
	}
	assertChanges(t, again, nil)
	if again.Version != first.Version {
		t.Errorf("got version %d, want the unchanged %d", again.Version, first.Version)
	}
	assertStateUnchanged(t, tx, before)
}

// Float noise in a score is rounded when parsed, so the next import of the
// same content compares equal to what the database holds.
func TestApply_FloatNoiseInScoreIsNotAPerpetualUpdate(t *testing.T) {
	tx := emptyTx(t)
	b := validBundle()
	setScore(t, &b, pairKey{"tortilla", "gal"}, noisyThreeTenths())
	mustApply(t, tx, parseString(t, bundleJSON(t, b)), realImport("first"))

	s := mustApply(t, tx, parseString(t, bundleJSON(t, b)), realImport("again"))

	if !s.NoChanges {
		t.Errorf("got changes %+v, want NoChanges", s.Changes)
	}
}

func TestApply_ScoredBundleReordersDishesAndRegionLists(t *testing.T) {
	tx := emptyTx(t)
	b := scoredBundle(t)
	mustApply(t, tx, b, realImport("scored"))
	svc := newService(tx)
	ctx := context.Background()

	dishes, _, err := svc.Dishes(ctx, "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	assertDeepEqual(t, "dish keys and orders", dishKeyOrders(dishes), []string{"1 cocido", "2 tortilla"})
	ast, _, err := svc.Region(ctx, "ast", "en")
	if err != nil {
		t.Fatalf("Region: %v", err)
	}
	assertDeepEqual(t, "asturias list", regionListKeys(ast.Variations), []string{"1 fabada-asturiana", "2 tortilla-chorizo"})
	assertCatalogMatches(t, tx, b)
}

func dishKeyOrders(ds []catalog.Dish) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, fmt.Sprintf("%d %s", d.Order, d.Key))
	}
	return out
}

func regionListKeys(vs []catalog.RegionVariation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, fmt.Sprintf("%d %s", v.RegionRank, v.RecipeKey))
	}
	return out
}

// The design bundle has no scores, so it orders exactly as before scores
// existed: dishes in bundle order numbered 1..n, region lists in dish order.
func TestApply_UnscoredDesignBundleKeepsEditorialOrder(t *testing.T) {
	tx := emptyTx(t)
	b := readDesignBundle(t)
	mustApply(t, tx, b, realImport("design"))
	svc := newService(tx)

	dishes, _, err := svc.Dishes(context.Background(), "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	var want []string
	for i, d := range b.Dishes {
		want = append(want, fmt.Sprintf("%d %s", i+1, d.Key))
	}
	assertDeepEqual(t, "dish keys and orders", dishKeyOrders(dishes), want)
	for _, r := range b.Regions {
		assertRegionInDishOrder(t, svc, r.Key, dishes)
	}
}

func assertRegionInDishOrder(t *testing.T, svc *catalog.Service, regionKey string, dishes []catalog.Dish) {
	t.Helper()
	got, _, err := svc.Region(context.Background(), regionKey, "en")
	if err != nil {
		t.Fatalf("Region(%s): %v", regionKey, err)
	}
	var want []string
	for _, d := range dishes {
		i := slices.IndexFunc(d.Variations, func(v catalog.Variation) bool { return v.RegionKey == regionKey })
		if i < 0 {
			continue
		}
		want = append(want, fmt.Sprintf("%d %s", len(want)+1, d.Variations[i].RecipeKey))
	}
	assertDeepEqual(t, regionKey+" list", regionListKeys(got.Variations), want)
}
