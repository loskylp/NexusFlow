#!/usr/bin/env bash
# TASK-005 Acceptance Tests — Task Submission via REST API
# REQ-001: Task submission returns a unique task ID
# REQ-003: Tasks transition to "queued" after submission, message present in Redis stream
# REQ-009: State transitions logged in task_state_log
#
# AC-1: POST /api/tasks with valid payload returns 201 with unique task ID
# AC-2: Task record exists in PostgreSQL with status "queued"; submitted→queued transition in task_state_log
# AC-3: Task message exists in the appropriate Redis stream (queue:{tag})
# AC-4: POST /api/tasks with invalid pipeline reference returns 400 with structured error
# AC-5: POST /api/tasks without retryConfig creates task with default retry settings (max_retries: 3, backoff: exponential)
# AC-6: Unauthenticated request returns 401
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-005-acceptance.sh
#
# Requires: curl, docker exec (for psql and redis-cli access)
# Services required: API server, PostgreSQL, Redis (all running via Docker Compose)

set -euo pipefail

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
  if [ "$actual" -eq "$expected" ]; then
    pass "$name"
  else
    fail "$name" "expected HTTP $expected, got HTTP $actual"
  fi
}

echo ""
echo "=== TASK-005 Acceptance Tests — Task Submission via REST API ==="
echo "    API: $API_URL"
echo ""

# ---------------------------------------------------------------------------
# Setup: Obtain admin session token
# Given: admin user exists (seeded on startup by TASK-003)
# When:  POST /api/auth/login with {"username":"admin","password":"admin"}
# Then:  200 OK with token
# ---------------------------------------------------------------------------
echo "--- Setup: admin login ---"

LOGIN_STATUS=$(curl -s -o /tmp/TASK005-login.json -w "%{http_code}" \
  -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}')
LOGIN_BODY=$(cat /tmp/TASK005-login.json 2>/dev/null || echo "")

if [ "$LOGIN_STATUS" != "200" ]; then
  echo "  FATAL: admin login failed (HTTP $LOGIN_STATUS) — cannot continue"
  exit 1
fi

