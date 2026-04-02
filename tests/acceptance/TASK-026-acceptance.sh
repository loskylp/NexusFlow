#!/usr/bin/env bash
# Acceptance tests for TASK-026: Schema mapping validation at design time
# Requirements: REQ-007, ADR-008
#
# AC-1: POST /api/pipelines with invalid schema mapping returns 400 with error identifying the invalid field
# AC-2: Valid schema mappings pass validation (POST returns 201, PUT returns 200)
# AC-3: Both DataSource->Process and Process->Sink mappings are validated
# AC-4: Error messages identify the specific field and mapping that failed
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-026-acceptance.sh
#
# Requires: curl
# Services required: API server, PostgreSQL, Redis (all running via Docker Compose)

set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-nexusflow-postgres-1}"

PASS=0
FAIL=0
RESULTS=()

# Colour helpers (only when stdout is a terminal)
if [[ -t 1 ]]; then
  GREEN='\033[0;32m'; RED='\033[0;31m'; RESET='\033[0m'
else
  GREEN=''; RED=''; RESET=''
fi

pass() {
  local name="$1"
  printf "${GREEN}  PASS${RESET}: %s\n" "$name"
  PASS=$((PASS + 1))
  RESULTS+=("PASS | $name")
}

fail() {
  local name="$1"
  local detail="${2:-}"
  printf "${RED}  FAIL${RESET}: %s\n" "$name"
  [ -n "$detail" ] && echo "        $detail"
  FAIL=$((FAIL + 1))
  RESULTS+=("FAIL | $name | $detail")
}

assert_status() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  if [ "$actual" -eq "$expected" ]; then
    pass "$name"
  else
    fail "$name" "expected HTTP $expected, got HTTP $actual"
  fi
}

assert_body_contains() {
  local name="$1"
  local needle="$2"
  local body="$3"
  if echo "$body" | grep -q "$needle"; then
    pass "$name"
  else
    fail "$name" "expected body to contain '$needle'; body: $body"
  fi
}

assert_body_not_contains() {
  local name="$1"
  local needle="$2"
  local body="$3"
  if ! echo "$body" | grep -q "$needle"; then
    pass "$name"
  else
    fail "$name" "expected body NOT to contain '$needle'; body: $body"
  fi
}

db_query() { docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc "$@"; }

echo ""
echo "=== TASK-026 Acceptance Tests — Schema Mapping Validation at Design Time ==="
echo "    API: $API_URL"
echo "    REQ: REQ-007, ADR-008"
echo ""

# ---------------------------------------------------------------------------
# Prerequisites check
# ---------------------------------------------------------------------------
echo "--- Prerequisites ---"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/api/pipelines" -H "Authorization: Bearer dummy" 2>/dev/null || echo "000")
if [ "$STATUS" = "000" ]; then
  echo "  ERROR: API not reachable at $API_URL — aborting."
  exit 1
fi
echo "  API is reachable."
echo ""

# ---------------------------------------------------------------------------
# Setup: clean up prior test data
# ---------------------------------------------------------------------------
echo "--- Setup: clean prior test data ---"
docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc \
  "DELETE FROM pipelines WHERE name LIKE 'verifier-026-%';" > /dev/null 2>&1 || true
echo "  Prior test data cleared."
echo ""

# ---------------------------------------------------------------------------
# Setup: Admin login
# Given: admin user seeded on startup (TASK-003)
# When:  POST /api/auth/login with {"username":"admin","password":"admin"}
# Then:  200 OK with token
# ---------------------------------------------------------------------------
echo "--- Setup: admin login ---"
ADMIN_LOGIN_STATUS=$(curl -s -o /tmp/TASK026-admin-login.json -w "%{http_code}" \
  -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}')
ADMIN_LOGIN_BODY=$(cat /tmp/TASK026-admin-login.json 2>/dev/null || echo "")

if [ "$ADMIN_LOGIN_STATUS" != "200" ]; then
  echo "  FATAL: admin login failed (HTTP $ADMIN_LOGIN_STATUS) — cannot continue."
  echo "  Body: $ADMIN_LOGIN_BODY"
  exit 1
fi

