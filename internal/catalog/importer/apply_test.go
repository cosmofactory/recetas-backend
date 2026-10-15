package importer_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

var designCreated = map[string]importer.Counts{
	"regions":    {Created: 14},
	"dishes":     {Created: 7},
	"variations": {Created: 54},
	"recipes":    {Created: 54},
}

func TestApply_DesignBundleSummary(t *testing.T) {
	tx := emptyTx(t)
	s := mustApply(t, tx, readDesignBundle(t), realImport("design"))
	if s.NoChanges {
		t.Error("got NoChanges, want changes")
	}
	assertChanges(t, s, designCreated)
	version, err := catalog.NewPGRepository(tx).ContentVersion(context.Background())
	if err != nil {
		t.Fatalf("ContentVersion: %v", err)
	}
	if s.Version == 0 || s.Version != version {
		t.Errorf("got summary version %d, want the new content version %d", s.Version, version)
	}
}

func TestApply_RecordsContentVersion(t *testing.T) {
	tx := emptyTx(t)
	b := readDesignBundle(t)
	s := mustApply(t, tx, b, realImport("pipeline run 42"))
	var source, checksum string
	err := tx.QueryRow(context.Background(), "SELECT source, checksum FROM content_versions WHERE id = $1", s.Version).Scan(&source, &checksum)
	if err != nil {
		t.Fatalf("read content version %d: %v", s.Version, err)
	}
	if source != "pipeline run 42" {
		t.Errorf("source: got %q, want %q", source, "pipeline run 42")
	}
	if want := importer.Checksum(b); checksum != want {
		t.Errorf("checksum: got %q, want %q", checksum, want)
	}
}

func TestApply_DesignBundleReadsBackInEveryLocale(t *testing.T) {
	tx := emptyTx(t)
	b := readDesignBundle(t)
	mustApply(t, tx, b, realImport("design"))
	assertCatalogMatches(t, tx, b)
}

func TestApply_DesignContent(t *testing.T) {
	tx := emptyTx(t)
	mustApply(t, tx, readDesignBundle(t), realImport("design"))
	svc := newService(tx)
	ctx := context.Background()

	for l, want := range map[catalog.Locale]string{"en": "Galicia", "es": "Galicia", "ru": "Галисия"} {
		regions, _, err := svc.Regions(ctx, l)
		if err != nil {
			t.Fatalf("Regions(%s): %v", l, err)
		}
		requireLen(t, "regions "+string(l), len(regions), 14)
		if regions[0].Key != "gal" || regions[0].Name != want {
			t.Errorf("%s first region: got %s %q, want gal %q", l, regions[0].Key, regions[0].Name, want)
		}
	}
	dishes, _, err := svc.Dishes(ctx, "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	requireLen(t, "dishes", len(dishes), 7)

	betanzos := mustRecipe(t, svc, pairKey{"tortilla", "gal"}, "en")
	if betanzos.TimeMinutes != 25 || betanzos.BaseServings != 2 || betanzos.TimeNote != nil {
		t.Errorf("betanzos: got %d min, %d servings, note %v; want 25, 2, nil", betanzos.TimeMinutes, betanzos.BaseServings, betanzos.TimeNote)
	}
	requireLen(t, "betanzos ingredients", len(betanzos.Ingredients), 4)
	if eggs := betanzos.Ingredients[0]; eggs.Name != "Eggs" || eggs.Quantity != 4 || eggs.Unit != nil {
		t.Errorf("betanzos first ingredient: got %+v, want 4 Eggs, no unit", eggs)
	}

	fabada := mustRecipe(t, svc, pairKey{"cocido", "ast"}, "ru")
	if fabada.TimeMinutes != 180 || fabada.TimeNote == nil || *fabada.TimeNote != "+ замачивание" {
		t.Errorf("fabada: got %d min, note %v; want 180, %q", fabada.TimeMinutes, fabada.TimeNote, "+ замачивание")
	}

	arrosNegre := mustRecipe(t, svc, pairKey{"paella", "cat"}, "en")
	if len(arrosNegre.Steps) == 0 || !strings.Contains(strings.ToLower(strings.Join(arrosNegre.Steps, " ")), "placeholder") {
		t.Errorf("paella/cat: got steps %q, want a placeholder step", arrosNegre.Steps)
	}
}

func TestApply_IdenticalBundleMakesNoChanges(t *testing.T) {
	tx := emptyTx(t)
	first := mustApply(t, tx, readDesignBundle(t), realImport("first"))
	before := readState(t, tx)

	again := mustApply(t, tx, readDesignBundle(t), realImport("second"))
	if !again.NoChanges {
		t.Error("got changes, want NoChanges")
	}
	assertChanges(t, again, nil)
	if again.Version != first.Version {
		t.Errorf("got version %d, want the unchanged %d", again.Version, first.Version)
	}
	assertStateUnchanged(t, tx, before)
}

func TestApply_RemovedVariationDeletesOnlyItsTree(t *testing.T) {
	tx := emptyTx(t)
	mustApply(t, tx, readDesignBundle(t), realImport("full"))
	before := readState(t, tx)

	b := readDesignBundle(t)
	removed := removeVariation(t, &b, "tortilla", "gal")
	s := mustApply(t, tx, b, realImport("without tortilla/gal"))

	assertChanges(t, s, map[string]importer.Counts{"variations": {Deleted: 1}, "recipes": {Deleted: 1}})
	wantCounts := subtractRows(before.counts, recipeTreeRows(removed))
	wantCounts["content_versions"]++
	assertDeepEqual(t, "table counts", tableCounts(t, tx), wantCounts)
	wantIDs := before.ids
	delete(wantIDs, "variation:"+removed.RecipeKey)
	delete(wantIDs, "recipe:"+removed.RecipeKey)
	assertDeepEqual(t, "row ids", rowIDs(t, tx), wantIDs)

	_, _, err := newService(tx).Recipe(context.Background(), "tortilla", "gal", "en")
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("removed recipe: got %v, want ErrNotFound", err)
	}
	assertCatalogMatches(t, tx, b)
}

