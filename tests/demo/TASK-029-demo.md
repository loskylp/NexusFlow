# Demo Script — TASK-029
**Feature:** DevOps Phase 2 — staging environment and CD pipeline
**Requirement(s):** ADR-005, FF-021, FF-025
**Environment:** nxlabs.cc staging host; staging URL: https://nexusflow.staging.nxlabs.cc; GitHub Actions: github.com/loskylp/nexusflow/actions

---

## Scenario 1: Trigger the CD pipeline with a demo tag
**REQ:** ADR-005, FF-021

Given: you have write access to the NexusFlow repository and the walking skeleton (TASK-001 through TASK-042) is merged to main

When: you run `make staging-tag V=v0.1` from the project root (or `git tag demo/v0.1 && git push origin demo/v0.1`)

Then: navigate to github.com/loskylp/nexusflow/actions and observe a "CD" workflow run triggered by the tag `demo/v0.1`; the workflow job "Build and Push Docker Images" completes successfully; the log shows four images pushed to ghcr.io/loskylp/nexusflow: api:v0.1, worker:v0.1, monitor:v0.1, web:v0.1 (and each also tagged :latest)

**Notes:** The workflow uses GITHUB_TOKEN and requires no additional secrets. If the ghcr.io packages are private, the workflow will still push because it uses the built-in token with packages:write permission. The `staging-tag` Make target guards against a missing V parameter — running `make staging-tag` without V= will print an error and exit 1 without creating a tag.

---

## Scenario 2: Watchtower auto-redeploy within 5 minutes
**REQ:** ADR-005, FF-021

Given: the staging stack is running on the nxlabs.cc host (`docker compose -f deploy/staging/docker-compose.yml up -d`) and scenario 1 has completed (new :latest images are in ghcr.io)

When: you wait up to 5 minutes after the CD workflow completes

Then: on the staging host, `docker compose -f deploy/staging/docker-compose.yml ps` shows all five application services (api, worker, monitor, web, redis) as "running"; the Watchtower logs (`docker compose -f deploy/staging/docker-compose.yml logs watchtower`) show entries indicating it checked the registry and redeployed the services that had new images; the container image SHAs for api, worker, monitor, and web match the images just pushed by the CD workflow

**Notes:** Watchtower polls every 300 seconds (5 minutes). The first check after an image push may happen anywhere in that 5-minute window. If you want to force an immediate check, you can restart the watchtower container: `docker compose -f deploy/staging/docker-compose.yml restart watchtower`. Redis is not managed by Watchtower (no watchtower.enable label) and will not be redeployed automatically.

---

## Scenario 3: Staging accessible at nexusflow.staging.nxlabs.cc with TLS
**REQ:** ADR-005

Given: the staging stack is running and Traefik is operational on the nxlabs.cc host

When: you open a browser to https://nexusflow.staging.nxlabs.cc/ and navigate to https://nexusflow.staging.nxlabs.cc/api/health

Then: the browser shows a valid TLS certificate issued by Let's Encrypt for nexusflow.staging.nxlabs.cc; the root URL (https://nexusflow.staging.nxlabs.cc/) returns the NexusFlow React frontend (HTTP 200); the health endpoint (https://nexusflow.staging.nxlabs.cc/api/health) returns HTTP 200 with a JSON body containing a "status" field

**Notes:** If the TLS certificate has not yet been issued (first deploy), Traefik may briefly serve a self-signed certificate while the ACME HTTP-01 challenge completes. This resolves within a few minutes. Verify TLS by checking the certificate details in the browser — the issuer should be "Let's Encrypt" and the domain should match nexusflow.staging.nxlabs.cc. The API is routed at `/api/*` and the frontend at all other paths — both are served from the same domain via different Traefik router rules.

---

## Scenario 4: Uptime Kuma monitors staging health endpoints
**REQ:** ADR-005, FF-025

Given: the staging stack is running with AutoKuma present on the nxlabs.cc host (AutoKuma reads Docker labels and creates Uptime Kuma monitors automatically)

When: you navigate to https://status.nxlabs.cc and look for the "NexusFlow Staging" group, or open the Uptime Kuma admin UI and inspect the monitors list

Then: two monitors are visible in the "NexusFlow Staging" group: "NexusFlow Staging API" (polling https://nexusflow.staging.nxlabs.cc/api/health, expected status 200) and "NexusFlow Staging Web" (polling https://nexusflow.staging.nxlabs.cc/, expected status 200); both monitors show green (up) status

**Notes:** If AutoKuma is not installed on the host, the monitors must be created manually per the procedure in `deploy/staging/uptime-kuma.md`. The manual fallback document specifies both monitor configurations exactly. The API monitor will show "down" if the staging database or Redis is unreachable (the health endpoint returns 503 in degraded state, which AutoKuma's `expected_status=200` interprets as down — this is correct behaviour, not a bug).

---

## Scenario 5: Staging runs same Docker images as production
**REQ:** ADR-005, FF-021

Given: the CD pipeline has run for a specific version (e.g. demo/v0.1) and staging is deployed with that version

When: after TASK-036 (production deploy) is complete, you compare image digests between staging and production: `docker inspect --format '{{.RepoDigests}}' ghcr.io/loskylp/nexusflow/api:v0.1` on both hosts

Then: the RepoDigests output is identical on staging and production for the same version tag; both environments are running the same image SHAs, confirming that staging and production have bitwise-identical application images

**Notes:** At Cycle 1 Demo Sign-off, production (TASK-036) is not yet deployed — this scenario demonstrates the principle using the staging images. The full staging-to-production SHA consistency check is part of TASK-036 acceptance. For this demo, confirm that `docker images` on the staging host shows `ghcr.io/loskylp/nexusflow/api:v0.1` with the same digest as reported by the CD workflow's "Log pushed image tags" step.

---
