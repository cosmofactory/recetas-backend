package importer

import (
	"regexp"
	"slices"
	"strings"
)

// Bounds shared with api/content-bundle.schema.json.
const (
	maxSlugLength  = 64
	minServings    = 1
	maxServings    = 20
	maxTimeMinutes = 7 * 24 * 60
	maxQuantity    = 9_999_999
	quantityScale  = 1000 // quantities are stored with 3 decimal places
)

var (
	slugPattern      = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	tileColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	photoHashPattern = regexp.MustCompile(`^[0-9a-f]{8,64}$`)

	courses = []string{"tapas", "mains", "desserts"}
	seasons = []string{"summer", "winter", "easter", "all"}
	units   = []string{"g", "ml", "tsp"}
)

// Validate checks the semantic rules the schema can't express and returns a
// *ValidationError with every problem found, or nil.
func Validate(b Bundle) error {
	var ps problems
	if b.SchemaVersion != SchemaVersion {
		ps.add("/schemaVersion", "unsupported schemaVersion %d; this importer supports %d", b.SchemaVersion, SchemaVersion)
	}
	regionKeys := validateRegions(&ps, b.Regions)
	refs := validateDishes(&ps, b.Dishes, regionKeys)
	validateRecipes(&ps, b.Recipes, refs)
	return ps.err()
}

func validSlug(s string) bool {
	return len(s) <= maxSlugLength && slugPattern.MatchString(s)
}

// keySet checks that keys are slugs and unique.
type keySet struct {
	what string
	seen map[string]bool
}

func newKeySet(what string) keySet {
	return keySet{what: what, seen: map[string]bool{}}
}

func (k keySet) check(ps *problems, at pointer, key string) {
	switch {
	case !validSlug(key):
		ps.add(at, "%s key %q is not a slug (lowercase a-z, 0-9 and single hyphens, at most %d characters)", k.what, key, maxSlugLength)
	case k.seen[key]:
		ps.add(at, "duplicate %s key %q", k.what, key)
	}
	k.seen[key] = true
}

func checkText(ps *problems, at pointer, t Text) {
	for i, v := range t.values() {
		if strings.TrimSpace(v) == "" {
			ps.add(at.at(locales[i]), "text is blank")
		}
	}
}

func checkEnum(ps *problems, at pointer, value string, allowed []string) {
	if !slices.Contains(allowed, value) {
		ps.add(at, "%q is not one of %s", value, strings.Join(allowed, ", "))
	}
}

func validateRegions(ps *problems, regions []Region) keySet {
	keys := newKeySet("region")
	for i, r := range regions {
		at := pointer("/regions").at(i)
		keys.check(ps, at.at("key"), r.Key)
		if !tileColorPattern.MatchString(r.TileColor) {
			ps.add(at.at("tileColor"), "tile color %q is not #RRGGBB", r.TileColor)
		}
		checkText(ps, at.at("name"), r.Name)
		checkText(ps, at.at("mapLabel"), r.MapLabel)
		checkText(ps, at.at("description"), r.Description)
		for j, name := range r.RegionalDishes {
			if strings.TrimSpace(name) == "" {
				ps.add(at.at("regionalDishes", j), "regional dish name is blank")
			}
		}
	}
	return keys
}

// checkPhotos requires every key to be prefix + "<role>-<hash>".
func checkPhotos(ps *problems, at pointer, p *Photos, prefix string) {
	if p == nil {
		return
	}
	for _, role := range []struct {
		name string
		key  *string
	}{{"thumb", p.Thumb}, {"card", p.Card}, {"hero", p.Hero}} {
		if role.key != nil && !validPhotoKey(*role.key, prefix+role.name+"-") {
			ps.add(at.at(role.name), "photo key %q is not %s%s-<hash> (8-64 lowercase hex digits)", *role.key, prefix, role.name)
		}
	}
}

func validPhotoKey(key, prefix string) bool {
	hash, ok := strings.CutPrefix(key, prefix)
	return ok && photoHashPattern.MatchString(hash)
}

// validQuantity is judged at the stored 3 decimals: a quantity that rounds
// to zero is rejected.
func validQuantity(q float64) bool {
	rounded := roundQuantity(q)
	return rounded > 0 && rounded <= maxQuantity
}
