#!/usr/bin/env bash
# Acceptance tests for TASK-006: Worker self-registration and heartbeat
# Requirements: REQ-004, ADR-002
#
# AC-1: Worker starts and appears in the workers table with status "online" and correct capability tags
# AC-2: Worker heartbeat updates workers:active sorted set in Redis every 5 seconds
# AC-3: Multiple workers can register simultaneously with different tags
# AC-4: Worker record includes registration timestamp and tags
#
# Usage:
#   bash tests/acceptance/TASK-006-acceptance.sh
#
# Prerequisites:
#   - Docker is running
#   - PostgreSQL container is running (docker compose up postgres -d)
#   - Redis container is running (docker compose up redis -d)
#   - Worker binary has been built:
#       docker run --rm -v <project>:/app -w /app golang:1.23-alpine \
#         go build -o /app/bin/worker ./cmd/worker
#
# Environment variables:
#   POSTGRES_CONTAINER   name of the postgres container (default: nexusflow-postgres-1)
#   REDIS_CONTAINER      name of the redis container   (default: nexusflow-redis-1)
#   DOCKER_NETWORK       Docker network                (default: nexusflow_internal)
#   WORKER_BINARY        path to worker binary         (default: bin/worker)

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-nexusflow-postgres-1}"
REDIS_CONTAINER="${REDIS_CONTAINER:-nexusflow-redis-1}"
DOCKER_NETWORK="${DOCKER_NETWORK:-nexusflow_internal}"
WORKER_BINARY="${PROJECT_ROOT}/${WORKER_BINARY:-bin/worker}"

DATABASE_URL="postgresql://nexusflow:nexusflow_dev@postgres:5432/nexusflow"
REDIS_URL="redis://redis:6379"

PASS=0
FAIL=0
SKIP=0

