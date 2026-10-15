package catalog_test

import (
	"slices"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogtest"
)

// Expected HTTP bodies in the contract's generated types, built straight from
// the fixture (not from the domain expectations) so the handler mapping is
// checked independently.

func apiPhotos(p catalogtest.Photos) apigen.Photos {
	return apigen.Photos{Thumb: p.Thumb, Card: p.Card, Hero: p.Hero}
}

func apiRegion(r catalogtest.Region, loc string) apigen.Region {
	return apigen.Region{
		Key:            r.Key,
		Order:          r.SortOrder,
		Name:           r.Name[loc],
		NameEs:         r.Name[spanish],
		MapLabel:       r.MapLabel[loc],
		TileColor:      r.TileColor,
		Description:    r.Description[loc],
		RegionalDishes: slices.Clone(r.RegionalDishes),
	}
}

func apiRegions(loc string) []apigen.Region {
	var out []apigen.Region
	for _, r := range fixture().Regions {
		out = append(out, apiRegion(r, loc))
	}
	return out
}

func apiRegionVariations(f catalogtest.Fixture, regionKey, loc string) []apigen.RegionVariation {
	var out []apigen.RegionVariation
	for _, d := range f.Dishes {
		i := slices.IndexFunc(d.Variations, func(v catalogtest.Variation) bool { return v.RegionKey == regionKey })
		if i < 0 {
			continue
		}
		v := d.Variations[i]
		out = append(out, apigen.RegionVariation{
			DishKey: d.Key, DishName: d.Name[loc], DishNameEs: d.Name[spanish],
			RegionKey: v.RegionKey, RecipeKey: v.RecipeKey,
			Name: v.Name[loc], NameEs: v.Name[spanish], Description: v.Description[loc],
			Photos: apiPhotos(v.Photos),
		})
	}
	return out
}

func apiRegionDetail(t *testing.T, key, loc string) apigen.RegionDetail {
	t.Helper()
	f := fixture()
	r := apiRegion(f.Region(t, key), loc)
	return apigen.RegionDetail{
		Key: r.Key, Order: r.Order, Name: r.Name, NameEs: r.NameEs, MapLabel: r.MapLabel,
		TileColor: r.TileColor, Description: r.Description, RegionalDishes: r.RegionalDishes,
		Variations: apiRegionVariations(f, key, loc),
	}
}

func apiVariation(f catalogtest.Fixture, v catalogtest.Variation, loc string) apigen.VariationSummary {
	return apigen.VariationSummary{
		RegionKey:   v.RegionKey,
		RegionName:  regionName(f, v.RegionKey, catalog.Locale(loc)),
		RecipeKey:   v.RecipeKey,
		Name:        v.Name[loc],
		NameEs:      v.Name[spanish],
		Description: v.Description[loc],
		Photos:      apiPhotos(v.Photos),
	}
}

func apiDish(f catalogtest.Fixture, d catalogtest.Dish, loc string) apigen.Dish {
	dish := apigen.Dish{
		Key:         d.Key,
		Order:       d.SortOrder,
		Name:        d.Name[loc],
		NameEs:      d.Name[spanish],
		Description: d.Description[loc],
		Flags: apigen.Flags{
			Vegetarian: d.Vegetarian, Seafood: d.Seafood, Pork: d.Pork,
			Quick: d.Quick, SlowCooked: d.SlowCooked,
		},
		Course:     apigen.Course(d.Course),
		Season:     apigen.Season(d.Season),
		Photos:     apiPhotos(d.Photos),
		Variations: []apigen.VariationSummary{},
	}
	for _, v := range d.Variations {
		dish.Variations = append(dish.Variations, apiVariation(f, v, loc))
	}
	return dish
}

func apiDishes(loc string) []apigen.Dish {
	f := fixture()
	var out []apigen.Dish
	for _, d := range f.Dishes {
		out = append(out, apiDish(f, d, loc))
	}
	return out
}

func apiDishByKey(t *testing.T, key, loc string) apigen.Dish {
	t.Helper()
	f := fixture()
	return apiDish(f, f.Dish(t, key), loc)
}

func apiIngredients(rc catalogtest.Recipe, loc string) []apigen.Ingredient {
	var out []apigen.Ingredient
	for _, ing := range rc.Ingredients {
		var u *apigen.Unit
		if ing.Unit != nil {
			v := apigen.Unit(*ing.Unit)
			u = &v
		}
		out = append(out, apigen.Ingredient{Quantity: ing.Quantity, Unit: u, Name: ing.Name[loc]})
	}
	return out
}

func apiRecipe(t *testing.T, p recipePair, loc string) apigen.Recipe {
	t.Helper()
	f := fixture()
	d, v, ok := fixtureVariation(f, p.dish, p.region)
	if !ok {
		t.Fatalf("fixture has no variation %s/%s", p.dish, p.region)
	}
	return apigen.Recipe{
		DishKey:      d.Key,
		DishName:     d.Name[loc],
		RegionKey:    v.RegionKey,
		RegionName:   regionName(f, v.RegionKey, catalog.Locale(loc)),
		RecipeKey:    v.RecipeKey,
		Name:         v.Name[loc],
		NameEs:       v.Name[spanish],
		Description:  v.Description[loc],
		TimeMinutes:  v.Recipe.TimeMinutes,
		TimeNote:     timeNote(v.Recipe, catalog.Locale(loc)),
		BaseServings: v.Recipe.BaseServings,
		Ingredients:  apiIngredients(v.Recipe, loc),
		Steps:        localeSteps(v.Recipe, catalog.Locale(loc)),
		Photos:       apiPhotos(v.Photos),
	}
}
