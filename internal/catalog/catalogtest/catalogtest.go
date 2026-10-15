// Package catalogtest loads a small, complete catalog into a test
// transaction. Texts come from the design's content.json where it has them;
// recipe keys are slugified (D-BE3-2).
//
// Load uses fixed keys and sort orders (gal, ast, tortilla, cocido, ...), so
// two uncommitted loads in concurrent transactions serialize on the unique
// indexes, and two loads in one transaction fail. Load once per transaction.
package catalogtest

import (
	"testing"
	"time"
)

// Locales are the content locales, in code order.
var Locales = []string{"en", "es", "ru"}

// Text is one translatable field: locale code to text.
type Text map[string]string

type Region struct {
	Key            string
	SortOrder      int
	TileColor      string
	Name           Text
	MapLabel       Text
	Description    Text
	RegionalDishes []string
}

type Ingredient struct {
	Quantity float64
	Unit     *string // nil = counted item
	Name     Text
}

type Recipe struct {
	TimeMinutes  int
	TimeNote     *Text // nil = no note (no recipe_translations rows)
	BaseServings int
	Ingredients  []Ingredient
	Steps        []Text
}

// Photos are media object key prefixes (stack.md X3:
// dishes/<dishKey>[/<regionKey>]/<role>-<hash>); nil = no photo yet.
type Photos struct {
	Thumb *string
	Card  *string
	Hero  *string
}

type Variation struct {
	RegionKey   string
	RecipeKey   string
	Name        Text
	Description Text
	Photos      Photos
	Recipe      Recipe
}

type Dish struct {
	Key         string
	SortOrder   int
	Course      string
	Season      string
	Vegetarian  bool
	Seafood     bool
	Pork        bool
	Quick       bool
	SlowCooked  bool
	Name        Text
	Description Text
	Photos      Photos
	Variations  []Variation // in region order
}

type ContentVersion struct {
	ImportedAt time.Time
	Source     string
	Checksum   string
}

// Fixture is the loaded data plus the database ids it was given.
type Fixture struct {
	Regions        []Region // in region order
	Dishes         []Dish   // in dish order
	ContentVersion ContentVersion

	RegionIDs        map[string]int64 // by region key
	DishIDs          map[string]int64 // by dish key
	VariationIDs     map[string]int64 // by recipe key
	RecipeIDs        map[string]int64 // by recipe key
	ContentVersionID int64
}

// Region returns the fixture region with key, failing the test if absent.
func (f Fixture) Region(t testing.TB, key string) Region {
	t.Helper()
	for _, r := range f.Regions {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("catalogtest: no region %q", key)
	return Region{}
}

// Dish returns the fixture dish with key, failing the test if absent.
func (f Fixture) Dish(t testing.TB, key string) Dish {
	t.Helper()
	for _, d := range f.Dishes {
		if d.Key == key {
			return d
		}
	}
	t.Fatalf("catalogtest: no dish %q", key)
	return Dish{}
}
