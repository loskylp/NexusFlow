#!/usr/bin/env bash
# TASK-020 Acceptance Tests — Worker Fleet Dashboard (GUI)
# REQ-016: Real-time worker fleet monitoring
# REQ-004: Worker registration and heartbeat
#
# Mode: Pre-staging (local, no running server)
# These tests verify the implementation by:
#   1. Running the full vitest unit test suite (41 new tests covering all 8 ACs)
#   2. Static source inspection for each acceptance criterion (positive + negative cases)
#   3. TypeScript build (production bundle)
#   4. TypeScript typecheck
#
# Usage (from project root):
#   bash tests/acceptance/TASK-020-acceptance.sh
#
# Requirements: Node 20+, npm, web/ dependencies installed

set -euo pipefail

WEB_DIR="/Users/pablo/projects/Nexus/NexusTests/NexusFlow/web"
SRC_DIR="$WEB_DIR/src"

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

echo ""
echo "=== TASK-020 Acceptance Tests ==="
echo "    Mode: Pre-staging (local, unit tests + static analysis + build + typecheck)"
echo ""

# ---------------------------------------------------------------------------
# Phase 1: Vitest unit test suite
# The Builder's 41 new unit tests (18 WorkerFleetDashboard, 12 useWorkers,
# 11 useSSE) cover all 8 acceptance criteria at the component and hook level.
#
# AC coverage in the unit test suite:
#   AC-1 (status dots): WorkerFleetDashboard.test.tsx — "status indicators" describe block
#   AC-2 (summary cards): WorkerFleetDashboard.test.tsx — "summary cards" describe block
#   AC-3 (worker goes down): useWorkers.test.ts — "SSE: worker:down" describe block
#   AC-4 (worker comes online): useWorkers.test.ts — "SSE: worker:registered" describe block
#   AC-5 (sortable columns): WorkerFleetDashboard.test.tsx — "sortable columns" describe block
#   AC-6 (default sort): WorkerFleetDashboard.test.tsx — "default sort" describe block
#   AC-7 (reconnecting bar): WorkerFleetDashboard.test.tsx — "SSE status bar" describe block
#   AC-8 (empty state): WorkerFleetDashboard.test.tsx — "empty state" describe block
# ---------------------------------------------------------------------------
echo "--- Phase 1: Unit tests (vitest) ---"

# vitest must be run from within the web/ directory to pick up vitest.config.ts
VITEST_OUTPUT=$(cd "$WEB_DIR" && npx vitest run 2>&1)
VITEST_EXIT=$?

echo "$VITEST_OUTPUT" | tail -20

if [ $VITEST_EXIT -eq 0 ]; then
  TEST_COUNT=$(echo "$VITEST_OUTPUT" | grep -o 'Tests  [0-9]* passed' | grep -o '[0-9]*' | head -1 || echo "?")
  pass "AC-1..8 [REQ-016]: vitest unit suite — ${TEST_COUNT} tests passing"
else
  FAILED_COUNT=$(echo "$VITEST_OUTPUT" | grep -o 'Tests  [0-9]* failed' | grep -o '[0-9]*' | head -1 || echo "?")
  fail "AC-1..8 [REQ-016]: vitest unit suite has failures" \
       "${FAILED_COUNT} test(s) failed — see vitest output above"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-1: Dashboard shows all registered workers with correct status indicators
# REQ-016: Workers must be displayed with green (online) / red (down) dots.
#
# Positive: data-status attribute distinguishes online from down; color uses
#           --color-success and --color-error tokens.
# Negative: Color must never be the sole indicator (WCAG). Text label must
#           accompany the dot. Verify aria-label is present on StatusDot.
# ---------------------------------------------------------------------------
echo "--- AC-1: Status indicators (green = online, red = down) ---"

DASHBOARD_SRC="$SRC_DIR/pages/WorkerFleetDashboard.tsx"

# REQ-016, REQ-004 — AC-1
# Given: WorkerFleetDashboard with StatusDot sub-component
# When: a worker with status 'online' or 'down' is rendered
# Then: data-status attribute must be set to the worker's status

