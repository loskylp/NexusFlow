#!/usr/bin/env bash
# TASK-011 Acceptance Tests — Dead letter queue with cascading cancellation
#
# REQ-012: Failed tasks are routed to a dead letter queue. If the failed task belongs
#   to a pipeline chain, all downstream tasks in the chain are cancelled with reason
#   "upstream task failed".
# REQ-014: Pipeline chains are ordered sequences of pipelines. The chain trigger fires
#   for each completed pipeline step.
#
# Acceptance criteria under test:
#   AC-1: Task exhausting retries appears in queue:dead-letter stream.
#   AC-2: Pipeline chain A→B→C: when task A enters DLQ, tasks B and C are cancelled
#         with reason "upstream task failed".
#   AC-3: Standalone task (not in a chain) enters DLQ without cascading cancellation.
#   AC-4: Dead letter tasks are visible via the task API with status "failed".
#
# AC-1 and AC-4 are also exercised by TASK-009/010; the tests here re-verify them
# in the context of the TASK-011 implementation with the updated NewMonitor signature.
#
# Usage:
#   API_URL=http://localhost:8080 bash tests/acceptance/TASK-011-acceptance.sh
#
# Requires:
#   - curl, jq, docker exec (for psql/redis-cli access)
#   - docker compose stack running: api, worker, monitor, postgres, redis
#   - Services built from TASK-011 code
#
# Services required: all services from docker-compose.yml
#
# Detection timing (ADR-002):
#   HeartbeatTimeout = 15s, PendingScanInterval = 10s.
#   Worst-case detection after worker pause: 15s + 10s = 25s.
#   We wait DETECTION_WAIT (default 30s) with a 5s safety margin.
#
# AC-2 and AC-3 approach:
#   The cascade logic lives entirely in the monitor. Rather than pausing the worker
#   and waiting 30s (which is impractical for every scenario), we exercise AC-2 and
#   AC-3 by injecting task state directly into PostgreSQL to simulate exhausted-retry
#   conditions and then triggering a monitor scan cycle via the detection wait. This
#   mirrors the approach used in TASK-010 acceptance tests for backoff timing.
#
#   Specifically:
#     - For AC-2: Create pipelines and a chain via the API, submit tasks for each
#       pipeline, then manipulate task A's retry_count to max_retries in the DB,
#       pause the worker, and wait for monitor detection.
#     - For AC-3: Same approach for a standalone pipeline (no chain defined).
#   This is the lowest-risk approach that validates the public API surface without
#   depending on precise timing of worker death detection.

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

skip() {
  local name="$1"
  local reason="$2"
  printf "  SKIP: %s — %s\n" "$name" "$reason"
  RESULTS+=("SKIP | $name | $reason")
}

psql_exec() {
  docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -t -c "$1" 2>/dev/null | tr -d ' \n'
}

redis_exec() {
  docker exec "$REDIS_CONTAINER" redis-cli "$@" 2>/dev/null
}

# ── prerequisite check ──────────────────────────────────────────────────────

printf "\n=== TASK-011 Acceptance Tests: Dead letter queue with cascading cancellation ===\n\n"
printf "API_URL=%s\n" "$API_URL"
printf "WORKER_CONTAINER=%s\n" "$WORKER_CONTAINER"
printf "DETECTION_WAIT=%ss\n\n" "$DETECTION_WAIT"

printf "Checking prerequisites...\n"

if ! curl -sf "$API_URL/healthz" > /dev/null 2>&1; then
  printf "ABORT: API not reachable at %s — is the stack up?\n" "$API_URL"
  exit 2
fi

if ! docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -c "SELECT 1" > /dev/null 2>&1; then
  printf "ABORT: PostgreSQL not reachable in container %s\n" "$POSTGRES_CONTAINER"
  exit 2
fi

if ! docker exec "$REDIS_CONTAINER" redis-cli ping > /dev/null 2>&1; then
  printf "ABORT: Redis not reachable in container %s\n" "$REDIS_CONTAINER"
  exit 2
fi

printf "Prerequisites OK\n\n"

# ── helper: obtain auth token ────────────────────────────────────────────────

auth_token() {
  curl -sf -X POST "$API_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin"}' | jq -r '.token // empty'
}

TOKEN=$(auth_token)
if [ -z "$TOKEN" ]; then
  printf "ABORT: Could not obtain auth token — check admin credentials\n"
  exit 2
