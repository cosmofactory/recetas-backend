package catalog_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
	"github.com/cosmofactory/recetas-backend/internal/catalog"
)

// endpoint is one catalog operation on a fixture resource.
type endpoint struct {
	name string
	path string
	etag func(version int64, loc string) string
	// body decodes the response strictly and returns it with the expected
	// value for loc.
	body func(t *testing.T, rec *httptest.ResponseRecorder, loc string) (got, want any)
}

func listETag(version int64, loc string) string { return fmt.Sprintf(`W/"%d-%s"`, version, loc) }

func recipeETag(dish, region string) func(int64, string) string {
	return func(version int64, loc string) string {
		return fmt.Sprintf(`W/"%d-%s-%s-%s"`, version, loc, dish, region)
	}
}

func endpoints() []endpoint {
	return []endpoint{
		{"listRegions", "/v1/regions", listETag, func(t *testing.T, rec *httptest.ResponseRecorder, loc string) (any, any) {
			return decodeStrict[[]apigen.Region](t, rec), apiRegions(loc)
		}},
		{"getRegion", "/v1/regions/ast", listETag, func(t *testing.T, rec *httptest.ResponseRecorder, loc string) (any, any) {
			return decodeStrict[apigen.RegionDetail](t, rec), apiRegionDetail(t, "ast", loc)
		}},
		{"listDishes", "/v1/dishes", listETag, func(t *testing.T, rec *httptest.ResponseRecorder, loc string) (any, any) {
			return decodeStrict[[]apigen.Dish](t, rec), apiDishes(loc)
		}},
		{"getDish", "/v1/dishes/tortilla", listETag, func(t *testing.T, rec *httptest.ResponseRecorder, loc string) (any, any) {
			return decodeStrict[apigen.Dish](t, rec), apiDishByKey(t, "tortilla", loc)
		}},
		{"getRecipe", "/v1/dishes/tortilla/variations/gal/recipe", recipeETag("tortilla", "gal"), func(t *testing.T, rec *httptest.ResponseRecorder, loc string) (any, any) {
			return decodeStrict[apigen.Recipe](t, rec), apiRecipe(t, recipePair{"tortilla", "gal"}, loc)
		}},
	}
}

func withLang(path, loc string) string { return path + "?lang=" + loc }

func TestHandler_ReturnsLocalisedBodyPerEndpoint(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, ep := range endpoints() {
		for _, l := range locales {
			loc := string(l)
			t.Run(ep.name+"/"+loc, func(t *testing.T) {
				rec := srv.get(t, withLang(ep.path, loc))

				assertStatus(t, rec, http.StatusOK)
				got, want := ep.body(t, rec, loc)
				assertSameJSON(t, got, want)
			})
		}
	}
}

func TestHandler_MissingLangDefaultsToEnglish(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, ep := range endpoints() {
		t.Run(ep.name, func(t *testing.T) {
			rec := srv.get(t, ep.path)

			assertStatus(t, rec, http.StatusOK)
			got, want := ep.body(t, rec, "en")
			assertSameJSON(t, got, want)
			assertHeader(t, rec, "ETag", ep.etag(fakeVersion, "en"))
		})
	}
}

func TestHandler_SendsCacheHeaders(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, ep := range endpoints() {
		for _, l := range locales {
			loc := string(l)
			t.Run(ep.name+"/"+loc, func(t *testing.T) {
				rec := srv.get(t, withLang(ep.path, loc))

				assertStatus(t, rec, http.StatusOK)
				assertHeader(t, rec, "ETag", ep.etag(fakeVersion, loc))
				assertHeader(t, rec, "Cache-Control", wantCacheControl)
				assertHeader(t, rec, "Content-Type", "application/json")
			})
		}
	}
}

func TestHandler_RecipeETagNamesTheVariation(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())

	rec := srv.get(t, "/v1/dishes/cocido/variations/ast/recipe?lang=ru")

	assertStatus(t, rec, http.StatusOK)
	assertHeader(t, rec, "ETag", `W/"5-ru-cocido-ast"`)
}

// conditionalCase builds the If-None-Match header lines from the current tag.
type conditionalCase struct {
	name        string
	ifNoneMatch func(current string) []string
	wantStatus  int
}

func conditionalCases() []conditionalCase {
	one := func(v string) func(string) []string { return func(string) []string { return []string{v} } }
	return []conditionalCase{
		{"exact match", func(c string) []string { return []string{c} }, http.StatusNotModified},
		{"match inside a list", func(c string) []string { return []string{`W/"4-en", ` + c + `, W/"x"`} }, http.StatusNotModified},
		{"match in a list without spaces", func(c string) []string { return []string{`W/"4-en",` + c} }, http.StatusNotModified},
		{"match on a second header line", func(c string) []string { return []string{`W/"4-en"`, c} }, http.StatusNotModified},
		{"wildcard", one("*"), http.StatusNotModified},
		{"strong form matches weakly", func(c string) []string { return []string{strings.TrimPrefix(c, "W/")} }, http.StatusNotModified},
		{"older content version", one(`W/"4-en"`), http.StatusOK},
		{"other locale", one(`W/"5-es"`), http.StatusOK},
		{"unquoted garbage", one("5-en"), http.StatusOK},
		{"empty header", one(""), http.StatusOK},
	}
}

