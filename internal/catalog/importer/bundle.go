// Package importer loads a content bundle, the full desired state of the
// catalog, into the database: Parse checks it against the JSON Schema,
// Validate checks the semantic rules and Apply replaces the stored catalog
// with it in one transaction.
package importer

// SchemaVersion is the bundle format this backend reads.
const SchemaVersion = 1

// Text is a human-readable field in every content locale.
type Text struct {
	En string `json:"en"`
	Es string `json:"es"`
	Ru string `json:"ru"`
}

// Photos are CDN-relative key prefixes, one per role; nil means no photo yet.
type Photos struct {
	Thumb *string `json:"thumb"`
	Card  *string `json:"card"`
	Hero  *string `json:"hero"`
}

type Region struct {
	Key            string   `json:"key"`
	TileColor      string   `json:"tileColor"`
	Name           Text     `json:"name"`
	MapLabel       Text     `json:"mapLabel"`
	Description    Text     `json:"description"`
	RegionalDishes []string `json:"regionalDishes"` // Spanish proper names, not translated
}

type Flags struct {
	Vegetarian bool `json:"vegetarian"`
	Seafood    bool `json:"seafood"`
	Pork       bool `json:"pork"`
	Quick      bool `json:"quick"`
	SlowCooked bool `json:"slowCooked"`
}

type Dish struct {
	Key         string      `json:"key"`
	Name        Text        `json:"name"`
	Description Text        `json:"description"`
	Flags       Flags       `json:"flags"`
	Course      string      `json:"course"`
	Season      string      `json:"season"`
	Photos      *Photos     `json:"photos"`
	Variations  []Variation `json:"variations"`
}

// Variation is a dish as cooked in one region; RecipeKey names its recipe.
type Variation struct {
	RegionKey   string  `json:"regionKey"`
	RecipeKey   string  `json:"recipeKey"`
	Name        Text    `json:"name"`
	Description Text    `json:"description"`
	Photos      *Photos `json:"photos"`
}

type Ingredient struct {
	Quantity float64 `json:"quantity"`
	Unit     *string `json:"unit"` // nil is a counted item
	Name     Text    `json:"name"`
}

type Recipe struct {
	RecipeKey    string       `json:"recipeKey"`
	TimeMinutes  int          `json:"timeMinutes"`
	TimeNote     *Text        `json:"timeNote"`
	BaseServings int          `json:"baseServings"`
	Ingredients  []Ingredient `json:"ingredients"`
	Steps        []Text       `json:"steps"`
}

// Bundle lists regions and dishes in editorial order, and one recipe per
// variation.
type Bundle struct {
	SchemaVersion int      `json:"schemaVersion"`
	Regions       []Region `json:"regions"`
	Dishes        []Dish   `json:"dishes"`
	Recipes       []Recipe `json:"recipes"`
}

// locales are the content locale codes, in the order of values.
var locales = []string{"en", "es", "ru"}

// values returns the texts in the order of locales.
func (t Text) values() []string {
	return []string{t.En, t.Es, t.Ru}
}
