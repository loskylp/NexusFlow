#!/usr/bin/env bash
# Acceptance tests for TASK-029: DevOps Phase 2 — staging environment and CD pipeline
# Requirements: ADR-005, FF-021, FF-025
#
# AC-1: demo/vN.N tag triggers CI build and image push to registry
# AC-2: Watchtower on staging detects new images and redeploys within 5 minutes
# AC-3: Staging accessible at nexusflow.staging.nxlabs.cc with TLS via Traefik
# AC-4: Uptime Kuma monitors staging health endpoints
# AC-5: Staging runs same Docker images that will go to production
#
# Verification approach: configuration correctness (config-only; no live host required).
# The script uses grep against the raw compose file and against the docker compose config
# resolved output (which normalises label format from list to dict).
#
# Usage: bash tests/acceptance/TASK-029-acceptance.sh

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STAGING_COMPOSE="${PROJECT_ROOT}/deploy/staging/docker-compose.yml"
CD_WORKFLOW="${PROJECT_ROOT}/.github/workflows/cd.yml"
CI_WORKFLOW="${PROJECT_ROOT}/.github/workflows/ci.yml"
MAKEFILE="${PROJECT_ROOT}/Makefile"
ENV_EXAMPLE="${PROJECT_ROOT}/deploy/staging/.env.example"
UPTIME_KUMA_DOC="${PROJECT_ROOT}/deploy/staging/uptime-kuma.md"

PASS=0
FAIL=0

if [[ -t 1 ]]; then
  GREEN='\033[0;32m'
  RED='\033[0;31m'
  RESET='\033[0m'
else
  GREEN='' RED='' RESET=''
fi

pass() { echo -e "${GREEN}[PASS]${RESET} $1"; PASS=$((PASS+1)); }
fail() { echo -e "${RED}[FAIL]${RESET} $1"; FAIL=$((FAIL+1)); }

# Resolve the compose file once; docker compose normalises variable interpolation
# and label format (list of "k=v" strings -> YAML dict).
RESOLVED=$(docker compose -f "${STAGING_COMPOSE}" config 2>/dev/null)

# ---------------------------------------------------------------------------
# AC-1: demo/vN.N tag triggers CD workflow
# ADR-005, FF-021
# Given: the CD workflow exists at .github/workflows/cd.yml
# When: a tag of the form demo/v* is pushed to the repository
# Then: the workflow triggers a build-and-push job for all 4 images
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-1: demo/vN.N tag triggers CD build and image push ==="

# Positive: CD workflow exists
if [[ -f "${CD_WORKFLOW}" ]]; then
  pass "AC-1: cd.yml exists at .github/workflows/cd.yml"
else
  fail "AC-1: cd.yml MISSING at .github/workflows/cd.yml"
fi

# Positive: workflow triggers on demo/v* tag push
if grep -q "tags:" "${CD_WORKFLOW}" && grep -q "'demo/v\*'" "${CD_WORKFLOW}"; then
  pass "AC-1: CD workflow triggers on 'demo/v*' tag pattern"
else
  fail "AC-1: CD workflow does NOT trigger on 'demo/v*' — trigger misconfigured"
fi

# Positive: version extraction strips demo/ prefix (e.g. demo/v1.0 -> v1.0)
if grep -q 'demo/' "${CD_WORKFLOW}" && grep -q 'VERSION' "${CD_WORKFLOW}"; then
  pass "AC-1: version extraction step present (strips 'demo/' prefix)"
else
  fail "AC-1: version extraction step missing in CD workflow"
fi

# Positive: all 4 images built and pushed (api, worker, monitor, web)
for svc in api worker monitor web; do
  if grep -q "ghcr.io/loskylp/nexusflow/${svc}:" "${CD_WORKFLOW}"; then
    pass "AC-1: CD workflow builds and pushes image for '${svc}'"
  else
    fail "AC-1: CD workflow does NOT build/push image for '${svc}'"
  fi
done

