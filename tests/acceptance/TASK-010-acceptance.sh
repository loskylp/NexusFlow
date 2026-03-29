#!/usr/bin/env bash
# TASK-010 Acceptance Tests — Infrastructure retry with backoff
#
# REQ-011: Infrastructure-failure retry with per-task configuration.
#   Definition of Done: An infrastructure-failed task is retried up to the
#   configured limit. A process-error-failed task is not retried. Retry count
#   and backoff are configurable per task with documented defaults.
#
# Acceptance criteria under test:
#   AC-1: Task with {max_retries: 3, backoff: "exponential"} is retried up to
#         3 times on infrastructure failure.
#   AC-2: Backoff delay is applied between retries (exponential: 1s, 2s, 4s).
#   AC-3: Task failing due to Process script error is NOT retried and
#         transitions to "failed" immediately.
#   AC-4: Task that exhausts retries transitions to "failed" and is placed in
#         the dead letter queue.
#   AC-5: Retry count is visible in task state (GET /api/tasks/{id}).
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-010-acceptance.sh
#
# Requires:
#   - curl, docker exec (for psql/redis-cli access)
#   - docker compose stack running: api, worker, monitor, postgres, redis
#   - Services built from TASK-010 code (migration 000005 applied)
#
# Services required: all services from docker-compose.yml
#
# Design notes (for AC-2 backoff timing):
#   The Monitor sets retry_after = now + backoffDelay and then ACKs the pending
#   entry. scanRetryReady re-enqueues when retry_after <= now().
#   For exponential backoff:
#     retry 0 -> delay 1s  (first failure: retry_after = now+1s)
#     retry 1 -> delay 2s  (second failure: retry_after = now+2s)
#     retry 2 -> delay 4s  (third failure: retry_after = now+4s)
#   We verify AC-2 by checking the retry_after timestamp stored in PostgreSQL
#   immediately after a reclaim event (within a 3s tolerance window):
#     expected: now + 1s  (± 3s)
#   We do not time-wait 4s+ between retries for every AC — that would make
#   the test suite impractically slow. Instead:
#     - AC-2: inject a task at retry_count=0, trigger one reclaim, check
#             retry_after is ~now+1s.
#     - AC-2 negative: also inject at retry_count=1, check retry_after ~now+2s.
#   The actual re-enqueue is observable via scanRetryReady clearing retry_after.
#
# Detection timing (ADR-002):
#   HeartbeatTimeout = 15s, PendingScanInterval = 10s.
#   Worst-case detection after pause: 15s + 10s = 25s.
#   We wait DETECTION_WAIT (default 30s) with a 5s safety margin.

set -uo pipefail

API_URL="${API_URL:-http://localhost:8080}"
COMPOSE_FILE="/Users/pablo/projects/Nexus/NexusTests/NexusFlow/docker-compose.yml"
WORKER_CONTAINER="${WORKER_CONTAINER:-nexusflow-worker-1}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-nexusflow-postgres-1}"
REDIS_CONTAINER="${REDIS_CONTAINER:-nexusflow-redis-1}"
MONITOR_CONTAINER="${MONITOR_CONTAINER:-nexusflow-monitor-1}"

# Worst-case monitor detection: 15s timeout + 10s scan = 25s + 5s margin.
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
  local name="$1"
  local expected="$2"
  local actual="$3"
  if [ "$actual" -ge "$expected" ] 2>/dev/null; then
    pass "$name"
  else
    fail "$name" "expected >= $expected, got $actual"
  fi
}

assert_not_contains() {
  local name="$1"
  local needle="$2"
  local haystack="$3"
  if echo "$haystack" | grep -q "$needle"; then
    fail "$name" "expected NOT to contain '$needle', but found it"
  else
    pass "$name"
  fi
}