func TestHandler_IfNoneMatch(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, ep := range endpoints() {
		current := ep.etag(fakeVersion, "en")
		for _, tc := range conditionalCases() {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				rec := srv.get(t, withLang(ep.path, "en"), tc.ifNoneMatch(current)...)

				assertStatus(t, rec, tc.wantStatus)
				assertHeader(t, rec, "ETag", current)
				assertHeader(t, rec, "Cache-Control", wantCacheControl)
			})
		}
	}
}

func TestHandler_NotModifiedHasNoBody(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, ep := range endpoints() {
		t.Run(ep.name, func(t *testing.T) {
			rec := srv.get(t, withLang(ep.path, "ru"), ep.etag(fakeVersion, "ru"))

			assertStatus(t, rec, http.StatusNotModified)
			if rec.Body.Len() != 0 {
				t.Errorf("got 304 body %q, want empty", rec.Body.String())
			}
		})
	}
}

func TestHandler_ListETagDoesNotMatchARecipe(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())

	rec := srv.get(t, "/v1/dishes/tortilla/variations/gal/recipe?lang=en", `W/"5-en"`)

	assertStatus(t, rec, http.StatusOK)
}

func TestHandler_ETagChangesWithContentVersion(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	srv := newAPIServer(t, repo)
	stale := listETag(fakeVersion, "en")
	repo.setVersion(fakeVersion + 1)

	rec := srv.get(t, "/v1/dishes?lang=en", stale)

	assertStatus(t, rec, http.StatusOK)
	assertHeader(t, rec, "ETag", listETag(fakeVersion+1, "en"))
}

// RFC 9110 13.1.2: "*" only matches when a current representation exists.
func TestHandler_WildcardOnUnknownResourceIsNotFound(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, target := range []string{"/v1/regions/nope", "/v1/dishes/nope", "/v1/dishes/cocido/variations/gal/recipe"} {
		t.Run(target, func(t *testing.T) {
			assertErrorCode(t, srv.get(t, target, "*"), http.StatusNotFound, "not_found")
		})
	}
}

func TestHandler_BeforeAnyImportListsAreEmpty(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newEmptyFakeRepo())
	for _, path := range []string{"/v1/regions", "/v1/dishes"} {
		for _, l := range locales {
			loc := string(l)
			t.Run(path+"/"+loc, func(t *testing.T) {
				rec := srv.get(t, withLang(path, loc))

				assertStatus(t, rec, http.StatusOK)
				if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
					t.Errorf("got body %q, want []", got)
				}
				assertHeader(t, rec, "ETag", listETag(0, loc))
				assertHeader(t, rec, "Cache-Control", wantCacheControl)
			})
		}
	}
}

func TestHandler_BeforeAnyImportDetailsAreNotFound(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newEmptyFakeRepo())
	for _, ep := range endpoints() {
		if !strings.HasPrefix(ep.name, "get") {
			continue
		}
		t.Run(ep.name, func(t *testing.T) {
			assertErrorCode(t, srv.get(t, ep.path), http.StatusNotFound, "not_found")
		})
	}
}

func TestHandler_UnknownKeysAreNotFound(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, target := range []string{
		"/v1/regions/nope",
		"/v1/dishes/nope",
		"/v1/dishes/nope/variations/gal/recipe",
		"/v1/dishes/tortilla/variations/nope/recipe",
		"/v1/dishes/cocido/variations/gal/recipe",
		"/v1/dishes/" + strings.Repeat("a", 64),
	} {
		t.Run(target, func(t *testing.T) {
			assertErrorCode(t, srv.get(t, target+"?lang=es"), http.StatusNotFound, "not_found")
		})
	}
}

func malformedSlugTargets() []string {
	tooLong := strings.Repeat("a", 65)
	var out []string
	for _, slug := range []string{"Tortilla", "a_b", tooLong, "-tortilla", "tortilla-", "a--b", "tortilla%C3%B1", "GAL"} {
		out = append(out,
			"/v1/regions/"+slug,
			"/v1/dishes/"+slug,
			"/v1/dishes/"+slug+"/variations/gal/recipe",
			"/v1/dishes/tortilla/variations/"+slug+"/recipe",
		)
	}
	return out
}

