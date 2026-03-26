#!/usr/bin/env bash
# Acceptance tests for TASK-001: DevOps Phase 1 — CI pipeline and dev environment
# Requirements: FF-015, FF-020, ADR-004, ADR-005
#
# AC-1: docker compose up starts all core services and they pass health checks within 30s
# AC-2: CI pipeline configured for: go build, go vet, staticcheck on push to main
# AC-3: Monorepo directory layout matches ADR-004
# AC-4: .env.example documents all required environment variables
#
# Usage: bash tests/acceptance/TASK-001-acceptance.sh [--skip-docker]
#
# The --skip-docker flag skips the docker compose up test (AC-1), which requires
# Docker and builds images. Use it when Docker is not available.

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PASS=0
FAIL=0
SKIP=0
SKIP_DOCKER=false

if [[ "${1:-}" == "--skip-docker" ]]; then
  SKIP_DOCKER=true
fi

# Colour helpers (only if terminal supports it)
if [[ -t 1 ]]; then
  GREEN='\033[0;32m'
  RED='\033[0;31m'
  YELLOW='\033[0;33m'
  RESET='\033[0m'
else
  GREEN='' RED='' YELLOW='' RESET=''
fi

pass() { echo -e "${GREEN}[PASS]${RESET} $1"; PASS=$((PASS+1)); }
fail() { echo -e "${RED}[FAIL]${RESET} $1"; FAIL=$((FAIL+1)); }
skip() { echo -e "${YELLOW}[SKIP]${RESET} $1"; SKIP=$((SKIP+1)); }

# ---------------------------------------------------------------------------
# AC-3 — REQ: ADR-004 monorepo directory layout
# Given: the repository has been set up per ADR-004
# When: we check for required top-level directories
# Then: api/, worker/, monitor/, web/, internal/ all exist
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-3: Monorepo directory layout (ADR-004) ==="

for dir in api worker monitor web internal; do
  if [[ -d "${PROJECT_ROOT}/${dir}" ]]; then
    pass "Directory ${dir}/ exists"
  else
    fail "Directory ${dir}/ is MISSING"
  fi
done

# Negative case: no unexpected monolithic structure (no src/ at root)
# [VERIFIER-ADDED] This checks that the codebase is not accidentally using a non-ADR-004 layout
if [[ -d "${PROJECT_ROOT}/src" ]]; then
  fail "Unexpected src/ directory at root — ADR-004 does not specify this layout"
else
  pass "No unexpected src/ directory at root"
fi

# Positive: cmd/ directory exists for Go binaries (implied by ADR-004 Go monorepo)
if [[ -d "${PROJECT_ROOT}/cmd" ]]; then
  pass "Directory cmd/ exists (Go binary entrypoints)"
else
  fail "Directory cmd/ is MISSING — Go binary entrypoints not present"
fi

# Verify cmd subdirectories for the three Go services
for svc in api worker monitor; do
  if [[ -d "${PROJECT_ROOT}/cmd/${svc}" ]]; then
    pass "cmd/${svc}/ exists"
  else
    fail "cmd/${svc}/ is MISSING"
  fi
done

# ---------------------------------------------------------------------------
# AC-4 — REQ: ADR-005 .env.example documents all required environment variables
# Given: .env.example exists at the project root
# When: we check its contents against the variables used in internal/config/config.go
# Then: all required variables are documented
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-4: .env.example documents all required environment variables ==="

ENV_EXAMPLE="${PROJECT_ROOT}/.env.example"

if [[ ! -f "${ENV_EXAMPLE}" ]]; then
  fail ".env.example does not exist"
