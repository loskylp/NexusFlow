# Builder Handoff — TASK-029
**Date:** 2026-03-27
**Task:** DevOps Phase 2 — staging environment and CD pipeline on nxlabs.cc
**Requirement(s):** ADR-005, FF-021, FF-025

## What Was Implemented

### `deploy/staging/docker-compose.yml`

Staging compose file for `nexusflow.staging.nxlabs.cc`. Contains all five core
services (api, worker, monitor, web, redis) plus a Watchtower container.

Key design decisions:
- All application images reference `ghcr.io/loskylp/nexusflow/<service>:${IMAGE_TAG:-latest}`.
  The `IMAGE_TAG` variable lets the first deploy pin to a specific version; subsequent
  redeploys are handled by Watchtower using the `latest` tag.
- Traefik labels on `api` and `web` use router names prefixed with `nexusflow-staging-`
  to avoid colliding with future production routers (`nexusflow-api`, `nexusflow-web`)
  on the same Traefik instance.
- Both `api` and `web` declare AutoKuma labels (`kuma.*`) so Uptime Kuma monitors
  are created automatically when the stack starts.
- Watchtower is scoped to labelled containers only (`WATCHTOWER_LABEL_ENABLE=true`);
  it does not touch other stacks on the host.
- The `postgres` external network is declared because the staging database uses the
  shared nxlabs.cc PostgreSQL instance. The `traefik` external network is declared
  for Traefik discovery. The `internal` bridge network handles service-to-service
  communication.
- Redis uses the same AOF+RDB persistence configuration as production (ADR-001).

### `deploy/staging/.env.example`

Documents every environment variable consumed by the staging compose stack.
Contains a placeholder `DATABASE_URL` that must be updated with the credentials
created by the nxlabs.cc PostgreSQL provisioning script.

### `.github/workflows/cd.yml`

CD workflow triggered by `demo/v*` tag pushes. Strips the `demo/` prefix to
derive a clean version string (e.g. `demo/v1.0` → `v1.0`). Builds all four
Docker images using `docker/build-push-action@v5` with GHA layer caching and
pushes two tags per image: the version tag and `latest`. Watchtower on staging
picks up the `latest` tag within 5 minutes.

The workflow uses `GITHUB_TOKEN` for registry authentication — no additional
secrets are needed for the initial push.

The existing `ci.yml` is untouched.

### `deploy/staging/uptime-kuma.md`

Describes how the AutoKuma labels in the compose file translate to Uptime Kuma
monitors, plus a manual fallback procedure if AutoKuma is not present on the host.
Maps monitors to FF-025 (infrastructure health) and FF-021 (image integrity).

### `Makefile` (additions)

Added staging targets: `staging-up`, `staging-pull`, `staging-down`, `staging-logs`,
and `staging-tag`. The `staging-tag V=v1.0` target creates the `demo/v1.0` git tag
and pushes it, triggering the CD pipeline. Added `.PHONY` declarations for all new
targets.

## Unit Tests

This task is infrastructure configuration — YAML, shell, and GitHub Actions workflow
files. There is no application logic to unit-test. The CI workflow (`ci.yml`) covers
the build smoke tests for all Docker images on every PR. The CD workflow is validated
by triggering it with a `demo/v*` tag during the Verifier's acceptance test.

- Tests written: 0 (infrastructure-only task; no testable application logic)
- All existing tests: unaffected — no application code was modified

## Deviations from Task Description

### 1. Watchtower as a compose service, not a host-level daemon

The task description says "Watchtower on staging" without specifying whether it
runs as a host-level daemon or a compose service. ADR-005 says "Watchtower for
automated container updates" without constraining the deployment method.

Decision: Watchtower is declared as a service in the staging compose file. This
keeps the staging stack self-contained — a single `docker compose up` deploys
everything, including Watchtower. If the nxlabs.cc host already runs a global
Watchtower daemon, remove the `watchtower` service from the compose file; the
`com.centurylinklabs.watchtower.enable=true` labels on the application services
work with either approach.

### 2. Image tags: both versioned and `latest`

The CD workflow pushes two tags per image: the version derived from the git tag
(e.g. `v1.0`) and `latest`. Watchtower watches the `latest` tag, which is the
standard Watchtower pattern. The compose file defaults to `IMAGE_TAG=latest` so
Watchtower-driven redeploys work without any environment variable changes. On
first deploy the operator can set `IMAGE_TAG=v1.0` to pin to a known-good version.

### 3. Router names prefixed with `nexusflow-staging-`

