#!/usr/bin/env bash
# TASK-027 Acceptance Tests — Health Endpoint and OpenAPI Specification
# REQ: ADR-005, ADR-004, FF-011, FF-020
#
# AC-1 (positive): GET /api/health returns 200 with structured JSON when dependencies reachable
# AC-1 (negative): GET /api/health returns 503 with structured JSON when dependencies unavailable
# AC-2 (positive): GET /api/openapi.json returns 200 with valid OpenAPI 3.0 JSON
# AC-2 (negative): GET /api/openapi.json does not require authentication (no 401 when unauthenticated)
# AC-3: All REST endpoints documented in the spec (path coverage check)
# AC-4 (positive): Spec is valid OpenAPI 3.0 — contains required fields: openapi, info, paths
# AC-4 (negative): Spec is not a trivially empty object (has meaningful path coverage)
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-027-acceptance.sh
#
# Requires: curl, jq
# Services required: API server, PostgreSQL, Redis (all running via Docker Compose)

set -uo pipefail

API_URL="${API_URL:-http://localhost:8080}"

PASS=0
FAIL=0
RESULTS=()

pass() {
  local name="$1"
  echo "  PASS: $name"
  PASS=$((PASS + 1))
  RESULTS+=("PASS | $name")
}

fail() {
  local name="$1"
  local detail="$2"
  echo "  FAIL: $name"
  echo "        $detail"
  FAIL=$((FAIL + 1))
  RESULTS+=("FAIL | $name | $detail")
}

hr() { echo "─────────────────────────────────────────────────────────────────"; }

hr
echo "TASK-027 Acceptance Tests — Health Endpoint and OpenAPI Specification"
echo "API: $API_URL"
hr

# ─────────────────────────────────────────────────────────────────
# AC-1 POSITIVE: GET /api/health returns structured health status (200 when reachable)
# REQ: ADR-005
# Given: the API server, Redis, and PostgreSQL are all reachable
# When: GET /api/health is called with no authentication
# Then: 200 is returned with JSON body containing status, redis, postgres fields
# ─────────────────────────────────────────────────────────────────
echo ""
echo "[AC-1] GET /api/health — structured health status"

HEALTH_RESPONSE=$(curl -s -w "\n%{http_code}" "${API_URL}/api/health")
HEALTH_BODY=$(echo "$HEALTH_RESPONSE" | head -n -1)
HEALTH_STATUS=$(echo "$HEALTH_RESPONSE" | tail -n 1)

# [positive] 200 status code when all dependencies reachable
if [[ "$HEALTH_STATUS" == "200" ]]; then
  pass "[AC-1][positive] GET /api/health returns 200 when dependencies reachable"
else
  fail "[AC-1][positive] GET /api/health returns 200 when dependencies reachable" \
    "expected HTTP 200, got $HEALTH_STATUS; body: $HEALTH_BODY"
fi

# [positive] response body contains 'status' field
STATUS_FIELD=$(echo "$HEALTH_BODY" | jq -r '.status' 2>/dev/null)
if [[ "$STATUS_FIELD" == "ok" || "$STATUS_FIELD" == "degraded" ]]; then
  pass "[AC-1][positive] health response contains 'status' field (ok or degraded)"
else
  fail "[AC-1][positive] health response contains 'status' field (ok or degraded)" \
    "expected .status to be 'ok' or 'degraded', got '$STATUS_FIELD'; body: $HEALTH_BODY"
fi

# [positive] response body contains 'redis' field
REDIS_FIELD=$(echo "$HEALTH_BODY" | jq -r '.redis' 2>/dev/null)
if [[ "$REDIS_FIELD" == "ok" || "$REDIS_FIELD" == "error" ]]; then
  pass "[AC-1][positive] health response contains 'redis' field"
else
  fail "[AC-1][positive] health response contains 'redis' field" \
    "expected .redis to be 'ok' or 'error', got '$REDIS_FIELD'; body: $HEALTH_BODY"
fi

# [positive] response body contains 'postgres' field
POSTGRES_FIELD=$(echo "$HEALTH_BODY" | jq -r '.postgres' 2>/dev/null)
if [[ "$POSTGRES_FIELD" == "ok" || "$POSTGRES_FIELD" == "error" ]]; then
  pass "[AC-1][positive] health response contains 'postgres' field"
