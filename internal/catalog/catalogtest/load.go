package catalogtest

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

// firstPosition is the position of the first regional dish, ingredient and
// step; positions are 1-based.
const firstPosition = 1

// Load inserts Data() into tx with raw SQL and returns it with its ids.
// A recipe gets recipe_translations rows only when it has a TimeNote.
func Load(t testing.TB, tx pgx.Tx) Fixture {
	t.Helper()
	l := &loader{t: t, tx: tx, f: Data()}
	l.f.RegionIDs = map[string]int64{}
	l.f.DishIDs = map[string]int64{}
	l.f.VariationIDs = map[string]int64{}
	l.f.RecipeIDs = map[string]int64{}

	for _, r := range l.f.Regions {
		l.insertRegion(r)
	}
	for _, d := range l.f.Dishes {
		l.insertDish(d)
	}
	l.insertContentVersion()
	return l.f
}

type loader struct {
	t  testing.TB
	tx pgx.Tx
	f  Fixture
}

func (l *loader) exec(what, sql string, args ...any) {
	l.t.Helper()
	if _, err := l.tx.Exec(context.Background(), sql, args...); err != nil {
		l.t.Fatalf("catalogtest: insert %s: %v", what, err)
	}
}

func (l *loader) queryID(what, sql string, args ...any) int64 {
	l.t.Helper()
	var id int64
	if err := l.tx.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		l.t.Fatalf("catalogtest: insert %s: %v", what, err)
	}
	return id
}

// requireLocales fails the test unless text has exactly the content locales.
func (l *loader) requireLocales(what string, text Text) {
	l.t.Helper()
	if len(text) != len(Locales) {
		l.t.Fatalf("catalogtest: %s has locales %v, want %v", what, sortedKeys(text), Locales)
	}
	for _, loc := range Locales {
		if _, ok := text[loc]; !ok {
			l.t.Fatalf("catalogtest: %s missing locale %q", what, loc)
		}
	}
}

// insertPerLocale runs sql once per locale with the args built for it.
func (l *loader) insertPerLocale(what, sql string, args func(locale string) []any) {
	l.t.Helper()
	for _, loc := range Locales {
		l.exec(what+"/"+loc, sql, args(loc)...)
	}
}

func (l *loader) insertRegion(r Region) {
	l.t.Helper()
	id := l.queryID("region "+r.Key,
		`INSERT INTO regions (key, sort_order, tile_color) VALUES ($1, $2, $3) RETURNING id`,
		r.Key, r.SortOrder, r.TileColor)
	l.f.RegionIDs[r.Key] = id
	l.insertRegionTranslations(id, r)
	l.insertRegionalDishes(id, r)
}

func (l *loader) insertRegionTranslations(regionID int64, r Region) {
	l.t.Helper()
	l.requireLocales("region "+r.Key+" name", r.Name)
	l.requireLocales("region "+r.Key+" mapLabel", r.MapLabel)
	l.requireLocales("region "+r.Key+" description", r.Description)
	l.insertPerLocale("region translation "+r.Key,
		`INSERT INTO region_translations (region_id, locale, name, map_label, description) VALUES ($1, $2, $3, $4, $5)`,
		func(loc string) []any { return []any{regionID, loc, r.Name[loc], r.MapLabel[loc], r.Description[loc]} })
}

func (l *loader) insertRegionalDishes(regionID int64, r Region) {
	l.t.Helper()
	for i, name := range r.RegionalDishes {
		l.exec("regional dish "+name,
			`INSERT INTO region_regional_dishes (region_id, position, name) VALUES ($1, $2, $3)`,
			regionID, firstPosition+i, name)
	}
}

