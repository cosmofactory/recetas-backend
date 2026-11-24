// Package apigen_test pins fields the contract requires on the generated
// models. It lives in its own package so a stale generation fails to compile
// here without breaking the internal/api handler tests.
package apigen_test

import (
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
)

func strp(s string) *string { return &s }

func intp(n int) *int { return &n }

func TestContractShape_GeneratedModelsHaveRequiredFields(t *testing.T) {
	t.Parallel()

	recipe := apigen.Recipe{
		DishName:    "Tortilla de patatas",
		RegionName:  strp("Galicia"),
		Name:        "Tortilla de Betanzos",
		NameEs:      "Tortilla de Betanzos",
		Description: "Jugosa, casi líquida por dentro.",
	}
	if got, want := recipe.DishName, "Tortilla de patatas"; got != want {
		t.Errorf("got Recipe.DishName %q, want %q", got, want)
	}

	dish := apigen.Dish{Order: 3}
	if got, want := dish.Order, 3; got != want {
		t.Errorf("got Dish.Order %d, want %d", got, want)
	}

	summary := apigen.VariationSummary{RegionName: strp("Galicia"), RegionRank: intp(1)}
	if got, want := *summary.RegionName, "Galicia"; got != want {
		t.Errorf("got VariationSummary.RegionName %q, want %q", got, want)
	}
}

// A Spanish (Spain-wide) variation has no region: regionName and regionRank
// are null and variationSlug names it (BE-9, D-ESP-3, D-ESP-5).
func TestContractShape_SpainWideFieldsAreNullable(t *testing.T) {
	t.Parallel()

	summary := apigen.VariationSummary{RegionKey: "esp", VariationSlug: strp("allioli-de-morter")}
	if summary.RegionName != nil || summary.RegionRank != nil {
		t.Errorf("got VariationSummary regionName %v, regionRank %v, want both nil", summary.RegionName, summary.RegionRank)
	}
	if got, want := *summary.VariationSlug, "allioli-de-morter"; got != want {
		t.Errorf("got VariationSummary.VariationSlug %q, want %q", got, want)
	}

	recipe := apigen.Recipe{RegionKey: "esp", VariationSlug: strp("allioli-de-morter")}
	if recipe.RegionName != nil {
		t.Errorf("got Recipe.RegionName %v, want nil", recipe.RegionName)
	}
	if got, want := *recipe.VariationSlug, "allioli-de-morter"; got != want {
		t.Errorf("got Recipe.VariationSlug %q, want %q", got, want)
	}

	req := apigen.GetSpainWideRecipeRequestObject{DishKey: "allioli", VariationSlug: "allioli-de-morter"}
	if req.DishKey != "allioli" || req.VariationSlug != "allioli-de-morter" {
		t.Errorf("got request %+v, want allioli/allioli-de-morter", req)
	}
}

func TestContractShape_CourseHasBasics(t *testing.T) {
	t.Parallel()

	if !apigen.Basics.Valid() {
		t.Errorf("got Course %q invalid, want valid", apigen.Basics)
	}
}
