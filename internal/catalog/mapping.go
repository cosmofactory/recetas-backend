package catalog

import "github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"

func regionsFromRows(rows []catalogdb.ListRegionsRow, l Locale) ([]Region, error) {
	regions := make([]Region, 0, len(rows))
	for _, row := range rows {
		err := firstMissing(
			translation{entityRegion, row.Key, l, []*string{row.Name, row.MapLabel, row.Description}},
			translation{entityRegion, row.Key, localeSpanish, []*string{row.NameEs}},
		)
		if err != nil {
			return nil, err
		}
		regions = append(regions, Region{
			Key:         row.Key,
			Order:       int(row.SortOrder),
			Name:        *row.Name,
			NameEs:      *row.NameEs,
			MapLabel:    *row.MapLabel,
			TileColor:   row.TileColor,
			Description: *row.Description,
		})
	}
	return regions, nil
}

// attachRegionalDishes appends each regional dish to its region. Rows of a
// region missing from regions (added between the two queries) are dropped.
func attachRegionalDishes(regions []Region, rows []catalogdb.ListRegionalDishesRow) {
	byKey := indexBy(regions, func(r Region) string { return r.Key })
	for _, row := range rows {
		if i, ok := byKey[row.RegionKey]; ok {
			regions[i].RegionalDishes = append(regions[i].RegionalDishes, row.Name)
		}
	}
}

func dishesFromRows(rows []catalogdb.ListDishesRow, l Locale) ([]Dish, error) {
	dishes := make([]Dish, 0, len(rows))
	for _, row := range rows {
		err := firstMissing(
			translation{entityDish, row.Key, l, []*string{row.Name, row.Description}},
			translation{entityDish, row.Key, localeSpanish, []*string{row.NameEs}},
		)
		if err != nil {
			return nil, err
		}
		dishes = append(dishes, dishFromRow(row))
	}
	return dishes, nil
}

// dishFromRow maps a row whose translations firstMissing has checked.
func dishFromRow(row catalogdb.ListDishesRow) Dish {
	return Dish{
		Key:         row.Key,
		Order:       int(row.SortOrder),
		Name:        *row.Name,
		NameEs:      *row.NameEs,
		Description: *row.Description,
		Flags: Flags{
			Vegetarian: row.IsVegetarian,
			Seafood:    row.IsSeafood,
			Pork:       row.HasPork,
			Quick:      row.IsQuick,
			SlowCooked: row.IsSlowCooked,
		},
		Course: row.Course,
		Season: row.Season,
		Photos: Photos{Thumb: row.PhotoThumb, Card: row.PhotoCard, Hero: row.PhotoHero},
	}
}

// attachVariations appends each variation to its dish, keeping the rows'
// region order. Rows of a dish missing from dishes (added between the two
// queries) are dropped.
func attachVariations(dishes []Dish, rows []catalogdb.ListVariationsRow, l Locale) error {
	byKey := indexBy(dishes, func(d Dish) string { return d.Key })
	for _, row := range rows {
		i, ok := byKey[row.DishKey]
		if !ok {
			continue
		}
		v, err := variationFromRow(row, l)
		if err != nil {
			return err
		}
		dishes[i].Variations = append(dishes[i].Variations, v)
	}
	return nil
}

func variationFromRow(row catalogdb.ListVariationsRow, l Locale) (Variation, error) {
	err := firstMissing(
		translation{entityVariation, row.RecipeKey, l, []*string{row.Name, row.Description}},
		translation{entityVariation, row.RecipeKey, localeSpanish, []*string{row.NameEs}},
		translation{entityRegion, row.RegionKey, l, []*string{row.RegionName}},
	)
	if err != nil {
		return Variation{}, err
	}
	return Variation{
		RegionKey:   row.RegionKey,
		RegionName:  *row.RegionName,
		RecipeKey:   row.RecipeKey,
		Name:        *row.Name,
		NameEs:      *row.NameEs,
		Description: *row.Description,
		Photos:      Photos{Thumb: row.PhotoThumb, Card: row.PhotoCard, Hero: row.PhotoHero},
	}, nil
}

func recipeFromRow(row catalogdb.GetRecipeByDishRegionRow, l Locale) (Recipe, error) {
	if err := recipeRowMissing(row, l); err != nil {
		return Recipe{}, err
	}
	return Recipe{
		DishKey:      row.DishKey,
		DishName:     *row.DishName,
		RegionKey:    row.RegionKey,
		RegionName:   *row.RegionName,
		RecipeKey:    row.RecipeKey,
		Name:         *row.Name,
		NameEs:       *row.NameEs,
		Description:  *row.Description,
		TimeMinutes:  int(row.TimeMinutes),
		TimeNote:     row.TimeNote,
		BaseServings: int(row.BaseServings),
		Photos:       Photos{Thumb: row.PhotoThumb, Card: row.PhotoCard, Hero: row.PhotoHero},
	}, nil
}

// recipeRowMissing checks the recipe row's translations. The time note is
// optional, but once a recipe has one it must have it in every locale.
func recipeRowMissing(row catalogdb.GetRecipeByDishRegionRow, l Locale) error {
	var timeNote []*string
	if row.HasTimeNote {
		timeNote = []*string{row.TimeNote}
	}
	return firstMissing(
		translation{entityDish, row.DishKey, l, []*string{row.DishName}},
		translation{entityRegion, row.RegionKey, l, []*string{row.RegionName}},
		translation{entityVariation, row.RecipeKey, l, []*string{row.Name, row.Description}},
		translation{entityVariation, row.RecipeKey, localeSpanish, []*string{row.NameEs}},
		translation{entityRecipeTimeNote, row.RecipeKey, l, timeNote},
	)
}

func ingredientsFromRows(rows []catalogdb.ListRecipeIngredientsRow, recipeKey string, l Locale) ([]Ingredient, error) {
	var ingredients []Ingredient
	for _, row := range rows {
		if row.Name == nil {
			return nil, &MissingTranslationError{Entity: entityRecipeIngredient, Key: recipeKey, Locale: l}
		}
		ingredients = append(ingredients, Ingredient{Quantity: row.Quantity, Unit: row.Unit, Name: *row.Name})
	}
	return ingredients, nil
}

func stepsFromRows(rows []*string, recipeKey string, l Locale) ([]string, error) {
	var steps []string
	for _, text := range rows {
		if text == nil {
			return nil, &MissingTranslationError{Entity: entityRecipeStep, Key: recipeKey, Locale: l}
		}
		steps = append(steps, *text)
	}
	return steps, nil
}

// indexBy maps each item's key to its position.
func indexBy[T any](items []T, key func(T) string) map[string]int {
	index := make(map[string]int, len(items))
	for i, item := range items {
		index[key(item)] = i
	}
	return index
}
