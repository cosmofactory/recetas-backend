package catalog_test

import "github.com/cosmofactory/recetas-backend/internal/catalog"

// variationScore is a fixture variation's score and the RegionRank it gives.
type variationScore struct {
	score      float64
	regionRank int
}

// fixtureScores make Asturias' list differ from dish order: tortilla
// (gal 100 + ast 1 = 101) stays before cocido (ast 50), but in Asturias
// fabada (50) ranks above tortilla con chorizo (1).
var fixtureScores = map[string]variationScore{
	"tortilla-betanzos": {score: 100, regionRank: 1},
	"tortilla-chorizo":  {score: 1, regionRank: 2},
	"fabada-asturiana":  {score: 50, regionRank: 1},
}

// scoredDishes is the fixture in display order (unchanged, by the sums
// above) with fixtureScores applied, as a repository returns it.
func scoredDishes(l catalog.Locale) []catalog.Dish {
	dishes := domainDishes(l)
	for di := range dishes {
		applyScores(dishes[di].Variations)
	}
	return dishes
}

func applyScores(variations []catalog.Variation) {
	for vi, v := range variations {
		s := fixtureScores[v.RecipeKey]
		variations[vi].Score, variations[vi].RegionRank = s.score, s.regionRank
	}
}

// wantScoredRegion is each region's recipe keys in RegionRank order under
// fixtureScores.
var wantScoredRegion = map[string][]string{
	"ast": {"fabada-asturiana", "tortilla-chorizo"},
	"gal": {"tortilla-betanzos"},
}

func variationRegionKeys(vs []catalog.Variation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.RegionKey)
	}
	return out
}

func regionRanks(vs []catalog.RegionVariation) []int {
	out := make([]int, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.RegionRank)
	}
	return out
}

// oneToN is 1, 2, ..., n: the ranks or orders of a list of n.
func oneToN(n int) []int {
	out := make([]int, 0, n)
	for i := range n {
		out = append(out, i+1)
	}
	return out
}
