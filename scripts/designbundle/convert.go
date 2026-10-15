package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

const (
	placeholderMinutes  = 30
	placeholderServings = 4
	minutesPerHour      = 60
)

var (
	placeholderIngredient = importer.Text{
		En: "Placeholder ingredient",
		Es: "Ingrediente provisional",
		Ru: "Ингредиент-заглушка",
	}
	placeholderStep = importer.Text{
		En: "Placeholder: the real recipe arrives with the content pipeline.",
		Es: "Provisional: la receta real llegará con el canal de contenidos.",
		Ru: "Заглушка: настоящий рецепт появится вместе с контент-конвейером.",
	}
)

// convert keys every variation's recipe by <dishKey>-<regionKey> and lists
// the recipes in variation order.
func convert(d design) (importer.Bundle, error) {
	if err := checkRecipesMatched(d); err != nil {
		return importer.Bundle{}, err
	}
	byKey := make(map[string]designRecipe, len(d.Recipes))
	for _, rc := range d.Recipes {
		byKey[rc.RecipeKey] = rc
	}
	b := importer.Bundle{SchemaVersion: importer.SchemaVersion, Regions: d.Regions, Dishes: d.Dishes}
	for i := range b.Dishes {
		recipes, err := convertDish(&b.Dishes[i], byKey)
		if err != nil {
			return importer.Bundle{}, err
		}
		b.Recipes = append(b.Recipes, recipes...)
	}
	return b, nil
}

// checkRecipesMatched fails on a design recipe no variation names, which
// would otherwise leave that variation with a placeholder.
func checkRecipesMatched(d design) error {
	named := map[string]bool{}
	for _, dish := range d.Dishes {
		for _, v := range dish.Variations {
			named[v.RecipeKey] = true
		}
	}
	for _, rc := range d.Recipes {
		if !named[rc.RecipeKey] {
			return fmt.Errorf("design recipe %q matches no variation", rc.RecipeKey)
		}
	}
	return nil
}

func convertDish(dish *importer.Dish, byKey map[string]designRecipe) ([]importer.Recipe, error) {
	recipes := make([]importer.Recipe, 0, len(dish.Variations))
	for j := range dish.Variations {
		v := &dish.Variations[j]
		designKey := v.RecipeKey
		v.RecipeKey = dish.Key + "-" + v.RegionKey
		dr, ok := byKey[designKey]
		if !ok {
			recipes = append(recipes, placeholder(v.RecipeKey))
			continue
		}
		rc, err := convertRecipe(v.RecipeKey, dr)
		if err != nil {
			return nil, fmt.Errorf("recipe %q: %w", designKey, err)
		}
		recipes = append(recipes, rc)
	}
	return recipes, nil
}

func placeholder(key string) importer.Recipe {
	return importer.Recipe{
		RecipeKey:    key,
		TimeMinutes:  placeholderMinutes,
		BaseServings: placeholderServings,
		Ingredients:  []importer.Ingredient{{Quantity: 1, Name: placeholderIngredient}},
		Steps:        []importer.Text{placeholderStep},
	}
}

func convertRecipe(key string, dr designRecipe) (importer.Recipe, error) {
	minutes, note, err := parseTime(dr.Time)
	if err != nil {
		return importer.Recipe{}, err
	}
	steps, err := zipSteps(dr.Steps)
	if err != nil {
		return importer.Recipe{}, err
	}
	return importer.Recipe{
		RecipeKey:    key,
		TimeMinutes:  minutes,
		TimeNote:     note,
		BaseServings: dr.BaseServings,
		Ingredients:  dr.Ingredients,
		Steps:        steps,
	}, nil
}

func zipSteps(byLocale map[string][]string) ([]importer.Text, error) {
	en, es, ru := byLocale["en"], byLocale["es"], byLocale["ru"]
	if len(es) != len(en) || len(ru) != len(en) {
		return nil, fmt.Errorf("steps: %d en, %d es, %d ru", len(en), len(es), len(ru))
	}
	steps := make([]importer.Text, len(en))
	for i := range en {
		steps[i] = importer.Text{En: en[i], Es: es[i], Ru: ru[i]}
	}
	return steps, nil
}

// parseTime reads the English duration ("1 h 15 min") and, after a "+" in
// every locale, the time note ("+ soaking").
func parseTime(t importer.Text) (int, *importer.Text, error) {
	duration, _, hasNote := strings.Cut(t.En, "+")
	minutes, err := parseDuration(duration)
	if err != nil {
		return 0, nil, fmt.Errorf("time %q: %w", t.En, err)
	}
	if !hasNote {
		return minutes, nil, nil
	}
	note, err := timeNote(t)
	if err != nil {
		return 0, nil, err
	}
	return minutes, &note, nil
}

func parseDuration(s string) (int, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 || len(fields)%2 != 0 {
		return 0, fmt.Errorf("want <n> h and/or <n> min")
	}
	total := 0
	for i := 0; i < len(fields); i += 2 {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			return 0, fmt.Errorf("number %q: %w", fields[i], err)
		}
		switch fields[i+1] {
		case "h":
			total += n * minutesPerHour
		case "min":
			total += n
		default:
			return 0, fmt.Errorf("unit %q, want h or min", fields[i+1])
		}
	}
	return total, nil
}

func timeNote(t importer.Text) (importer.Text, error) {
	var notes [3]string
	for i, s := range []string{t.En, t.Es, t.Ru} {
		_, note, ok := strings.Cut(s, "+")
		if !ok {
			return importer.Text{}, fmt.Errorf("time %q has no note while the English one does", s)
		}
		notes[i] = "+ " + strings.TrimSpace(note)
	}
	return importer.Text{En: notes[0], Es: notes[1], Ru: notes[2]}, nil
}
