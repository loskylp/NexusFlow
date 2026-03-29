#!/usr/bin/env bash
# TASK-009 Acceptance Tests — Monitor service: heartbeat checking and failover
#
# REQ-013: Monitor detects downed workers and reassigns tasks automatically.
# REQ-004: Workers self-register and emit heartbeats; Monitor detects expiry.
# REQ-011: Task retry counter is incremented on each failover reassignment.
# ADR-002: Heartbeat timeout=15s, pending scan interval=10s, worst-case 25s.
#
# Acceptance criteria under test:
#   AC-1: Worker that stops heartbeating for >15s is marked "down" in PostgreSQL.
#   AC-2: Worker down event published to events:workers via Redis Pub/Sub.
#   AC-3: Task pending on a downed worker is reclaimed via XCLAIM and re-queued.
#   AC-4: Task retry counter is incremented on each failover reassignment.
#   AC-5: Reclaimed task is picked up by a healthy matching worker.
#   AC-6: Task with exhausted retries (default 3) is moved to queue:dead-letter
#         and status set to "failed".
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-009-acceptance.sh
#
# Requires:
#   - curl, docker exec (for psql/redis-cli access)
#   - docker compose stack running: api, worker, monitor, postgres, redis
#   - Worker and monitor built from TASK-009 code (go build ./...)
#
# Services required: all services from docker-compose.yml
#
# Strategy for in-flight task capture (AC-3/4/5):
#   1. Subscribe to Redis events:workers channel in background.
#   2. Submit a task; wait until the worker picks it up (status = running).
#   3. Pause the worker container — this freezes the process without killing it,
#      preserving the task in the worker's XREADGROUP pending list.
#   4. Stop heartbeating: the worker is paused so its heartbeat timer stops.
#   5. Wait 30 seconds (> 15s timeout + 10s scan interval per ADR-002).
#   6. Verify AC-1: worker status = "down" in PostgreSQL.
#   7. Verify AC-2: worker:down event captured from events:workers pub/sub.
#   8. Verify AC-3: task status = "queued" (monitor reclaimed and re-queued).
#   9. Verify AC-4: retry_count incremented in PostgreSQL.
#   10. Resume (unpause) the worker for AC-5 health check, or start a second one.
#
# Strategy for AC-5:
#   After the monitor re-queues the task, unpause the worker container. The
#   worker reconnects, picks up the re-queued task, and runs it to completion.
#
# Strategy for AC-6:
#   Submit a task that will exhaust retries. We use docker pause/unpause cycles
#   to trigger failover three times. After the third failover, retry_count=3
#   equals max_retries=3, so the monitor routes to dead-letter on the next scan.

set -uo pipefail

API_URL="${API_URL:-http://localhost:8080}"
COMPOSE_FILE="/Users/pablo/projects/Nexus/NexusTests/NexusFlow/docker-compose.yml"
WORKER_CONTAINER="${WORKER_CONTAINER:-nexusflow-worker-1}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-nexusflow-postgres-1}"
REDIS_CONTAINER="${REDIS_CONTAINER:-nexusflow-redis-1}"

# Detection + scan worst-case from ADR-002: 15s timeout + 10s scan = 25s.
# We wait 30s to provide a 5s safety margin.
DETECTION_WAIT="${DETECTION_WAIT:-30}"

PASS=0
FAIL=0
RESULTS=()

pass() {
  local name="$1"
  printf "  PASS: %s\n" "$name"
  PASS=$((PASS + 1))
  RESULTS+=("PASS | $name")
}

fail() {
  local name="$1"
  local detail="$2"
  printf "  FAIL: %s\n" "$name"
  printf "        %s\n" "$detail"
  FAIL=$((FAIL + 1))
  RESULTS+=("FAIL | $name | $detail")
}

assert_eq() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  if [ "$actual" = "$expected" ]; then
    pass "$name"
  else
    fail "$name" "expected='$expected' actual='$actual'"
  fi
}