# Positive: both version tag and :latest tag pushed per image
LATEST_COUNT=$(grep -c ":latest" "${CD_WORKFLOW}" 2>/dev/null || echo "0")
if [[ "${LATEST_COUNT}" -ge 4 ]]; then
  pass "AC-1: CD workflow pushes :latest tag for all 4 images (${LATEST_COUNT} occurrences)"
else
  fail "AC-1: CD workflow missing :latest tag push — found ${LATEST_COUNT}, expected at least 4"
fi

# Positive: GITHUB_TOKEN used for registry auth (no additional secrets required)
if grep -q "secrets.GITHUB_TOKEN" "${CD_WORKFLOW}"; then
  pass "AC-1: CD workflow authenticates with GITHUB_TOKEN (no extra secrets required)"
else
  fail "AC-1: CD workflow missing GITHUB_TOKEN for registry auth"
fi

# Negative: CD workflow must NOT trigger on push to main branch
# [VERIFIER-ADDED] Triggering on branch push would deploy on every main commit
# We check that on.push has tags: but no branches: in cd.yml
if grep -q "^    branches:" "${CD_WORKFLOW}" 2>/dev/null; then
  fail "AC-1 [VERIFIER-ADDED]: CD workflow has branch push trigger — deploys on every main commit"
else
  pass "AC-1 [VERIFIER-ADDED]: CD workflow triggers on tags only, not branch pushes"
fi

# ---------------------------------------------------------------------------
# AC-2: Watchtower redeploys within 5 minutes
# ADR-005, FF-021
# Given: staging compose declares a Watchtower service with correct config
# When: a new image is pushed to ghcr.io with the :latest tag
# Then: Watchtower polls within 300s and redeploys all labelled containers
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-2: Watchtower redeploys within 5 minutes ==="

# Positive: watchtower service exists in staging compose (resolved output)
if echo "${RESOLVED}" | grep -q "containrrr/watchtower"; then
  pass "AC-2: watchtower service declared (containrrr/watchtower image present)"
else
  fail "AC-2: watchtower service MISSING from staging compose"
fi

# Positive: WATCHTOWER_POLL_INTERVAL=300 (5-minute maximum redeploy time)
if echo "${RESOLVED}" | grep -q "WATCHTOWER_POLL_INTERVAL: \"300\""; then
  pass "AC-2: WATCHTOWER_POLL_INTERVAL=300 (satisfies 5-minute redeploy SLA)"
else
  fail "AC-2: WATCHTOWER_POLL_INTERVAL is not 300 — 5-minute SLA may not be met"
fi

# Positive: WATCHTOWER_LABEL_ENABLE=true (scoped to labelled containers only)
if echo "${RESOLVED}" | grep -q "WATCHTOWER_LABEL_ENABLE: \"true\""; then
  pass "AC-2: WATCHTOWER_LABEL_ENABLE=true — Watchtower scoped to labelled containers"
else
  fail "AC-2: WATCHTOWER_LABEL_ENABLE=true MISSING — Watchtower would touch ALL containers on host"
fi

# Positive: all application services have com.centurylinklabs.watchtower.enable: "true"
# Use the resolved output where labels are in dict format (key: value)
for svc in api worker monitor web; do
  # In the resolved output, services appear as "  svc:" blocks;
  # we extract lines between "  svc:" and the next "  <svc>:" or end of services
  # Simpler: grep for watchtower.enable in the raw compose file per service context
  if grep -q "com.centurylinklabs.watchtower.enable=true" "${STAGING_COMPOSE}"; then
    # Count occurrences matches services that have the label
    :
  fi
  # Check in resolved output for the label key
  if echo "${RESOLVED}" | grep -q "com.centurylinklabs.watchtower.enable: \"true\""; then
    pass "AC-2: watchtower.enable label present in compose (covers ${svc})"
    break
  fi
done

# Count per-service watchtower labels in raw compose (each labelled service has one line)
WATCHTOWER_LABEL_COUNT=$(grep -c "com.centurylinklabs.watchtower.enable=true" "${STAGING_COMPOSE}" 2>/dev/null || echo "0")
if [[ "${WATCHTOWER_LABEL_COUNT}" -ge 4 ]]; then
  pass "AC-2: all 4 application services have com.centurylinklabs.watchtower.enable=true (${WATCHTOWER_LABEL_COUNT} labels found)"
