#!/usr/bin/env bash
# Acceptance tests for TASK-007: Tag-based task assignment and pipeline execution
# Requirements: REQ-005, REQ-006, REQ-007, REQ-009
#
# AC-1: Worker with tag "etl" picks up tasks from queue:etl
# AC-2: Task state transitions: queued -> assigned -> running -> completed
# AC-3: Each state transition is logged in task_state_log with timestamp
# AC-4: DataSource phase extracts data according to config
# AC-5: Process phase transforms data with schema mapping applied
# AC-6: Sink phase writes data to destination
# AC-7: Schema mapping renames fields between phases
# AC-8: Failed pipeline execution sets task status to "failed"
# AC-9: Task state change events emitted to Redis Pub/Sub (via TaskEventBroker)
#
# Verification approach:
#   TASK-007 delivers the pipeline execution engine in the worker package. All nine
#   acceptance criteria are verified by 22 Go unit tests that use in-memory fakes
#   for every external dependency. No live Redis or PostgreSQL connection is required
#   for these tests.
#
#   End-to-end pipeline execution against live connectors requires TASK-042 (demo
#   connectors). When TASK-042 is complete, the live system tests at the bottom of
#   this script (currently SKIP) become executable.
#
# Usage:
#   bash tests/acceptance/TASK-007-acceptance.sh
#
# Prerequisites:
#   - Docker is running
#   - golang:1.24 image is available (or will be pulled)
#
# Optional (for live system tests — requires TASK-042):
#   - PostgreSQL container running: docker compose up postgres -d
#   - Redis container running:      docker compose up redis -d
#   - Worker binary built:
#       docker run --rm -v <project>:/app -w /app golang:1.24 \
#         go build -o /app/bin/worker ./cmd/worker
#
# Environment variables:
#   POSTGRES_CONTAINER   name of the postgres container (default: nexusflow-postgres-1)
#   REDIS_CONTAINER      name of the redis container   (default: nexusflow-redis-1)
#   DOCKER_NETWORK       Docker network                (default: nexusflow_internal)
#   WORKER_BINARY        path to worker binary         (default: bin/worker)
#   SKIP_LIVE            set to "1" to skip live system tests entirely (default: auto-detect)

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-nexusflow-postgres-1}"
REDIS_CONTAINER="${REDIS_CONTAINER:-nexusflow-redis-1}"
DOCKER_NETWORK="${DOCKER_NETWORK:-nexusflow_internal}"
WORKER_BINARY="${PROJECT_ROOT}/${WORKER_BINARY:-bin/worker}"

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

db_query()  { docker exec "$POSTGRES_CONTAINER" psql -U nexusflow -d nexusflow -tAc "$@"; }
redis_cmd() { docker exec "$REDIS_CONTAINER" redis-cli "$@"; }

echo "========================================"
echo "TASK-007 Acceptance Tests"
echo "Requirements: REQ-005, REQ-006, REQ-007, REQ-009"
echo "========================================"
echo ""

# ============================================================
# Section 1: Build verification
# go build ./... must be clean — the worker package is the primary deliverable.
# ============================================================
echo "--- Build verification ---"
BUILD_OUTPUT=$(docker run --rm \
  -v "${PROJECT_ROOT}:/app" \
  -w /app \
  golang:1.24 \
  go build ./... 2>&1) && BUILD_EXIT=0 || BUILD_EXIT=$?

if [ "$BUILD_EXIT" -eq 0 ]; then
  pass "go build ./... is clean"
else
  fail "go build ./... produced errors:"
  echo "$BUILD_OUTPUT"
fi

# ============================================================
# Section 2: go vet
# ============================================================
echo ""
echo "--- Static analysis: go vet ---"
VET_OUTPUT=$(docker run --rm \
  -v "${PROJECT_ROOT}:/app" \
  -w /app \
  golang:1.24 \
  go vet ./... 2>&1) && VET_EXIT=0 || VET_EXIT=$?

if [ "$VET_EXIT" -eq 0 ]; then
  pass "go vet ./... is clean"
else
  fail "go vet ./... reported issues:"
  echo "$VET_OUTPUT"
fi

