package catalogtest_test

import (
	"strconv"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogtest"
)

func TestDataIsCompleteInEveryLocale(t *testing.T) {
	f := catalogtest.Data()
	for _, r := range f.Regions {
		checkRegion(t, r)
	}
	for _, d := range f.Dishes {
		checkLocales(t, d.Key+" name", d.Name)
		checkLocales(t, d.Key+" description", d.Description)
		for _, v := range d.Variations {
			checkVariation(t, v)
		}
	}
}

func checkLocales(t *testing.T, what string, text catalogtest.Text) {
	t.Helper()
	if len(text) != len(catalogtest.Locales) {
		t.Errorf("%s: got %d locales, want %d", what, len(text), len(catalogtest.Locales))
	}
	for _, loc := range catalogtest.Locales {
		if text[loc] == "" {
			t.Errorf("%s: missing %q", what, loc)
		}
	}
}

func checkRegion(t *testing.T, r catalogtest.Region) {
	t.Helper()
	checkLocales(t, r.Key+" name", r.Name)
	checkLocales(t, r.Key+" mapLabel", r.MapLabel)
	checkLocales(t, r.Key+" description", r.Description)
	if len(r.RegionalDishes) == 0 {
		t.Errorf("%s: got no regional dishes, want some", r.Key)
	}
}

func checkVariation(t *testing.T, v catalogtest.Variation) {
	t.Helper()
	checkLocales(t, v.RecipeKey+" name", v.Name)
	checkLocales(t, v.RecipeKey+" description", v.Description)
	if v.Recipe.TimeNote != nil {
		checkLocales(t, v.RecipeKey+" time note", *v.Recipe.TimeNote)
	}
	for i, ing := range v.Recipe.Ingredients {
		checkLocales(t, v.RecipeKey+" ingredient "+strconv.Itoa(i+1), ing.Name)
	}
	for i, s := range v.Recipe.Steps {
		checkLocales(t, v.RecipeKey+" step "+strconv.Itoa(i+1), s)
	}
}

// Only the tortilla dish and its Betanzos variation have photos, so the
// round-trip covers both set and NULL columns.
func TestDataPhotosOnTortillaAndBetanzosOnly(t *testing.T) {
	f := catalogtest.Data()
	for _, d := range f.Dishes {
		checkPhotos(t, "dish "+d.Key, d.Photos, d.Key == "tortilla")
		for _, v := range d.Variations {
			checkPhotos(t, "variation "+v.RecipeKey, v.Photos, v.RecipeKey == "tortilla-betanzos")
		}
	}
}

func checkPhotos(t *testing.T, what string, p catalogtest.Photos, wantSet bool) {
	t.Helper()
	for role, key := range map[string]*string{"thumb": p.Thumb, "card": p.Card, "hero": p.Hero} {
		if (key != nil) != wantSet {
			t.Errorf("%s %s photo: got set=%v, want set=%v", what, role, key != nil, wantSet)
		}
	}
}
