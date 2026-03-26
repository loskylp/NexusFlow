#!/usr/bin/env bash
# Acceptance tests for TASK-013: Pipeline CRUD via REST API
# Requirement: REQ-022
#
# AC-1: POST /api/pipelines creates a pipeline with DataSource, Process, Sink config; returns 201
# AC-2: GET /api/pipelines returns user's own pipelines (User) or all pipelines (Admin)
# AC-3: GET /api/pipelines/{id} returns pipeline details for the owning user
# AC-4: PUT /api/pipelines/{id} updates pipeline config; returns 200
# AC-5: DELETE /api/pipelines/{id} deletes a pipeline if no active tasks; returns 204
# AC-6: DELETE /api/pipelines/{id} returns 409 if active (non-terminal) tasks reference the pipeline
# AC-7: Non-owner non-admin user gets 403 on GET, PUT, DELETE for a pipeline owned by another user
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-013-acceptance.sh
#
# Requires: curl, docker exec (psql access)
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

db_query() { docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc "$@"; }

echo ""
echo "=== TASK-013 Acceptance Tests — Pipeline CRUD via REST API ==="
echo "    API: $API_URL"
echo "    REQ: REQ-022"
echo ""

# ---------------------------------------------------------------------------
# Prerequisites check
# ---------------------------------------------------------------------------
echo "--- Prerequisites ---"
if ! curl -sf "$API_URL/api/health" > /dev/null 2>&1; then
  echo "  ERROR: API not reachable at $API_URL/api/health — aborting."
  exit 1
fi
echo "  API is reachable."

if ! docker exec "$POSTGRES_CONTAINER" pg_isready -U nexusflow > /dev/null 2>&1; then
  echo "  ERROR: PostgreSQL container '$POSTGRES_CONTAINER' not ready — aborting."
  exit 1
fi
echo "  PostgreSQL is ready."
echo ""

# ---------------------------------------------------------------------------
# Setup: clean up any test data from prior runs
# ---------------------------------------------------------------------------
echo "--- Setup: clean prior test data ---"
db_query "DELETE FROM tasks      WHERE execution_id LIKE 'verifier-task-013-%';" > /dev/null 2>&1 || true
db_query "DELETE FROM pipelines  WHERE name LIKE 'verifier-task-013-%';" > /dev/null 2>&1 || true
db_query "DELETE FROM users      WHERE username = 'verifier-user-013';" > /dev/null 2>&1 || true
echo "  Prior test data cleared."
echo ""

# ---------------------------------------------------------------------------
# Setup: Admin login
# Given: admin user seeded on startup (TASK-003)
# When:  POST /api/auth/login with {"username":"admin","password":"admin"}
# Then:  200 OK with token
# ---------------------------------------------------------------------------
echo "--- Setup: admin login ---"
ADMIN_LOGIN_STATUS=$(curl -s -o /tmp/TASK013-admin-login.json -w "%{http_code}" \
  -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}')
ADMIN_LOGIN_BODY=$(cat /tmp/TASK013-admin-login.json 2>/dev/null || echo "")

if [ "$ADMIN_LOGIN_STATUS" != "200" ]; then
  echo "  FATAL: admin login failed (HTTP $ADMIN_LOGIN_STATUS) — cannot continue."
  echo "  Body: $ADMIN_LOGIN_BODY"
  exit 1
fi

