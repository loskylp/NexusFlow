# Uptime Kuma — Staging Monitor Configuration

This document describes how Uptime Kuma monitors the NexusFlow staging environment at
`nexusflow.staging.nxlabs.cc`.

## Automatic configuration via AutoKuma

The nxlabs.cc infrastructure runs AutoKuma alongside Uptime Kuma. AutoKuma reads
Docker container labels prefixed with `kuma.` and creates/updates Uptime Kuma
monitors automatically when containers start.

The staging compose stack (`deploy/staging/docker-compose.yml`) uses the nxlabs.cc
convention of **groups** (one per system) and **tags** (one per category) to organise
monitors in the Uptime Kuma dashboard.

### Group

All NexusFlow Staging monitors are nested under a single collapsible group:

| Label | Value |
|---|---|
| `kuma.nexusflow-staging-group.group.name` | NexusFlow Staging |

The group is defined on the `api` container (the anchor container for the stack).

### Tags

Three category tags are defined on the `api` container and referenced by each monitor:

| Tag slug | Display name | Colour | Used by |
|---|---|---|---|
| `tag-nexusflow-backend` | Backend | `#FF9800` | api monitor |
| `tag-nexusflow-frontend` | Frontend | `#9C27B0` | web monitor |
| `tag-nexusflow-data` | Data | `#2196F3` | redis monitor |

### API health monitor

Defined on the `api` container.

| Label | Value |
|---|---|
| `kuma.nexusflow-staging-api.http.name` | NexusFlow Staging API |
| `kuma.nexusflow-staging-api.http.url` | https://nexusflow.staging.nxlabs.cc/api/health |
| `kuma.nexusflow-staging-api.http.parent_name` | nexusflow-staging-group |
| `kuma.nexusflow-staging-api.http.tag_names` | `[{"name": "tag-nexusflow-backend"}]` |

### Web frontend monitor

Defined on the `web` container.

| Label | Value |
|---|---|
| `kuma.nexusflow-staging-web.http.name` | NexusFlow Staging Web |
| `kuma.nexusflow-staging-web.http.url` | https://nexusflow.staging.nxlabs.cc/ |
| `kuma.nexusflow-staging-web.http.parent_name` | nexusflow-staging-group |
| `kuma.nexusflow-staging-web.http.tag_names` | `[{"name": "tag-nexusflow-frontend"}]` |

### Redis TCP port monitor

Defined on the `redis` container.

| Label | Value |
|---|---|
| `kuma.nexusflow-staging-redis.port.name` | NexusFlow Staging Redis |
| `kuma.nexusflow-staging-redis.port.hostname` | nexusflow-staging-redis-1 |
| `kuma.nexusflow-staging-redis.port.port` | 6379 |
| `kuma.nexusflow-staging-redis.port.parent_name` | nexusflow-staging-group |
| `kuma.nexusflow-staging-redis.port.tag_names` | `[{"name": "tag-nexusflow-data"}]` |

**Network note:** Redis is on the `internal` Docker network only — no host port is
exposed. The TCP port monitor will succeed only if the AutoKuma container is also
connected to that internal network. If AutoKuma runs in the shared infrastructure
network and cannot reach the container directly, the Redis port check will fail.
In that case, Redis health is still observable indirectly: the API `/api/health`
endpoint checks Redis connectivity and returns `{"redis":"ok"}` when the connection
is live — a Redis failure will cause the API monitor to report a degraded response.

### Worker and monitor services

`worker` and `monitor` do not expose any HTTP endpoint or TCP port accessible
outside the `internal` network. AutoKuma has no monitor type for process-level
or heartbeat-based health. These services are monitored indirectly:

- Worker health: the API `/api/health` response includes worker activity indicators
  visible via the `/api/workers` endpoint; a worker that stops heartbeating is
  detected by the `monitor` service and marked down in the database.
- Monitor health: if the monitor service stops, heartbeat timeout detection ceases;
  this is observable via the task failure rate increasing over time, not via an
  Uptime Kuma check.

Both services are included in the Docker Compose stack and Docker's own health
check (`restart: unless-stopped`) handles automatic restart on crash.

## Manual configuration (fallback)

If AutoKuma is not available on the target host, create the following monitors
manually in Uptime Kuma:

### Group: NexusFlow Staging

Create a group named "NexusFlow Staging" in Uptime Kuma. Nest all monitors below under it.

### Monitor 1: NexusFlow Staging API

- **Type:** HTTP(s)
- **Name:** NexusFlow Staging API
- **URL:** https://nexusflow.staging.nxlabs.cc/api/health
- **Heartbeat interval:** 60 seconds
- **Group:** NexusFlow Staging
- **Tags:** Backend

### Monitor 2: NexusFlow Staging Web

- **Type:** HTTP(s)
- **Name:** NexusFlow Staging Web
- **URL:** https://nexusflow.staging.nxlabs.cc/
- **Heartbeat interval:** 60 seconds
- **Group:** NexusFlow Staging
- **Tags:** Frontend

### Monitor 3: NexusFlow Staging Redis

- **Type:** TCP Port
- **Name:** NexusFlow Staging Redis
- **Hostname:** nexusflow-staging-redis-1 (only reachable if monitoring from within the internal network)
- **Port:** 6379
- **Heartbeat interval:** 60 seconds
- **Group:** NexusFlow Staging
- **Tags:** Data

## Fitness function mapping

| Fitness Function | Monitor | Warning threshold | Critical threshold |
|---|---|---|---|
| FF-025 | API + Web monitors | Availability < 99.5% | PostgreSQL connection failure (API health returns 503) |
| FF-021 | API monitor | Container restart > 2x in 10 min | Different image SHAs after release |
| FF-024 | Redis monitor (or API health redis field) | — | Redis unreachable (API health reports redis: down) |

## Status page

Uptime Kuma status page for all nxlabs.cc services:
https://status.nxlabs.cc