TOKEN=$(echo "$LOGIN_BODY" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$TOKEN" ]; then
  echo "  FATAL: no token in login response — cannot continue"
  exit 1
fi
echo "  admin session token obtained (${#TOKEN} chars)"

# ---------------------------------------------------------------------------
# Setup: Obtain admin user ID from PostgreSQL (needed to own the seeded pipeline)
# ---------------------------------------------------------------------------
ADMIN_USER_ID=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM users WHERE username = 'admin';" 2>&1 | tr -d ' \n')

if [ -z "$ADMIN_USER_ID" ]; then
  echo "  FATAL: could not retrieve admin user ID from PostgreSQL — cannot continue"
  exit 1
fi
echo "  admin user id: $ADMIN_USER_ID"

# ---------------------------------------------------------------------------
# Setup: Insert a test pipeline directly into PostgreSQL
# Pipeline CRUD (TASK-013) is not yet implemented; direct INSERT is the approved
# integration approach for TASK-005 verification.
# ---------------------------------------------------------------------------
echo "  inserting test pipeline into PostgreSQL..."

PIPELINE_INSERT_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO pipelines (name, user_id, data_source_config, process_config, sink_config)
   VALUES (
     'task005-test-pipeline',
     '$ADMIN_USER_ID',
     '{\"connectorType\":\"demo\",\"config\":{},\"outputSchema\":[\"field1\"]}',
     '{\"connectorType\":\"demo\",\"config\":{},\"inputMappings\":[],\"outputSchema\":[\"field1\"]}',
     '{\"connectorType\":\"demo\",\"config\":{},\"inputMappings\":[]}'
   )
   RETURNING id;" 2>&1 | tr -d ' \n')

TEST_PIPELINE_ID=$(echo "$PIPELINE_INSERT_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")

if [ -z "$TEST_PIPELINE_ID" ]; then
  echo "  FATAL: could not insert test pipeline (result: $PIPELINE_INSERT_RESULT) — cannot continue"
  exit 1
fi
echo "  test pipeline id: $TEST_PIPELINE_ID"
echo ""

# ---------------------------------------------------------------------------
# AC-1 (positive): POST /api/tasks with valid payload returns 201 with unique task ID
# REQ-001: Authenticated user submits a task and receives a unique task ID
#
# Given: an authenticated admin session and a valid pipeline ID
# When:  POST /api/tasks with a complete, valid body including pipelineId and tags
# Then:  response is 201 Created; body contains a non-empty "taskId" UUID and "status":"queued"
#
# [VERIFIER-ADDED] AC-1b: A second submission with the same payload produces a different task ID
# ---------------------------------------------------------------------------
echo "AC-1: Valid payload returns 201 with unique task ID"

SUBMIT_STATUS=$(curl -s -o /tmp/TASK005-submit1.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"etl\"],\"input\":{\"key\":\"value\"}}")
SUBMIT_BODY=$(cat /tmp/TASK005-submit1.json 2>/dev/null || echo "")

assert_status "AC-1a [REQ-001]: POST /api/tasks returns 201" 201 "$SUBMIT_STATUS"

TASK_ID=$(echo "$SUBMIT_BODY" | grep -o '"taskId":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -n "$TASK_ID" ] && echo "$TASK_ID" | grep -qE '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'; then
  pass "AC-1b [REQ-001]: response body contains a valid UUID taskId"
else
  fail "AC-1b [REQ-001]: response body contains a valid UUID taskId" "got taskId='$TASK_ID'"
fi

STATUS_FIELD=$(echo "$SUBMIT_BODY" | grep -o '"status":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ "$STATUS_FIELD" = "queued" ]; then
  pass "AC-1c [REQ-001]: response body status is 'queued'"
else
  fail "AC-1c [REQ-001]: response body status is 'queued'" "got status='$STATUS_FIELD'"
fi

# Second submission must produce a different task ID (uniqueness check)
SUBMIT2_STATUS=$(curl -s -o /tmp/TASK005-submit2.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"etl\"],\"input\":{\"key\":\"value\"}}")
SUBMIT2_BODY=$(cat /tmp/TASK005-submit2.json 2>/dev/null || echo "")
TASK_ID_2=$(echo "$SUBMIT2_BODY" | grep -o '"taskId":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")

if [ "$SUBMIT2_STATUS" = "201" ] && [ -n "$TASK_ID_2" ] && [ "$TASK_ID_2" != "$TASK_ID" ]; then
  pass "AC-1d [REQ-001] [VERIFIER-ADDED]: second submission produces a different task ID"
else
  fail "AC-1d [REQ-001] [VERIFIER-ADDED]: second submission produces a different task ID" \
    "first=$TASK_ID second=$TASK_ID_2 (status=$SUBMIT2_STATUS)"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-2 (positive): Task record exists in PostgreSQL with status "queued";
#                  submitted→queued transition logged in task_state_log
# REQ-003: Tasks transition to "queued" after validation
# REQ-009: Each transition is persisted as an audit log entry
#
# Given: a task was successfully submitted (AC-1 passed, TASK_ID is set)
# When:  query the tasks table for the task's status
# Then:  status = 'queued'
#
# And when: query task_state_log for the task
# Then:     at least one row exists with from_state='submitted' and to_state='queued'
#
# [VERIFIER-ADDED] AC-2c: The task_state_log entry has a non-empty reason
# ---------------------------------------------------------------------------
echo "AC-2: Task in PostgreSQL with status 'queued'; submitted→queued transition logged"

if [ -z "$TASK_ID" ]; then
  fail "AC-2a [REQ-003]: task status in PostgreSQL is 'queued'" "skipped — TASK_ID not set (AC-1 failed)"
  fail "AC-2b [REQ-009]: task_state_log has submitted→queued row" "skipped — TASK_ID not set"
else
  DB_STATUS=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
    "SELECT status FROM tasks WHERE id = '$TASK_ID';" 2>&1 | tr -d ' \n')

  if [ "$DB_STATUS" = "queued" ]; then
    pass "AC-2a [REQ-003]: tasks table status = 'queued' for task $TASK_ID"
  else
    fail "AC-2a [REQ-003]: tasks table status = 'queued' for task $TASK_ID" "got status='$DB_STATUS'"
  fi

  STATE_LOG_COUNT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
    "SELECT COUNT(*) FROM task_state_log WHERE task_id = '$TASK_ID' AND from_state = 'submitted' AND to_state = 'queued';" \
    2>&1 | tr -d ' \n')

  if [ "$STATE_LOG_COUNT" = "1" ]; then
    pass "AC-2b [REQ-009]: task_state_log has exactly one submitted→queued row for task $TASK_ID"
  else
    fail "AC-2b [REQ-009]: task_state_log has exactly one submitted→queued row for task $TASK_ID" \
      "got count=$STATE_LOG_COUNT"
  fi

  STATE_LOG_REASON=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
    "SELECT reason FROM task_state_log WHERE task_id = '$TASK_ID' AND from_state = 'submitted' AND to_state = 'queued' LIMIT 1;" \
    2>&1 | tr -d '\n' | sed 's/^ *//')

  if [ -n "$STATE_LOG_REASON" ]; then
    pass "AC-2c [REQ-009] [VERIFIER-ADDED]: task_state_log submitted→queued entry has non-empty reason ('$STATE_LOG_REASON')"
  else
    fail "AC-2c [REQ-009] [VERIFIER-ADDED]: task_state_log submitted→queued entry has non-empty reason" "reason is empty"
  fi
fi

echo ""

# ---------------------------------------------------------------------------
# AC-3 (positive): Task message exists in the appropriate Redis stream (queue:{tag})
# REQ-003: Tasks are enqueued to the correct Redis stream after validation
# ADR-001: Per-tag stream topology — tags drive stream selection
#
# Given: a task was submitted with tags=["etl"]
# When:  XLEN queue:etl is checked
# Then:  stream has at least one entry
#
# And when: XRANGE queue:etl is inspected
# Then:     at least one entry's payload contains the submitted task's ID
#
# [VERIFIER-ADDED] AC-3c: A task submitted with tags=["report"] appears in queue:report, not queue:etl
# ---------------------------------------------------------------------------
echo "AC-3: Task message in Redis stream queue:{tag}"

if [ -z "$TASK_ID" ]; then
  fail "AC-3a [REQ-003]: queue:etl stream has at least one entry" "skipped — TASK_ID not set (AC-1 failed)"
  fail "AC-3b [REQ-003]: queue:etl entry contains task ID" "skipped — TASK_ID not set"
else
  ETL_LEN=$(docker exec nexusflow-redis-1 redis-cli XLEN queue:etl 2>&1 | tr -d ' \n')
  if [ "$ETL_LEN" -ge 1 ] 2>/dev/null; then
    pass "AC-3a [REQ-003]: queue:etl stream has $ETL_LEN entries"
  else
    fail "AC-3a [REQ-003]: queue:etl stream has at least one entry" "XLEN returned '$ETL_LEN'"
  fi

  # XRANGE output: check if any entry contains the task ID
  RANGE_OUTPUT=$(docker exec nexusflow-redis-1 redis-cli XRANGE queue:etl - + 2>&1)
  if echo "$RANGE_OUTPUT" | grep -q "$TASK_ID"; then
    pass "AC-3b [REQ-003]: queue:etl contains an entry with task ID $TASK_ID"
  else
    fail "AC-3b [REQ-003]: queue:etl contains an entry with task ID $TASK_ID" \
      "task ID not found in XRANGE output"
  fi
fi

# [VERIFIER-ADDED] Submit a task with tag "report" and verify it lands in queue:report, not queue:etl
REPORT_STATUS=$(curl -s -o /tmp/TASK005-submit-report.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"report\"],\"input\":{}}")
REPORT_BODY=$(cat /tmp/TASK005-submit-report.json 2>/dev/null || echo "")
REPORT_TASK_ID=$(echo "$REPORT_BODY" | grep -o '"taskId":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")

if [ "$REPORT_STATUS" = "201" ] && [ -n "$REPORT_TASK_ID" ]; then
  REPORT_RANGE=$(docker exec nexusflow-redis-1 redis-cli XRANGE queue:report - + 2>&1)
  ETL_RANGE=$(docker exec nexusflow-redis-1 redis-cli XRANGE queue:etl - + 2>&1)
  if echo "$REPORT_RANGE" | grep -q "$REPORT_TASK_ID"; then
    pass "AC-3c [REQ-003] [VERIFIER-ADDED]: tag='report' task routed to queue:report stream"
  else
    fail "AC-3c [REQ-003] [VERIFIER-ADDED]: tag='report' task routed to queue:report stream" \
      "task $REPORT_TASK_ID not found in queue:report"
  fi
  if echo "$ETL_RANGE" | grep -q "$REPORT_TASK_ID"; then
    fail "AC-3d [REQ-003] [VERIFIER-ADDED]: tag='report' task NOT routed to queue:etl" \
      "task $REPORT_TASK_ID found in queue:etl (wrong stream)"
  else
    pass "AC-3d [REQ-003] [VERIFIER-ADDED]: tag='report' task correctly absent from queue:etl"
  fi
else
  fail "AC-3c [REQ-003] [VERIFIER-ADDED]: tag='report' task submitted successfully" \
    "status=$REPORT_STATUS body=$REPORT_BODY"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-4 (negative): POST /api/tasks with invalid pipeline reference returns 400 with structured error
# REQ-001: Submission with non-existent pipeline must be rejected
#
# Given: an authenticated session
# When:  POST /api/tasks with a well-formed UUID that does not exist in the pipelines table
# Then:  response is 400 Bad Request; body is {"error":"pipeline not found"} (structured JSON)
#
# [VERIFIER-ADDED] AC-4b: POST /api/tasks with a non-UUID pipelineId string also returns 400
# [VERIFIER-ADDED] AC-4c: Malformed JSON body returns 400
# [VERIFIER-ADDED] AC-4d: Missing tags field returns 400
# ---------------------------------------------------------------------------
echo "AC-4: Invalid pipeline reference returns 400 with structured error"

# Use a well-formed UUID that is guaranteed not to exist in the pipelines table
NONEXISTENT_PIPELINE_UUID="00000000-dead-beef-cafe-000000000000"

AC4_STATUS=$(curl -s -o /tmp/TASK005-ac4.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"pipelineId\":\"$NONEXISTENT_PIPELINE_UUID\",\"tags\":[\"etl\"]}")
AC4_BODY=$(cat /tmp/TASK005-ac4.json 2>/dev/null || echo "")

assert_status "AC-4a [REQ-001]: non-existent pipeline UUID returns 400" 400 "$AC4_STATUS"

AC4_ERROR=$(echo "$AC4_BODY" | grep -o '"error":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ "$AC4_ERROR" = "pipeline not found" ]; then
  pass "AC-4b [REQ-001]: error body is structured JSON with error='pipeline not found'"
else
  fail "AC-4b [REQ-001]: error body is structured JSON with error='pipeline not found'" \
    "got error='$AC4_ERROR' body='$AC4_BODY'"
fi

# [VERIFIER-ADDED] Non-UUID pipelineId format
AC4C_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"pipelineId":"not-a-uuid","tags":["etl"]}')
assert_status "AC-4c [REQ-001] [VERIFIER-ADDED]: non-UUID pipelineId format returns 400" 400 "$AC4C_STATUS"

# [VERIFIER-ADDED] Malformed JSON body
AC4D_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d 'NOT_VALID_JSON')
assert_status "AC-4d [REQ-001] [VERIFIER-ADDED]: malformed JSON body returns 400" 400 "$AC4D_STATUS"

# [VERIFIER-ADDED] Missing tags field
AC4E_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\"}")
assert_status "AC-4e [REQ-001] [VERIFIER-ADDED]: missing tags field returns 400" 400 "$AC4E_STATUS"

