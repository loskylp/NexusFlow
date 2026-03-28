# Staging Provisioning Report — Cycle 1

**Date:** 2026-03-27
**DevOps Phase:** Phase 2 — Staging environment and CD pipeline
**Tag triggered:** `demo/v0.1`
**CD run:** https://github.com/loskylp/NexusFlow/actions/runs/23665342083
**Staging URL:** https://nexusflow.staging.nxlabs.cc

---

## Step 1 — CD Pipeline (demo/v0.1 tag)

Tag `demo/v0.1` was created and pushed to `origin`. The CD workflow at `.github/workflows/cd.yml` triggered automatically.

**Run result:** SUCCESS — completed in 1m58s.

Images pushed to `ghcr.io/loskylp/nexusflow/`:

| Image | Tags |
|---|---|
| `ghcr.io/loskylp/nexusflow/api` | `v0.1`, `latest` |
| `ghcr.io/loskylp/nexusflow/worker` | `v0.1`, `latest` |
| `ghcr.io/loskylp/nexusflow/monitor` | `v0.1`, `latest` |
| `ghcr.io/loskylp/nexusflow/web` | `v0.1`, `latest` |

All images are public on ghcr.io — no registry auth required on the staging host.

---

## Step 2 — Staging Provisioning (nxlabs.cc)

**Host:** nxlabs.cc (187.124.233.130)
**Staging directory:** `/opt/nexusflow-staging/`
**Compose file:** `/opt/nexusflow-staging/docker-compose.yml`
**Environment file:** `/opt/nexusflow-staging/.env`

### Pre-deployment checks

- External Docker network `traefik`: present (managed by nxlabs.cc Traefik)
- External Docker network `postgres`: present (managed by nxlabs.cc PostgreSQL)
- Database provisioned via `/opt/postgres/provision.sh nexusflow_staging`

### Services deployed

| Service | Image | Status |
|---|---|---|
| api | `ghcr.io/loskylp/nexusflow/api:v0.1` | Up, healthy |
| worker | `ghcr.io/loskylp/nexusflow/worker:v0.1` | Up |
| monitor | `ghcr.io/loskylp/nexusflow/monitor:v0.1` | Up |
| web | `ghcr.io/loskylp/nexusflow/web:v0.1` | Up |
| redis | `redis:7-alpine` | Up, healthy |

**Note:** NexusFlow does not run its own Watchtower. The nxlabs.cc infrastructure
Watchtower (`nickfedor/watchtower:latest`) manages container updates across all
projects on the host. NexusFlow containers are opted in via the
`com.centurylinklabs.watchtower.enable=true` label on each service. Confirmed
scanned=7 in the infra Watchtower logs — all NexusFlow containers are being watched.

### Environment configuration

The `.env` file was written on the staging host with the following variables (values
are on the host only — not committed):

```
IMAGE_TAG=v0.1
DATABASE_URL=postgresql://nexusflow_staging:***@postgres:5432/nexusflow_staging
WORKER_TAGS=demo,etl
SESSION_TTL_HOURS=24
HEARTBEAT_INTERVAL_SECONDS=5
HEARTBEAT_TIMEOUT_SECONDS=15
PENDING_SCAN_INTERVAL_SECONDS=10
LOG_HOT_RETENTION_HOURS=72
LOG_COLD_RETENTION_HOURS=720
```

---

## Step 3 — Smoke Test Results

All tests executed against `https://nexusflow.staging.nxlabs.cc`.

| # | Test | Expected | Result | Notes |
|---|---|---|---|---|
| 1 | GET /api/health | HTTP 200, `{"status":"ok"}` | **PASS** | `{"status":"ok","redis":"ok","postgres":"ok"}` |
| 2 | POST /api/auth/login (admin/admin) | HTTP 200 + token | **PASS** | Token and user returned |
| 3 | POST /api/pipelines (demo connectors) | HTTP 201 + pipeline ID | **PASS** | Pipeline `95012a56` created with connectorType="demo" |
| 4 | POST /api/tasks (pipelineId, tags:["demo"]) | HTTP 201 + taskId + queued | **PASS** | Task `5c2c3325` queued |
| 5 | Task reaches "completed" | DB status = completed | **PASS** | Demo sink committed 3 records; worker `213fd7c5c361-9db83b0b` |
| 6 | GET /api/workers — at least 1 worker online | HTTP 200, ≥1 worker | **PASS** | 1 worker online, tags=[demo,etl] |
| 7 | Frontend serves at root URL | HTTP 200, text/html | **PASS** | `https://nexusflow.staging.nxlabs.cc/` returns 200 |

**Overall smoke result: 7/7 PASS**