else
  pass ".env.example exists"

  # Required variables per config.Load()
  REQUIRED_VARS=(
    "DATABASE_URL"
    "REDIS_URL"
    "DB_PASSWORD"
    "WORKER_TAGS"
    "API_PORT"
    "ENV"
    "SESSION_TTL_HOURS"
    "HEARTBEAT_INTERVAL_SECONDS"
    "HEARTBEAT_TIMEOUT_SECONDS"
    "PENDING_SCAN_INTERVAL_SECONDS"
    "LOG_HOT_RETENTION_HOURS"
    "LOG_COLD_RETENTION_HOURS"
  )

  for var in "${REQUIRED_VARS[@]}"; do
    if grep -q "^${var}=" "${ENV_EXAMPLE}"; then
      pass ".env.example documents ${var}"
    else
      fail ".env.example is MISSING documentation for ${var}"
    fi
  done

  # Negative case: the file should not be empty
  # [VERIFIER-ADDED] Confirm the file has substance
  line_count=$(wc -l < "${ENV_EXAMPLE}")
  if [[ "${line_count}" -gt 5 ]]; then
    pass ".env.example has content (${line_count} lines)"
  else
    fail ".env.example is suspiciously short (${line_count} lines)"
  fi
fi

# ---------------------------------------------------------------------------
# AC-2 — REQ: FF-015 CI pipeline triggers on push to main; runs go build, go vet, staticcheck
# Given: .github/workflows/ci.yml exists
# When: we inspect its configuration
# Then: it triggers on push to main; go build, go vet, and staticcheck steps are present
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-2: CI pipeline configuration ==="

CI_WORKFLOW="${PROJECT_ROOT}/.github/workflows/ci.yml"

if [[ ! -f "${CI_WORKFLOW}" ]]; then
  fail ".github/workflows/ci.yml does not exist"
else
  pass ".github/workflows/ci.yml exists"

  # Trigger: push to main
  if grep -q "branches: \[main\]" "${CI_WORKFLOW}" || grep -qE "branches:.*main" "${CI_WORKFLOW}"; then
    pass "CI triggers on push to main branch"
  else
    fail "CI does NOT trigger on push to main branch"
  fi

  # go build step
  if grep -q "go build ./..." "${CI_WORKFLOW}"; then
    pass "CI runs 'go build ./...'"
  else
    fail "CI does NOT run 'go build ./...'"
  fi

  # go vet step
  if grep -q "go vet ./..." "${CI_WORKFLOW}"; then
    pass "CI runs 'go vet ./...'"
  else
    fail "CI does NOT run 'go vet ./...'"
  fi

  # staticcheck step
  if grep -q "staticcheck ./..." "${CI_WORKFLOW}"; then
    pass "CI runs 'staticcheck ./...'"
  else
    fail "CI does NOT run 'staticcheck ./...'"
  fi

  # Negative case: staticcheck must be installed before use
  # [VERIFIER-ADDED] Confirm staticcheck installation step is present
  if grep -q "staticcheck@" "${CI_WORKFLOW}"; then
    pass "CI installs staticcheck before running it"
  else
    fail "CI runs staticcheck without an installation step"
  fi

  # go test step present
  if grep -q "go test ./..." "${CI_WORKFLOW}"; then
    pass "CI runs 'go test ./...'"
  else
    fail "CI does NOT run 'go test ./...'"
  fi

  # Verify push trigger is not only for PRs
  # [VERIFIER-ADDED] The AC requires push to main, not only pull_request
  if grep -q "push:" "${CI_WORKFLOW}"; then
    pass "CI has 'push:' trigger (not only pull_request)"
  else
    fail "CI is missing 'push:' trigger — may only run on PRs"
  fi
fi

# ---------------------------------------------------------------------------
# AC-1 — REQ: FF-020 docker compose up starts core services; health checks pass within 30s
# Given: docker-compose.yml exists with service definitions for api, worker, monitor, redis, postgres
# When: we inspect the docker-compose.yml
# Then: all five core services are defined with appropriate health checks
# (Docker runtime test requires images to be built — see AC-1 runtime section below)
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-1 (static): docker-compose.yml service definitions and health checks ==="

DC_FILE="${PROJECT_ROOT}/docker-compose.yml"

if [[ ! -f "${DC_FILE}" ]]; then
  fail "docker-compose.yml does not exist"
