<!--
Copyright 2026 Pablo Ochendrowitsch

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Demo Script — TASK-001
**Feature:** DevOps Phase 1 — CI pipeline and dev environment
**Requirement(s):** FF-015, FF-020, ADR-004, ADR-005
**Environment:** Local — run from the project root with Docker running

---

## Scenario 1: Dev environment starts cleanly with docker compose up
**REQ:** FF-020

**Given:** Docker is running and `.env` exists at the project root (copy from `.env.example` if absent)

**When:** Run `docker compose up -d` from the project root, then wait approximately 20–30 seconds

**Then:** Run `docker compose ps` — all five core services (api, worker, monitor, redis, postgres) should appear in the output with state `running`. Redis and postgres should show `(healthy)`. The api container should show `(healthy)`.

**Notes:** The web service also starts (serving the React frontend on port 3000) but is not part of the core five required by the acceptance criterion. Worker and monitor have no healthcheck defined — they are healthy by virtue of running.

---

## Scenario 2: API health endpoint responds under the degraded condition expected at TASK-001
**REQ:** FF-020

**Given:** The dev environment is running (`docker compose up -d` has completed, api shows `healthy`)

**When:** Run `curl http://localhost:8080/api/health`

**Then:** The response should be `{"status":"degraded","redis":"ok","postgres":"error"}` with HTTP status 503. This is the correct and expected response at TASK-001: redis is connected and responding, but the postgres pool is not wired until TASK-002.

**Notes:** The Docker Compose healthcheck for the api service passes on both 200 and 503 — it checks that the response body contains a `"status"` field, not that the HTTP status is 200. So the api container is correctly declared `healthy` even while returning 503. This will change to a 200 response once TASK-002 wires the postgres pool.

---

## Scenario 3: Monorepo directory layout matches ADR-004
**REQ:** ADR-004

**Given:** The repository has been cloned or checked out

**When:** Run `ls` at the project root

**Then:** The following directories should be visible: `api/`, `worker/`, `monitor/`, `web/`, `internal/`, `cmd/`. No `src/` directory should appear at the root. Each of `cmd/api/`, `cmd/worker/`, and `cmd/monitor/` should contain a `main.go` file.

---

## Scenario 4: .env.example documents all environment variables
**REQ:** ADR-005

**Given:** The repository has been cloned or checked out

**When:** Open `.env.example` at the project root

**Then:** The file should contain documented entries for all configuration variables: DATABASE_URL, REDIS_URL, DB_PASSWORD, WORKER_TAGS, API_PORT, ENV, SESSION_TTL_HOURS, HEARTBEAT_INTERVAL_SECONDS, HEARTBEAT_TIMEOUT_SECONDS, PENDING_SCAN_INTERVAL_SECONDS, LOG_HOT_RETENTION_HOURS, LOG_COLD_RETENTION_HOURS. Each variable should have a comment explaining its purpose and, where applicable, a reference to the ADR that governs it.

---

## Scenario 5: CI pipeline is configured for push to main
**REQ:** FF-015

**Given:** The repository is connected to GitHub

**When:** Navigate to the repository's Actions tab on GitHub after any push to `main`

**Then:** A workflow run titled "CI" should appear. It should contain three jobs: "Go Build, Vet, and Test", "Frontend Build and Typecheck", and "Docker Build Smoke Test". The Go job should include steps for `go build ./...`, `go vet ./...`, staticcheck installation, `staticcheck ./...`, and `go test ./...`.

**Notes:** At TASK-001, the Go unit tests cover `internal/config` only (7 tests). The docker-build job runs after both the go and frontend jobs pass, and builds all four Docker images (api, worker, monitor, web).

---

## Teardown

After demonstrating, stop the dev environment:

```
docker compose down
```

To also remove persistent volumes (postgres data, redis data):

```
docker compose down -v
```
