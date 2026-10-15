package importer_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

func parseString(t *testing.T, input string) importer.Bundle {
	t.Helper()
	b, err := importer.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return b
}

func TestChecksum_IsSHA256Hex(t *testing.T) {
	if got := importer.Checksum(validBundle()); !sha256Hex.MatchString(got) {
		t.Errorf("got %q, want 64 lowercase hex digits", got)
	}
}

func TestChecksum_IgnoresFormattingOfTheInput(t *testing.T) {
	// Re-encoding through a map sorts the keys, unlike minimalBundleJSON.
	reordered, err := json.MarshalIndent(minimalDoc(t), "", "\t\t")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	compact := encode(t, minimalDoc(t))
	want := importer.Checksum(parseString(t, minimalBundleJSON))
	for name, input := range map[string]string{
		"sorted keys, tab indented": string(reordered),
		"compact":                   compact,
		"surrounding whitespace":    "\n  " + minimalBundleJSON + "\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := importer.Checksum(parseString(t, input)); got != want {
				t.Errorf("got %s, want %s", got, want)
			}
		})
	}
}

func TestChecksum_IsDeterministic(t *testing.T) {
	if a, b := importer.Checksum(validBundle()), importer.Checksum(validBundle()); a != b {
		t.Errorf("got %s then %s, want equal", a, b)
	}
}

func TestChecksum_ChangesWithContent(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
	}{
		{"region name", func(b *importer.Bundle) { b.Regions[0].Name.Ru = "Галисия" }},
		{"region map label", func(b *importer.Bundle) { b.Regions[0].MapLabel.En = "Gal." }},
		{"regional dish", func(b *importer.Bundle) { b.Regions[1].RegionalDishes[0] = "Fabada" }},
		{"tile color", func(b *importer.Bundle) { b.Regions[2].TileColor = "#000000" }},
		{"region order", func(b *importer.Bundle) { b.Regions[0], b.Regions[1] = b.Regions[1], b.Regions[0] }},
		{"dish order", func(b *importer.Bundle) { b.Dishes[0], b.Dishes[1] = b.Dishes[1], b.Dishes[0] }},
		{"dish description", func(b *importer.Bundle) { b.Dishes[0].Description.Es = "Otra" }},
		{"dish flag", func(b *importer.Bundle) { b.Dishes[0].Flags.Seafood = true }},
		{"dish season", func(b *importer.Bundle) { b.Dishes[0].Season = "summer" }},
		{"dish photo", func(b *importer.Bundle) { b.Dishes[0].Photos.Hero = strp("dishes/tortilla/hero-ffffffff") }},
		{"variation name", func(b *importer.Bundle) { b.Dishes[1].Variations[1].Name.En = "Fabada" }},
		{"variation photo removed", func(b *importer.Bundle) { b.Dishes[0].Variations[0].Photos = nil }},
		{"time", func(b *importer.Bundle) { b.Recipes[0].TimeMinutes = 26 }},
		{"time note", func(b *importer.Bundle) { b.Recipes[3].TimeNote.En = "+ overnight soaking" }},
		{"servings", func(b *importer.Bundle) { b.Recipes[0].BaseServings = 3 }},
		{"ingredient quantity", func(b *importer.Bundle) { b.Recipes[0].Ingredients[3].Quantity = 1.25 }},
		{"ingredient unit", func(b *importer.Bundle) { b.Recipes[0].Ingredients[0].Unit = strp("g") }},
		{"ingredient name", func(b *importer.Bundle) { b.Recipes[1].Ingredients[0].Name.Ru = "Яйца" }},
		{"step text", func(b *importer.Bundle) { b.Recipes[2].Steps[1].Es = "Sirve." }},
	}
	base := importer.Checksum(validBundle())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := validBundle()
			tt.edit(&b)
			if got := importer.Checksum(b); got == base {
				t.Errorf("got the unchanged checksum %s, want a different one", got)
			}
		})
	}
}