echo ""

# ---------------------------------------------------------------------------
# AC-5 (positive): POST /api/tasks without retryConfig creates task with default retry settings
#                  (max_retries: 3, backoff: exponential)
# REQ-001: Safe defaults applied when retryConfig is absent
#
# Given: an authenticated session and a valid pipeline
# When:  POST /api/tasks with no "retryConfig" field in the body
# Then:  the task record in PostgreSQL has retry_config = '{"maxRetries":3,"backoff":"exponential"}'
#
# [VERIFIER-ADDED] AC-5b: An explicit retryConfig in the body is preserved, not overridden
# ---------------------------------------------------------------------------
echo "AC-5: No retryConfig defaults to maxRetries=3, backoff=exponential"

AC5_STATUS=$(curl -s -o /tmp/TASK005-ac5.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"etl\"],\"input\":{}}")
AC5_BODY=$(cat /tmp/TASK005-ac5.json 2>/dev/null || echo "")
AC5_TASK_ID=$(echo "$AC5_BODY" | grep -o '"taskId":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")

if [ "$AC5_STATUS" != "201" ] || [ -z "$AC5_TASK_ID" ]; then
  fail "AC-5 setup: task submitted without retryConfig" "status=$AC5_STATUS body=$AC5_BODY"
else
  # Query the retry_config JSONB column directly
  RETRY_CONFIG=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
    "SELECT retry_config FROM tasks WHERE id = '$AC5_TASK_ID';" 2>&1 | tr -d ' \n')

  MAX_RETRIES=$(echo "$RETRY_CONFIG" | grep -o '"maxRetries":[0-9]*' | cut -d':' -f2 || echo "")
  BACKOFF=$(echo "$RETRY_CONFIG" | grep -o '"backoff":"[^"]*"' | cut -d'"' -f4 || echo "")

  if [ "$MAX_RETRIES" = "3" ]; then
    pass "AC-5a [REQ-001]: default maxRetries = 3 (got retry_config=$RETRY_CONFIG)"
  else
    fail "AC-5a [REQ-001]: default maxRetries = 3" "got maxRetries='$MAX_RETRIES' retry_config='$RETRY_CONFIG'"
  fi

  if [ "$BACKOFF" = "exponential" ]; then
    pass "AC-5b [REQ-001]: default backoff = 'exponential'"
  else
    fail "AC-5b [REQ-001]: default backoff = 'exponential'" "got backoff='$BACKOFF'"
  fi
