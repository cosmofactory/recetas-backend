package catalog_test

import (
	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
	"github.com/cosmofactory/recetas-backend/internal/catalog"
)

// A Spanish (Spain-wide) dish beside the catalogtest fixture, as a repository
// serves it (BE-9): allioli has a regional variation in ast, then two Spanish
// variations in bundle order. A Spanish variation has the region key esp, no
// region name, no region rank and a slug (D-ESP-3, D-ESP-5). Texts are the
// same in every locale.

const (
	spainWideKey = "esp"
	allioliKey   = "allioli"
	morterSlug   = "allioli-de-morter"
	codonySlug   = "allioli-de-codony"
	allioliAst   = "allioli-asturiano"
	// allioliAstRank follows tortilla-chorizo and fabada-asturiana in ast.
	allioliAstRank = 3
)

func spainWideRecipeKey(slug string) string { return allioliKey + "-" + spainWideKey + "-" + slug }

func spainWidePath(dishKey, slug string) string {
	return "/v1/dishes/" + dishKey + "/variations/esp/" + slug + "/recipe"
}

// spainWideDishes is the fixture's dishes in locale l, then allioli.
func spainWideDishes(l catalog.Locale) []catalog.Dish {
	dishes := domainDishes(l)
	return append(dishes, allioliDish(len(dishes)+1))
}

func allioliDish(order int) catalog.Dish {
	return catalog.Dish{
		Key: allioliKey, Order: order, Name: "Allioli", NameEs: "Allioli", Description: "Garlic and oil.",
		Flags: catalog.Flags{Vegetarian: true, Quick: true}, Course: "basics", Season: "all",
		Variations: []catalog.Variation{
			{
				RegionKey: "ast", RegionName: "Asturias", RecipeKey: allioliAst, Name: "Allioli asturiano",
				NameEs: "Allioli asturiano", Description: "With cider vinegar.", RegionRank: allioliAstRank,
			},
			spainWideVariation(morterSlug, "Allioli de morter"),
			spainWideVariation(codonySlug, "Allioli de codony"),
		},
	}
}

func spainWideVariation(slug, name string) catalog.Variation {
	return catalog.Variation{
		RegionKey: spainWideKey, VariationSlug: slug, RecipeKey: spainWideRecipeKey(slug),
		Name: name, NameEs: name, Description: "One line about " + name + ".",
	}
}

// spainWideRecipes are allioli's Spanish recipes by "<dish>/<slug>".
func spainWideRecipes() map[string]catalog.Recipe {
	out := map[string]catalog.Recipe{}
	for _, v := range allioliDish(1).Variations[1:] {
		out[allioliKey+"/"+v.VariationSlug] = spainWideRecipe(v)
	}
	return out
}

func spainWideRecipe(v catalog.Variation) catalog.Recipe {
	return catalog.Recipe{
		DishKey: allioliKey, DishName: "Allioli", RegionKey: spainWideKey, VariationSlug: v.VariationSlug,
		RecipeKey: v.RecipeKey, Name: v.Name, NameEs: v.NameEs, Description: v.Description,
		TimeMinutes: 15, BaseServings: 4,
		Ingredients: []catalog.Ingredient{{Quantity: 4, Name: "Garlic cloves"}},
		Steps:       []string{"Pound the garlic.", "Add the oil a thread at a time."},
	}
}

// apiAllioli is allioli as the contract sends it: the Spanish variations
// with null regionName and regionRank and their variationSlug, the regional
// one the other way round.
func apiAllioli(order int) apigen.Dish {
	return apigen.Dish{
		Key: allioliKey, Order: order, Name: "Allioli", NameEs: "Allioli", Description: "Garlic and oil.",
		Flags:  apigen.Flags{Vegetarian: true, Quick: true},
		Course: apigen.Basics, Season: apigen.All,
		Variations: []apigen.VariationSummary{
			{
				RegionKey: "ast", RegionName: ptr("Asturias"), RegionRank: ptr(allioliAstRank), RecipeKey: allioliAst,
				Name: "Allioli asturiano", NameEs: "Allioli asturiano", Description: "With cider vinegar.",
			},
			apiSpainWideVariation(morterSlug, "Allioli de morter"),
			apiSpainWideVariation(codonySlug, "Allioli de codony"),
		},
	}
}

func apiSpainWideVariation(slug, name string) apigen.VariationSummary {
	return apigen.VariationSummary{
		RegionKey: spainWideKey, VariationSlug: ptr(slug), RecipeKey: spainWideRecipeKey(slug),
		Name: name, NameEs: name, Description: "One line about " + name + ".",
	}
}

func apiSpainWideRecipe(slug, name string) apigen.Recipe {
	return apigen.Recipe{
		DishKey: allioliKey, DishName: "Allioli", RegionKey: spainWideKey, VariationSlug: ptr(slug),
		RecipeKey: spainWideRecipeKey(slug), Name: name, NameEs: name, Description: "One line about " + name + ".",
		TimeMinutes: 15, BaseServings: 4,
		Ingredients: []apigen.Ingredient{{Quantity: 4, Name: "Garlic cloves"}},
		Steps:       []string{"Pound the garlic.", "Add the oil a thread at a time."},
		Tips:        []string{},
	}
}