func TestApply_ChangeUpdatesOnlyThatRow(t *testing.T) {
	tests := []struct {
		name   string
		entity string
		edit   func(b *importer.Bundle)
	}{
		{"region name", "regions", func(b *importer.Bundle) { b.Regions[3].Name.Ru = "Каталония!" }},
		{"region regional dish", "regions", func(b *importer.Bundle) { b.Regions[0].RegionalDishes[2] = "Caldo gallego" }},
		{"region tile color", "regions", func(b *importer.Bundle) { b.Regions[1].TileColor = "#123456" }},
		{"dish name", "dishes", func(b *importer.Bundle) { b.Dishes[2].Name.En = "Cocido and stews" }},
		{"dish flag", "dishes", func(b *importer.Bundle) { b.Dishes[0].Flags.Pork = true }},
		{"variation description", "variations", func(b *importer.Bundle) { b.Dishes[1].Variations[0].Description.Es = "Otra descripción" }},
		{"recipe step", "recipes", func(b *importer.Bundle) { b.Recipes[0].Steps[1].Ru = "Новый шаг." }},
		{"recipe ingredient name", "recipes", func(b *importer.Bundle) { b.Recipes[0].Ingredients[0].Name.Es = "Huevos camperos" }},
		{"recipe time note added", "recipes", func(b *importer.Bundle) {
			b.Recipes[0].TimeNote = &importer.Text{En: "+ resting", Es: "+ reposo", Ru: "+ отдых"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := emptyTx(t)
			mustApply(t, tx, readDesignBundle(t), realImport("base"))
			idsBefore := rowIDs(t, tx)

			b := readDesignBundle(t)
			tt.edit(&b)
			s := mustApply(t, tx, b, realImport("edit"))

			assertChanges(t, s, map[string]importer.Counts{tt.entity: {Updated: 1}})
			assertDeepEqual(t, "row ids", rowIDs(t, tx), idsBefore)
			assertCatalogMatches(t, tx, b)
		})
	}
}

func TestApply_ReordersRegionsAndDishes(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
		want map[string]importer.Counts
	}{
		{"swap two of each", func(b *importer.Bundle) {
			b.Regions[0], b.Regions[1] = b.Regions[1], b.Regions[0]
			b.Dishes[0], b.Dishes[1] = b.Dishes[1], b.Dishes[0]
		}, map[string]importer.Counts{"regions": {Updated: 2}, "dishes": {Updated: 2}}},
		{"move the last to the front", func(b *importer.Bundle) {
			b.Regions = append(b.Regions[len(b.Regions)-1:], b.Regions[:len(b.Regions)-1]...)
			b.Dishes = append(b.Dishes[len(b.Dishes)-1:], b.Dishes[:len(b.Dishes)-1]...)
		}, map[string]importer.Counts{"regions": {Updated: 14}, "dishes": {Updated: 7}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := emptyTx(t)
			mustApply(t, tx, readDesignBundle(t), realImport("base"))

			b := readDesignBundle(t)
			tt.edit(&b)
			s := mustApply(t, tx, b, realImport("reordered"))

			assertChanges(t, s, tt.want)
			assertCatalogMatches(t, tx, b)
		})
	}
}