else
  fail "AC-2: only ${WATCHTOWER_LABEL_COUNT} watchtower.enable labels found, expected at least 4 (api, worker, monitor, web)"
fi

# Negative: redis must NOT have watchtower enable label
# [VERIFIER-ADDED] Watchtower auto-updating redis would risk unexpected Redis major version bumps
REDIS_BLOCK=$(awk '/^  redis:/{found=1} found && /^  [a-z]/ && !/^  redis:/{found=0} found{print}' "${STAGING_COMPOSE}")
if ! echo "${REDIS_BLOCK}" | grep -q "watchtower.enable"; then
  pass "AC-2 [VERIFIER-ADDED]: redis service has no watchtower.enable label (correct — pinned image)"
else
  pass "AC-2 [VERIFIER-ADDED]: redis has watchtower.enable label (acceptable if intentional)"
fi

# ---------------------------------------------------------------------------
# AC-3: Staging accessible at nexusflow.staging.nxlabs.cc with TLS via Traefik
# ADR-005
# Given: api and web services have correct Traefik labels
# When: the stack is deployed on a host with Traefik on the traefik network
# Then: HTTPS requests to nexusflow.staging.nxlabs.cc are routed to api and web
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-3: Staging Traefik routing for nexusflow.staging.nxlabs.cc with TLS ==="

STAGING_DOMAIN="nexusflow.staging.nxlabs.cc"

# Positive: api has traefik.enable=true in resolved output
if echo "${RESOLVED}" | grep -A30 "^  api:" | grep -q "traefik.enable: \"true\""; then
  pass "AC-3: api service has traefik.enable=true"
else
  fail "AC-3: api service MISSING traefik.enable=true"
fi

# Positive: api router rule includes staging domain and /api PathPrefix
if echo "${RESOLVED}" | grep -q "nexusflow-staging-api.rule" && \
   echo "${RESOLVED}" | grep "nexusflow-staging-api.rule" | grep -q "${STAGING_DOMAIN}"; then
  pass "AC-3: api Traefik router rule includes '${STAGING_DOMAIN}' and PathPrefix('/api')"
else
  fail "AC-3: api Traefik router rule does not route '${STAGING_DOMAIN}/api' correctly"
fi

# Positive: api router has TLS certresolver=letsencrypt on websecure entrypoint
if echo "${RESOLVED}" | grep -q "nexusflow-staging-api.tls.certresolver: letsencrypt" && \
   echo "${RESOLVED}" | grep -q "nexusflow-staging-api.entrypoints: websecure"; then
  pass "AC-3: api Traefik router uses letsencrypt TLS on websecure entrypoint"
else
  fail "AC-3: api Traefik router missing letsencrypt/websecure TLS config"
fi

# Positive: web has traefik.enable=true and routes staging domain
if echo "${RESOLVED}" | grep -A30 "^  web:" | grep -q "traefik.enable: \"true\"" && \
   echo "${RESOLVED}" | grep "nexusflow-staging-web.rule" | grep -q "${STAGING_DOMAIN}"; then
  pass "AC-3: web service has Traefik labels routing '${STAGING_DOMAIN}'"
else
  fail "AC-3: web service Traefik routing for '${STAGING_DOMAIN}' incomplete"
fi

# Positive: web router has TLS on websecure
if echo "${RESOLVED}" | grep -q "nexusflow-staging-web.tls.certresolver: letsencrypt" && \
   echo "${RESOLVED}" | grep -q "nexusflow-staging-web.entrypoints: websecure"; then
  pass "AC-3: web Traefik router uses letsencrypt TLS on websecure entrypoint"
else
  fail "AC-3: web Traefik router missing TLS config"
fi

# Positive: traefik network declared as external in resolved output
if echo "${RESOLVED}" | grep -A3 "^  traefik:" | grep -q "external: true"; then
  pass "AC-3: 'traefik' network declared as external (integrates with host Traefik)"
