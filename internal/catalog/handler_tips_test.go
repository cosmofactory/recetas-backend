package catalog_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// chorizoPair is the fixture recipe without tips.
var chorizoPair = recipePair{"tortilla", "ast"}

func recipePath(p recipePair) string {
	return fmt.Sprintf("/v1/dishes/%s/variations/%s/recipe", p.dish, p.region)
}

// rawTips returns the response's "tips" member as sent, failing the test
// when it is missing.
func rawTips(t *testing.T, rec *httptest.ResponseRecorder) json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not a JSON object: %v (body=%.300q)", err, rec.Body.String())
	}
	tips, ok := body["tips"]
	if !ok {
		t.Fatalf("got no tips field, want one (body=%.300q)", rec.Body.String())
	}
	return tips
}

func decodeTips(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var tips []string
	if err := json.Unmarshal(raw, &tips); err != nil {
		t.Fatalf("tips %s is not an array of strings: %v", raw, err)
	}
	return tips
}

func TestHandler_RecipeTipsAreLocalisedInOrder(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, p := range fixtureRecipePairs {
		for _, l := range locales {
			t.Run(fmt.Sprintf("%s/%s/%s", p.dish, p.region, l), func(t *testing.T) {
				rec := srv.get(t, withLang(recipePath(p), string(l)))

				assertStatus(t, rec, http.StatusOK)
				want := mustDomainRecipe(t, p, l).Tips
				if got := decodeTips(t, rawTips(t, rec)); !slices.Equal(got, want) {
					t.Errorf("got tips %q, want %q", got, want)
				}
			})
		}
	}
}

func TestHandler_RecipeWithoutTipsSendsEmptyArray(t *testing.T) {
	t.Parallel()
	srv := newAPIServer(t, newFakeRepo())
	for _, l := range locales {
		t.Run(string(l), func(t *testing.T) {
			rec := srv.get(t, withLang(recipePath(chorizoPair), string(l)))

			assertStatus(t, rec, http.StatusOK)
			if got := strings.TrimSpace(string(rawTips(t, rec))); got != "[]" {
				t.Errorf("got tips %s, want []", got)
			}
		})
	}
}