assert_contains() {
  local name="$1"
  local needle="$2"
  local haystack="$3"
  if echo "$haystack" | grep -q "$needle"; then
    pass "$name"
  else
    fail "$name" "expected to contain '$needle', got: $haystack"
  fi
}

assert_ge() {
  # Assert actual >= expected (numeric)
  local name="$1"
  local expected="$2"
  local actual="$3"
  if [ "$actual" -ge "$expected" ] 2>/dev/null; then
    pass "$name"
  else
    fail "$name" "expected >= $expected, got $actual"
  fi
}

cleanup() {
  # Ensure worker container is always unpaused/running on exit, even on failure.
  docker unpause "$WORKER_CONTAINER" >/dev/null 2>&1 || true
  # Remove background subscriber process if running.
  if [ -n "${SUBSCRIBER_PID:-}" ]; then
    kill "$SUBSCRIBER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

printf "\n"
printf "=== TASK-009 Acceptance Tests: Monitor heartbeat checking and failover ===\n"
printf "    API: %s\n" "$API_URL"
printf "    Worker container: %s\n" "$WORKER_CONTAINER"
printf "    Detection wait: %ss\n" "$DETECTION_WAIT"
printf "\n"

# ---------------------------------------------------------------------------
# Pre-flight: Verify services are reachable
# ---------------------------------------------------------------------------
printf "Pre-flight checks\n"
printf "=================\n"

HEALTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/api/health" 2>/dev/null || echo "0")
if [ "$HEALTH_STATUS" = "200" ] || [ "$HEALTH_STATUS" = "503" ]; then
  pass "pre-flight: API server reachable at $API_URL"
else
  fail "pre-flight: API server reachable at $API_URL" "HTTP $HEALTH_STATUS — cannot continue"
  exit 1
fi

if docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c "SELECT 1;" >/dev/null 2>&1; then
  pass "pre-flight: PostgreSQL accessible"
else
  fail "pre-flight: PostgreSQL accessible" "docker exec psql failed — cannot continue"
  exit 1
fi

if docker exec "$REDIS_CONTAINER" redis-cli ping 2>&1 | grep -q PONG; then
  pass "pre-flight: Redis accessible"
else
  fail "pre-flight: Redis accessible" "redis-cli ping failed — cannot continue"
  exit 1
fi

WORKER_STATE=$(docker inspect --format '{{.State.Status}}' "$WORKER_CONTAINER" 2>/dev/null || echo "missing")
if [ "$WORKER_STATE" = "running" ]; then
  pass "pre-flight: worker container is running"
else
  fail "pre-flight: worker container is running" "container state=$WORKER_STATE — cannot continue"
  exit 1
fi

# Restart the worker to ensure a fresh registration with status='online' in PostgreSQL.
# The worker re-registers with a new UUID-suffixed ID on each process start.
# Without a fresh restart, prior paused/down workers would not have status='online'.
printf "  restarting worker container for fresh registration...\n"
docker compose -f "$COMPOSE_FILE" restart worker >/dev/null 2>&1
sleep 8
WORKER_ONLINE=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" 2>&1 | tr -d ' \n')
if [ -n "$WORKER_ONLINE" ]; then
  pass "pre-flight: worker re-registered as 'online' in PostgreSQL (id=$WORKER_ONLINE)"
else
  fail "pre-flight: worker re-registered as 'online' in PostgreSQL" "no online worker found after restart -- cannot continue"
  exit 1
fi

MONITOR_STATE=$(docker inspect --format '{{.State.Status}}' nexusflow-monitor-1 2>/dev/null || echo "missing")
if [ "$MONITOR_STATE" = "running" ]; then
  pass "pre-flight: monitor container is running"
else
  fail "pre-flight: monitor container is running" "container state=$MONITOR_STATE — cannot continue"
  exit 1
fi

printf "\n"

# ---------------------------------------------------------------------------
# Setup: Admin login
# ---------------------------------------------------------------------------
printf "Setup: admin login\n"

LOGIN_RESP=$(curl -s -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' 2>/dev/null)

ADMIN_TOKEN=$(echo "$LOGIN_RESP" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$ADMIN_TOKEN" ]; then
  fail "setup: admin login" "no token in response: $LOGIN_RESP — cannot continue"
  exit 1
fi
printf "  admin session token obtained\n"

# ---------------------------------------------------------------------------
# Setup: Find or create a test pipeline
# ---------------------------------------------------------------------------
printf "Setup: test pipeline\n"

ADMIN_USER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM users WHERE username = 'admin';" 2>&1 | tr -d ' \n')
if [ -z "$ADMIN_USER_ID" ]; then
  fail "setup: admin user ID" "could not retrieve from PostgreSQL — cannot continue"
  exit 1
fi

# Use an existing demo pipeline or insert one.
PIPELINE_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM pipelines WHERE name = 'task009-acceptance-pipeline' LIMIT 1;" \
  2>&1 | tr -d ' \n')

if [ -z "$PIPELINE_ID" ]; then
  PIPELINE_RESULT=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "INSERT INTO pipelines (name, user_id, data_source_config, process_config, sink_config)
     VALUES (
       'task009-acceptance-pipeline',
       '$ADMIN_USER_ID',
       '{\"connectorType\":\"demo\",\"config\":{},\"outputSchema\":[\"field1\"]}',
       '{\"connectorType\":\"demo\",\"config\":{},\"inputMappings\":[],\"outputSchema\":[\"field1\"]}',
       '{\"connectorType\":\"demo\",\"config\":{},\"inputMappings\":[]}'
     )
     RETURNING id;" 2>&1 | tr -d ' \n')
  PIPELINE_ID=$(echo "$PIPELINE_RESULT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || echo "")
fi

if [ -z "$PIPELINE_ID" ]; then
  fail "setup: test pipeline" "could not create pipeline — cannot continue"
  exit 1
fi
printf "  pipeline id: %s\n" "$PIPELINE_ID"
printf "\n"

# ---------------------------------------------------------------------------
# Setup: Start background subscriber to capture events:workers pub/sub messages
# ---------------------------------------------------------------------------
printf "Setup: Redis pub/sub subscriber\n"

WORKER_EVENT_FILE="/tmp/task009-worker-events-$(date +%s).txt"
# Subscribe to events:workers in background; redirect output to file for later inspection.
docker exec "$REDIS_CONTAINER" redis-cli --no-auth-warning SUBSCRIBE "events:workers" > "$WORKER_EVENT_FILE" 2>&1 &
SUBSCRIBER_PID=$!
sleep 1
printf "  background subscriber running (PID=%s), output -> %s\n" "$SUBSCRIBER_PID" "$WORKER_EVENT_FILE"
printf "\n"

# ===========================================================================
# AC-1 and AC-2: Worker heartbeat expiry detection
#
# Given: a healthy worker emitting heartbeats every 5 seconds
# When:  the worker is paused (heartbeats stop for >15 seconds)
# Then:  [AC-1] the worker's status in PostgreSQL becomes "down"
#        [AC-2] a worker:down event is published to events:workers via Redis Pub/Sub
#
# Negative case for AC-1: a healthy worker must NOT be marked down before the timeout
# ===========================================================================
printf "=== AC-1 and AC-2: Heartbeat expiry detection ===\n"

# Record the worker ID before pausing.
TARGET_WORKER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" \
  2>&1 | tr -d ' \n')

if [ -z "$TARGET_WORKER_ID" ]; then
  fail "setup: find online worker" "no online worker found in PostgreSQL — cannot continue"
  exit 1
fi
printf "  target worker ID: %s\n" "$TARGET_WORKER_ID"

# AC-1 negative case: before pausing, worker must NOT be "down"
PRE_PAUSE_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM workers WHERE id='$TARGET_WORKER_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-1 [REQ-013] [VERIFIER-ADDED] negative: online worker is not 'down' before pause" \
  "online" "$PRE_PAUSE_STATUS"

printf "  pausing worker container (heartbeats will stop)...\n"
docker pause "$WORKER_CONTAINER" >/dev/null 2>&1

printf "  waiting %ss for monitor to detect expiry (ADR-002: 15s timeout + 10s scan)...\n" "$DETECTION_WAIT"
sleep "$DETECTION_WAIT"

# AC-1 positive: worker must now be "down" in PostgreSQL
POST_PAUSE_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM workers WHERE id='$TARGET_WORKER_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-1 [REQ-013]: worker that stops heartbeating for >15s is marked 'down' in PostgreSQL" \
  "down" "$POST_PAUSE_STATUS"

# AC-2 positive: events:workers channel must have received a worker:down event
WORKER_EVENT_CONTENT=$(cat "$WORKER_EVENT_FILE" 2>/dev/null || echo "")
if echo "$WORKER_EVENT_CONTENT" | grep -q '"down"\|worker:down\|"status":"down"'; then
  pass "AC-2 [REQ-013]: worker:down event published to events:workers via Redis Pub/Sub"
else
  fail "AC-2 [REQ-013]: worker:down event published to events:workers via Redis Pub/Sub" \
    "no worker:down message captured in events:workers; file: $WORKER_EVENT_FILE content: $WORKER_EVENT_CONTENT"
fi

# AC-2 negative: verify the worker-down event contains the correct worker ID
if echo "$WORKER_EVENT_CONTENT" | grep -q "$TARGET_WORKER_ID"; then
  pass "AC-2 [REQ-013] [VERIFIER-ADDED] negative: event payload contains the downed worker ID (not a spurious event)"
else
  fail "AC-2 [REQ-013] [VERIFIER-ADDED] negative: event payload contains the downed worker ID" \
    "event did not mention worker $TARGET_WORKER_ID; content: $WORKER_EVENT_CONTENT"
fi

printf "\n"

# ===========================================================================
# AC-3 and AC-4: Pending entry XCLAIM reclamation and retry count increment
#
# Given: a task in the downed worker's XREADGROUP pending list (running state)
# When:  the pending entry has been idle >= HeartbeatTimeout (15s) AND
#        the monitor runs its scan (every 10s)
# Then:  [AC-3] the task is reclaimed via XCLAIM and re-queued (status = "queued")
#        [AC-4] the task's retry_count is incremented in PostgreSQL
#
# NOTE on timing: ListPendingOlderThan filters by idle time >= HeartbeatTimeout (15s).
# The pending entry must have been idle for 15s BEFORE the monitor can claim it.
# We inject the entry immediately after confirming the worker is down (from AC-1),
# then wait 25s (15s idle + 10s max scan interval) before asserting.
#
# Negative case for AC-3: task NOT reclaimed keeps original retry_count
# Negative case for AC-4: task with no failover keeps retry_count = 0
# ===========================================================================
printf "=== AC-3 and AC-4: XCLAIM reclamation and retry counter increment ===\n"
printf "  (worker is still paused; injecting task into pending list)\n"

# Step 1: Create a task in PostgreSQL with status 'running' and retry_count=0.
# Direct INSERT: the worker is paused so we cannot go through the API submission flow.
RECLAIM_TASK_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (pipeline_id, user_id, status, retry_config, retry_count, execution_id, input)
   VALUES (
     '$PIPELINE_ID',
     '$ADMIN_USER_ID',
     'running',
     '{\"maxRetries\":3,\"backoff\":\"exponential\"}',
     0,
     gen_random_uuid()::text || ':0',
     '{\"key\":\"task009-ac3-ac4-test\"}'
   )
   RETURNING id;" 2>&1 | tr -d ' \n')
RECLAIM_TASK_ID=$(echo "$RECLAIM_TASK_ID" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)

if [ -z "$RECLAIM_TASK_ID" ]; then
  fail "setup: insert reclaim test task" "INSERT failed -- cannot continue"
  exit 1
fi
printf "  reclaim test task ID: %s\n" "$RECLAIM_TASK_ID"

# Step 2: Insert state_log transitions so the DB trigger is satisfied.
docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c \
  "INSERT INTO task_state_log (task_id, from_state, to_state, reason)
   VALUES
     ('$RECLAIM_TASK_ID', 'submitted', 'queued', 'task submitted'),
     ('$RECLAIM_TASK_ID', 'queued', 'assigned', 'assigned to worker'),
     ('$RECLAIM_TASK_ID', 'assigned', 'running', 'execution started');" >/dev/null 2>&1

# Step 3: XADD the task to the stream, then XREADGROUP consume it as the downed
# worker -- this puts the entry in the worker's pending list.
STREAM_ID=$(docker exec "$REDIS_CONTAINER" redis-cli XADD "queue:demo" "*" \
  "payload" "{\"taskId\":\"$RECLAIM_TASK_ID\",\"pipelineId\":\"$PIPELINE_ID\",\"userId\":\"$ADMIN_USER_ID\",\"executionId\":\"$RECLAIM_TASK_ID:0\"}" 2>&1 | tr -d ' \n')
printf "  XADD stream entry ID: %s\n" "$STREAM_ID"

if [ -z "$STREAM_ID" ]; then
  fail "setup: XADD task to stream" "XADD returned empty -- cannot continue"
  exit 1
fi

# Ensure consumer group exists.
docker exec "$REDIS_CONTAINER" redis-cli XGROUP CREATE "queue:demo" "workers" "0" 2>/dev/null || true

# Read it as the downed worker -- this creates the pending entry.
docker exec "$REDIS_CONTAINER" redis-cli XREADGROUP GROUP "workers" "$TARGET_WORKER_ID" \
  COUNT 1 STREAMS "queue:demo" ">" >/dev/null 2>&1

# Verify the entry is in the pending list before waiting.
PENDING_COUNT=$(docker exec "$REDIS_CONTAINER" redis-cli XPENDING "queue:demo" "workers" - + 100 2>&1 | grep -c "$TARGET_WORKER_ID" || echo "0")
printf "  pending entries for downed worker: %s\n" "$PENDING_COUNT"

# Wait: 15s (for idle time to exceed HeartbeatTimeout) + 10s (scan interval) + 5s buffer = 30s.
# The monitor filters entries by IDLE >= HeartbeatTimeout (15s). Without waiting at least
# 15s after the XREADGROUP call, the entry will not appear in ListPendingOlderThan results.
printf "  waiting 30s for idle threshold and monitor scan cycle (ADR-002: 15s idle + 10s scan)...\n"
sleep 30

# AC-3 positive: task status must be "queued" (reclaimed and re-queued by monitor)
RECLAIMED_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id='$RECLAIM_TASK_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-3 [REQ-013]: pending task on downed worker is reclaimed via XCLAIM and re-queued" \
  "queued" "$RECLAIMED_STATUS"

# AC-3 negative: the XCLAIM pending entry must no longer be owned by the downed worker
DOWNED_WORKER_PENDING=$(docker exec "$REDIS_CONTAINER" redis-cli XPENDING "queue:demo" "workers" - + 100 2>&1)
if echo "$DOWNED_WORKER_PENDING" | grep -q "$TARGET_WORKER_ID"; then
  fail "AC-3 [REQ-013] [VERIFIER-ADDED] negative: downed worker no longer owns the pending entry after XCLAIM" \
    "entry still shows downed worker $TARGET_WORKER_ID in pending list"
else
  pass "AC-3 [REQ-013] [VERIFIER-ADDED] negative: downed worker no longer owns the pending entry after XCLAIM"
fi

# AC-4 positive: retry_count must be 1 (incremented from 0)
RETRY_COUNT=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT retry_count FROM tasks WHERE id='$RECLAIM_TASK_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-4 [REQ-011]: task retry_count incremented to 1 after first failover reassignment" \
  "1" "$RETRY_COUNT"

# AC-4 negative: task that was NOT reclaimed (completed by a healthy worker) must NOT have
# retry_count incremented. We check the previously completed test task (submitted earlier).
COMPLETED_TASK_RETRY=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT retry_count FROM tasks WHERE id='c541310c-ed89-4afc-ba1c-b31b43ae0774';" \
  2>&1 | tr -d ' \n')
if [ "$COMPLETED_TASK_RETRY" = "0" ]; then
  pass "AC-4 [REQ-011] [VERIFIER-ADDED] negative: healthy-completed task retry_count not incremented (still 0)"
else
  fail "AC-4 [REQ-011] [VERIFIER-ADDED] negative: healthy-completed task retry_count not incremented" \
    "expected 0, got $COMPLETED_TASK_RETRY"
fi

printf "\n"

# ===========================================================================
# AC-5: Reclaimed task picked up by a healthy matching worker
#
# Given: the monitor re-queued the reclaimed task (via re-XADD to the stream)
# When:  a healthy worker with matching tag resumes and reads from the stream
# Then:  the task is eventually completed (or at least assigned/running) by the worker
#
# Strategy: unpause the worker; the re-queued task is in the stream; the worker
# picks it up via XREADGROUP ">". Wait up to 15s for the task to reach a
# terminal or in-progress state.
# ===========================================================================
printf "=== AC-5: Reclaimed task picked up by healthy matching worker ===\n"

printf "  unpausing worker container...\n"
docker unpause "$WORKER_CONTAINER" >/dev/null 2>&1
printf "  waiting 15s for worker to pick up and complete the reclaimed task...\n"
sleep 15

# AC-5 positive: task must have progressed past "queued" — assigned, running, or completed
FINAL_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id='$RECLAIM_TASK_ID';" 2>&1 | tr -d ' \n')
if [ "$FINAL_STATUS" = "assigned" ] || [ "$FINAL_STATUS" = "running" ] || [ "$FINAL_STATUS" = "completed" ]; then
  pass "AC-5 [REQ-013]: reclaimed task picked up by healthy matching worker (status='$FINAL_STATUS')"
else
  fail "AC-5 [REQ-013]: reclaimed task picked up by healthy matching worker" \
    "expected assigned/running/completed, got '$FINAL_STATUS'"
fi

# AC-5 negative: the task must NOT still be in the dead-letter queue
DLQ_CHECK=$(docker exec "$REDIS_CONTAINER" redis-cli XRANGE "queue:dead-letter" - + 2>&1)
if echo "$DLQ_CHECK" | grep -q "$RECLAIM_TASK_ID"; then
  fail "AC-5 [REQ-013] [VERIFIER-ADDED] negative: reclaimed task (with retries remaining) not in dead-letter queue" \
    "task $RECLAIM_TASK_ID found in queue:dead-letter — should have been re-queued, not dead-lettered"
else
  pass "AC-5 [REQ-013] [VERIFIER-ADDED] negative: reclaimed task (with retries remaining) not in dead-letter queue"
fi

printf "\n"

# ===========================================================================
# AC-6: Task with exhausted retries moved to queue:dead-letter; status = "failed"
#
# Given: a task with retry_count already at max_retries (3 of 3)
# When:  the monitor scans and finds the task in the pending list
# Then:  the task is moved to queue:dead-letter and status set to "failed"
#        (NOT reclaimed and re-queued)
#
# Negative case: a task with retries REMAINING is NOT sent to dead-letter
# ===========================================================================
printf "=== AC-6: Exhausted retries -> queue:dead-letter, status 'failed' ===\n"

# Restart the worker to get a fresh registration before the AC-6 pause cycle.
printf "  restarting worker for fresh online registration (needed for AC-6 pause cycle)...\n"
docker compose -f "$COMPOSE_FILE" restart worker >/dev/null 2>&1
sleep 8
AC6_WORKER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" 2>&1 | tr -d ' \n')
if [ -z "$AC6_WORKER_ID" ]; then
  fail "AC-6 setup: worker re-registration" "no online worker found -- cannot continue"
  exit 1
fi
printf "  fresh worker ID for AC-6: %s\n" "$AC6_WORKER_ID"

# Create a task with retry_count=3 (= max_retries=3, so retries are exhausted).
DLQ_TASK_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (pipeline_id, user_id, status, retry_config, retry_count, execution_id, input)
   VALUES (
     '$PIPELINE_ID',
     '$ADMIN_USER_ID',
     'running',
     '{\"maxRetries\":3,\"backoff\":\"exponential\"}',
     3,
     gen_random_uuid()::text || ':3',
     '{\"key\":\"task009-ac6-exhausted\"}'
   )
   RETURNING id;" 2>&1 | tr -d ' \n')
DLQ_TASK_ID=$(echo "$DLQ_TASK_ID" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)

if [ -z "$DLQ_TASK_ID" ]; then
  fail "setup: insert dead-letter test task" "INSERT failed — cannot continue"
  exit 1
fi
printf "  dead-letter test task ID: %s\n" "$DLQ_TASK_ID"

# Add state log entries for the dead-letter task.
docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c \
  "INSERT INTO task_state_log (task_id, from_state, to_state, reason)
   VALUES
     ('$DLQ_TASK_ID', 'submitted', 'queued', 'task submitted'),
     ('$DLQ_TASK_ID', 'queued', 'assigned', 'assigned to worker'),
     ('$DLQ_TASK_ID', 'assigned', 'running', 'execution started');" >/dev/null 2>&1

# Pause the worker to stop it from consuming messages.
docker pause "$WORKER_CONTAINER" >/dev/null 2>&1

# Use the fresh worker ID from the restart above.
DLQ_WORKER_ID="$AC6_WORKER_ID"
printf "  paused worker (ID=%s) -- heartbeats will stop\n" "$DLQ_WORKER_ID"

# XADD the task to the stream.
DLQ_STREAM_ID=$(docker exec "$REDIS_CONTAINER" redis-cli XADD "queue:demo" "*" \
  "payload" "{\"taskId\":\"$DLQ_TASK_ID\",\"pipelineId\":\"$PIPELINE_ID\",\"userId\":\"$ADMIN_USER_ID\",\"executionId\":\"$DLQ_TASK_ID:3\"}" 2>&1 | tr -d ' \n')
printf "  XADD stream entry ID for dead-letter task: %s\n" "$DLQ_STREAM_ID"

# XREADGROUP consume it as the current worker to put it in the pending list.
docker exec "$REDIS_CONTAINER" redis-cli XREADGROUP GROUP "workers" "$DLQ_WORKER_ID" \
  COUNT 1 STREAMS "queue:demo" ">" >/dev/null 2>&1

# Record the dead-letter stream length before the monitor runs.
DLQ_LEN_BEFORE=$(docker exec "$REDIS_CONTAINER" redis-cli XLEN "queue:dead-letter" 2>&1 | tr -d ' \n')
printf "  queue:dead-letter length before monitor run: %s\n" "$DLQ_LEN_BEFORE"

# Wait for monitor to detect worker down AND scan pending entries.
printf "  waiting %ss for monitor to detect expiry and dead-letter the exhausted task...\n" "$DETECTION_WAIT"
sleep "$DETECTION_WAIT"

# AC-6 positive: task status must be "failed"
DLQ_TASK_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id='$DLQ_TASK_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-6 [REQ-013]: exhausted-retry task status is 'failed' in PostgreSQL" \
  "failed" "$DLQ_TASK_STATUS"

# AC-6 positive: task must be present in queue:dead-letter
DLQ_LEN_AFTER=$(docker exec "$REDIS_CONTAINER" redis-cli XLEN "queue:dead-letter" 2>&1 | tr -d ' \n')
DLQ_CONTENT=$(docker exec "$REDIS_CONTAINER" redis-cli XRANGE "queue:dead-letter" - + 2>&1)

if echo "$DLQ_CONTENT" | grep -q "$DLQ_TASK_ID"; then
  pass "AC-6 [REQ-013]: exhausted-retry task found in queue:dead-letter stream"
else
  fail "AC-6 [REQ-013]: exhausted-retry task found in queue:dead-letter stream" \
    "task $DLQ_TASK_ID not found in queue:dead-letter (len before=$DLQ_LEN_BEFORE, after=$DLQ_LEN_AFTER)"
fi

# AC-6 positive: dead-letter stream length must have grown
assert_ge "AC-6 [REQ-013]: queue:dead-letter entry count grew after exhausted-retry failover" \
  "$((DLQ_LEN_BEFORE + 1))" "$DLQ_LEN_AFTER"

# AC-6 negative: a task with retries remaining must NOT be in dead-letter queue.
# We verify the AC-3/4/5 reclaimed task is NOT in dead-letter.
if echo "$DLQ_CONTENT" | grep -q "$RECLAIM_TASK_ID"; then
  fail "AC-6 [REQ-013] [VERIFIER-ADDED] negative: task with retries remaining NOT dead-lettered" \
    "task $RECLAIM_TASK_ID (retry_count=1, max=3) was incorrectly sent to queue:dead-letter"
else
  pass "AC-6 [REQ-013] [VERIFIER-ADDED] negative: task with retries remaining was NOT sent to queue:dead-letter"
fi

# Unpause worker for cleanup.
docker unpause "$WORKER_CONTAINER" >/dev/null 2>&1

printf "\n"

# ===========================================================================
# Additional negative case: A worker with a recent heartbeat is NOT marked down
# [VERIFIER-ADDED] Verifies AC-1's negative path: no false positives on
# healthy workers (critical for production stability)
# ===========================================================================
printf "=== [VERIFIER-ADDED] Negative: healthy worker is not incorrectly marked down ===\n"

# Restart the worker to get a fresh registration with status='online'.
# This directly verifies that a worker that has NOT stopped heartbeating
# is NOT marked down by the monitor. We verify:
#   1. After restart the worker is "online" in PostgreSQL.
#   2. After one full monitor scan cycle (10s), it remains "online".
# This is the definitive negative case for AC-1: the monitor must NOT mark
# a worker down if its heartbeats are within the 15s timeout.
printf "  restarting worker to get fresh 'online' registration...\n"
docker compose -f "$COMPOSE_FILE" restart worker >/dev/null 2>&1
sleep 8

HEALTHY_WORKER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" \
  2>&1 | tr -d ' \n')

if [ -z "$HEALTHY_WORKER_ID" ]; then
  fail "AC-1 [REQ-013] [VERIFIER-ADDED] negative: healthy worker not incorrectly marked down" \
    "no online worker found after restart"
else
  # Wait one full scan cycle (10s) + buffer (5s) = 15s.
  # The monitor scans every 10s; if the worker is healthy it must remain 'online'.
  printf "  waiting 15s (one full scan cycle + buffer) while worker heartbeats normally...\n"
  sleep 15

  HEALTHY_WORKER_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "SELECT status FROM workers WHERE id='$HEALTHY_WORKER_ID';" 2>&1 | tr -d ' \n')
  if [ "$HEALTHY_WORKER_STATUS" = "online" ]; then
    pass "AC-1 [REQ-013] [VERIFIER-ADDED] negative: healthy worker ($HEALTHY_WORKER_ID) remains 'online' after scan cycle (no false positive)"
  else
    fail "AC-1 [REQ-013] [VERIFIER-ADDED] negative: healthy worker remains 'online' after scan cycle" \
      "worker $HEALTHY_WORKER_ID status='$HEALTHY_WORKER_STATUS' -- monitor false-positive detected"
  fi
fi

printf "\n"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
printf "=== Results ===\n"
for r in "${RESULTS[@]}"; do
  printf "  %s\n" "$r"
done
printf "\n"
printf "  Passed: %d\n" "$PASS"
printf "  Failed: %d\n" "$FAIL"
printf "\n"

if [ "$FAIL" -eq 0 ]; then
  printf "VERDICT: PASS\n"
  exit 0
else
  printf "VERDICT: FAIL\n"
  exit 1
fi
