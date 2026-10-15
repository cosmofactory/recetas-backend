package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

// The stored catalog, read back into bundle types so it compares directly.
type (
	// storedOrdered is a region or dish row.
	storedOrdered[T any] struct {
		id    int64
		order int
		value T
	}
	storedRegion    = storedOrdered[Region]
	storedDish      = storedOrdered[Dish] // without variations
	storedVariation struct {
		id        int64
		dishKey   string
		variation Variation
	}
	storedRecipe struct {
		id     int64
		recipe Recipe
	}
)

type state struct {
	regions    map[string]storedRegion    // by key
	dishes     map[string]storedDish      // by key
	variations map[string]storedVariation // by recipe key
	recipes    map[string]storedRecipe    // by recipe key
}

func loadState(ctx context.Context, q *catalogdb.Queries) (state, error) {
	var s state
	var err error
	if s.regions, err = loadRegions(ctx, q); err != nil {
		return state{}, err
	}
	if s.dishes, err = loadDishes(ctx, q); err != nil {
		return state{}, err
	}
	if s.variations, err = loadVariations(ctx, q); err != nil {
		return state{}, err
	}
	if s.recipes, err = loadRecipes(ctx, q); err != nil {
		return state{}, err
	}
	return s, nil
}

func loadRegions(ctx context.Context, q *catalogdb.Queries) (map[string]storedRegion, error) {
	rows, err := q.ImportListRegions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list regions: %w", err)
	}
	dishRows, err := q.ImportListRegionalDishes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list regional dishes: %w", err)
	}
	names := map[int64][]string{}
	for _, d := range dishRows {
		names[d.RegionID] = append(names[d.RegionID], d.Name)
	}
	out := make(map[string]storedRegion, len(rows))
	for _, r := range rows {
		out[r.Key] = storedRegion{id: r.ID, order: int(r.SortOrder), value: Region{
			Key:            r.Key,
			TileColor:      r.TileColor,
			Name:           Text{r.NameEn, r.NameEs, r.NameRu},
			MapLabel:       Text{r.MapLabelEn, r.MapLabelEs, r.MapLabelRu},
			Description:    Text{r.DescriptionEn, r.DescriptionEs, r.DescriptionRu},
			RegionalDishes: names[r.ID],
		}}
	}
	return out, nil
}

func loadDishes(ctx context.Context, q *catalogdb.Queries) (map[string]storedDish, error) {
	rows, err := q.ImportListDishes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list dishes: %w", err)
	}
	out := make(map[string]storedDish, len(rows))
	for _, r := range rows {
		out[r.Key] = storedDish{id: r.ID, order: int(r.SortOrder), value: Dish{
			Key:         r.Key,
			Name:        Text{r.NameEn, r.NameEs, r.NameRu},
			Description: Text{r.DescriptionEn, r.DescriptionEs, r.DescriptionRu},
			Flags:       Flags{r.IsVegetarian, r.IsSeafood, r.HasPork, r.IsQuick, r.IsSlowCooked},
			Course:      r.Course,
			Season:      r.Season,
			Photos:      &Photos{Thumb: r.PhotoThumb, Card: r.PhotoCard, Hero: r.PhotoHero},
		}}
	}
	return out, nil
}

func loadVariations(ctx context.Context, q *catalogdb.Queries) (map[string]storedVariation, error) {
	rows, err := q.ImportListVariations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list variations: %w", err)
	}
	out := make(map[string]storedVariation, len(rows))
	for _, r := range rows {
		out[r.RecipeKey] = storedVariation{id: r.ID, dishKey: r.DishKey, variation: Variation{
			RegionKey:   r.RegionKey,
			RecipeKey:   r.RecipeKey,
			Name:        Text{r.NameEn, r.NameEs, r.NameRu},
			Description: Text{r.DescriptionEn, r.DescriptionEs, r.DescriptionRu},
			Photos:      &Photos{Thumb: r.PhotoThumb, Card: r.PhotoCard, Hero: r.PhotoHero},
		}}
	}
	return out, nil
}
