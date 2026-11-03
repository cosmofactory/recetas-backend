package catalog_test

import (
	"context"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
)

func TestService_RegionListsVariationsByRegionRank(t *testing.T) {
	t.Parallel()
	svc := catalog.NewService(newFakeRepoWithDishes(scoredDishes("en")))
	for region, want := range wantScoredRegion {
		t.Run(region, func(t *testing.T) {
			got, _, err := svc.Region(context.Background(), region, "en")
			if err != nil {
				t.Fatalf("Region(%q): %v", region, err)
			}
			assertDeepEqual(t, "recipe keys", recipeKeys(got.Variations), want)
			assertDeepEqual(t, "region ranks", regionRanks(got.Variations), oneToN(len(want)))
		})
	}
}

// The region list is built from the memoised dish list, which other
// requests share: ranking a region must not reorder the dishes or their
// variations.
func TestService_RegionLeavesTheDishListUntouched(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := catalog.NewService(newFakeRepoWithDishes(scoredDishes("en")))
	if _, _, err := svc.Region(ctx, "ast", "en"); err != nil {
		t.Fatalf("Region: %v", err)
	}

	dishes, _, err := svc.Dishes(ctx, "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	assertDeepEqual(t, "dishes", dishes, scoredDishes("en"))
}
