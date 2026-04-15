#!/usr/bin/env bash
# TASK-038 Acceptance Test — Fitness function CI gate.
#
# Validates:
#   1. Fitness function test binary compiles with -tags integration.
#   2. TestFF015_CompileTimeSafety passes (always — binary exists).
#   3. TestFF013_AuthEnforcement passes against a running API server.
#   4. TestFF019_SchemaValidation passes against a running API server.
#   5. TestFF017_SchemaMigration passes against a clean database.
#
# Tests that require Docker socket (FF-001, FF-007, FF-008, FF-024) are skipped
# in the standard CI run and annotated for the ops-fitness CI job.
#
# Preconditions:
#   - Go toolchain available.
#   - For integration tests: API and Redis running at DATABASE_URL + REDIS_URL.
#
# See: TASK-038, process/architect/fitness-functions.md
set -euo pipefail

echo "TASK-038 acceptance: fitness function CI gate"
echo ""
echo "Step 1: compile fitness function test binary"
go test -tags integration -c -o /tmp/fitness-test ./tests/system/ 2>&1 || {
    echo "FAIL: fitness function test binary failed to compile"
    exit 1
}
echo "PASS: binary compiled"
echo ""
echo "Step 2: run compile-time safety test (FF-015)"
/tmp/fitness-test -test.run TestFF015_CompileTimeSafety -test.v 2>&1 || {
    echo "FAIL: FF-015 compile-time safety test failed"
    exit 1
}
echo ""
echo "Step 3: run remaining non-Docker tests"
echo "NOTE: Docker-dependent tests (FF-001, FF-007, FF-008, FF-024) are skipped"
/tmp/fitness-test -test.run "TestFF013|TestFF017|TestFF019|TestFF020" -test.v 2>&1
echo ""
echo "TASK-038 acceptance: done"
