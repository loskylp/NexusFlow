// Package api — unit tests for OpenAPIHandler (GET /api/openapi.json).
// Tests confirm the handler serves valid JSON with the correct Content-Type
// and Cache-Control headers, regardless of what spec bytes are injected.
// Also tests yamlToJSON conversion and MustLoadOpenAPISpecJSON.
// See: ADR-004, TASK-027
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// minimalSpecJSON is a minimal valid OpenAPI 3.0 JSON document used in tests.
// It is not the production spec — it is the smallest JSON object that lets the
// tests verify structural invariants (Content-Type, Cache-Control, valid JSON body).
const minimalSpecJSON = `{"openapi":"3.0.3","info":{"title":"Test","version":"1.0.0"},"paths":{}}`

// TestNewOpenAPIHandler_StoresSpecJSON verifies that NewOpenAPIHandler stores the
// supplied bytes so ServeSpec can return them later.
func TestNewOpenAPIHandler_StoresSpecJSON(t *testing.T) {
	spec := []byte(minimalSpecJSON)
	h := NewOpenAPIHandler(spec)
	if h == nil {
		t.Fatal("NewOpenAPIHandler returned nil")
	}
	if string(h.SpecJSON) != minimalSpecJSON {
		t.Errorf("SpecJSON mismatch: expected %q, got %q", minimalSpecJSON, string(h.SpecJSON))
	}
}