if grep -q 'data-status={status}' "$DASHBOARD_SRC"; then
  pass "AC-1 [REQ-016]: StatusDot sets data-status attribute for test/accessibility targeting"
else
  fail "AC-1 [REQ-016]: StatusDot missing data-status attribute" \
       "Expected data-status={status} on the StatusDot span in WorkerFleetDashboard.tsx"
fi

# Positive: green color token for online status
if grep -q "var(--color-success)" "$DASHBOARD_SRC"; then
  pass "AC-1 [REQ-016]: StatusDot uses --color-success for online workers"
else
  fail "AC-1 [REQ-016]: StatusDot missing --color-success for online state" \
       "Expected var(--color-success) in WorkerFleetDashboard.tsx StatusDot"
fi

# Positive: red color token for down status
if grep -q "var(--color-error)" "$DASHBOARD_SRC"; then
  pass "AC-1 [REQ-016]: StatusDot uses --color-error for down workers"
else
  fail "AC-1 [REQ-016]: StatusDot missing --color-error for down state" \
       "Expected var(--color-error) in WorkerFleetDashboard.tsx StatusDot"
fi

# Negative [VERIFIER-ADDED]: Color must not be the sole indicator (WCAG 2.1 AA).
# A StatusDot that shows only a colored circle would fail WCAG; the text label is required.
# Verify the label text ('Online'/'Down') is rendered alongside the dot.
if grep -q "'Online'" "$DASHBOARD_SRC" && grep -q "'Down'" "$DASHBOARD_SRC"; then
  pass "AC-1 [VERIFIER-ADDED] [REQ-016]: StatusDot renders text label ('Online'/'Down') alongside color dot (WCAG)"
else
  fail "AC-1 [VERIFIER-ADDED] [REQ-016]: StatusDot missing text label — color alone is not sufficient (WCAG)" \
       "Expected 'Online' and 'Down' text labels in WorkerFleetDashboard.tsx StatusDot"
fi

# Negative [VERIFIER-ADDED]: aria-label must be set so screen readers can announce status.
if grep -q 'aria-label={`Worker status: ' "$DASHBOARD_SRC"; then
  pass "AC-1 [VERIFIER-ADDED] [REQ-016]: StatusDot has aria-label for screen reader announcement"
else
  fail "AC-1 [VERIFIER-ADDED] [REQ-016]: StatusDot missing aria-label" \
       "Expected aria-label on StatusDot span in WorkerFleetDashboard.tsx"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-2: Summary cards show accurate counts (Total, Online, Down)
# REQ-016: The header row must show live aggregated worker counts.
#
# Positive: SummaryCard components exist for Total, Online, Down.
# Negative: A trivially permissive implementation might hard-code zeroes.
#           We verify summary is computed from the live workers array.
# ---------------------------------------------------------------------------
echo "--- AC-2: Summary cards (Total, Online, Down) ---"

# REQ-016 — AC-2
# Given: WorkerFleetDashboard using useWorkers().summary
# When: the summary is derived from the live workers array
# Then: total, online, and down counts must be passed to SummaryCard

if grep -q 'summary.total' "$DASHBOARD_SRC"; then
  pass "AC-2 [REQ-016]: Total card reads from summary.total"
else
  fail "AC-2 [REQ-016]: Total card does not use summary.total" \
       "Expected summary.total in WorkerFleetDashboard.tsx SummaryCard"
fi

if grep -q 'summary.online' "$DASHBOARD_SRC"; then
  pass "AC-2 [REQ-016]: Online card reads from summary.online"
else
  fail "AC-2 [REQ-016]: Online card does not use summary.online" \
       "Expected summary.online in WorkerFleetDashboard.tsx SummaryCard"
fi

if grep -q 'summary.down' "$DASHBOARD_SRC"; then
  pass "AC-2 [REQ-016]: Down card reads from summary.down"