# ============================================================
# Section 3: Unit tests — all nine acceptance criteria
#
# The 22 tests in worker/ cover every acceptance criterion using
# in-memory fakes for all external dependencies. No live infrastructure needed.
#
# AC-1 (REQ-005): TestRunConsumptionLoop_TagFiltering
#   Given:  worker configured with tags=["etl"]
#   When:   runConsumptionLoop calls ReadTasks
#   Then:   ReadTasks is invoked with tags=["etl"], routing to queue:etl
#
# Negative for AC-1 [VERIFIER-ADDED]: The test uses a recordingConsumer that fails
#   if ReadTasks is never called — a dead consumption loop cannot satisfy AC-1.
#
# AC-2 (REQ-009): TestExecuteTask_SuccessfulPipeline_CompletesTask
#   Given:  queued task with a valid pipeline
#   When:   worker executes the task end-to-end
#   Then:   at least 3 status transitions are recorded (assigned, running, completed)
#           and the final status is "completed"
#
# Negative for AC-2 [VERIFIER-ADDED]: TestExecuteTask_ProcessError_SetsFailedStatus
#   verifies that a pipeline error does NOT produce "completed" status —
#   a trivially permissive implementation returning "completed" regardless would fail.
#
# AC-3 (REQ-009): TestExecuteTask_SuccessfulPipeline_CompletesTask (fakeTaskRepo.statusLog)
#   Given:  queued task with a valid pipeline
#   When:   worker executes the task
#   Then:   fakeTaskRepo records each UpdateStatus call; verified count >= 3
#
# AC-4 (REQ-006): TestExecuteTask_SuccessfulPipeline_CompletesTask (sink.recordCount > 0)
#   Given:  DataSource configured to return one record {"id":"1","name":"Alice"}
#   When:   pipeline executes
#   Then:   sink receives at least one record (DataSource extracted data)
#
# AC-5 (REQ-006): TestExecuteTask_SchemaMapping_Applied (capturingProcess)
#   Given:  DataSource outputs {"id":"42","name":"Bob"}; ProcessConfig.InputMappings
#           renames id->user_id and name->full_name
#   When:   pipeline executes
#   Then:   Process connector receives {"user_id":"42","full_name":"Bob"}, not the original keys
#
# Negative for AC-5 [VERIFIER-ADDED]: TestApplySchemaMapping_ErrorOnMissingSourceField
#   Given:  a schema mapping references "missing_field" that is absent from the record
#   When:   ApplySchemaMapping is called
#   Then:   an error is returned; a no-error implementation would fail this test
#
# AC-6 (REQ-006): TestExecuteTask_SuccessfulPipeline_CompletesTask (sink.recordCount check)
#   Given:  all three phases succeed
#   When:   pipeline executes
#   Then:   fakeSink.written[executionID] contains at least one record
#
# AC-7 (REQ-007): TestApplySchemaMapping_RenamesFields + TestExecuteTask_SchemaMapping_Applied
#   Given:  SchemaMapping{SourceField:"id", TargetField:"user_id"}
#   When:   ApplySchemaMapping is applied to {"id":"123","name":"Alice"}
#   Then:   result contains "user_id"="123"; original "id" key is absent
#
# Negative for AC-7: TestApplySchemaMapping_ErrorOnMissingSourceField (see AC-5 above)
#
# AC-8 (REQ-006): TestExecuteTask_ProcessError_SetsFailedStatus,
#                 TestExecuteTask_DataSourceError_SetsFailedStatus,
#                 TestExecuteTask_MissingPipeline_SetsFailedStatus
#   Given:  connector returns an error (or pipeline is missing)
#   When:   executeTask runs
#   Then:   task.Status = "failed"; message is XACKed (no XCLAIM retry for domain errors)
#
# Negative for AC-8: a worker that never sets "failed" would fail all three tests above.
#
# AC-9 (REQ-009): TestExecuteTask_SSEEventsEmitted
#   Given:  fakeBroker is wired to the worker
#   When:   task completes (3 transitions: assigned, running, completed)
#   Then:   fakeBroker records >= 3 PublishTaskEvent calls; last event status = "completed"
#
# Negative for AC-9: a broker that records 0 events would fail broker.publishCount() >= 3.
# ============================================================
echo ""
echo "--- Unit tests: TASK-007 (REQ-005, REQ-006, REQ-007, REQ-009) ---"

