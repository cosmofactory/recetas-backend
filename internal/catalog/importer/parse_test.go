package importer_test

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// minimalBundleJSON is the smallest valid bundle: one of everything, no
// photos and no time note (both given as explicit nulls).
const minimalBundleJSON = `{
  "schemaVersion": 1,
  "regions": [{
    "key": "gal",
    "tileColor": "#9A9A5E",
    "name": {"en": "Galicia", "es": "Galicia", "ru": "Галисия"},
    "mapLabel": {"en": "Galicia", "es": "Galicia", "ru": "Галисия"},
    "description": {"en": "Atlantic seafood.", "es": "Marisco atlántico.", "ru": "Атлантические морепродукты."},
    "regionalDishes": ["Pulpo á feira"]
  }],
  "dishes": [{
    "key": "tortilla",
    "name": {"en": "Tortilla de patatas", "es": "Tortilla de patatas", "ru": "Тортилья"},
    "description": {"en": "One dish.", "es": "Un plato.", "ru": "Одно блюдо."},
    "flags": {"vegetarian": true, "seafood": false, "pork": false, "quick": true, "slowCooked": false},
    "course": "tapas",
    "season": "all",
    "photos": null,
    "variations": [{
      "regionKey": "gal",
      "recipeKey": "tortilla-betanzos",
      "name": {"en": "Betanzos-style runny", "es": "Estilo Betanzos, jugosa", "ru": "По-бетансосски"},
      "description": {"en": "Barely set.", "es": "Apenas cuajada.", "ru": "Едва схваченная."},
      "photos": null
    }]
  }],
  "recipes": [{
    "recipeKey": "tortilla-betanzos",
    "timeMinutes": 25,
    "timeNote": null,
    "baseServings": 2,
    "ingredients": [{"quantity": 4, "unit": null, "name": {"en": "Eggs", "es": "Huevos", "ru": "Яйца"}}],
    "steps": [{"en": "Beat the eggs.", "es": "Bate los huevos.", "ru": "Взбейте яйца."}]
  }]
}`

// jsonDoc is a decoded JSON document that tests edit before re-encoding.
type jsonDoc = map[string]any

func minimalDoc(t *testing.T) jsonDoc {
	t.Helper()
	var doc jsonDoc
	if err := json.Unmarshal([]byte(minimalBundleJSON), &doc); err != nil {
		t.Fatalf("decode minimal bundle: %v", err)
	}
	return doc
}

// member returns the object at doc[name][0], e.g. the first region.
func member(doc jsonDoc, name string) jsonDoc {
	return doc[name].([]any)[0].(jsonDoc)
}

func firstVariation(doc jsonDoc) jsonDoc {
	return member(member(doc, "dishes"), "variations")
}

func encode(t *testing.T, doc jsonDoc) string {
	t.Helper()
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode bundle: %v", err)
	}
	return string(out)
}