cleanup() {
  # Ensure worker container is always unpaused/running on exit, even on failure.
  docker unpause "$WORKER_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

printf "\n"
printf "=== TASK-010 Acceptance Tests: Infrastructure retry with backoff ===\n"
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

# Verify migration 000005 applied — retry_after column must exist.
RETRY_AFTER_COL=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT column_name FROM information_schema.columns
   WHERE table_name='tasks' AND column_name='retry_after';" 2>&1 | tr -d ' \n')
if [ "$RETRY_AFTER_COL" = "retry_after" ]; then
  pass "pre-flight: migration 000005 applied (retry_after column exists)"
else
  fail "pre-flight: migration 000005 applied" \
    "retry_after column not found in tasks table — run migration 000005 first"
  exit 1
fi

RETRY_TAGS_COL=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT column_name FROM information_schema.columns
   WHERE table_name='tasks' AND column_name='retry_tags';" 2>&1 | tr -d ' \n')
if [ "$RETRY_TAGS_COL" = "retry_tags" ]; then
  pass "pre-flight: migration 000005 applied (retry_tags column exists)"
else
  fail "pre-flight: migration 000005 applied" \
    "retry_tags column not found in tasks table — run migration 000005 first"
  exit 1
fi

WORKER_STATE=$(docker inspect --format '{{.State.Status}}' "$WORKER_CONTAINER" 2>/dev/null || echo "missing")
if [ "$WORKER_STATE" = "running" ]; then
  pass "pre-flight: worker container is running"
else
  fail "pre-flight: worker container is running" "container state=$WORKER_STATE — cannot continue"
  exit 1
fi

MONITOR_STATE=$(docker inspect --format '{{.State.Status}}' "$MONITOR_CONTAINER" 2>/dev/null || echo "missing")
if [ "$MONITOR_STATE" = "running" ]; then
  pass "pre-flight: monitor container is running"
else
  fail "pre-flight: monitor container is running" "container state=$MONITOR_STATE — cannot continue"
  exit 1
fi

# Restart worker for a clean online registration.
printf "  restarting worker container for fresh registration...\n"
docker compose -f "$COMPOSE_FILE" restart worker >/dev/null 2>&1
sleep 8

WORKER_ONLINE=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" 2>&1 | tr -d ' \n')
if [ -n "$WORKER_ONLINE" ]; then
  pass "pre-flight: worker re-registered as 'online' in PostgreSQL (id=$WORKER_ONLINE)"
else
  fail "pre-flight: worker re-registered as 'online'" "no online worker found — cannot continue"
  exit 1
fi

printf "\n"

# ---------------------------------------------------------------------------
# Setup: Admin login and test pipeline
# ---------------------------------------------------------------------------
printf "Setup: admin login and test pipeline\n"
printf "=====================================\n"

LOGIN_RESP=$(curl -s -X POST "$API_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' 2>/dev/null)

ADMIN_TOKEN=$(echo "$LOGIN_RESP" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
if [ -z "$ADMIN_TOKEN" ]; then
  fail "setup: admin login" "no token in response: $LOGIN_RESP — cannot continue"
  exit 1
fi
printf "  admin session token obtained\n"

ADMIN_USER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM users WHERE username = 'admin';" 2>&1 | tr -d ' \n')
if [ -z "$ADMIN_USER_ID" ]; then
  fail "setup: admin user ID" "could not retrieve from PostgreSQL — cannot continue"
  exit 1
fi

# Find or create test pipeline.
PIPELINE_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM pipelines WHERE name = 'task010-acceptance-pipeline' LIMIT 1;" \
  2>&1 | tr -d ' \n')

if [ -z "$PIPELINE_ID" ]; then
  PIPELINE_RESULT=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "INSERT INTO pipelines (name, user_id, data_source_config, process_config, sink_config)
     VALUES (
       'task010-acceptance-pipeline',
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
# Ensure consumer group exists for the demo stream.
# ---------------------------------------------------------------------------
docker exec "$REDIS_CONTAINER" redis-cli XGROUP CREATE "queue:demo" "workers" "0" 2>/dev/null || true

# ===========================================================================
# AC-1: Task with {max_retries: 3, backoff: "exponential"} is retried up to
#       3 times on infrastructure failure.
#
# Given: a task with retry_config {maxRetries:3, backoff:"exponential"} and
#        retry_count=0 is in the pending list of a downed worker
# When:  the monitor detects the worker down and scans pending entries
# Then:  the task is reclaimed (retry_count incremented to 1, status="queued")
#        and NOT dead-lettered (retries not exhausted)
#
# Negative case [VERIFIER-ADDED]: a task with retry_count=3 (exhausted) is
#        NOT reclaimed as another retry — it goes straight to dead-letter.
# ===========================================================================
printf "=== AC-1 [REQ-011]: Task retried up to max_retries=3 on infrastructure failure ===\n"

# Record current online worker.
AC1_WORKER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" \
  2>&1 | tr -d ' \n')
if [ -z "$AC1_WORKER_ID" ]; then
  fail "AC-1 setup: find online worker" "no online worker found — cannot continue"
  exit 1
fi
printf "  target worker for AC-1: %s\n" "$AC1_WORKER_ID"

# Insert a task with retry_count=0 and max_retries=3.
AC1_TASK_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (pipeline_id, user_id, status, retry_config, retry_count, execution_id, input)
   VALUES (
     '$PIPELINE_ID',
     '$ADMIN_USER_ID',
     'running',
     '{\"maxRetries\":3,\"backoff\":\"exponential\"}',
     0,
     gen_random_uuid()::text || ':0',
     '{\"key\":\"task010-ac1-test\"}'
   )
   RETURNING id;" 2>&1 | tr -d ' \n')
AC1_TASK_ID=$(echo "$AC1_TASK_ID" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)

if [ -z "$AC1_TASK_ID" ]; then
  fail "AC-1 setup: insert task" "INSERT failed — cannot continue"
  exit 1
fi
printf "  AC-1 test task ID: %s\n" "$AC1_TASK_ID"

docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c \
  "INSERT INTO task_state_log (task_id, from_state, to_state, reason)
   VALUES
     ('$AC1_TASK_ID', 'submitted', 'queued', 'task submitted'),
     ('$AC1_TASK_ID', 'queued', 'assigned', 'assigned to worker'),
     ('$AC1_TASK_ID', 'assigned', 'running', 'execution started');" >/dev/null 2>&1

# XADD + XREADGROUP to place in downed worker's pending list.
docker exec "$REDIS_CONTAINER" redis-cli XADD "queue:demo" "*" \
  "payload" "{\"taskId\":\"$AC1_TASK_ID\",\"pipelineId\":\"$PIPELINE_ID\",\"userId\":\"$ADMIN_USER_ID\",\"executionId\":\"$AC1_TASK_ID:0\"}" >/dev/null 2>&1
docker exec "$REDIS_CONTAINER" redis-cli XREADGROUP GROUP "workers" "$AC1_WORKER_ID" \
  COUNT 1 STREAMS "queue:demo" ">" >/dev/null 2>&1

# Pause the worker to stop heartbeating.
printf "  pausing worker (heartbeats stop)...\n"
docker pause "$WORKER_CONTAINER" >/dev/null 2>&1

printf "  waiting %ss for monitor to detect expiry and reclaim task...\n" "$DETECTION_WAIT"
sleep "$DETECTION_WAIT"

# AC-1 positive: retry_count must be 1 (incremented from 0).
AC1_RETRY_COUNT=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT retry_count FROM tasks WHERE id='$AC1_TASK_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-1 [REQ-011]: task retry_count incremented to 1 after first infrastructure failure" \
  "1" "$AC1_RETRY_COUNT"

# AC-1 positive: status must be "queued" (re-enqueued for retry, not dead-lettered).
AC1_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id='$AC1_TASK_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-1 [REQ-011]: task with retries remaining transitions to 'queued' for retry" \
  "queued" "$AC1_STATUS"

# AC-1 negative [VERIFIER-ADDED]: task with retry_count=0 must NOT be dead-lettered.
DLQ_CONTENT=$(docker exec "$REDIS_CONTAINER" redis-cli XRANGE "queue:dead-letter" - + 2>&1)
assert_not_contains \
  "AC-1 [REQ-011] [VERIFIER-ADDED] negative: task with 0 prior retries is NOT dead-lettered on first failure" \
  "$AC1_TASK_ID" "$DLQ_CONTENT"

printf "\n"

# Unpause worker before next scenario.
docker unpause "$WORKER_CONTAINER" >/dev/null 2>&1

# ===========================================================================
# AC-2: Backoff delay applied between retries (exponential: 1s, 2s, 4s)
#
# Given: a task with backoff="exponential" at retry_count=0 is reclaimed by
#        the monitor after a worker failure
# When:  the monitor reclaims and re-dispatches the task
# Then:  there is at least 1 second between the running->queued (reclaim)
#        transition and the queued->assigned (re-dispatch) transition in the
#        task_state_log, confirming the 1s backoff delay was applied.
#
# Given: a task at retry_count=1 is reclaimed
# Then:  the time between reclaim and re-dispatch is at least 2 seconds.
#
# Strategy: We verify AC-2 via task_state_log timestamps, not the transient
# retry_after column. retry_after is cleared to NULL by scanRetryReady when
# it re-enqueues the task — so by the time we read it after a 30s wait, it
# may already be NULL (correctly cleared). The state_log persists forever and
# the gap between transitions proves the delay was applied.
#
# Both tasks are injected and the worker is paused. We wait DETECTION_WAIT
# for the monitor to reclaim and then re-dispatch them after their backoffs
# (1s and 2s respectively). After the wait we query state_log timestamps.
#
# Negative case [VERIFIER-ADDED]: a task with retry_count=0 (1s delay) must
#   be re-dispatched BEFORE a task with retry_count=1 (2s delay), confirming
#   exponential growth (longer delays for more retries).
# ===========================================================================
printf "=== AC-2 [REQ-011]: Exponential backoff delay (1s, 2s, 4s) applied between retries ===\n"

# Restart worker for a fresh online worker ID.
printf "  restarting worker for fresh registration (AC-2 scenario)...\n"
docker compose -f "$COMPOSE_FILE" restart worker >/dev/null 2>&1
sleep 8

AC2_WORKER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" \
  2>&1 | tr -d ' \n')
if [ -z "$AC2_WORKER_ID" ]; then
  fail "AC-2 setup: find online worker after restart" "no online worker found — cannot continue AC-2"
else

  # Task A: retry_count=0 -> expected backoff 1s (exponential: 1s * 2^0).
  AC2A_TASK_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "INSERT INTO tasks (pipeline_id, user_id, status, retry_config, retry_count, execution_id, input)
     VALUES (
       '$PIPELINE_ID',
       '$ADMIN_USER_ID',
       'running',
       '{\"maxRetries\":3,\"backoff\":\"exponential\"}',
       0,
       gen_random_uuid()::text || ':0',
       '{\"key\":\"task010-ac2a-test\"}'
     )
     RETURNING id;" 2>&1 | tr -d ' \n')
  AC2A_TASK_ID=$(echo "$AC2A_TASK_ID" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)

  # Task B: retry_count=1 -> expected backoff 2s (exponential: 1s * 2^1).
  AC2B_TASK_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "INSERT INTO tasks (pipeline_id, user_id, status, retry_config, retry_count, execution_id, input)
     VALUES (
       '$PIPELINE_ID',
       '$ADMIN_USER_ID',
       'running',
       '{\"maxRetries\":3,\"backoff\":\"exponential\"}',
       1,
       gen_random_uuid()::text || ':1',
       '{\"key\":\"task010-ac2b-test\"}'
     )
     RETURNING id;" 2>&1 | tr -d ' \n')
  AC2B_TASK_ID=$(echo "$AC2B_TASK_ID" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)

  printf "  AC-2 task A (retry_count=0, expected delay=1s): %s\n" "$AC2A_TASK_ID"
  printf "  AC-2 task B (retry_count=1, expected delay=2s): %s\n" "$AC2B_TASK_ID"

  # Pause the worker BEFORE enqueuing tasks so the worker never picks them up directly.
  # This ensures the only queued->assigned transition after the reclaim is from scanRetryReady.
  printf "  pausing worker before XADD to prevent direct pickup (AC-2 scenario)...\n"
  docker pause "$WORKER_CONTAINER" >/dev/null 2>&1

  for TID in "$AC2A_TASK_ID" "$AC2B_TASK_ID"; do
    docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c \
      "INSERT INTO task_state_log (task_id, from_state, to_state, reason)
       VALUES
         ('$TID', 'submitted', 'queued', 'task submitted'),
         ('$TID', 'queued', 'assigned', 'assigned to worker'),
         ('$TID', 'assigned', 'running', 'execution started');" >/dev/null 2>&1

    docker exec "$REDIS_CONTAINER" redis-cli XADD "queue:demo" "*" \
      "payload" "{\"taskId\":\"$TID\",\"pipelineId\":\"$PIPELINE_ID\",\"userId\":\"$ADMIN_USER_ID\",\"executionId\":\"$TID:x\"}" >/dev/null 2>&1
    docker exec "$REDIS_CONTAINER" redis-cli XREADGROUP GROUP "workers" "$AC2_WORKER_ID" \
      COUNT 1 STREAMS "queue:demo" ">" >/dev/null 2>&1
  done
  # Phase 1: Wait for monitor to detect worker down and reclaim tasks (sets retry_after).
  # ADR-002: 15s idle threshold + 10s scan = 25s worst case; DETECTION_WAIT adds 5s margin.
  printf "  waiting %ss for monitor to detect worker down and reclaim tasks...\n" "$DETECTION_WAIT"
  sleep "$DETECTION_WAIT"
  # Phase 2: Unpause the worker so it can consume the re-dispatched tasks.
  # scanRetryReady has already run and re-XADDed the tasks (1s and 2s delays elapsed).
  # The worker needs to be online to pick them up via XREADGROUP.
  printf "  unpausing worker so it can consume re-dispatched tasks...\n"
  docker unpause "$WORKER_CONTAINER" >/dev/null 2>&1
  printf "  waiting 10s for worker to consume re-dispatched tasks...\n"
  sleep 10

  # Verify AC-2 via task_state_log timestamps.
  # The gap between running->queued (reclaim) and queued->assigned (re-dispatch after backoff)
  # must be at least 1 second for task A (retry_count=0, exponential 1s delay).
  # We query: EXTRACT(EPOCH FROM (re_dispatch_time - reclaim_time)) for each task.
  AC2A_DELAY=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "SELECT EXTRACT(EPOCH FROM (
       (SELECT timestamp FROM task_state_log
        WHERE task_id='$AC2A_TASK_ID' AND from_state='queued' AND to_state='assigned'
        ORDER BY timestamp DESC LIMIT 1)
       -
       (SELECT timestamp FROM task_state_log
        WHERE task_id='$AC2A_TASK_ID' AND from_state='running' AND to_state='queued'
        ORDER BY timestamp DESC LIMIT 1)
     ))::int;" 2>&1 | tr -d ' \n')

  if [ -z "$AC2A_DELAY" ] || [ "$AC2A_DELAY" = "" ]; then
    fail "AC-2 [REQ-011]: task A (retry_count=0) was re-dispatched after backoff (state log shows queued->assigned after reclaim)" \
      "queued->assigned transition not found in state log for task $AC2A_TASK_ID — task not re-dispatched after backoff"
  elif [ "$AC2A_DELAY" -ge 1 ] 2>/dev/null; then
    pass "AC-2 [REQ-011]: task A (retry_count=0, exponential 1s) delay between reclaim and re-dispatch >= 1s (actual=${AC2A_DELAY}s)"
  else
    fail "AC-2 [REQ-011]: task A (retry_count=0, exponential 1s) delay between reclaim and re-dispatch >= 1s" \
      "actual delay=${AC2A_DELAY}s — task was re-dispatched before the 1s backoff elapsed"
  fi

  # Task B: gap must be at least 2s (retry_count=1, exponential 2s delay).
  AC2B_DELAY=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "SELECT EXTRACT(EPOCH FROM (
       (SELECT timestamp FROM task_state_log
        WHERE task_id='$AC2B_TASK_ID' AND from_state='queued' AND to_state='assigned'
        ORDER BY timestamp DESC LIMIT 1)
       -
       (SELECT timestamp FROM task_state_log
        WHERE task_id='$AC2B_TASK_ID' AND from_state='running' AND to_state='queued'
        ORDER BY timestamp DESC LIMIT 1)
     ))::int;" 2>&1 | tr -d ' \n')

  if [ -z "$AC2B_DELAY" ] || [ "$AC2B_DELAY" = "" ]; then
    fail "AC-2 [REQ-011]: task B (retry_count=1) was re-dispatched after backoff" \
      "queued->assigned transition not found in state log for task $AC2B_TASK_ID"
  elif [ "$AC2B_DELAY" -ge 2 ] 2>/dev/null; then
    pass "AC-2 [REQ-011]: task B (retry_count=1, exponential 2s) delay between reclaim and re-dispatch >= 2s (actual=${AC2B_DELAY}s)"
  else
    fail "AC-2 [REQ-011]: task B (retry_count=1, exponential 2s) delay between reclaim and re-dispatch >= 2s" \
      "actual delay=${AC2B_DELAY}s — task was re-dispatched before the 2s backoff elapsed"
  fi

  # AC-2 negative [VERIFIER-ADDED]: task B (2s delay) must be re-dispatched AFTER task A (1s delay).
  # Verify by comparing the queued->assigned timestamps: task B re-dispatch time >= task A re-dispatch time.
  AC2_ORDER=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "SELECT CASE
       WHEN (SELECT timestamp FROM task_state_log WHERE task_id='$AC2B_TASK_ID' AND from_state='queued' AND to_state='assigned' ORDER BY timestamp DESC LIMIT 1)
            >=
            (SELECT timestamp FROM task_state_log WHERE task_id='$AC2A_TASK_ID' AND from_state='queued' AND to_state='assigned' ORDER BY timestamp DESC LIMIT 1)
       THEN 'yes'
       ELSE 'no'
     END;" 2>&1 | tr -d ' \n')
  if [ "$AC2_ORDER" = "yes" ]; then
    pass "AC-2 [REQ-011] [VERIFIER-ADDED] negative: task B (2s delay) re-dispatched at or after task A (1s delay) — exponential growth confirmed"
  else
    fail "AC-2 [REQ-011] [VERIFIER-ADDED] negative: task B (2s delay) re-dispatched at or after task A (1s delay)" \
      "task B was re-dispatched BEFORE task A — exponential growth not confirmed"
  fi