func TestHandler_MalformedSlugsAreNotFoundWithoutTouchingTheRepository(t *testing.T) {
	t.Parallel()
	for _, target := range malformedSlugTargets() {
		t.Run(target, func(t *testing.T) {
			repo := newFakeRepo()
			srv := newAPIServer(t, repo)

			assertErrorCode(t, srv.get(t, target), http.StatusNotFound, "not_found")
			if n := repo.totalCalls(); n != 0 {
				t.Errorf("got %d repository calls, want 0 (calls=%v)", n, repo.calls)
			}
		})
	}
}

func TestHandler_UnknownLangIsBadRequestWithoutTouchingTheRepository(t *testing.T) {
	t.Parallel()
	for _, ep := range endpoints() {
		t.Run(ep.name, func(t *testing.T) {
			repo := newFakeRepo()
			srv := newAPIServer(t, repo)

			assertErrorCode(t, srv.get(t, withLang(ep.path, "fr")), http.StatusBadRequest, "bad_request")
			if n := repo.totalCalls(); n != 0 {
				t.Errorf("got %d repository calls, want 0", n)
			}
		})
	}
}

// failingCall is an endpoint whose request fails in one repository method.
type failingCall struct {
	target  string
	failsAt string
}

func failingCalls() []failingCall {
	return []failingCall{
		{"/v1/regions?lang=ru", methodRegions},
		{"/v1/regions?lang=ru", methodContentVersion},
		{"/v1/regions/gal?lang=ru", methodRegions},
		{"/v1/regions/gal?lang=ru", methodDishes},
		{"/v1/dishes?lang=ru", methodDishes},
		{"/v1/dishes?lang=ru", methodContentVersion},
		{"/v1/dishes/tortilla?lang=ru", methodDishes},
		{"/v1/dishes/tortilla/variations/gal/recipe?lang=ru", methodRecipe},
		{"/v1/dishes/tortilla/variations/gal/recipe?lang=ru", methodContentVersion},
	}
}

const wantInternalBody = `{"error":{"code":"internal","message":"internal error"}}`

func TestHandler_RepositoryFailureIsAGenericInternalError(t *testing.T) {
	t.Parallel()
	repoErrors := []struct {
		name   string
		err    error
		inLogs []string
	}{
		{"plain", errRepoDown, []string{"hunter2"}},
		{"missing translation", missingRuDish(), []string{"tortilla", "ru"}},
	}
	for _, re := range repoErrors {
		for _, fc := range failingCalls() {
			t.Run(re.name+"/"+fc.failsAt+fc.target, func(t *testing.T) {
				repo := newFakeRepo()
				repo.failWith(fc.failsAt, re.err)
				srv := newAPIServer(t, repo)

				rec := srv.get(t, fc.target)

				assertInternalError(t, rec, re.err)
				assertOneErrorLog(t, srv, re.inLogs)
			})
		}
	}
}

func assertInternalError(t *testing.T, rec *httptest.ResponseRecorder, cause error) {
	t.Helper()
	assertStatus(t, rec, http.StatusInternalServerError)
	assertHeader(t, rec, "Content-Type", "application/json")
	if got := strings.TrimSpace(rec.Body.String()); got != wantInternalBody {
		t.Errorf("got body %s, want %s", got, wantInternalBody)
	}
	if strings.Contains(rec.Body.String(), cause.Error()) {
		t.Errorf("body leaks the internal error text %q", cause.Error())
	}
	if rec.Header().Get("ETag") != "" {
		t.Errorf("got ETag %q on a 500, want none", rec.Header().Get("ETag"))
	}
}

func assertOneErrorLog(t *testing.T, srv apiServer, wantInErr []string) {
	t.Helper()
	lines := srv.errorLogLines(t)
	if len(lines) != 1 {
		t.Fatalf("got %d ERROR log lines, want 1 (logs=%s)", len(lines), srv.logs.String())
	}
	errText, _ := lines[0]["err"].(string)
	for _, want := range wantInErr {
		if !strings.Contains(errText, want) {
			t.Errorf("got ERROR log err %q, want it to contain %q", errText, want)
		}
	}
}

func TestHandler_NotFoundIsNotLoggedAsError(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())

	srv.get(t, "/v1/dishes/nope")
	srv.get(t, "/v1/dishes/Tortilla")

	if lines := srv.errorLogLines(t); len(lines) != 0 {
		t.Errorf("got %d ERROR log lines for 404s, want 0 (logs=%s)", len(lines), srv.logs.String())
	}
}

func TestHandler_WrappedNotFoundFromRepositoryIsNotFound(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.failWith(methodRecipe, fmt.Errorf("wrapped twice: %w", fmt.Errorf("lookup: %w", catalog.ErrNotFound)))
	srv := newAPIServer(t, repo)

	rec := srv.get(t, "/v1/dishes/tortilla/variations/gal/recipe")

	assertErrorCode(t, rec, http.StatusNotFound, "not_found")
	if lines := srv.errorLogLines(t); len(lines) != 0 {
		t.Errorf("got %d ERROR log lines for a wrapped ErrNotFound, want 0", len(lines))
	}
}