fi

AUTH_HEADER="Authorization: Bearer $TOKEN"

# ── helper: pause/resume worker ──────────────────────────────────────────────

pause_worker() {
  docker pause "$WORKER_CONTAINER" > /dev/null 2>&1
}

resume_worker() {
  docker unpause "$WORKER_CONTAINER" > /dev/null 2>&1 || true
}

# ── ensure worker is running at the start ────────────────────────────────────

resume_worker

# ── AC-1: Task exhausting retries appears in queue:dead-letter ───────────────
# REQ-012
# Given: a task with max_retries=0 is submitted (no retries allowed) and is assigned
#        to the worker
# When: the worker is paused (infrastructure failure); the monitor detects the worker
#       down and finds RetryCount >= MaxRetries
# Then: the task appears in queue:dead-letter; task status = "failed"

printf "--- AC-1: Task exhausting retries appears in queue:dead-letter ---\n"

PIPELINE_AC1=$(curl -sf -X POST "$API_URL/api/pipelines" \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"name":"ac1-dlq-pipeline","phases":[{"type":"source","config":{"connector":"demo-source"}},{"type":"sink","config":{"connector":"demo-sink"}}]}' \
  | jq -r '.pipeline.id // empty')

if [ -z "$PIPELINE_AC1" ]; then
  fail "AC-1: task-appears-in-dead-letter-stream" "Could not create pipeline for AC-1"
else
  # Submit task with max_retries=0 so that any infrastructure failure immediately dead-letters
  TASK_AC1=$(curl -sf -X POST "$API_URL/api/tasks" \
    -H "$AUTH_HEADER" -H "Content-Type: application/json" \
    -d "{\"pipelineId\":\"$PIPELINE_AC1\",\"tags\":[\"demo\"],\"retryConfig\":{\"maxRetries\":0,\"backoff\":\"exponential\"},\"input\":{}}" \
    | jq -r '.task.id // empty')

  if [ -z "$TASK_AC1" ]; then
    fail "AC-1: task-appears-in-dead-letter-stream" "Could not submit task for AC-1"
  else
    printf "  Submitted task %s (max_retries=0); waiting for 'running' status...\n" "$TASK_AC1"

    # Wait up to 15s for the task to reach 'running' (picked up by worker)
    RUNNING=false
    for _ in $(seq 1 15); do
      STATUS=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_AC1'")
      if [ "$STATUS" = "running" ]; then
        RUNNING=true
        break
      fi
      sleep 1
    done

    if ! $RUNNING; then
      fail "AC-1: task-appears-in-dead-letter-stream" \
        "Task $TASK_AC1 did not reach 'running' within 15s (current: $STATUS) — worker may not be up"
    else
      # Negative case: task is NOT yet in dead-letter before worker failure
      DLQ_BEFORE=$(redis_exec XRANGE queue:dead-letter - + | grep -c "$TASK_AC1" || true)
      if [ "$DLQ_BEFORE" -gt "0" ]; then
        fail "AC-1 [VERIFIER-ADDED negative]: task-not-in-dlq-before-failure" \
          "Task $TASK_AC1 appeared in queue:dead-letter BEFORE worker failure — premature DLQ entry"
      else
        pass "AC-1 [negative]: task-not-in-dlq-before-worker-failure"
      fi

      # Pause the worker to simulate infrastructure failure
      pause_worker
      printf "  Worker paused. Waiting %ss for monitor detection...\n" "$DETECTION_WAIT"
      sleep "$DETECTION_WAIT"

      # Verify task appears in queue:dead-letter
      DLQ_ENTRIES=$(redis_exec XRANGE queue:dead-letter - +)
      if echo "$DLQ_ENTRIES" | grep -q "$TASK_AC1"; then
        pass "AC-1: task-appears-in-dead-letter-stream"
      else
        fail "AC-1: task-appears-in-dead-letter-stream" \
          "Task $TASK_AC1 not found in queue:dead-letter after ${DETECTION_WAIT}s wait"
      fi

      # Verify task status in PostgreSQL
      FINAL_STATUS=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_AC1'")
      if [ "$FINAL_STATUS" = "failed" ]; then
        pass "AC-1: task-status-failed-in-postgres"
      else
        fail "AC-1: task-status-failed-in-postgres" \
          "Task $TASK_AC1 status=$FINAL_STATUS, want 'failed'"
      fi

      resume_worker
    fi
  fi
