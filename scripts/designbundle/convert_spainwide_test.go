package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// Spanish (Spain-wide) variations in the design converter (BE-9, D-ESP-2):
// the converter derives variationSlug once from the Spanish name and keys
// the recipe <dish>-esp-<slug>.

const (
	spainWideKey = "esp"
	maxSlugLen   = 64
	// designSubsetPath is the design content.json cut down to its Spanish
	// samples; the design handoff itself is outside this repo and CI.
	designSubsetPath = "testdata/design_spainwide.json"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// spanishName is a design text whose Spanish differs from the other locales,
// so a slug taken from the wrong locale shows.
func spanishName(es string) importer.Text {
	return importer.Text{En: "English " + es, Es: es, Ru: "Русский " + es}
}

// spainWideDesign has the dish allioli with one Spanish variation per
// Spanish name (its design recipe key is the name), and the given recipes.
func spainWideDesign(names []string, recipes ...designRecipe) design {
	d := design{Regions: []importer.Region{{Key: "gal", Name: designText("Galicia")}}}
	dish := importer.Dish{Key: "allioli"}
	for _, name := range names {
		dish.Variations = append(dish.Variations, importer.Variation{RegionKey: spainWideKey, RecipeKey: name, Name: spanishName(name)})
	}
	d.Dishes = []importer.Dish{dish}
	d.Recipes = recipes
	return d
}

func mustConvert(t *testing.T, d design) importer.Bundle {
	t.Helper()
	b, err := convert(d)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	return b
}

func TestConvert_SpainWideSlugIsTheSpanishNameSlugified(t *testing.T) {
	tests := []struct{ name, want string }{
		{"Allioli de morter", "allioli-de-morter"},
		{"Arroz blanco con tomate", "arroz-blanco-con-tomate"},
		{"Caldo de cocido (pringá)", "caldo-de-cocido-pringa"},
		{"Salsa  verde, a la vasca!", "salsa-verde-a-la-vasca"},
		{"Ñora y AJÍ", "nora-y-aji"},
		{"Fideuà", "fideua"},
		{"Allioli d'Ivars", "allioli-d-ivars"},
		{"  ¿Sofrito básico?  ", "sofrito-basico"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := mustConvert(t, spainWideDesign([]string{tt.name})).Dishes[0].Variations[0]

			if v.VariationSlug != tt.want {
				t.Errorf("variationSlug: got %q, want %q", v.VariationSlug, tt.want)
			}
			if want := "allioli-esp-" + tt.want; v.RecipeKey != want {
				t.Errorf("recipeKey: got %q, want %q", v.RecipeKey, want)
			}
		})
	}
}

func TestConvert_LongSpanishNameGivesASlugOfAtMost64(t *testing.T) {
	name := strings.Repeat("Salsa de almendras tostadas ", 5)

	slug := mustConvert(t, spainWideDesign([]string{name})).Dishes[0].Variations[0].VariationSlug

	if len(slug) > maxSlugLen || !slugPattern.MatchString(slug) {
		t.Errorf("got slug %q (%d characters), want a slug of at most %d", slug, len(slug), maxSlugLen)
	}
}

// A design recipe is matched to its Spanish variation and keyed by dish and
// slug; one without a design recipe gets a placeholder keyed the same way.
func TestConvert_SpainWideRecipesAreKeyedByDishAndSlug(t *testing.T) {
	d := spainWideDesign([]string{"Allioli de morter", "Allioli de codony"}, designRecipeFor("Allioli de morter"))

	b := mustConvert(t, d)

	var got []string
	for _, rc := range b.Recipes {
		got = append(got, rc.RecipeKey)
	}
	want := []string{"allioli-esp-allioli-de-morter", "allioli-esp-allioli-de-codony"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recipe keys: got %q, want %q", got, want)
	}
	if b.Recipes[0].TimeMinutes != 25 {
		t.Errorf("morter time: got %d, want 25 from the design recipe", b.Recipes[0].TimeMinutes)
	}
}

func TestConvert_RegionalVariationHasNoSlug(t *testing.T) {
	v := mustConvert(t, oneVariationDesign(designRecipeFor("Betanzos-style runny"))).Dishes[0].Variations[0]

	if v.VariationSlug != "" || v.RecipeKey != "tortilla-gal" {
		t.Errorf("got slug %q, recipe key %q; want none, tortilla-gal", v.VariationSlug, v.RecipeKey)
	}
}

func TestConvert_SpainWideBundleValidates(t *testing.T) {
	d := spainWideDesign([]string{"Allioli de morter", "Allioli de codony"})
	d.Regions[0] = importer.Region{
		Key: "gal", TileColor: "#9A9A5E", Name: designText("Galicia"), MapLabel: designText("Galicia"), Description: designText("Sea."),
	}
	d.Dishes[0].Name, d.Dishes[0].Description = designText("Allioli"), designText("Garlic and oil.")
	d.Dishes[0].Course, d.Dishes[0].Season = "basics", "all"
	for i := range d.Dishes[0].Variations {
		d.Dishes[0].Variations[i].Description = designText("One line.")
	}

	if err := importer.Validate(mustConvert(t, d)); err != nil {
		t.Errorf("Validate: got %v, want nil", err)
	}
}

// The design-seed round trip: the design content, spainWide block included
// (design data only, D-ESP-3), converts to a valid bundle with both Spanish
// samples.
func TestRun_DesignContentConvertsWithSpainWideSamples(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.json")

	if err := run(designSubsetPath, out); err != nil {
		t.Fatalf("run: %v", err)
	}

	b := readBundle(t, out)
	assertVariations(t, b, "allioli", []string{
		"esp/allioli-de-morter/allioli-esp-allioli-de-morter",
		"esp/allioli-de-codony/allioli-esp-allioli-de-codony",
	})
	assertVariations(t, b, "arrozblanco", []string{
		"esp/arroz-blanco-con-tomate/arrozblanco-esp-arroz-blanco-con-tomate",
	})
}

func TestRun_BundleCarriesNoSpainWideNames(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.json")
	if err := run(designSubsetPath, out); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	if strings.Contains(string(data), "spainWide") {
		t.Error("got spainWide in the bundle, want the design-only block dropped")
	}
}

func readBundle(t *testing.T, path string) importer.Bundle {
	t.Helper()
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	b, err := importer.Parse(f)
	if err != nil {
		t.Fatalf("Parse bundle: %v", err)
	}
	return b
}

// assertVariations compares a dish's variations as regionKey/slug/recipeKey,
// in bundle order.
func assertVariations(t *testing.T, b importer.Bundle, dishKey string, want []string) {
	t.Helper()
	var got []string
	for _, d := range b.Dishes {
		if d.Key != dishKey {
			continue
		}
		for _, v := range d.Variations {
			got = append(got, v.RegionKey+"/"+v.VariationSlug+"/"+v.RecipeKey)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s variations: got %q, want %q", dishKey, got, want)
	}
}
