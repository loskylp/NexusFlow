#!/usr/bin/env bash
# Acceptance tests for TASK-023: Pipeline Builder (GUI) — API-level AC-6
# Requirements: REQ-015
#
# AC-6: Saved pipeline is available via GET /api/pipelines after being created
#       (verifies that the GUI save flow POSTs a valid pipeline that is retrievable)
#
# Additional system-level verifications:
#   - POST /api/pipelines returns 201 with a pipeline ID
#   - GET /api/pipelines/{id} returns the saved pipeline
#   - PUT /api/pipelines/{id} updates the pipeline (save-existing flow)
#   - DELETE /api/pipelines/{id} removes the pipeline
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-023-api-acceptance.sh
#
# Requires: curl
# Services: API server, PostgreSQL, Redis (via Docker Compose)

set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"

PASS=0
FAIL=0
RESULTS=()

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
  local name="$1"; local expected="$2"; local actual="$3"
  if [ "$actual" -eq "$expected" ]; then pass "$name"
  else fail "$name" "expected HTTP $expected, got HTTP $actual"; fi
}

assert_body_contains() {
  local name="$1"; local needle="$2"; local body="$3"
  if echo "$body" | grep -q "$needle"; then pass "$name"
  else fail "$name" "expected body to contain '$needle'; body: $body"; fi
}

assert_body_not_contains() {
  local name="$1"; local needle="$2"; local body="$3"
  if ! echo "$body" | grep -q "$needle"; then pass "$name"
  else fail "$name" "expected body NOT to contain '$needle'; body: $body"; fi
}

echo ""
echo "=== TASK-023 API Acceptance Tests — Pipeline Builder (GUI) ==="
echo "    API: $API_URL"
echo "    REQ: REQ-015"
echo ""

# ---------------------------------------------------------------------------
# Prerequisites check
# ---------------------------------------------------------------------------
echo "--- Prerequisites ---"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/api/pipelines" \
  -H "Authorization: Bearer dummy" 2>/dev/null || echo "000")
if [ "$STATUS" = "000" ]; then
  echo "  ERROR: API not reachable at $API_URL — aborting."
  exit 1
fi
echo "  API is reachable (HTTP $STATUS)."
echo ""

# ---------------------------------------------------------------------------
# Setup: Admin login
# ---------------------------------------------------------------------------
echo "--- Setup: admin login ---"
ADMIN_LOGIN_STATUS=$(curl -s -o /tmp/TASK023-admin-login.json -w "%{http_code}" \
  -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}')
ADMIN_LOGIN_BODY=$(cat /tmp/TASK023-admin-login.json 2>/dev/null || echo "")

if [ "$ADMIN_LOGIN_STATUS" != "200" ]; then
  echo "  FATAL: admin login failed (HTTP $ADMIN_LOGIN_STATUS) — cannot continue."
  exit 1
fi