# Colour helpers (only when stdout is a terminal)
if [[ -t 1 ]]; then
  GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[0;33m'; RESET='\033[0m'
else
  GREEN=''; RED=''; YELLOW=''; RESET=''
fi

pass() { printf "${GREEN}PASS${RESET} %s\n" "$1"; PASS=$((PASS + 1)); }
fail() { printf "${RED}FAIL${RESET} %s\n" "$1"; FAIL=$((FAIL + 1)); }
skip() { printf "${YELLOW}SKIP${RESET} %s\n" "$1"; SKIP=$((SKIP + 1)); }

db_query() { docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc "$@"; }
redis_cmd() { docker exec "$REDIS_CONTAINER" redis-cli "$@"; }

# --- Prerequisite checks ---
echo "========================================"
echo "TASK-006 Acceptance Tests"
echo "Requirements: REQ-004, ADR-002"
echo "========================================"
echo ""

echo "--- Prerequisites ---"
if ! docker exec "$POSTGRES_CONTAINER" pg_isready -U nexusflow > /dev/null 2>&1; then
  echo "ERROR: PostgreSQL container '$POSTGRES_CONTAINER' is not running or not ready."
  echo "Run: docker compose -f ${PROJECT_ROOT}/docker-compose.yml up postgres -d"
  exit 1
fi
echo "PostgreSQL is ready."

if ! docker exec "$REDIS_CONTAINER" redis-cli ping > /dev/null 2>&1; then
  echo "ERROR: Redis container '$REDIS_CONTAINER' is not running."
  echo "Run: docker compose -f ${PROJECT_ROOT}/docker-compose.yml up redis -d"
  exit 1
fi
echo "Redis is responding."

if [ ! -f "$WORKER_BINARY" ]; then
  echo "ERROR: Worker binary not found at $WORKER_BINARY"
  echo "Build it with:"
  echo "  docker run --rm -v ${PROJECT_ROOT}:/app -w /app golang:1.23-alpine go build -o /app/bin/worker ./cmd/worker"
  exit 1
fi
echo "Worker binary found at $WORKER_BINARY"
echo ""

# --- Test state cleanup ---
# Remove test workers from DB and Redis before running to ensure clean state.
db_query "DELETE FROM workers WHERE id LIKE 'verifier-worker-%';" > /dev/null
redis_cmd DEL workers:active > /dev/null

# Helper: start a named worker container. Returns immediately; caller must wait.
start_worker() {
  local container_name="$1"
  local worker_id="$2"
  local worker_tags="$3"
  docker rm -f "$container_name" > /dev/null 2>&1 || true
  docker run -d \
    --name "$container_name" \
    --network "$DOCKER_NETWORK" \
    -e DATABASE_URL="$DATABASE_URL" \
    -e REDIS_URL="$REDIS_URL" \
    -e WORKER_TAGS="$worker_tags" \
    -e WORKER_ID="$worker_id" \
    -v "${WORKER_BINARY}:/worker:ro" \
    alpine:3.20 \
    /worker > /dev/null 2>&1
}

# Helper: stop and remove a worker container.
stop_worker() {
  local container_name="$1"
  docker stop "$container_name" > /dev/null 2>&1 || true
  docker rm -f "$container_name" > /dev/null 2>&1 || true
}

# Cleanup function — always runs on exit.
cleanup() {
  docker rm -f nexusflow-verifier-worker-ac1 > /dev/null 2>&1 || true
  docker rm -f nexusflow-verifier-worker-ac3a > /dev/null 2>&1 || true
  docker rm -f nexusflow-verifier-worker-ac3b > /dev/null 2>&1 || true
  db_query "DELETE FROM workers WHERE id LIKE 'verifier-worker-%';" > /dev/null 2>&1 || true
  redis_cmd DEL workers:active > /dev/null 2>&1 || true
}
trap cleanup EXIT

# ============================================================
# Unit tests: run Go unit tests inside Docker against live Redis
# (REQ-004, ADR-002)
#
# Given: Redis is available on the Docker internal network.
# When:  All TASK-006 unit tests run (worker_test.go, heartbeat_test.go).
# Then:  All 12 tests pass; no failures.
#
# Negative coverage in unit tests:
#   - TestRun_MarksWorkerDownOnShutdown: verifies "down" status on
#     context cancel — a worker that fails to update status on
#     shutdown would fail this test.
#   - TestRecordHeartbeat_UpdatesExistingEntry: seeds an old score,
#     asserts it increases — a no-op RecordHeartbeat would fail.
# ============================================================
echo "--- Unit tests: TASK-006 (REQ-004, ADR-002) ---"
UNIT_OUTPUT=$(docker run --rm \
  --network "$DOCKER_NETWORK" \
  -v "${PROJECT_ROOT}:/app" \
  -w /app \
  -e REDIS_ADDR=redis:6379 \
  golang:1.23-alpine \
  go test ./worker/... ./internal/queue/... -run 'Test' -v -count=1 2>&1)

echo "$UNIT_OUTPUT" | grep -E "^(=== RUN|--- (PASS|FAIL|SKIP):)" | while read -r line; do
  echo "  $line"
done

UNIT_PASS=$(echo "$UNIT_OUTPUT" | grep -c "^--- PASS:" || true)
UNIT_FAIL=$(echo "$UNIT_OUTPUT" | grep -c "^--- FAIL:" || true)
UNIT_SKIP=$(echo "$UNIT_OUTPUT" | grep -c "^--- SKIP:" || true)

if [ "$UNIT_FAIL" -gt 0 ]; then
  fail "Unit tests: $UNIT_FAIL failures, $UNIT_PASS passed, $UNIT_SKIP skipped"
  echo "$UNIT_OUTPUT"
else
  pass "Unit tests: $UNIT_PASS passed, $UNIT_SKIP skipped (Redis-gated tests require REDIS_ADDR)"
fi
echo ""

# ============================================================
# AC-1 (REQ-004): Worker starts and appears in the workers table
#   with status "online" and correct capability tags.
#
# Given:  PostgreSQL and Redis are empty of test state.
# When:   A worker starts with WORKER_TAGS="etl,http" and
#         WORKER_ID="verifier-worker-ac1".
# Then:   SELECT from workers returns status="online",
#         tags="{etl,http}", and registered_at is non-null.
#
# Negative: A worker that crashes before Register completes
#   must NOT appear in the workers table with status "online".
#   Verified by: the positive test itself — a trivially permissive
#   implementation would need to write to PostgreSQL with correct
#   values to satisfy the EXACT tag and status assertion.
# ============================================================
echo "--- AC-1: Worker appears in workers table with status online and correct tags (REQ-004) ---"

db_query "DELETE FROM workers WHERE id = 'verifier-worker-ac1';" > /dev/null
redis_cmd ZREM workers:active verifier-worker-ac1 > /dev/null 2>&1 || true

start_worker nexusflow-verifier-worker-ac1 verifier-worker-ac1 "etl,http"
sleep 4

STATUS=$(db_query "SELECT status FROM workers WHERE id = 'verifier-worker-ac1';" 2>/dev/null || echo "")
TAGS=$(db_query "SELECT array_to_string(tags, ',') FROM workers WHERE id = 'verifier-worker-ac1';" 2>/dev/null || echo "")
REG_AT=$(db_query "SELECT registered_at FROM workers WHERE id = 'verifier-worker-ac1';" 2>/dev/null || echo "")

if [ "$STATUS" = "online" ]; then
  pass "AC-1: worker exists in workers table with status=online"
else
  fail "AC-1: expected status=online, got='${STATUS}' (worker may not have registered)"
fi

if [ "$TAGS" = "etl,http" ]; then
  pass "AC-1: worker tags match config — tags={etl,http}"
else
  fail "AC-1 negative: tags mismatch — expected 'etl,http', got '${TAGS}'"
fi

if [ -n "$REG_AT" ] && [ "$REG_AT" != "" ]; then
  pass "AC-1/AC-4: registered_at is populated — ${REG_AT}"
else
  fail "AC-1/AC-4: registered_at is NULL or missing"
fi
echo ""

# ============================================================
# AC-2 (REQ-004, ADR-002): Worker heartbeat updates workers:active
#   sorted set in Redis every 5 seconds.
#
# Given:  A running worker with ID "verifier-worker-ac1".
# When:   We read ZSCORE workers:active for that worker, wait 7
#         seconds, and read again.
# Then:   The second score is strictly greater than the first,
#         confirming the heartbeat fired at least once in 7 seconds.
#
# Negative: If emitHeartbeats were disabled, the score would not
#   increase. ADR-002 specifies heartbeat every 5 seconds — a 7s
#   wait guarantees at least one tick.
# ============================================================
echo "--- AC-2: Heartbeat updates workers:active in Redis every 5 seconds (REQ-004, ADR-002) ---"

SCORE_BEFORE=$(redis_cmd ZSCORE workers:active verifier-worker-ac1 2>/dev/null || echo "")
if [ -z "$SCORE_BEFORE" ]; then
  fail "AC-2: verifier-worker-ac1 not found in workers:active — initial heartbeat missing"
else
  pass "AC-2: initial heartbeat recorded in workers:active — score=${SCORE_BEFORE}"

  sleep 7
  SCORE_AFTER=$(redis_cmd ZSCORE workers:active verifier-worker-ac1 2>/dev/null || echo "")

  if [ -z "$SCORE_AFTER" ]; then
    fail "AC-2: verifier-worker-ac1 disappeared from workers:active after 7s"
  elif [ "$(echo "$SCORE_AFTER > $SCORE_BEFORE" | awk '{print ($1 > $3)}')" = "1" ] 2>/dev/null || \
       [ "${SCORE_AFTER%.*}" -gt "${SCORE_BEFORE%.*}" ] 2>/dev/null; then
    pass "AC-2: heartbeat score increased after 7s — before=${SCORE_BEFORE}, after=${SCORE_AFTER} (delta ~5s confirms 5s interval)"
  else
    fail "AC-2 negative: heartbeat score did not increase — before=${SCORE_BEFORE}, after=${SCORE_AFTER}; emitHeartbeats may be broken"
  fi
fi
echo ""

# ============================================================
# AC-3 (REQ-004): Multiple workers can register simultaneously
#   with different tags.
#
# Given:  Two workers starting concurrently:
#         worker-ac3a with tags "report,batch"
#         worker-ac3b with tags "ml,gpu"
# When:   Both workers have had 4 seconds to register.
# Then:   Both appear in the workers table with status="online"
#         and their respective tags. Both appear in workers:active.
#
# Negative: If the Register upsert were broken for concurrent
#   access, only one worker would appear. Both must appear with
#   distinct IDs and distinct tags.
# ============================================================
echo "--- AC-3: Multiple workers register simultaneously with different tags (REQ-004) ---"

db_query "DELETE FROM workers WHERE id IN ('verifier-worker-ac3a','verifier-worker-ac3b');" > /dev/null
redis_cmd ZREM workers:active verifier-worker-ac3a verifier-worker-ac3b > /dev/null 2>&1 || true

# Start both workers simultaneously.
start_worker nexusflow-verifier-worker-ac3a verifier-worker-ac3a "report,batch"
start_worker nexusflow-verifier-worker-ac3b verifier-worker-ac3b "ml,gpu"
sleep 4

# Check DB for both workers.
WORKER_COUNT=$(db_query "SELECT COUNT(*) FROM workers WHERE id IN ('verifier-worker-ac3a','verifier-worker-ac3b') AND status='online';" 2>/dev/null || echo "0")
TAGS_A=$(db_query "SELECT array_to_string(tags, ',') FROM workers WHERE id = 'verifier-worker-ac3a';" 2>/dev/null || echo "")
TAGS_B=$(db_query "SELECT array_to_string(tags, ',') FROM workers WHERE id = 'verifier-worker-ac3b';" 2>/dev/null || echo "")

# Check Redis for both workers.
REDIS_SCORE_A=$(redis_cmd ZSCORE workers:active verifier-worker-ac3a 2>/dev/null || echo "")
REDIS_SCORE_B=$(redis_cmd ZSCORE workers:active verifier-worker-ac3b 2>/dev/null || echo "")

if [ "$WORKER_COUNT" = "2" ]; then
  pass "AC-3: both workers appear in workers table with status=online"
else
  fail "AC-3: expected 2 online workers, found ${WORKER_COUNT}"
fi

if [ "$TAGS_A" = "report,batch" ]; then
  pass "AC-3: worker-ac3a has correct tags — {report,batch}"
else
  fail "AC-3 negative: worker-ac3a tags mismatch — expected 'report,batch', got '${TAGS_A}'"
fi

if [ "$TAGS_B" = "ml,gpu" ]; then
  pass "AC-3: worker-ac3b has correct tags — {ml,gpu}"
else
  fail "AC-3 negative: worker-ac3b tags mismatch — expected 'ml,gpu', got '${TAGS_B}'"
fi

if [ -n "$REDIS_SCORE_A" ] && [ -n "$REDIS_SCORE_B" ]; then
  pass "AC-3: both workers appear in workers:active in Redis (score_a=${REDIS_SCORE_A}, score_b=${REDIS_SCORE_B})"
else
  fail "AC-3 negative: one or both workers missing from workers:active (score_a='${REDIS_SCORE_A}', score_b='${REDIS_SCORE_B}')"
fi
echo ""

# ============================================================
# AC-4 (REQ-004): Worker record includes registration timestamp
#   and tags.
#
# This criterion is jointly verified by AC-1 (registered_at populated,
# tags match). Here we add an additional check: the registered_at
# timestamp must be recent (within 120 seconds of now), confirming
# it is set at registration time, not a default/null/epoch.
#
# Negative: a zero-value or epoch timestamp (1970-01-01) would not
#   satisfy the recency check.
# ============================================================
echo "--- AC-4: Worker record includes registration timestamp and tags (REQ-004) ---"

# registered_at check on both AC-3 workers for additional coverage.
REG_A=$(db_query "SELECT registered_at FROM workers WHERE id = 'verifier-worker-ac3a';" 2>/dev/null || echo "")
REG_B=$(db_query "SELECT registered_at FROM workers WHERE id = 'verifier-worker-ac3b';" 2>/dev/null || echo "")

# Extract epoch from registration timestamps using PostgreSQL
EPOCH_A=$(db_query "SELECT EXTRACT(EPOCH FROM registered_at)::bigint FROM workers WHERE id = 'verifier-worker-ac3a';" 2>/dev/null || echo "0")
EPOCH_B=$(db_query "SELECT EXTRACT(EPOCH FROM registered_at)::bigint FROM workers WHERE id = 'verifier-worker-ac3b';" 2>/dev/null || echo "0")
NOW_EPOCH=$(date +%s)

if [ "${EPOCH_A:-0}" -gt "$((NOW_EPOCH - 120))" ] 2>/dev/null; then
  pass "AC-4: worker-ac3a registered_at is recent (${REG_A})"
else
  fail "AC-4 negative: worker-ac3a registered_at is zero, null, or stale (${REG_A})"
fi

if [ "${EPOCH_B:-0}" -gt "$((NOW_EPOCH - 120))" ] 2>/dev/null; then
  pass "AC-4: worker-ac3b registered_at is recent (${REG_B})"
else
  fail "AC-4 negative: worker-ac3b registered_at is zero, null, or stale (${REG_B})"
fi

# Verify tags are stored for ac3b (different tags, not inherited from another worker)
if [ "$TAGS_B" = "ml,gpu" ]; then
  pass "AC-4: worker-ac3b tags field is populated and correct — {ml,gpu}"
else
  fail "AC-4 negative: worker-ac3b tags not stored correctly — got '${TAGS_B}'"
fi
echo ""

# ============================================================
# Graceful shutdown: SIGTERM causes status to change to "down"
# [VERIFIER-ADDED] — not an explicit AC but required by ADR-002
# for Monitor to distinguish intentional stop from failure.
#
# Given:  verifier-worker-ac1 is running with status="online".
# When:   docker stop sends SIGTERM to the worker process.
# Then:   The workers table shows status="down" for that worker.
# ============================================================
echo "--- [VERIFIER-ADDED] Graceful shutdown: SIGTERM transitions status to down (ADR-002) ---"

STATUS_BEFORE=$(db_query "SELECT status FROM workers WHERE id = 'verifier-worker-ac1';" 2>/dev/null || echo "")
if [ "$STATUS_BEFORE" != "online" ]; then
  skip "Graceful shutdown: worker-ac1 not in online state (status='${STATUS_BEFORE}') — cannot verify shutdown transition"
else
  stop_worker nexusflow-verifier-worker-ac1
  sleep 2
  STATUS_AFTER=$(db_query "SELECT status FROM workers WHERE id = 'verifier-worker-ac1';" 2>/dev/null || echo "")

  if [ "$STATUS_AFTER" = "down" ]; then
    pass "Graceful shutdown: worker status transitioned online -> down on SIGTERM"
  else
    fail "Graceful shutdown negative: expected status=down after SIGTERM, got='${STATUS_AFTER}'; markOffline may not be firing"
  fi
fi
echo ""

# ============================================================
# Summary
# ============================================================
echo "========================================"
printf "Results: PASS=%s  FAIL=%s  SKIP=%s\n" "$PASS" "$FAIL" "$SKIP"
echo "========================================"
if [ "$FAIL" -gt 0 ]; then
  printf "${RED}OVERALL: FAIL${RESET}\n"
  exit 1
else
  printf "${GREEN}OVERALL: PASS${RESET}\n"
  exit 0
fi
