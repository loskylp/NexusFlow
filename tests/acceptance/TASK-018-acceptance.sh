#!/usr/bin/env bash
# TASK-018 Acceptance Tests — Sink atomicity with idempotency
# REQ-008, ADR-003, ADR-009
#
# Acceptance criteria:
#   AC-1: Database Sink forced failure mid-write rolls back all partial records; destination has zero records
#   AC-2: S3 Sink forced failure mid-write aborts multipart upload; no partial objects
#   AC-3: Successful Sink write commits all records atomically
#   AC-4: Duplicate execution (same task ID + attempt number) is detected and skipped (no duplicate writes)
#   AC-5: Execution ID is recorded at the destination for deduplication
#
# These tests exercise the SinkConnector interface using in-memory fakes (no live
# PostgreSQL or S3 required). All five acceptance criteria are verified at the
# component boundary via Go acceptance tests in TASK-018-acceptance_test.go.
#
# Usage:
#   bash tests/acceptance/TASK-018-acceptance.sh
#   (from the module root)
#
# The shell script is a thin runner — the acceptance logic lives in the Go test file.
# This pattern is consistent with TASK-018's in-memory-only implementation scope:
# there is no public HTTP interface to test at the system layer for these connectors.
#
# Requires: docker (for Go 1.23 toolchain)

set -uo pipefail

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
echo "=== TASK-018 Acceptance Tests — Sink atomicity with idempotency ==="
echo "    REQ-008, ADR-003, ADR-009"
echo ""

# ---------------------------------------------------------------------------
# Resolve module root: this script can be invoked from any directory.
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "--- Module root: $MODULE_ROOT"
echo ""

# ---------------------------------------------------------------------------
# AC-1 through AC-5: Run Go acceptance tests via Docker.
# All five criteria are covered by TASK-018-acceptance_test.go.
# ---------------------------------------------------------------------------
echo "--- Running Go acceptance tests (docker run golang:1.23-alpine) ---"
echo ""

if docker run --rm \
    -v "$MODULE_ROOT:/workspace" \
    -w /workspace \
    golang:1.23-alpine \
    go test ./tests/acceptance/... -v -run TASK018 2>&1; then

  pass "AC-1 [REQ-008]: Database Sink forced failure rolls back all partial records (zero rows)"
  pass "AC-2 [REQ-008]: S3 Sink forced failure aborts multipart upload (no partial objects)"
  pass "AC-3 [REQ-008]: Successful Sink write commits all records atomically (all three connector types)"
  pass "AC-4 [ADR-003]: Duplicate execution ID is detected and skipped (no duplicate writes)"
  pass "AC-5 [ADR-003]: Execution ID is recorded at the destination for deduplication"
else
  fail "Go acceptance tests" "one or more TASK-018 acceptance tests failed — see output above for details"
fi

echo ""

# ---------------------------------------------------------------------------
# Regression: full test suite must remain green after TASK-018 changes.
# ---------------------------------------------------------------------------
echo "--- Regression: full test suite ---"

if docker run --rm \
    -v "$MODULE_ROOT:/workspace" \
    -w /workspace \
    golang:1.23-alpine \
    go test ./... 2>&1 | grep -E "^(ok|FAIL|---)" ; then
  pass "Regression: go test ./... — all packages pass"
else
  fail "Regression: go test ./..." "full test suite reported failures — check output above"
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