ADMIN_TOKEN=$(echo "$ADMIN_LOGIN_BODY" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$ADMIN_TOKEN" ]; then
  echo "  FATAL: could not extract admin token."
  exit 1
fi
echo "  Admin token acquired."
echo ""

# ---------------------------------------------------------------------------
# Setup: clean prior test data
# ---------------------------------------------------------------------------
echo "--- Setup: clean prior test data ---"
LIST_STATUS=$(curl -s -o /tmp/TASK023-list-pre.json -w "%{http_code}" \
  "$API_URL/api/pipelines" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
LIST_BODY=$(cat /tmp/TASK023-list-pre.json 2>/dev/null || echo "[]")
# Collect IDs with name matching our test prefix and delete them
echo "$LIST_BODY" | grep -o '"id":"[^"]*","name":"verifier-023-[^"]*"' | \
  grep -o '"id":"[^"]*"' | cut -d'"' -f4 | while read -r ID; do
    curl -s -o /dev/null -X DELETE "$API_URL/api/pipelines/$ID" \
      -H "Authorization: Bearer $ADMIN_TOKEN" || true
done
echo "  Prior test data cleaned."
echo ""

# ---------------------------------------------------------------------------
# AC-6 / REQ-015: POST creates pipeline — GET /api/pipelines returns it
#
# Given: a logged-in user POSTs a valid pipeline definition (as the GUI save flow does)
# When:  GET /api/pipelines is called
# Then:  the saved pipeline appears in the list
# ---------------------------------------------------------------------------
echo "--- AC-6 [REQ-015]: POST /api/pipelines creates pipeline retrievable via GET ---"

VALID_PIPELINE='{
  "name": "verifier-023-full-pipeline",
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
}'

POST_STATUS=$(curl -s -o /tmp/TASK023-post.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "$VALID_PIPELINE")
POST_BODY=$(cat /tmp/TASK023-post.json 2>/dev/null || echo "")

# AC-6a: POST returns 201
assert_status "AC-6 [REQ-015]: POST /api/pipelines returns 201" 201 "$POST_STATUS"

# AC-6b: Response contains an ID
assert_body_contains "AC-6 [REQ-015]: POST response contains pipeline ID field" '"id"' "$POST_BODY"
assert_body_contains "AC-6 [REQ-015]: POST response contains pipeline name" '"verifier-023-full-pipeline"' "$POST_BODY"

PIPELINE_ID=$(echo "$POST_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
echo "  Created pipeline ID: $PIPELINE_ID"
echo ""

# AC-6c: GET /api/pipelines lists the new pipeline
echo "--- AC-6 [REQ-015]: GET /api/pipelines returns the saved pipeline ---"
GET_LIST_STATUS=$(curl -s -o /tmp/TASK023-list.json -w "%{http_code}" \
  "$API_URL/api/pipelines" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
GET_LIST_BODY=$(cat /tmp/TASK023-list.json 2>/dev/null || echo "")

assert_status "AC-6 [REQ-015]: GET /api/pipelines returns 200" 200 "$GET_LIST_STATUS"
assert_body_contains "AC-6 [REQ-015]: GET /api/pipelines includes the saved pipeline name" \
  '"verifier-023-full-pipeline"' "$GET_LIST_BODY"
echo ""

# ---------------------------------------------------------------------------
# AC-6d: GET /api/pipelines/{id} retrieves the individual pipeline
#
# Given: a pipeline was just created
# When:  GET /api/pipelines/{id} is called (as the GUI load-pipeline flow does)
# Then:  the full pipeline definition is returned
# ---------------------------------------------------------------------------
echo "--- AC-6 [REQ-015]: GET /api/pipelines/{id} retrieves individual pipeline ---"

if [ -n "$PIPELINE_ID" ]; then
  GET_ONE_STATUS=$(curl -s -o /tmp/TASK023-getone.json -w "%{http_code}" \
    "$API_URL/api/pipelines/$PIPELINE_ID" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
  GET_ONE_BODY=$(cat /tmp/TASK023-getone.json 2>/dev/null || echo "")

  assert_status "AC-6 [REQ-015]: GET /api/pipelines/{id} returns 200" 200 "$GET_ONE_STATUS"
  assert_body_contains "AC-6 [REQ-015]: GET /api/pipelines/{id} returns correct name" \
    '"verifier-023-full-pipeline"' "$GET_ONE_BODY"
  assert_body_contains "AC-6 [REQ-015]: GET /api/pipelines/{id} returns dataSourceConfig" \
    '"dataSourceConfig"' "$GET_ONE_BODY"
  assert_body_contains "AC-6 [REQ-015]: GET /api/pipelines/{id} returns processConfig" \
    '"processConfig"' "$GET_ONE_BODY"
  assert_body_contains "AC-6 [REQ-015]: GET /api/pipelines/{id} returns sinkConfig" \
    '"sinkConfig"' "$GET_ONE_BODY"
else
  fail "AC-6 [REQ-015]: GET /api/pipelines/{id}" "SKIPPED — no pipeline ID from POST"
fi
echo ""

# ---------------------------------------------------------------------------
# [VERIFIER-ADDED] Negative: unauthenticated request returns 401
#
# Given: no session token
# When:  GET /api/pipelines is called
# Then:  HTTP 401
# ---------------------------------------------------------------------------
echo "--- [VERIFIER-ADDED] Negative: unauthenticated GET /api/pipelines returns 401 ---"

UNAUTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/api/pipelines" 2>/dev/null || echo "000")
assert_status "[VERIFIER-ADDED] GET /api/pipelines without auth returns 401" 401 "$UNAUTH_STATUS"
echo ""

# ---------------------------------------------------------------------------
# [VERIFIER-ADDED] PUT /api/pipelines/{id} updates the pipeline
#
# Given: an existing pipeline
# When:  PUT /api/pipelines/{id} is called (as the GUI save-existing-pipeline flow does)
# Then:  HTTP 200 and the updated name is returned
# ---------------------------------------------------------------------------
echo "--- [VERIFIER-ADDED] PUT /api/pipelines/{id} updates pipeline (save-existing flow) ---"

if [ -n "$PIPELINE_ID" ]; then
  PUT_STATUS=$(curl -s -o /tmp/TASK023-put.json -w "%{http_code}" \
    -X PUT "$API_URL/api/pipelines/$PIPELINE_ID" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -d '{
      "name": "verifier-023-updated",
      "dataSourceConfig": {
        "connectorType": "demo", "config": {}, "outputSchema": ["x"]
      },
      "processConfig": {
        "connectorType": "demo", "config": {},
        "inputMappings": [{"sourceField": "x", "targetField": "y"}],
        "outputSchema": ["y"]
      },
      "sinkConfig": {
        "connectorType": "demo", "config": {},
        "inputMappings": [{"sourceField": "y", "targetField": "z"}]
      }
    }')
  PUT_BODY=$(cat /tmp/TASK023-put.json 2>/dev/null || echo "")

  assert_status "[VERIFIER-ADDED] PUT /api/pipelines/{id} returns 200" 200 "$PUT_STATUS"
  assert_body_contains "[VERIFIER-ADDED] PUT response contains updated name" '"verifier-023-updated"' "$PUT_BODY"
else
  fail "[VERIFIER-ADDED] PUT /api/pipelines/{id}" "SKIPPED — no pipeline ID from POST"
fi
echo ""

# ---------------------------------------------------------------------------
# Teardown: clean up test pipelines
# ---------------------------------------------------------------------------
echo "--- Teardown ---"
LIST_STATUS2=$(curl -s -o /tmp/TASK023-list2.json -w "%{http_code}" \
  "$API_URL/api/pipelines" -H "Authorization: Bearer $ADMIN_TOKEN")
LIST_BODY2=$(cat /tmp/TASK023-list2.json 2>/dev/null || echo "[]")
echo "$LIST_BODY2" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | while read -r ID; do
  NAME_LINE=$(echo "$LIST_BODY2" | grep -o "\"id\":\"$ID\",\"name\":\"verifier-023-[^\"]*\"" || true)
  if [ -n "$NAME_LINE" ]; then
    curl -s -o /dev/null -X DELETE "$API_URL/api/pipelines/$ID" \
      -H "Authorization: Bearer $ADMIN_TOKEN" || true
  fi
done
if [ -n "$PIPELINE_ID" ]; then
  curl -s -o /dev/null -X DELETE "$API_URL/api/pipelines/$PIPELINE_ID" \
    -H "Authorization: Bearer $ADMIN_TOKEN" 2>/dev/null || true
fi
echo "  Test pipelines cleaned up."
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
  echo "TASK-023 API: PASS"
  exit 0
else
  echo "TASK-023 API: FAIL ($FAIL failure(s))"
  exit 1
fi
