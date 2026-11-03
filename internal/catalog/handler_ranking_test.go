package catalog_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
)

func TestHandler_DishesCarryRegionRankPerVariation(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepoWithDishes(scoredDishes("en")))

	rec := srv.get(t, "/v1/dishes?lang=en")

	assertStatus(t, rec, http.StatusOK)
	got := map[string]int{}
	for _, d := range decodeStrict[[]apigen.Dish](t, rec) {
		for _, v := range d.Variations {
			got[v.RecipeKey] = v.RegionRank
		}
	}
	want := map[string]int{}
	for key, s := range fixtureScores {
		want[key] = s.regionRank
	}
	assertDeepEqual(t, "region ranks", got, want)
}

func TestHandler_RegionListsVariationsByRegionRank(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepoWithDishes(scoredDishes("en")))

	rec := srv.get(t, "/v1/regions/ast?lang=en")

	assertStatus(t, rec, http.StatusOK)
	var got []string
	for _, v := range decodeStrict[apigen.RegionDetail](t, rec).Variations {
		got = append(got, v.RecipeKey)
	}
	assertDeepEqual(t, "recipe keys", got, wantScoredRegion["ast"])
}

// The importance score orders lists but is never shown (stack.md X15): no
// response carries it, under any name.
func TestHandler_ScoreIsNeverInAResponse(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepoWithDishes(scoredDishes("en")))
	for _, ep := range endpoints() {
		t.Run(ep.name, func(t *testing.T) {
			rec := srv.get(t, withLang(ep.path, "en"))

			assertStatus(t, rec, http.StatusOK)
			var body any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if key, found := findScoreKey(body); found {
				t.Errorf("got key %q in %s, want no score in any response", key, ep.path)
			}
		})
	}
}

// findScoreKey walks a decoded JSON value for an object key naming a score.
func findScoreKey(v any) (string, bool) {
	switch node := v.(type) {
	case map[string]any:
		return findScoreKeyInObject(node)
	case []any:
		return findScoreKeyInArray(node)
	}
	return "", false
}

func findScoreKeyInArray(items []any) (string, bool) {
	for _, item := range items {
		if key, found := findScoreKey(item); found {
			return key, true
		}
	}
	return "", false
}

func findScoreKeyInObject(obj map[string]any) (string, bool) {
	for key, value := range obj {
		if strings.Contains(strings.ToLower(key), "score") {
			return key, true
		}
		if found, ok := findScoreKey(value); ok {
			return found, true
		}
	}
	return "", false
}
