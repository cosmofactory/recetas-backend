package importer_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// allioliAt is the pointer of the allioli dish in spainWideBundle.
const allioliAt = "/dishes/2"

func TestValidate_AcceptsSpainWideVariations(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
	}{
		{"spanish variations around a regional one", func(*importer.Bundle) {}},
		{"dish with only spanish variations", func(b *importer.Bundle) {
			b.Dishes[2].Variations = []importer.Variation{b.Dishes[2].Variations[0], b.Dishes[2].Variations[2]}
			b.Recipes = []importer.Recipe{b.Recipes[0], b.Recipes[1], b.Recipes[2], b.Recipes[3], b.Recipes[4], b.Recipes[6]}
		}},
		{"dish with one spanish variation", func(b *importer.Bundle) {
			b.Dishes[2].Variations = b.Dishes[2].Variations[:1]
			b.Recipes = b.Recipes[:5]
		}},
		{"same slug in two dishes", func(b *importer.Bundle) {
			b.Dishes[0].Variations = append(b.Dishes[0].Variations, spainWideVariation("tortilla", morterSlug))
			b.Recipes = append(b.Recipes, recipe(spainWideRecipeKey("tortilla", morterSlug)))
		}},
		{"slug of 64 characters", func(b *importer.Bundle) { b.Dishes[2].Variations[0].VariationSlug = strings.Repeat("a", 64) }},
		{"course basics on any dish", func(b *importer.Bundle) { b.Dishes[1].Course = "basics" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := spainWideBundle()
			tt.edit(&b)
			if err := importer.Validate(b); err != nil {
				t.Errorf("got %v, want nil", err)
			}
		})
	}
}

func TestValidate_RejectsSpainWideMistakesWithPointers(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
		want []string // sorted
	}{
		{"spanish variation without a slug", func(b *importer.Bundle) { b.Dishes[2].Variations[0].VariationSlug = "" },
			[]string{allioliAt + "/variations/0/variationSlug"}},
		{"slug on a regional variation", func(b *importer.Bundle) { b.Dishes[2].Variations[1].VariationSlug = "allioli-asturiano" },
			[]string{allioliAt + "/variations/1/variationSlug"}},
		{"duplicate slug in a dish", func(b *importer.Bundle) { b.Dishes[2].Variations[2].VariationSlug = morterSlug },
			[]string{allioliAt + "/variations/2/variationSlug"}},
		{"slug with spaces and capitals", func(b *importer.Bundle) { b.Dishes[2].Variations[0].VariationSlug = "Allioli de morter" },
			[]string{allioliAt + "/variations/0/variationSlug"}},
		{"slug with an accent", func(b *importer.Bundle) { b.Dishes[2].Variations[0].VariationSlug = "allioli-de-codoñy" },
			[]string{allioliAt + "/variations/0/variationSlug"}},
		{"slug of 65 characters", func(b *importer.Bundle) { b.Dishes[2].Variations[0].VariationSlug = strings.Repeat("a", 65) },
			[]string{allioliAt + "/variations/0/variationSlug"}},
		{"region keyed esp", func(b *importer.Bundle) { b.Regions = append(b.Regions, region(spainWideKey, "#000000")) },
			[]string{"/regions/3/key"}},
		{"unknown course", func(b *importer.Bundle) { b.Dishes[2].Course = "sauces" },
			[]string{allioliAt + "/course"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := spainWideBundle()
			tt.edit(&b)
			got := sortedPointers(t, validationProblems(t, importer.Validate(b)))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got pointers %q, want %q", got, tt.want)
			}
		})
	}
}