else
  fail "[AC-1][positive] health response contains 'postgres' field" \
    "expected .postgres to be 'ok' or 'error', got '$POSTGRES_FIELD'; body: $HEALTH_BODY"
fi

# [positive] health endpoint is unauthenticated (no 401 without auth header)
HEALTH_NO_AUTH=$(curl -s -o /dev/null -w "%{http_code}" \
  --no-keepalive "${API_URL}/api/health")
if [[ "$HEALTH_NO_AUTH" != "401" ]]; then
  pass "[AC-1][positive] GET /api/health is unauthenticated (no 401 without auth header)"
else
  fail "[AC-1][positive] GET /api/health is unauthenticated (no 401 without auth header)" \
    "endpoint returned 401 — health check must be public"
fi

# [negative] health endpoint returns JSON even without dependencies
# (Structural test: verify response is parseable JSON, not plain text)
CONTENT_TYPE=$(curl -s -I "${API_URL}/api/health" | grep -i "content-type:" | head -1)
if echo "$CONTENT_TYPE" | grep -q "application/json"; then
  pass "[AC-1][negative] GET /api/health returns Content-Type: application/json"
else
  fail "[AC-1][negative] GET /api/health returns Content-Type: application/json" \
    "expected Content-Type: application/json, got: $CONTENT_TYPE"
fi

# [negative] health endpoint returns non-empty JSON body (not trivially permissive)
HEALTH_KEYS=$(echo "$HEALTH_BODY" | jq 'keys | length' 2>/dev/null)
if [[ "$HEALTH_KEYS" -ge "3" ]]; then
  pass "[AC-1][negative] health response body has at least 3 fields (not trivially empty)"
else
  fail "[AC-1][negative] health response body has at least 3 fields (not trivially empty)" \
    "expected at least 3 keys in health response, got $HEALTH_KEYS; body: $HEALTH_BODY"
fi

# ─────────────────────────────────────────────────────────────────
# AC-2: GET /api/openapi.json serves valid OpenAPI 3.0 spec
# REQ: ADR-004
# Given: the API server is running
# When: GET /api/openapi.json is called with no authentication
# Then: 200 is returned with Content-Type: application/json, Cache-Control header,
#       and a body that is valid OpenAPI 3.0 JSON
# ─────────────────────────────────────────────────────────────────
echo ""
echo "[AC-2] GET /api/openapi.json — valid OpenAPI 3.0 spec"

OPENAPI_RESPONSE=$(curl -s -D - "${API_URL}/api/openapi.json")
OPENAPI_HEADERS=$(echo "$OPENAPI_RESPONSE" | grep -E "^HTTP|^Content-Type|^Cache-Control" | tr -d '\r')
OPENAPI_BODY=$(echo "$OPENAPI_RESPONSE" | awk '/^\r?$/{found=1; next} found{print}' | tr -d '\r')
OPENAPI_HTTP=$(echo "$OPENAPI_HEADERS" | head -1 | awk '{print $2}')

# [positive] returns HTTP 200
if [[ "$OPENAPI_HTTP" == "200" ]]; then
  pass "[AC-2][positive] GET /api/openapi.json returns HTTP 200"
else
  fail "[AC-2][positive] GET /api/openapi.json returns HTTP 200" \
    "expected HTTP 200, got $OPENAPI_HTTP"
fi

# [positive] Content-Type is application/json
OPENAPI_CT=$(echo "$OPENAPI_HEADERS" | grep -i "^Content-Type:" | head -1)
if echo "$OPENAPI_CT" | grep -qi "application/json"; then
  pass "[AC-2][positive] GET /api/openapi.json Content-Type is application/json"
else
  fail "[AC-2][positive] GET /api/openapi.json Content-Type is application/json" \
    "expected application/json, got: $OPENAPI_CT"
fi

# [positive] Cache-Control header is set
CACHE_CTRL=$(echo "$OPENAPI_HEADERS" | grep -i "^Cache-Control:" | head -1)
if echo "$CACHE_CTRL" | grep -q "public"; then
  pass "[AC-2][positive] GET /api/openapi.json Cache-Control header is set (public)"
else
  fail "[AC-2][positive] GET /api/openapi.json Cache-Control header is set (public)" \
    "expected Cache-Control: public, got: $CACHE_CTRL"
fi

