// Package catalog serves the read-only atlas: regions, dishes with their
// variations (one per dish and region), and one recipe per variation.
package catalog

import "context"

// Locale is a content locale code (en, es, ru).
type Locale string

// localeSpanish is the locale that feeds every NameEs field.
const localeSpanish Locale = "es"

// Photos are media object key prefixes; nil means no photo yet.
type Photos struct {
	Thumb *string
	Card  *string
	Hero  *string
}

type Region struct {
	Key            string
	Order          int
	Name           string
	NameEs         string
	MapLabel       string
	TileColor      string
	Description    string
	RegionalDishes []string // Spanish proper names, not translated
}

type Flags struct {
	Vegetarian bool
	Seafood    bool
	Pork       bool
	Quick      bool
	SlowCooked bool
}

// Variation is a dish as cooked in one region. All variations of a dish are
// equal; they are ordered by region order only.
type Variation struct {
	RegionKey   string
	RegionName  string
	RecipeKey   string
	Name        string
	NameEs      string
	Description string
	Photos      Photos
}

type Dish struct {
	Key         string
	Order       int
	Name        string
	NameEs      string
	Description string
	Flags       Flags
	Course      string
	Season      string
	Photos      Photos
	Variations  []Variation // in region order
}

// RegionVariation is a variation listed under its region, named by its dish.
type RegionVariation struct {
	DishKey    string
	DishName   string
	DishNameEs string
	Variation
}

type RegionDetail struct {
	Region
	Variations []RegionVariation // in dish order
}

type Ingredient struct {
	Quantity float64
	Unit     *string // nil is a counted item
	Name     string
}

type Recipe struct {
	DishKey      string
	DishName     string
	RegionKey    string
	RegionName   string
	RecipeKey    string
	Name         string
	NameEs       string
	Description  string
	TimeMinutes  int
	TimeNote     *string
	BaseServings int
	Ingredients  []Ingredient
	Steps        []string
	Photos       Photos
}

// Repository reads the catalog in one locale. Lists come in editorial order.
type Repository interface {
	// ContentVersion is the latest content import, or 0 before the first.
	ContentVersion(ctx context.Context) (int64, error)
	Regions(ctx context.Context, l Locale) ([]Region, error)
	Dishes(ctx context.Context, l Locale) ([]Dish, error)
	// Recipe returns ErrNotFound when the dish has no variation in the region.
	Recipe(ctx context.Context, dishKey, regionKey string, l Locale) (Recipe, error)
}