fi

# ── AC-4: Dead letter tasks visible via task API with status "failed" ────────
# REQ-012
# Uses the task dead-lettered in AC-1 above if it succeeded.
# Given: a task has been dead-lettered (status = "failed" in PostgreSQL, AC-1 above)
# When: GET /api/tasks?status=failed is called
# Then: the task appears in the response body with status "failed"

printf "\n--- AC-4: Dead letter tasks visible via task API with status failed ---\n"

if [ -z "${TASK_AC1:-}" ]; then
  skip "AC-4: dead-letter-tasks-visible-via-api" "AC-1 setup failed; no dead-lettered task available"
else
  # Positive case: task must appear in ?status=failed
  TASKS_FAILED=$(curl -sf "$API_URL/api/tasks?status=failed" \
    -H "$AUTH_HEADER" | jq -r '[.tasks[].id] | .[]' 2>/dev/null || true)

  if echo "$TASKS_FAILED" | grep -q "$TASK_AC1"; then
    pass "AC-4: dead-letter-task-visible-via-api-status-failed"
  else
    fail "AC-4: dead-letter-task-visible-via-api-status-failed" \
      "Task $TASK_AC1 not returned by GET /api/tasks?status=failed"
  fi

  # Negative case: task must NOT appear under a different terminal status
  TASKS_COMPLETED=$(curl -sf "$API_URL/api/tasks?status=completed" \
    -H "$AUTH_HEADER" | jq -r '[.tasks[].id] | .[]' 2>/dev/null || true)

  if echo "$TASKS_COMPLETED" | grep -q "$TASK_AC1"; then
    fail "AC-4 [VERIFIER-ADDED negative]: dead-letter-task-not-in-completed" \
      "Task $TASK_AC1 incorrectly appears in GET /api/tasks?status=completed"
  else
    pass "AC-4 [negative]: dead-letter-task-not-visible-as-completed"
  fi

  # Negative case: individual GET must return status "failed"
  SINGLE_TASK_STATUS=$(curl -sf "$API_URL/api/tasks/$TASK_AC1" \
    -H "$AUTH_HEADER" | jq -r '.task.status // empty')
  if [ "$SINGLE_TASK_STATUS" = "failed" ]; then
    pass "AC-4: GET /api/tasks/{id} returns status failed for dead-lettered task"
  else
    fail "AC-4: GET /api/tasks/{id} returns status failed" \
      "GET /api/tasks/$TASK_AC1 returned status=$SINGLE_TASK_STATUS, want 'failed'"
  fi
fi

# ── AC-2: Chain A→B→C — dead-lettering A cancels B and C ────────────────────
# REQ-012, REQ-014
# Given: pipelines A, B, C and a chain A→B→C are created; tasks are submitted for
#        each pipeline; all tasks reach non-terminal states
# When: task A exhausts retries and is dead-lettered by the monitor
# Then: tasks B and C are cancelled with reason "upstream task failed"
# Negative: upstream/completed tasks are not touched; a task in pipeline D
#           (separate chain) is not cancelled

printf "\n--- AC-2: Chain A→B→C cascade cancellation ---\n"

resume_worker

# Create three pipelines
PIPELINE_A=$(curl -sf -X POST "$API_URL/api/pipelines" \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"name":"ac2-chain-pipeline-a","phases":[{"type":"source","config":{"connector":"demo-source"}},{"type":"sink","config":{"connector":"demo-sink"}}]}' \
  | jq -r '.pipeline.id // empty')

PIPELINE_B=$(curl -sf -X POST "$API_URL/api/pipelines" \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"name":"ac2-chain-pipeline-b","phases":[{"type":"source","config":{"connector":"demo-source"}},{"type":"sink","config":{"connector":"demo-sink"}}]}' \
  | jq -r '.pipeline.id // empty')

PIPELINE_C=$(curl -sf -X POST "$API_URL/api/pipelines" \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"name":"ac2-chain-pipeline-c","phases":[{"type":"source","config":{"connector":"demo-source"}},{"type":"sink","config":{"connector":"demo-sink"}}]}' \
  | jq -r '.pipeline.id // empty')

