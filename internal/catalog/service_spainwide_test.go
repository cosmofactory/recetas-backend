package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
)

func TestService_SpainWideRecipeReturnsRecipeAndVersion(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newSpainWideFakeRepo())

	got, version, err := svc.SpainWideRecipe(context.Background(), allioliKey, codonySlug, "ru")
	if err != nil {
		t.Fatalf("SpainWideRecipe: %v", err)
	}
	assertVersion(t, version, fakeVersion)
	assertDeepEqual(t, "recipe", got, spainWideRecipes()[allioliKey+"/"+codonySlug])
}

func TestService_SpainWideRecipeNotFound(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newSpainWideFakeRepo())

	_, _, err := svc.SpainWideRecipe(context.Background(), allioliKey, "allioli-a-mano", "en")

	assertNotFound(t, err)
}

func TestService_SpainWideRecipeWrapsRepositoryErrors(t *testing.T) {
	t.Parallel()
	repo := newSpainWideFakeRepo()
	repo.failWith(methodSpainWide, errRepoDown)
	svc := catalog.NewService(repo)

	_, _, err := svc.SpainWideRecipe(context.Background(), allioliKey, morterSlug, "en")

	if !errors.Is(err, errRepoDown) {
		t.Errorf("got error %v, want it to wrap %v", err, errRepoDown)
	}
}

// The service keeps the repository's variation order: regional first, then
// Spanish in bundle order (D-ESP-4).
func TestService_DishKeepsSpanishVariationsAfterRegionalOnes(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newSpainWideFakeRepo())

	got, _, err := svc.Dish(context.Background(), allioliKey, "en")
	if err != nil {
		t.Fatalf("Dish: %v", err)
	}
	assertDeepEqual(t, "allioli", got, allioliDish(len(fixture().Dishes)+1))
}

func TestService_EspIsNotARegion(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newSpainWideFakeRepo())

	_, _, err := svc.Region(context.Background(), spainWideKey, "en")

	assertNotFound(t, err)
}

func TestService_RegionLeavesOutSpanishVariations(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newSpainWideFakeRepo())

	got, _, err := svc.Region(context.Background(), "ast", "en")
	if err != nil {
		t.Fatalf("Region: %v", err)
	}
	want := []string{"tortilla-chorizo", "fabada-asturiana", allioliAst}
	assertDeepEqual(t, "ast recipe keys", recipeKeys(got.Variations), want)
}
