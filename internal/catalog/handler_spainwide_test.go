package catalog_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
)

// getSpainWideRecipe and the Spanish fields of the other operations (BE-9).
// srv.get validates every response against api/openapi.yaml, so required
// nullable fields must be present as null.

func TestHandler_DishSendsSpanishVariationsWithNullRegion(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newSpainWideFakeRepo())

	rec := srv.get(t, "/v1/dishes/allioli?lang=en")

	assertStatus(t, rec, http.StatusOK)
	assertSameJSON(t, decodeStrict[apigen.Dish](t, rec), apiAllioli(len(fixture().Dishes)+1))
}

func TestHandler_DishListSendsSpanishVariationsWithNullRegion(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newSpainWideFakeRepo())

	rec := srv.get(t, "/v1/dishes?lang=en")

	assertStatus(t, rec, http.StatusOK)
	dishes := decodeStrict[[]apigen.Dish](t, rec)
	if len(dishes) == 0 {
		t.Fatal("got no dishes, want the fixture plus allioli")
	}
	assertSameJSON(t, dishes[len(dishes)-1], apiAllioli(len(dishes)))
}

// The region list is built from regional variations only: allioli's ast
// variation is there, its Spanish ones are in no region (D-ESP-5).
func TestHandler_RegionLeavesOutSpanishVariations(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newSpainWideFakeRepo())

	rec := srv.get(t, "/v1/regions/ast?lang=en")

	assertStatus(t, rec, http.StatusOK)
	var got []string
	for _, v := range decodeStrict[apigen.RegionDetail](t, rec).Variations {
		got = append(got, v.RegionKey+"/"+v.RecipeKey)
	}
	want := []string{"ast/tortilla-chorizo", "ast/fabada-asturiana", "ast/" + allioliAst}
	assertDeepEqual(t, "ast variations", got, want)
}

func TestHandler_SpainWideRecipeReturnsTheRecipe(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newSpainWideFakeRepo())
	for slug, name := range map[string]string{morterSlug: "Allioli de morter", codonySlug: "Allioli de codony"} {
		t.Run(slug, func(t *testing.T) {
			rec := srv.get(t, spainWidePath(allioliKey, slug)+"?lang=es")

			assertStatus(t, rec, http.StatusOK)
			assertSameJSON(t, decodeStrict[apigen.Recipe](t, rec), apiSpainWideRecipe(slug, name))
			assertHeader(t, rec, "Cache-Control", wantCacheControl)
			assertHeader(t, rec, "Content-Type", "application/json")
			if rec.Header().Get("ETag") == "" {
				t.Error("got no ETag, want one")
			}
		})
	}
}

// A regional recipe sends variationSlug null and its region name.
func TestHandler_RegionalRecipeSendsNullSlug(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newSpainWideFakeRepo())

	rec := srv.get(t, "/v1/dishes/tortilla/variations/gal/recipe?lang=en")

	assertStatus(t, rec, http.StatusOK)
	got := decodeStrict[apigen.Recipe](t, rec)
	if got.VariationSlug != nil || got.RegionName == nil || *got.RegionName != "Galicia" {
		t.Errorf("got variationSlug %v, regionName %v; want null and Galicia", got.VariationSlug, got.RegionName)
	}
}

func TestHandler_SpainWideRecipeIfNoneMatch(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newSpainWideFakeRepo())
	path := spainWidePath(allioliKey, morterSlug) + "?lang=ru"
	current := srv.get(t, path).Header().Get("ETag")

	rec := srv.get(t, path, current)

	assertStatus(t, rec, http.StatusNotModified)
	assertHeader(t, rec, "ETag", current)
	if rec.Body.Len() != 0 {
		t.Errorf("got 304 body %q, want empty", rec.Body.String())
	}
}