ADMIN_TOKEN=$(echo "$ADMIN_LOGIN_BODY" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$ADMIN_TOKEN" ]; then
  echo "  FATAL: could not extract admin token from login response — cannot continue."
  exit 1
fi
echo "  Admin token acquired."
echo ""

# ---------------------------------------------------------------------------
# REQ-007 / AC-2: Valid schema mappings pass — POST returns 201
#
# Given: a pipeline definition with valid DS->Process and Process->Sink mappings
# When:  POST /api/pipelines is called with that definition
# Then:  HTTP 201 is returned and the pipeline is created
# ---------------------------------------------------------------------------
echo "--- AC-2: Valid schema mappings pass — POST /api/pipelines returns 201 ---"

VALID_POST_STATUS=$(curl -s -o /tmp/TASK026-valid-post.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "verifier-026-valid",
    "dataSourceConfig": {
      "connectorType": "demo",
      "config": {},
      "outputSchema": ["userId", "amount"]
    },
    "processConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": [
        {"sourceField": "userId", "targetField": "uid"},
        {"sourceField": "amount", "targetField": "value"}
      ],
      "outputSchema": ["uid", "value"]
    },
    "sinkConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": [
        {"sourceField": "uid", "targetField": "dest_uid"},
        {"sourceField": "value", "targetField": "dest_value"}
      ]
    }
  }')
VALID_POST_BODY=$(cat /tmp/TASK026-valid-post.json 2>/dev/null || echo "")

# REQ-007: AC-2 positive — valid mappings accepted
assert_status "AC-2 [REQ-007]: POST valid DS->Process+Process->Sink mappings returns 201" \
  201 "$VALID_POST_STATUS"

# Extract the pipeline ID for subsequent PUT tests
PIPELINE_ID=$(echo "$VALID_POST_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$PIPELINE_ID" ]; then
  echo "  WARNING: could not extract pipeline ID from POST response — PUT tests will be skipped."
fi
echo ""

# ---------------------------------------------------------------------------
# REQ-007 / AC-1, AC-3, AC-4: Invalid DS->Process mapping — POST returns 400
#
# Given: a pipeline definition where processConfig.inputMappings references a
#        field not in dataSourceConfig.outputSchema
# When:  POST /api/pipelines is called
# Then:  HTTP 400 is returned with an error message identifying the missing field
# ---------------------------------------------------------------------------
echo "--- AC-1/AC-3/AC-4: Invalid DS->Process mapping — POST returns 400 with field name ---"

DS_PROC_STATUS=$(curl -s -o /tmp/TASK026-ds-proc.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "verifier-026-bad-ds-proc",
    "dataSourceConfig": {
      "connectorType": "demo",
      "config": {},
      "outputSchema": ["userId", "amount"]
    },
    "processConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": [
        {"sourceField": "userId", "targetField": "uid"},
        {"sourceField": "nonexistent_field", "targetField": "x"}
      ],
      "outputSchema": ["uid"]
    },
    "sinkConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": []
    }
  }')
DS_PROC_BODY=$(cat /tmp/TASK026-ds-proc.json 2>/dev/null || echo "")

# AC-1: status is 400
assert_status "AC-1 [REQ-007]: POST invalid DS->Process mapping returns 400" \
  400 "$DS_PROC_STATUS"

# AC-3: the mapping context ("process input mapping") is named in the error
assert_body_contains \
  "AC-3 [REQ-007]: error identifies DS->Process transition ('process input mapping')" \
  "process input mapping" \
  "$DS_PROC_BODY"

# AC-4: the specific field name appears in the error
assert_body_contains \
  "AC-4 [REQ-007]: error message names the missing field ('nonexistent_field')" \
  "nonexistent_field" \
  "$DS_PROC_BODY"

# [VERIFIER-ADDED] Negative: the invalid pipeline must NOT have been persisted
if [ -n "$POSTGRES_CONTAINER" ]; then
  BAD_DS_COUNT=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc \
    "SELECT COUNT(*) FROM pipelines WHERE name='verifier-026-bad-ds-proc';" 2>/dev/null | tr -d '[:space:]' || echo "")
  if [ "$BAD_DS_COUNT" = "0" ]; then
    pass "[VERIFIER-ADDED] AC-1: invalid DS->Process pipeline is NOT persisted on 400"
  else
    fail "[VERIFIER-ADDED] AC-1: invalid DS->Process pipeline should not be persisted; found $BAD_DS_COUNT row(s)"
  fi
fi
echo ""