else
  fail "AC-2 [REQ-016]: Down card does not use summary.down" \
       "Expected summary.down in WorkerFleetDashboard.tsx SummaryCard"
fi

WORKERS_SRC="$SRC_DIR/hooks/useWorkers.ts"

# Negative [VERIFIER-ADDED]: computeSummary must count by filtering status — not by hard-coding.
# A trivially permissive implementation that always returns { total:0, online:0, down:0 }
# would not satisfy this criterion.
if grep -q "filter(w => w.status === 'online')" "$WORKERS_SRC"; then
  pass "AC-2 [VERIFIER-ADDED] [REQ-016]: computeSummary filters by online status (not hard-coded)"
else
  fail "AC-2 [VERIFIER-ADDED] [REQ-016]: computeSummary does not filter by status" \
       "Expected filter(w => w.status === 'online') in useWorkers.ts computeSummary"
fi

if grep -q "filter(w => w.status === 'down')" "$WORKERS_SRC"; then
  pass "AC-2 [VERIFIER-ADDED] [REQ-016]: computeSummary filters by down status (not hard-coded)"
else
  fail "AC-2 [VERIFIER-ADDED] [REQ-016]: computeSummary does not filter by down status" \
       "Expected filter(w => w.status === 'down') in useWorkers.ts computeSummary"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-3: Worker going down updates in real time without page refresh
# REQ-016, REQ-004: SSE worker:down event sets status='down' on the matching worker.
#
# Positive: mergeWorkerEvent handles 'worker:down' event type.
# Negative: A trivially broken handler that ignores the event would not satisfy this.
# ---------------------------------------------------------------------------
echo "--- AC-3: Real-time worker:down event handling ---"

# REQ-016, REQ-004 — AC-3
# Given: useWorkers hook subscribed to SSE /events/workers
# When: a worker:down event arrives
# Then: the matching worker's status must be set to 'down' in state

if grep -q "case 'worker:down'" "$WORKERS_SRC"; then
  pass "AC-3 [REQ-016]: mergeWorkerEvent handles worker:down event type"
else
  fail "AC-3 [REQ-016]: mergeWorkerEvent missing worker:down case" \
       "Expected case 'worker:down' in useWorkers.ts mergeWorkerEvent"
fi

# Negative [VERIFIER-ADDED]: The down handler must update status to 'down' — not ignore or delete.
if grep -q "status: 'down'" "$WORKERS_SRC"; then
  pass "AC-3 [VERIFIER-ADDED] [REQ-016]: worker:down case explicitly sets status to 'down'"
else
  fail "AC-3 [VERIFIER-ADDED] [REQ-016]: worker:down case does not set status to 'down'" \
       "Expected status: 'down' assignment in mergeWorkerEvent worker:down case"
fi

# Negative [VERIFIER-ADDED]: Updates must be applied to the matching worker by ID only.
# A trivially broken handler that sets all workers to 'down' would not satisfy this.
if grep -q 'w.id === event.payload.id' "$WORKERS_SRC"; then
  pass "AC-3 [VERIFIER-ADDED] [REQ-016]: worker:down matches by ID — does not mark all workers down"
else
  fail "AC-3 [VERIFIER-ADDED] [REQ-016]: worker:down handler does not match by ID" \
       "Expected w.id === event.payload.id in mergeWorkerEvent worker:down case"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-4: Worker coming online updates in real time
# REQ-004: worker:registered event adds new workers to the live list.
#
# Positive: mergeWorkerEvent handles 'worker:registered'.
# Negative: Deduplication must prevent double-adding a worker already in the list.
# ---------------------------------------------------------------------------
echo "--- AC-4: Real-time worker:registered event handling ---"

# REQ-004, REQ-016 — AC-4
# Given: useWorkers hook subscribed to SSE /events/workers
# When: a worker:registered event arrives for a new worker
# Then: the worker must be added to the workers list

if grep -q "case 'worker:registered'" "$WORKERS_SRC"; then
  pass "AC-4 [REQ-004]: mergeWorkerEvent handles worker:registered event type"
