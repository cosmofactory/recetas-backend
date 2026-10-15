package catalog

import (
	"context"
	"fmt"
	"slices"
)

// Service serves the catalog at the latest content version, which it reads
// on every call. The region and dish lists are memoised per (locale,
// version); the detail endpoints are answered from those lists. Returned
// lists are shared and must not be modified.
type Service struct {
	repo    Repository
	regions *memo[[]Region]
	dishes  *memo[[]Dish]
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, regions: newMemo[[]Region](), dishes: newMemo[[]Dish]()}
}

// Regions returns every region in region order, and the content version.
func (s *Service) Regions(ctx context.Context, l Locale) ([]Region, int64, error) {
	version, err := s.contentVersion(ctx)
	if err != nil {
		return nil, 0, err
	}
	regions, err := s.regionsAt(ctx, l, version)
	if err != nil {
		return nil, 0, err
	}
	return regions, version, nil
}

// Region returns one region with every dish's variation there, in dish order.
func (s *Service) Region(ctx context.Context, key string, l Locale) (RegionDetail, int64, error) {
	regions, version, err := s.Regions(ctx, l)
	if err != nil {
		return RegionDetail{}, 0, err
	}
	i := slices.IndexFunc(regions, func(r Region) bool { return r.Key == key })
	if i < 0 {
		return RegionDetail{}, 0, fmt.Errorf("region %q: %w", key, ErrNotFound)
	}
	dishes, err := s.dishesAt(ctx, l, version)
	if err != nil {
		return RegionDetail{}, 0, err
	}
	return RegionDetail{Region: regions[i], Variations: variationsIn(dishes, key)}, version, nil
}

// Dishes returns every dish in dish order, and the content version.
func (s *Service) Dishes(ctx context.Context, l Locale) ([]Dish, int64, error) {
	version, err := s.contentVersion(ctx)
	if err != nil {
		return nil, 0, err
	}
	dishes, err := s.dishesAt(ctx, l, version)
	if err != nil {
		return nil, 0, err
	}
	return dishes, version, nil
}

// Dish returns one dish with all its variations.
func (s *Service) Dish(ctx context.Context, key string, l Locale) (Dish, int64, error) {
	dishes, version, err := s.Dishes(ctx, l)
	if err != nil {
		return Dish{}, 0, err
	}
	i := slices.IndexFunc(dishes, func(d Dish) bool { return d.Key == key })
	if i < 0 {
		return Dish{}, 0, fmt.Errorf("dish %q: %w", key, ErrNotFound)
	}
	return dishes[i], version, nil
}

// Recipe returns the recipe of dishKey's variation in regionKey, or
// ErrNotFound when the dish, the region or that variation doesn't exist.
func (s *Service) Recipe(ctx context.Context, dishKey, regionKey string, l Locale) (Recipe, int64, error) {
	version, err := s.contentVersion(ctx)
	if err != nil {
		return Recipe{}, 0, err
	}
	rc, err := s.repo.Recipe(ctx, dishKey, regionKey, l)
	if err != nil {
		return Recipe{}, 0, fmt.Errorf("load recipe: %w", err)
	}
	return rc, version, nil
}

func (s *Service) contentVersion(ctx context.Context) (int64, error) {
	version, err := s.repo.ContentVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("load content version: %w", err)
	}
	return version, nil
}

func (s *Service) regionsAt(ctx context.Context, l Locale, version int64) ([]Region, error) {
	regions, err := s.regions.get(ctx, memoKey{locale: l, version: version}, s.repo.Regions)
	if err != nil {
		return nil, fmt.Errorf("load regions: %w", err)
	}
	return regions, nil
}

func (s *Service) dishesAt(ctx context.Context, l Locale, version int64) ([]Dish, error) {
	dishes, err := s.dishes.get(ctx, memoKey{locale: l, version: version}, s.repo.Dishes)
	if err != nil {
		return nil, fmt.Errorf("load dishes: %w", err)
	}
	return dishes, nil
}

// variationsIn lists each dish's variation in regionKey, in dish order.
func variationsIn(dishes []Dish, regionKey string) []RegionVariation {
	var out []RegionVariation
	for _, d := range dishes {
		i := slices.IndexFunc(d.Variations, func(v Variation) bool { return v.RegionKey == regionKey })
		if i < 0 {
			continue
		}
		out = append(out, RegionVariation{DishKey: d.Key, DishName: d.Name, DishNameEs: d.NameEs, Variation: d.Variations[i]})
	}
	return out
}