ADMIN_TOKEN=$(echo "$ADMIN_LOGIN_BODY" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
# Login response shape: {"token":"...","user":{"id":"...","username":"admin","role":"admin"}}
# Extract admin user id from the database to avoid fragile JSON parsing.
ADMIN_USER_ID=$(db_query "SELECT id FROM users WHERE username='admin';" 2>/dev/null | tr -d '[:space:]')

if [ -z "$ADMIN_TOKEN" ]; then
  echo "  FATAL: admin token missing from login response — cannot continue."
  echo "  Body: $ADMIN_LOGIN_BODY"
  exit 1
fi
echo "  Admin login OK. UserID=$ADMIN_USER_ID"
echo ""

# ---------------------------------------------------------------------------
# Setup: Create a second (non-admin) user for AC-7 via direct DB insert
# The API has no public registration endpoint in this cycle.
# ---------------------------------------------------------------------------
echo "--- Setup: create non-admin user for AC-7 ---"
VERIFIER_USER_ID=$(db_query "
  INSERT INTO users (username, password_hash, role, active)
  VALUES ('verifier-user-013', '\$2a\$10\$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'user', true)
  ON CONFLICT (username) DO UPDATE SET active = true
  RETURNING id;" 2>/dev/null | grep -E '^[0-9a-f-]{36}$' | head -1)

# The password_hash above is bcrypt of "password" (cost 10).
# Inject a session for this user directly into Redis.
VERIFIER_TOKEN="verifier-task-013-user-token"
VERIFIER_SESSION_JSON="{\"userId\":\"${VERIFIER_USER_ID}\",\"role\":\"user\",\"createdAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
docker exec nexusflow-redis-1 redis-cli SET "session:${VERIFIER_TOKEN}" "$VERIFIER_SESSION_JSON" EX 3600 > /dev/null 2>&1

if [ -n "$VERIFIER_USER_ID" ]; then
  echo "  Non-admin user created. UserID=$VERIFIER_USER_ID"
else
  echo "  FATAL: could not create non-admin user — cannot continue."
  exit 1
fi
echo ""

# Minimal valid pipeline body used across multiple tests.
PIPELINE_BODY() {
  local name="$1"
  cat <<EOF
{
  "name": "$name",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {"rows": 10},
    "outputSchema": ["id", "name", "value"]
  },
  "processConfig": {
    "connectorType": "passthrough",
    "config": {},
    "inputMappings": [
      {"sourceField": "id",    "targetField": "record_id"},
      {"sourceField": "name",  "targetField": "label"},
      {"sourceField": "value", "targetField": "amount"}
    ],
    "outputSchema": ["record_id", "label", "amount"]
  },
  "sinkConfig": {
    "connectorType": "demo-sink",
    "config": {"target": "stdout"},
    "inputMappings": [
      {"sourceField": "record_id", "targetField": "id"},
      {"sourceField": "label",     "targetField": "name"},
      {"sourceField": "amount",    "targetField": "value"}
    ]
  }
}
EOF
}

# ============================================================
# AC-1 (REQ-022): POST /api/pipelines creates a pipeline with
#   DataSource, Process, Sink config; returns 201.
#
# Given:  a logged-in admin user.
# When:   POST /api/pipelines with a valid JSON body containing
#         name, dataSourceConfig, processConfig, and sinkConfig.
# Then:   response is 201 Created; body contains id, name, userId
#         (matching admin), dataSourceConfig, processConfig,
#         sinkConfig, createdAt, updatedAt; row exists in DB.
#
# Negative: POST with missing name returns 400 (not 201).
# ============================================================
echo "--- AC-1 (REQ-022): POST /api/pipelines creates pipeline, returns 201 ---"

CREATE_STATUS=$(curl -s -o /tmp/TASK013-create.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "$(PIPELINE_BODY "verifier-task-013-pipeline-a")")
CREATE_BODY=$(cat /tmp/TASK013-create.json 2>/dev/null || echo "")

assert_status "AC-1: POST /api/pipelines returns 201" 201 "$CREATE_STATUS"

PIPELINE_ID=$(echo "$CREATE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
PIPELINE_NAME=$(echo "$CREATE_BODY" | grep -o '"name":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
PIPELINE_USER_ID=$(echo "$CREATE_BODY" | grep -o '"userId":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")

if [ -n "$PIPELINE_ID" ]; then
  pass "AC-1: response body contains id field"
else
  fail "AC-1: response body missing id field" "body: $CREATE_BODY"
fi

if [ "$PIPELINE_NAME" = "verifier-task-013-pipeline-a" ]; then
  pass "AC-1: response body name matches submitted name"
else
  fail "AC-1: name mismatch" "expected 'verifier-task-013-pipeline-a', got '$PIPELINE_NAME'"
fi

if [ "$PIPELINE_USER_ID" = "$ADMIN_USER_ID" ]; then
  pass "AC-1: userId in response matches authenticated user (not caller-supplied)"
else
  fail "AC-1: userId mismatch — handler must set userId from session, not request body" \
    "expected '$ADMIN_USER_ID', got '$PIPELINE_USER_ID'"
fi

# Verify all three phase config fields are present in the response.
if echo "$CREATE_BODY" | grep -q '"dataSourceConfig"' && \
   echo "$CREATE_BODY" | grep -q '"processConfig"' && \
   echo "$CREATE_BODY" | grep -q '"sinkConfig"'; then
  pass "AC-1: response contains dataSourceConfig, processConfig, sinkConfig"
else
  fail "AC-1: response missing one or more phase config fields" "body: $CREATE_BODY"
fi

# Verify timestamps are present.
if echo "$CREATE_BODY" | grep -q '"createdAt"' && echo "$CREATE_BODY" | grep -q '"updatedAt"'; then
  pass "AC-1: response contains createdAt and updatedAt"
else
  fail "AC-1: response missing createdAt or updatedAt" "body: $CREATE_BODY"
fi

# Verify row exists in the database.
if [ -n "$PIPELINE_ID" ]; then
  DB_ROW=$(db_query "SELECT id FROM pipelines WHERE id = '${PIPELINE_ID}';" 2>/dev/null | tr -d '[:space:]')
  if [ "$DB_ROW" = "$PIPELINE_ID" ]; then
    pass "AC-1: pipeline row persisted in PostgreSQL"
  else
    fail "AC-1: pipeline row not found in PostgreSQL after 201" "id=$PIPELINE_ID"
  fi
fi

# AC-1 negative: POST with missing name returns 400.
CREATE_NO_NAME_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"dataSourceConfig":{},"processConfig":{},"sinkConfig":{}}')
assert_status "AC-1 negative: POST without name returns 400" 400 "$CREATE_NO_NAME_STATUS"

# AC-1 negative: POST unauthenticated returns 401.
CREATE_UNAUTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -d "$(PIPELINE_BODY "verifier-task-013-unauth-test")")
assert_status "AC-1 negative: POST without auth returns 401" 401 "$CREATE_UNAUTH_STATUS"

echo ""

# ============================================================
# AC-2 (REQ-022): GET /api/pipelines returns user's own pipelines
#   (User role) or all pipelines (Admin role).
#
# Given:  admin has created pipeline A (above).
#         A separate pipeline B owned by a non-admin user.
# When:   admin calls GET /api/pipelines.
# Then:   both pipeline A and pipeline B appear in the response.
# When:   non-admin user calls GET /api/pipelines.
# Then:   only pipeline B (owned by that user) appears.
#
# Negative: A pipeline owned by a different user must NOT appear
#   in the non-admin user's list.
# ============================================================
echo "--- AC-2 (REQ-022): GET /api/pipelines — user sees own; admin sees all ---"

# Create a pipeline owned by the non-admin user.
CREATE_USER_STATUS=$(curl -s -o /tmp/TASK013-create-user.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $VERIFIER_TOKEN" \
  -d "$(PIPELINE_BODY "verifier-task-013-pipeline-b")")
CREATE_USER_BODY=$(cat /tmp/TASK013-create-user.json 2>/dev/null || echo "")
PIPELINE_B_ID=$(echo "$CREATE_USER_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")

if [ "$CREATE_USER_STATUS" = "201" ] && [ -n "$PIPELINE_B_ID" ]; then
  echo "  Setup: non-admin user's pipeline B created (id=$PIPELINE_B_ID)"
else
  fail "AC-2 setup: non-admin user could not create pipeline B (HTTP $CREATE_USER_STATUS)" \
    "body: $CREATE_USER_BODY"
fi

# Admin sees all pipelines (both A and B).
ADMIN_LIST_STATUS=$(curl -s -o /tmp/TASK013-list-admin.json -w "%{http_code}" \
  -X GET "$API_URL/api/pipelines" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
ADMIN_LIST_BODY=$(cat /tmp/TASK013-list-admin.json 2>/dev/null || echo "")

assert_status "AC-2: GET /api/pipelines returns 200 for admin" 200 "$ADMIN_LIST_STATUS"

if echo "$ADMIN_LIST_BODY" | grep -q "$PIPELINE_ID" && \
   echo "$ADMIN_LIST_BODY" | grep -q "$PIPELINE_B_ID"; then
  pass "AC-2: admin list contains pipelines from both users"
else
  fail "AC-2: admin list does not contain pipelines from all users" \
    "A=$PIPELINE_ID B=$PIPELINE_B_ID body=$ADMIN_LIST_BODY"
fi

# Non-admin user sees only their own pipeline (B), not admin's (A).
USER_LIST_STATUS=$(curl -s -o /tmp/TASK013-list-user.json -w "%{http_code}" \
  -X GET "$API_URL/api/pipelines" \
  -H "Authorization: Bearer $VERIFIER_TOKEN")
USER_LIST_BODY=$(cat /tmp/TASK013-list-user.json 2>/dev/null || echo "")

assert_status "AC-2: GET /api/pipelines returns 200 for non-admin user" 200 "$USER_LIST_STATUS"

if echo "$USER_LIST_BODY" | grep -q "$PIPELINE_B_ID"; then
  pass "AC-2: non-admin user sees their own pipeline B"
else
  fail "AC-2: non-admin user's list does not contain their pipeline B" \
    "B=$PIPELINE_B_ID body=$USER_LIST_BODY"
fi

if echo "$USER_LIST_BODY" | grep -q "$PIPELINE_ID"; then
  fail "AC-2 negative: non-admin user can see admin's pipeline A — ownership filter is broken" \
    "A=$PIPELINE_ID body=$USER_LIST_BODY"
else
  pass "AC-2 negative: non-admin user does NOT see admin's pipeline A (correct filtering)"
fi

# AC-2 negative: unauthenticated request returns 401.
LIST_UNAUTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/pipelines")
assert_status "AC-2 negative: GET /api/pipelines without auth returns 401" 401 "$LIST_UNAUTH_STATUS"

echo ""

# ============================================================
# AC-3 (REQ-022): GET /api/pipelines/{id} returns pipeline details.
#
# Given:  admin owns pipeline A (id=$PIPELINE_ID).
# When:   admin calls GET /api/pipelines/{id}.
# Then:   response is 200 with correct pipeline JSON.
#
# Negative: GET on a non-existent UUID returns 404.
# Negative: GET with a non-UUID id returns 400.
# ============================================================
echo "--- AC-3 (REQ-022): GET /api/pipelines/{id} returns pipeline details ---"

GET_STATUS=$(curl -s -o /tmp/TASK013-get.json -w "%{http_code}" \
  -X GET "$API_URL/api/pipelines/$PIPELINE_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
GET_BODY=$(cat /tmp/TASK013-get.json 2>/dev/null || echo "")

assert_status "AC-3: GET /api/pipelines/{id} returns 200" 200 "$GET_STATUS"

GET_ID=$(echo "$GET_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
if [ "$GET_ID" = "$PIPELINE_ID" ]; then
  pass "AC-3: response body id matches requested pipeline id"
else
  fail "AC-3: id mismatch in GET response" "expected $PIPELINE_ID, got $GET_ID"
fi

if echo "$GET_BODY" | grep -q '"dataSourceConfig"' && \
   echo "$GET_BODY" | grep -q '"processConfig"' && \
   echo "$GET_BODY" | grep -q '"sinkConfig"'; then
  pass "AC-3: GET response contains all three phase configs"
else
  fail "AC-3: GET response missing one or more phase config fields" "body: $GET_BODY"
fi

# AC-3 negative: non-existent pipeline returns 404.
NONEXIST_UUID="00000000-0000-0000-0000-000000000099"
GET_404_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/pipelines/$NONEXIST_UUID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-3 negative: GET non-existent pipeline returns 404" 404 "$GET_404_STATUS"

# AC-3 negative: invalid UUID format returns 400.
GET_400_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/pipelines/not-a-uuid" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-3 negative: GET with invalid UUID returns 400" 400 "$GET_400_STATUS"

echo ""

# ============================================================
# AC-4 (REQ-022): PUT /api/pipelines/{id} updates pipeline; returns 200.
#
# Given:  admin owns pipeline A.
# When:   admin calls PUT /api/pipelines/{id} with updated name.
# Then:   response is 200 with updated name; DB row reflects change.
#         userId is preserved from the original record (not caller-supplied).
#
# Negative: PUT on a non-existent UUID returns 404.
# ============================================================
echo "--- AC-4 (REQ-022): PUT /api/pipelines/{id} updates pipeline ---"

UPDATE_BODY_JSON() {
  cat <<EOF
{
  "name": "verifier-task-013-pipeline-a-updated",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {"rows": 20},
    "outputSchema": ["id", "name", "value"]
  },
  "processConfig": {
    "connectorType": "passthrough",
    "config": {},
    "inputMappings": [
      {"sourceField": "id",    "targetField": "record_id"},
      {"sourceField": "name",  "targetField": "label"},
      {"sourceField": "value", "targetField": "amount"}
    ],
    "outputSchema": ["record_id", "label", "amount"]
  },
  "sinkConfig": {
    "connectorType": "demo-sink",
    "config": {"target": "file"},
    "inputMappings": [
      {"sourceField": "record_id", "targetField": "id"},
      {"sourceField": "label",     "targetField": "name"},
      {"sourceField": "amount",    "targetField": "value"}
    ]
  }
}
EOF
}

PUT_STATUS=$(curl -s -o /tmp/TASK013-put.json -w "%{http_code}" \
  -X PUT "$API_URL/api/pipelines/$PIPELINE_ID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "$(UPDATE_BODY_JSON)")
PUT_BODY=$(cat /tmp/TASK013-put.json 2>/dev/null || echo "")

assert_status "AC-4: PUT /api/pipelines/{id} returns 200" 200 "$PUT_STATUS"

PUT_NAME=$(echo "$PUT_BODY" | grep -o '"name":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
if [ "$PUT_NAME" = "verifier-task-013-pipeline-a-updated" ]; then
  pass "AC-4: PUT response contains updated name"
else
  fail "AC-4: PUT response name not updated" "expected 'verifier-task-013-pipeline-a-updated', got '$PUT_NAME'"
fi

# Verify DB reflects the update.
DB_NAME=$(db_query "SELECT name FROM pipelines WHERE id = '${PIPELINE_ID}';" 2>/dev/null | tr -d '[:space:]')
if [ "$DB_NAME" = "verifier-task-013-pipeline-a-updated" ]; then
  pass "AC-4: database row reflects updated name"
else
  fail "AC-4: database name not updated" "DB has '$DB_NAME'"
fi

# Verify userId is preserved.
PUT_USER_ID=$(echo "$PUT_BODY" | grep -o '"userId":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
if [ "$PUT_USER_ID" = "$ADMIN_USER_ID" ]; then
  pass "AC-4: userId preserved in PUT response (not overwritten)"
else
  fail "AC-4: userId changed after PUT" "expected '$ADMIN_USER_ID', got '$PUT_USER_ID'"
fi

# AC-4 negative: PUT on non-existent UUID returns 404.
PUT_404_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X PUT "$API_URL/api/pipelines/$NONEXIST_UUID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "$(UPDATE_BODY_JSON)")
assert_status "AC-4 negative: PUT non-existent pipeline returns 404" 404 "$PUT_404_STATUS"

echo ""

# ============================================================
# AC-5 (REQ-022): DELETE /api/pipelines/{id} deletes pipeline; returns 204.
#
# Create a disposable pipeline specifically for this test so that
# AC-6's 409 test can run independently on pipeline A.
#
# Given:  admin creates a disposable pipeline C.
# When:   admin calls DELETE /api/pipelines/{id}.
# Then:   response is 204 No Content; row absent from PostgreSQL.
#
# Negative: DELETE on a non-existent UUID returns 404.
# ============================================================
echo "--- AC-5 (REQ-022): DELETE /api/pipelines/{id} deletes pipeline, returns 204 ---"

# Create disposable pipeline C.
CREATE_C_STATUS=$(curl -s -o /tmp/TASK013-create-c.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "$(PIPELINE_BODY "verifier-task-013-pipeline-c")")
CREATE_C_BODY=$(cat /tmp/TASK013-create-c.json 2>/dev/null || echo "")
PIPELINE_C_ID=$(echo "$CREATE_C_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")

if [ "$CREATE_C_STATUS" = "201" ] && [ -n "$PIPELINE_C_ID" ]; then
  echo "  Setup: disposable pipeline C created (id=$PIPELINE_C_ID)"
else
  fail "AC-5 setup: could not create pipeline C for deletion test (HTTP $CREATE_C_STATUS)" \
    "body: $CREATE_C_BODY"
fi

DELETE_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X DELETE "$API_URL/api/pipelines/$PIPELINE_C_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")

assert_status "AC-5: DELETE /api/pipelines/{id} returns 204" 204 "$DELETE_STATUS"

DB_CHECK=$(db_query "SELECT COUNT(*) FROM pipelines WHERE id = '${PIPELINE_C_ID}';" 2>/dev/null | tr -d '[:space:]')
if [ "$DB_CHECK" = "0" ]; then
  pass "AC-5: pipeline row absent from PostgreSQL after DELETE"
else
  fail "AC-5: pipeline row still present in PostgreSQL after DELETE" "count=$DB_CHECK"
fi

# AC-5 negative: DELETE on non-existent UUID returns 404.
DELETE_404_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X DELETE "$API_URL/api/pipelines/$NONEXIST_UUID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-5 negative: DELETE non-existent pipeline returns 404" 404 "$DELETE_404_STATUS"

echo ""

# ============================================================
# AC-6 (REQ-022): DELETE /api/pipelines/{id} returns 409 when
#   active (non-terminal) tasks reference the pipeline.
#
# Given:  pipeline A exists (owned by admin).
# When:   a task with status='running' referencing pipeline A is
#         inserted directly into PostgreSQL.
# When:   admin calls DELETE /api/pipelines/{id}.
# Then:   response is 409 Conflict; pipeline row still present in DB.
#
# Active statuses (non-terminal): submitted, queued, assigned, running.
# Terminal statuses: completed, failed, cancelled.
#
# Negative: After updating the task to status='completed', a
#   subsequent DELETE returns 204 (pipeline is no longer blocked).
# ============================================================
echo "--- AC-6 (REQ-022): DELETE returns 409 when active tasks reference the pipeline ---"

# Insert a running task referencing pipeline A directly into PostgreSQL.
# execution_id uses the verifier prefix so it is cleaned up on teardown.
TASK_ID=$(db_query "
  INSERT INTO tasks (pipeline_id, user_id, status, execution_id, input)
  VALUES (
    '${PIPELINE_ID}',
    '${ADMIN_USER_ID}',
    'running',
    'verifier-task-013-running-001',
    '{}'
  )
  RETURNING id;" 2>/dev/null | grep -E '^[0-9a-f-]{36}$' | head -1)

if [ -n "$TASK_ID" ]; then
  echo "  Setup: running task inserted (id=$TASK_ID, pipeline=$PIPELINE_ID)"
else
  fail "AC-6 setup: could not insert running task into PostgreSQL" ""
fi

DELETE_409_STATUS=$(curl -s -o /tmp/TASK013-delete-409.json -w "%{http_code}" \
  -X DELETE "$API_URL/api/pipelines/$PIPELINE_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
DELETE_409_BODY=$(cat /tmp/TASK013-delete-409.json 2>/dev/null || echo "")

assert_status "AC-6: DELETE with active tasks returns 409" 409 "$DELETE_409_STATUS"

# Confirm pipeline still exists in DB (not deleted on 409).
DB_STILL=$(db_query "SELECT COUNT(*) FROM pipelines WHERE id = '${PIPELINE_ID}';" 2>/dev/null | tr -d '[:space:]')
if [ "$DB_STILL" = "1" ]; then
  pass "AC-6: pipeline still present in PostgreSQL after 409 (not deleted)"
else
  fail "AC-6: pipeline was deleted despite active tasks (expected 409 to leave it intact)" \
    "count=$DB_STILL"
fi

# AC-6 negative: updating task to 'completed' unblocks deletion.
db_query "UPDATE tasks SET status='completed' WHERE id='${TASK_ID}';" > /dev/null 2>&1
DELETE_AFTER_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X DELETE "$API_URL/api/pipelines/$PIPELINE_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-6 negative: DELETE succeeds (204) after task reaches terminal status" \
  204 "$DELETE_AFTER_STATUS"

echo ""

# ============================================================
# AC-7 (REQ-022): Non-owner non-admin gets 403 on GET, PUT, DELETE
#   for a pipeline owned by another user. Admin always succeeds.
#
# Given:  pipeline B is owned by the non-admin user (verifier-user-013).
#         The admin user is a different user.
# When:   admin calls GET /api/pipelines/{id} on pipeline B.
# Then:   200 (admin can access any pipeline).
# When:   non-admin user calls GET /api/pipelines/{id} on pipeline A (admin's).
# Then:   403 Forbidden.
# When:   non-admin user calls PUT /api/pipelines/{id} on pipeline A.
# Then:   403 Forbidden.
# When:   non-admin user calls DELETE /api/pipelines/{id} on pipeline A.
# Then:   403 Forbidden (pipeline A was deleted by AC-6 cleanup, recreate).
#
# Note: pipeline A was deleted in AC-6 negative test. Recreate for AC-7 DELETE test.
# ============================================================
echo "--- AC-7 (REQ-022): Non-owner non-admin gets 403; admin gets 200/204 ---"

# Recreate pipeline A for the 403 DELETE test (it was deleted in AC-6 negative).
CREATE_FINAL_STATUS=$(curl -s -o /tmp/TASK013-create-final.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "$(PIPELINE_BODY "verifier-task-013-pipeline-a-final")")
CREATE_FINAL_BODY=$(cat /tmp/TASK013-create-final.json 2>/dev/null || echo "")
PIPELINE_FINAL_ID=$(echo "$CREATE_FINAL_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")

if [ "$CREATE_FINAL_STATUS" = "201" ] && [ -n "$PIPELINE_FINAL_ID" ]; then
  echo "  Setup: pipeline for 403 tests created (id=$PIPELINE_FINAL_ID)"
else
  fail "AC-7 setup: could not recreate admin pipeline for 403 test (HTTP $CREATE_FINAL_STATUS)" \
    "body: $CREATE_FINAL_BODY"
fi

# Admin can GET pipeline B (non-admin's pipeline).
ADMIN_GET_B_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/pipelines/$PIPELINE_B_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-7: admin can GET another user's pipeline (200)" 200 "$ADMIN_GET_B_STATUS"

# Non-admin user gets 403 on GET for admin's pipeline.
USER_GET_ADMIN_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/pipelines/$PIPELINE_FINAL_ID" \
  -H "Authorization: Bearer $VERIFIER_TOKEN")
assert_status "AC-7: non-owner GET admin's pipeline returns 403" 403 "$USER_GET_ADMIN_STATUS"

# Non-admin user gets 403 on PUT for admin's pipeline.
USER_PUT_ADMIN_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X PUT "$API_URL/api/pipelines/$PIPELINE_FINAL_ID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $VERIFIER_TOKEN" \
  -d "$(UPDATE_BODY_JSON)")
assert_status "AC-7: non-owner PUT admin's pipeline returns 403" 403 "$USER_PUT_ADMIN_STATUS"

# Non-admin user gets 403 on DELETE for admin's pipeline.
USER_DELETE_ADMIN_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X DELETE "$API_URL/api/pipelines/$PIPELINE_FINAL_ID" \
  -H "Authorization: Bearer $VERIFIER_TOKEN")
assert_status "AC-7: non-owner DELETE admin's pipeline returns 403" 403 "$USER_DELETE_ADMIN_STATUS"

# Admin can DELETE non-admin's pipeline B (cross-user access confirmed).
ADMIN_DELETE_B_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X DELETE "$API_URL/api/pipelines/$PIPELINE_B_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-7: admin can DELETE another user's pipeline (204)" 204 "$ADMIN_DELETE_B_STATUS"

echo ""

# ============================================================
# Cleanup
# ============================================================
echo "--- Cleanup ---"
db_query "DELETE FROM tasks     WHERE execution_id LIKE 'verifier-task-013-%';" > /dev/null 2>&1 || true
db_query "DELETE FROM pipelines WHERE name LIKE 'verifier-task-013-%';" > /dev/null 2>&1 || true
db_query "DELETE FROM users     WHERE username = 'verifier-user-013';" > /dev/null 2>&1 || true
docker exec nexusflow-redis-1 redis-cli DEL "session:${VERIFIER_TOKEN}" > /dev/null 2>&1 || true
echo "  Test data cleaned up."
echo ""

# ============================================================
# Summary
# ============================================================
echo "========================================"
printf "Results: PASS=%s  FAIL=%s\n" "$PASS" "$FAIL"
echo "========================================"
echo ""
echo "Detailed results:"
for r in "${RESULTS[@]}"; do
  echo "  $r"
done
echo ""

if [ "$FAIL" -gt 0 ]; then
  printf "${RED}OVERALL: FAIL${RESET}\n"
  exit 1
else
  printf "${GREEN}OVERALL: PASS${RESET}\n"
  exit 0
fi
