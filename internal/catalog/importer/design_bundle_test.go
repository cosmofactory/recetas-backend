package importer_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// designRecipe is one of the seven recipes the design content.json has.
// variation is the region key, or esp/<slug> for a Spanish variation.
type designRecipe struct {
	dish, variation  string
	timeMinutes      int
	timeNote         *importer.Text
	baseServings     int
	firstIngredient  importer.Ingredient
	ingredientsCount int
	stepsCount       int
}

var (
	eggs   = importer.Text{En: "Eggs", Es: "Huevos", Ru: "Яйца"}
	garlic = importer.Text{En: "Garlic cloves", Es: "Dientes de ajo", Ru: "Зубчики чеснока"}
)

var designRecipes = []designRecipe{
	{"tortilla", "gal", 25, nil, 2, importer.Ingredient{Quantity: 4, Name: eggs}, 4, 5},
	{"tortilla", "mad", 50, nil, 6, importer.Ingredient{Quantity: 8, Name: eggs}, 6, 5},
	{"paella", "val", 75, nil, 4, importer.Ingredient{Quantity: 400, Unit: strp("g"), Name: importer.Text{En: "Bomba rice", Es: "Arroz bomba", Ru: "Рис бомба"}}, 9, 5},
	{"cocido", "ast", 180, &importer.Text{En: "+ soaking", Es: "+ remojo", Ru: "+ замачивание"}, 4,
		importer.Ingredient{Quantity: 500, Unit: strp("g"), Name: importer.Text{En: "Fabes de la Granja", Es: "Fabes de la Granja", Ru: "Фасоль фабес-де-ла-гранха"}}, 5, 5},
	{"allioli", "esp/allioli-de-morter", 15, nil, 4, importer.Ingredient{Quantity: 4, Name: garlic}, 3, 3},
	{"allioli", "esp/allioli-de-codony", 60, nil, 6, importer.Ingredient{Quantity: 300, Unit: strp("g"), Name: importer.Text{En: "Quince", Es: "Membrillo", Ru: "Айва"}}, 4, 3},
	{"arrozblanco", "esp/arroz-blanco-con-tomate", 30, nil, 4,
		importer.Ingredient{Quantity: 300, Unit: strp("g"), Name: importer.Text{En: "Long-grain rice", Es: "Arroz largo", Ru: "Длиннозёрный рис"}}, 6, 3},
}

func TestDesignBundle_Shape(t *testing.T) {
	b := readDesignBundle(t)
	if b.SchemaVersion != importer.SchemaVersion {
		t.Errorf("got schemaVersion %d, want %d", b.SchemaVersion, importer.SchemaVersion)
	}
	wantRegions := []string{"gal", "ast", "cnb", "bas", "nav", "rio", "cat", "cyl", "mad", "ara", "val", "ext", "clm", "mur", "and", "bal", "can"}
	if got := regionKeysOf(b); !reflect.DeepEqual(got, wantRegions) {
		t.Errorf("got region keys %q, want %q", got, wantRegions)
	}
	wantDishes := []string{"tortilla", "paella", "cocido", "gazpacho", "croquetas", "torrijas", "arrozleche", "allioli", "arrozblanco"}
	if got := dishKeysOf(b); !reflect.DeepEqual(got, wantDishes) {
		t.Errorf("got dish keys %q, want %q", got, wantDishes)
	}
	if got := len(allVariations(b)); got != 57 {
		t.Errorf("got %d variations, want 57", got)
	}
	if got := len(b.Recipes); got != 57 {
		t.Errorf("got %d recipes, want 57", got)
	}
}

func TestDesignBundle_KeepsDesignTexts(t *testing.T) {
	b := readDesignBundle(t)
	if got, want := b.Regions[3].Name, (importer.Text{En: "Basque Country", Es: "País Vasco", Ru: "Страна Басков"}); got != want {
		t.Errorf("bas name: got %+v, want %+v", got, want)
	}
	if got, want := b.Regions[0].RegionalDishes, []string{"Pulpo á feira", "Empanada gallega", "Lacón con grelos"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gal regional dishes: got %q, want %q", got, want)
	}
	tortilla := b.Dishes[0]
	if want := (importer.Flags{Vegetarian: true, Quick: true}); tortilla.Flags != want || tortilla.Course != "tapas" || tortilla.Season != "all" {
		t.Errorf("tortilla: got %+v %s %s, want %+v tapas all", tortilla.Flags, tortilla.Course, tortilla.Season, want)
	}
	if got, want := tortilla.Variations[0].Name, (importer.Text{En: "Betanzos-style runny", Es: "Estilo Betanzos, jugosa", Ru: "По-бетансосски, жидкая внутри"}); got != want {
		t.Errorf("tortilla/gal name: got %+v, want %+v", got, want)
	}
}

func TestDesignBundle_HasNoPhotos(t *testing.T) {
	b := readDesignBundle(t)
	for _, d := range b.Dishes {
		if d.Photos != nil {
			t.Errorf("dish %s: got photos %+v, want nil", d.Key, d.Photos)
		}
	}
	for _, v := range allVariations(b) {
		if v.Photos != nil {
			t.Errorf("variation %s: got photos %+v, want nil", v.RecipeKey, v.Photos)
		}
	}
}

func TestDesignBundle_RecipeKeysAreSlugs(t *testing.T) {
	for _, rc := range readDesignBundle(t).Recipes {
		if !slugPattern.MatchString(rc.RecipeKey) {
			t.Errorf("got recipe key %q, want a slug", rc.RecipeKey)
		}
	}
}