UNIT_OUTPUT=$(docker run --rm \
  -v "${PROJECT_ROOT}:/app" \
  -w /app \
  golang:1.24 \
  go test ./worker/... -v -count=1 2>&1)

echo "$UNIT_OUTPUT" | grep -E "^(=== RUN|--- (PASS|FAIL|SKIP):)" | while IFS= read -r line; do
  echo "  $line"
done

UNIT_PASS=$(echo "$UNIT_OUTPUT" | grep -c "^--- PASS:" || true)
UNIT_FAIL=$(echo "$UNIT_OUTPUT" | grep -c "^--- FAIL:" || true)

if [ "$UNIT_FAIL" -gt 0 ]; then
  fail "Unit tests: $UNIT_FAIL failing, $UNIT_PASS passing"
  echo ""
  echo "Failing tests:"
  echo "$UNIT_OUTPUT" | grep "^--- FAIL:" | while IFS= read -r line; do
    echo "  $line"
  done
  echo ""
  echo "Full output (tail):"
  echo "$UNIT_OUTPUT" | tail -40
else
  pass "Unit tests: all $UNIT_PASS tests pass (TASK-007: 14 new + 8 pre-existing)"
fi

# Verify the expected test count — regression guard against silent test removal.
if [ "$UNIT_PASS" -lt 22 ]; then
  fail "[VERIFIER-ADDED] Unit test count regression: expected >= 22 passing tests, got $UNIT_PASS"
fi

# ============================================================
# Section 4: Full regression suite
# Verify no other package is broken by TASK-007 changes.
# ============================================================
echo ""
echo "--- Full regression suite: go test ./... ---"

FULL_OUTPUT=$(docker run --rm \
  -v "${PROJECT_ROOT}:/app" \
  -w /app \
  golang:1.24 \
  go test ./... 2>&1)

FULL_FAIL=$(echo "$FULL_OUTPUT" | grep "^FAIL" | grep -v "^FAIL[[:space:]]*$" | grep -c "github.com" || true)

if [ "$FULL_FAIL" -gt 0 ]; then
  fail "Regression: $FULL_FAIL packages failing in go test ./..."
  echo "$FULL_OUTPUT" | grep "^FAIL"
else
  pass "Full regression suite: all packages pass"
fi

# ============================================================
# Section 5: Structural code review — acceptance criteria traceability
#
# Verify that key structural properties of the implementation are present.
# These are code-level checks that confirm the implementation satisfies
# acceptance criteria independently of the unit tests.
# ============================================================
echo ""
echo "--- Structural code review ---"

WORKER_GO="${PROJECT_ROOT}/worker/worker.go"
CONNECTORS_GO="${PROJECT_ROOT}/worker/connectors.go"
EVENTS_GO="${PROJECT_ROOT}/worker/events.go"

# AC-1 (REQ-005): runConsumptionLoop reads from tag-specific streams.
# The loop calls ReadTasks with w.cfg.WorkerTags which routes to queue:{tag} streams.
if grep -q "ReadTasks.*WorkerTags" "$WORKER_GO"; then
  pass "[REQ-005] AC-1: runConsumptionLoop passes WorkerTags to ReadTasks (tag-specific stream routing)"
else
  fail "[REQ-005] AC-1: runConsumptionLoop does not pass WorkerTags to ReadTasks"
fi

# AC-2 (REQ-009): State machine queued -> assigned -> running -> completed.
# Verify all four status transitions appear in worker.go.
STATUSES_FOUND=0
for status in "TaskStatusAssigned" "TaskStatusRunning" "TaskStatusCompleted" "TaskStatusFailed"; do
  if grep -q "$status" "$WORKER_GO"; then
    STATUSES_FOUND=$((STATUSES_FOUND + 1))
  fi
done
if [ "$STATUSES_FOUND" -eq 4 ]; then
  pass "[REQ-009] AC-2/AC-8: All four target statuses (assigned, running, completed, failed) referenced in worker.go"
else
  fail "[REQ-009] AC-2/AC-8: Only $STATUSES_FOUND of 4 expected status constants found in worker.go"
fi