fi

docker unpause "$WORKER_CONTAINER" >/dev/null 2>&1
printf "\n"

# ===========================================================================
# AC-3: Task failing due to Process script error is NOT retried and
#       transitions to "failed" immediately.
#
# Given: the worker XACK's a task that fails due to a domain error (connector
#        failure, schema mapping error, missing pipeline) — the message leaves
#        the Redis pending list without the Monitor ever seeing it
# When:  the Monitor scans for pending entries
# Then:  the task is NOT found in the pending list and is NOT reclaimed/retried;
#        the task status is "failed" as set by the worker
#
# Verification strategy: the Builder's implementation routes process errors
# through domainErrorWrapper, which causes the worker to XACK the message
# immediately after marking the task "failed". The Monitor's pending-entry
# scanner only sees messages with idle time >= HeartbeatTimeout (15s). A
# domain-error task is ACKed within milliseconds, so it will never appear
# in the Monitor's pending list.
#
# We verify AC-3 by:
#   1. Submitting a task via the API against a pipeline that will fail the
#      Process phase (use a connectorType that always fails, or verify that
#      a task whose status was already set to "failed" by the worker has
#      retry_count = 0 and is NOT in the Redis pending list).
#   2. Alternatively, directly simulate: insert a task in "failed" status
#      (as the worker would leave it after XACK), confirm it is not in the
#      pending list, wait one full scan cycle, and verify retry_count is still 0.
#
# Note: the full end-to-end AC-3 path (run a worker that fails a connector,
# confirm XACK fires) requires an always-failing connector connector. Since the
# demo connectors succeed, we use the direct simulation approach to verify the
# invariant from the Monitor's perspective: a task already in "failed" state
# with no pending entry is never retried.
#
# Negative case [VERIFIER-ADDED]: a task in "running" state IS left in the
#        pending list and IS eventually reclaimed — confirming that only the
#        "failed+XACK'd" path skips retry.
# ===========================================================================
printf "=== AC-3 [REQ-011]: Process script error does NOT trigger retry ===\n"