# ---------------------------------------------------------------------------
# REQ-007 / AC-3, AC-4: Invalid Process->Sink mapping — POST returns 400
#
# Given: a pipeline definition with valid DS->Process mappings but sinkConfig.inputMappings
#        references a field not in processConfig.outputSchema
# When:  POST /api/pipelines is called
# Then:  HTTP 400 is returned with an error message identifying the missing field
# ---------------------------------------------------------------------------
echo "--- AC-3/AC-4: Invalid Process->Sink mapping — POST returns 400 with field name ---"

PROC_SINK_STATUS=$(curl -s -o /tmp/TASK026-proc-sink.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "verifier-026-bad-proc-sink",
    "dataSourceConfig": {
      "connectorType": "demo",
      "config": {},
      "outputSchema": ["raw"]
    },
    "processConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": [
        {"sourceField": "raw", "targetField": "processed"}
      ],
      "outputSchema": ["processed"]
    },
    "sinkConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": [
        {"sourceField": "processed", "targetField": "dest"},
        {"sourceField": "ghost_field", "targetField": "x"}
      ]
    }
  }')
PROC_SINK_BODY=$(cat /tmp/TASK026-proc-sink.json 2>/dev/null || echo "")

# AC-3: validation catches the Process->Sink transition
assert_status "AC-3 [REQ-007]: POST invalid Process->Sink mapping returns 400" \
  400 "$PROC_SINK_STATUS"

# AC-3: the mapping context ("sink input mapping") is named in the error
assert_body_contains \
  "AC-3 [REQ-007]: error identifies Process->Sink transition ('sink input mapping')" \
  "sink input mapping" \
  "$PROC_SINK_BODY"

# AC-4: the specific field name appears in the error
assert_body_contains \
  "AC-4 [REQ-007]: error message names the missing field ('ghost_field')" \
  "ghost_field" \
  "$PROC_SINK_BODY"

# [VERIFIER-ADDED] Negative: invalid Process->Sink pipeline must NOT have been persisted
if [ -n "$POSTGRES_CONTAINER" ]; then
  BAD_SINK_COUNT=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc \
    "SELECT COUNT(*) FROM pipelines WHERE name='verifier-026-bad-proc-sink';" 2>/dev/null | tr -d '[:space:]' || echo "")
  if [ "$BAD_SINK_COUNT" = "0" ]; then
    pass "[VERIFIER-ADDED] AC-3: invalid Process->Sink pipeline is NOT persisted on 400"
  else
    fail "[VERIFIER-ADDED] AC-3: invalid Process->Sink pipeline should not be persisted; found $BAD_SINK_COUNT row(s)"
  fi
fi
echo ""

# ---------------------------------------------------------------------------
# REQ-007 / AC-2: Valid schema mappings pass — PUT returns 200
#
# Given: an existing pipeline and an update request with valid mappings
# When:  PUT /api/pipelines/{id} is called
# Then:  HTTP 200 is returned
# ---------------------------------------------------------------------------
echo "--- AC-2: Valid schema mappings pass — PUT /api/pipelines/{id} returns 200 ---"

if [ -n "$PIPELINE_ID" ]; then
  VALID_PUT_STATUS=$(curl -s -o /tmp/TASK026-valid-put.json -w "%{http_code}" \
    -X PUT "$API_URL/api/pipelines/$PIPELINE_ID" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -d '{
      "name": "verifier-026-valid-updated",
      "dataSourceConfig": {
        "connectorType": "demo",
        "config": {},
        "outputSchema": ["x", "y"]
      },
      "processConfig": {
        "connectorType": "demo",
        "config": {},
        "inputMappings": [
          {"sourceField": "x", "targetField": "alpha"},
          {"sourceField": "y", "targetField": "beta"}
        ],
        "outputSchema": ["alpha", "beta"]
      },
      "sinkConfig": {
        "connectorType": "demo",
        "config": {},
        "inputMappings": [
          {"sourceField": "alpha", "targetField": "dest_alpha"},
          {"sourceField": "beta", "targetField": "dest_beta"}
        ]
      }
    }')

  assert_status "AC-2 [REQ-007]: PUT valid mappings returns 200" 200 "$VALID_PUT_STATUS"
else
  fail "AC-2 [REQ-007]: PUT valid mappings returns 200" "SKIPPED — no pipeline ID from POST (check earlier failures)"
