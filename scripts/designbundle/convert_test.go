package main

import (
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

func designText(s string) importer.Text {
	return importer.Text{En: s, Es: s + " (es)", Ru: s + " (ru)"}
}

func designRecipeFor(key string) designRecipe {
	return designRecipe{
		RecipeKey:    key,
		BaseServings: 2,
		Time:         importer.Text{En: "25 min", Es: "25 min", Ru: "25 мин"},
		Ingredients:  []importer.Ingredient{{Quantity: 4, Name: designText("Eggs")}},
		Steps:        map[string][]string{"en": {"Beat."}, "es": {"Bate."}, "ru": {"Взбейте."}},
	}
}

// oneVariationDesign has the dish tortilla with the variation
// "Betanzos-style runny" in gal, and the given recipes.
func oneVariationDesign(recipes ...designRecipe) design {
	return design{
		Regions: []importer.Region{{Key: "gal", Name: designText("Galicia")}},
		Dishes: []importer.Dish{{
			Key:        "tortilla",
			Variations: []importer.Variation{{RegionKey: "gal", RecipeKey: "Betanzos-style runny"}},
		}},
		Recipes: recipes,
	}
}

func TestConvert_MatchedRecipeIsKeyedByDishAndRegion(t *testing.T) {
	b, err := convert(oneVariationDesign(designRecipeFor("Betanzos-style runny")))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(b.Recipes) != 1 || b.Recipes[0].RecipeKey != "tortilla-gal" {
		t.Fatalf("got recipes %+v, want one keyed tortilla-gal", b.Recipes)
	}
	if got := b.Recipes[0].TimeMinutes; got != 25 {
		t.Errorf("time: got %d, want 25 from the design recipe", got)
	}
}

// A design recipe no variation names would silently become a placeholder
// recipe; the conversion fails instead and names it.
func TestConvert_RecipeMatchingNoVariationIsAnError(t *testing.T) {
	d := oneVariationDesign(designRecipeFor("Betanzos-style runny"), designRecipeFor("Betanzos style runny"))
	_, err := convert(d)
	if err == nil {
		t.Fatal("got nil, want an error naming the unmatched recipe")
	}
	if !strings.Contains(err.Error(), `Betanzos style runny`) {
		t.Errorf("got %q, want it to name %q", err, "Betanzos style runny")
	}
}