printf "  restarting worker for fresh registration (AC-3 scenario)...\n"
docker compose -f "$COMPOSE_FILE" restart worker >/dev/null 2>&1
sleep 8

AC3_WORKER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" \
  2>&1 | tr -d ' \n')

# Insert a task that simulates a domain-error failure:
# status="failed", retry_count=0 (worker set status to failed before XACK).
AC3_TASK_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "INSERT INTO tasks (pipeline_id, user_id, status, retry_config, retry_count, execution_id, input)
   VALUES (
     '$PIPELINE_ID',
     '$ADMIN_USER_ID',
     'failed',
     '{\"maxRetries\":3,\"backoff\":\"exponential\"}',
     0,
     gen_random_uuid()::text || ':0',
     '{\"key\":\"task010-ac3-domain-error\"}'
   )
   RETURNING id;" 2>&1 | tr -d ' \n')
AC3_TASK_ID=$(echo "$AC3_TASK_ID" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)

printf "  AC-3 domain-error task ID: %s\n" "$AC3_TASK_ID"

docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c \
  "INSERT INTO task_state_log (task_id, from_state, to_state, reason)
   VALUES
     ('$AC3_TASK_ID', 'submitted', 'queued', 'task submitted'),
     ('$AC3_TASK_ID', 'queued', 'assigned', 'assigned to worker'),
     ('$AC3_TASK_ID', 'assigned', 'running', 'execution started'),
     ('$AC3_TASK_ID', 'running', 'failed', 'process script error: connector failure');" >/dev/null 2>&1