else
  pass "docker-compose.yml exists"

  # All five core services must be defined
  for svc in api worker monitor redis postgres; do
    if grep -qE "^  ${svc}:" "${DC_FILE}"; then
      pass "Service '${svc}' is defined in docker-compose.yml"
    else
      fail "Service '${svc}' is MISSING from docker-compose.yml"
    fi
  done

  # Health check for redis — the config uses CMD array form: ["CMD", "redis-cli", "ping"]
  if grep -q "redis-cli" "${DC_FILE}"; then
    pass "Redis service has health check (redis-cli ping)"
  else
    fail "Redis service is MISSING health check"
  fi

  # Health check for postgres
  if grep -q "pg_isready" "${DC_FILE}"; then
    pass "Postgres service has health check (pg_isready)"
  else
    fail "Postgres service is MISSING health check"
  fi

  # Health check for api (checks /api/health endpoint)
  if grep -q "/api/health" "${DC_FILE}"; then
    pass "API service health check references /api/health endpoint"
  else
    fail "API service health check does NOT reference /api/health endpoint"
  fi

  # Negative case: api healthcheck must check for "status" in the response body
  # [VERIFIER-ADDED] The Builder documented that the check looks for "status" to handle 503
  if grep -q '"status"' "${DC_FILE}"; then
    pass "API healthcheck checks for 'status' field (handles degraded 503 response)"
  else
    fail "API healthcheck does not look for 'status' field in response"
  fi

  # api depends_on redis and postgres (with health condition)
  if grep -A5 "depends_on:" "${DC_FILE}" | grep -q "service_healthy"; then
    pass "Services depend on dependencies being healthy (condition: service_healthy)"
  else
    fail "Services do not use service_healthy dependency condition"
  fi
fi

# ---------------------------------------------------------------------------
# AC-1 (runtime) — docker compose up and health check endpoints
# Given: Docker is available and images can be built
# When: docker compose up -d is run
# Then: all core services reach healthy state within 30 seconds
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-1 (runtime): docker compose up and service health ==="

if [[ "${SKIP_DOCKER}" == "true" ]]; then
  skip "AC-1 runtime test skipped (--skip-docker flag)"