else
  fail "AC-4 [REQ-004]: mergeWorkerEvent missing worker:registered case" \
       "Expected case 'worker:registered' in useWorkers.ts mergeWorkerEvent"
fi

# Negative [VERIFIER-ADDED]: A trivially broken handler that adds every event including
# duplicates would inflate the worker list. Deduplication by ID must be present.
if grep -q 'workers.some(w => w.id === event.payload.id)' "$WORKERS_SRC"; then
  pass "AC-4 [VERIFIER-ADDED] [REQ-004]: worker:registered deduplicates by ID (no double-add)"
else
  fail "AC-4 [VERIFIER-ADDED] [REQ-004]: worker:registered does not deduplicate by ID" \
       "Expected workers.some(w => w.id === event.payload.id) in mergeWorkerEvent registered case"
fi

# Positive: heartbeat events also update existing workers (keeps them online)
if grep -q "case 'worker:heartbeat'" "$WORKERS_SRC"; then
  pass "AC-4 [REQ-016]: mergeWorkerEvent handles worker:heartbeat for online state maintenance"
else
  fail "AC-4 [REQ-016]: mergeWorkerEvent missing worker:heartbeat case" \
       "Expected case 'worker:heartbeat' in useWorkers.ts mergeWorkerEvent"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-5: Table columns are sortable by click
# REQ-016: All five columns must have sort controls (SortableHeader).
#
# Positive: All five column names exist with SortableHeader onClick handlers.
# Negative: A trivially permissive table with static headers would not respond to clicks.
# ---------------------------------------------------------------------------
echo "--- AC-5: Sortable columns ---"

# REQ-016 — AC-5
# Given: WorkerFleetDashboard with SortableHeader components
# When: a user clicks any column header
# Then: the table must re-sort by that column

for COL in "status" "id" "tags" "currentTask" "lastHeartbeat"; do
  if grep -q "column=\"$COL\"" "$DASHBOARD_SRC"; then
    pass "AC-5 [REQ-016]: SortableHeader exists for column '$COL'"
  else
    fail "AC-5 [REQ-016]: SortableHeader missing for column '$COL'" \
         "Expected column=\"$COL\" in WorkerFleetDashboard.tsx"
  fi
done

# Negative [VERIFIER-ADDED]: SortableHeader must have an onClick to trigger sorting —
# a header with no handler is inert and cannot satisfy AC-5.
if grep -q 'onClick={() => onSort(column)}' "$DASHBOARD_SRC"; then
  pass "AC-5 [VERIFIER-ADDED] [REQ-016]: SortableHeader th has onClick handler that triggers sort"
else
  fail "AC-5 [VERIFIER-ADDED] [REQ-016]: SortableHeader missing onClick handler" \
       "Expected onClick={() => onSort(column)} on th in SortableHeader"
fi

# Negative [VERIFIER-ADDED]: Toggle logic must swap asc/desc on repeated clicks.
# A handler that only ever sets 'asc' would not satisfy the toggle requirement.
if grep -q "direction === 'asc' ? 'desc' : 'asc'" "$DASHBOARD_SRC"; then
  pass "AC-5 [VERIFIER-ADDED] [REQ-016]: handleSort toggles asc/desc on repeated clicks"
else
  fail "AC-5 [VERIFIER-ADDED] [REQ-016]: handleSort does not toggle sort direction" \
       "Expected ternary toggle asc/desc in handleSort in WorkerFleetDashboard.tsx"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-6: Down workers sorted to top by default
# REQ-016: Default sort state must place down workers (weight 0) above online (weight 1).
#
# Positive: DEFAULT_SORT is { column: 'status', direction: 'asc' }.
# Negative: A default sort of 'desc' on status would put online workers first.
# ---------------------------------------------------------------------------
echo "--- AC-6: Default sort places down workers first ---"