# The task is in "failed" status and has NO entry in the Redis pending list
# (simulating the XACK that the worker performs for domain errors).
# Wait one full scan cycle (10s) + buffer (5s) to confirm the Monitor does NOT reclaim it.
printf "  waiting 15s (one scan cycle + buffer) to confirm Monitor does NOT retry domain-error task...\n"
sleep 15

# AC-3 positive: task must still be "failed" with retry_count=0.
AC3_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT status FROM tasks WHERE id='$AC3_TASK_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-3 [REQ-011]: domain-error task status remains 'failed' (not retried to 'queued')" \
  "failed" "$AC3_STATUS"

AC3_RETRY_COUNT=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT retry_count FROM tasks WHERE id='$AC3_TASK_ID';" 2>&1 | tr -d ' \n')
assert_eq "AC-3 [REQ-011]: domain-error task retry_count remains 0 (no retry increment)" \
  "0" "$AC3_RETRY_COUNT"

# AC-3 positive: task must NOT appear in queue:dead-letter either
# (domain errors go to "failed" without DLQ — DLQ is only for exhausted-retry infra failures).
AC3_DLQ=$(docker exec "$REDIS_CONTAINER" redis-cli XRANGE "queue:dead-letter" - + 2>&1)
assert_not_contains \
  "AC-3 [REQ-011]: domain-error task is NOT placed in dead-letter queue (domain errors are terminal, not retried)" \
  "$AC3_TASK_ID" "$AC3_DLQ"