// Each Spanish recipe has its own validator: another slug's, another
// locale's or the list's doesn't match.
func TestHandler_SpainWideRecipeETagNamesTheVariation(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newSpainWideFakeRepo())
	morter := srv.get(t, spainWidePath(allioliKey, morterSlug)+"?lang=en").Header().Get("ETag")
	for name, target := range map[string]string{
		"another slug":   spainWidePath(allioliKey, codonySlug) + "?lang=en",
		"another locale": spainWidePath(allioliKey, morterSlug) + "?lang=es",
	} {
		t.Run(name, func(t *testing.T) {
			assertStatus(t, srv.get(t, target, morter), http.StatusOK)
		})
	}
	t.Run("the list's", func(t *testing.T) {
		assertStatus(t, srv.get(t, spainWidePath(allioliKey, morterSlug)+"?lang=en", listETag(fakeVersion, "en")), http.StatusOK)
	})
}

func TestHandler_SpainWideRecipeNotFound(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newSpainWideFakeRepo())
	tests := []struct {
		name        string
		target      string
		ifNoneMatch []string
	}{
		{"unknown slug", spainWidePath(allioliKey, "allioli-a-mano"), nil},
		{"a regional variation's recipe key", spainWidePath(allioliKey, allioliAst), nil},
		{"slug of another dish", spainWidePath("tortilla", morterSlug), nil},
		{"unknown dish", spainWidePath("salmorejo", morterSlug), nil},
		{"wildcard on an unknown recipe", spainWidePath(allioliKey, "allioli-a-mano"), []string{"*"}},
		{"esp on the regional route", "/v1/dishes/allioli/variations/esp/recipe", nil},
		{"esp is not a region", "/v1/regions/esp", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorCode(t, srv.get(t, tt.target+"?lang=en", tt.ifNoneMatch...), http.StatusNotFound, "not_found")
		})
	}
}

func TestHandler_SpainWideRecipeBeforeAnyImportIsNotFound(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newEmptyFakeRepo())

	assertErrorCode(t, srv.get(t, spainWidePath(allioliKey, morterSlug)), http.StatusNotFound, "not_found")
}

func TestHandler_MalformedVariationSlugIsNotFoundWithoutTouchingTheRepository(t *testing.T) {
	t.Parallel()
	for _, slug := range []string{"Allioli", "a_b", strings.Repeat("a", 65), "-allioli", "allioli-", "a--b", "allioli%C3%B1"} {
		t.Run(slug, func(t *testing.T) {
			repo := newSpainWideFakeRepo()
			srv := newAPIServer(t, repo)

			assertErrorCode(t, srv.get(t, spainWidePath(allioliKey, slug)), http.StatusNotFound, "not_found")
			assertErrorCode(t, srv.get(t, spainWidePath("Allioli", morterSlug)), http.StatusNotFound, "not_found")
			if n := repo.totalCalls(); n != 0 {
				t.Errorf("got %d repository calls, want 0 (calls=%v)", n, repo.calls)
			}
		})
	}
}

func TestHandler_SpainWideRecipeUnknownLangIsBadRequest(t *testing.T) {
	t.Parallel()
	repo := newSpainWideFakeRepo()
	srv := newAPIServer(t, repo)

	assertErrorCode(t, srv.get(t, spainWidePath(allioliKey, morterSlug)+"?lang=fr"), http.StatusBadRequest, "bad_request")
	if n := repo.totalCalls(); n != 0 {
		t.Errorf("got %d repository calls, want 0", n)
	}
}

func TestHandler_SpainWideRecipeRepositoryFailureIsInternal(t *testing.T) {
	t.Parallel()
	for _, method := range []string{methodSpainWide, methodContentVersion} {
		t.Run(method, func(t *testing.T) {
			repo := newSpainWideFakeRepo()
			repo.failWith(method, errRepoDown)
			srv := newAPIServer(t, repo)

			rec := srv.get(t, spainWidePath(allioliKey, morterSlug)+"?lang=ru")

			assertInternalError(t, rec, errRepoDown)
			assertOneErrorLog(t, srv, []string{"hunter2"})
		})
	}
}