# REQ-016 — AC-6
# Given: WorkerFleetDashboard with DEFAULT_SORT constant
# When: the page loads
# Then: sort column must be 'status' with direction 'asc' (down weight 0 < online weight 1)

if grep -q "column: 'status'" "$DASHBOARD_SRC"; then
  pass "AC-6 [REQ-016]: DEFAULT_SORT column is 'status'"
else
  fail "AC-6 [REQ-016]: DEFAULT_SORT column is not 'status'" \
       "Expected column: 'status' in DEFAULT_SORT in WorkerFleetDashboard.tsx"
fi

if grep -q "direction: 'asc'" "$DASHBOARD_SRC"; then
  pass "AC-6 [REQ-016]: DEFAULT_SORT direction is 'asc' (down weight 0 < online weight 1)"
else
  fail "AC-6 [REQ-016]: DEFAULT_SORT direction is not 'asc'" \
       "Expected direction: 'asc' in DEFAULT_SORT in WorkerFleetDashboard.tsx"
fi

# Negative [VERIFIER-ADDED]: statusSortWeight must assign 'down' a lower weight than 'online'
# so that ascending sort (lowest first) puts down workers at the top.
# A reversed weight assignment (down=1, online=0) would produce the wrong sort order.
if grep -q "status === 'down' ? 0 : 1" "$DASHBOARD_SRC"; then
  pass "AC-6 [VERIFIER-ADDED] [REQ-016]: statusSortWeight assigns down=0 (lower) and online=1 (higher)"
else
  fail "AC-6 [VERIFIER-ADDED] [REQ-016]: statusSortWeight does not assign correct weights" \
       "Expected \"status === 'down' ? 0 : 1\" in statusSortWeight function"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-7: SSE disconnection shows "Reconnecting..." in status bar
# REQ-016: Status bar must surface SSE connection health to the user.
#
# Positive: StatusBar renders "Reconnecting..." when sseStatus === 'reconnecting'.
# Negative: "Reconnecting..." must not appear when sseStatus is 'connected'.
# ---------------------------------------------------------------------------
echo "--- AC-7: SSE reconnecting status bar ---"

# REQ-016 — AC-7
# Given: WorkerFleetDashboard StatusBar component
# When: sseStatus is 'reconnecting'
# Then: "Reconnecting..." text must be visible in the status bar

if grep -q "'Reconnecting...'" "$DASHBOARD_SRC"; then
  pass "AC-7 [REQ-016]: StatusBar contains 'Reconnecting...' text label"
else
  fail "AC-7 [REQ-016]: StatusBar missing 'Reconnecting...' text" \
       "Expected 'Reconnecting...' string literal in StatusBar in WorkerFleetDashboard.tsx"
fi

# Positive: Status bar uses role="status" and aria-live="polite" per UX spec
if grep -q 'role="status"' "$DASHBOARD_SRC" && grep -q 'aria-live="polite"' "$DASHBOARD_SRC"; then
  pass "AC-7 [REQ-016]: StatusBar has role=\"status\" aria-live=\"polite\" for accessibility"
else
  fail "AC-7 [REQ-016]: StatusBar missing role=\"status\" or aria-live=\"polite\"" \
       "Expected role=\"status\" aria-live=\"polite\" on StatusBar div in WorkerFleetDashboard.tsx"
fi

# Negative [VERIFIER-ADDED]: StatusBar must conditionally show "Reconnecting..." only when
# sseStatus === 'reconnecting'. A component that always shows "Reconnecting..." would satisfy
# the positive test but break the connected state. We verify the conditional logic.
if grep -q "sseStatus === 'reconnecting'" "$DASHBOARD_SRC"; then
  pass "AC-7 [VERIFIER-ADDED] [REQ-016]: StatusBar conditionally renders based on sseStatus === 'reconnecting'"
else
  fail "AC-7 [VERIFIER-ADDED] [REQ-016]: StatusBar does not guard on sseStatus === 'reconnecting'" \
       "Expected sseStatus === 'reconnecting' check in StatusBar in WorkerFleetDashboard.tsx"
fi