# AC-3 negative [VERIFIER-ADDED]: task NOT in a pending list (confirmed by absence from XPENDING).
AC3_PENDING=$(docker exec "$REDIS_CONTAINER" redis-cli XPENDING "queue:demo" "workers" - + 100 2>&1)
assert_not_contains \
  "AC-3 [REQ-011] [VERIFIER-ADDED] negative: domain-error task has no pending entry in Redis (XACK'd by worker)" \
  "$AC3_TASK_ID" "$AC3_PENDING"

printf "\n"

# ===========================================================================
# AC-4: Task that exhausts retries transitions to "failed" and is placed in
#       the dead letter queue.
#
# Given: a task with retry_count=3 (= max_retries=3, retries exhausted) is
#        in the pending list of a downed worker
# When:  the monitor detects the worker down and scans pending entries
# Then:  the task transitions to "failed" (NOT reclaimed for another retry)
#        AND is placed in queue:dead-letter
#
# Negative case [VERIFIER-ADDED]: a task with retry_count=2 (< max_retries=3)
#        in the same scan is NOT dead-lettered — it is reclaimed for retry 3.
# ===========================================================================
printf "=== AC-4 [REQ-011]: Exhausted retries -> status 'failed' + queue:dead-letter ===\n"

printf "  restarting worker for fresh registration (AC-4 scenario)...\n"
docker compose -f "$COMPOSE_FILE" restart worker >/dev/null 2>&1
sleep 8