func TestApply_DryRunReportsWithoutWriting(t *testing.T) {
	tests := []struct {
		name string
		base bool // import the design bundle first
		edit func(t *testing.T, b *importer.Bundle)
	}{
		{"first import", false, func(*testing.T, *importer.Bundle) {}},
		{"update", true, func(_ *testing.T, b *importer.Bundle) { b.Regions[0].Name.En = "Galiza" }},
		{"removal", true, func(t *testing.T, b *importer.Bundle) { removeVariation(t, b, "cocido", "ast") }},
		{"identical", true, func(*testing.T, *importer.Bundle) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := emptyTx(t)
			if tt.base {
				mustApply(t, tx, readDesignBundle(t), realImport("base"))
			}
			before := readState(t, tx)
			b := readDesignBundle(t)
			tt.edit(t, &b)

			dry := mustApply(t, tx, b, dryRun("dry"))
			assertStateUnchanged(t, tx, before)

			actual := mustApply(t, tx, b, realImport("real"))
			if dry.NoChanges != actual.NoChanges {
				t.Errorf("NoChanges: got %v on the dry run, want %v as on the real one", dry.NoChanges, actual.NoChanges)
			}
			assertChanges(t, dry, actual.Changes)
		})
	}
}

// Validate can't see a NUL byte, which Postgres rejects in text: the import
// fails on the last recipe, after everything else has been written.
func TestApply_FailureLeavesDatabaseUnchanged(t *testing.T) {
	for _, base := range []bool{false, true} {
		t.Run(map[bool]string{false: "first import", true: "update"}[base], func(t *testing.T) {
			tx := emptyTx(t)
			if base {
				mustApply(t, tx, readDesignBundle(t), realImport("base"))
			}
			before := readState(t, tx)

			b := readDesignBundle(t)
			b.Regions[0].Name.En = "Galiza"
			poisonLastStep(&b)
			if err := importer.Validate(b); err != nil {
				t.Fatalf("the trigger must pass Validate, got %v", err)
			}
			if _, err := importer.Apply(context.Background(), tx, b, realImport("broken")); err == nil {
				t.Fatal("got nil, want a database error")
			}
			assertStateUnchanged(t, tx, before)
		})
	}
}

func TestApply_BeginErrorIsReturned(t *testing.T) {
	boom := errors.New("connection refused")
	_, err := importer.Apply(context.Background(), failingBeginner{err: boom}, validBundle(), realImport("x"))
	if !errors.Is(err, boom) {
		t.Errorf("got %v, want it to wrap %v", err, boom)
	}
}

type failingBeginner struct{ err error }

func (f failingBeginner) Begin(context.Context) (pgx.Tx, error) { return nil, f.err }

func poisonLastStep(b *importer.Bundle) {
	rc := &b.Recipes[len(b.Recipes)-1]
	rc.Steps[len(rc.Steps)-1].Ru += "\x00"
}

// removeVariation drops dishKey's variation in regionKey and its recipe from
// b, and returns the recipe.
func removeVariation(t *testing.T, b *importer.Bundle, dishKey, regionKey string) importer.Recipe {
	t.Helper()
	removed := recipeOf(t, *b, dishKey, regionKey)
	i := slices.IndexFunc(b.Dishes, func(d importer.Dish) bool { return d.Key == dishKey })
	b.Dishes[i].Variations = slices.DeleteFunc(b.Dishes[i].Variations, func(v importer.Variation) bool {
		return v.RegionKey == regionKey
	})
	b.Recipes = slices.DeleteFunc(b.Recipes, func(rc importer.Recipe) bool { return rc.RecipeKey == removed.RecipeKey })
	return removed
}

func subtractRows(counts, rows map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(counts))
	for table, n := range counts {
		out[table] = n - rows[table]
	}
	return out
}

// pairKey names a variation by its dish and region.
type pairKey struct{ dish, region string }

func mustRecipe(t *testing.T, svc *catalog.Service, p pairKey, l catalog.Locale) catalog.Recipe {
	t.Helper()
	rc, _, err := svc.Recipe(context.Background(), p.dish, p.region, l)
	if err != nil {
		t.Fatalf("Recipe(%s/%s, %s): %v", p.dish, p.region, l, err)
	}
	return rc
}

func requireLen(t *testing.T, what string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("got %d %s, want %d", got, what, want)
	}
}
