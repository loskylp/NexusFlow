# Uptime Kuma — Staging Monitor Configuration

This document describes how Uptime Kuma monitors the NexusFlow staging environment at
`nexusflow.staging.nxlabs.cc`.

## Automatic configuration via AutoKuma

The nxlabs.cc infrastructure runs AutoKuma alongside Uptime Kuma. AutoKuma reads
Docker container labels prefixed with `kuma.` and creates/updates Uptime Kuma
monitors automatically when containers start.

The staging compose stack (`deploy/staging/docker-compose.yml`) declares the following
monitor labels:

### API health monitor

| Label | Value |
|---|---|
| `kuma.nexusflow-staging-api.http.name` | NexusFlow Staging API |
| `kuma.nexusflow-staging-api.http.url` | https://nexusflow.staging.nxlabs.cc/api/health |
| `kuma.nexusflow-staging-api.http.group` | NexusFlow Staging |
| `kuma.nexusflow-staging-api.http.expected_status` | 200 |

### Web frontend monitor

| Label | Value |
|---|---|
| `kuma.nexusflow-staging-web.http.name` | NexusFlow Staging Web |
| `kuma.nexusflow-staging-web.http.url` | https://nexusflow.staging.nxlabs.cc/ |
| `kuma.nexusflow-staging-web.http.group` | NexusFlow Staging |
| `kuma.nexusflow-staging-web.http.expected_status` | 200 |

When `docker compose up` is run, AutoKuma detects these labels and creates both
monitors in Uptime Kuma. No manual configuration in the Uptime Kuma UI is needed.

## Manual configuration (fallback)

If AutoKuma is not available on the target host, create the following monitors
manually in Uptime Kuma:

### Monitor 1: NexusFlow Staging API

- **Type:** HTTP(s)
- **Name:** NexusFlow Staging API
- **URL:** https://nexusflow.staging.nxlabs.cc/api/health
- **Heartbeat interval:** 60 seconds
- **Expected HTTP status:** 200
- **Group:** NexusFlow Staging

### Monitor 2: NexusFlow Staging Web

- **Type:** HTTP(s)
- **Name:** NexusFlow Staging Web
- **URL:** https://nexusflow.staging.nxlabs.cc/
- **Heartbeat interval:** 60 seconds
- **Expected HTTP status:** 200
- **Group:** NexusFlow Staging

## Fitness function mapping

| Fitness Function | Monitor | Warning threshold | Critical threshold |
|---|---|---|---|
| FF-025 | Both monitors | Availability < 99.5% | PostgreSQL connection failure (API health returns 503) |
| FF-021 | API monitor | Container restart > 2x in 10 min | Different image SHAs after release |

## Status page

Uptime Kuma status page for all nxlabs.cc services:
https://status.nxlabs.cc
