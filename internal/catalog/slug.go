package catalog

import "regexp"

// Slug limits from the contract's Slug schema, which the generated wrapper
// doesn't enforce.
const maxSlugLength = 64

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func validSlug(s string) bool {
	return len(s) <= maxSlugLength && slugPattern.MatchString(s)
}
