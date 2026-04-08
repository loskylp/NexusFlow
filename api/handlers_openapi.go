// Package api — OpenAPI specification handler (TASK-027).
//
// GET /api/openapi.json serves the OpenAPI 3.0 specification for all
// NexusFlow REST endpoints. The spec is embedded at build time from
// api/openapi.yaml and served as JSON.
//
// This endpoint is unauthenticated so external API clients and tooling
// (e.g., swagger-ui, openapi-typescript code generation) can access it.
//
// TypeScript types for the React frontend are generated from this spec via:
//
//	npx openapi-typescript /api/openapi.json -o web/src/types/openapi.ts
//
// See: ADR-004, TASK-027, FF-011, FF-020
package api

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"

	"gopkg.in/yaml.v3"
)

// openAPISpecYAML holds the raw YAML bytes of the OpenAPI spec, embedded at
// build time from api/openapi.yaml. The path is relative to this source file.
//
//go:embed openapi.yaml
var openAPISpecYAML []byte

// MustLoadOpenAPISpecJSON converts the embedded YAML spec to JSON and returns
// the result. It panics on parse failure because a corrupt or malformed spec is
// a build-time defect, not a runtime condition — the server must not start with
// an invalid spec.
//
// Called once at startup in cmd/api/main.go to produce the []byte passed to
// NewOpenAPIHandler.
//
// Postconditions:
//   - Returns valid JSON bytes representing the OpenAPI 3.0 spec.
//   - Panics if openapi.yaml cannot be parsed or marshalled to JSON.
func MustLoadOpenAPISpecJSON() []byte {
	specJSON, err := yamlToJSON(openAPISpecYAML)
	if err != nil {
		panic(fmt.Sprintf("api: failed to convert openapi.yaml to JSON: %v", err))
	}
	return specJSON
}

// yamlToJSON converts a YAML byte slice to its JSON representation.
// Parses the YAML into an any value, then marshals it to JSON using
// encoding/json. YAML keys are preserved as-is.
//
// Args:
//
//	src: YAML-encoded bytes to convert.
//
// Returns:
//
//	JSON-encoded bytes on success.
//	An error if the YAML cannot be parsed or the result cannot be marshalled.
func yamlToJSON(src []byte) ([]byte, error) {
	var raw any
	if err := yaml.Unmarshal(src, &raw); err != nil {
		return nil, fmt.Errorf("yaml.Unmarshal: %w", err)
	}
	// yaml.v3 unmarshals maps as map[string]any — compatible with encoding/json.
	out, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal: %w", err)
	}
	return out, nil
}

// OpenAPIHandler handles GET /api/openapi.json.
type OpenAPIHandler struct {
	// specJSON holds the raw OpenAPI spec JSON, embedded at build time
	// from api/openapi.yaml via go:embed.
	SpecJSON []byte
}

// NewOpenAPIHandler constructs an OpenAPIHandler with the pre-converted JSON spec.
//
// Args:
//
//	specJSON: The OpenAPI spec as a JSON byte slice. Must be valid JSON.
//
// Preconditions:
//   - specJSON is non-nil and contains valid OpenAPI 3.0 JSON.
func NewOpenAPIHandler(specJSON []byte) *OpenAPIHandler {
	return &OpenAPIHandler{SpecJSON: specJSON}
}

// ServeSpec handles GET /api/openapi.json.
// Returns the embedded OpenAPI spec as application/json.
// The endpoint is unauthenticated (mounted outside the auth middleware chain).
//
// Response:
//
//	200: OpenAPI 3.0 JSON spec with Content-Type: application/json
//
// Postconditions:
//   - Always returns a valid JSON body.
//   - Sets Cache-Control: public, max-age=3600 to allow CDN caching.
func (h *OpenAPIHandler) ServeSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	// A write error indicates the client disconnected. Nothing to recover.
	_, _ = w.Write(h.SpecJSON)
}
