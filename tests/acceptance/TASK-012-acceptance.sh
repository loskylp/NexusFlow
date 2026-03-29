#!/usr/bin/env bash
# TASK-012 Acceptance Tests — Task cancellation
# REQ-010: Cancel a running task
#
# Acceptance criteria:
#   AC-1: POST /api/tasks/{id}/cancel by task owner returns 204 and sets status to "cancelled"
#   AC-2: Admin can cancel any task
#   AC-3: Non-owner non-admin gets 403
#   AC-4: Cancelling a task in terminal state (completed/failed/cancelled) returns 409
#   AC-5: Running task: cancellation signal sent to worker via Redis, worker halts execution
#   AC-6: Cancellation creates a task_state_log entry
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-012-acceptance.sh
#
# Requires: curl, docker exec (for psql and redis-cli access)
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
  local detail="${2:-}"
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
echo "=== TASK-012 Acceptance Tests — Task cancellation ==="
echo "    API: $API_URL"
echo ""

# ---------------------------------------------------------------------------
# Setup: Admin login
# ---------------------------------------------------------------------------
echo "--- Setup: admin login ---"

LOGIN_STATUS=$(curl -s -o /tmp/TASK012-login-admin.json -w "%{http_code}" \
  -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}')
LOGIN_BODY=$(cat /tmp/TASK012-login-admin.json 2>/dev/null || echo "")

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
# Setup: Get admin user ID
# ---------------------------------------------------------------------------
ADMIN_USER_ID=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM users WHERE username = 'admin';" 2>&1 | tr -d ' \n')

if [ -z "$ADMIN_USER_ID" ]; then
  echo "  FATAL: could not retrieve admin user ID — cannot continue"
  exit 1
fi
echo "  admin user id: $ADMIN_USER_ID"

# ---------------------------------------------------------------------------
# Setup: Create a second (non-admin) user for AC-3
# Session token is injected directly into Redis — the approved integration
# test pattern (see TASK-008 acceptance tests).
# ---------------------------------------------------------------------------
echo "--- Setup: create non-admin user 'task012user' ---"

docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO users (username, password_hash, role)
   VALUES ('task012user', 'placeholder-not-used', 'user')
   ON CONFLICT (username) DO NOTHING;" > /dev/null 2>&1

USER_ID=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM users WHERE username = 'task012user';" 2>&1 | tr -d ' \n')

if [ -z "$USER_ID" ]; then
  echo "  FATAL: could not create or find task012user in PostgreSQL — cannot continue"
  exit 1
fi
echo "  task012user id: $USER_ID"

USER_TOKEN="task012-acceptance-token-$(date +%s)"
USER_SESSION_JSON="{\"userId\":\"$USER_ID\",\"role\":\"user\",\"createdAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
docker exec nexusflow-redis-1 redis-cli SET "session:$USER_TOKEN" "$USER_SESSION_JSON" EX 3600 > /dev/null 2>&1
echo "  task012user session token injected into Redis"

# ---------------------------------------------------------------------------
# Setup: Insert a test pipeline owned by admin
# ---------------------------------------------------------------------------
echo "--- Setup: create test pipeline ---"

