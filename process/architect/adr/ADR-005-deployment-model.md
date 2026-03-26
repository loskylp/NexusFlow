# ADR-005: Deployment Model

**Status:** Revised (supersedes v1)
**Date:** 2026-03-26
**Characteristic:** Deployability, Operability, Reliability

## Context

The system deploys to the nxlabs.cc production server (187.124.233.130, Ubuntu 24.04 LTS). The infrastructure provides: Traefik v3 as reverse proxy with automatic TLS via Let's Encrypt, Watchtower for automated container updates, shared PostgreSQL 16 with per-service provisioning, Uptime Kuma + AutoKuma for monitoring, CrowdSec for attack detection, and Netdata for VPS metrics.

The system has multiple runtime components: API server, worker processes, monitor service, frontend, and Redis (ADR-004). PostgreSQL is shared infrastructure -- provisioned per-service via the server's provisioning script. Redis is NOT shared infrastructure -- it must be managed as a service-owned container within NexusFlow's Docker Compose stack.

The Nexus is a solo developer (Brief -- Stakeholders). The deployment model must be operable without a dedicated platform team.

**Revision note:** This revision targets the nxlabs.cc infrastructure specification, replacing the generic Docker Compose deployment model from v1. Key changes: Traefik integration via Docker labels, Watchtower auto-updates, shared PostgreSQL (not self-hosted), service-managed Redis, and Uptime Kuma monitoring labels.

## Trade-off Analysis

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Docker Compose on nxlabs.cc with Traefik integration | Matches existing infrastructure; Traefik handles TLS and routing; Watchtower handles updates; solo developer can operate; PostgreSQL already provisioned | Single host (no multi-host failover); Redis is service-managed (operational responsibility); depends on nxlabs.cc infrastructure conventions | Acceptable for stated scale (10K tasks/hour, single organization) | Medium -- migrating to Kubernetes later requires K8s manifests but not application code changes |
| Kubernetes on nxlabs.cc | Auto-restart; scaling; rolling deploys; health checks native | Massive operational overhead for a solo developer; cluster management on a single VPS; overkill | Solo developer drowns in K8s operations | High -- K8s YAML, Helm charts, cluster provisioning |
| Bare metal / systemd | Zero container overhead | No reproducibility; dependency drift; manual process management; does not integrate with existing Traefik/Watchtower infrastructure | Works in dev, breaks in deployment | Medium -- containerize later |

## Decision

**Docker Compose on nxlabs.cc** with Traefik labels for routing and TLS, Watchtower for automated updates, shared PostgreSQL via infrastructure provisioning, and service-managed Redis.

### Service definitions

- `api` -- Go API server (1 instance); exposed via Traefik as `nexusflow.nxlabs.cc`
- `worker` -- Go worker process (N instances, scaled via `docker compose up --scale worker=N`)
- `web` -- React frontend served by nginx (1 instance); exposed via Traefik as `nexusflow.nxlabs.cc` (path-based or subdomain routing -- see below)
- `redis` -- Redis 7+ with AOF+RDB persistence (ADR-001); service-managed container with persistent volume
- `monitor` -- Go monitor service for heartbeat checking and pending entry scanning (ADR-002)

Demo infrastructure (DEMO-001 through DEMO-004):
- `minio` -- MinIO S3-compatible storage (DEMO-001: Fake-S3)
- `demo-postgres` -- Separate PostgreSQL instance pre-seeded with 10K rows (DEMO-002: Mock-Postgres)

### Environments

| Environment | Domain | Purpose |
|---|---|---|
| Production | `nexusflow.nxlabs.cc` | Live system |
| Staging | `nexusflow.staging.nxlabs.cc` | Pre-production validation |
| Development | `localhost` | Local development with docker compose |

### Docker Compose configuration (production)

