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
//   npx openapi-typescript /api/openapi.json -o web/src/types/openapi.ts
//
// See: ADR-004, TASK-027, FF-011, FF-020
package api

import (
	"net/http"
)

// OpenAPIHandler handles GET /api/openapi.json.
type OpenAPIHandler struct {
	// specJSON holds the raw OpenAPI spec JSON, embedded at build time
	// from api/openapi.yaml via go:embed.
	specJSON []byte
}

// NewOpenAPIHandler constructs an OpenAPIHandler.
// specPath is the embedded YAML spec converted to JSON at startup.
//
// Args:
//
//	specJSON: The OpenAPI spec as a JSON byte slice. Must be valid JSON.
//
// Preconditions:
//   - specJSON is non-nil and contains valid OpenAPI 3.0 JSON.
func NewOpenAPIHandler(specJSON []byte) *OpenAPIHandler {
	// TODO: implement
	panic("not implemented")
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
	// TODO: implement
	panic("not implemented")
}