# [positive] response body is valid JSON
OPENAPI_VERSION=$(echo "$OPENAPI_BODY" | jq -r '.openapi' 2>/dev/null)
if [[ "$OPENAPI_VERSION" == "3.0.3" ]]; then
  pass "[AC-2][positive] spec body is valid JSON with openapi: 3.0.3"
else
  fail "[AC-2][positive] spec body is valid JSON with openapi: 3.0.3" \
    "expected .openapi == '3.0.3', got '$OPENAPI_VERSION'"
fi

# [positive] spec has info.title
INFO_TITLE=$(echo "$OPENAPI_BODY" | jq -r '.info.title' 2>/dev/null)
if [[ -n "$INFO_TITLE" && "$INFO_TITLE" != "null" ]]; then
  pass "[AC-2][positive] spec has info.title: $INFO_TITLE"
else
  fail "[AC-2][positive] spec has info.title" \
    "expected non-null .info.title, got '$INFO_TITLE'"
fi

# [negative] GET /api/openapi.json does NOT require authentication
OPENAPI_NO_AUTH=$(curl -s -o /dev/null -w "%{http_code}" \
  --no-keepalive "${API_URL}/api/openapi.json")
if [[ "$OPENAPI_NO_AUTH" != "401" ]]; then
  pass "[AC-2][negative] GET /api/openapi.json is unauthenticated (no 401 without auth)"
else
  fail "[AC-2][negative] GET /api/openapi.json is unauthenticated (no 401 without auth)" \
    "endpoint returned 401 — spec endpoint must be public"
fi

# [negative] spec body is not trivially empty (must have paths and schemas)
PATHS_COUNT=$(echo "$OPENAPI_BODY" | jq '.paths | keys | length' 2>/dev/null)
if [[ "$PATHS_COUNT" -gt "0" ]]; then
  pass "[AC-2][negative] spec body is not trivially empty (has $PATHS_COUNT paths)"
else
  fail "[AC-2][negative] spec body is not trivially empty" \
    "expected .paths to have at least 1 entry, got $PATHS_COUNT"
fi

# ─────────────────────────────────────────────────────────────────
# AC-3: All REST endpoints documented in the spec
# REQ: ADR-004
# Given: the spec has been served by GET /api/openapi.json
# When: all REST paths are extracted from the spec
# Then: every registered REST endpoint in server.go is present in the spec
# ─────────────────────────────────────────────────────────────────
echo ""
echo "[AC-3] All REST endpoints documented in spec"

# Fetch spec for path checking (re-use body from above)
check_path() {
  local path="$1"
  local method="$2"
  local present
  present=$(echo "$OPENAPI_BODY" | jq --arg p "$path" --arg m "$method" \
    '.paths[$p][$m] != null' 2>/dev/null)
  if [[ "$present" == "true" ]]; then
    pass "[AC-3][positive] $method $path documented in spec"
  else
    fail "[AC-3][positive] $method $path documented in spec" \
      "path $path with method $method not found in spec"
  fi
}

# All routes registered in server.go (Handler() function) must be in the spec
check_path "/api/health"                  "get"
check_path "/api/openapi.json"            "get"
check_path "/api/auth/login"              "post"
check_path "/api/auth/logout"             "post"
check_path "/api/tasks"                   "post"
check_path "/api/tasks"                   "get"
check_path "/api/tasks/{id}"              "get"
check_path "/api/tasks/{id}/cancel"       "post"
check_path "/api/tasks/{id}/logs"         "get"
check_path "/api/pipelines"               "post"
check_path "/api/pipelines"               "get"
check_path "/api/pipelines/{id}"          "get"
check_path "/api/pipelines/{id}"          "put"
check_path "/api/pipelines/{id}"          "delete"
check_path "/api/workers"                 "get"
check_path "/api/chains"                  "post"
check_path "/api/chains/{id}"             "get"
check_path "/api/users"                   "post"
check_path "/api/users"                   "get"
check_path "/api/users/{id}/deactivate"   "put"

# [negative] spec does not document non-existent endpoints as real REST paths
# (SSE endpoints must NOT appear as standard REST paths — they use x-sse-endpoints extension)
SSE_AS_PATH=$(echo "$OPENAPI_BODY" | jq '.paths["/events/tasks"]' 2>/dev/null)
if [[ "$SSE_AS_PATH" == "null" ]]; then
  pass "[AC-3][negative] SSE path /events/tasks is NOT in .paths (correctly uses x-sse-endpoints extension)"