# An unrelated pipeline in a separate chain (used for negative test)
PIPELINE_D=$(curl -sf -X POST "$API_URL/api/pipelines" \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"name":"ac2-unrelated-pipeline-d","phases":[{"type":"source","config":{"connector":"demo-source"}},{"type":"sink","config":{"connector":"demo-sink"}}]}' \
  | jq -r '.pipeline.id // empty')

if [ -z "$PIPELINE_A" ] || [ -z "$PIPELINE_B" ] || [ -z "$PIPELINE_C" ] || [ -z "$PIPELINE_D" ]; then
  fail "AC-2: pipeline-chain-cascade-cancellation" "Failed to create one or more pipelines for AC-2 chain test"
else
  # Create the chain A→B→C
  CHAIN_ID=$(curl -sf -X POST "$API_URL/api/chains" \
    -H "$AUTH_HEADER" -H "Content-Type: application/json" \
    -d "{\"name\":\"ac2-test-chain\",\"pipelineIds\":[\"$PIPELINE_A\",\"$PIPELINE_B\",\"$PIPELINE_C\"]}" \
    | jq -r '.chain.id // empty')

  if [ -z "$CHAIN_ID" ]; then
    fail "AC-2: pipeline-chain-cascade-cancellation" "Failed to create chain A→B→C via POST /api/chains"
  else
    # Submit task A with max_retries=0 (will be immediately dead-lettered on failure)
    TASK_A=$(curl -sf -X POST "$API_URL/api/tasks" \
      -H "$AUTH_HEADER" -H "Content-Type: application/json" \
      -d "{\"pipelineId\":\"$PIPELINE_A\",\"tags\":[\"demo\"],\"retryConfig\":{\"maxRetries\":0,\"backoff\":\"exponential\"},\"input\":{}}" \
      | jq -r '.task.id // empty')

    # Submit tasks B and C directly (simulating chain trigger having fired)
    TASK_B=$(curl -sf -X POST "$API_URL/api/tasks" \
      -H "$AUTH_HEADER" -H "Content-Type: application/json" \
      -d "{\"pipelineId\":\"$PIPELINE_B\",\"tags\":[\"demo\"],\"retryConfig\":{\"maxRetries\":3,\"backoff\":\"exponential\"},\"input\":{}}" \
      | jq -r '.task.id // empty')

    TASK_C=$(curl -sf -X POST "$API_URL/api/tasks" \
      -H "$AUTH_HEADER" -H "Content-Type: application/json" \
      -d "{\"pipelineId\":\"$PIPELINE_C\",\"tags\":[\"demo\"],\"retryConfig\":{\"maxRetries\":3,\"backoff\":\"exponential\"},\"input\":{}}" \
      | jq -r '.task.id // empty')

    # Submit task D (unrelated) — must NOT be cancelled
    TASK_D=$(curl -sf -X POST "$API_URL/api/tasks" \
      -H "$AUTH_HEADER" -H "Content-Type: application/json" \
      -d "{\"pipelineId\":\"$PIPELINE_D\",\"tags\":[\"demo\"],\"retryConfig\":{\"maxRetries\":3,\"backoff\":\"exponential\"},\"input\":{}}" \
      | jq -r '.task.id // empty')

    if [ -z "$TASK_A" ] || [ -z "$TASK_B" ] || [ -z "$TASK_C" ] || [ -z "$TASK_D" ]; then
      fail "AC-2: pipeline-chain-cascade-cancellation" "Failed to submit one or more tasks for AC-2 test"
    else
      printf "  Tasks: A=%s B=%s C=%s D=%s\n" "$TASK_A" "$TASK_B" "$TASK_C" "$TASK_D"
      printf "  Waiting for task A to reach 'running' status...\n"

      RUNNING_A=false
      for _ in $(seq 1 15); do
        STATUS_A=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_A'")
        if [ "$STATUS_A" = "running" ]; then
          RUNNING_A=true
          break
        fi
        sleep 1
      done

      if ! $RUNNING_A; then
        fail "AC-2: pipeline-chain-cascade-cancellation" \
          "Task A $TASK_A did not reach 'running' within 15s (status: $STATUS_A)"
      else
        # Record pre-condition: B and C are non-terminal before worker failure
        STATUS_B_BEFORE=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_B'")
        STATUS_C_BEFORE=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_C'")
        printf "  Pre-pause: B=%s C=%s\n" "$STATUS_B_BEFORE" "$STATUS_C_BEFORE"

        # Negative: before worker failure, no downstream cancellation yet
        if [ "$STATUS_B_BEFORE" = "cancelled" ] || [ "$STATUS_C_BEFORE" = "cancelled" ]; then
          fail "AC-2 [VERIFIER-ADDED negative]: downstream-not-cancelled-before-failure" \
            "Tasks B or C are already cancelled before task A fails — premature cascade"
        else
          pass "AC-2 [negative]: downstream-tasks-not-cancelled-before-upstream-failure"
        fi

        # Pause worker to trigger dead-letter via infrastructure failure
        pause_worker
        printf "  Worker paused. Waiting %ss for monitor detection and cascade...\n" "$DETECTION_WAIT"
        sleep "$DETECTION_WAIT"

        STATUS_A_AFTER=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_A'")
        STATUS_B_AFTER=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_B'")
        STATUS_C_AFTER=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_C'")
        STATUS_D_AFTER=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_D'")
        REASON_B=$(psql_exec "SELECT reason FROM task_state_log WHERE task_id='$TASK_B' AND to_status='cancelled' ORDER BY created_at DESC LIMIT 1")
        REASON_C=$(psql_exec "SELECT reason FROM task_state_log WHERE task_id='$TASK_C' AND to_status='cancelled' ORDER BY created_at DESC LIMIT 1")

        printf "  Post-wait: A=%s B=%s C=%s D=%s\n" "$STATUS_A_AFTER" "$STATUS_B_AFTER" "$STATUS_C_AFTER" "$STATUS_D_AFTER"
        printf "  Cancellation reasons: B=%s C=%s\n" "$REASON_B" "$REASON_C"

        # AC-2 positive: B and C must be cancelled
        if [ "$STATUS_B_AFTER" = "cancelled" ] && [ "$STATUS_C_AFTER" = "cancelled" ]; then
          pass "AC-2: downstream-tasks-B-and-C-cancelled"
        else
          fail "AC-2: downstream-tasks-B-and-C-cancelled" \
            "Expected B=cancelled C=cancelled; got B=$STATUS_B_AFTER C=$STATUS_C_AFTER"
        fi

        # AC-2 positive: reason must be "upstream task failed"
        if [ "$REASON_B" = "upstream task failed" ] && [ "$REASON_C" = "upstream task failed" ]; then
          pass "AC-2: cancellation-reason-is-upstream-task-failed"
        else
          fail "AC-2: cancellation-reason-is-upstream-task-failed" \
            "Expected reason='upstream task failed'; got B='$REASON_B' C='$REASON_C'"
        fi

        # AC-2 negative: task D (unrelated pipeline) must NOT be cascade-cancelled
        if [ "$STATUS_D_AFTER" != "cancelled" ]; then
          pass "AC-2 [negative]: unrelated-pipeline-task-not-cascade-cancelled"
        else
          fail "AC-2 [negative]: unrelated-pipeline-task-not-cascade-cancelled" \
            "Task D ($TASK_D) in unrelated pipeline was incorrectly cancelled"
        fi

        resume_worker
      fi
    fi
  fi
