package importer_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// Removals need an explicit allowance (INF-7, D-INF7-3). A removal is a
// public path, without a locale, that the stored catalog serves and the
// bundle no longer does: /regions/<region>, /dishes/<dish>,
// /dishes/<dish>/<region> or /dishes/<dish>/esp/<slug>. A recipe goes with
// its variation, so it has no path of its own. All cases start from
// spainWideBundle:
//
//	regions gal, ast, mad
//	tortilla [gal, ast], cocido [mad, ast], allioli [esp morter, ast, esp codony]

type removalCase struct {
	name    string
	edit    func(t *testing.T, b *importer.Bundle)
	removed []string // sorted
}

func removalCases() []removalCase {
	return []removalCase{
		{"regional variation", func(t *testing.T, b *importer.Bundle) {
			removeVariation(t, b, "tortilla", "ast")
		}, []string{"/dishes/tortilla/ast"}},
		{"Spanish variation", func(_ *testing.T, b *importer.Bundle) {
			b.Dishes[2].Variations = b.Dishes[2].Variations[:2]
			b.Recipes = b.Recipes[:6]
		}, []string{"/dishes/allioli/esp/allioli-de-codony"}},
		{"dish with its variations", func(_ *testing.T, b *importer.Bundle) {
			removeDish(b, "cocido")
		}, []string{"/dishes/cocido", "/dishes/cocido/ast", "/dishes/cocido/mad"}},
		{"region with its variations", func(_ *testing.T, b *importer.Bundle) {
			removeRegion(b, "ast")
		}, []string{"/dishes/allioli/ast", "/dishes/cocido/ast", "/dishes/tortilla/ast", "/regions/ast"}},
		{"dish key renamed", func(_ *testing.T, b *importer.Bundle) {
			b.Dishes[1].Key = "olla"
		}, []string{"/dishes/cocido", "/dishes/cocido/ast", "/dishes/cocido/mad"}},
		{"region key renamed", func(_ *testing.T, b *importer.Bundle) {
			b.Regions[2].Key = "madrid"
			b.Dishes[1].Variations[0].RegionKey = "madrid"
		}, []string{"/dishes/cocido/mad", "/regions/mad"}},
		{"Spanish slug renamed", func(_ *testing.T, b *importer.Bundle) {
			b.Dishes[2].Variations[0].VariationSlug = "allioli-a-mano"
		}, []string{"/dishes/allioli/esp/" + morterSlug}},
		{"variation moved to another region", func(_ *testing.T, b *importer.Bundle) {
			b.Dishes[0].Variations[1].RegionKey = "mad"
		}, []string{"/dishes/tortilla/ast"}},
		{"regional variation made Spanish", func(_ *testing.T, b *importer.Bundle) {
			b.Dishes[2].Variations[1].RegionKey = spainWideKey
			b.Dishes[2].Variations[1].VariationSlug = "allioli-asturiano"
		}, []string{"/dishes/allioli/ast"}},
	}
}

type importMode struct {
	name string
	opts importer.Options
}

var importModes = []importMode{
	{"import", realImport("removals")},
	{"dry run", dryRun("removals")},
}

func TestApply_RefusesUnlistedRemovals(t *testing.T) {
	for _, tt := range removalCases() {
		for _, mode := range importModes {
			t.Run(tt.name+"/"+mode.name, func(t *testing.T) {
				tx, _ := importedSpainWide(t)
				before := readState(t, tx)
				b := editedSpainWide(t, tt.edit)

				_, err := importer.Apply(context.Background(), tx, b, mode.opts)

				assertUnlisted(t, err, tt.removed)
				assertStateUnchanged(t, tx, before)
			})
		}
	}
}

// Listing a path that isn't removed is not an error: the allowance file may
// outlive the release it was written for.
func TestApply_ListedRemovalsGoThrough(t *testing.T) {
	for _, tt := range removalCases() {
		t.Run(tt.name, func(t *testing.T) {
			tx, _ := importedSpainWide(t)
			b := editedSpainWide(t, tt.edit)
			allowed := slices.Concat(tt.removed, []string{"/dishes/paella", "/regions/cat"})

			s := mustApply(t, tx, b, allowRemovals(realImport("listed"), allowed...))

			assertDeepEqual(t, "removed paths", s.Removed, tt.removed)
			assertCatalogMatches(t, tx, b)
		})
	}
}

func TestApply_DryRunReportsListedRemovals(t *testing.T) {
	for _, tt := range removalCases() {
		t.Run(tt.name, func(t *testing.T) {
			tx, _ := importedSpainWide(t)
			before := readState(t, tx)
			b := editedSpainWide(t, tt.edit)

			s := mustApply(t, tx, b, allowRemovals(dryRun("listed"), tt.removed...))

			assertDeepEqual(t, "removed paths", s.Removed, tt.removed)
			assertStateUnchanged(t, tx, before)
		})
	}
}

