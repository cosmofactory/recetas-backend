// Command designbundle converts the design handoff's content.json into an
// import bundle (testdata/content.design.json): recipe keys become
// <dishKey>-<regionKey>, recipe times become minutes plus a time note, photos
// are null, and every variation without a design recipe gets a placeholder.
//
//	go run ./scripts/designbundle -in ../design/design_handoff_asadorov_desktop/content.json -out testdata/content.design.json
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// The design's content.json shape where it differs from the bundle's.
type (
	design struct {
		Note    string            `json:"_note"`
		Regions []importer.Region `json:"regions"`
		Dishes  []importer.Dish   `json:"dishes"`
		Recipes []designRecipe    `json:"recipes"`
	}
	designRecipe struct {
		RecipeKey    string                `json:"recipeKey"` // free text, matching a variation's
		BaseServings int                   `json:"baseServings"`
		Time         importer.Text         `json:"time"` // e.g. "1 h 15 min", "3 h + soaking"
		Ingredients  []importer.Ingredient `json:"ingredients"`
		Steps        map[string][]string   `json:"steps"` // by locale
	}
)

func main() {
	in := flag.String("in", "", "design content.json")
	out := flag.String("out", "", "bundle file to write")
	flag.Parse()
	if *in == "" || *out == "" {
		_, _ = fmt.Fprintln(os.Stderr, "usage: designbundle -in content.json -out bundle.json")
		os.Exit(2)
	}
	if err := run(*in, *out); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "designbundle:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	d, err := readDesign(in)
	if err != nil {
		return err
	}
	b, err := convert(d)
	if err != nil {
		return err
	}
	if err := importer.Validate(b); err != nil {
		return fmt.Errorf("converted bundle: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return fmt.Errorf("encode bundle: %w", err)
	}
	return os.WriteFile(filepath.Clean(out), buf.Bytes(), 0o600)
}

func readDesign(path string) (design, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return design{}, fmt.Errorf("read design content: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var d design
	if err := dec.Decode(&d); err != nil {
		return design{}, fmt.Errorf("decode design content: %w", err)
	}
	return d, nil
}