### Smoke test notes

**Test 3 — Pipeline create JSON field naming:** The smoke test instructions used
`"type": "demo"` for connector configs. The API model uses `"connectorType"` (camelCase).
The first pipeline create attempt used the wrong field name; a second pipeline was
created with the correct schema. Both the pipeline create endpoint and the connector
config persistence are working correctly — the field name discrepancy is in the
smoke test instructions, not the API.

**Test 4 — Task submit JSON field naming:** The instructions used `"pipeline_id"`
(snake_case). The API requires `"pipelineId"` (camelCase). First attempt returned
400; corrected on retry.

**Test 5 — Task GET endpoint not implemented:** `GET /api/tasks/{id}` and
`GET /api/tasks` (list) panic with `not implemented` (handlers_tasks.go:196, :210).
Task completion was confirmed by direct PostgreSQL query. This is a Builder-side
gap — the task GET and list handlers are stub bodies. The task create and queue
dispatch paths are fully functional.

---

## Issues Encountered

### Issue 1 — NexusFlow was incorrectly running its own Watchtower (resolved)

**Symptom:** An earlier version of `deploy/staging/docker-compose.yml` included a
`watchtower` service block that would start a NexusFlow-specific Watchtower container.
This violates the nxlabs.cc infrastructure model: Watchtower is a shared infrastructure
service managed by the server owner, not a per-project service.

**Root cause:** The integration guide (`INTEGRATION.md` in `loskylp/nxlabs.cc-infra`)
was not consulted during initial compose authoring. Projects on nxlabs.cc are expected
to opt containers in to the infrastructure Watchtower via the
`com.centurylinklabs.watchtower.enable=true` label only — they must not run their own
Watchtower instance.

**Resolution (2026-03-27):**
- The `watchtower` service block was removed from `deploy/staging/docker-compose.yml`.
- The updated compose was synced to `/opt/nexusflow-staging/docker-compose.yml` on the host.
- `docker compose up -d` was run to reconcile — all five NexusFlow services continued
  running without restart; no rogue Watchtower container was present at time of fix.
- The infrastructure Watchtower (`nickfedor/watchtower:latest`) remains healthy and is
  scanning all opted-in containers (`scanned=7` confirmed in logs).
- All NexusFlow containers retain the `com.centurylinklabs.watchtower.enable=true` label
  and are correctly managed by the infrastructure Watchtower.

**Impact:** None to running services. Auto-update is now correctly handled by the
infrastructure Watchtower. No manual update step is required.

### Issue 2 — Task GET / List endpoints not implemented (Builder gap)

**Symptom:** `GET /api/tasks/{id}` and `GET /api/tasks` return HTTP 500 with
`panic: not implemented`.

**Root cause:** `api/handlers_tasks.go` lines 196 and 210 are stub bodies. The
create and dispatch paths work correctly.

**Impact on smoke tests:** Task completion could not be verified via the API.
Verified via direct DB query instead. For the Demo, the task status polling UI
may not function.

**Required escalation:** Builder must implement `TaskHandler.Get` and
`TaskHandler.List`. This is a blocking item for Demo Sign-off if the demo
workflow includes viewing task status in the UI.

---

## Staging Access

- **API:** `https://nexusflow.staging.nxlabs.cc/api/health`
- **Frontend:** `https://nexusflow.staging.nxlabs.cc/`
- **TLS:** Let's Encrypt certificate via Traefik on nxlabs.cc
- **Login:** admin / admin (accepted for staging per SEC-001)

---

## nxlabs.cc Deployment Prerequisites

For future deployments to the same host (e.g. production):

1. External Docker networks `traefik` and `postgres` must exist (already present).
2. Database must be provisioned: `sudo /opt/postgres/provision.sh <db_name>`.
3. Deploy user has `docker` group membership — no sudo needed for compose commands.
4. ghcr.io images are public — no registry auth configuration required.
5. Auto-updates are handled by the nxlabs.cc infrastructure Watchtower — NexusFlow
   services must carry `com.centurylinklabs.watchtower.enable=true` and must NOT
   run their own Watchtower service.

---

## Parity Gap: Staging vs. Production

The following parity gaps are documented for Nexus awareness before the release cut:

| Gap | Staging | Production (planned) | Risk |
|---|---|---|---|
| Auto-updates | Infrastructure Watchtower (running, scanned=7) | Infrastructure Watchtower (same host) | None — both environments use the shared infra Watchtower |
| Task GET/List API | Not implemented | Not implemented | Demo UI task status view may not work |
| Database | Shared nxlabs.cc PostgreSQL | Shared nxlabs.cc PostgreSQL (separate DB) | Low — same infrastructure |