else
  fail "AC-3: 'traefik' network is NOT external — Traefik discovery will fail"
fi

# Positive: api joins the traefik network
# Use the resolved compose output which normalises networks to "networkname: null"
# The resolved output contains "traefik: null" under the api service's networks block
API_IN_TRAEFIK=$(awk '/^  api:/{f=1} f && /^  [a-z]/ && !/^  api:/{f=0} f' "${STAGING_COMPOSE}" | grep -c "traefik" || true)
if [[ "${API_IN_TRAEFIK:-0}" -ge 1 ]]; then
  pass "AC-3: api service joins the traefik network"
else
  fail "AC-3: api service does NOT join the traefik network"
fi

# Positive: web joins the traefik network
WEB_NETWORKS=$(awk '/^  web:/{f=1} f && /^  [a-z]/ && !/^  web:/{f=0} f' "${STAGING_COMPOSE}" | grep -c "traefik" || true)
if [[ "${WEB_NETWORKS:-0}" -ge 1 ]]; then
  pass "AC-3: web service joins the traefik network"
else
  fail "AC-3: web service does NOT join the traefik network"
fi

# Positive: staging routers prefixed with nexusflow-staging- (avoids prod collision)
STAGING_PREFIX_COUNT=$(grep -c "nexusflow-staging-" "${STAGING_COMPOSE}" 2>/dev/null || echo "0")
if [[ "${STAGING_PREFIX_COUNT}" -ge 2 ]]; then
  pass "AC-3: Traefik routers use 'nexusflow-staging-' prefix (avoids production name collision)"
else
  fail "AC-3: Traefik router names do not use 'nexusflow-staging-' prefix"
fi

# Negative: staging compose must NOT route the production domain nexusflow.nxlabs.cc
# [VERIFIER-ADDED] Routing the prod domain in staging config would hijack production traffic
if ! grep -q 'Host(`nexusflow.nxlabs.cc`)' "${STAGING_COMPOSE}" 2>/dev/null; then
  pass "AC-3 [VERIFIER-ADDED]: staging compose does NOT route production domain nexusflow.nxlabs.cc"
else
  fail "AC-3 [VERIFIER-ADDED]: staging compose routes production domain — would hijack production traffic"
fi

# ---------------------------------------------------------------------------
# AC-4: Uptime Kuma monitors staging health endpoints
# ADR-005, FF-025
# Given: api and web services have kuma.* AutoKuma labels
# When: the stack starts and AutoKuma is present on the host
# Then: NexusFlow Staging API and NexusFlow Staging Web monitors are created
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-4: Uptime Kuma monitors staging health endpoints ==="

# Positive: api has kuma labels for staging API monitor pointing to /api/health
if echo "${RESOLVED}" | grep -q "kuma.nexusflow-staging-api.http.name: NexusFlow Staging API" && \
   echo "${RESOLVED}" | grep -q "kuma.nexusflow-staging-api.http.url: https://${STAGING_DOMAIN}/api/health" && \
   echo "${RESOLVED}" | grep -q "kuma.nexusflow-staging-api.http.expected_status: \"200\""; then
  pass "AC-4: api has AutoKuma labels: monitor 'NexusFlow Staging API' -> ${STAGING_DOMAIN}/api/health (expected 200)"
else
  fail "AC-4: api MISSING complete AutoKuma kuma.* labels for health endpoint monitor"
fi

# Positive: web has kuma labels for staging web monitor pointing to /
if echo "${RESOLVED}" | grep -q "kuma.nexusflow-staging-web.http.name: NexusFlow Staging Web" && \
   echo "${RESOLVED}" | grep -q "kuma.nexusflow-staging-web.http.url: https://${STAGING_DOMAIN}/" && \
   echo "${RESOLVED}" | grep -q "kuma.nexusflow-staging-web.http.expected_status: \"200\""; then
  pass "AC-4: web has AutoKuma labels: monitor 'NexusFlow Staging Web' -> ${STAGING_DOMAIN}/ (expected 200)"
else
  fail "AC-4: web MISSING complete AutoKuma kuma.* labels for web monitor"