func TestParse_MinimalBundle(t *testing.T) {
	b, err := importer.Parse(strings.NewReader(minimalBundleJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := importer.Bundle{
		SchemaVersion: 1,
		Regions: []importer.Region{{
			Key:            "gal",
			TileColor:      "#9A9A5E",
			Name:           importer.Text{En: "Galicia", Es: "Galicia", Ru: "Галисия"},
			MapLabel:       importer.Text{En: "Galicia", Es: "Galicia", Ru: "Галисия"},
			Description:    importer.Text{En: "Atlantic seafood.", Es: "Marisco atlántico.", Ru: "Атлантические морепродукты."},
			RegionalDishes: []string{"Pulpo á feira"},
		}},
		Dishes: []importer.Dish{{
			Key:         "tortilla",
			Name:        importer.Text{En: "Tortilla de patatas", Es: "Tortilla de patatas", Ru: "Тортилья"},
			Description: importer.Text{En: "One dish.", Es: "Un plato.", Ru: "Одно блюдо."},
			Flags:       importer.Flags{Vegetarian: true, Quick: true},
			Course:      "tapas",
			Season:      "all",
			Variations: []importer.Variation{{
				RegionKey:   "gal",
				RecipeKey:   "tortilla-betanzos",
				Name:        importer.Text{En: "Betanzos-style runny", Es: "Estilo Betanzos, jugosa", Ru: "По-бетансосски"},
				Description: importer.Text{En: "Barely set.", Es: "Apenas cuajada.", Ru: "Едва схваченная."},
			}},
		}},
		Recipes: []importer.Recipe{{
			RecipeKey:    "tortilla-betanzos",
			TimeMinutes:  25,
			BaseServings: 2,
			Ingredients:  []importer.Ingredient{{Quantity: 4, Name: importer.Text{En: "Eggs", Es: "Huevos", Ru: "Яйца"}}},
			Steps:        []importer.Text{{En: "Beat the eggs.", Es: "Bate los huevos.", Ru: "Взбейте яйца."}},
		}},
	}
	if !reflect.DeepEqual(b, want) {
		t.Errorf("got %+v, want %+v", b, want)
	}
}

func TestParse_OptionalFieldsAndPhotos(t *testing.T) {
	doc := minimalDoc(t)
	delete(firstVariation(doc), "photos")
	delete(member(doc, "recipes"), "timeNote")
	member(doc, "dishes")["photos"] = jsonDoc{
		"thumb": "dishes/tortilla/thumb-1a2b3c4d", "card": nil, "hero": "dishes/tortilla/hero-9c0d1e2f",
	}
	rc := member(doc, "recipes")
	rc["timeNote"] = jsonDoc{"en": "+ soaking", "es": "+ remojo", "ru": "+ замачивание"}
	member(rc, "ingredients")["quantity"] = 1.5
	member(rc, "ingredients")["unit"] = "tsp"

	b, err := importer.Parse(strings.NewReader(encode(t, doc)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(b.Dishes) != 1 || len(b.Dishes[0].Variations) != 1 || len(b.Recipes) != 1 || len(b.Recipes[0].Ingredients) != 1 {
		t.Fatalf("got %+v, want one dish, variation, recipe and ingredient", b)
	}
	wantPhotos := &importer.Photos{Thumb: strp("dishes/tortilla/thumb-1a2b3c4d"), Hero: strp("dishes/tortilla/hero-9c0d1e2f")}
	if got := b.Dishes[0].Photos; !reflect.DeepEqual(got, wantPhotos) {
		t.Errorf("dish photos: got %+v, want %+v", got, wantPhotos)
	}
	if got := b.Dishes[0].Variations[0].Photos; got != nil {
		t.Errorf("omitted variation photos: got %+v, want nil", got)
	}
	wantNote := &importer.Text{En: "+ soaking", Es: "+ remojo", Ru: "+ замачивание"}
	if got := b.Recipes[0].TimeNote; !reflect.DeepEqual(got, wantNote) {
		t.Errorf("time note: got %+v, want %+v", got, wantNote)
	}
	ing := b.Recipes[0].Ingredients[0]
	if ing.Quantity != 1.5 || ing.Unit == nil || *ing.Unit != "tsp" {
		t.Errorf("ingredient: got %v %v, want 1.5 tsp", ing.Quantity, ing.Unit)
	}
}

func TestParse_SchemaErrorsCarryPointers(t *testing.T) {
	tests := []struct {
		name        string
		edit        func(doc jsonDoc)
		wantPointer string
		wantInMsg   string // the offending field name, when the pointer is its parent
	}{
		{
			name:        "missing required field points at its object",
			edit:        func(doc jsonDoc) { delete(member(doc, "regions"), "tileColor") },
			wantPointer: "/regions/0",
			wantInMsg:   "tileColor",
		},
		{
			name:        "missing schemaVersion points at the root",
			edit:        func(doc jsonDoc) { delete(doc, "schemaVersion") },
			wantPointer: "",
			wantInMsg:   "schemaVersion",
		},
		{
			name:        "missing nested translation",
			edit:        func(doc jsonDoc) { delete(firstVariation(doc)["name"].(jsonDoc), "ru") },
			wantPointer: "/dishes/0/variations/0/name",
			wantInMsg:   "ru",
		},
		{
			name:        "wrong type",
			edit:        func(doc jsonDoc) { member(doc, "recipes")["baseServings"] = "2" },
			wantPointer: "/recipes/0/baseServings",
		},
		{
			name:        "wrong type for a text",
			edit:        func(doc jsonDoc) { member(doc, "dishes")["name"] = "Tortilla" },
			wantPointer: "/dishes/0/name",
		},
		{
			name:        "unknown field",
			edit:        func(doc jsonDoc) { member(doc, "dishes")["rating"] = 5 },
			wantPointer: "/dishes/0",
			wantInMsg:   "rating",
		},
		{
			name:        "unknown top-level field",
			edit:        func(doc jsonDoc) { doc["extras"] = []any{} },
			wantPointer: "",
			wantInMsg:   "extras",
		},
		{
			name:        "unknown locale",
			edit:        func(doc jsonDoc) { member(doc, "regions")["name"].(jsonDoc)["fr"] = "Galice" },
			wantPointer: "/regions/0/name",
			wantInMsg:   "fr",
		},
		{
			name:        "bad course enum",
			edit:        func(doc jsonDoc) { member(doc, "dishes")["course"] = "brunch" },
			wantPointer: "/dishes/0/course",
		},
		{
			name:        "bad season enum",
			edit:        func(doc jsonDoc) { member(doc, "dishes")["season"] = "autumn" },
			wantPointer: "/dishes/0/season",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := minimalDoc(t)
			tt.edit(doc)
			_, err := importer.Parse(strings.NewReader(encode(t, doc)))
			assertHasProblem(t, validationProblems(t, err), tt.wantPointer, tt.wantInMsg)
		})
	}
}

func TestParse_MalformedJSONIsAValidationError(t *testing.T) {
	for name, input := range map[string]string{
		"garbage":   "{not json",
		"truncated": minimalBundleJSON[:len(minimalBundleJSON)/2],
		"empty":     "",
		"array":     "[]",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := importer.Parse(strings.NewReader(input))
			validationProblems(t, err)
		})
	}
}

// assertHasProblem fails unless a problem sits at pointer and, when
// wantInMsg is set, mentions it.
func assertHasProblem(t *testing.T, problems []importer.Problem, pointer, wantInMsg string) {
	t.Helper()
	for _, p := range problems {
		if p.Pointer == pointer && strings.Contains(p.Message, wantInMsg) {
			return
		}
	}
	t.Errorf("got problems %+v, want one at %q mentioning %q", problems, pointer, wantInMsg)
}

// A bundle of a newer format is rejected for its version, whatever else in it
// this backend doesn't know.
func TestParse_UnsupportedSchemaVersion(t *testing.T) {
	tests := []struct {
		name string
		edit func(doc jsonDoc)
	}{
		{"otherwise valid", func(doc jsonDoc) { doc["schemaVersion"] = 2 }},
		{"with an unknown top-level field", func(doc jsonDoc) {
			doc["schemaVersion"] = 2
			doc["collections"] = []any{}
		}},
	}
	supported := strconv.Itoa(importer.SchemaVersion)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := minimalDoc(t)
			tt.edit(doc)
			_, err := importer.Parse(strings.NewReader(encode(t, doc)))
			assertHasProblem(t, validationProblems(t, err), "/schemaVersion", supported)
		})
	}
}