```yaml
services:
  api:
    image: ghcr.io/nexusflow/api:latest
    restart: unless-stopped
    environment:
      - DATABASE_URL=postgresql://nexusflow:${DB_PASSWORD}@postgres:5432/nexusflow
      - REDIS_URL=redis://redis:6379
    networks:
      - traefik
      - postgres
      - internal
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.nexusflow-api.rule=Host(`nexusflow.nxlabs.cc`) && PathPrefix(`/api`)"
      - "traefik.http.routers.nexusflow-api.tls.certresolver=letsencrypt"
      - "traefik.http.routers.nexusflow-api.entrypoints=websecure"
      - "com.centurylinklabs.watchtower.enable=true"
      - "kuma.nexusflow-api.http.name=NexusFlow API"
      - "kuma.nexusflow-api.http.url=https://nexusflow.nxlabs.cc/api/health"
      - "kuma.nexusflow-api.http.group=NexusFlow"
      - "kuma.nexusflow-api.http.expected_status=200"

  web:
    image: ghcr.io/nexusflow/web:latest
    restart: unless-stopped
    networks:
      - traefik
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.nexusflow-web.rule=Host(`nexusflow.nxlabs.cc`)"
      - "traefik.http.routers.nexusflow-web.tls.certresolver=letsencrypt"
      - "traefik.http.routers.nexusflow-web.entrypoints=websecure"
      - "com.centurylinklabs.watchtower.enable=true"
      - "kuma.nexusflow-web.http.name=NexusFlow Web"
      - "kuma.nexusflow-web.http.url=https://nexusflow.nxlabs.cc/"
      - "kuma.nexusflow-web.http.group=NexusFlow"
      - "kuma.nexusflow-web.http.expected_status=200"

  worker:
    image: ghcr.io/nexusflow/worker:latest
    restart: unless-stopped
    environment:
      - DATABASE_URL=postgresql://nexusflow:${DB_PASSWORD}@postgres:5432/nexusflow
      - REDIS_URL=redis://redis:6379
    networks:
      - postgres
      - internal
    labels:
      - "com.centurylinklabs.watchtower.enable=true"

  monitor:
    image: ghcr.io/nexusflow/monitor:latest
    restart: unless-stopped
    environment:
      - DATABASE_URL=postgresql://nexusflow:${DB_PASSWORD}@postgres:5432/nexusflow
      - REDIS_URL=redis://redis:6379
    networks:
      - postgres
      - internal
    labels:
      - "com.centurylinklabs.watchtower.enable=true"

  redis:
    image: redis:7-alpine
    restart: unless-stopped
    command: redis-server --appendonly yes --appendfsync everysec --save 900 1 --save 300 10 --save 60 10000
    volumes:
      - redis-data:/data
    networks:
      - internal
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 3

  minio:
    image: minio/minio:latest
    restart: unless-stopped
    command: server /data
    environment:
      - MINIO_ROOT_USER=minioadmin
      - MINIO_ROOT_PASSWORD=minioadmin
    volumes:
      - minio-data:/data
    networks:
      - internal
    profiles:
      - demo

  demo-postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      - POSTGRES_DB=demo
      - POSTGRES_USER=demo
      - POSTGRES_PASSWORD=demo
    volumes:
      - demo-pg-data:/var/lib/postgresql/data
    networks:
      - internal
    profiles:
      - demo

volumes:
  redis-data:
  minio-data:
  demo-pg-data:

networks:
  traefik:
    external: true
  postgres:
    external: true
  internal:
    driver: bridge
```

### Key infrastructure integration points

**PostgreSQL (shared infrastructure):**
- Provisioned via: `ssh deploy@nxlabs.cc /opt/postgres/provision.sh nexusflow`
- Connected via the `postgres` Docker network (hostname `postgres`, port 5432)
- Credentials managed per-service; stored in `.env` on the deployment host
- NexusFlow does NOT run its own PostgreSQL container in production -- it uses the shared instance