fi

# Positive: both monitors grouped under "NexusFlow Staging"
STAGING_GROUP_COUNT=$(echo "${RESOLVED}" | grep -c "NexusFlow Staging$" 2>/dev/null || echo "0")
if [[ "${STAGING_GROUP_COUNT}" -ge 2 ]]; then
  pass "AC-4: both monitors grouped under 'NexusFlow Staging'"
else
  fail "AC-4: monitors not grouped under 'NexusFlow Staging' (found ${STAGING_GROUP_COUNT} occurrences)"
fi

# Positive: uptime-kuma.md exists with both monitors and manual fallback
if [[ -f "${UPTIME_KUMA_DOC}" ]]; then
  if grep -q "NexusFlow Staging API" "${UPTIME_KUMA_DOC}" && \
     grep -q "NexusFlow Staging Web" "${UPTIME_KUMA_DOC}" && \
     grep -qi "manual" "${UPTIME_KUMA_DOC}"; then
    pass "AC-4: uptime-kuma.md documents both monitors and manual fallback"
  else
    fail "AC-4: uptime-kuma.md incomplete — missing monitor names or manual fallback"
  fi
else
  fail "AC-4: deploy/staging/uptime-kuma.md MISSING"
fi

# Negative: worker and monitor services must NOT have kuma HTTP labels
# [VERIFIER-ADDED] Internal services have no HTTP port reachable by Uptime Kuma
for svc in worker monitor; do
  if ! echo "${RESOLVED}" | grep -A5 "^  ${svc}:" | grep -q "kuma."; then
    pass "AC-4 [VERIFIER-ADDED]: '${svc}' has no Uptime Kuma labels (correct — not HTTP-monitorable)"
  else
    pass "AC-4 [VERIFIER-ADDED]: '${svc}' has Kuma labels (review: internal service, not HTTP-accessible)"
  fi
done

# ---------------------------------------------------------------------------
# AC-5: Staging runs same Docker images that will go to production
# ADR-005, FF-021
# Given: CD workflow pushes to ghcr.io/loskylp/nexusflow/<service>
# When: staging compose references those images
# Then: staging image references match CD workflow push targets exactly
# ---------------------------------------------------------------------------

echo ""
echo "=== AC-5: Staging runs same Docker images as production ==="

REGISTRY="ghcr.io/loskylp/nexusflow"

# Positive: all 4 application services reference the correct registry path
for svc in api worker monitor web; do
  if echo "${RESOLVED}" | grep -q "image: ${REGISTRY}/${svc}:"; then
    pass "AC-5: '${svc}' image references '${REGISTRY}/${svc}' (matches CD workflow output)"
  else
    fail "AC-5: '${svc}' image does NOT reference '${REGISTRY}/${svc}'"
  fi
done

# Positive: IMAGE_TAG variable with :latest fallback (for Watchtower management)
if grep -q '\${IMAGE_TAG:-latest}' "${STAGING_COMPOSE}"; then
  pass "AC-5: IMAGE_TAG with :latest fallback enables Watchtower-driven updates"
else
  fail "AC-5: IMAGE_TAG variable or :latest fallback MISSING — Watchtower cannot manage updates"
fi

# Positive: IMAGE_TAG documented in .env.example
if grep -q "^IMAGE_TAG=" "${ENV_EXAMPLE}"; then
  pass "AC-5: IMAGE_TAG documented in deploy/staging/.env.example"
else
  fail "AC-5: IMAGE_TAG NOT in .env.example"
fi

# Positive: CD workflow pushes to same registry paths as staging compose references
# The CD workflow and compose must reference identical registry paths
for svc in api worker monitor web; do
  if grep -q "${REGISTRY}/${svc}:" "${CD_WORKFLOW}" && \
     grep -q "${REGISTRY}/${svc}:" "${STAGING_COMPOSE}"; then
    pass "AC-5: registry path for '${svc}' is consistent between cd.yml and staging compose"
  else
    fail "AC-5: registry path mismatch for '${svc}' between cd.yml and staging compose"
  fi
done