fi

# [VERIFIER-ADDED] Explicit retryConfig is preserved
AC5C_STATUS=$(curl -s -o /tmp/TASK005-ac5c.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"etl\"],\"input\":{},\"retryConfig\":{\"maxRetries\":5,\"backoff\":\"linear\"}}")
AC5C_BODY=$(cat /tmp/TASK005-ac5c.json 2>/dev/null || echo "")
AC5C_TASK_ID=$(echo "$AC5C_BODY" | grep -o '"taskId":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")

if [ "$AC5C_STATUS" = "201" ] && [ -n "$AC5C_TASK_ID" ]; then
  EXPLICIT_RETRY=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
    "SELECT retry_config FROM tasks WHERE id = '$AC5C_TASK_ID';" 2>&1 | tr -d ' \n')
  EXPLICIT_MAX=$(echo "$EXPLICIT_RETRY" | grep -o '"maxRetries":[0-9]*' | cut -d':' -f2 || echo "")
  EXPLICIT_BACKOFF=$(echo "$EXPLICIT_RETRY" | grep -o '"backoff":"[^"]*"' | cut -d'"' -f4 || echo "")
  if [ "$EXPLICIT_MAX" = "5" ] && [ "$EXPLICIT_BACKOFF" = "linear" ]; then
    pass "AC-5c [REQ-001] [VERIFIER-ADDED]: explicit retryConfig preserved (maxRetries=5, backoff=linear)"
  else
    fail "AC-5c [REQ-001] [VERIFIER-ADDED]: explicit retryConfig preserved" \
      "got maxRetries='$EXPLICIT_MAX' backoff='$EXPLICIT_BACKOFF' retry_config='$EXPLICIT_RETRY'"
  fi