**Redis (service-managed):**
- Redis is NOT available as shared infrastructure on nxlabs.cc
- NexusFlow manages its own Redis container within the Docker Compose stack
- Persistence: AOF (appendfsync everysec) + RDB snapshots (ADR-001)
- Data volume: `redis-data` mounted to `/data` inside the container
- Accessible only within the `internal` Docker network -- not exposed externally

**Traefik (reverse proxy):**
- Routes traffic via Docker labels -- no central Traefik configuration needed
- Auto-TLS via Let's Encrypt ACME HTTP challenge
- API server exposed at `nexusflow.nxlabs.cc/api/*`
- Web frontend exposed at `nexusflow.nxlabs.cc` (all non-`/api` paths)
- Services join the external `traefik` Docker network to be discovered

**Watchtower (auto-updates):**
- Polls container registries every 5 minutes
- Enabled per-service via `com.centurylinklabs.watchtower.enable=true` label
- Supports CI/CD push-to-deploy: CI pushes new image to registry, Watchtower detects and redeploys

**Uptime Kuma (monitoring):**
- Self-registration via `kuma.*` Docker labels on exposed services (api, web)
- Monitors health endpoint availability
- Status page at https://status.nxlabs.cc

### Deployment workflow

**CI/CD pipeline:**
1. Commit to `main` -- CI runs tests, builds Go binaries, builds Docker images
2. On `demo/vN.N` tag -- CI pushes images tagged with version to registry; Watchtower picks up staging images (staging compose uses version-tagged images)
3. On `release/vN.N` tag -- CI retags staging images as `latest`; Watchtower on production picks up the new `latest` tag

**Manual deployment (fallback):**
```bash
ssh deploy@nxlabs.cc
cd /opt/nexusflow
docker compose pull && docker compose up -d
```

**First-time setup:**
```bash
# Provision PostgreSQL database
ssh deploy@nxlabs.cc /opt/postgres/provision.sh nexusflow

# Deploy stack
ssh deploy@nxlabs.cc
mkdir -p /opt/nexusflow
cd /opt/nexusflow
# Copy docker-compose.yml and .env
docker compose up -d
```

**Door type:** Two-way -- Docker Compose to Kubernetes migration requires writing K8s manifests but zero application code changes. The containerization (Dockerfiles) and infrastructure conventions (environment variables, health endpoints) transfer directly.

**Cost to change later:** Medium -- the orchestration layer changes but the application does not. Dockerfiles, health endpoints, and environment variable configuration are reusable. The nxlabs.cc-specific labels (Traefik, Watchtower, Uptime Kuma) would be replaced by their Kubernetes equivalents.

## Rationale

**nxlabs.cc infrastructure** provides everything this project needs without additional provisioning: reverse proxy with auto-TLS, monitoring, metrics, attack detection, and a shared PostgreSQL instance. The solo developer does not need to set up or maintain any of these -- they are already operational.

**Docker Compose** satisfies every deployment requirement in the Manifest:
- **Tag-based workflow:** CI builds Docker images; Watchtower auto-deploys when new images are pushed to the registry. This is simpler than manual `docker compose pull` for every deploy.
- **Reproducible environments:** Same Docker images in dev, staging, and production. Environment differences are limited to compose override files and `.env` files.
- **Solo developer operability:** The nxlabs.cc infrastructure handles TLS, monitoring, metrics, and security. The developer focuses on the application.
- **Worker scaling:** `docker compose up --scale worker=3` provides manual fleet scaling.

**Service-managed Redis** because Redis is not yet provisioned as shared infrastructure on nxlabs.cc. Running Redis as a container within the NexusFlow compose stack keeps it isolated, with persistence configured via AOF+RDB (ADR-001). The `redis-data` Docker volume ensures data survives container restarts. If Redis becomes shared infrastructure in the future, the migration is a configuration change (update REDIS_URL to point to the shared instance, remove the redis service from compose).

**Shared PostgreSQL** because nxlabs.cc already runs PostgreSQL 16 as shared infrastructure with per-service isolation (separate database and user per service). Using the shared instance avoids running a second PostgreSQL container and leverages the existing backup and maintenance procedures.

