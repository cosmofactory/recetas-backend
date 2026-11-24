package importer_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// variationPointer is the first variation in minimalDoc.
const variationPointer = "/dishes/0/variations/0"

// spainWideMinimalDoc is minimalDoc with its one variation made Spanish and
// its dish a basics dish (D-ESP-6).
func spainWideMinimalDoc(t *testing.T) jsonDoc {
	t.Helper()
	doc := minimalDoc(t)
	member(doc, "dishes")["course"] = "basics"
	v := firstVariation(doc)
	v["regionKey"] = spainWideKey
	v["variationSlug"] = "tortilla-de-diario"
	v["recipeKey"] = "tortilla-esp-tortilla-de-diario"
	member(doc, "recipes")["recipeKey"] = "tortilla-esp-tortilla-de-diario"
	return doc
}

func TestParse_ReadsSpainWideVariation(t *testing.T) {
	b := parseString(t, encode(t, spainWideMinimalDoc(t)))

	v := b.Dishes[0].Variations[0]
	if v.RegionKey != spainWideKey || v.VariationSlug != "tortilla-de-diario" {
		t.Errorf("variation: got regionKey %q, variationSlug %q, want esp, tortilla-de-diario", v.RegionKey, v.VariationSlug)
	}
	if got := b.Dishes[0].Course; got != "basics" {
		t.Errorf("course: got %q, want basics", got)
	}
	if err := importer.Validate(b); err != nil {
		t.Errorf("Validate: got %v, want nil", err)
	}
}

func TestParse_SpainWideSchemaErrorsCarryPointers(t *testing.T) {
	tests := []struct {
		name        string
		edit        func(doc jsonDoc)
		wantPointer string
	}{
		{"slug not a string", func(doc jsonDoc) { firstVariation(doc)["variationSlug"] = 7 },
			variationPointer + "/variationSlug"},
		{"slug not a slug", func(doc jsonDoc) { firstVariation(doc)["variationSlug"] = "Tortilla de diario" },
			variationPointer + "/variationSlug"},
		{"slug too long", func(doc jsonDoc) { firstVariation(doc)["variationSlug"] = strings.Repeat("a", 65) },
			variationPointer + "/variationSlug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := spainWideMinimalDoc(t)
			tt.edit(doc)
			_, err := importer.Parse(strings.NewReader(encode(t, doc)))
			assertHasProblem(t, validationProblems(t, err), tt.wantPointer, "")
		})
	}
}

// The D-ESP-6 rules hold on the JSON as the pipeline writes it.
func TestParseAndValidate_SpainWideRules(t *testing.T) {
	tests := []struct {
		name        string
		edit        func(doc jsonDoc)
		wantPointer string
	}{
		{"esp without variationSlug", func(doc jsonDoc) { delete(firstVariation(doc), "variationSlug") },
			variationPointer + "/variationSlug"},
		{"variationSlug on a regional variation", func(doc jsonDoc) { firstVariation(doc)["regionKey"] = "gal" },
			variationPointer + "/variationSlug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := spainWideMinimalDoc(t)
			tt.edit(doc)
			assertHasProblem(t, validationProblems(t, parseAndValidate(encode(t, doc))), tt.wantPointer, "")
		})
	}
}

// A regional variation encodes without variationSlug, so bundles written
// before BE-9 keep their checksum (D-ESP-6: additive, schemaVersion 1).
func TestBundle_RegionalVariationEncodesWithoutSlug(t *testing.T) {
	data, err := json.Marshal(validBundle())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(data), "variationSlug") {
		t.Errorf("got variationSlug in a bundle with no Spanish variation, want it omitted (body=%.300s)", data)
	}
}

func TestChecksum_ChangesWithVariationSlug(t *testing.T) {
	base := importer.Checksum(spainWideBundle())
	b := spainWideBundle()
	b.Dishes[2].Variations[0].VariationSlug = "allioli-a-mano"

	if got := importer.Checksum(b); got == base {
		t.Errorf("got the unchanged checksum %s, want a different one", got)
	}
}