fi

# ── AC-3: Standalone task enters DLQ without cascading cancellation ──────────
# REQ-012
# Given: a standalone task (pipeline not in any chain) is submitted and reaches 'running'
# When: the worker is paused; task exhausts retries; monitor dead-letters it
# Then: task appears in queue:dead-letter with status "failed"; no other tasks are cancelled
# Negative: an unrelated queued task in a separate standalone pipeline is not cancelled

printf "\n--- AC-3: Standalone task enters DLQ without cascade ---\n"

resume_worker

PIPELINE_STANDALONE=$(curl -sf -X POST "$API_URL/api/pipelines" \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"name":"ac3-standalone-pipeline","phases":[{"type":"source","config":{"connector":"demo-source"}},{"type":"sink","config":{"connector":"demo-sink"}}]}' \
  | jq -r '.pipeline.id // empty')

PIPELINE_BYSTANDER=$(curl -sf -X POST "$API_URL/api/pipelines" \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"name":"ac3-bystander-pipeline","phases":[{"type":"source","config":{"connector":"demo-source"}},{"type":"sink","config":{"connector":"demo-sink"}}]}' \
  | jq -r '.pipeline.id // empty')

if [ -z "$PIPELINE_STANDALONE" ] || [ -z "$PIPELINE_BYSTANDER" ]; then
  fail "AC-3: standalone-task-dlq-no-cascade" "Failed to create pipelines for AC-3 test"