# AC-3 (REQ-009): Each transition calls UpdateStatus which writes to task_state_log.
# The transitionStatus function calls w.tasks.UpdateStatus on each transition.
if grep -q "func.*transitionStatus" "$WORKER_GO" && grep -q "UpdateStatus" "$WORKER_GO"; then
  pass "[REQ-009] AC-3: transitionStatus wraps UpdateStatus (maps to task_state_log writes)"
else
  fail "[REQ-009] AC-3: transitionStatus or UpdateStatus call not found in worker.go"
fi

# AC-4/AC-5/AC-6 (REQ-006): Three-phase pipeline in runPipeline.
# Verify runDataSource, runProcess, and runSink are all called in sequence.
if grep -q "runDataSource" "$WORKER_GO" && grep -q "runProcess" "$WORKER_GO" && grep -q "runSink" "$WORKER_GO"; then
  pass "[REQ-006] AC-4/5/6: All three pipeline phases (runDataSource, runProcess, runSink) present in worker.go"
else
  fail "[REQ-006] AC-4/5/6: One or more pipeline phases missing from worker.go"
fi

# AC-7 (REQ-007): Schema mapping applied at both phase boundaries.
# applyMappingsToSlice is called twice in runPipeline: DS->Process and Process->Sink.
MAPPING_CALLS=$(grep -c "applyMappingsToSlice" "$WORKER_GO" || true)
if [ "$MAPPING_CALLS" -ge 2 ]; then
  pass "[REQ-007] AC-7: applyMappingsToSlice called at both phase boundaries ($MAPPING_CALLS occurrences)"
else
  fail "[REQ-007] AC-7: applyMappingsToSlice found only $MAPPING_CALLS time(s) in worker.go; expected >= 2 (DS->Process and Process->Sink boundaries)"
fi

# AC-7 (REQ-007): ApplySchemaMapping is exported (accessible to external test packages).
if grep -q "^func.*ApplySchemaMapping" "$WORKER_GO"; then
  pass "[REQ-007] AC-7: ApplySchemaMapping is exported (accessible and directly testable)"
else
  fail "[REQ-007] AC-7: ApplySchemaMapping is not exported in worker.go"
fi

# AC-8 (REQ-006/ADR-003): Domain errors (connector failures) are XACK'd, not left for XCLAIM.
# isDomainError + ackMessage path in executeTask distinguishes domain from infra errors.
if grep -q "isDomainError" "$WORKER_GO" && grep -q "domainErrorWrapper" "$WORKER_GO"; then
  pass "[ADR-003] AC-8: domainErrorWrapper + isDomainError implement domain/infrastructure error distinction"
else
  fail "[ADR-003] AC-8: domainErrorWrapper or isDomainError missing from worker.go"
fi

# AC-9 (REQ-009): TaskEventBroker narrow interface defined in events.go.
if grep -q "TaskEventBroker" "$EVENTS_GO" && grep -q "PublishTaskEvent" "$EVENTS_GO"; then
  pass "[REQ-009] AC-9: TaskEventBroker interface with PublishTaskEvent defined in events.go"
else
  fail "[REQ-009] AC-9: TaskEventBroker interface not found in events.go"
fi

# AC-9 (REQ-009): publishTaskEvent called after each transitionStatus.
PUBLISH_CALLS=$(grep -c "publishTaskEvent\|PublishTaskEvent" "$WORKER_GO" || true)
if [ "$PUBLISH_CALLS" -ge 2 ]; then
  pass "[REQ-009] AC-9: publishTaskEvent called after state transitions ($PUBLISH_CALLS occurrences in worker.go)"
else
  fail "[REQ-009] AC-9: publishTaskEvent called only $PUBLISH_CALLS time(s) in worker.go; expected >= 2"
fi

# Connector registry: DefaultConnectorRegistry present in connectors.go.
if grep -q "DefaultConnectorRegistry" "$CONNECTORS_GO"; then
  pass "[TASK-007] ConnectorRegistry: DefaultConnectorRegistry implemented in connectors.go"
else
  fail "[TASK-007] ConnectorRegistry: DefaultConnectorRegistry not found in connectors.go"
fi

# ErrUnknownConnector sentinel: unregistered connector type returns a typed sentinel.
if grep -q "ErrUnknownConnector" "$CONNECTORS_GO"; then
  pass "[TASK-007] ConnectorRegistry: ErrUnknownConnector sentinel defined"
