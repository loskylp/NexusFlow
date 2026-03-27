#!/usr/bin/env bash
# TASK-008 Acceptance Tests — Task lifecycle state tracking and query API
# REQ-009: Task state transitions are persisted in task_state_log and queryable via API
# REQ-017: Task list and detail endpoints enforce visibility isolation by role
#
# Acceptance criteria:
#   AC-1: GET /api/tasks returns list of tasks (user sees own, admin sees all)
#   AC-2: GET /api/tasks/{id} returns task details including current status
#   AC-3: GET /api/tasks/{id} includes state transition history from task_state_log
#   AC-4: Unauthenticated requests return 401
#   AC-5: Non-owner non-admin gets 403 on GET /api/tasks/{id}
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-008-acceptance.sh
#
# Requires: curl, docker exec (for psql access)
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

assert_status() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  if [ "$actual" -eq "$expected" ] 2>/dev/null; then
    pass "$name"
  else
    fail "$name" "expected HTTP $expected, got HTTP $actual"
  fi
}

echo ""
echo "=== TASK-008 Acceptance Tests — Task lifecycle state tracking and query API ==="
echo "    API: $API_URL"
echo ""

# ---------------------------------------------------------------------------
# Setup: Admin login
# ---------------------------------------------------------------------------
echo "--- Setup: admin login ---"

LOGIN_STATUS=$(curl -s -o /tmp/TASK008-login-admin.json -w "%{http_code}" \
  -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}')
LOGIN_BODY=$(cat /tmp/TASK008-login-admin.json 2>/dev/null || echo "")

if [ "$LOGIN_STATUS" != "200" ]; then
  echo "  FATAL: admin login failed (HTTP $LOGIN_STATUS) — cannot continue"
  exit 1
fi

