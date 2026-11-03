package catalog

import (
	"reflect"
	"testing"
)

// rankCase is a dish list in display order and the RegionRank each variation
// must get, by recipe key.
type rankCase struct {
	name   string
	dishes []Dish
	want   map[string]int
}

func rankedDish(key string, variations ...Variation) Dish {
	return Dish{Key: key, Variations: variations}
}

func scoredVariation(regionKey, recipeKey string, score float64) Variation {
	return Variation{RegionKey: regionKey, RecipeKey: recipeKey, Score: score}
}

func rankCases() []rankCase {
	return []rankCase{
		{name: "no dishes", dishes: nil, want: map[string]int{}},
		{
			name:   "one variation is first in its region",
			dishes: []Dish{rankedDish("tortilla", scoredVariation("gal", "betanzos", 0))},
			want:   map[string]int{"betanzos": 1},
		},
		{
			name: "all scores 0 follow dish display order",
			dishes: []Dish{
				rankedDish("tortilla", scoredVariation("gal", "betanzos", 0), scoredVariation("ast", "chorizo", 0)),
				rankedDish("paella", scoredVariation("val", "valenciana", 0)),
				rankedDish("cocido", scoredVariation("ast", "fabada", 0), scoredVariation("mad", "madrileno", 0)),
			},
			want: map[string]int{"betanzos": 1, "chorizo": 1, "valenciana": 1, "fabada": 2, "madrileno": 1},
		},
		{
			name: "higher variation score ranks first, whatever the dish order",
			dishes: []Dish{
				rankedDish("tortilla", scoredVariation("gal", "betanzos", 100), scoredVariation("ast", "chorizo", 1)),
				rankedDish("cocido", scoredVariation("ast", "fabada", 50)),
				rankedDish("pulpo", scoredVariation("ast", "pulpo-ast", 7)),
			},
			want: map[string]int{"betanzos": 1, "fabada": 1, "pulpo-ast": 2, "chorizo": 3},
		},
		{
			name: "equal scores fall back to dish display order, not sort order",
			dishes: []Dish{
				{Key: "cocido", Order: 1, Variations: []Variation{scoredVariation("ast", "fabada", 4)}},
				{Key: "tortilla", Order: 2, Variations: []Variation{scoredVariation("ast", "chorizo", 4)}},
				{Key: "fabes", Order: 3, Variations: []Variation{scoredVariation("ast", "fabes-almejas", 4)}},
			},
			want: map[string]int{"fabada": 1, "chorizo": 2, "fabes-almejas": 3},
		},
		{
			name: "a dish without a variation in the region takes no rank there",
			dishes: []Dish{
				rankedDish("tortilla", scoredVariation("gal", "betanzos", 0)),
				rankedDish("cocido", scoredVariation("ast", "fabada", 0)),
				rankedDish("pulpo", scoredVariation("gal", "pulpo-feira", 0)),
			},
			want: map[string]int{"betanzos": 1, "fabada": 1, "pulpo-feira": 2},
		},
		{
			name: "fractional scores are compared exactly",
			dishes: []Dish{
				rankedDish("tortilla", scoredVariation("gal", "betanzos", 0.001)),
				rankedDish("pulpo", scoredVariation("gal", "pulpo-feira", 0.002)),
				rankedDish("empanada", scoredVariation("gal", "empanada-gallega", 0)),
			},
			want: map[string]int{"pulpo-feira": 1, "betanzos": 2, "empanada-gallega": 3},
		},
		{
			name: "regions are ranked independently",
			dishes: []Dish{
				rankedDish("tortilla", scoredVariation("gal", "betanzos", 1), scoredVariation("ast", "chorizo", 9)),
				rankedDish("cocido", scoredVariation("gal", "gallego", 2), scoredVariation("ast", "fabada", 3)),
			},
			want: map[string]int{"gallego": 1, "betanzos": 2, "chorizo": 1, "fabada": 2},
		},
	}
}

func TestRankRegions_SetsEachVariationsPositionInItsRegion(t *testing.T) {
	for _, tc := range rankCases() {
		t.Run(tc.name, func(t *testing.T) {
			rankRegions(tc.dishes)

			got := regionRanksByRecipe(tc.dishes)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got ranks %v, want %v", got, tc.want)
			}
		})
	}
}

// Ranking only fills RegionRank: dishes keep display order and each dish's
// variations keep region order, so all variations of a dish stay equal.
func TestRankRegions_KeepsDishAndVariationOrder(t *testing.T) {
	dishes := []Dish{
		rankedDish("tortilla", scoredVariation("gal", "betanzos", 0), scoredVariation("ast", "chorizo", 90)),
		rankedDish("cocido", scoredVariation("ast", "fabada", 5), scoredVariation("mad", "madrileno", 1)),
	}

	rankRegions(dishes)

	var got []string
	for _, d := range dishes {
		for _, v := range d.Variations {
			got = append(got, d.Key+"/"+v.RegionKey)
		}
	}
	want := []string{"tortilla/gal", "tortilla/ast", "cocido/ast", "cocido/mad"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got order %v, want %v", got, want)
	}
}

func regionRanksByRecipe(dishes []Dish) map[string]int {
	out := map[string]int{}
	for _, d := range dishes {
		for _, v := range d.Variations {
			out[v.RecipeKey] = v.RegionRank
		}
	}
	return out
}