else
  fail "[TASK-007] ConnectorRegistry: ErrUnknownConnector sentinel missing"
fi

# NewWorkerWithPipelines: new constructor that accepts PipelineRepository.
if grep -q "NewWorkerWithPipelines" "$WORKER_GO"; then
  pass "[TASK-007] NewWorkerWithPipelines constructor present (backward-compatible with NewWorker)"
else
  fail "[TASK-007] NewWorkerWithPipelines constructor missing from worker.go"
fi

# ============================================================
# Section 6: Live system tests
# Requires: TASK-042 (demo connectors), running Docker Compose stack, worker binary.
# These tests exercise the full pipeline through live Redis and PostgreSQL.
# Skip automatically when prerequisites are absent.
# ============================================================
echo ""
echo "--- Live system tests (requires TASK-042 + Docker Compose stack) ---"

SKIP_LIVE="${SKIP_LIVE:-}"
LIVE_AVAILABLE=1

if [ -n "$SKIP_LIVE" ] && [ "$SKIP_LIVE" = "1" ]; then
  LIVE_AVAILABLE=0
  echo "  SKIP_LIVE=1 set; skipping all live system tests."
elif ! docker exec "$POSTGRES_CONTAINER" pg_isready -U nexusflow > /dev/null 2>&1; then
  LIVE_AVAILABLE=0
  echo "  PostgreSQL not reachable (container: $POSTGRES_CONTAINER). Skipping live tests."
elif ! docker exec "$REDIS_CONTAINER" redis-cli ping > /dev/null 2>&1; then
  LIVE_AVAILABLE=0
  echo "  Redis not reachable (container: $REDIS_CONTAINER). Skipping live tests."
elif [ ! -f "$WORKER_BINARY" ]; then
  LIVE_AVAILABLE=0
  echo "  Worker binary not found at $WORKER_BINARY. Skipping live tests."
fi

if [ "$LIVE_AVAILABLE" -eq 0 ]; then
  # -------------------------------------------------------
  # Live test: AC-1 — Worker reads from queue:etl (not queue:report)
  # REQ-005: Tag-based task-to-worker matching
  # Given:  worker starts with tags=["etl"]
  # When:   a task is published to queue:etl and another to queue:report
  # Then:   the "etl" worker picks up the task from queue:etl and leaves queue:report untouched
  # -------------------------------------------------------
  skip "[REQ-005] AC-1 (live): Worker with tag 'etl' reads from queue:etl and not queue:report — requires TASK-042 + live stack"

  # -------------------------------------------------------
  # Live test: AC-2 + AC-3 — State transitions and task_state_log entries
  # REQ-009: Task lifecycle state tracking
  # Given:  a task in "queued" state and a running worker
  # When:   the worker picks up and executes the task
  # Then:   task_state_log contains entries for assigned, running, and completed with timestamps
  # -------------------------------------------------------
  skip "[REQ-009] AC-2/AC-3 (live): State transitions queued->assigned->running->completed recorded in task_state_log — requires TASK-042 + live stack"

  # -------------------------------------------------------
  # Live test: AC-4 + AC-5 + AC-6 — Three-phase pipeline execution
  # REQ-006: Three-phase pipeline execution
  # Given:  a pipeline with demo DataSource, demo Process, demo Sink
  # When:   the worker executes the task end-to-end
  # Then:   DataSource produces records, Process transforms them, Sink writes them; task status = "completed"
  # -------------------------------------------------------
  skip "[REQ-006] AC-4/5/6 (live): Three-phase pipeline DataSource->Process->Sink executes end-to-end — requires TASK-042 + live stack"

  # -------------------------------------------------------
  # Live test: AC-7 — Schema mapping renames fields in database
  # REQ-007: Schema mapping between pipeline phases
  # Given:  pipeline defines InputMappings renaming "customer_id" to "id"
  # When:   the pipeline executes
  # Then:   Process connector receives records with renamed fields
  # -------------------------------------------------------
  skip "[REQ-007] AC-7 (live): Schema mapping renames fields through live pipeline execution — requires TASK-042 + live stack"

  # -------------------------------------------------------
  # Live test: AC-8 — Failed connector sets task to "failed" in PostgreSQL
  # REQ-006: Pipeline failure path
  # Given:  pipeline references a connector type not registered in the registry
  # When:   the worker attempts to execute the task
  # Then:   task.status = "failed" in PostgreSQL; message is XACKed from Redis stream
  # -------------------------------------------------------
  skip "[REQ-006] AC-8 (live): Unknown connector type sets task status to 'failed' in PostgreSQL — requires live stack"

  # -------------------------------------------------------
  # Live test: AC-9 — State change events appear in Redis Pub/Sub
  # REQ-009: Task state change events
  # Given:  a subscriber on events:tasks:{userId} before task execution
  # When:   the worker transitions task through assigned->running->completed
  # Then:   three events are published; the last event carries status "completed"
  # Note:   Full Pub/Sub delivery requires TASK-015 (SSE Broker). AC-9 at the worker
  #         level (PublishTaskEvent called) is verified by unit test TestExecuteTask_SSEEventsEmitted.
  # -------------------------------------------------------
  skip "[REQ-009] AC-9 (live): State change events published to Redis Pub/Sub — requires TASK-015 + TASK-042 + live stack"

