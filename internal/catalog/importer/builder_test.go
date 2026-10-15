package importer_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// designBundlePath is the design content converted to the bundle format.
const designBundlePath = "../../../testdata/content.design.json"

// Entities keyed in Summary.Changes.
var entities = []string{"regions", "dishes", "variations", "recipes"}

// txt is a complete translation whose locales differ, so a mix-up shows.
func txt(en string) importer.Text {
	return importer.Text{En: en, Es: en + " (es)", Ru: en + " (ru)"}
}

func strp(s string) *string { return &s }

// validBundle is a small, valid bundle:
//
//	regions gal, ast, mad
//	dishes  tortilla [gal tortilla-betanzos (photos), ast tortilla-chorizo]
//	        cocido   [mad cocido-madrileno, ast fabada-asturiana]
//	recipes in that order; fabada has a time note.
func validBundle() importer.Bundle {
	return importer.Bundle{
		SchemaVersion: importer.SchemaVersion,
		Regions: []importer.Region{
			region("gal", "#9A9A5E"), region("ast", "#C97B4A"), region("mad", "#A8632F"),
		},
		Dishes: []importer.Dish{tortillaDish(), cocidoDish()},
		Recipes: []importer.Recipe{
			recipe("tortilla-betanzos"), recipe("tortilla-chorizo"),
			recipe("cocido-madrileno"), fabadaRecipe(),
		},
	}
}

func region(key, tileColor string) importer.Region {
	return importer.Region{
		Key:            key,
		TileColor:      tileColor,
		Name:           txt("Region " + key),
		MapLabel:       txt("Label " + key),
		Description:    txt("About " + key),
		RegionalDishes: []string{"Plato de " + key, "Otro plato"},
	}
}

func tortillaDish() importer.Dish {
	betanzos := variation("gal", "tortilla-betanzos")
	betanzos.Photos = variationPhotos("tortilla", "gal")
	return importer.Dish{
		Key:         "tortilla",
		Name:        txt("Tortilla de patatas"),
		Description: txt("Onion or none"),
		Flags:       importer.Flags{Vegetarian: true, Quick: true},
		Course:      "tapas",
		Season:      "all",
		Photos:      dishPhotos("tortilla"),
		Variations:  []importer.Variation{betanzos, variation("ast", "tortilla-chorizo")},
	}
}

func cocidoDish() importer.Dish {
	return importer.Dish{
		Key:         "cocido",
		Name:        txt("Cocido"),
		Description: txt("Stews"),
		Flags:       importer.Flags{Pork: true, SlowCooked: true},
		Course:      "mains",
		Season:      "winter",
		Variations: []importer.Variation{
			variation("mad", "cocido-madrileno"), variation("ast", "fabada-asturiana"),
		},
	}
}

func variation(regionKey, recipeKey string) importer.Variation {
	return importer.Variation{
		RegionKey:   regionKey,
		RecipeKey:   recipeKey,
		Name:        txt("Variation " + recipeKey),
		Description: txt("One line about " + recipeKey),
	}
}

func dishPhotos(dishKey string) *importer.Photos {
	prefix := "dishes/" + dishKey + "/"
	return &importer.Photos{
		Thumb: strp(prefix + "thumb-1a2b3c4d"),
		Card:  strp(prefix + "card-5e6f7a8b"),
		Hero:  strp(prefix + "hero-9c0d1e2f"),
	}
}

func variationPhotos(dishKey, regionKey string) *importer.Photos {
	prefix := "dishes/" + dishKey + "/" + regionKey + "/"
	return &importer.Photos{
		Thumb: strp(prefix + "thumb-3a4b5c6d"),
		Card:  strp(prefix + "card-7e8f9a0b"),
		Hero:  strp(prefix + "hero-1c2d3e4f0a"),
	}
}

func recipe(key string) importer.Recipe {
	return importer.Recipe{
		RecipeKey:    key,
		TimeMinutes:  25,
		BaseServings: 2,
		Ingredients: []importer.Ingredient{
			{Quantity: 4, Name: txt("Eggs")},
			{Quantity: 400, Unit: strp("g"), Name: txt("Potatoes")},
			{Quantity: 250, Unit: strp("ml"), Name: txt("Olive oil")},
			{Quantity: 1.5, Unit: strp("tsp"), Name: txt("Salt")},
		},
		Steps: []importer.Text{txt("Slice the potatoes."), txt("Fry and serve.")},
	}
}

func fabadaRecipe() importer.Recipe {
	r := recipe("fabada-asturiana")
	r.TimeMinutes = 180
	note := importer.Text{En: "+ soaking", Es: "+ remojo", Ru: "+ замачивание"}
	r.TimeNote = &note
	return r
}

// validationProblems returns the problems of a *ValidationError, failing the
// test for any other error.
func validationProblems(t *testing.T, err error) []importer.Problem {
	t.Helper()
	var verr *importer.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got error %v (%T), want *importer.ValidationError", err, err)
	}
	if len(verr.Problems) == 0 {
		t.Fatal("got a ValidationError with no problems, want at least one")
	}
	return verr.Problems
}

// sortedPointers returns the problems' distinct pointers, sorted (one field may
// carry several messages), failing the test for a problem without a message.
func sortedPointers(t *testing.T, problems []importer.Problem) []string {
	t.Helper()
	pointers := make([]string, 0, len(problems))
	for _, p := range problems {
		if strings.TrimSpace(p.Message) == "" {
			t.Errorf("problem at %q has no message", p.Pointer)
		}
		pointers = append(pointers, p.Pointer)
	}
	slices.Sort(pointers)
	return slices.Compact(pointers)
}

// readDesignBundle parses testdata/content.design.json.
func readDesignBundle(t *testing.T) importer.Bundle {
	t.Helper()
	f, err := os.Open(designBundlePath)
	if err != nil {
		t.Fatalf("open design bundle: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	b, err := importer.Parse(f)
	if err != nil {
		t.Fatalf("Parse design bundle: %v", err)
	}
	return b
}