func TestDesignBundle_RealRecipes(t *testing.T) {
	b := readDesignBundle(t)
	for _, want := range designRecipes {
		t.Run(want.dish+"/"+want.variation, func(t *testing.T) {
			rc := recipeOf(t, b, want.dish, want.variation)
			if rc.TimeMinutes != want.timeMinutes || rc.BaseServings != want.baseServings {
				t.Errorf("got %d min for %d, want %d min for %d", rc.TimeMinutes, rc.BaseServings, want.timeMinutes, want.baseServings)
			}
			if !reflect.DeepEqual(rc.TimeNote, want.timeNote) {
				t.Errorf("got time note %+v, want %+v", rc.TimeNote, want.timeNote)
			}
			if len(rc.Ingredients) != want.ingredientsCount || len(rc.Steps) != want.stepsCount {
				t.Fatalf("got %d ingredients and %d steps, want %d and %d", len(rc.Ingredients), len(rc.Steps), want.ingredientsCount, want.stepsCount)
			}
			if !reflect.DeepEqual(rc.Ingredients[0], want.firstIngredient) {
				t.Errorf("got first ingredient %+v, want %+v", rc.Ingredients[0], want.firstIngredient)
			}
		})
	}
}

// Every variation without a design recipe gets a placeholder that says so.
func TestDesignBundle_PlaceholderRecipes(t *testing.T) {
	b := readDesignBundle(t)
	designed := map[string]bool{}
	for _, r := range designRecipes {
		designed[recipeOf(t, b, r.dish, r.variation).RecipeKey] = true
	}
	placeholders := 0
	for _, rc := range b.Recipes {
		if designed[rc.RecipeKey] {
			continue
		}
		placeholders++
		if !isPlaceholder(rc) {
			t.Errorf("recipe %s: got steps %+v, want a step marked as a placeholder", rc.RecipeKey, rc.Steps)
		}
	}
	if placeholders != 50 {
		t.Errorf("got %d placeholder recipes, want 50", placeholders)
	}
}

func isPlaceholder(rc importer.Recipe) bool {
	for _, s := range rc.Steps {
		if strings.Contains(strings.ToLower(s.En), "placeholder") {
			return true
		}
	}
	return false
}

func regionKeysOf(b importer.Bundle) []string {
	keys := make([]string, 0, len(b.Regions))
	for _, r := range b.Regions {
		keys = append(keys, r.Key)
	}
	return keys
}

func dishKeysOf(b importer.Bundle) []string {
	keys := make([]string, 0, len(b.Dishes))
	for _, d := range b.Dishes {
		keys = append(keys, d.Key)
	}
	return keys
}

func allVariations(b importer.Bundle) []importer.Variation {
	var vs []importer.Variation
	for _, d := range b.Dishes {
		vs = append(vs, d.Variations...)
	}
	return vs
}

// recipeOf returns the recipe of dishKey's variation named by variationID.
func recipeOf(t *testing.T, b importer.Bundle, dishKey, id string) importer.Recipe {
	t.Helper()
	recipeKey := variationOf(t, b, dishKey, id).RecipeKey
	for _, rc := range b.Recipes {
		if rc.RecipeKey == recipeKey {
			return rc
		}
	}
	t.Fatalf("no recipe %q for %s/%s", recipeKey, dishKey, id)
	return importer.Recipe{}
}

// variationOf returns dishKey's variation named by variationID.
func variationOf(t *testing.T, b importer.Bundle, dishKey, id string) importer.Variation {
	t.Helper()
	for _, v := range variationsOfDish(b, dishKey) {
		if variationID(v) == id {
			return v
		}
	}
	t.Fatalf("no variation %s/%s", dishKey, id)
	return importer.Variation{}
}

// variationID is a regional variation's region key, or esp/<slug> for a
// Spanish one.
func variationID(v importer.Variation) string {
	if v.RegionKey == spainWideKey {
		return spainWideKey + "/" + v.VariationSlug
	}
	return v.RegionKey
}

func variationsOfDish(b importer.Bundle, dishKey string) []importer.Variation {
	for _, d := range b.Dishes {
		if d.Key == dishKey {
			return d.Variations
		}
	}
	return nil
}

// The design's two Spanish samples convert with their slugs and
// <dish>-esp-<slug> recipe keys, in design order (D-ESP-2).
func TestDesignBundle_SpainWideSamples(t *testing.T) {
	b := readDesignBundle(t)
	tests := []struct {
		dish   string
		course string
		want   []string // regionKey/variationSlug/recipeKey, in bundle order
	}{
		{"allioli", "basics", []string{
			"esp/allioli-de-morter/allioli-esp-allioli-de-morter",
			"esp/allioli-de-codony/allioli-esp-allioli-de-codony",
		}},
		{"arrozblanco", "mains", []string{
			"esp/arroz-blanco-con-tomate/arrozblanco-esp-arroz-blanco-con-tomate",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.dish, func(t *testing.T) {
			d := dishOf(t, b, tt.dish)
			if d.Course != tt.course {
				t.Errorf("course: got %q, want %q", d.Course, tt.course)
			}
			var got []string
			for _, v := range d.Variations {
				got = append(got, v.RegionKey+"/"+v.VariationSlug+"/"+v.RecipeKey)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("variations: got %q, want %q", got, tt.want)
			}
		})
	}
}

// Only Spanish variations carry a slug.
func TestDesignBundle_RegionalVariationsHaveNoSlug(t *testing.T) {
	for _, v := range allVariations(readDesignBundle(t)) {
		if v.RegionKey != spainWideKey && v.VariationSlug != "" {
			t.Errorf("variation %s: got slug %q, want none", v.RecipeKey, v.VariationSlug)
		}
	}
}

func dishOf(t *testing.T, b importer.Bundle, key string) importer.Dish {
	t.Helper()
	for _, d := range b.Dishes {
		if d.Key == key {
			return d
		}
	}
	t.Fatalf("no dish %s", key)
	return importer.Dish{}
}