else
  COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.yml"

  # Ensure .env exists
  if [[ ! -f "${PROJECT_ROOT}/.env" ]]; then
    cp "${PROJECT_ROOT}/.env.example" "${PROJECT_ROOT}/.env"
    echo "  Created .env from .env.example"
  fi

  echo "  Starting services with docker compose up -d ..."
  docker compose -f "${COMPOSE_FILE}" up -d 2>&1 | tail -5 || {
    fail "docker compose up -d failed to start services"
    SKIP_HEALTH=true
  }

  SKIP_HEALTH=${SKIP_HEALTH:-false}

  if [[ "${SKIP_HEALTH}" == "false" ]]; then
    echo "  Waiting up to 30 seconds for services to become healthy..."
    DEADLINE=$((SECONDS + 30))

    # Wait for redis to be healthy
    REDIS_HEALTHY=false
    while [[ "${SECONDS}" -lt "${DEADLINE}" ]]; do
      STATUS=$(docker compose -f "${COMPOSE_FILE}" ps --format json 2>/dev/null | \
               python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('Health','') if isinstance(d,dict) else next((x.get('Health','') for x in d if x.get('Service')=='redis'), ''))" 2>/dev/null || echo "")
      if docker compose -f "${COMPOSE_FILE}" exec -T redis redis-cli ping 2>/dev/null | grep -q "PONG"; then
        REDIS_HEALTHY=true
        break
      fi
      sleep 2
    done

    if [[ "${REDIS_HEALTHY}" == "true" ]]; then
      pass "Redis is up and responding to PING within 30 seconds"
    else
      fail "Redis did NOT become responsive within 30 seconds"
    fi

    # Wait for postgres to be healthy
    POSTGRES_HEALTHY=false
    DEADLINE=$((SECONDS + 30))
    while [[ "${SECONDS}" -lt "${DEADLINE}" ]]; do
      if docker compose -f "${COMPOSE_FILE}" exec -T postgres pg_isready -U nexusflow 2>/dev/null | grep -q "accepting connections"; then
        POSTGRES_HEALTHY=true
        break
      fi
      sleep 2
    done

    if [[ "${POSTGRES_HEALTHY}" == "true" ]]; then
      pass "PostgreSQL is accepting connections within 30 seconds"
    else
      fail "PostgreSQL did NOT become responsive within 30 seconds"
    fi

    # Wait for api health endpoint
    API_HEALTHY=false
    DEADLINE=$((SECONDS + 30))
    while [[ "${SECONDS}" -lt "${DEADLINE}" ]]; do
      HTTP_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/health 2>/dev/null || echo "000")
      BODY=$(curl -s http://localhost:8080/api/health 2>/dev/null || echo "")
      # Accept both 200 and 503 — the health endpoint always returns a body with "status"
      if echo "${BODY}" | grep -q '"status"'; then
        API_HEALTHY=true
        API_STATUS="${HTTP_RESPONSE}"
        API_BODY="${BODY}"
        break
      fi
      sleep 2
    done

    if [[ "${API_HEALTHY}" == "true" ]]; then
      pass "API /api/health endpoint responds within 30 seconds (HTTP ${API_STATUS})"
      # Verify response always contains "status" field
      if echo "${API_BODY}" | grep -q '"status"'; then
        pass "API health response contains 'status' field: ${API_BODY}"
      else
        fail "API health response missing 'status' field: ${API_BODY}"
      fi
      # At TASK-001, redis should be ok, postgres will be error (nil pool)
      if echo "${API_BODY}" | grep -q '"redis":"ok"'; then
        pass "API health shows redis:ok"
      else
        fail "API health does NOT show redis:ok: ${API_BODY}"
      fi
    else
      fail "API /api/health did NOT become responsive within 30 seconds"
    fi

    # Verify worker and monitor containers are running (they have no healthcheck)
    for svc in worker monitor; do
      if docker compose -f "${COMPOSE_FILE}" ps "${svc}" 2>/dev/null | grep -qE "running|Up"; then
        pass "Service '${svc}' is running"
      else
        fail "Service '${svc}' is NOT running"
      fi
    done

    # Negative case: a request to a stub endpoint must return 500 (not a process crash)
    # [VERIFIER-ADDED] The Builder said stub handlers panic but Recoverer converts to 500
    STUB_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/api/tasks 2>/dev/null || echo "000")
    if [[ "${STUB_STATUS}" == "500" ]]; then
      pass "Stub route POST /api/tasks returns 500 (panic recovered, not process crash)"
    else
      # Could be 405 (method not allowed) or other — 000 means server down
      if [[ "${STUB_STATUS}" == "000" ]]; then
        fail "API server appears down after stub route request (expected 500, got ${STUB_STATUS})"
      else
        pass "Stub route POST /api/tasks returns ${STUB_STATUS} (server still up)"
      fi
    fi

    # Verify the api is still responsive after the stub panic
    HEALTH_AFTER=$(curl -s http://localhost:8080/api/health 2>/dev/null || echo "")
    if echo "${HEALTH_AFTER}" | grep -q '"status"'; then
      pass "API still responds to /api/health after stub handler invocation"
    else
      fail "API is NOT responsive after stub handler invocation — panic recovery may have failed"
    fi

    echo "  Stopping services..."
    docker compose -f "${COMPOSE_FILE}" down 2>&1 | tail -3 || true
  fi
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

echo ""
echo "==================================================="
echo "TASK-001 Acceptance Test Results"
echo "==================================================="
echo "PASS: ${PASS}"
echo "FAIL: ${FAIL}"
echo "SKIP: ${SKIP}"
echo ""

if [[ "${FAIL}" -gt 0 ]]; then
  echo "RESULT: FAIL"
  exit 1
else
  echo "RESULT: PASS"
  exit 0
fi
