package catalog

import (
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/api/apigen"
)

// cacheControl is the BE-2 caching policy for every catalog response.
const cacheControl = "public, max-age=60, stale-while-revalidate=600"

// listETag tags a list or detail response; it changes with every import.
func listETag(version int64, l Locale) string {
	return fmt.Sprintf(`W/"%d-%s"`, version, l)
}

// recipeETag tags one variation's recipe.
func recipeETag(version int64, l Locale, rc Recipe) string {
	return fmt.Sprintf(`W/"%d-%s-%s-%s"`, version, l, rc.DishKey, variationKey(rc))
}

// variationKey names a variation within its dish: its region key, or
// esp-<slug> for a Spanish variation (D-ESP-7).
func variationKey(rc Recipe) string {
	if rc.VariationSlug != "" {
		return rc.RegionKey + "-" + rc.VariationSlug
	}
	return rc.RegionKey
}

// cacheHeaders are the ETag and Cache-Control of a 200; every generated
// 200 headers type converts from it.
func cacheHeaders(etag string) apigen.NotModifiedResponseHeaders {
	policy := cacheControl
	return apigen.NotModifiedResponseHeaders{CacheControl: &policy, ETag: &etag}
}
