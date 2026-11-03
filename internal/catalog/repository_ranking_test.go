package catalog_test

import (
	"context"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
)

// setScores stores each fixture variation's score, by recipe key.
func (p pgFixture) setScores(t *testing.T, scores map[string]float64) {
	t.Helper()
	for recipeKey, score := range scores {
		p.exec(t, `UPDATE variations SET score = $1 WHERE id = $2`, score, p.f.VariationIDs[recipeKey])
	}
}

func (p pgFixture) fixtureDishes(t *testing.T) []catalog.Dish {
	t.Helper()
	dishes, err := p.repo.Dishes(context.Background(), "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	dishes = fixtureDishesOnly(dishes)
	requireLen(t, "fixture dishes", len(dishes), 2)
	return dishes
}

func dishOrders(ds []catalog.Dish) []int {
	out := make([]int, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Order)
	}
	return out
}

// Fixture: tortilla (sort order 1) has gal tortilla-betanzos and ast
// tortilla-chorizo; cocido (sort order 3) has ast fabada-asturiana.
func TestPGRepository_DishesOrderedBySummedScoreThenEditorialOrder(t *testing.T) {
	cases := []struct {
		name     string
		scores   map[string]float64
		wantKeys []string
	}{
		{"all scores 0 keep editorial order", nil, []string{"tortilla", "cocido"}},
		{"a higher dish score comes first", map[string]float64{"fabada-asturiana": 5}, []string{"cocido", "tortilla"}},
		{"a dish score is the sum of its variations", map[string]float64{
			"tortilla-betanzos": 3, "tortilla-chorizo": 3, "fabada-asturiana": 5,
		}, []string{"tortilla", "cocido"}},
		{"equal dish scores keep editorial order", map[string]float64{
			"tortilla-betanzos": 2.5, "tortilla-chorizo": 2.5, "fabada-asturiana": 5,
		}, []string{"tortilla", "cocido"}},
		{"a tie at stored precision keeps editorial order despite float sums", map[string]float64{
			"tortilla-betanzos": 0.1, "tortilla-chorizo": 0.7, "fabada-asturiana": 0.8,
		}, []string{"tortilla", "cocido"}},
		{"fractional scores count", map[string]float64{
			"tortilla-betanzos": 0.001, "fabada-asturiana": 0.002,
		}, []string{"cocido", "tortilla"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := loadPGFixture(t)
			p.setScores(t, tc.scores)

			dishes := p.fixtureDishes(t)

			assertDeepEqual(t, "dish keys", dishKeys(dishes), tc.wantKeys)
			assertDeepEqual(t, "dish orders", dishOrders(dishes), oneToN(len(dishes)))
		})
	}
}

func TestPGRepository_VariationsCarryTheirStoredScore(t *testing.T) {
	scores := map[string]float64{"tortilla-betanzos": 12.5, "tortilla-chorizo": 0, "fabada-asturiana": 0.125}
	p := loadPGFixture(t)
	p.setScores(t, scores)

	got := map[string]float64{}
	for _, d := range p.fixtureDishes(t) {
		for _, v := range d.Variations {
			got[v.RecipeKey] = v.Score
		}
	}
	assertDeepEqual(t, "scores", got, scores)
}

func TestPGRepository_RegionRankByVariationScoreThenDishOrder(t *testing.T) {
	cases := []struct {
		name   string
		scores map[string]float64
		want   map[string]int
	}{
		{"all scores 0 follow dish order", nil,
			map[string]int{"tortilla-betanzos": 1, "tortilla-chorizo": 1, "fabada-asturiana": 2}},
		{"a higher variation score ranks first though its dish comes later", map[string]float64{
			"tortilla-betanzos": 100, "tortilla-chorizo": 1, "fabada-asturiana": 50,
		}, map[string]int{"tortilla-betanzos": 1, "tortilla-chorizo": 2, "fabada-asturiana": 1}},
		{"equal variation scores follow dish display order", map[string]float64{
			"tortilla-chorizo": 4, "fabada-asturiana": 4, "tortilla-betanzos": 1,
		}, map[string]int{"tortilla-betanzos": 1, "tortilla-chorizo": 1, "fabada-asturiana": 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := loadPGFixture(t)
			p.setScores(t, tc.scores)

			got := map[string]int{}
			for _, d := range p.fixtureDishes(t) {
				for _, v := range d.Variations {
					got[v.RecipeKey] = v.RegionRank
				}
			}
			assertDeepEqual(t, "region ranks", got, tc.want)
		})
	}
}

// A dish's variations stay in region order even when its later-region
// variation outscores the earlier one.
func TestPGRepository_DishVariationsStayInRegionOrderWithScores(t *testing.T) {
	p := loadPGFixture(t)
	p.setScores(t, map[string]float64{"tortilla-betanzos": 0, "tortilla-chorizo": 100})

	tortilla := p.fixtureDishes(t)[0]

	assertDeepEqual(t, "tortilla variation regions", variationRegionKeys(tortilla.Variations), []string{"gal", "ast"})
}

func TestPGService_RegionListsVariationsByRegionRank(t *testing.T) {
	p := loadPGFixture(t)
	scores := map[string]float64{}
	for key, s := range fixtureScores {
		scores[key] = s.score
	}
	p.setScores(t, scores)
	svc := catalog.NewService(p.repo)

	got, _, err := svc.Region(context.Background(), "ast", "en")
	if err != nil {
		t.Fatalf("Region: %v", err)
	}
	assertDeepEqual(t, "recipe keys", recipeKeys(got.Variations), wantScoredRegion["ast"])
	assertDeepEqual(t, "region ranks", regionRanks(got.Variations), []int{1, 2})
}