ADMIN_TOKEN=$(echo "$LOGIN_BODY" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$ADMIN_TOKEN" ]; then
  echo "  FATAL: no token in admin login response — cannot continue"
  exit 1
fi
echo "  admin session token obtained"

# ---------------------------------------------------------------------------
# Setup: Obtain admin user ID
# ---------------------------------------------------------------------------
ADMIN_USER_ID=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM users WHERE username = 'admin';" 2>&1 | tr -d ' \n')

if [ -z "$ADMIN_USER_ID" ]; then
  echo "  FATAL: could not retrieve admin user ID — cannot continue"
  exit 1
fi
echo "  admin user id: $ADMIN_USER_ID"

# ---------------------------------------------------------------------------
# Setup: Create a second (non-admin) user for isolation testing.
# No registration endpoint exists in Cycle 1. The user is inserted directly into
# PostgreSQL, and the session token is injected directly into Redis using the same
# key format that auth.Middleware reads ("session:<token>").
# This is the approved integration test pattern — see TASK-005 acceptance tests for
# the same Redis injection pattern used for revoked-token testing.
# ---------------------------------------------------------------------------
echo "--- Setup: create non-admin user 'task008user' via PostgreSQL + Redis session ---"

# Insert user into PostgreSQL (idempotent — ON CONFLICT DO NOTHING)
# password_hash value is a placeholder; we do not exercise password auth for this user.
# The session token is injected directly into Redis, bypassing the login flow.
docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO users (username, password_hash, role)
   VALUES ('task008user', 'placeholder-not-used-session-injected-directly', 'user')
   ON CONFLICT (username) DO NOTHING;" > /dev/null 2>&1

USER_ID=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM users WHERE username = 'task008user';" 2>&1 | tr -d ' \n')

if [ -z "$USER_ID" ]; then
  echo "  FATAL: could not create or find task008user in PostgreSQL — cannot continue"
  exit 1
fi
echo "  task008user id: $USER_ID"

# Inject a valid session token into Redis for task008user (role: user)
USER_TOKEN="task008-acceptance-token-$(date +%s)"
USER_SESSION_JSON="{\"userId\":\"$USER_ID\",\"role\":\"user\",\"createdAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
docker exec nexusflow-redis-1 redis-cli SET "session:$USER_TOKEN" "$USER_SESSION_JSON" EX 3600 > /dev/null 2>&1
echo "  task008user session token injected into Redis"

# ---------------------------------------------------------------------------
# Setup: Insert a test pipeline owned by admin
# ---------------------------------------------------------------------------
echo "--- Setup: insert test pipeline ---"

PIPELINE_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO pipelines (name, user_id, data_source_config, process_config, sink_config)
   VALUES (
     'task008-test-pipeline',
     '$ADMIN_USER_ID',
     '{\"connectorType\":\"demo\",\"config\":{},\"outputSchema\":[\"field1\"]}',
     '{\"connectorType\":\"demo\",\"config\":{},\"inputMappings\":[],\"outputSchema\":[\"field1\"]}',
     '{\"connectorType\":\"demo\",\"config\":{},\"inputMappings\":[]}'
   )
   RETURNING id;" 2>&1 | tr -d ' \n')

TEST_PIPELINE_ID=$(echo "$PIPELINE_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")
if [ -z "$TEST_PIPELINE_ID" ]; then
  echo "  FATAL: could not insert test pipeline (result: $PIPELINE_RESULT) — cannot continue"
  exit 1
fi
echo "  test pipeline id: $TEST_PIPELINE_ID"

# ---------------------------------------------------------------------------
# Setup: Submit a task as admin so we have a task to query
# ---------------------------------------------------------------------------
echo "--- Setup: submit a task as admin ---"

SUBMIT_STATUS=$(curl -s -o /tmp/TASK008-submit.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"etl\"],\"input\":{\"key\":\"task008\"}}")
SUBMIT_BODY=$(cat /tmp/TASK008-submit.json 2>/dev/null || echo "")

if [ "$SUBMIT_STATUS" != "201" ]; then
  echo "  FATAL: task submission failed (HTTP $SUBMIT_STATUS body=$SUBMIT_BODY) — cannot continue"
  exit 1
fi

TASK_ID=$(echo "$SUBMIT_BODY" | grep -o '"taskId":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$TASK_ID" ]; then
  echo "  FATAL: no taskId in submit response — cannot continue"
  exit 1
fi
echo "  task submitted: $TASK_ID"
echo ""

# ---------------------------------------------------------------------------
# AC-1 (positive): GET /api/tasks — admin sees all tasks (including the submitted one)
# REQ-017: Admin role receives all tasks across all users
#
# Given: an authenticated admin session and at least one task exists
# When:  GET /api/tasks with admin token
# Then:  200 OK; response is a JSON array containing the submitted task
# ---------------------------------------------------------------------------
echo "AC-1: GET /api/tasks — admin sees all tasks"

AC1_STATUS=$(curl -s -o /tmp/TASK008-ac1.json -w "%{http_code}" \
  -X GET "$API_URL/api/tasks" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
AC1_BODY=$(cat /tmp/TASK008-ac1.json 2>/dev/null || echo "")

assert_status "AC-1a [REQ-017]: GET /api/tasks as admin returns 200" 200 "$AC1_STATUS"

# Response must be a JSON array
if echo "$AC1_BODY" | grep -q '^\['; then
  pass "AC-1b [REQ-017]: response body is a JSON array"
else
  fail "AC-1b [REQ-017]: response body is a JSON array" "body does not start with '[': $AC1_BODY"
fi

# Array must contain the submitted task ID
if echo "$AC1_BODY" | grep -q "$TASK_ID"; then
  pass "AC-1c [REQ-017]: admin task list contains the submitted task $TASK_ID"
else
  fail "AC-1c [REQ-017]: admin task list contains the submitted task $TASK_ID" \
    "task ID not found in response"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-1 (positive): GET /api/tasks — user sees only own tasks, not other users' tasks
# REQ-017: User role receives only tasks where user_id = session.UserID (Domain Invariant 5)
#
# Given: task008user is authenticated; the only submitted task is owned by admin
# When:  GET /api/tasks with task008user token
# Then:  200 OK; response is a JSON array that does NOT contain the admin's task
#
# [VERIFIER-ADDED]: User list returns empty array (not 403) when user has no tasks
# ---------------------------------------------------------------------------
echo "AC-1 (user isolation): GET /api/tasks — user sees only own tasks"

AC1U_STATUS=$(curl -s -o /tmp/TASK008-ac1u.json -w "%{http_code}" \
  -X GET "$API_URL/api/tasks" \
  -H "Authorization: Bearer $USER_TOKEN")
AC1U_BODY=$(cat /tmp/TASK008-ac1u.json 2>/dev/null || echo "")

assert_status "AC-1d [REQ-017]: GET /api/tasks as task008user returns 200" 200 "$AC1U_STATUS"

if echo "$AC1U_BODY" | grep -q "$TASK_ID"; then
  fail "AC-1e [REQ-017]: task008user does NOT see admin's task (visibility isolation)" \
    "admin task $TASK_ID appears in task008user's list"
else
  pass "AC-1e [REQ-017]: admin's task $TASK_ID not visible to task008user (visibility isolation)"
fi

# [VERIFIER-ADDED] The list must be a JSON array (even if empty) — not 403
if echo "$AC1U_BODY" | grep -q '^\['; then
  pass "AC-1f [REQ-017] [VERIFIER-ADDED]: user with no tasks gets empty array, not 403"
else
  fail "AC-1f [REQ-017] [VERIFIER-ADDED]: user with no tasks gets empty array, not 403" \
    "body: $AC1U_BODY"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-2 (positive): GET /api/tasks/{id} returns task details including current status
# REQ-009: Task detail endpoint returns full task data
#
# Given: an authenticated admin session and a known task ID
# When:  GET /api/tasks/{id}
# Then:  200 OK; response body includes "task" object with matching ID and a "status" field
# ---------------------------------------------------------------------------
echo "AC-2: GET /api/tasks/{id} returns task details including current status"

AC2_STATUS=$(curl -s -o /tmp/TASK008-ac2.json -w "%{http_code}" \
  -X GET "$API_URL/api/tasks/$TASK_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
AC2_BODY=$(cat /tmp/TASK008-ac2.json 2>/dev/null || echo "")

assert_status "AC-2a [REQ-009]: GET /api/tasks/{id} as admin returns 200" 200 "$AC2_STATUS"

# Response must contain a "task" object
if echo "$AC2_BODY" | grep -q '"task"'; then
  pass "AC-2b [REQ-009]: response body contains 'task' key"
else
  fail "AC-2b [REQ-009]: response body contains 'task' key" "body: $AC2_BODY"
fi

# Task object must contain the correct ID
if echo "$AC2_BODY" | grep -q "$TASK_ID"; then
  pass "AC-2c [REQ-009]: task object contains the correct task ID"
else
  fail "AC-2c [REQ-009]: task object contains the correct task ID" \
    "task ID $TASK_ID not found in response"
fi

# Task object must have a "status" field
if echo "$AC2_BODY" | grep -q '"status"'; then
  pass "AC-2d [REQ-009]: task object contains 'status' field"
else
  fail "AC-2d [REQ-009]: task object contains 'status' field" "body: $AC2_BODY"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-3 (positive): GET /api/tasks/{id} includes state transition history from task_state_log
# REQ-009: stateHistory array is always present; contains submitted→queued transition
#
# Given: a task that has been submitted (submitted→queued transition logged)
# When:  GET /api/tasks/{id}
# Then:  response body contains "stateHistory" as a JSON array with at least one entry
#
# [VERIFIER-ADDED] AC-3b: stateHistory is a JSON array even when the task has no transitions
# ---------------------------------------------------------------------------
echo "AC-3: GET /api/tasks/{id} includes stateHistory from task_state_log"

# AC2_BODY was fetched above — reuse it
if echo "$AC2_BODY" | grep -q '"stateHistory"'; then
  pass "AC-3a [REQ-009]: response body contains 'stateHistory' key"
else
  fail "AC-3a [REQ-009]: response body contains 'stateHistory' key" "body: $AC2_BODY"
fi

# stateHistory must be a JSON array (starts with '[' after the key)
STATE_HISTORY_VALUE=$(echo "$AC2_BODY" | grep -o '"stateHistory":\[.*\]' | head -1 || echo "")
if [ -n "$STATE_HISTORY_VALUE" ]; then
  pass "AC-3b [REQ-009]: stateHistory is a JSON array"
else
  fail "AC-3b [REQ-009]: stateHistory is a JSON array" \
    "could not find stateHistory array in body: $AC2_BODY"
fi

# stateHistory must have at least one entry (submitted→queued was logged on submission)
# Verify via PostgreSQL that the log entry exists
STATE_LOG_COUNT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT COUNT(*) FROM task_state_log WHERE task_id = '$TASK_ID';" 2>&1 | tr -d ' \n')

if [ "$STATE_LOG_COUNT" -ge 1 ] 2>/dev/null; then
  pass "AC-3c [REQ-009]: task_state_log has $STATE_LOG_COUNT entries for task $TASK_ID"
else
  fail "AC-3c [REQ-009]: task_state_log has at least one entry for task $TASK_ID" \
    "got count=$STATE_LOG_COUNT"
fi

# The stateHistory array in the API response must be non-empty (consistent with DB)
if echo "$AC2_BODY" | grep -q '"stateHistory":\[{'; then
  pass "AC-3d [REQ-009]: stateHistory array in API response is non-empty"
else
  # Could be empty if the regex doesn't match exactly — check differently
  HISTORY_CONTENT=$(echo "$AC2_BODY" | grep -o '"stateHistory":\[[^]]*\]' | head -1 || echo "")
  if echo "$HISTORY_CONTENT" | grep -q '"fromState"\|"toState"\|"timestamp"'; then
    pass "AC-3d [REQ-009]: stateHistory array contains transition entries"
  else
    fail "AC-3d [REQ-009]: stateHistory array in API response is non-empty" \
      "stateHistory appears empty or malformed: $HISTORY_CONTENT"
  fi
fi

echo ""

# ---------------------------------------------------------------------------
# AC-4 (negative): Unauthenticated requests return 401
# REQ-017: Protected endpoints require authentication
#
# Given: no Authorization header is provided
# When:  GET /api/tasks without token
# Then:  401 Unauthorized
#
# And when: GET /api/tasks/{id} without token
# Then:     401 Unauthorized
#
# [VERIFIER-ADDED] AC-4c: A non-existent token also returns 401
# ---------------------------------------------------------------------------
echo "AC-4: Unauthenticated requests return 401"

# GET /api/tasks — no auth
AC4A_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/tasks")
assert_status "AC-4a [REQ-017]: GET /api/tasks without auth returns 401" 401 "$AC4A_STATUS"

# GET /api/tasks/{id} — no auth
AC4B_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/tasks/$TASK_ID")
assert_status "AC-4b [REQ-017]: GET /api/tasks/{id} without auth returns 401" 401 "$AC4B_STATUS"

# [VERIFIER-ADDED] Non-existent token
AC4C_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/tasks" \
  -H "Authorization: Bearer 0000000000000000000000000000000000000000000000000000000000000000")
assert_status "AC-4c [REQ-017] [VERIFIER-ADDED]: non-existent token returns 401 on GET /api/tasks" 401 "$AC4C_STATUS"

AC4D_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/tasks/$TASK_ID" \
  -H "Authorization: Bearer 0000000000000000000000000000000000000000000000000000000000000000")
assert_status "AC-4d [REQ-017] [VERIFIER-ADDED]: non-existent token returns 401 on GET /api/tasks/{id}" 401 "$AC4D_STATUS"

echo ""

# ---------------------------------------------------------------------------
# AC-5 (negative): Non-owner non-admin gets 403 on GET /api/tasks/{id}
# REQ-017: Ownership enforcement — non-owner, non-admin caller is forbidden
#
# Given: task008user (non-admin) and a task owned by admin
# When:  GET /api/tasks/{TASK_ID} with task008user token
# Then:  403 Forbidden; no task data is disclosed
#
# [VERIFIER-ADDED] AC-5b: 403 response body is structured JSON (not empty / not 500)
# [VERIFIER-ADDED] AC-5c: Admin can read the same task (positive counterpart)
# ---------------------------------------------------------------------------
echo "AC-5: Non-owner non-admin gets 403 on GET /api/tasks/{id}"

AC5_STATUS=$(curl -s -o /tmp/TASK008-ac5.json -w "%{http_code}" \
  -X GET "$API_URL/api/tasks/$TASK_ID" \
  -H "Authorization: Bearer $USER_TOKEN")
AC5_BODY=$(cat /tmp/TASK008-ac5.json 2>/dev/null || echo "")

assert_status "AC-5a [REQ-017]: non-owner non-admin gets 403 on GET /api/tasks/{id}" 403 "$AC5_STATUS"

# [VERIFIER-ADDED] 403 response must be structured JSON — no task data disclosed
AC5_ERROR=$(echo "$AC5_BODY" | grep -o '"error":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -n "$AC5_ERROR" ]; then
  pass "AC-5b [REQ-017] [VERIFIER-ADDED]: 403 response body is structured JSON with error field ('$AC5_ERROR')"
else
  fail "AC-5b [REQ-017] [VERIFIER-ADDED]: 403 response body is structured JSON with error field" \
    "body: $AC5_BODY"
fi

# No task data must be disclosed in the 403 response
if echo "$AC5_BODY" | grep -q "$TASK_ID"; then
  fail "AC-5c [REQ-017] [VERIFIER-ADDED]: 403 response does not disclose task data" \
    "task ID found in 403 response body — data leaked"
else
  pass "AC-5c [REQ-017] [VERIFIER-ADDED]: 403 response does not disclose task ID or task data"
fi

# [VERIFIER-ADDED] Admin can read the same task — positive counterpart to AC-5
AC5D_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/tasks/$TASK_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-5d [REQ-017] [VERIFIER-ADDED]: admin can read task owned by another user (200)" 200 "$AC5D_STATUS"

echo ""

# ---------------------------------------------------------------------------
# [VERIFIER-ADDED] GET /api/tasks/{id} with invalid UUID path segment returns 400
# REQ-009: Malformed IDs must be rejected before any DB lookup
# ---------------------------------------------------------------------------
echo "[VERIFIER-ADDED] Invalid UUID path segment returns 400"

AC_EXTRA_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/tasks/not-a-valid-uuid" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-extra [REQ-009] [VERIFIER-ADDED]: non-UUID task ID in path returns 400" 400 "$AC_EXTRA_STATUS"

echo ""

# ---------------------------------------------------------------------------
# [VERIFIER-ADDED] GET /api/tasks/{id} for a non-existent UUID returns 404
# REQ-009: Unknown task must return 404, not 500
# ---------------------------------------------------------------------------
echo "[VERIFIER-ADDED] Non-existent task ID returns 404"

NONEXISTENT_UUID="00000000-dead-beef-cafe-000000000000"
AC_404_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X GET "$API_URL/api/tasks/$NONEXISTENT_UUID" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-404 [REQ-009] [VERIFIER-ADDED]: non-existent task UUID returns 404" 404 "$AC_404_STATUS"

echo ""

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo "=== Results ==="
for r in "${RESULTS[@]}"; do
  echo "  $r"
done
echo ""
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo ""

if [ "$FAIL" -eq 0 ]; then
  echo "VERDICT: PASS"
  exit 0
else
  echo "VERDICT: FAIL"
  exit 1
fi