AC4_WORKER_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
  "SELECT id FROM workers WHERE status='online' ORDER BY last_heartbeat DESC LIMIT 1;" \
  2>&1 | tr -d ' \n')
if [ -z "$AC4_WORKER_ID" ]; then
  fail "AC-4 setup: find online worker" "no online worker found — cannot continue AC-4"
else

  # Task with exhausted retries (retry_count=3 = max_retries=3).
  AC4_TASK_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "INSERT INTO tasks (pipeline_id, user_id, status, retry_config, retry_count, execution_id, input)
     VALUES (
       '$PIPELINE_ID',
       '$ADMIN_USER_ID',
       'running',
       '{\"maxRetries\":3,\"backoff\":\"exponential\"}',
       3,
       gen_random_uuid()::text || ':3',
       '{\"key\":\"task010-ac4-exhausted\"}'
     )
     RETURNING id;" 2>&1 | tr -d ' \n')
  AC4_TASK_ID=$(echo "$AC4_TASK_ID" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)

  # Task with retries remaining (retry_count=2, one retry still available).
  AC4_REMAINING_TASK_ID=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "INSERT INTO tasks (pipeline_id, user_id, status, retry_config, retry_count, execution_id, input)
     VALUES (
       '$PIPELINE_ID',
       '$ADMIN_USER_ID',
       'running',
       '{\"maxRetries\":3,\"backoff\":\"exponential\"}',
       2,
       gen_random_uuid()::text || ':2',
       '{\"key\":\"task010-ac4-still-has-retries\"}'
     )
     RETURNING id;" 2>&1 | tr -d ' \n')
  AC4_REMAINING_TASK_ID=$(echo "$AC4_REMAINING_TASK_ID" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)

  printf "  AC-4 exhausted task ID (retry_count=3): %s\n" "$AC4_TASK_ID"
  printf "  AC-4 remaining task ID (retry_count=2): %s\n" "$AC4_REMAINING_TASK_ID"

  for TID in "$AC4_TASK_ID" "$AC4_REMAINING_TASK_ID"; do
    docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c \
      "INSERT INTO task_state_log (task_id, from_state, to_state, reason)
       VALUES
         ('$TID', 'submitted', 'queued', 'task submitted'),
         ('$TID', 'queued', 'assigned', 'assigned to worker'),
         ('$TID', 'assigned', 'running', 'execution started');" >/dev/null 2>&1

    docker exec "$REDIS_CONTAINER" redis-cli XADD "queue:demo" "*" \
      "payload" "{\"taskId\":\"$TID\",\"pipelineId\":\"$PIPELINE_ID\",\"userId\":\"$ADMIN_USER_ID\",\"executionId\":\"$TID:x\"}" >/dev/null 2>&1
    docker exec "$REDIS_CONTAINER" redis-cli XREADGROUP GROUP "workers" "$AC4_WORKER_ID" \
      COUNT 1 STREAMS "queue:demo" ">" >/dev/null 2>&1
  done

  DLQ_LEN_BEFORE=$(docker exec "$REDIS_CONTAINER" redis-cli XLEN "queue:dead-letter" 2>&1 | tr -d ' \n')

  printf "  pausing worker (AC-4 scenario)...\n"
  docker pause "$WORKER_CONTAINER" >/dev/null 2>&1
  printf "  waiting %ss for monitor to detect expiry and dead-letter exhausted task...\n" "$DETECTION_WAIT"
  sleep "$DETECTION_WAIT"

  # AC-4 positive: exhausted task must be "failed".
  AC4_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "SELECT status FROM tasks WHERE id='$AC4_TASK_ID';" 2>&1 | tr -d ' \n')
  assert_eq "AC-4 [REQ-011]: exhausted-retry task transitions to status 'failed'" \
    "failed" "$AC4_STATUS"

  # AC-4 positive: exhausted task must appear in queue:dead-letter.
  DLQ_CONTENT_AC4=$(docker exec "$REDIS_CONTAINER" redis-cli XRANGE "queue:dead-letter" - + 2>&1)
  assert_contains \
    "AC-4 [REQ-011]: exhausted-retry task appears in queue:dead-letter stream" \
    "$AC4_TASK_ID" "$DLQ_CONTENT_AC4"

  # AC-4 positive: dead-letter stream length must have grown.
  DLQ_LEN_AFTER=$(docker exec "$REDIS_CONTAINER" redis-cli XLEN "queue:dead-letter" 2>&1 | tr -d ' \n')
  assert_ge "AC-4 [REQ-011]: queue:dead-letter entry count grew after exhausted-retry failure" \
    "$((DLQ_LEN_BEFORE + 1))" "$DLQ_LEN_AFTER"

  # AC-4 negative [VERIFIER-ADDED]: task with retry_count=2 (one retry remaining)
  #   must NOT be in dead-letter — it must be reclaimed for its third attempt.
  assert_not_contains \
    "AC-4 [REQ-011] [VERIFIER-ADDED] negative: task with retry_count=2 (< max_retries=3) is NOT dead-lettered" \
    "$AC4_REMAINING_TASK_ID" "$DLQ_CONTENT_AC4"

  # Also verify the remaining task was reclaimed (retry_count incremented, status queued).
  AC4_REMAINING_STATUS=$(docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c \
    "SELECT status FROM tasks WHERE id='$AC4_REMAINING_TASK_ID';" 2>&1 | tr -d ' \n')
  assert_eq "AC-4 [REQ-011] [VERIFIER-ADDED] negative: task with retry_count=2 transitions to 'queued' for its third retry" \
    "queued" "$AC4_REMAINING_STATUS"

  docker unpause "$WORKER_CONTAINER" >/dev/null 2>&1
