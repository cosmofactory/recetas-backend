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
func recipeETag(version int64, l Locale, dishKey, regionKey string) string {
	return fmt.Sprintf(`W/"%d-%s-%s-%s"`, version, l, dishKey, regionKey)
}

// cacheHeaders are the ETag and Cache-Control of a 200; every generated
// 200 headers type converts from it.
func cacheHeaders(etag string) apigen.NotModifiedResponseHeaders {
	policy := cacheControl
	return apigen.NotModifiedResponseHeaders{CacheControl: &policy, ETag: &etag}
}