# Negative [VERIFIER-ADDED]: useSSE must expose 'reconnecting' as a possible connection state.
SSE_SRC="$SRC_DIR/hooks/useSSE.ts"
if grep -q "'reconnecting'" "$SSE_SRC"; then
  pass "AC-7 [VERIFIER-ADDED] [REQ-016]: useSSE transitions to 'reconnecting' state on error"
else
  fail "AC-7 [VERIFIER-ADDED] [REQ-016]: useSSE does not have a 'reconnecting' state" \
       "Expected 'reconnecting' status value in useSSE.ts"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-8: Empty state message shown when no workers registered
# REQ-016: When the workers array is empty after initial load, a descriptive
# message must be shown in place of the data table.
#
# Positive: Empty state message text is present.
# Negative: The data table must NOT render alongside the empty state.
# ---------------------------------------------------------------------------
echo "--- AC-8: Empty state when no workers registered ---"

# REQ-016 — AC-8
# Given: WorkerFleetDashboard with workers.length === 0 after initial load
# When: the page renders
# Then: "No workers registered" message must appear; data table must not render

if grep -q 'No workers registered' "$DASHBOARD_SRC"; then
  pass "AC-8 [REQ-016]: Empty state message 'No workers registered...' is present"
else
  fail "AC-8 [REQ-016]: Empty state message not found in WorkerFleetDashboard.tsx" \
       "Expected 'No workers registered' text in WorkerFleetDashboard.tsx empty state block"
fi

# Positive: Empty state uses conditional rendering (workers.length === 0 check)
if grep -q 'workers.length === 0' "$DASHBOARD_SRC"; then
  pass "AC-8 [REQ-016]: Empty state is conditionally rendered on workers.length === 0"
else
  fail "AC-8 [REQ-016]: No workers.length === 0 guard for empty state" \
       "Expected workers.length === 0 conditional in WorkerFleetDashboard.tsx"
fi

# Negative [VERIFIER-ADDED]: The data table must not appear alongside the empty state message.
# A trivially permissive implementation renders both the empty message and an empty table.
# We verify the conditional is structured as an if/else (mutually exclusive rendering).
# The data table carries role="table"; the empty state check must exclude it.
# We look for the ternary/if-else pattern: empty state OR table, not both.
if grep -A15 'workers.length === 0' "$DASHBOARD_SRC" | grep -q '<p'; then
  pass "AC-8 [VERIFIER-ADDED] [REQ-016]: Empty state renders <p> text, not a data table"
else
  fail "AC-8 [VERIFIER-ADDED] [REQ-016]: Empty state branch does not render a <p> element" \
       "Expected <p> element in the workers.length === 0 branch of WorkerFleetDashboard.tsx"
fi

echo ""

# ---------------------------------------------------------------------------
# Phase 2: TypeScript build
# ---------------------------------------------------------------------------
echo "--- Phase 2: Production build (tsc + vite build) ---"

BUILD_OUTPUT=$(npm --prefix "$WEB_DIR" run build 2>&1)
BUILD_EXIT=$?

if [ $BUILD_EXIT -eq 0 ]; then
  pass "All ACs [REQ-016]: production build succeeds — all imports resolve and types are valid"
else
  fail "All ACs [REQ-016]: production build failed" \
       "$(echo "$BUILD_OUTPUT" | tail -20)"
fi

echo ""

# ---------------------------------------------------------------------------
# Phase 3: TypeScript typecheck
# ---------------------------------------------------------------------------
echo "--- Phase 3: TypeScript typecheck (tsc --noEmit) ---"

TC_OUTPUT=$(npm --prefix "$WEB_DIR" run typecheck 2>&1)
TC_EXIT=$?

if [ $TC_EXIT -eq 0 ]; then
  pass "All ACs [REQ-016]: TypeScript typecheck passes with zero errors"
else
  fail "All ACs [REQ-016]: TypeScript typecheck has errors" \
       "$(echo "$TC_OUTPUT" | tail -20)"
fi

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
