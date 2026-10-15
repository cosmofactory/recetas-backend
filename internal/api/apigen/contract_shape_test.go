// Package apigen_test pins fields the contract requires on the generated
// models. It lives in its own package so a stale generation fails to compile
// here without breaking the internal/api handler tests.
package apigen_test

import (
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
)

func TestContractShape_GeneratedModelsHaveRequiredFields(t *testing.T) {
	t.Parallel()

	recipe := apigen.Recipe{
		DishName:    "Tortilla de patatas",
		RegionName:  "Galicia",
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

	summary := apigen.VariationSummary{RegionName: "Galicia"}
	if got, want := summary.RegionName, "Galicia"; got != want {
		t.Errorf("got VariationSummary.RegionName %q, want %q", got, want)
	}
}