func TestParse_RoundsQuantitiesToThreeDecimals(t *testing.T) {
	doc := minimalDoc(t)
	member(member(doc, "recipes"), "ingredients")["quantity"] = noisyThreeTenths()
	input := encode(t, doc)
	if !strings.Contains(input, "0.30000000000000004") {
		t.Fatalf("got input %s, want the float noise in it", input)
	}

	b := parseString(t, input)

	if got := b.Recipes[0].Ingredients[0].Quantity; got != 0.3 {
		t.Errorf("quantity: got %v, want 0.3", got)
	}
	member(member(doc, "recipes"), "ingredients")["quantity"] = 0.3
	if got, want := importer.Checksum(b), importer.Checksum(parseString(t, encode(t, doc))); got != want {
		t.Errorf("checksum: got %s, want %s as for 0.3", got, want)
	}
}

// A quantity that rounds to zero at the stored 3 decimals is rejected, not
// left for the database to refuse.
func TestParse_QuantityRoundingToZeroIsRejected(t *testing.T) {
	for _, q := range []float64{1e-10, 0.0004} {
		t.Run(strconv.FormatFloat(q, 'g', -1, 64), func(t *testing.T) {
			doc := minimalDoc(t)
			member(member(doc, "recipes"), "ingredients")["quantity"] = q
			err := parseAndValidate(encode(t, doc))
			assertHasProblem(t, validationProblems(t, err), "/recipes/0/ingredients/0/quantity", "")
		})
	}
}

// parseAndValidate is what the import CLI does before touching the database.
func parseAndValidate(input string) error {
	b, err := importer.Parse(strings.NewReader(input))
	if err != nil {
		return err
	}
	return importer.Validate(b)
}