func (l *loader) insertDish(d Dish) {
	l.t.Helper()
	id := l.queryID("dish "+d.Key, `
		INSERT INTO dishes (key, sort_order, course, season,
			is_vegetarian, is_seafood, has_pork, is_quick, is_slow_cooked,
			photo_thumb, photo_card, photo_hero)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		d.Key, d.SortOrder, d.Course, d.Season,
		d.Vegetarian, d.Seafood, d.Pork, d.Quick, d.SlowCooked,
		d.Photos.Thumb, d.Photos.Card, d.Photos.Hero)
	l.f.DishIDs[d.Key] = id

	l.requireLocales("dish "+d.Key+" name", d.Name)
	l.requireLocales("dish "+d.Key+" description", d.Description)
	l.insertPerLocale("dish translation "+d.Key,
		`INSERT INTO dish_translations (dish_id, locale, name, description) VALUES ($1, $2, $3, $4)`,
		func(loc string) []any { return []any{id, loc, d.Name[loc], d.Description[loc]} })

	for _, v := range d.Variations {
		l.insertVariation(id, v)
	}
}

func (l *loader) insertVariation(dishID int64, v Variation) {
	l.t.Helper()
	regionID, ok := l.f.RegionIDs[v.RegionKey]
	if !ok {
		l.t.Fatalf("catalogtest: variation %s: unknown region %q", v.RecipeKey, v.RegionKey)
	}
	id := l.queryID("variation "+v.RecipeKey,
		`INSERT INTO variations (dish_id, region_id, recipe_key, photo_thumb, photo_card, photo_hero)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		dishID, regionID, v.RecipeKey, v.Photos.Thumb, v.Photos.Card, v.Photos.Hero)
	l.f.VariationIDs[v.RecipeKey] = id

	l.requireLocales("variation "+v.RecipeKey+" name", v.Name)
	l.requireLocales("variation "+v.RecipeKey+" description", v.Description)
	l.insertPerLocale("variation translation "+v.RecipeKey,
		`INSERT INTO variation_translations (variation_id, locale, name, description) VALUES ($1, $2, $3, $4)`,
		func(loc string) []any { return []any{id, loc, v.Name[loc], v.Description[loc]} })

	l.insertRecipe(id, v)
}

func (l *loader) insertRecipe(variationID int64, v Variation) {
	l.t.Helper()
	rc := v.Recipe
	id := l.queryID("recipe "+v.RecipeKey,
		`INSERT INTO recipes (variation_id, time_minutes, base_servings) VALUES ($1, $2, $3) RETURNING id`,
		variationID, rc.TimeMinutes, rc.BaseServings)
	l.f.RecipeIDs[v.RecipeKey] = id

	l.insertTimeNote(id, v.RecipeKey, rc.TimeNote)
	for i, ing := range rc.Ingredients {
		l.insertIngredient(id, firstPosition+i, ing)
	}
	for i, step := range rc.Steps {
		l.insertStep(id, firstPosition+i, step)
	}
}

func (l *loader) insertTimeNote(recipeID int64, recipeKey string, note *Text) {
	l.t.Helper()
	if note == nil {
		return
	}
	l.requireLocales("recipe "+recipeKey+" time note", *note)
	l.insertPerLocale("recipe translation "+recipeKey,
		`INSERT INTO recipe_translations (recipe_id, locale, time_note) VALUES ($1, $2, $3)`,
		func(loc string) []any { return []any{recipeID, loc, (*note)[loc]} })
}

func (l *loader) insertIngredient(recipeID int64, position int, ing Ingredient) {
	l.t.Helper()
	l.exec("ingredient",
		`INSERT INTO recipe_ingredients (recipe_id, position, quantity, unit) VALUES ($1, $2, $3, $4)`,
		recipeID, position, ing.Quantity, ing.Unit)
	l.requireLocales("ingredient name", ing.Name)
	l.insertPerLocale("ingredient translation",
		`INSERT INTO recipe_ingredient_translations (recipe_id, position, locale, name) VALUES ($1, $2, $3, $4)`,
		func(loc string) []any { return []any{recipeID, position, loc, ing.Name[loc]} })
}

func (l *loader) insertStep(recipeID int64, position int, step Text) {
	l.t.Helper()
	l.exec("step",
		`INSERT INTO recipe_steps (recipe_id, position) VALUES ($1, $2)`,
		recipeID, position)
	l.requireLocales("step", step)
	l.insertPerLocale("step translation",
		`INSERT INTO recipe_step_translations (recipe_id, position, locale, text) VALUES ($1, $2, $3, $4)`,
		func(loc string) []any { return []any{recipeID, position, loc, step[loc]} })
}

func (l *loader) insertContentVersion() {
	l.t.Helper()
	cv := l.f.ContentVersion
	l.f.ContentVersionID = l.queryID("content version",
		`INSERT INTO content_versions (imported_at, source, checksum) VALUES ($1, $2, $3) RETURNING id`,
		cv.ImportedAt, cv.Source, cv.Checksum)
}

func sortedKeys(text Text) []string {
	keys := make([]string, 0, len(text))
	for k := range text {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
