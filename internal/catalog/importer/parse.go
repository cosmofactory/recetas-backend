package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/cosmofactory/recetas-backend/api"
)

// MaxBundleBytes bounds the input Parse reads.
const MaxBundleBytes = 16 << 20

const schemaURL = "https://asadorov.es/schemas/content-bundle.schema.json"

var (
	bundleSchema  = sync.OnceValues(compileSchema)
	schemaPrinter = message.NewPrinter(language.English)
)

func compileSchema() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(api.ContentBundleSchema))
	if err != nil {
		return nil, fmt.Errorf("decode bundle schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaURL, doc); err != nil {
		return nil, fmt.Errorf("add bundle schema: %w", err)
	}
	sch, err := c.Compile(schemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile bundle schema: %w", err)
	}
	return sch, nil
}

// Parse reads a bundle and checks it against the JSON Schema. Malformed JSON,
// an oversized input and schema violations are a *ValidationError; the
// semantic rules are Validate's.
func Parse(r io.Reader) (Bundle, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBundleBytes+1))
	if err != nil {
		return Bundle{}, fmt.Errorf("read bundle: %w", err)
	}
	if len(data) > MaxBundleBytes {
		return Bundle{}, invalidDocument("bundle is larger than %d bytes", MaxBundleBytes)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return Bundle{}, invalidDocument("invalid JSON: %v", err)
	}
	if err := checkSchemaVersion(data); err != nil {
		return Bundle{}, err
	}
	if err := checkSchema(doc); err != nil {
		return Bundle{}, err
	}
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return Bundle{}, invalidDocument("decode bundle: %v", err)
	}
	roundQuantities(b.Recipes)
	return b, nil
}

// checkSchemaVersion rejects a bundle of another format by its version alone,
// before the schema reports the fields this importer doesn't know. A missing
// or non-numeric version is left to the schema.
func checkSchemaVersion(data []byte) error {
	var probe struct {
		SchemaVersion *float64 `json:"schemaVersion"`
	}
	if json.Unmarshal(data, &probe) != nil || probe.SchemaVersion == nil || *probe.SchemaVersion == SchemaVersion {
		return nil
	}
	var ps problems
	ps.add("/schemaVersion", "unsupported schemaVersion %v; this importer supports %d", *probe.SchemaVersion, SchemaVersion)
	return ps.err()
}

// roundQuantities rounds to the 3 decimals the database stores, so float
// noise neither changes the checksum nor shows up as an update.
func roundQuantities(recipes []Recipe) {
	for _, r := range recipes {
		for i := range r.Ingredients {
			r.Ingredients[i].Quantity = roundQuantity(r.Ingredients[i].Quantity)
		}
	}
}

func roundQuantity(q float64) float64 {
	return math.Round(q*quantityScale) / quantityScale
}

func invalidDocument(format string, args ...any) error {
	var ps problems
	ps.add("", format, args...)
	return ps.err()
}

func checkSchema(doc any) error {
	sch, err := bundleSchema()
	if err != nil {
		return err
	}
	err = sch.Validate(doc)
	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		return err
	}
	var ps problems
	collectSchemaProblems(&ps, verr)
	return ps.err()
}

// collectSchemaProblems adds the leaves of the error tree: the failed
// keywords themselves, not the $ref and group nodes above them.
func collectSchemaProblems(ps *problems, e *jsonschema.ValidationError) {
	if len(e.Causes) == 0 {
		ps.add(pointer("").at(tokens(e.InstanceLocation)...), "%s", e.ErrorKind.LocalizedString(schemaPrinter))
		return
	}
	for _, cause := range e.Causes {
		collectSchemaProblems(ps, cause)
	}
}

func tokens(location []string) []any {
	out := make([]any, len(location))
	for i, tok := range location {
		out[i] = tok
	}
	return out
}