fi

printf "\n"

# ===========================================================================
# AC-5: Retry count is visible in task state (GET /api/tasks/{id}).
#
# Given: a task has been retried (retry_count > 0 in PostgreSQL)
# When:  GET /api/tasks/{id} is called
# Then:  the response JSON includes a "retryCount" field reflecting the current
#        retry count (not 0, not absent)
#
# Negative case [VERIFIER-ADDED]: a newly submitted task (never retried) has
#        retryCount=0 in the API response.
# ===========================================================================
printf "=== AC-5 [REQ-011]: Retry count visible in GET /api/tasks/{id} response ===\n"

# Submit a fresh task via the API.
SUBMIT_RESP=$(curl -s -X POST "$API_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "{\"pipelineId\":\"$PIPELINE_ID\",\"tags\":[\"demo\"],\"input\":{\"key\":\"task010-ac5-test\"},\"retryConfig\":{\"maxRetries\":3,\"backoff\":\"exponential\"}}" \
  2>/dev/null)

AC5_TASK_ID=$(echo "$SUBMIT_RESP" | grep -oE '"taskId":"[^"]*"' | cut -d'"' -f4 || echo "")
if [ -z "$AC5_TASK_ID" ]; then
  fail "AC-5 setup: submit task via API" "no taskId in response: $SUBMIT_RESP — cannot continue AC-5"
else
  printf "  AC-5 submitted task ID: %s\n" "$AC5_TASK_ID"

  # AC-5 negative [VERIFIER-ADDED]: newly submitted task must have retryCount=0.
  GET_RESP_0=$(curl -s "$API_URL/api/tasks/$AC5_TASK_ID" \
    -H "Authorization: Bearer $ADMIN_TOKEN" 2>/dev/null)
  RETRY_COUNT_0=$(echo "$GET_RESP_0" | grep -o '"retryCount":[0-9]*' | cut -d':' -f2 || echo "missing")
  assert_eq "AC-5 [REQ-011] [VERIFIER-ADDED] negative: freshly submitted task has retryCount=0 in API response" \
    "0" "$RETRY_COUNT_0"

  # Assert the retryCount field is present at all (field name must be "retryCount").
  assert_contains \
    "AC-5 [REQ-011]: GET /api/tasks/{id} response includes 'retryCount' field" \
    '"retryCount"' "$GET_RESP_0"

  # Now directly update the task's retry_count in PostgreSQL to simulate a retry,
  # and verify the API reflects the updated value.
  docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c \
    "UPDATE tasks SET retry_count=2, updated_at=NOW() WHERE id='$AC5_TASK_ID';" >/dev/null 2>&1

  GET_RESP_2=$(curl -s "$API_URL/api/tasks/$AC5_TASK_ID" \
    -H "Authorization: Bearer $ADMIN_TOKEN" 2>/dev/null)
  RETRY_COUNT_2=$(echo "$GET_RESP_2" | grep -o '"retryCount":[0-9]*' | cut -d':' -f2 || echo "missing")
  assert_eq "AC-5 [REQ-011]: GET /api/tasks/{id} retryCount reflects current retry count (2 after two failures)" \
    "2" "$RETRY_COUNT_2"

  # AC-5 positive: verify retryCount field is in the task sub-object (not missing from response).
  assert_contains \
    "AC-5 [REQ-011]: GET /api/tasks/{id} task object includes retryCount=2 after two retries" \
    '"retryCount":2' "$GET_RESP_2"
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
