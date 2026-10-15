// Package api embeds the OpenAPI contract so the server can serve it, and the
// content bundle schema that the import validates against.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPIYAML []byte

// ContentBundleSchema is the JSON Schema (2020-12) of an import bundle.
//
//go:embed content-bundle.schema.json
var ContentBundleSchema []byte