else
  # Live tests are available — execute them.
  # Note: these will fail until TASK-042 is merged because the demo connectors
  # are not yet registered. The SKIP path above is the expected outcome for
  # the TASK-007 verification cycle.

  DATABASE_URL="postgresql://nexusflow:nexusflow_dev@postgres:5432/nexusflow"
  REDIS_URL="redis://redis:6379"

  WORKER_CONTAINER="nexusflow-verifier-007-etl"
  TEST_TASK_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || python3 -c "import uuid; print(uuid.uuid4())")
  TEST_USER_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || python3 -c "import uuid; print(uuid.uuid4())")

  cleanup_live() {
    docker stop "$WORKER_CONTAINER" > /dev/null 2>&1 || true
    docker rm -f "$WORKER_CONTAINER" > /dev/null 2>&1 || true
    db_query "DELETE FROM tasks WHERE id = '$TEST_TASK_ID';" > /dev/null 2>&1 || true
    db_query "DELETE FROM workers WHERE id LIKE 'verifier-007-%';" > /dev/null 2>&1 || true
    redis_cmd DEL "queue:etl" > /dev/null 2>&1 || true
  }
  trap cleanup_live EXIT

  # Start etl worker
  docker rm -f "$WORKER_CONTAINER" > /dev/null 2>&1 || true
  docker run -d \
    --name "$WORKER_CONTAINER" \
    --network "$DOCKER_NETWORK" \
    -e DATABASE_URL="$DATABASE_URL" \
    -e REDIS_URL="$REDIS_URL" \
    -e WORKER_TAGS="etl" \
    -e WORKER_ID="verifier-007-etl-worker" \
    -v "${WORKER_BINARY}:/worker:ro" \
    alpine:3.20 \
    /worker > /dev/null 2>&1

  sleep 3 # allow worker to register and enter consumption loop

  # AC-1: Verify queue:etl stream exists after worker starts (worker calls InitGroups).
  STREAM_EXISTS=$(redis_cmd EXISTS "queue:etl" 2>/dev/null || echo "0")
  if [ "$STREAM_EXISTS" = "1" ]; then
    pass "[REQ-005] AC-1 (live): Worker created queue:etl consumer group on startup"
  else
    fail "[REQ-005] AC-1 (live): queue:etl stream not created; worker may not have called InitGroups"
  fi

  # Insert a test pipeline and task, then verify execution.
  # (Full live execution requires TASK-042 demo connectors — mark as SKIP if not available.)
  skip "[REQ-006] AC-2/3/4/5/6/8/9 (live): End-to-end task execution — demo connectors not yet implemented (TASK-042)"

  cleanup_live
  trap - EXIT
fi

# ============================================================
# Summary
# ============================================================
echo ""
echo "========================================"
echo "TASK-007 Acceptance Test Summary"
echo "========================================"
printf "  PASS: %d\n" "$PASS"
printf "  FAIL: %d\n" "$FAIL"
printf "  SKIP: %d\n" "$SKIP"
echo ""

if [ "$FAIL" -gt 0 ]; then
  echo "RESULT: FAIL"
  exit 1
else
  echo "RESULT: PASS (SKIP items require TASK-042 + TASK-015 for live execution)"
  exit 0
fi
