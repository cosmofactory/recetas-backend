package importer_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

func TestValidate_AcceptsValidBundles(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
	}{
		{"builder bundle", func(*importer.Bundle) {}},
		{"one serving", func(b *importer.Bundle) { b.Recipes[0].BaseServings = 1 }},
		{"twenty servings", func(b *importer.Bundle) { b.Recipes[0].BaseServings = 20 }},
		{"one minute", func(b *importer.Bundle) { b.Recipes[0].TimeMinutes = 1 }},
		{"fractional quantity", func(b *importer.Bundle) { b.Recipes[0].Ingredients[0].Quantity = 0.25 }},
		{"lowercase tile color", func(b *importer.Bundle) { b.Regions[0].TileColor = "#9a9a5e" }},
		{"hyphenated keys", func(b *importer.Bundle) {
			b.Dishes[1].Key = "cocido-y-guisos"
			b.Dishes[1].Variations[0].RecipeKey = "cocido-2"
			b.Recipes[2].RecipeKey = "cocido-2"
		}},
		{"some photo roles missing", func(b *importer.Bundle) {
			b.Dishes[0].Photos.Card = nil
			b.Dishes[0].Variations[0].Photos.Thumb = nil
		}},
		{"photos object with no roles", func(b *importer.Bundle) { b.Dishes[1].Photos = &importer.Photos{} }},
		{"no regional dishes", func(b *importer.Bundle) { b.Regions[2].RegionalDishes = nil }},
		{"region without variations", func(b *importer.Bundle) {
			b.Regions = append(b.Regions, region("can", "#A8632F"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := validBundle()
			tt.edit(&b)
			if err := importer.Validate(b); err != nil {
				t.Errorf("got %v, want nil", err)
			}
		})
	}
}

func TestValidate_DesignBundleIsValid(t *testing.T) {
	if err := importer.Validate(readDesignBundle(t)); err != nil {
		t.Errorf("got %v, want nil", err)
	}
}

func TestValidate_RejectsWithPointers(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
		want []string // sorted
	}{
		// schema version
		{"newer schema version", func(b *importer.Bundle) { b.SchemaVersion = importer.SchemaVersion + 1 },
			[]string{"/schemaVersion"}},
		{"zero schema version", func(b *importer.Bundle) { b.SchemaVersion = 0 },
			[]string{"/schemaVersion"}},

		// keys
		{"duplicate region key", func(b *importer.Bundle) { b.Regions = append(b.Regions, region("gal", "#000000")) },
			[]string{"/regions/3/key"}},
		{"region key not a slug", func(b *importer.Bundle) { b.Regions = append(b.Regions, region("Bad Key", "#000000")) },
			[]string{"/regions/3/key"}},
		{"duplicate dish key", func(b *importer.Bundle) { appendDish(b, "tortilla", "tortilla-madrid") },
			[]string{"/dishes/2/key"}},
		{"dish key not a slug", func(b *importer.Bundle) { b.Dishes[1].Key = "Cocido" },
			[]string{"/dishes/1/key"}},
		{"recipe key not a slug", func(b *importer.Bundle) {
			b.Dishes[0].Variations[1].RecipeKey = "With chorizo"
			b.Recipes[1].RecipeKey = "With chorizo"
		}, []string{"/dishes/0/variations/1/recipeKey", "/recipes/1/recipeKey"}},
		{"duplicate recipe key", func(b *importer.Bundle) { b.Recipes = append(b.Recipes, recipe("tortilla-betanzos")) },
			[]string{"/recipes/4/recipeKey"}},

		// references
		{"unknown variation region", func(b *importer.Bundle) { b.Dishes[0].Variations[1].RegionKey = "xyz" },
			[]string{"/dishes/0/variations/1/regionKey"}},
		{"two variations of a dish in one region", func(b *importer.Bundle) { b.Dishes[1].Variations[1].RegionKey = "mad" },
			[]string{"/dishes/1/variations/1/regionKey"}},
		{"variation recipe missing", func(b *importer.Bundle) { b.Recipes = slices.Delete(b.Recipes, 1, 2) },
			[]string{"/dishes/0/variations/1/recipeKey"}},
		{"recipe not referenced", func(b *importer.Bundle) { b.Recipes = append(b.Recipes, recipe("orphan")) },
			[]string{"/recipes/4/recipeKey"}},
		{"recipe referenced twice", func(b *importer.Bundle) { b.Dishes[1].Variations[1].RecipeKey = "cocido-madrileno" },
			[]string{"/dishes/1/variations/1/recipeKey", "/recipes/3/recipeKey"}},

		// recipe values
		{"unit kg", func(b *importer.Bundle) { b.Recipes[0].Ingredients[1].Unit = strp("kg") },
			[]string{"/recipes/0/ingredients/1/unit"}},
		{"unit uppercase", func(b *importer.Bundle) { b.Recipes[0].Ingredients[1].Unit = strp("G") },
			[]string{"/recipes/0/ingredients/1/unit"}},
		{"unit empty string", func(b *importer.Bundle) { b.Recipes[0].Ingredients[0].Unit = strp("") },
			[]string{"/recipes/0/ingredients/0/unit"}},
		{"zero servings", func(b *importer.Bundle) { b.Recipes[0].BaseServings = 0 },
			[]string{"/recipes/0/baseServings"}},
		{"twenty-one servings", func(b *importer.Bundle) { b.Recipes[2].BaseServings = 21 },
			[]string{"/recipes/2/baseServings"}},
		{"zero minutes", func(b *importer.Bundle) { b.Recipes[0].TimeMinutes = 0 },
			[]string{"/recipes/0/timeMinutes"}},
		{"negative minutes", func(b *importer.Bundle) { b.Recipes[3].TimeMinutes = -5 },
			[]string{"/recipes/3/timeMinutes"}},
		{"zero quantity", func(b *importer.Bundle) { b.Recipes[0].Ingredients[0].Quantity = 0 },
			[]string{"/recipes/0/ingredients/0/quantity"}},
		{"negative quantity", func(b *importer.Bundle) { b.Recipes[1].Ingredients[3].Quantity = -1 },
			[]string{"/recipes/1/ingredients/3/quantity"}},
		{"quantity rounding to zero", func(b *importer.Bundle) { b.Recipes[0].Ingredients[1].Quantity = 1e-10 },
			[]string{"/recipes/0/ingredients/1/quantity"}},
		{"no ingredients", func(b *importer.Bundle) { b.Recipes[0].Ingredients = nil },
			[]string{"/recipes/0/ingredients"}},
		{"no steps", func(b *importer.Bundle) { b.Recipes[2].Steps = []importer.Text{} },
			[]string{"/recipes/2/steps"}},

		// tile color
		{"short tile color", func(b *importer.Bundle) { b.Regions[0].TileColor = "#ABC" },
			[]string{"/regions/0/tileColor"}},
		{"tile color without hash", func(b *importer.Bundle) { b.Regions[1].TileColor = "9A9A5E" },
			[]string{"/regions/1/tileColor"}},
		{"tile color not hex", func(b *importer.Bundle) { b.Regions[2].TileColor = "#GGGGGG" },
			[]string{"/regions/2/tileColor"}},
		{"tile color empty", func(b *importer.Bundle) { b.Regions[0].TileColor = "" },
			[]string{"/regions/0/tileColor"}},

		// translations
		{"region name en", func(b *importer.Bundle) { b.Regions[0].Name.En = "" },
			[]string{"/regions/0/name/en"}},
		{"region map label es", func(b *importer.Bundle) { b.Regions[1].MapLabel.Es = "" },
			[]string{"/regions/1/mapLabel/es"}},
		{"region description ru", func(b *importer.Bundle) { b.Regions[2].Description.Ru = "" },
			[]string{"/regions/2/description/ru"}},
		{"dish name ru", func(b *importer.Bundle) { b.Dishes[0].Name.Ru = "" },
			[]string{"/dishes/0/name/ru"}},
		{"dish description en", func(b *importer.Bundle) { b.Dishes[1].Description.En = "" },
			[]string{"/dishes/1/description/en"}},
		{"variation name es", func(b *importer.Bundle) { b.Dishes[0].Variations[1].Name.Es = "" },
			[]string{"/dishes/0/variations/1/name/es"}},
		{"variation description ru", func(b *importer.Bundle) { b.Dishes[1].Variations[0].Description.Ru = "" },
			[]string{"/dishes/1/variations/0/description/ru"}},
		{"time note ru", func(b *importer.Bundle) { b.Recipes[3].TimeNote.Ru = "" },
			[]string{"/recipes/3/timeNote/ru"}},
		{"ingredient name en", func(b *importer.Bundle) { b.Recipes[0].Ingredients[2].Name.En = "" },
			[]string{"/recipes/0/ingredients/2/name/en"}},
		{"step ru", func(b *importer.Bundle) { b.Recipes[0].Steps[1].Ru = "" },
			[]string{"/recipes/0/steps/1/ru"}},
		{"whitespace-only text", func(b *importer.Bundle) { b.Dishes[0].Name.Es = " \t\n" },
			[]string{"/dishes/0/name/es"}},
		{"every locale of one text", func(b *importer.Bundle) { b.Recipes[1].Steps[0] = importer.Text{} },
			[]string{"/recipes/1/steps/0/en", "/recipes/1/steps/0/es", "/recipes/1/steps/0/ru"}},

		// photos
		{"dish photo of another dish", func(b *importer.Bundle) { b.Dishes[0].Photos.Thumb = strp("dishes/cocido/thumb-1a2b3c4d") },
			[]string{"/dishes/0/photos/thumb"}},
		{"dish photo in the wrong role", func(b *importer.Bundle) { b.Dishes[0].Photos.Card = strp("dishes/tortilla/thumb-1a2b3c4d") },
			[]string{"/dishes/0/photos/card"}},
		{"dish photo with a region", func(b *importer.Bundle) { b.Dishes[0].Photos.Hero = strp("dishes/tortilla/gal/hero-1a2b3c4d") },
			[]string{"/dishes/0/photos/hero"}},
		{"variation photo without its region", func(b *importer.Bundle) {
			b.Dishes[0].Variations[0].Photos.Card = strp("dishes/tortilla/card-1a2b3c4d")
		}, []string{"/dishes/0/variations/0/photos/card"}},
		{"variation photo of another region", func(b *importer.Bundle) {
			b.Dishes[0].Variations[0].Photos.Hero = strp("dishes/tortilla/ast/hero-1a2b3c4d")
		}, []string{"/dishes/0/variations/0/photos/hero"}},
		{"variation photo of another dish", func(b *importer.Bundle) {
			b.Dishes[0].Variations[0].Photos.Thumb = strp("dishes/cocido/gal/thumb-1a2b3c4d")
		}, []string{"/dishes/0/variations/0/photos/thumb"}},
		{"photo hash too short", func(b *importer.Bundle) { b.Dishes[0].Photos.Thumb = strp("dishes/tortilla/thumb-1a2b3c4") },
			[]string{"/dishes/0/photos/thumb"}},
		{"photo hash uppercase", func(b *importer.Bundle) { b.Dishes[0].Photos.Thumb = strp("dishes/tortilla/thumb-1A2B3C4D") },
			[]string{"/dishes/0/photos/thumb"}},
		{"photo with a file extension", func(b *importer.Bundle) { b.Dishes[0].Photos.Card = strp("dishes/tortilla/card-5e6f7a8b.jpg") },
			[]string{"/dishes/0/photos/card"}},
		{"photo with a leading slash", func(b *importer.Bundle) { b.Dishes[0].Photos.Card = strp("/dishes/tortilla/card-5e6f7a8b") },
			[]string{"/dishes/0/photos/card"}},
		{"photo empty string", func(b *importer.Bundle) { b.Dishes[0].Photos.Hero = strp("") },
			[]string{"/dishes/0/photos/hero"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := validBundle()
			tt.edit(&b)
			got := sortedPointers(t, validationProblems(t, importer.Validate(b)))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got pointers %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidate_RejectsBadSlugs(t *testing.T) {
	for _, key := range []string{
		"", "Gal", "ga l", "gal-", "-gal", "ga--l", "ga_l", "andalucía", "gal/ast", strings.Repeat("a", 65),
	} {
		t.Run(key, func(t *testing.T) {
			b := validBundle()
			b.Regions = append(b.Regions, region(key, "#000000"))
			got := sortedPointers(t, validationProblems(t, importer.Validate(b)))
			if want := []string{"/regions/3/key"}; !reflect.DeepEqual(got, want) {
				t.Errorf("got pointers %q, want %q", got, want)
			}
		})
	}
}

func TestValidate_CollectsEveryProblem(t *testing.T) {
	b := validBundle()
	b.Regions[1].TileColor = "red"
	b.Dishes[0].Variations[1].RegionKey = "xyz"
	b.Recipes[2].BaseServings = 0
	b.Recipes[3].Steps[0].Es = ""

	got := sortedPointers(t, validationProblems(t, importer.Validate(b)))
	want := []string{
		"/dishes/0/variations/1/regionKey",
		"/recipes/2/baseServings",
		"/recipes/3/steps/0/es",
		"/regions/1/tileColor",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got pointers %q, want %q", got, want)
	}
}

func TestValidationError_ErrorListsPointers(t *testing.T) {
	b := validBundle()
	b.Regions[1].TileColor = "red"
	b.Recipes[2].BaseServings = 0
	err := importer.Validate(b)
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	for _, pointer := range []string{"/regions/1/tileColor", "/recipes/2/baseServings"} {
		if !strings.Contains(err.Error(), pointer) {
			t.Errorf("got %q, want it to mention %q", err.Error(), pointer)
		}
	}
}

// appendDish adds a dish with one variation in mad and its recipe.
func appendDish(b *importer.Bundle, key, recipeKey string) {
	d := cocidoDish()
	d.Key = key
	d.Variations = []importer.Variation{variation("mad", recipeKey)}
	b.Dishes = append(b.Dishes, d)
	b.Recipes = append(b.Recipes, recipe(recipeKey))
}