else
  # Neither pipeline is added to a chain — both are truly standalone

  TASK_STANDALONE=$(curl -sf -X POST "$API_URL/api/tasks" \
    -H "$AUTH_HEADER" -H "Content-Type: application/json" \
    -d "{\"pipelineId\":\"$PIPELINE_STANDALONE\",\"tags\":[\"demo\"],\"retryConfig\":{\"maxRetries\":0,\"backoff\":\"exponential\"},\"input\":{}}" \
    | jq -r '.task.id // empty')

  TASK_BYSTANDER=$(curl -sf -X POST "$API_URL/api/tasks" \
    -H "$AUTH_HEADER" -H "Content-Type: application/json" \
    -d "{\"pipelineId\":\"$PIPELINE_BYSTANDER\",\"tags\":[\"demo\"],\"retryConfig\":{\"maxRetries\":3,\"backoff\":\"exponential\"},\"input\":{}}" \
    | jq -r '.task.id // empty')

  if [ -z "$TASK_STANDALONE" ] || [ -z "$TASK_BYSTANDER" ]; then
    fail "AC-3: standalone-task-dlq-no-cascade" "Failed to submit tasks for AC-3"
  else
    printf "  Tasks: standalone=%s bystander=%s\n" "$TASK_STANDALONE" "$TASK_BYSTANDER"
    printf "  Waiting for standalone task to reach 'running'...\n"

    RUNNING_SA=false
    for _ in $(seq 1 15); do
      STATUS_SA=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_STANDALONE'")
      if [ "$STATUS_SA" = "running" ]; then
        RUNNING_SA=true
        break
      fi
      sleep 1
    done

    if ! $RUNNING_SA; then
      fail "AC-3: standalone-task-dlq-no-cascade" \
        "Standalone task $TASK_STANDALONE did not reach 'running' (status: $STATUS_SA)"
    else
      pause_worker
      printf "  Worker paused. Waiting %ss for monitor detection...\n" "$DETECTION_WAIT"
      sleep "$DETECTION_WAIT"

      STATUS_SA_AFTER=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_STANDALONE'")
      STATUS_BY_AFTER=$(psql_exec "SELECT status FROM tasks WHERE id='$TASK_BYSTANDER'")
      DLQ_SA=$(redis_exec XRANGE queue:dead-letter - + | grep -c "$TASK_STANDALONE" || true)

      printf "  Post-wait: standalone=%s bystander=%s dlq_entries=%s\n" \
        "$STATUS_SA_AFTER" "$STATUS_BY_AFTER" "$DLQ_SA"

      # AC-3 positive: standalone task must be in DLQ with status "failed"
      if [ "$STATUS_SA_AFTER" = "failed" ]; then
        pass "AC-3: standalone-task-status-failed"
      else
        fail "AC-3: standalone-task-status-failed" \
          "Standalone task status=$STATUS_SA_AFTER, want 'failed'"
      fi

      if [ "$DLQ_SA" -gt "0" ]; then
        pass "AC-3: standalone-task-appears-in-dead-letter-stream"
      else
        fail "AC-3: standalone-task-appears-in-dead-letter-stream" \
          "Standalone task $TASK_STANDALONE not found in queue:dead-letter"
      fi

      # AC-3 negative: bystander task in a separate standalone pipeline must NOT be cancelled
      if [ "$STATUS_BY_AFTER" != "cancelled" ]; then
        pass "AC-3 [negative]: bystander-task-not-cancelled"
      else
        fail "AC-3 [negative]: bystander-task-not-cancelled" \
          "Bystander task $TASK_BYSTANDER was incorrectly cancelled — cascade leaked to standalone pipeline"
      fi

      resume_worker
    fi
  fi
fi

# ── summary ──────────────────────────────────────────────────────────────────

printf "\n=== Results ===\n"
for r in "${RESULTS[@]}"; do
  printf "  %s\n" "$r"
done

printf "\n  PASS: %d  FAIL: %d\n" "$PASS" "$FAIL"

if [ "$FAIL" -eq 0 ]; then
  printf "\nOVERALL: PASS\n"
  exit 0
else
  printf "\nOVERALL: FAIL\n"
  exit 1
fi
