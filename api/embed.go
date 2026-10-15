// Package api embeds the OpenAPI contract so the server can serve it.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPIYAML []byte
