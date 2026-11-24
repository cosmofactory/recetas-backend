package main

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// spainWideKeys derives a Spanish variation's slug from its Spanish name and
// its recipe key <dishKey>-esp-<slug> (D-ESP-2), shortening the slug so the
// recipe key fits importer.MaxKeyLength.
func spainWideKeys(dishKey, spanishName string) (slug, recipeKey string, err error) {
	prefix := dishKey + "-" + importer.SpainWideRegionKey + "-"
	room := importer.MaxKeyLength - len(prefix)
	if room < 1 {
		return "", "", fmt.Errorf("dish key %q leaves no room for a variation slug", dishKey)
	}
	slug = shorten(slugify(spanishName), room)
	if slug == "" {
		return "", "", fmt.Errorf("spanish name %q gives an empty slug", spanishName)
	}
	return slug, prefix + slug, nil
}

// slugify lowercases name, strips its accents and joins the runs of a-z and
// 0-9 with single hyphens: "Caldo de cocido (pringá)" is
// "caldo-de-cocido-pringa".
func slugify(name string) string {
	words := strings.FieldsFunc(stripMarks(strings.ToLower(name)), func(r rune) bool { return !isSlugRune(r) })
	return strings.Join(words, "-")
}

func isSlugRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// stripMarks drops the combining marks of the decomposed text, so "ñ" is
// "n" and "à" is "a".
func stripMarks(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// shorten cuts slug to at most limit bytes, at a hyphen when one is within
// the limit, so a word is only split when it alone exceeds it.
func shorten(slug string, limit int) string {
	if len(slug) <= limit {
		return slug
	}
	if slug[limit] == '-' {
		return slug[:limit]
	}
	if i := strings.LastIndexByte(slug[:limit], '-'); i > 0 {
		return slug[:i]
	}
	return slug[:limit]
}