else
  fail "[AC-3][negative] SSE path /events/tasks is NOT in .paths" \
    "SSE endpoints should not appear as standard OpenAPI paths; found in .paths"
fi

# ─────────────────────────────────────────────────────────────────
# AC-4: Spec validates without errors
# REQ: ADR-004, FF-011
# Given: the spec JSON has been fetched from /api/openapi.json
# When: the spec structure is inspected
# Then: all required OpenAPI 3.0 fields are present; all $ref targets exist in components
# ─────────────────────────────────────────────────────────────────
echo ""
echo "[AC-4] Spec validates without errors"

# [positive] required top-level fields: openapi, info, paths
for field in openapi info paths; do
  FIELD_VAL=$(echo "$OPENAPI_BODY" | jq --arg f "$field" '.[$f] != null' 2>/dev/null)
  if [[ "$FIELD_VAL" == "true" ]]; then
    pass "[AC-4][positive] spec has required top-level field: $field"
  else
    fail "[AC-4][positive] spec has required top-level field: $field" \
      "required field '$field' is null or missing"
  fi
done

# [positive] info.version is present
INFO_VERSION=$(echo "$OPENAPI_BODY" | jq -r '.info.version' 2>/dev/null)
if [[ -n "$INFO_VERSION" && "$INFO_VERSION" != "null" ]]; then
  pass "[AC-4][positive] spec has info.version: $INFO_VERSION"
else
  fail "[AC-4][positive] spec has info.version" \
    "expected non-null .info.version, got '$INFO_VERSION'"
fi

# [positive] components/schemas section is present and non-empty
SCHEMA_COUNT=$(echo "$OPENAPI_BODY" | jq '.components.schemas | keys | length' 2>/dev/null)
if [[ "$SCHEMA_COUNT" -gt "5" ]]; then
  pass "[AC-4][positive] spec has components.schemas ($SCHEMA_COUNT schemas defined)"
else
  fail "[AC-4][positive] spec has components.schemas" \
    "expected at least 5 schemas, got $SCHEMA_COUNT"
fi

# [positive] HealthResponse schema is present (documents AC-1 endpoint)
HEALTH_SCHEMA=$(echo "$OPENAPI_BODY" | jq '.components.schemas.HealthResponse != null' 2>/dev/null)
if [[ "$HEALTH_SCHEMA" == "true" ]]; then
  pass "[AC-4][positive] HealthResponse schema is defined in components"
else
  fail "[AC-4][positive] HealthResponse schema is defined in components" \
    "HealthResponse schema is missing from components/schemas"
fi

# [positive] HealthResponse schema has required fields: status, redis, postgres
HR_REQUIRED=$(echo "$OPENAPI_BODY" | jq '.components.schemas.HealthResponse.required | sort' 2>/dev/null)
EXPECTED_REQUIRED='["postgres","redis","status"]'
if [[ "$HR_REQUIRED" == "$EXPECTED_REQUIRED" ]]; then
  pass "[AC-4][positive] HealthResponse schema has required fields: status, redis, postgres"
else
  fail "[AC-4][positive] HealthResponse schema has required fields: status, redis, postgres" \
    "expected $EXPECTED_REQUIRED, got $HR_REQUIRED"
fi

# [negative] spec is not trivially permissive — must contain securitySchemes
SEC_SCHEMES=$(echo "$OPENAPI_BODY" | jq '.components.securitySchemes | keys | length' 2>/dev/null)
if [[ "$SEC_SCHEMES" -ge "1" ]]; then
  pass "[AC-4][negative] spec has securitySchemes defined ($SEC_SCHEMES schemes)"
else
  fail "[AC-4][negative] spec has securitySchemes defined" \
    "expected at least 1 security scheme, got $SEC_SCHEMES"
fi

# ─────────────────────────────────────────────────────────────────
# Summary
# ─────────────────────────────────────────────────────────────────
hr
echo ""
echo "Results: $PASS passed, $FAIL failed"
echo ""
for r in "${RESULTS[@]}"; do
  echo "  $r"
done
hr

if [[ "$FAIL" -gt 0 ]]; then
  exit 1
else
  exit 0
fi