PIPELINE_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO pipelines (name, user_id, data_source_config, process_config, sink_config)
   VALUES (
     'task012-cancel-test-pipeline',
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
echo ""

# ---------------------------------------------------------------------------
# AC-1 (positive): Owner cancels own queued task — 204, status becomes "cancelled"
# REQ-010: A cancel request from the task owner transitions the task to "cancelled"
#
# Given: admin has a queued task belonging to them
# When:  admin sends POST /api/tasks/{id}/cancel
# Then:  HTTP 204 No Content; task status in DB is "cancelled"
# ---------------------------------------------------------------------------
echo "AC-1: Owner cancels own task — 204 + status = cancelled"

# Submit a new task (admin owns it)
AC1_SUBMIT_STATUS=$(curl -s -o /tmp/TASK012-ac1-submit.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "{\"pipelineId\":\"$TEST_PIPELINE_ID\",\"tags\":[\"nomatchtag012\"],\"input\":{}}")
AC1_SUBMIT_BODY=$(cat /tmp/TASK012-ac1-submit.json 2>/dev/null || echo "")

if [ "$AC1_SUBMIT_STATUS" != "201" ]; then
  echo "  FATAL: AC-1 task submission failed (HTTP $AC1_SUBMIT_STATUS, body=$AC1_SUBMIT_BODY)"
  exit 1
fi
AC1_TASK_ID=$(echo "$AC1_SUBMIT_BODY" | grep -o '"taskId":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$AC1_TASK_ID" ]; then
  echo "  FATAL: AC-1 no taskId in submit response — cannot continue"
  exit 1
fi
echo "  task submitted for AC-1: $AC1_TASK_ID"

# Cancel the task as owner (admin)
AC1_CANCEL_STATUS=$(curl -s -o /tmp/TASK012-ac1-cancel.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/$AC1_TASK_ID/cancel" \
  -H "Authorization: Bearer $ADMIN_TOKEN")

assert_status "AC-1a [REQ-010]: owner POST /cancel returns 204" 204 "$AC1_CANCEL_STATUS"

# Verify task status is "cancelled" in DB
AC1_DB_STATUS=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id = '$AC1_TASK_ID';" 2>&1 | tr -d ' \n')

if [ "$AC1_DB_STATUS" = "cancelled" ]; then
  pass "AC-1b [REQ-010]: task status is 'cancelled' in database after owner cancel"
else
  fail "AC-1b [REQ-010]: task status is 'cancelled' in database after owner cancel" \
    "got status='$AC1_DB_STATUS'"
fi

# Negative counterpart: a second cancel on the already-cancelled task must return 409
AC1_NEG_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/$AC1_TASK_ID/cancel" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "AC-1c [REQ-010] [VERIFIER-ADDED]: double-cancel (cancelled->cancel) returns 409" 409 "$AC1_NEG_STATUS"

echo ""

# ---------------------------------------------------------------------------
# AC-2 (positive): Admin cancels any user's task — 204
# REQ-010: An admin may cancel any task regardless of ownership
#
# Given: task012user owns a queued task; admin is authenticated
# When:  admin sends POST /api/tasks/{id}/cancel for task012user's task
# Then:  HTTP 204; task status = "cancelled"
# ---------------------------------------------------------------------------
echo "AC-2: Admin cancels another user's task — 204"

# Submit a task as task012user (inject directly into DB so it is owned by task012user)
AC2_TASK_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id, input, created_at, updated_at)
   VALUES (
     gen_random_uuid(),
     '$TEST_PIPELINE_ID',
     '$USER_ID',
     'queued',
     'ac2-exec-id',
     '{}',
     NOW(),
     NOW()
   )
   RETURNING id;" 2>&1 | tr -d ' \n')

AC2_TASK_ID=$(echo "$AC2_TASK_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")
if [ -z "$AC2_TASK_ID" ]; then
  echo "  FATAL: AC-2 could not insert task012user task (result: $AC2_TASK_RESULT)"
  exit 1
fi
echo "  task012user task inserted: $AC2_TASK_ID"

# Admin cancels task012user's task
AC2_CANCEL_STATUS=$(curl -s -o /tmp/TASK012-ac2-cancel.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/$AC2_TASK_ID/cancel" \
  -H "Authorization: Bearer $ADMIN_TOKEN")

assert_status "AC-2a [REQ-010]: admin cancels another user's task — returns 204" 204 "$AC2_CANCEL_STATUS"

# Verify DB status
AC2_DB_STATUS=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id = '$AC2_TASK_ID';" 2>&1 | tr -d ' \n')

if [ "$AC2_DB_STATUS" = "cancelled" ]; then
  pass "AC-2b [REQ-010]: task status is 'cancelled' in database after admin cancel"
else
  fail "AC-2b [REQ-010]: task status is 'cancelled' in database after admin cancel" \
    "got status='$AC2_DB_STATUS'"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-3 (negative): Non-owner non-admin gets 403
# REQ-010: A cancel request from a non-owner non-admin is rejected
#
# Given: admin owns a queued task; task012user (non-admin) is authenticated
# When:  task012user sends POST /api/tasks/{id}/cancel for admin's task
# Then:  HTTP 403 Forbidden; task status is unchanged
# ---------------------------------------------------------------------------
echo "AC-3: Non-owner non-admin cancel returns 403"

# Insert a task owned by admin for this test
AC3_TASK_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id, input, created_at, updated_at)
   VALUES (
     gen_random_uuid(),
     '$TEST_PIPELINE_ID',
     '$ADMIN_USER_ID',
     'queued',
     'ac3-exec-id',
     '{}',
     NOW(),
     NOW()
   )
   RETURNING id;" 2>&1 | tr -d ' \n')

AC3_TASK_ID=$(echo "$AC3_TASK_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")
if [ -z "$AC3_TASK_ID" ]; then
  echo "  FATAL: AC-3 could not insert admin task (result: $AC3_TASK_RESULT)"
  exit 1
fi
echo "  admin-owned task inserted for AC-3: $AC3_TASK_ID"

# task012user attempts to cancel admin's task
AC3_CANCEL_STATUS=$(curl -s -o /tmp/TASK012-ac3-cancel.json -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/$AC3_TASK_ID/cancel" \
  -H "Authorization: Bearer $USER_TOKEN")
AC3_CANCEL_BODY=$(cat /tmp/TASK012-ac3-cancel.json 2>/dev/null || echo "")

assert_status "AC-3a [REQ-010]: non-owner non-admin cancel returns 403" 403 "$AC3_CANCEL_STATUS"

# Task status must remain unchanged (queued)
AC3_DB_STATUS=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id = '$AC3_TASK_ID';" 2>&1 | tr -d ' \n')

if [ "$AC3_DB_STATUS" = "queued" ]; then
  pass "AC-3b [REQ-010]: task status unchanged (still 'queued') after 403 cancel"
else
  fail "AC-3b [REQ-010]: task status unchanged (still 'queued') after 403 cancel" \
    "got status='$AC3_DB_STATUS'"
fi

# 403 response body must contain an error field (not disclose task data)
AC3_ERROR=$(echo "$AC3_CANCEL_BODY" | grep -o '"error":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -n "$AC3_ERROR" ]; then
  pass "AC-3c [REQ-010] [VERIFIER-ADDED]: 403 response is structured JSON with error field"
else
  fail "AC-3c [REQ-010] [VERIFIER-ADDED]: 403 response is structured JSON with error field" \
    "body: $AC3_CANCEL_BODY"
fi

# Unauthenticated cancel attempt must return 401
AC3_UNAUTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/$AC3_TASK_ID/cancel")
assert_status "AC-3d [REQ-010] [VERIFIER-ADDED]: unauthenticated cancel returns 401" 401 "$AC3_UNAUTH_STATUS"

echo ""

# ---------------------------------------------------------------------------
# AC-4 (negative): Cancelling terminal state tasks returns 409
# REQ-010: Cancellation of a completed/failed/cancelled task is rejected
#
# Given: tasks in each terminal state (completed, failed, cancelled)
# When:  owner sends POST /api/tasks/{id}/cancel
# Then:  HTTP 409 Conflict for each
# ---------------------------------------------------------------------------
echo "AC-4: Cancelling terminal-state tasks returns 409"

for terminal_status in "completed" "failed" "cancelled"; do
  TERM_TASK_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
    "INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id, input, created_at, updated_at)
     VALUES (
       gen_random_uuid(),
       '$TEST_PIPELINE_ID',
       '$ADMIN_USER_ID',
       '$terminal_status',
       'ac4-exec-${terminal_status}',
       '{}',
       NOW(),
       NOW()
     )
     RETURNING id;" 2>&1 | tr -d ' \n')

  TERM_TASK_ID=$(echo "$TERM_TASK_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")
  if [ -z "$TERM_TASK_ID" ]; then
    fail "AC-4 setup [REQ-010]: insert task with status '$terminal_status'" \
      "could not insert task (result: $TERM_TASK_RESULT)"
    continue
  fi

  TERM_CANCEL_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$API_URL/api/tasks/$TERM_TASK_ID/cancel" \
    -H "Authorization: Bearer $ADMIN_TOKEN")

  assert_status "AC-4 [REQ-010]: cancel '$terminal_status' task returns 409" 409 "$TERM_CANCEL_STATUS"
done

echo ""

# ---------------------------------------------------------------------------
# AC-5 (positive): Running task — cancel flag set in Redis, worker halts
# REQ-010: Cancellation of a running task causes the worker to stop
#
# Given: a task is manually set to "running" status in the DB
# When:  the task owner sends POST /api/tasks/{id}/cancel
# Then:  HTTP 204; cancel:{taskID} key exists in Redis with a TTL
#
# Note: End-to-end worker halting requires a task actively executing in the
# worker. The worker uses non-matching tags ("nomatchtag012") for most tests
# to prevent pickup. For the Redis flag test, we verify the flag is set in
# Redis within 60s (the cancel flag TTL). Verification that the worker does
# NOT write the Sink is covered by unit tests in worker/cancellation_test.go.
# ---------------------------------------------------------------------------
echo "AC-5: Running task cancel — Redis flag set"

# Insert a task directly in 'running' state (bypassing queue — the worker won't
# have picked this up, but we are testing the cancel handler's Redis flag logic)
AC5_TASK_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id, input, created_at, updated_at)
   VALUES (
     gen_random_uuid(),
     '$TEST_PIPELINE_ID',
     '$ADMIN_USER_ID',
     'running',
     'ac5-exec-id',
     '{}',
     NOW(),
     NOW()
   )
   RETURNING id;" 2>&1 | tr -d ' \n')

AC5_TASK_ID=$(echo "$AC5_TASK_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")
if [ -z "$AC5_TASK_ID" ]; then
  echo "  FATAL: AC-5 could not insert running task (result: $AC5_TASK_RESULT)"
  exit 1
fi
echo "  running task inserted for AC-5: $AC5_TASK_ID"

# Cancel the running task
AC5_CANCEL_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/$AC5_TASK_ID/cancel" \
  -H "Authorization: Bearer $ADMIN_TOKEN")

assert_status "AC-5a [REQ-010]: cancel running task returns 204" 204 "$AC5_CANCEL_STATUS"

# Verify the cancel flag exists in Redis
AC5_REDIS_KEY="cancel:$AC5_TASK_ID"
AC5_FLAG_VALUE=$(docker exec nexusflow-redis-1 redis-cli GET "$AC5_REDIS_KEY" 2>&1 | tr -d ' \n')

if [ "$AC5_FLAG_VALUE" = "1" ]; then
  pass "AC-5b [REQ-010]: cancel flag 'cancel:$AC5_TASK_ID' exists in Redis with value '1'"
else
  fail "AC-5b [REQ-010]: cancel flag 'cancel:$AC5_TASK_ID' exists in Redis" \
    "expected '1', got '$AC5_FLAG_VALUE' (key may be absent or expired)"
fi

# Verify the flag has a TTL (must be between 1 and 60 seconds)
AC5_FLAG_TTL=$(docker exec nexusflow-redis-1 redis-cli TTL "$AC5_REDIS_KEY" 2>&1 | tr -d ' \n')
if [ "$AC5_FLAG_TTL" -gt 0 ] && [ "$AC5_FLAG_TTL" -le 60 ] 2>/dev/null; then
  pass "AC-5c [REQ-010] [VERIFIER-ADDED]: cancel flag TTL is ${AC5_FLAG_TTL}s (within 1-60s range)"
else
  fail "AC-5c [REQ-010] [VERIFIER-ADDED]: cancel flag has TTL between 1 and 60 seconds" \
    "got TTL=$AC5_FLAG_TTL"
fi

# Verify DB status is also "cancelled" (DB and Redis updated together)
AC5_DB_STATUS=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id = '$AC5_TASK_ID';" 2>&1 | tr -d ' \n')
if [ "$AC5_DB_STATUS" = "cancelled" ]; then
  pass "AC-5d [REQ-010]: running task DB status is 'cancelled' after cancel"
else
  fail "AC-5d [REQ-010]: running task DB status is 'cancelled' after cancel" \
    "got status='$AC5_DB_STATUS'"
fi

# Negative: non-running task (queued) must NOT get a Redis cancel flag
AC5_NEG_TASK_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id, input, created_at, updated_at)
   VALUES (
     gen_random_uuid(),
     '$TEST_PIPELINE_ID',
     '$ADMIN_USER_ID',
     'queued',
     'ac5-neg-exec-id',
     '{}',
     NOW(),
     NOW()
   )
   RETURNING id;" 2>&1 | tr -d ' \n')

AC5_NEG_TASK_ID=$(echo "$AC5_NEG_TASK_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")
if [ -n "$AC5_NEG_TASK_ID" ]; then
  curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$API_URL/api/tasks/$AC5_NEG_TASK_ID/cancel" \
    -H "Authorization: Bearer $ADMIN_TOKEN" > /dev/null 2>&1

  AC5_NEG_FLAG=$(docker exec nexusflow-redis-1 redis-cli EXISTS "cancel:$AC5_NEG_TASK_ID" 2>&1 | tr -d ' \n')
  if [ "$AC5_NEG_FLAG" = "0" ]; then
    pass "AC-5e [REQ-010] [VERIFIER-ADDED]: queued task cancel does NOT set Redis cancel flag"
  else
    fail "AC-5e [REQ-010] [VERIFIER-ADDED]: queued task cancel does NOT set Redis cancel flag" \
      "Redis key 'cancel:$AC5_NEG_TASK_ID' exists (flag=$AC5_NEG_FLAG) — should not for non-running tasks"
  fi
fi

echo ""

# ---------------------------------------------------------------------------
# AC-6 (positive): Cancellation creates a task_state_log entry
# REQ-010: The state transition to "cancelled" is recorded in task_state_log
#
# Given: a cancellable task in "queued" state
# When:  the owner cancels it via POST /api/tasks/{id}/cancel
# Then:  a task_state_log row exists for that task with to_state = 'cancelled'
#
# IMPORTANT: This tests the real PostgreSQL path via PgTaskRepository.Cancel.
# The handoff note flags that PgTaskRepository.Cancel calls CancelTask directly
# without the transactional task_state_log write used by UpdateStatus.
# If CancelTask does not trigger a DB-level log entry, this test will FAIL.
# ---------------------------------------------------------------------------
echo "AC-6: Cancellation creates a task_state_log entry"

# Insert a queued task for AC-6
AC6_TASK_RESULT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id, input, created_at, updated_at)
   VALUES (
     gen_random_uuid(),
     '$TEST_PIPELINE_ID',
     '$ADMIN_USER_ID',
     'queued',
     'ac6-exec-id',
     '{}',
     NOW(),
     NOW()
   )
   RETURNING id;" 2>&1 | tr -d ' \n')

AC6_TASK_ID=$(echo "$AC6_TASK_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")
if [ -z "$AC6_TASK_ID" ]; then
  echo "  FATAL: AC-6 could not insert queued task (result: $AC6_TASK_RESULT)"
  exit 1
fi
echo "  queued task inserted for AC-6: $AC6_TASK_ID"

# Cancel the task
AC6_CANCEL_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/$AC6_TASK_ID/cancel" \
  -H "Authorization: Bearer $ADMIN_TOKEN")

assert_status "AC-6a [REQ-010]: cancel returns 204 (prerequisite for log check)" 204 "$AC6_CANCEL_STATUS"

# Query task_state_log for an entry with to_state = 'cancelled' for this task
AC6_LOG_COUNT=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT COUNT(*) FROM task_state_log WHERE task_id = '$AC6_TASK_ID' AND to_state = 'cancelled';" \
  2>&1 | tr -d ' \n')

if [ "$AC6_LOG_COUNT" -ge 1 ] 2>/dev/null; then
  pass "AC-6b [REQ-010]: task_state_log has $AC6_LOG_COUNT cancellation entry/entries for task $AC6_TASK_ID"
else
  fail "AC-6b [REQ-010]: task_state_log has a cancellation entry for task $AC6_TASK_ID" \
    "got count=$AC6_LOG_COUNT — PgTaskRepository.Cancel does not write task_state_log (deviation flagged in handoff)"
fi

# Verify the log entry content
AC6_LOG_ROW=$(docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -t -c \
  "SELECT from_state, to_state, reason FROM task_state_log WHERE task_id = '$AC6_TASK_ID' AND to_state = 'cancelled' LIMIT 1;" \
  2>&1 | tr -d '\n' | xargs)

if echo "$AC6_LOG_ROW" | grep -q "cancelled"; then
  pass "AC-6c [REQ-010] [VERIFIER-ADDED]: task_state_log entry contains 'cancelled' in to_state (row: $AC6_LOG_ROW)"
else
  # Only fail if the row count test above already passed
  if [ "$AC6_LOG_COUNT" -ge 1 ] 2>/dev/null; then
    fail "AC-6c [REQ-010] [VERIFIER-ADDED]: task_state_log to_state is 'cancelled'" \
      "row content: $AC6_LOG_ROW"
  else
    fail "AC-6c [REQ-010] [VERIFIER-ADDED]: no task_state_log entry to verify (AC-6b already failed)"
  fi
fi

echo ""

# ---------------------------------------------------------------------------
# [VERIFIER-ADDED] Error handling — invalid UUID and non-existent task
# REQ-010: Malformed IDs and missing tasks must be rejected cleanly
# ---------------------------------------------------------------------------
echo "[VERIFIER-ADDED] Error handling: 400 for invalid UUID, 404 for non-existent task"

BADID_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/not-a-uuid/cancel" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "[VERIFIER-ADDED] [REQ-010]: cancel with invalid UUID returns 400" 400 "$BADID_STATUS"

NOTFOUND_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$API_URL/api/tasks/00000000-dead-beef-cafe-000000000000/cancel" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
assert_status "[VERIFIER-ADDED] [REQ-010]: cancel non-existent task returns 404" 404 "$NOTFOUND_STATUS"

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
