package catalog_test

import (
	"slices"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogtest"
)

// Expected domain values, built from the catalogtest fixture so the fake
// repository and the Postgres repository are held to the same data.

var locales = []catalog.Locale{"en", "es", "ru"}

const spanish = "es"

func fixture() catalogtest.Fixture { return catalogtest.Data() }

func regionName(f catalogtest.Fixture, key string, l catalog.Locale) string {
	i := slices.IndexFunc(f.Regions, func(r catalogtest.Region) bool { return r.Key == key })
	if i < 0 {
		return ""
	}
	return f.Regions[i].Name[string(l)]
}

func domainPhotos(p catalogtest.Photos) catalog.Photos {
	return catalog.Photos{Thumb: p.Thumb, Card: p.Card, Hero: p.Hero}
}

func domainRegion(r catalogtest.Region, l catalog.Locale) catalog.Region {
	loc := string(l)
	return catalog.Region{
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

func domainRegions(l catalog.Locale) []catalog.Region {
	var out []catalog.Region
	for _, r := range fixture().Regions {
		out = append(out, domainRegion(r, l))
	}
	return out
}

func domainVariation(f catalogtest.Fixture, v catalogtest.Variation, l catalog.Locale) catalog.Variation {
	loc := string(l)
	return catalog.Variation{
		RegionKey:   v.RegionKey,
		RegionName:  regionName(f, v.RegionKey, l),
		RecipeKey:   v.RecipeKey,
		Name:        v.Name[loc],
		NameEs:      v.Name[spanish],
		Description: v.Description[loc],
		Photos:      domainPhotos(v.Photos),
	}
}

func domainDish(f catalogtest.Fixture, d catalogtest.Dish, l catalog.Locale) catalog.Dish {
	loc := string(l)
	dish := catalog.Dish{
		Key:         d.Key,
		Order:       d.SortOrder,
		Name:        d.Name[loc],
		NameEs:      d.Name[spanish],
		Description: d.Description[loc],
		Flags: catalog.Flags{
			Vegetarian: d.Vegetarian, Seafood: d.Seafood, Pork: d.Pork,
			Quick: d.Quick, SlowCooked: d.SlowCooked,
		},
		Course: d.Course,
		Season: d.Season,
		Photos: domainPhotos(d.Photos),
	}
	for _, v := range d.Variations {
		dish.Variations = append(dish.Variations, domainVariation(f, v, l))
	}
	return dish
}

func domainDishes(l catalog.Locale) []catalog.Dish {
	f := fixture()
	var out []catalog.Dish
	for _, d := range f.Dishes {
		out = append(out, domainDish(f, d, l))
	}
	return out
}

func domainDishByKey(t *testing.T, key string, l catalog.Locale) catalog.Dish {
	t.Helper()
	f := fixture()
	return domainDish(f, f.Dish(t, key), l)
}

// domainRegionVariations is every dish's variation in regionKey, in dish order.
func domainRegionVariations(regionKey string, l catalog.Locale) []catalog.RegionVariation {
	var out []catalog.RegionVariation
	for _, d := range domainDishes(l) {
		i := slices.IndexFunc(d.Variations, func(v catalog.Variation) bool { return v.RegionKey == regionKey })
		if i < 0 {
			continue
		}
		out = append(out, catalog.RegionVariation{
			DishKey: d.Key, DishName: d.Name, DishNameEs: d.NameEs, Variation: d.Variations[i],
		})
	}
	return out
}

func domainRegionDetail(t *testing.T, key string, l catalog.Locale) catalog.RegionDetail {
	t.Helper()
	return catalog.RegionDetail{
		Region:     domainRegion(fixture().Region(t, key), l),
		Variations: domainRegionVariations(key, l),
	}
}

// fixtureVariation finds the variation of dishKey in regionKey.
func fixtureVariation(f catalogtest.Fixture, dishKey, regionKey string) (catalogtest.Dish, catalogtest.Variation, bool) {
	di := slices.IndexFunc(f.Dishes, func(d catalogtest.Dish) bool { return d.Key == dishKey })
	if di < 0 {
		return catalogtest.Dish{}, catalogtest.Variation{}, false
	}
	d := f.Dishes[di]
	vi := slices.IndexFunc(d.Variations, func(v catalogtest.Variation) bool { return v.RegionKey == regionKey })
	if vi < 0 {
		return catalogtest.Dish{}, catalogtest.Variation{}, false
	}
	return d, d.Variations[vi], true
}

func domainIngredients(rc catalogtest.Recipe, l catalog.Locale) []catalog.Ingredient {
	var out []catalog.Ingredient
	for _, ing := range rc.Ingredients {
		out = append(out, catalog.Ingredient{Quantity: ing.Quantity, Unit: ing.Unit, Name: ing.Name[string(l)]})
	}
	return out
}

func localeSteps(rc catalogtest.Recipe, l catalog.Locale) []string {
	var out []string
	for _, s := range rc.Steps {
		out = append(out, s[string(l)])
	}
	return out
}

func timeNote(rc catalogtest.Recipe, l catalog.Locale) *string {
	if rc.TimeNote == nil {
		return nil
	}
	note := (*rc.TimeNote)[string(l)]
	return &note
}

// domainRecipe is the recipe of dishKey in regionKey; ok is false when the
// fixture has no such variation.
func domainRecipe(dishKey, regionKey string, l catalog.Locale) (catalog.Recipe, bool) {
	f := fixture()
	d, v, ok := fixtureVariation(f, dishKey, regionKey)
	if !ok {
		return catalog.Recipe{}, false
	}
	loc := string(l)
	return catalog.Recipe{
		DishKey:      d.Key,
		DishName:     d.Name[loc],
		RegionKey:    v.RegionKey,
		RegionName:   regionName(f, v.RegionKey, l),
		RecipeKey:    v.RecipeKey,
		Name:         v.Name[loc],
		NameEs:       v.Name[spanish],
		Description:  v.Description[loc],
		TimeMinutes:  v.Recipe.TimeMinutes,
		TimeNote:     timeNote(v.Recipe, l),
		BaseServings: v.Recipe.BaseServings,
		Ingredients:  domainIngredients(v.Recipe, l),
		Steps:        localeSteps(v.Recipe, l),
		Photos:       domainPhotos(v.Photos),
	}, true
}

// recipePair is a (dish, region) pair that has a variation in the fixture.
type recipePair struct{ dish, region string }

var fixtureRecipePairs = []recipePair{
	{"tortilla", "gal"},
	{"tortilla", "ast"},
	{"cocido", "ast"},
}

func mustDomainRecipe(t *testing.T, p recipePair, l catalog.Locale) catalog.Recipe {
	t.Helper()
	rc, ok := domainRecipe(p.dish, p.region, l)
	if !ok {
		t.Fatalf("fixture has no variation %s/%s", p.dish, p.region)
	}
	return rc
}