// Only the unlisted paths are reported, and nothing is written: an import
// with any unlisted removal is refused as a whole. Paths are matched exactly,
// so a locale prefix or another variation's path doesn't count.
func TestApply_PartlyListedRemovalsAreRefused(t *testing.T) {
	tests := []struct {
		name     string
		edit     func(t *testing.T, b *importer.Bundle)
		allowed  []string
		unlisted []string
	}{
		{"dish listed without its variations", func(_ *testing.T, b *importer.Bundle) {
			removeDish(b, "cocido")
		}, []string{"/dishes/cocido"}, []string{"/dishes/cocido/ast", "/dishes/cocido/mad"}},
		{"region listed without its variations", func(_ *testing.T, b *importer.Bundle) {
			removeRegion(b, "ast")
		}, []string{"/regions/ast"}, []string{"/dishes/allioli/ast", "/dishes/cocido/ast", "/dishes/tortilla/ast"}},
		{"variations listed without their region", func(_ *testing.T, b *importer.Bundle) {
			removeRegion(b, "ast")
		}, []string{"/dishes/allioli/ast", "/dishes/cocido/ast", "/dishes/tortilla/ast"}, []string{"/regions/ast"}},
		{"path with a locale", func(t *testing.T, b *importer.Bundle) {
			removeVariation(t, b, "tortilla", "ast")
		}, []string{"/en/dishes/tortilla/ast"}, []string{"/dishes/tortilla/ast"}},
		{"another dish's variation", func(t *testing.T, b *importer.Bundle) {
			removeVariation(t, b, "tortilla", "ast")
		}, []string{"/dishes/cocido/ast"}, []string{"/dishes/tortilla/ast"}},
		{"old slug listed for a renamed Spanish variation's new slug", func(_ *testing.T, b *importer.Bundle) {
			b.Dishes[2].Variations[0].VariationSlug = "allioli-a-mano"
		}, []string{"/dishes/allioli/esp/allioli-a-mano"}, []string{"/dishes/allioli/esp/" + morterSlug}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, _ := importedSpainWide(t)
			before := readState(t, tx)
			b := editedSpainWide(t, tt.edit)

			_, err := importer.Apply(context.Background(), tx, b, allowRemovals(realImport("partly listed"), tt.allowed...))

			assertUnlisted(t, err, tt.unlisted)
			assertStateUnchanged(t, tx, before)
		})
	}
}

// An import that removes no public path needs no allowance, even when it
// deletes and recreates rows: a recipe key renamed at the same dish and
// region keeps its URL.
func TestApply_NoRemovalsNeedNoAllowance(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
	}{
		{"identical", func(*importer.Bundle) {}},
		{"update", func(b *importer.Bundle) { b.Regions[0].Name.En = "Galiza" }},
		{"recipe key renamed at the same dish and region", func(b *importer.Bundle) {
			b.Dishes[1].Variations[0].RecipeKey = "cocido-de-madrid"
			b.Recipes[2].RecipeKey = "cocido-de-madrid"
		}},
		{"Spanish variations reordered", func(b *importer.Bundle) {
			vs := b.Dishes[2].Variations
			vs[0], vs[2] = vs[2], vs[0]
		}},
		{"variation added", func(b *importer.Bundle) {
			b.Dishes[0].Variations = append(b.Dishes[0].Variations, variation("mad", "tortilla-madrid"))
			b.Recipes = append(b.Recipes, recipe("tortilla-madrid"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, _ := importedSpainWide(t)
			b := spainWideBundle()
			tt.edit(&b)

			s := mustApply(t, tx, b, realImport("no removals"))

			assertNoneRemoved(t, s)
			assertCatalogMatches(t, tx, b)
		})
	}
}

func TestApply_FirstImportNeedsNoAllowance(t *testing.T) {
	tx := emptyTx(t)

	s := mustApply(t, tx, readDesignBundle(t), realImport("first"))

	assertNoneRemoved(t, s)
}

// editedSpainWide is spainWideBundle after edit.
func editedSpainWide(t *testing.T, edit func(t *testing.T, b *importer.Bundle)) importer.Bundle {
	t.Helper()
	b := spainWideBundle()
	edit(t, &b)
	return b
}

// removeDish drops the dish and its variations' recipes from b.
func removeDish(b *importer.Bundle, dishKey string) {
	i := slices.IndexFunc(b.Dishes, func(d importer.Dish) bool { return d.Key == dishKey })
	gone := map[string]bool{}
	for _, v := range b.Dishes[i].Variations {
		gone[v.RecipeKey] = true
	}
	b.Dishes = slices.Delete(b.Dishes, i, i+1)
	b.Recipes = slices.DeleteFunc(b.Recipes, func(rc importer.Recipe) bool { return gone[rc.RecipeKey] })
}

// assertUnlisted requires err to refuse exactly the unlisted paths, sorted,
// and to name each of them in its message.
func assertUnlisted(t *testing.T, err error, unlisted []string) {
	t.Helper()
	var rerr *importer.UnlistedRemovalsError
	if !errors.As(err, &rerr) {
		t.Fatalf("got error %v (%T), want *importer.UnlistedRemovalsError", err, err)
	}
	assertDeepEqual(t, "unlisted paths", rerr.Paths, unlisted)
	for _, p := range unlisted {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q: want it to name %s", err, p)
		}
	}
}

func assertNoneRemoved(t *testing.T, s importer.Summary) {
	t.Helper()
	if len(s.Removed) != 0 {
		t.Errorf("removed paths: got %v, want none", s.Removed)
	}
}