fi
echo ""

# ---------------------------------------------------------------------------
# REQ-007 / AC-1: Invalid mapping on PUT returns 400
#
# Given: an existing pipeline and an update request where processConfig.inputMappings
#        references a field absent from dataSourceConfig.outputSchema
# When:  PUT /api/pipelines/{id} is called
# Then:  HTTP 400 is returned with an error identifying the invalid field
# ---------------------------------------------------------------------------
echo "--- AC-1: Invalid mapping on PUT /api/pipelines/{id} returns 400 ---"

if [ -n "$PIPELINE_ID" ]; then
  INVALID_PUT_STATUS=$(curl -s -o /tmp/TASK026-invalid-put.json -w "%{http_code}" \
    -X PUT "$API_URL/api/pipelines/$PIPELINE_ID" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -d '{
      "name": "verifier-026-invalid-update",
      "dataSourceConfig": {
        "connectorType": "demo",
        "config": {},
        "outputSchema": ["userId"]
      },
      "processConfig": {
        "connectorType": "demo",
        "config": {},
        "inputMappings": [
          {"sourceField": "missing_on_update", "targetField": "x"}
        ],
        "outputSchema": ["x"]
      },
      "sinkConfig": {
        "connectorType": "demo",
        "config": {},
        "inputMappings": []
      }
    }')
  INVALID_PUT_BODY=$(cat /tmp/TASK026-invalid-put.json 2>/dev/null || echo "")

  assert_status "AC-1 [REQ-007]: PUT invalid mapping returns 400" 400 "$INVALID_PUT_STATUS"
  assert_body_contains \
    "AC-4 [REQ-007]: PUT error message names the missing field ('missing_on_update')" \
    "missing_on_update" \
    "$INVALID_PUT_BODY"

  # [VERIFIER-ADDED] Negative: verify the pipeline name was NOT updated (no partial mutation on failure)
  STORED_NAME=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc \
    "SELECT name FROM pipelines WHERE id='$PIPELINE_ID';" 2>/dev/null | tr -d '[:space:]' || echo "")
  if [ "$STORED_NAME" = "verifier-026-valid-updated" ]; then
    pass "[VERIFIER-ADDED] AC-1: pipeline not mutated when PUT mapping validation fails"
  else
    fail "[VERIFIER-ADDED] AC-1: pipeline name after failed PUT is '$STORED_NAME', expected 'verifier-026-valid-updated'"
  fi
else
  fail "AC-1 [REQ-007]: PUT invalid mapping returns 400" "SKIPPED — no pipeline ID from POST (check earlier failures)"
fi
echo ""

# ---------------------------------------------------------------------------
# [VERIFIER-ADDED] Negative: empty mappings always pass (no false positives)
#
# Given: a pipeline definition with no inputMappings in either processConfig or sinkConfig
# When:  POST /api/pipelines is called
# Then:  HTTP 201 is returned (empty mappings are not an error)
# ---------------------------------------------------------------------------
echo "--- [VERIFIER-ADDED] Negative: empty mappings are not rejected ---"

EMPTY_MAP_STATUS=$(curl -s -o /tmp/TASK026-empty-map.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "verifier-026-empty-mappings",
    "dataSourceConfig": {
      "connectorType": "demo",
      "config": {},
      "outputSchema": ["field1"]
    },
    "processConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": [],
      "outputSchema": ["out1"]
    },
    "sinkConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": []
    }
  }')

assert_status "[VERIFIER-ADDED] AC-2: empty mappings pass validation (POST returns 201)" \
  201 "$EMPTY_MAP_STATUS"
echo ""

# ---------------------------------------------------------------------------
# Teardown: clean up test pipelines
# ---------------------------------------------------------------------------
echo "--- Teardown ---"
docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc \
  "DELETE FROM pipelines WHERE name LIKE 'verifier-026-%';" > /dev/null 2>&1 || true
echo "  Test pipelines removed."
echo ""

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
TOTAL=$((PASS + FAIL))
echo "=== Results: $PASS/$TOTAL passed ==="
for r in "${RESULTS[@]}"; do
  echo "  $r"
done
echo ""

if [ "$FAIL" -eq 0 ]; then
  echo "TASK-026: PASS"
  exit 0
else
  echo "TASK-026: FAIL ($FAIL failure(s))"
  exit 1
fi