// TestServeSpec_Returns200 verifies that GET /api/openapi.json returns HTTP 200.
func TestServeSpec_Returns200(t *testing.T) {
	h := NewOpenAPIHandler([]byte(minimalSpecJSON))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	h.ServeSpec(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestServeSpec_ContentTypeIsApplicationJSON verifies that the response carries
// Content-Type: application/json, which is required for browser and tooling
// (e.g. swagger-ui, openapi-typescript) to correctly parse the body.
func TestServeSpec_ContentTypeIsApplicationJSON(t *testing.T) {
	h := NewOpenAPIHandler([]byte(minimalSpecJSON))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	h.ServeSpec(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: expected application/json, got %q", ct)
	}
}

// TestServeSpec_CacheControlIsPublic verifies the Cache-Control header is set to
// allow CDN caching per the handler contract: public, max-age=3600.
func TestServeSpec_CacheControlIsPublic(t *testing.T) {
	h := NewOpenAPIHandler([]byte(minimalSpecJSON))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	h.ServeSpec(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if cc != "public, max-age=3600" {
		t.Errorf("Cache-Control: expected %q, got %q", "public, max-age=3600", cc)
	}
}

// TestServeSpec_BodyIsValidJSON verifies that the response body is parseable as JSON.
// This guards against the spec bytes being truncated or corrupted during embedding.
func TestServeSpec_BodyIsValidJSON(t *testing.T) {
	h := NewOpenAPIHandler([]byte(minimalSpecJSON))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	h.ServeSpec(rec, req)

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("response body is not valid JSON: %v\nbody: %s", err, rec.Body.String())
	}
}

// TestServeSpec_BodyMatchesInjectedSpec verifies that the exact bytes injected at
// construction time are returned by ServeSpec unchanged.
func TestServeSpec_BodyMatchesInjectedSpec(t *testing.T) {
	spec := []byte(minimalSpecJSON)
	h := NewOpenAPIHandler(spec)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	h.ServeSpec(rec, req)

	// Trim any trailing newline added by the handler before comparison.
	got := rec.Body.Bytes()
	// Allow a single trailing newline — json.Encoder adds one.
	trimmed := got
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	if string(trimmed) != minimalSpecJSON {
		t.Errorf("body mismatch:\nexpected: %s\ngot:      %s", minimalSpecJSON, string(trimmed))
	}
}

// TestServeSpec_IsUnauthenticated verifies that ServeSpec does not require a session
// in the request context — it is mounted outside the auth middleware group.
// This test confirms the handler does not call auth.SessionFromContext and does not
// return 401 when no session is present.
func TestServeSpec_IsUnauthenticated(t *testing.T) {
	h := NewOpenAPIHandler([]byte(minimalSpecJSON))
	rec := httptest.NewRecorder()
	// Request with no Authorization header and no session in context.
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	h.ServeSpec(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Error("ServeSpec returned 401 — endpoint must be public (no auth required)")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// minimalSpecYAML is a minimal valid OpenAPI YAML document used to test yamlToJSON.
const minimalSpecYAML = `openapi: "3.0.3"
info:
  title: Test
  version: "1.0.0"
paths: {}`

// TestYamlToJSON_ValidYAMLProducesValidJSON verifies that a valid YAML input is
// converted to a parseable JSON byte slice with the same field values.
func TestYamlToJSON_ValidYAMLProducesValidJSON(t *testing.T) {
	out, err := yamlToJSON([]byte(minimalSpecYAML))
	if err != nil {
		t.Fatalf("yamlToJSON returned unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("yamlToJSON output is not valid JSON: %v\noutput: %s", err, string(out))
	}
	if result["openapi"] != "3.0.3" {
		t.Errorf("openapi field: expected 3.0.3, got %v", result["openapi"])
	}
}

// TestYamlToJSON_InvalidYAMLReturnsError verifies that malformed YAML input causes
// yamlToJSON to return a non-nil error and nil bytes.
func TestYamlToJSON_InvalidYAMLReturnsError(t *testing.T) {
	// A mapping key that contains a colon and space but has inconsistent block
	// scalar indentation: the following is deliberately structurally invalid YAML
	// that go-yaml v3 cannot parse (unclosed flow sequence).
	invalidYAML := []byte("key: [unclosed bracket")
	out, err := yamlToJSON(invalidYAML)
	if err == nil {
		t.Errorf("yamlToJSON expected error for invalid YAML, got nil (output: %s)", string(out))
	}
	if out != nil {
		t.Errorf("yamlToJSON expected nil output on error, got %s", string(out))
	}
}

// TestYamlToJSON_PreservesStringFields verifies that string fields are preserved
// intact during the YAML→JSON conversion round-trip.
func TestYamlToJSON_PreservesStringFields(t *testing.T) {
	yaml := []byte(`name: "NexusFlow"
version: "1.0.0"`)
	out, err := yamlToJSON(yaml)
	if err != nil {
		t.Fatalf("yamlToJSON error: %v", err)
	}
	if !strings.Contains(string(out), "NexusFlow") {
		t.Errorf("converted JSON missing 'NexusFlow': %s", string(out))
	}
}

// TestMustLoadOpenAPISpecJSON_ReturnsValidJSON verifies that the embedded openapi.yaml
// is successfully converted to valid JSON bytes at startup. This test exercises the
// actual go:embed path — if openapi.yaml is malformed, this test panics and the build
// is considered broken.
func TestMustLoadOpenAPISpecJSON_ReturnsValidJSON(t *testing.T) {
	// MustLoadOpenAPISpecJSON panics on a malformed spec. If this test runs without
	// panic, the embedded YAML is well-formed and parseable.
	specJSON := MustLoadOpenAPISpecJSON()
	if len(specJSON) == 0 {
		t.Fatal("MustLoadOpenAPISpecJSON returned empty bytes")
	}
	var result map[string]any
	if err := json.Unmarshal(specJSON, &result); err != nil {
		t.Fatalf("MustLoadOpenAPISpecJSON output is not valid JSON: %v", err)
	}
	// Verify the spec has the expected top-level OpenAPI version field.
	if result["openapi"] == nil {
		t.Error("converted spec is missing 'openapi' field")
	}
}

// TestMustLoadOpenAPISpecJSON_ContainsAllExpectedPaths verifies that the embedded
// spec documents the core REST endpoints so the spec cannot accidentally be
// deployed in a partial or empty state.
func TestMustLoadOpenAPISpecJSON_ContainsAllExpectedPaths(t *testing.T) {
	specJSON := MustLoadOpenAPISpecJSON()

	var result map[string]any
	if err := json.Unmarshal(specJSON, &result); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	paths, ok := result["paths"].(map[string]any)
	if !ok {
		t.Fatalf("'paths' field is missing or not an object")
	}

	required := []string{
		"/api/health",
		"/api/openapi.json",
		"/api/auth/login",
		"/api/auth/logout",
		"/api/tasks",
		"/api/tasks/{id}",
		"/api/tasks/{id}/cancel",
		"/api/tasks/{id}/logs",
		"/api/pipelines",
		"/api/pipelines/{id}",
		"/api/workers",
		"/api/chains",
		"/api/chains/{id}",
		"/api/users",
		"/api/users/{id}/deactivate",
	}
	for _, path := range required {
		if _, exists := paths[path]; !exists {
			t.Errorf("spec is missing expected path: %s", path)
		}
	}
}
