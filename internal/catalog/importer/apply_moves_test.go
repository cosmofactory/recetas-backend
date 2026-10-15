package importer_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

var replacedOne = map[string]importer.Counts{
	"variations": {Created: 1, Deleted: 1},
	"recipes":    {Created: 1, Deleted: 1},
}

// A variation whose (dish, region) or recipe key changes is a new variation:
// the old one and its recipe are deleted, the new one created.
func TestApply_ReplacedVariation(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
		gone pairKey
	}{
		{"moved to another region, same recipe key", func(b *importer.Bundle) {
			b.Dishes[0].Variations[1].RegionKey = "mad"
		}, pairKey{"tortilla", "ast"}},
		{"recipe key renamed at the same dish and region", func(b *importer.Bundle) {
			b.Dishes[1].Variations[0].RecipeKey = "cocido-de-madrid"
			b.Recipes[2].RecipeKey = "cocido-de-madrid"
		}, pairKey{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := emptyTx(t)
			mustApply(t, tx, validBundle(), realImport("base"))

			b := validBundle()
			tt.edit(&b)
			s := mustApply(t, tx, b, realImport("replaced"))

			assertChanges(t, s, replacedOne)
			assertCatalogMatches(t, tx, b)
			if tt.gone != (pairKey{}) {
				assertNoRecipe(t, newService(tx), tt.gone)
			}
		})
	}
}

// Removing a region removes its variations and their recipes, and shifts the
// order of every later region, which counts as an update.
func TestApply_RemovedRegionDeletesItsVariations(t *testing.T) {
	tx := emptyTx(t)
	mustApply(t, tx, validBundle(), realImport("base"))
	idsBefore := rowIDs(t, tx)

	b := validBundle()
	removed := removeRegion(&b, "ast")
	s := mustApply(t, tx, b, realImport("without ast"))

	assertChanges(t, s, map[string]importer.Counts{
		"regions":    {Updated: 1, Deleted: 1},
		"variations": {Deleted: 2},
		"recipes":    {Deleted: 2},
	})
	delete(idsBefore, "region:ast")
	for _, key := range removed {
		delete(idsBefore, "variation:"+key)
		delete(idsBefore, "recipe:"+key)
	}
	assertDeepEqual(t, "row ids", rowIDs(t, tx), idsBefore)
	assertCatalogMatches(t, tx, b)
}

// removeRegion drops the region, its variations and their recipes from b, and
// returns the removed recipe keys.
func removeRegion(b *importer.Bundle, regionKey string) []string {
	var removed []string
	b.Regions = slices.DeleteFunc(b.Regions, func(r importer.Region) bool { return r.Key == regionKey })
	for i := range b.Dishes {
		b.Dishes[i].Variations = slices.DeleteFunc(b.Dishes[i].Variations, func(v importer.Variation) bool {
			if v.RegionKey == regionKey {
				removed = append(removed, v.RecipeKey)
			}
			return v.RegionKey == regionKey
		})
	}
	b.Recipes = slices.DeleteFunc(b.Recipes, func(rc importer.Recipe) bool { return slices.Contains(removed, rc.RecipeKey) })
	return removed
}

func assertNoRecipe(t *testing.T, svc *catalog.Service, p pairKey) {
	t.Helper()
	_, _, err := svc.Recipe(context.Background(), p.dish, p.region, "en")
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("Recipe(%s/%s): got %v, want ErrNotFound", p.dish, p.region, err)
	}
}