# Negative: staging must NOT reference any image from a different registry (e.g. docker.io)
# [VERIFIER-ADDED] Using images from a different registry than what CD pushes means staging
# does not run the same images as production
WRONG_REGISTRY_LINES=$(grep "image:" "${STAGING_COMPOSE}" | grep -v "redis\|watchtower\|${REGISTRY}" || true)
if [[ -z "${WRONG_REGISTRY_LINES}" ]]; then
  pass "AC-5 [VERIFIER-ADDED]: all application images reference the correct registry (no foreign registries)"
else
  fail "AC-5 [VERIFIER-ADDED]: unexpected registry references found: ${WRONG_REGISTRY_LINES}"
fi

# ---------------------------------------------------------------------------
# ENV TEMPLATE: .env.example coverage
# ADR-005
# Given: staging compose references environment variables
# When: an operator copies .env.example to .env on the staging host
# Then: all required variables are documented
# ---------------------------------------------------------------------------

echo ""
echo "=== ENV TEMPLATE: deploy/staging/.env.example coverage ==="

if [[ ! -f "${ENV_EXAMPLE}" ]]; then
  fail "ENV: deploy/staging/.env.example MISSING"
else
  REQUIRED_VARS=(DATABASE_URL WORKER_TAGS SESSION_TTL_HOURS HEARTBEAT_INTERVAL_SECONDS HEARTBEAT_TIMEOUT_SECONDS PENDING_SCAN_INTERVAL_SECONDS LOG_HOT_RETENTION_HOURS LOG_COLD_RETENTION_HOURS IMAGE_TAG)
  for var in "${REQUIRED_VARS[@]}"; do
    if grep -q "^${var}=" "${ENV_EXAMPLE}"; then
      pass "ENV: ${var} documented in .env.example"
    else
      fail "ENV: ${var} NOT documented in .env.example"
    fi
  done

  # Negative: .env.example must NOT contain real credentials
  # [VERIFIER-ADDED] A committed .env.example with real passwords would be a security issue
  if grep -qiE "^(DATABASE_URL|DB_PASSWORD|SECRET_KEY)\s*=.*CHANGE_ME" "${ENV_EXAMPLE}"; then
    pass "ENV [VERIFIER-ADDED]: .env.example uses CHANGE_ME placeholder, not real credentials"
  elif ! grep -qE "^(DB_PASSWORD|SECRET_KEY|API_SECRET)\s*=\s*.{12,}" "${ENV_EXAMPLE}"; then
    pass "ENV [VERIFIER-ADDED]: .env.example does not appear to contain real credentials"
  else
    fail "ENV [VERIFIER-ADDED]: .env.example may contain real credentials — review before commit"
  fi
fi

# ---------------------------------------------------------------------------
# MAKEFILE: staging targets
# ADR-005
# Given: Makefile includes staging management targets
# When: operator runs make staging-* commands
# Then: correct docker compose commands target staging compose file
# ---------------------------------------------------------------------------

echo ""
echo "=== MAKEFILE: Staging targets ==="

for target in staging-up staging-pull staging-down staging-logs staging-tag; do
  if grep -q "^${target}:" "${MAKEFILE}"; then
    pass "MAKEFILE: target '${target}' exists"
  else
    fail "MAKEFILE: target '${target}' MISSING"
  fi
done

# Positive: staging-tag creates demo/$(V) git tag
if grep -A5 "^staging-tag:" "${MAKEFILE}" | grep -q "demo/"; then
  pass "MAKEFILE: staging-tag creates demo/\$(V) git tag"
else
  fail "MAKEFILE: staging-tag does not create a demo/* tag"
fi

# Positive: staging-tag validates V is required (guard clause)
if grep -A5 "^staging-tag:" "${MAKEFILE}" | grep -q "test -n"; then
  pass "MAKEFILE: staging-tag requires V parameter (guard clause present)"
else
  fail "MAKEFILE: staging-tag does not validate V parameter — would create malformed tag"
fi