else
  fail "AC-5c [REQ-001] [VERIFIER-ADDED]: task with explicit retryConfig submitted" \
    "status=$AC5C_STATUS body=$AC5C_BODY"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-6 (negative): Unauthenticated request returns 401
# REQ-019: Protected endpoints require authentication
#
# Given: no session token is provided
# When:  POST /api/tasks without any Authorization header or session cookie
# Then:  response is 401 Unauthorized; body contains structured JSON {"error":"unauthorized"}
#
# [VERIFIER-ADDED] AC-6b: An expired or non-existent token in Authorization header also returns 401
# [VERIFIER-ADDED] AC-6c: A syntactically valid but revoked token returns 401 (not 400/500)
# ---------------------------------------------------------------------------
echo "AC-6: Unauthenticated request returns 401"

AC6_STATUS=$(curl -s -o /tmp/TASK005-ac6.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"etl\"]}")
AC6_BODY=$(cat /tmp/TASK005-ac6.json 2>/dev/null || echo "")

assert_status "AC-6a [REQ-019]: POST /api/tasks without auth returns 401" 401 "$AC6_STATUS"

AC6_ERROR=$(echo "$AC6_BODY" | grep -o '"error":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -n "$AC6_ERROR" ]; then
  pass "AC-6b [REQ-019]: 401 response body is structured JSON with an error field"
else
  fail "AC-6b [REQ-019]: 401 response body is structured JSON with an error field" \
    "error field empty or missing; body='$AC6_BODY'"
fi

# Non-existent token format
AC6C_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer 0000000000000000000000000000000000000000000000000000000000000000" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"etl\"]}")
assert_status "AC-6c [REQ-019] [VERIFIER-ADDED]: non-existent token returns 401" 401 "$AC6C_STATUS"

# Simulate a revoked token by injecting a session into Redis then deleting it
REVOKED_TOKEN="task005revoked00000000000000000000000000000000000000000000000002"
REVOKE_SESSION_JSON="{\"userId\":\"$ADMIN_USER_ID\",\"role\":\"admin\",\"createdAt\":\"2026-03-26T00:00:00Z\"}"
docker exec nexusflow-redis-1 redis-cli SET "session:$REVOKED_TOKEN" "$REVOKE_SESSION_JSON" EX 3600 > /dev/null 2>&1
docker exec nexusflow-redis-1 redis-cli DEL "session:$REVOKED_TOKEN" > /dev/null 2>&1

AC6D_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $REVOKED_TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"etl\"]}")
assert_status "AC-6d [REQ-019] [VERIFIER-ADDED]: revoked token returns 401" 401 "$AC6D_STATUS"

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
