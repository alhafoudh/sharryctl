// Package api embeds the bundled OpenAPI specification.
package api

import _ "embed"

// OpenAPI is the bundled OpenAPI 3 spec (YAML).
//
//go:embed openapi.yaml
var OpenAPI []byte
