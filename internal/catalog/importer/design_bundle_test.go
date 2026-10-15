package importer_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// designRecipe is one of the four recipes the design content.json has.
type designRecipe struct {
	dish, region     string
	timeMinutes      int
	timeNote         *importer.Text
	baseServings     int
	firstIngredient  importer.Ingredient
	ingredientsCount int
}

var designRecipes = []designRecipe{
	{"tortilla", "gal", 25, nil, 2, importer.Ingredient{Quantity: 4, Name: importer.Text{En: "Eggs", Es: "Huevos", Ru: "Яйца"}}, 4},
	{"tortilla", "mad", 50, nil, 6, importer.Ingredient{Quantity: 8, Name: importer.Text{En: "Eggs", Es: "Huevos", Ru: "Яйца"}}, 6},
	{"paella", "val", 75, nil, 4, importer.Ingredient{Quantity: 400, Unit: strp("g"), Name: importer.Text{En: "Bomba rice", Es: "Arroz bomba", Ru: "Рис бомба"}}, 9},
	{"cocido", "ast", 180, &importer.Text{En: "+ soaking", Es: "+ remojo", Ru: "+ замачивание"}, 4,
		importer.Ingredient{Quantity: 500, Unit: strp("g"), Name: importer.Text{En: "Fabes de la Granja", Es: "Fabes de la Granja", Ru: "Фасоль фабес-де-ла-гранха"}}, 5},
}

func TestDesignBundle_Shape(t *testing.T) {
	b := readDesignBundle(t)
	if b.SchemaVersion != importer.SchemaVersion {
		t.Errorf("got schemaVersion %d, want %d", b.SchemaVersion, importer.SchemaVersion)
	}
	wantRegions := []string{"gal", "ast", "bas", "cat", "cyl", "mad", "ara", "val", "ext", "clm", "mur", "and", "bal", "can"}
	if got := regionKeysOf(b); !reflect.DeepEqual(got, wantRegions) {
		t.Errorf("got region keys %q, want %q", got, wantRegions)
	}
	wantDishes := []string{"tortilla", "paella", "cocido", "gazpacho", "croquetas", "torrijas", "arrozleche"}
	if got := dishKeysOf(b); !reflect.DeepEqual(got, wantDishes) {
		t.Errorf("got dish keys %q, want %q", got, wantDishes)
	}
	if got := len(allVariations(b)); got != 54 {
		t.Errorf("got %d variations, want 54", got)
	}
	if got := len(b.Recipes); got != 54 {
		t.Errorf("got %d recipes, want 54", got)
	}
}

func TestDesignBundle_KeepsDesignTexts(t *testing.T) {
	b := readDesignBundle(t)
	if got, want := b.Regions[2].Name, (importer.Text{En: "Basque Country", Es: "País Vasco", Ru: "Страна Басков"}); got != want {
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
		t.Run(want.dish+"/"+want.region, func(t *testing.T) {
			rc := recipeOf(t, b, want.dish, want.region)
			if rc.TimeMinutes != want.timeMinutes || rc.BaseServings != want.baseServings {
				t.Errorf("got %d min for %d, want %d min for %d", rc.TimeMinutes, rc.BaseServings, want.timeMinutes, want.baseServings)
			}
			if !reflect.DeepEqual(rc.TimeNote, want.timeNote) {
				t.Errorf("got time note %+v, want %+v", rc.TimeNote, want.timeNote)
			}
			if len(rc.Ingredients) != want.ingredientsCount || len(rc.Steps) != 5 {
				t.Fatalf("got %d ingredients and %d steps, want %d and 5", len(rc.Ingredients), len(rc.Steps), want.ingredientsCount)
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
		designed[recipeOf(t, b, r.dish, r.region).RecipeKey] = true
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

// recipeOf returns the recipe of dishKey's variation in regionKey.
func recipeOf(t *testing.T, b importer.Bundle, dishKey, regionKey string) importer.Recipe {
	t.Helper()
	recipeKey := variationOf(t, b, dishKey, regionKey).RecipeKey
	for _, rc := range b.Recipes {
		if rc.RecipeKey == recipeKey {
			return rc
		}
	}
	t.Fatalf("no recipe %q for %s/%s", recipeKey, dishKey, regionKey)
	return importer.Recipe{}
}

func variationOf(t *testing.T, b importer.Bundle, dishKey, regionKey string) importer.Variation {
	t.Helper()
	for _, v := range variationsOfDish(b, dishKey) {
		if v.RegionKey == regionKey {
			return v
		}
	}
	t.Fatalf("no variation %s/%s", dishKey, regionKey)
	return importer.Variation{}
}

func variationsOfDish(b importer.Bundle, dishKey string) []importer.Variation {
	for _, d := range b.Dishes {
		if d.Key == dishKey {
			return d.Variations
		}
	}
	return nil
}
