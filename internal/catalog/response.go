package catalog

import "github.com/cosmofactory/recetas-backend/internal/api/apigen"

// Domain values map onto the contract's generated types. Nil slices become
// empty ones, because the contract's arrays are never null.

func notFoundBody() apigen.NotFoundJSONResponse {
	var body apigen.NotFoundJSONResponse
	body.Error.Code = apigen.ErrorErrorCodeNotFound
	body.Error.Message = "not found"
	return body
}

func internalBody() apigen.InternalJSONResponse {
	var body apigen.InternalJSONResponse
	body.Error.Code = apigen.ErrorErrorCodeInternal
	body.Error.Message = "internal error"
	return body
}

func toAPIPhotos(p Photos) apigen.Photos {
	return apigen.Photos{Thumb: p.Thumb, Card: p.Card, Hero: p.Hero}
}

func toAPIRegions(regions []Region) []apigen.Region {
	return mapSlice(regions, toAPIRegion)
}

func toAPIRegion(r Region) apigen.Region {
	return apigen.Region{
		Key:            r.Key,
		Order:          r.Order,
		Name:           r.Name,
		NameEs:         r.NameEs,
		MapLabel:       r.MapLabel,
		TileColor:      r.TileColor,
		Description:    r.Description,
		RegionalDishes: nonNil(r.RegionalDishes),
	}
}

func toAPIRegionDetail(d RegionDetail) apigen.RegionDetail {
	r := toAPIRegion(d.Region)
	return apigen.RegionDetail{
		Key:            r.Key,
		Order:          r.Order,
		Name:           r.Name,
		NameEs:         r.NameEs,
		MapLabel:       r.MapLabel,
		TileColor:      r.TileColor,
		Description:    r.Description,
		RegionalDishes: r.RegionalDishes,
		Variations:     mapSlice(d.Variations, toAPIRegionVariation),
	}
}

func toAPIRegionVariation(v RegionVariation) apigen.RegionVariation {
	return apigen.RegionVariation{
		DishKey:     v.DishKey,
		DishName:    v.DishName,
		DishNameEs:  v.DishNameEs,
		RegionKey:   v.RegionKey,
		RecipeKey:   v.RecipeKey,
		Name:        v.Name,
		NameEs:      v.NameEs,
		Description: v.Description,
		Photos:      toAPIPhotos(v.Photos),
	}
}

func toAPIDishes(dishes []Dish) []apigen.Dish {
	return mapSlice(dishes, toAPIDish)
}

func toAPIDish(d Dish) apigen.Dish {
	return apigen.Dish{
		Key:         d.Key,
		Order:       d.Order,
		Name:        d.Name,
		NameEs:      d.NameEs,
		Description: d.Description,
		Flags: apigen.Flags{
			Vegetarian: d.Flags.Vegetarian,
			Seafood:    d.Flags.Seafood,
			Pork:       d.Flags.Pork,
			Quick:      d.Flags.Quick,
			SlowCooked: d.Flags.SlowCooked,
		},
		Course:     apigen.Course(d.Course),
		Season:     apigen.Season(d.Season),
		Photos:     toAPIPhotos(d.Photos),
		Variations: mapSlice(d.Variations, toAPIVariation),
	}
}

func toAPIVariation(v Variation) apigen.VariationSummary {
	return apigen.VariationSummary{
		RegionKey:   v.RegionKey,
		RegionName:  v.RegionName,
		RecipeKey:   v.RecipeKey,
		Name:        v.Name,
		NameEs:      v.NameEs,
		Description: v.Description,
		Photos:      toAPIPhotos(v.Photos),
	}
}

func toAPIRecipe(rc Recipe) apigen.Recipe {
	return apigen.Recipe{
		DishKey:      rc.DishKey,
		DishName:     rc.DishName,
		RegionKey:    rc.RegionKey,
		RegionName:   rc.RegionName,
		RecipeKey:    rc.RecipeKey,
		Name:         rc.Name,
		NameEs:       rc.NameEs,
		Description:  rc.Description,
		TimeMinutes:  rc.TimeMinutes,
		TimeNote:     rc.TimeNote,
		BaseServings: rc.BaseServings,
		Ingredients:  mapSlice(rc.Ingredients, toAPIIngredient),
		Steps:        nonNil(rc.Steps),
		Photos:       toAPIPhotos(rc.Photos),
	}
}

func toAPIIngredient(i Ingredient) apigen.Ingredient {
	out := apigen.Ingredient{Quantity: i.Quantity, Name: i.Name}
	if i.Unit != nil {
		unit := apigen.Unit(*i.Unit)
		out.Unit = &unit
	}
	return out
}

func mapSlice[From, To any](items []From, convert func(From) To) []To {
	out := make([]To, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