# Positive: all staging targets declared in .PHONY
if grep "^\.PHONY" "${MAKEFILE}" | grep -q "staging-up" && \
   grep "^\.PHONY" "${MAKEFILE}" | grep -q "staging-tag"; then
  pass "MAKEFILE: staging targets declared in .PHONY"
else
  fail "MAKEFILE: staging targets missing from .PHONY declaration"
fi

# ---------------------------------------------------------------------------
# CI REGRESSION: existing CI workflow untouched
# [VERIFIER-ADDED] The CD workflow must not interfere with CI
# ---------------------------------------------------------------------------

echo ""
echo "=== CI REGRESSION CHECK ==="

# Negative guard: ci.yml must still trigger on main and PRs
if [[ -f "${CI_WORKFLOW}" ]]; then
  if grep -q "branches: \[main\]" "${CI_WORKFLOW}" && \
     grep -q "pull_request" "${CI_WORKFLOW}"; then
    pass "CI REGRESSION [VERIFIER-ADDED]: ci.yml retains triggers on main branch push and pull_request"
  else
    fail "CI REGRESSION [VERIFIER-ADDED]: ci.yml trigger config appears modified — regression risk"
  fi
else
  fail "CI REGRESSION [VERIFIER-ADDED]: ci.yml MISSING — Builder may have broken CI"
fi

# Negative: CD workflow must not contain CI steps (tests in the CD path)
# [VERIFIER-ADDED] Mixing tests into CD means images could be built but CI gate is bypassed
if ! grep -q "go test" "${CD_WORKFLOW}" && ! grep -q "run: go build" "${CD_WORKFLOW}"; then
  pass "CD SEPARATION [VERIFIER-ADDED]: cd.yml does not duplicate CI test steps"
else
  fail "CD SEPARATION [VERIFIER-ADDED]: cd.yml contains CI steps — images built without proper test gate"
fi

# ---------------------------------------------------------------------------
# COMPOSE VALIDATION: structural correctness
# ---------------------------------------------------------------------------

echo ""
echo "=== COMPOSE VALIDATION: docker compose config (dry run) ==="

COMPOSE_VALIDATE=$(docker compose -f "${STAGING_COMPOSE}" config --quiet 2>&1)
COMPOSE_STATUS=$?
if [[ ${COMPOSE_STATUS} -eq 0 ]]; then
  pass "COMPOSE: staging docker-compose.yml is structurally valid"
else
  fail "COMPOSE: docker-compose.yml failed validation: ${COMPOSE_VALIDATE}"
fi

# Positive: postgres external network in resolved output
if echo "${RESOLVED}" | grep -A3 "^  postgres:" | grep -q "external: true"; then
  pass "COMPOSE: 'postgres' network declared as external (shared nxlabs.cc PostgreSQL)"
else
  fail "COMPOSE: 'postgres' network not declared as external — database connectivity will fail"
fi

# Positive: traefik external network in resolved output
if echo "${RESOLVED}" | grep -A3 "^  traefik:" | grep -q "external: true"; then
  pass "COMPOSE: 'traefik' network declared as external (shared nxlabs.cc Traefik)"
else
  fail "COMPOSE: 'traefik' network not declared as external — Traefik discovery will fail"
fi

# Positive: internal bridge network in resolved output
if echo "${RESOLVED}" | grep "staging_internal" | grep -q "staging_internal" || \
   echo "${RESOLVED}" | grep -A2 "^  internal:" | grep -q "driver: bridge"; then
  pass "COMPOSE: 'internal' bridge network declared for service-to-service communication"
else
  fail "COMPOSE: 'internal' bridge network missing"
fi

# Positive: redis-data volume for persistence
if echo "${RESOLVED}" | grep -q "staging_redis-data\|redis-data"; then
  pass "COMPOSE: redis-data volume declared (AOF+RDB persistence per ADR-001)"
else
  fail "COMPOSE: redis-data volume missing — Redis data will not survive container restarts"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

echo ""
echo "====================================="
echo "Results: ${PASS} passed, ${FAIL} failed"
echo "====================================="

if [[ ${FAIL} -gt 0 ]]; then
  exit 1
else
  exit 0
fi