### Health endpoints

Every service exposes a health endpoint:
- API: `GET /api/health` -- checks Redis and PostgreSQL connectivity
- Worker: health reported via heartbeat mechanism (ADR-002)
- Web: nginx returns 200 on `/` if static assets are served
- Redis: `redis-cli ping` via Docker healthcheck

These support both Uptime Kuma monitoring (external) and Docker Compose healthcheck (internal).

### Environment configuration

All configuration via environment variables (12-factor). A `.env.example` file documents every variable. Docker Compose interpolates from `.env` files per environment.

### Network topology

```mermaid
graph LR
    classDef external fill:#e8d4b8,stroke:#9e6b2d,color:#2e1a0a,font-weight:bold
    classDef service  fill:#b8e8c9,stroke:#2d9e5a,color:#0a1e0a,font-weight:bold
    classDef infra    fill:#b8d4e8,stroke:#2d6b9e,color:#0a1a2e,font-weight:bold

    Internet["Internet"]:::external

    subgraph "nxlabs.cc (187.124.233.130)"
        Traefik["Traefik v3<br/>:80 :443"]:::infra

        subgraph "NexusFlow Stack (/opt/nexusflow)"
            api["api (Go)"]:::service
            web["web (nginx)"]:::service
            worker["worker (Go) x N"]:::service
            monitor["monitor (Go)"]:::service
            redis["redis 7<br/>(service-managed)"]:::service
        end

        PG["PostgreSQL 16<br/>(shared infra)"]:::infra
        Kuma["Uptime Kuma"]:::infra
    end

    Internet --> Traefik
    Traefik -->|"nexusflow.nxlabs.cc/api/*"| api
    Traefik -->|"nexusflow.nxlabs.cc/*"| web
    api --> redis
    api --> PG
    worker --> redis
    worker --> PG
    monitor --> redis
    monitor --> PG
    Kuma -.->|"health checks"| api
    Kuma -.->|"health checks"| web
```

## Fitness Function
**Characteristic threshold:** All services start and pass health checks; staging matches production topology; Watchtower auto-deploys within 5 minutes of image push; Redis data persists across container restarts

| | Specification |
|---|---|
| **Dev check** | CI verifies: `docker compose build` succeeds; `docker compose up` starts all services; health endpoints respond within 30 seconds; `docker compose down` tears down cleanly. Redis persistence test: write data, restart redis container, verify data is retained. |
| **Prod metric** | Container restart count; health endpoint response time; Uptime Kuma availability percentage; Redis memory usage; image SHA consistency between staging and production. |
| **Warning threshold** | Any container restarting more than twice in 10 minutes; health endpoint response > 5s; Redis memory > 75% of available; Uptime Kuma reports availability < 99.5% |
| **Critical threshold** | Any core service (api, worker, redis) not running; staging and production running different image SHAs after a release; Redis data loss after container restart; PostgreSQL connection failure |
| **Alarm meaning** | Warning: a service is unstable or Redis is approaching memory limits -- check logs for crash loops or memory leaks. Critical: the system is partially down, deployment integrity is compromised, or data persistence has failed. |

## Consequences
**Easier:** Local development (one command); CI/CD pipeline (build images, push, Watchtower auto-deploys); TLS and routing handled by existing Traefik; monitoring handled by existing Uptime Kuma; PostgreSQL already provisioned and maintained; demo infrastructure included as compose profiles.
**Harder:** Redis is service-managed -- NexusFlow is responsible for its persistence, memory limits, and health; single host means no automatic failover of the infrastructure itself; Traefik label conventions must be followed precisely.
**Newly required:** Dockerfiles for api, worker, web, and monitor services (Go multi-stage builds); docker-compose.yml with Traefik labels, Watchtower labels, Uptime Kuma labels, and Redis persistence configuration; health endpoint implementation in each Go service; `.env` management for database credentials; CI pipeline that builds and pushes images to container registry.
