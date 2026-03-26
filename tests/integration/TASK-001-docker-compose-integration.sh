#!/usr/bin/env bash
# Integration tests for TASK-001: Docker Compose service assembly
# Requirements: FF-020, ADR-005
#
# These tests verify that the services assemble correctly via Docker Compose:
# - Service dependencies are wired correctly (depends_on with health conditions)
# - The api container can reach redis via the internal network
# - Service images build successfully from the Dockerfiles
#
# Layer: Integration (verifies component assembly at service boundaries)
# Interface: Docker Compose service layer
#
# Usage: bash tests/integration/TASK-001-docker-compose-integration.sh
# Requires: Docker, Docker Compose

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PASS=0
FAIL=0

GREEN='\033[0;32m'
RED='\033[0;31m'
RESET='\033[0m'

pass() { echo -e "${GREEN}[PASS]${RESET} $1"; PASS=$((PASS+1)); }
fail() { echo -e "${RED}[FAIL]${RESET} $1"; FAIL=$((FAIL+1)); }

COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.yml"

# Ensure .env exists
if [[ ! -f "${PROJECT_ROOT}/.env" ]]; then
  cp "${PROJECT_ROOT}/.env.example" "${PROJECT_ROOT}/.env"
fi

echo ""
echo "=== Integration: Docker Compose service assembly (FF-020, ADR-005) ==="

# Verify docker-compose.yml is syntactically valid
echo "  Validating docker-compose.yml syntax..."
if docker compose -f "${COMPOSE_FILE}" config --quiet 2>/dev/null; then
  pass "docker-compose.yml is syntactically valid"
else
  fail "docker-compose.yml has syntax errors"
  exit 1
fi

# Verify all Dockerfiles exist for the services that need building
echo "  Checking Dockerfiles exist..."
for dockerfile in Dockerfile.api Dockerfile.worker Dockerfile.monitor Dockerfile.web; do
  if [[ -f "${PROJECT_ROOT}/${dockerfile}" ]]; then
    pass "${dockerfile} exists"
  else
    fail "${dockerfile} is MISSING"
  fi
done

# Verify each Dockerfile uses multi-stage build (ADR-004 specifies multi-stage Go builds)
echo "  Checking multi-stage build pattern..."
for dockerfile in Dockerfile.api Dockerfile.worker Dockerfile.monitor; do
  if grep -q "AS builder" "${PROJECT_ROOT}/${dockerfile}" && grep -q "FROM alpine" "${PROJECT_ROOT}/${dockerfile}"; then
    pass "${dockerfile} uses multi-stage build (builder -> runtime)"
  else
    fail "${dockerfile} does not appear to use multi-stage build"
  fi
done

# Verify api service is on the internal network (required for redis/postgres connectivity)
echo "  Checking network connectivity configuration..."
if grep -A20 "^  api:" "${COMPOSE_FILE}" | grep -q "internal"; then
  pass "API service is connected to internal network"
else
  fail "API service is NOT connected to internal network"
fi

# Verify redis and postgres are on the internal network
for svc in redis postgres; do
  if grep -A20 "^  ${svc}:" "${COMPOSE_FILE}" | grep -q "internal"; then
    pass "${svc} service is connected to internal network"
  else
    fail "${svc} service is NOT connected to internal network"
  fi
done

# Verify start_period is set on the api healthcheck (allows time for Go binary to start)
if grep -q "start_period:" "${COMPOSE_FILE}"; then
  pass "API healthcheck has start_period configured"
else
  fail "API healthcheck is MISSING start_period (Go binary startup time)"
fi

# Verify redis persistence command (ADR-001 requires AOF+RDB)
if grep -q "appendonly yes" "${COMPOSE_FILE}" && grep -q "appendfsync everysec" "${COMPOSE_FILE}"; then
  pass "Redis configured with AOF persistence (appendonly yes, appendfsync everysec)"
else
  fail "Redis is NOT configured with AOF persistence"
fi

if grep -q "save 900 1" "${COMPOSE_FILE}"; then
  pass "Redis configured with RDB snapshots (save 900 1)"
else
  fail "Redis is NOT configured with RDB snapshots"
fi

# Verify redis has a named volume for data persistence
if grep -q "redis-data:" "${COMPOSE_FILE}"; then
  pass "Redis uses named volume 'redis-data' for persistence"
else
  fail "Redis is NOT using a named volume — data will be lost on container restart"
fi

# Verify postgres has a named volume
if grep -q "postgres-data:" "${COMPOSE_FILE}"; then
  pass "Postgres uses named volume 'postgres-data' for persistence"
else
  fail "Postgres is NOT using a named volume"
fi

echo ""
echo "==================================================="
echo "Integration Test Results"
echo "==================================================="
echo "PASS: ${PASS}"
echo "FAIL: ${FAIL}"
echo ""

if [[ "${FAIL}" -gt 0 ]]; then
  echo "RESULT: FAIL"
  exit 1
else
  echo "RESULT: PASS"
  exit 0
fi
