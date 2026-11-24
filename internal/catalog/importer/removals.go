package importer

import (
	"fmt"
	"slices"
	"strings"
)

// UnlistedRemovalsError refuses an import that would remove public paths
// missing from Options.AllowedRemovals (D-INF7-3). Paths are sorted.
type UnlistedRemovalsError struct {
	Paths []string
}

func (e *UnlistedRemovalsError) Error() string {
	return fmt.Sprintf("import would remove %d public path(s) not listed as allowed removals: %s",
		len(e.Paths), strings.Join(e.Paths, ", "))
}

// removedPaths are the public paths, without a locale, that the stored
// catalog serves and the bundle no longer does, sorted. A variation row
// recreated at the same dish and region (or Spanish slug) keeps its path.
func removedPaths(cur state, b Bundle) []string {
	keep := bundlePaths(b)
	var removed []string
	for p := range storedPaths(cur) {
		if !keep[p] {
			removed = append(removed, p)
		}
	}
	slices.Sort(removed)
	return removed
}

// unlisted returns the removed paths that allowed doesn't list.
func unlisted(removed []string, allowed map[string]bool) []string {
	var out []string
	for _, p := range removed {
		if !allowed[p] {
			out = append(out, p)
		}
	}
	return out
}

func storedPaths(cur state) map[string]bool {
	paths := map[string]bool{}
	for key := range cur.regions {
		paths[regionPath(key)] = true
	}
	for key := range cur.dishes {
		paths[dishPath(key)] = true
	}
	for _, v := range cur.variations {
		paths[variationPath(v.dishKey, v.variation)] = true
	}
	return paths
}

func bundlePaths(b Bundle) map[string]bool {
	paths := map[string]bool{}
	for _, r := range b.Regions {
		paths[regionPath(r.Key)] = true
	}
	for _, d := range b.Dishes {
		paths[dishPath(d.Key)] = true
		for _, v := range d.Variations {
			paths[variationPath(d.Key, v)] = true
		}
	}
	return paths
}

func regionPath(key string) string { return "/regions/" + key }

func dishPath(key string) string { return "/dishes/" + key }

// variationPath is /dishes/<dish>/<region>, or /dishes/<dish>/esp/<slug> for
// a Spanish variation.
func variationPath(dishKey string, v Variation) string {
	if v.spainWide() {
		return dishPath(dishKey) + "/" + SpainWideRegionKey + "/" + v.VariationSlug
	}
	return dishPath(dishKey) + "/" + v.RegionKey
}
