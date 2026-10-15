package catalog

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("catalog: not found")

// MissingTranslationError reports a translation row that the content should
// have but doesn't. It is a data bug: there is no fallback locale (D-BE3-3).
type MissingTranslationError struct {
	Entity string
	Key    string
	Locale Locale
}

func (e *MissingTranslationError) Error() string {
	return fmt.Sprintf("catalog: %s %q has no %s translation", e.Entity, e.Key, e.Locale)
}

// Entities named in a MissingTranslationError.
const (
	entityRegion           = "region"
	entityDish             = "dish"
	entityVariation        = "variation"
	entityRecipeTimeNote   = "recipe time note"
	entityRecipeIngredient = "recipe ingredient"
	entityRecipeStep       = "recipe step"
)

// translation names the translated fields of one entity in one locale.
type translation struct {
	entity string
	key    string
	locale Locale
	fields []*string
}

// firstMissing returns a *MissingTranslationError for the first translation
// with a nil field, or nil when every field is present.
func firstMissing(translations ...translation) error {
	for _, t := range translations {
		if hasNil(t.fields) {
			return &MissingTranslationError{Entity: t.entity, Key: t.key, Locale: t.locale}
		}
	}
	return nil
}

func hasNil(fields []*string) bool {
	for _, f := range fields {
		if f == nil {
			return true
		}
	}
	return false
}