ADR-005 uses router names `nexusflow-api` and `nexusflow-web` in its production
example. Using the same names on staging would create a Traefik routing conflict
when both stacks run on the same host. The staging routers are named
`nexusflow-staging-api` and `nexusflow-staging-web` to prevent this. TASK-036
(production) should use the unqualified names `nexusflow-api` / `nexusflow-web`
as in the ADR example.

### 4. No self-hosted PostgreSQL in staging compose

ADR-005 is explicit: staging uses the shared nxlabs.cc PostgreSQL instance, not
a self-hosted container. The staging compose file does not include a `postgres`
service. The operator must provision the staging database first:
```
ssh deploy@nxlabs.cc /opt/postgres/provision.sh nexusflow_staging
```

## Known Limitations

1. **Registry authentication for Watchtower.** The staging Watchtower service mounts
   `/root/.docker/config.json` to provide credentials for pulling from `ghcr.io`.
   If the ghcr.io packages are made public (the simplest option for an open-source
   project), this mount can be removed. If private, the operator must log in on the
   staging host before the first deploy: `docker login ghcr.io -u <github_username>`.

2. **First-time staging host setup.** The `traefik` and `postgres` external Docker
   networks must exist before `docker compose up` will succeed. These are created by
   the nxlabs.cc infrastructure setup scripts and should already be present on the
   host. If not, create them with:
   ```
   docker network create traefik
   docker network create postgres
   ```

3. **Health endpoint status.** The staging API monitor in Uptime Kuma expects HTTP
   200. The current `/api/health` implementation returns 200 when all dependencies
   are healthy and 503 when degraded (TASK-001). AutoKuma's `expected_status=200`
   will therefore report the API as "down" if Redis or PostgreSQL is unreachable,
   which is correct behaviour for a staging health check.

4. **TASK-027 health endpoint.** TASK-029's dependency was changed from TASK-027
   (full health endpoint) to TASK-042 (demo connectors) in plan v2.1. The current
   `/api/health` endpoint from TASK-001 is sufficient for basic Uptime Kuma HTTP
   monitoring. When TASK-027 lands, the Uptime Kuma monitor will gain richer
   dependency status without any compose or workflow changes.

## For the Verifier

### Acceptance criteria mapping

| AC | Criterion | Where to verify |
|---|---|---|
| AC-1 | `demo/vN.N` tag triggers CI build and image push | Push `demo/v0.1` tag; confirm CD workflow runs in GitHub Actions; confirm images appear in ghcr.io/loskylp/nexusflow |
| AC-2 | Watchtower redeploys within 5 minutes | After AC-1: wait up to 5 min; confirm containers on staging host are running the new image SHA |
| AC-3 | Staging accessible at nexusflow.staging.nxlabs.cc with TLS | `curl -I https://nexusflow.staging.nxlabs.cc/` → HTTP 200 with valid TLS cert; `curl -I https://nexusflow.staging.nxlabs.cc/api/health` → HTTP 200 |
| AC-4 | Uptime Kuma monitors staging health endpoints | Check Uptime Kuma UI or status.nxlabs.cc for "NexusFlow Staging API" and "NexusFlow Staging Web" monitors showing green |
| AC-5 | Staging runs same Docker images as production | Compare image SHAs: `docker inspect --format '{{.RepoDigests}}' ghcr.io/loskylp/nexusflow/api:v1.0` on staging and production should match after TASK-036 |

### First-time staging deploy procedure

```bash
# 1. Provision the staging database
ssh deploy@nxlabs.cc /opt/postgres/provision.sh nexusflow_staging

# 2. Copy files to staging host
scp deploy/staging/docker-compose.yml deploy@nxlabs.cc:/opt/nexusflow-staging/docker-compose.yml
scp deploy/staging/.env.example deploy@nxlabs.cc:/opt/nexusflow-staging/.env.example

# 3. On the staging host: populate .env
ssh deploy@nxlabs.cc
cp /opt/nexusflow-staging/.env.example /opt/nexusflow-staging/.env
# Edit .env: set DATABASE_URL with credentials from provisioning script

# 4. Log in to ghcr.io (if packages are private)
docker login ghcr.io

# 5. Deploy the stack
cd /opt/nexusflow-staging
docker compose up -d

# 6. Verify
docker compose ps
curl -I https://nexusflow.staging.nxlabs.cc/api/health
```

### Triggering the CD pipeline

```bash
# From the project root, on a clean main branch:
make staging-tag V=v0.1
# or manually:
git tag demo/v0.1
git push origin demo/v0.1
```
