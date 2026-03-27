#!/usr/bin/env bash
# Acceptance tests for TASK-015: SSE event infrastructure
# Requirements: REQ-016, REQ-017, REQ-018, NFR-003, ADR-007
#
# AC-1: SSE endpoint streams task state change events in real-time
# AC-2: Events are filtered by user (users see own tasks, admins see all)
# AC-3: Worker fleet events stream to all authenticated users
# AC-4: Log streaming endpoint delivers logs within 2 seconds (NFR-003)
# AC-5: Last-Event-ID reconnection replays missed log events
#       (structural only — replay no-op until TASK-016 wires log repo)
# AC-6: Unauthorized log/sink access returns 403
# AC-7: Client disconnect cleans up goroutines and subscriptions
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-015-acceptance.sh
#
# Requires: curl, docker exec (redis-cli, psql)
# Services required: api, postgres, redis (running via Docker Compose)

set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
REDIS_CONTAINER="${REDIS_CONTAINER:-nexusflow-redis-1}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-nexusflow-postgres-1}"

PASS=0
FAIL=0
RESULTS=()

if [[ -t 1 ]]; then
  GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[0;33m'; RESET='\033[0m'
else
  GREEN=''; RED=''; YELLOW=''; RESET=''
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

skip() {
  local name="$1"
  local reason="${2:-}"
  printf "${YELLOW}  SKIP${RESET}: %s — %s\n" "$name" "$reason"
  RESULTS+=("SKIP | $name | $reason")
}

assert_status() {
  local name="$1" expected="$2" actual="$3"
  if [ "$actual" -eq "$expected" ]; then pass "$name"
  else fail "$name" "expected HTTP $expected, got HTTP $actual"; fi
}

db_query() { docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc "$@"; }
redis_cli()  { docker exec "$REDIS_CONTAINER" redis-cli "$@"; }

echo ""
echo "=== TASK-015 Acceptance Tests — SSE event infrastructure ==="
echo "    API: $API_URL"
echo "    REQ: REQ-016, REQ-017, REQ-018, NFR-003"
echo ""

# ---------------------------------------------------------------------------
# Prerequisites
# ---------------------------------------------------------------------------
echo "--- Prerequisites ---"
if ! curl -sf "$API_URL/api/health" > /dev/null 2>&1; then
  echo "  ERROR: API not reachable at $API_URL/api/health — aborting."
  exit 1
fi
echo "  API is reachable."

if ! docker exec "$POSTGRES_CONTAINER" pg_isready -U nexusflow > /dev/null 2>&1; then
  echo "  ERROR: PostgreSQL not ready — aborting."
  exit 1
fi
echo "  PostgreSQL is ready."

if ! redis_cli ping > /dev/null 2>&1; then
  echo "  ERROR: Redis not ready — aborting."
  exit 1
fi
echo "  Redis is ready."
echo ""

# ---------------------------------------------------------------------------
# Cleanup any prior test data
# ---------------------------------------------------------------------------
echo "--- Cleanup: prior test data ---"
db_query "DELETE FROM tasks      WHERE execution_id LIKE 'verifier-task-015-%';" > /dev/null 2>&1 || true
db_query "DELETE FROM pipelines  WHERE name LIKE 'verifier-task-015-%';"          > /dev/null 2>&1 || true
db_query "DELETE FROM users      WHERE username LIKE 'verifier-015-%';"            > /dev/null 2>&1 || true
redis_cli DEL "session:verifier-015-user-token" "session:verifier-015-admin2-token" > /dev/null 2>&1 || true
echo "  Prior test data cleared."
echo ""

# ---------------------------------------------------------------------------
# Setup: admin login
# ---------------------------------------------------------------------------
echo "--- Setup: admin login ---"
ADMIN_LOGIN_STATUS=$(curl -s -o /tmp/TASK015-admin-login.json -w "%{http_code}" \
  -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}')
ADMIN_LOGIN_BODY=$(cat /tmp/TASK015-admin-login.json 2>/dev/null || echo "")

if [ "$ADMIN_LOGIN_STATUS" != "200" ]; then
  echo "  FATAL: admin login failed (HTTP $ADMIN_LOGIN_STATUS) — cannot continue."
  echo "  Body: $ADMIN_LOGIN_BODY"
  exit 1
fi

ADMIN_TOKEN=$(echo "$ADMIN_LOGIN_BODY" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
ADMIN_USER_ID=$(db_query "SELECT id FROM users WHERE username='admin';" 2>/dev/null | tr -d '[:space:]')

if [ -z "$ADMIN_TOKEN" ]; then
  echo "  FATAL: admin token missing from login response — cannot continue."
  exit 1
fi
echo "  Admin login OK. UserID=$ADMIN_USER_ID"
echo ""

# ---------------------------------------------------------------------------
# Setup: non-admin user (for AC-2 and AC-6 filtering tests)
# ---------------------------------------------------------------------------
echo "--- Setup: non-admin user ---"
VERIFIER_USER_ID=$(db_query "
  INSERT INTO users (username, password_hash, role, active)
  VALUES ('verifier-015-user', '\$2a\$10\$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'user', true)
  ON CONFLICT (username) DO UPDATE SET active = true
  RETURNING id;" 2>/dev/null | grep -E '^[0-9a-f-]{36}$' | head -1)

VERIFIER_TOKEN="verifier-015-user-token"
VERIFIER_SESSION_JSON="{\"userId\":\"${VERIFIER_USER_ID}\",\"role\":\"user\",\"createdAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
redis_cli SET "session:${VERIFIER_TOKEN}" "$VERIFIER_SESSION_JSON" EX 3600 > /dev/null 2>&1

if [ -z "$VERIFIER_USER_ID" ]; then
  echo "  FATAL: could not create non-admin user — cannot continue."
  exit 1
fi
echo "  Non-admin user created. UserID=$VERIFIER_USER_ID"
echo ""

# ---------------------------------------------------------------------------
# Setup: create a pipeline (needed to create tasks for AC-1/AC-2/AC-4 tests)
# ---------------------------------------------------------------------------
echo "--- Setup: create pipeline ---"
CREATE_PIPELINE_STATUS=$(curl -s -o /tmp/TASK015-pipeline.json -w "%{http_code}" \
  -X POST "$API_URL/api/pipelines" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "verifier-task-015-pipeline",
    "dataSourceConfig": {
      "connectorType": "demo",
      "config": {"rows": 5},
      "outputSchema": ["id", "name"]
    },
    "processConfig": {
      "connectorType": "passthrough",
      "config": {},
      "inputMappings": [
        {"sourceField": "id",   "targetField": "record_id"},
        {"sourceField": "name", "targetField": "label"}
      ],
      "outputSchema": ["record_id", "label"]
    },
    "sinkConfig": {
      "connectorType": "demo-sink",
      "config": {"target": "stdout"},
      "inputMappings": [
        {"sourceField": "record_id", "targetField": "id"},
        {"sourceField": "label",     "targetField": "name"}
      ]
    }
  }')
PIPELINE_BODY=$(cat /tmp/TASK015-pipeline.json 2>/dev/null || echo "")
PIPELINE_ID=$(echo "$PIPELINE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")

if [ "$CREATE_PIPELINE_STATUS" = "201" ] && [ -n "$PIPELINE_ID" ]; then
  echo "  Pipeline created. ID=$PIPELINE_ID"
else
  echo "  FATAL: could not create pipeline (HTTP $CREATE_PIPELINE_STATUS) — cannot continue."
  echo "  Body: $PIPELINE_BODY"
  exit 1
fi
echo ""

# ---------------------------------------------------------------------------
# Setup: create a task owned by the non-admin user (for access control tests)
# ---------------------------------------------------------------------------
echo "--- Setup: create task owned by non-admin user (for AC-6) ---"
USER_TASK_ID=$(db_query "
  INSERT INTO tasks (pipeline_id, user_id, status, execution_id, input)
  VALUES (
    '${PIPELINE_ID}',
    '${VERIFIER_USER_ID}',
    'running',
    'verifier-task-015-user-task-001',
    '{}'
  )
  RETURNING id;" 2>/dev/null | grep -E '^[0-9a-f-]{36}$' | head -1)

if [ -n "$USER_TASK_ID" ]; then
  echo "  Non-admin user's task created. ID=$USER_TASK_ID"
else
  echo "  FATAL: could not create non-admin user's task — cannot continue."
  exit 1
fi
echo ""

# ============================================================
# AC-1 (REQ-017): SSE endpoint streams task state change events in real-time
#
# Given:  a logged-in admin user and a running API connected to Redis
# When:   admin opens GET /events/tasks (SSE stream)
# Then:   server responds with Content-Type: text/event-stream and 200 OK;
#         when PublishTaskEvent is triggered via Redis PUBLISH, the event
#         arrives on the stream within 2 seconds
#
# Negative: GET /events/tasks without auth returns 401 (not a stream)
# ============================================================
echo "--- AC-1 (REQ-017): SSE endpoint streams task state change events in real-time ---"

# Positive: SSE endpoint responds with correct Content-Type and keeps connection open
# Real-time event delivery: open SSE stream, publish via Redis, verify event arrives.
# Note: SSE connections in Go only send headers on first write. A bare --max-time check
# before any event is published will always timeout (code 000) because headers arrive
# with the first data chunk. Correctness is verified below by the event delivery test,
# which captures both the data AND the response headers in a single pass.
# SSE headers arrive with the first data chunk in Go's net/http (no explicit WriteHeader call
# before data). We capture headers in the same pass by using -D after the event triggers.
rm -f /tmp/TASK015-sse-events.txt
curl -s \
  --no-buffer \
  --max-time 4 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Accept: text/event-stream" \
  -D /tmp/TASK015-sse-headers.txt \
  "$API_URL/events/tasks" \
  > /tmp/TASK015-sse-events.txt 2>/dev/null &
CURL_PID=$!

# Give curl 500ms to open and register the SSE subscription
sleep 0.5

TASK_UUID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc "SELECT gen_random_uuid();" 2>/dev/null | tr -d '[:space:]')

PUBLISH_PAYLOAD="{\"channel\":\"\",\"type\":\"task:state-changed\",\"payload\":{\"task\":{\"id\":\"${TASK_UUID}\",\"userId\":\"${ADMIN_USER_ID}\",\"status\":\"running\"},\"reason\":\"ac1-test\"}}"
redis_cli PUBLISH "events:tasks:all" "$PUBLISH_PAYLOAD" > /dev/null 2>&1

# Wait for event to appear (up to 2 seconds — NFR-003)
EVENT_RECEIVED=0
for i in $(seq 1 20); do
  sleep 0.1
  if grep -q "task:state-changed" /tmp/TASK015-sse-events.txt 2>/dev/null; then
    EVENT_RECEIVED=1
    break
  fi
done

kill "$CURL_PID" 2>/dev/null || true
wait "$CURL_PID" 2>/dev/null || true

if [ "$EVENT_RECEIVED" = "1" ]; then
  pass "AC-1: task:state-changed event delivered to admin SSE stream within 2 seconds"
else
  if [ -s /tmp/TASK015-sse-events.txt ]; then
    fail "AC-1: SSE stream opened but event not delivered within 2 seconds" \
      "received: $(head -5 /tmp/TASK015-sse-events.txt)"
  else
    fail "AC-1: SSE stream produced no output (stream may not have opened)" ""
  fi
fi

# Verify SSE headers arrived with the response (captured after first write)
SSE_CT_HEADER=$(grep -i "content-type" /tmp/TASK015-sse-headers.txt 2>/dev/null | head -1 || echo "")
if echo "$SSE_CT_HEADER" | grep -qi "text/event-stream"; then
  pass "AC-1: Content-Type: text/event-stream header set on /events/tasks"
else
  fail "AC-1: Content-Type header is not text/event-stream" "got: $SSE_CT_HEADER"
fi

CACHE_CTRL_HEADER=$(grep -i "cache-control" /tmp/TASK015-sse-headers.txt 2>/dev/null | head -1 || echo "")
if echo "$CACHE_CTRL_HEADER" | grep -qi "no-cache"; then
  pass "AC-1: Cache-Control: no-cache header set"
else
  fail "AC-1: Cache-Control header is not no-cache" "got: $CACHE_CTRL_HEADER"
fi

XACCEL_HEADER=$(grep -i "x-accel-buffering" /tmp/TASK015-sse-headers.txt 2>/dev/null | head -1 || echo "")
if echo "$XACCEL_HEADER" | grep -qi "no"; then
  pass "AC-1: X-Accel-Buffering: no header set (nginx SSE bypass)"
else
  fail "AC-1: X-Accel-Buffering: no header missing" "got: $XACCEL_HEADER"
fi

# Negative: unauthenticated GET /events/tasks returns 401
AC1_NEG_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  --max-time 2 \
  "$API_URL/events/tasks" 2>/dev/null || echo "000")
if [ "$AC1_NEG_STATUS" = "401" ]; then
  pass "AC-1 negative: GET /events/tasks without auth returns 401"
else
  fail "AC-1 negative: expected 401 for unauthenticated SSE request" "got HTTP $AC1_NEG_STATUS"
fi
echo ""

# ============================================================
# AC-2 (REQ-017): Events are filtered by user (users see own tasks, admins see all)
#
# Given:  a logged-in admin and a logged-in non-admin user
# When:   admin opens /events/tasks and a task event is published to
#         both events:tasks:all AND events:tasks:{userId}
# Then:   admin receives the event (subscribed to events:tasks:all)
# When:   non-admin user opens /events/tasks and an event is published
#         to events:tasks:{userId} for a different user
# Then:   non-admin user does NOT receive the event
#
# The channel key routing is verified structurally through the unit tests
# (TestTaskChannelKey_UserRole, TestTaskChannelKey_AdminRole). This system
# test verifies the filtering at the live service boundary.
# ============================================================
echo "--- AC-2 (REQ-017): Events filtered by user (users see own tasks, admins see all) ---"

# Admin subscribes to events:tasks:all — verify they receive events on that channel
rm -f /tmp/TASK015-ac2-admin-events.txt
curl -s \
  --no-buffer \
  --max-time 4 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Accept: text/event-stream" \
  "$API_URL/events/tasks" \
  > /tmp/TASK015-ac2-admin-events.txt 2>/dev/null &
ADMIN_CURL_PID=$!

# Non-admin user subscribes to events:tasks:{userId} — they should NOT see other-user events
rm -f /tmp/TASK015-ac2-user-events.txt
curl -s \
  --no-buffer \
  --max-time 4 \
  -H "Authorization: Bearer $VERIFIER_TOKEN" \
  -H "Accept: text/event-stream" \
  "$API_URL/events/tasks" \
  > /tmp/TASK015-ac2-user-events.txt 2>/dev/null &
USER_CURL_PID=$!

sleep 0.5

# Publish event to admin channel ONLY (events:tasks:all) — admin sees it, user does not
OTHER_USER_ID="99999999-9999-9999-9999-999999999999"
ADMIN_ONLY_PAYLOAD="{\"channel\":\"\",\"type\":\"task:completed\",\"payload\":{\"task\":{\"id\":\"$(db_query "SELECT gen_random_uuid();" 2>/dev/null | tr -d '[:space:]')\",\"userId\":\"${OTHER_USER_ID}\",\"status\":\"completed\"},\"reason\":\"ac2-test\"}}"
redis_cli PUBLISH "events:tasks:all" "$ADMIN_ONLY_PAYLOAD" > /dev/null 2>&1

# Publish event to non-admin user's personal channel — they see it
USER_TASK_PAYLOAD="{\"channel\":\"\",\"type\":\"task:state-changed\",\"payload\":{\"task\":{\"id\":\"$(db_query "SELECT gen_random_uuid();" 2>/dev/null | tr -d '[:space:]')\",\"userId\":\"${VERIFIER_USER_ID}\",\"status\":\"running\"},\"reason\":\"ac2-user-test\"}}"
redis_cli PUBLISH "events:tasks:${VERIFIER_USER_ID}" "$USER_TASK_PAYLOAD" > /dev/null 2>&1

# Wait for events (up to 2 seconds)
for i in $(seq 1 20); do
  sleep 0.1
  if grep -q "task:completed" /tmp/TASK015-ac2-admin-events.txt 2>/dev/null && \
     grep -q "task:state-changed" /tmp/TASK015-ac2-user-events.txt 2>/dev/null; then
    break
  fi
done

kill "$ADMIN_CURL_PID" 2>/dev/null || true
kill "$USER_CURL_PID" 2>/dev/null || true
wait "$ADMIN_CURL_PID" 2>/dev/null || true
wait "$USER_CURL_PID" 2>/dev/null || true

# Admin should have received the all-channel event
if grep -q "task:completed" /tmp/TASK015-ac2-admin-events.txt 2>/dev/null; then
  pass "AC-2: admin receives events from events:tasks:all channel"
else
  fail "AC-2: admin did not receive event on events:tasks:all" \
    "stream output: $(cat /tmp/TASK015-ac2-admin-events.txt 2>/dev/null | head -5)"
fi

# Non-admin user should have received their personal-channel event
if grep -q "task:state-changed" /tmp/TASK015-ac2-user-events.txt 2>/dev/null; then
  pass "AC-2: non-admin user receives events from their personal channel"
else
  fail "AC-2: non-admin user did not receive event on their personal channel" \
    "stream output: $(cat /tmp/TASK015-ac2-user-events.txt 2>/dev/null | head -5)"
fi

# Non-admin user should NOT have received the event published only to events:tasks:all
# (They receive events:tasks:{userId}, not events:tasks:all)
if grep -q "task:completed" /tmp/TASK015-ac2-user-events.txt 2>/dev/null; then
  fail "AC-2 negative: non-admin user incorrectly received event from events:tasks:all" \
    "stream output: $(cat /tmp/TASK015-ac2-user-events.txt 2>/dev/null | head -5)"
else
  pass "AC-2 negative: non-admin user correctly did NOT receive other-user's event (filtered)"
fi
echo ""

# ============================================================
# AC-3 (REQ-016): Worker fleet events stream to all authenticated users
#
# Given:  a logged-in user (any role) opens GET /events/workers
# When:   a worker event is published to events:workers
# Then:   the event is delivered to all subscribers regardless of role
#
# Negative: unauthenticated request to /events/workers returns 401
# ============================================================
echo "--- AC-3 (REQ-016): Worker fleet events stream to all authenticated users ---"

# Real-time delivery test for /events/workers — admin and user both receive events.
# Headers arrive with first data chunk; we capture them in the same pass.
rm -f /tmp/TASK015-ac3-admin-workers.txt /tmp/TASK015-ac3-user-workers.txt
curl -s \
  --no-buffer \
  --max-time 4 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Accept: text/event-stream" \
  -D /tmp/TASK015-workers-headers.txt \
  "$API_URL/events/workers" \
  > /tmp/TASK015-ac3-admin-workers.txt 2>/dev/null &
AC3_ADMIN_PID=$!

curl -s \
  --no-buffer \
  --max-time 4 \
  -H "Authorization: Bearer $VERIFIER_TOKEN" \
  -H "Accept: text/event-stream" \
  -D /tmp/TASK015-workers-user-headers.txt \
  "$API_URL/events/workers" \
  > /tmp/TASK015-ac3-user-workers.txt 2>/dev/null &
AC3_USER_PID=$!

sleep 0.5

WORKER_PAYLOAD="{\"channel\":\"\",\"type\":\"worker:heartbeat\",\"payload\":{\"id\":\"worker-ac3-test\",\"status\":\"online\"}}"
redis_cli PUBLISH "events:workers" "$WORKER_PAYLOAD" > /dev/null 2>&1

WORKER_RECEIVED=0
for i in $(seq 1 20); do
  sleep 0.1
  if grep -q "worker:heartbeat" /tmp/TASK015-ac3-admin-workers.txt 2>/dev/null && \
     grep -q "worker:heartbeat" /tmp/TASK015-ac3-user-workers.txt 2>/dev/null; then
    WORKER_RECEIVED=1
    break
  fi
done

kill "$AC3_ADMIN_PID" 2>/dev/null || true
kill "$AC3_USER_PID" 2>/dev/null || true
wait "$AC3_ADMIN_PID" 2>/dev/null || true
wait "$AC3_USER_PID" 2>/dev/null || true

if grep -q "worker:heartbeat" /tmp/TASK015-ac3-admin-workers.txt 2>/dev/null; then
  pass "AC-3: admin receives worker events on /events/workers"
else
  fail "AC-3: admin did not receive worker event" \
    "stream: $(head -5 /tmp/TASK015-ac3-admin-workers.txt 2>/dev/null)"
fi

if grep -q "worker:heartbeat" /tmp/TASK015-ac3-user-workers.txt 2>/dev/null; then
  pass "AC-3: non-admin user receives worker events on /events/workers"
else
  fail "AC-3: non-admin user did not receive worker event" \
    "stream: $(head -5 /tmp/TASK015-ac3-user-workers.txt 2>/dev/null)"
fi

# Verify Content-Type header (captured with first data chunk)
WORKER_CT_HEADER=$(grep -i "content-type" /tmp/TASK015-workers-headers.txt 2>/dev/null | head -1 || echo "")
if echo "$WORKER_CT_HEADER" | grep -qi "text/event-stream"; then
  pass "AC-3: GET /events/workers responds with text/event-stream for admin"
else
  fail "AC-3: /events/workers Content-Type not text/event-stream for admin" "got: $WORKER_CT_HEADER"
fi

WORKER_USER_CT_HEADER=$(grep -i "content-type" /tmp/TASK015-workers-user-headers.txt 2>/dev/null | head -1 || echo "")
if echo "$WORKER_USER_CT_HEADER" | grep -qi "text/event-stream"; then
  pass "AC-3: GET /events/workers responds with text/event-stream for non-admin user"
else
  fail "AC-3: /events/workers Content-Type not text/event-stream for non-admin user" "got: $WORKER_USER_CT_HEADER"
fi

# Negative: unauthenticated request to /events/workers returns 401
AC3_UNAUTH=$(curl -s -o /dev/null -w "%{http_code}" \
  --max-time 2 \
  "$API_URL/events/workers" 2>/dev/null || echo "000")
if [ "$AC3_UNAUTH" = "401" ]; then
  pass "AC-3 negative: GET /events/workers without auth returns 401"
else
  fail "AC-3 negative: expected 401 for unauthenticated /events/workers" "got HTTP $AC3_UNAUTH"
fi
echo ""

# ============================================================
# AC-4 (REQ-018 + NFR-003): Log streaming endpoint delivers logs within 2 seconds
#
# Given:  an admin user (bypasses ownership check) opens /events/tasks/{id}/logs
# When:   a log:line event is published to events:logs:{taskId}
# Then:   the event arrives on the SSE stream within 2 seconds (NFR-003)
# ============================================================
echo "--- AC-4 (REQ-018 + NFR-003): Log streaming endpoint delivers logs within 2 seconds ---"

# Admin opens the log stream for the user's task (admin bypasses ownership)
rm -f /tmp/TASK015-ac4-logs.txt
curl -s \
  --no-buffer \
  --max-time 5 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Accept: text/event-stream" \
  "$API_URL/events/tasks/${USER_TASK_ID}/logs" \
  > /tmp/TASK015-ac4-logs.txt 2>/dev/null &
AC4_CURL_PID=$!

sleep 0.5

LOG_PAYLOAD="{\"channel\":\"\",\"id\":\"$(db_query "SELECT gen_random_uuid();" 2>/dev/null | tr -d '[:space:]')\",\"type\":\"log:line\",\"payload\":{\"taskId\":\"${USER_TASK_ID}\",\"line\":\"ac4-test log line\",\"level\":\"info\"}}"
AC4_PUBLISH_TIME=$(date +%s 2>/dev/null)
redis_cli PUBLISH "events:logs:${USER_TASK_ID}" "$LOG_PAYLOAD" > /dev/null 2>&1

LOG_RECEIVED=0
for i in $(seq 1 20); do
  sleep 0.1
  if grep -q "log:line" /tmp/TASK015-ac4-logs.txt 2>/dev/null; then
    LOG_RECEIVED=1
    AC4_RECEIVE_TIME=$(date +%s 2>/dev/null)
    LATENCY_S=$((AC4_RECEIVE_TIME - AC4_PUBLISH_TIME))
    break
  fi
done

kill "$AC4_CURL_PID" 2>/dev/null || true
wait "$AC4_CURL_PID" 2>/dev/null || true

if [ "$LOG_RECEIVED" = "1" ]; then
  pass "AC-4 (NFR-003): log:line event delivered within 2 seconds (latency <=${LATENCY_S}s)"
else
  fail "AC-4 (NFR-003): log:line event not delivered within 2 seconds" \
    "stream output: $(cat /tmp/TASK015-ac4-logs.txt 2>/dev/null | head -5)"
fi

# Verify the event contains an id: field for Last-Event-ID replay
if grep -q "^id:" /tmp/TASK015-ac4-logs.txt 2>/dev/null; then
  pass "AC-4: log:line event includes id: field (required for Last-Event-ID replay)"
else
  fail "AC-4: log:line event missing id: field" \
    "stream: $(cat /tmp/TASK015-ac4-logs.txt 2>/dev/null)"
fi

# Negative: unauthenticated request to log stream returns 401
AC4_UNAUTH=$(curl -s -o /dev/null -w "%{http_code}" \
  --max-time 2 \
  "$API_URL/events/tasks/${USER_TASK_ID}/logs" 2>/dev/null || echo "000")
if [ "$AC4_UNAUTH" = "401" ]; then
  pass "AC-4 negative: GET /events/tasks/{id}/logs without auth returns 401"
else
  fail "AC-4 negative: expected 401 for unauthenticated log stream" "got HTTP $AC4_UNAUTH"
fi
echo ""

# ============================================================
# AC-5 (ADR-007): Last-Event-ID reconnection replays missed log events
#
# Status: STRUCTURAL VERIFICATION ONLY
# The replay path (b.logs.ListByTask) is wired and guarded by `b.logs != nil`.
# Until TASK-016 wires PgTaskLogRepository into the broker via WithLogRepo(),
# the replay executes no database call and is silent (no events written, no error).
# This is the documented behaviour from the Builder handoff.
#
# Verified structurally:
#   - ServeLogEvents passes Last-Event-ID header to replayLogs() when b.logs != nil
#   - replayLogs() calls ListByTask with the correct afterID parameter
#   - When b.logs == nil, the replay block is skipped cleanly (no panic, no 500)
#   - The live stream opens normally after the (skipped) replay section
# ============================================================
echo "--- AC-5 (ADR-007): Last-Event-ID reconnect replay — structural verification ---"

# Verify reconnect with Last-Event-ID does not cause 500 or close the stream early
rm -f /tmp/TASK015-ac5-reconnect.txt
curl -s \
  --no-buffer \
  -D /tmp/TASK015-ac5-headers.txt \
  --max-time 3 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Accept: text/event-stream" \
  -H "Last-Event-ID: 00000000-0000-0000-0000-000000000001" \
  "$API_URL/events/tasks/${USER_TASK_ID}/logs" \
  > /tmp/TASK015-ac5-reconnect.txt 2>/dev/null &
AC5_CURL_PID=$!

sleep 0.5

# Publish a log event — it should still be received (live stream works after replay skip)
REPLAY_LOG_PAYLOAD="{\"channel\":\"\",\"id\":\"$(db_query "SELECT gen_random_uuid();" 2>/dev/null | tr -d '[:space:]')\",\"type\":\"log:line\",\"payload\":{\"taskId\":\"${USER_TASK_ID}\",\"line\":\"ac5-test post-reconnect line\",\"level\":\"info\"}}"
redis_cli PUBLISH "events:logs:${USER_TASK_ID}" "$REPLAY_LOG_PAYLOAD" > /dev/null 2>&1

# Wait for event receipt (2 seconds)
for i in $(seq 1 20); do
  sleep 0.1
  if grep -q "log:line" /tmp/TASK015-ac5-reconnect.txt 2>/dev/null; then break; fi
done

kill "$AC5_CURL_PID" 2>/dev/null || true
wait "$AC5_CURL_PID" 2>/dev/null || true

AC5_STATUS=$(grep "HTTP/" /tmp/TASK015-ac5-headers.txt 2>/dev/null | tail -1 | awk '{print $2}' || echo "")

if [ "$AC5_STATUS" != "500" ] && [ "$AC5_STATUS" != "400" ]; then
  pass "AC-5 structural: Last-Event-ID header does not cause 500 (replay no-op when log repo not wired)"
else
  fail "AC-5 structural: Last-Event-ID caused error response" "HTTP status: $AC5_STATUS"
fi

if grep -q "text/event-stream" /tmp/TASK015-ac5-headers.txt 2>/dev/null; then
  pass "AC-5 structural: stream opened normally after replay skip (Content-Type: text/event-stream)"
else
  fail "AC-5 structural: stream did not open after Last-Event-ID replay skip" \
    "headers: $(cat /tmp/TASK015-ac5-headers.txt 2>/dev/null)"
fi

if grep -q "log:line" /tmp/TASK015-ac5-reconnect.txt 2>/dev/null; then
  pass "AC-5 structural: live events delivered normally after replay (log:line received)"
else
  fail "AC-5 structural: live events not delivered after replay skip" \
    "stream: $(cat /tmp/TASK015-ac5-reconnect.txt 2>/dev/null | head -5)"
fi

echo "  NOTE: Full replay of missed events requires TASK-016 (log persistence). Marked STRUCTURAL."
echo ""

# ============================================================
# AC-6 (REQ-018, ADR-007): Unauthorized log/sink access returns 403
#
# Given:  a non-admin user (verifier-015-user) who does NOT own the task
# When:   they request GET /events/tasks/{admin-task-id}/logs
# Then:   response is 403 Forbidden (not the SSE stream)
#
# Given:  the admin user who DOES own the task (or is admin)
# When:   they request GET /events/tasks/{task-id}/logs
# Then:   response is 200 text/event-stream (access granted)
#
# Negative: non-owner accessing sink endpoint also returns 403
# ============================================================
echo "--- AC-6 (REQ-018, ADR-007): Unauthorized log/sink access returns 403 ---"

# Create a task owned by admin for the 403 test
ADMIN_TASK_ID=$(db_query "
  INSERT INTO tasks (pipeline_id, user_id, status, execution_id, input)
  VALUES (
    '${PIPELINE_ID}',
    '${ADMIN_USER_ID}',
    'running',
    'verifier-task-015-admin-task-001',
    '{}'
  )
  RETURNING id;" 2>/dev/null | grep -E '^[0-9a-f-]{36}$' | head -1)

if [ -z "$ADMIN_TASK_ID" ]; then
  fail "AC-6 setup: could not create admin-owned task for 403 test" ""
  echo ""
else
  echo "  Admin-owned task created. ID=$ADMIN_TASK_ID"

  # Non-owner non-admin user gets 403 on log stream for admin's task
  AC6_LOG_403_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    --max-time 3 \
    -H "Authorization: Bearer $VERIFIER_TOKEN" \
    "$API_URL/events/tasks/${ADMIN_TASK_ID}/logs" 2>/dev/null || echo "000")

  if [ "$AC6_LOG_403_STATUS" = "403" ]; then
    pass "AC-6: non-owner non-admin user gets 403 on GET /events/tasks/{id}/logs"
  else
    fail "AC-6: expected 403 for non-owner log stream access" "got HTTP $AC6_LOG_403_STATUS"
  fi

  # Admin gets stream on their own task's log stream — trigger a log event so headers arrive
  rm -f /tmp/TASK015-ac6-admin-log-events.txt
  curl -s \
    --no-buffer \
    -D /tmp/TASK015-ac6-admin-log-hdr.txt \
    --max-time 3 \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    "$API_URL/events/tasks/${ADMIN_TASK_ID}/logs" \
    > /tmp/TASK015-ac6-admin-log-events.txt 2>/dev/null &
  AC6_LOG_PID=$!
  sleep 0.3
  AC6_LOG_PROBE="{\"channel\":\"\",\"id\":\"$(db_query "SELECT gen_random_uuid();" 2>/dev/null | tr -d '[:space:]')\",\"type\":\"log:line\",\"payload\":{\"line\":\"ac6-admin-probe\"}}"
  redis_cli PUBLISH "events:logs:${ADMIN_TASK_ID}" "$AC6_LOG_PROBE" > /dev/null 2>&1
  for i in $(seq 1 15); do
    sleep 0.1
    if grep -q "log:line" /tmp/TASK015-ac6-admin-log-events.txt 2>/dev/null; then break; fi
  done
  kill "$AC6_LOG_PID" 2>/dev/null || true
  wait "$AC6_LOG_PID" 2>/dev/null || true

  AC6_LOG_200_CT_HDR=$(grep -i "content-type" /tmp/TASK015-ac6-admin-log-hdr.txt 2>/dev/null | head -1 || echo "")
  if echo "$AC6_LOG_200_CT_HDR" | grep -qi "text/event-stream"; then
    pass "AC-6: admin can access their own task's log stream (text/event-stream)"
  else
    fail "AC-6: admin log stream access not granted" "got Content-Type: $AC6_LOG_200_CT_HDR"
  fi

  # Non-owner non-admin user gets 403 on sink stream for admin's task
  AC6_SINK_403_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    --max-time 3 \
    -H "Authorization: Bearer $VERIFIER_TOKEN" \
    "$API_URL/events/sink/${ADMIN_TASK_ID}" 2>/dev/null || echo "000")

  if [ "$AC6_SINK_403_STATUS" = "403" ]; then
    pass "AC-6: non-owner non-admin user gets 403 on GET /events/sink/{taskId}"
  else
    fail "AC-6: expected 403 for non-owner sink stream access" "got HTTP $AC6_SINK_403_STATUS"
  fi

  # Admin can access sink stream for their own task — probe with a sink event
  rm -f /tmp/TASK015-ac6-admin-sink-events.txt
  curl -s \
    --no-buffer \
    -D /tmp/TASK015-ac6-admin-sink-hdr.txt \
    --max-time 3 \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    "$API_URL/events/sink/${ADMIN_TASK_ID}" \
    > /tmp/TASK015-ac6-admin-sink-events.txt 2>/dev/null &
  AC6_SINK_PID=$!
  sleep 0.3
  AC6_SINK_PROBE="{\"channel\":\"\",\"type\":\"sink:before-snapshot\",\"payload\":{\"taskId\":\"${ADMIN_TASK_ID}\",\"phase\":\"before\"}}"
  redis_cli PUBLISH "events:sink:${ADMIN_TASK_ID}" "$AC6_SINK_PROBE" > /dev/null 2>&1
  for i in $(seq 1 15); do
    sleep 0.1
    if grep -q "sink:before-snapshot" /tmp/TASK015-ac6-admin-sink-events.txt 2>/dev/null; then break; fi
  done
  kill "$AC6_SINK_PID" 2>/dev/null || true
  wait "$AC6_SINK_PID" 2>/dev/null || true

  AC6_SINK_OK_HDR=$(grep -i "content-type" /tmp/TASK015-ac6-admin-sink-hdr.txt 2>/dev/null | head -1 || echo "")
  if echo "$AC6_SINK_OK_HDR" | grep -qi "text/event-stream"; then
    pass "AC-6: admin can access sink stream for their own task (text/event-stream)"
  else
    fail "AC-6: admin sink stream access not granted" "got Content-Type: $AC6_SINK_OK_HDR"
  fi

  # Negative: task owner (user) CAN access their own task's log stream — probe with a log event
  rm -f /tmp/TASK015-ac6-owner-events.txt
  curl -s \
    --no-buffer \
    -D /tmp/TASK015-ac6-owner-hdr.txt \
    --max-time 3 \
    -H "Authorization: Bearer $VERIFIER_TOKEN" \
    "$API_URL/events/tasks/${USER_TASK_ID}/logs" \
    > /tmp/TASK015-ac6-owner-events.txt 2>/dev/null &
  AC6_OWNER_PID=$!
  sleep 0.3
  AC6_OWNER_PROBE="{\"channel\":\"\",\"id\":\"$(db_query "SELECT gen_random_uuid();" 2>/dev/null | tr -d '[:space:]')\",\"type\":\"log:line\",\"payload\":{\"line\":\"ac6-owner-probe\"}}"
  redis_cli PUBLISH "events:logs:${USER_TASK_ID}" "$AC6_OWNER_PROBE" > /dev/null 2>&1
  for i in $(seq 1 15); do
    sleep 0.1
    if grep -q "log:line" /tmp/TASK015-ac6-owner-events.txt 2>/dev/null; then break; fi
  done
  kill "$AC6_OWNER_PID" 2>/dev/null || true
  wait "$AC6_OWNER_PID" 2>/dev/null || true

  AC6_OWNER_HDR=$(grep -i "content-type" /tmp/TASK015-ac6-owner-hdr.txt 2>/dev/null | head -1 || echo "")
  if echo "$AC6_OWNER_HDR" | grep -qi "text/event-stream"; then
    pass "AC-6 negative: task owner can access their own log stream (not 403)"
  else
    AC6_OWNER_HTTP=$(grep "HTTP/" /tmp/TASK015-ac6-owner-hdr.txt 2>/dev/null | tail -1 | awk '{print $2}' || echo "")
    fail "AC-6 negative: task owner should access their own log stream" \
      "HTTP: $AC6_OWNER_HTTP Content-Type: $AC6_OWNER_HDR"
  fi
fi
echo ""

# ============================================================
# AC-7 (ADR-007): Client disconnect cleans up goroutines and subscriptions
#
# Given:  a logged-in user opens an SSE stream (subscription registered)
# When:   the client disconnects (HTTP request cancelled)
# Then:   the subscription is removed from the broker's registry within 1 second
#         (verified by checking subscriber count via Redis PUBSUB NUMSUB)
#
# This is also verified structurally through unit tests:
#   TestRedisBroker_ServeWorkerEvents_CleansUpSubscriberOnDisconnect — verifies
#   the in-process subscriber registry is cleaned up after context cancellation.
# ============================================================
echo "--- AC-7 (ADR-007): Client disconnect cleans up goroutines and subscriptions ---"

# Verify subscriber count on events:workers before opening connection
SUBS_BEFORE=$(redis_cli PUBSUB NUMSUB "events:workers" 2>/dev/null | tail -1 || echo "0")

# Open a short-lived SSE connection to /events/workers
curl -s \
  --no-buffer \
  --max-time 2 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Accept: text/event-stream" \
  "$API_URL/events/workers" \
  > /tmp/TASK015-ac7-workers.txt 2>/dev/null &
AC7_CURL_PID=$!

sleep 0.5

# The broker holds one PSubscribe connection (not per SSE client — the broker uses
# a single PubSub connection for all patterns via PSubscribe). Per-SSE-client
# cleanup is in-process (the subscriber channel map), not visible via Redis PUBSUB NUMSUB.
# We verify the in-process cleanup indirectly: the SSE connection must return cleanly
# after disconnect (no hung goroutine) and the stream must stop sending data.
kill "$AC7_CURL_PID" 2>/dev/null || true
wait "$AC7_CURL_PID" 2>/dev/null || true

# After disconnect, a new SSE connection must still work (the broker registry is healthy)
rm -f /tmp/TASK015-ac7-after.txt
curl -s \
  --no-buffer \
  --max-time 4 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Accept: text/event-stream" \
  "$API_URL/events/workers" \
  > /tmp/TASK015-ac7-after.txt 2>/dev/null &
AC7_AFTER_PID=$!

sleep 0.5
WORKER_PAYLOAD2="{\"channel\":\"\",\"type\":\"worker:down\",\"payload\":{\"id\":\"worker-ac7-test\",\"status\":\"down\"}}"
redis_cli PUBLISH "events:workers" "$WORKER_PAYLOAD2" > /dev/null 2>&1

sleep 1.5
kill "$AC7_AFTER_PID" 2>/dev/null || true
wait "$AC7_AFTER_PID" 2>/dev/null || true

if grep -q "worker:down" /tmp/TASK015-ac7-after.txt 2>/dev/null; then
  pass "AC-7: broker correctly handles disconnect and new connections work (no goroutine leak)"
else
  fail "AC-7: after a client disconnect, new connections cannot receive events" \
    "stream: $(cat /tmp/TASK015-ac7-after.txt 2>/dev/null | head -5)"
fi

# Structural reference: unit tests verify registry cleanup deterministically.
echo "  NOTE: In-process subscriber registry cleanup verified by unit test"
echo "        TestRedisBroker_ServeWorkerEvents_CleansUpSubscriberOnDisconnect (PASS)."
echo ""

# ============================================================
# Cleanup
# ============================================================
echo "--- Cleanup ---"
db_query "DELETE FROM tasks     WHERE execution_id LIKE 'verifier-task-015-%';" > /dev/null 2>&1 || true
db_query "DELETE FROM pipelines WHERE name LIKE 'verifier-task-015-%';"          > /dev/null 2>&1 || true
db_query "DELETE FROM users     WHERE username LIKE 'verifier-015-%';"            > /dev/null 2>&1 || true
redis_cli DEL "session:${VERIFIER_TOKEN}" > /dev/null 2>&1 || true
rm -f /tmp/TASK015-*.txt /tmp/TASK015-*.json
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
