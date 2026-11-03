package catalog

import (
	"cmp"
	"slices"
)

// variationRef locates a variation in a dish list.
type variationRef struct{ dish, variation int }

// rankRegions sets every variation's RegionRank: its position among its
// region's variations by score descending, then by dish display order. The
// dishes must be in display order; neither they nor their variations move.
func rankRegions(dishes []Dish) {
	for _, refs := range variationsByRegion(dishes) {
		score := func(r variationRef) float64 { return dishes[r.dish].Variations[r.variation].Score }
		slices.SortStableFunc(refs, func(a, b variationRef) int { return cmp.Compare(score(b), score(a)) })
		for i, r := range refs {
			dishes[r.dish].Variations[r.variation].RegionRank = i + 1
		}
	}
}

// variationsByRegion groups the variations by region key, in dish order.
func variationsByRegion(dishes []Dish) map[string][]variationRef {
	byRegion := map[string][]variationRef{}
	for di, d := range dishes {
		for vi, v := range d.Variations {
			byRegion[v.RegionKey] = append(byRegion[v.RegionKey], variationRef{dish: di, variation: vi})
		}
	}
	return byRegion
}
