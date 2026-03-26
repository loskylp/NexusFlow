# Fitness Functions Index -- NexusFlow
**Version:** 2 | **Date:** 2026-03-26
**Revision:** Updated for Go backend + nxlabs.cc deployment

This index lists all fitness functions defined in the architecture. Each row references its defining ADR for full context (thresholds, rationale, alarm meaning). Agents that need to enumerate fitness functions (Planner, Verifier) read this index and follow pointers to ADRs.

---

## Reliability and Data Integrity

| ID | Characteristic | Metric | Dev Check | Warning | Critical | Defining ADR |
|---|---|---|---|---|---|---|
| FF-001 | Queue persistence | Zero task loss across Redis restart | Enqueue 100 tasks, restart Redis, verify all recoverable | -- | Any task lost after restart | ADR-001 |
| FF-002 | Queuing latency | XADD p95 latency | Enqueue 1,000 tasks, assert p95 < 50ms | p95 > 30ms | p95 > 45ms | ADR-001 |
| FF-003 | Queue backlog | Pending entry count per stream | -- | Pending > 100 per stream | Pending > 500 per stream | ADR-001 |
| FF-004 | Delivery guarantee | Zero duplicate Sink writes | Kill worker mid-execution, verify redelivery completes once at Sink | Redelivery rate > 5% | Any duplicate Sink write | ADR-003 |
| FF-005 | Chain trigger dedup | Zero duplicate chain triggers | Simulate duplicate completion, verify one downstream task | Any dedup rejection (investigate) | Duplicate chain trigger created | ADR-003 |
| FF-006 | Sink atomicity | Zero partial writes | Force Sink failure mid-write, verify rollback | Sink failure rate > 5% | Any partial write detected | ADR-009 |

## Resilience and Failover

| ID | Characteristic | Metric | Dev Check | Warning | Critical | Defining ADR |
|---|---|---|---|---|---|---|
| FF-007 | Failover detection | Detection-to-reassignment latency | Kill worker with 3 in-flight tasks, verify reassignment to healthy worker | Latency > 30s | Latency > 60s | ADR-002 |
| FF-008 | Task recovery | Zero orphaned tasks | Kill worker, verify no task stuck as pending > 60s | > 3 failover events in 5 min (flapping) | Any task orphaned > 60s | ADR-002 |
| FF-009 | Fleet resilience | System operates under 50% fleet loss | Kill 50% of workers, verify all tasks eventually complete | Worker count drops below 2 | Worker count drops to 0 | ADR-002 |

## Performance and Throughput

| ID | Characteristic | Metric | Dev Check | Warning | Critical | Defining ADR |
|---|---|---|---|---|---|---|
| FF-010 | Throughput capacity | 10,000 tasks/hour processed | Load test: submit 10K tasks, verify all reach terminal state in 1 hour | -- | Any task dropped or lost | REQ-021 (architecture supports via ADR-001 stream topology) |
| FF-011 | API response time | p95 non-queuing endpoint latency | Smoke test response times | p95 > 100ms | p95 > 500ms | ADR-004 |
| FF-012 | Real-time latency | SSE event delivery latency | Change task state, assert SSE receives within 2s | p95 > 1.5s | p95 > 2s (NFR-003 breach) | ADR-007 |

## Security and Auth

| ID | Characteristic | Metric | Dev Check | Warning | Critical | Defining ADR |
|---|---|---|---|---|---|---|
| FF-013 | Auth enforcement | No request bypasses auth | Test suite: unauthenticated -> 401; wrong role -> 403; deactivated -> 401 | Login failure rate > 20% | Any unauthenticated request returns 200 | ADR-006 |
| FF-014 | Session performance | Session lookup latency | -- | p95 > 5ms | p95 > 50ms | ADR-006 |

## Maintainability and Type Safety

| ID | Characteristic | Metric | Dev Check | Warning | Critical | Defining ADR |
|---|---|---|---|---|---|---|
| FF-015 | Compile-time safety | Zero compilation errors; go vet and staticcheck clean | Go build succeeds; `go vet` passes; `staticcheck` passes; sqlc compile succeeds | -- | Go build failure in CI | ADR-004 |
| FF-016 | Frontend bundle | Bundle size | -- | > 2MB | -- | ADR-004 |

## Data Management

| ID | Characteristic | Metric | Dev Check | Warning | Critical | Defining ADR |
|---|---|---|---|---|---|---|
| FF-017 | Schema migration | Zero data loss during migration | Apply migrations to fresh + seeded DB, verify schema matches sqlc expectations | Migration > 30s | Migration failure in CI/staging | ADR-008 |
| FF-018 | Log retention | Logs pruned within 1 day of threshold | Insert old logs, run pruning, verify removal | task_logs > 10GB | Partitions not pruned > 7 days past retention | ADR-008 |
| FF-019 | Schema validation | Invalid mappings rejected at design-time | Save pipeline with invalid mapping, assert rejection | -- | Invalid mapping saved without error | ADR-008 |

## Deployment and Operability

| ID | Characteristic | Metric | Dev Check | Warning | Critical | Defining ADR |
|---|---|---|---|---|---|---|
| FF-020 | Service startup | All services start from single command | `docker compose up` succeeds; health endpoints respond in 30s | -- | Any core service not running | ADR-005 |
| FF-021 | Image integrity | Staging and production run same image | -- | Container restart > 2x in 10 min | Different image SHAs after release | ADR-005 |
| FF-024 | Redis persistence | Data survives container restart | Write data, restart redis container, verify data retained | Redis memory > 75% of available | Redis data loss after restart | ADR-005 |
| FF-025 | Infrastructure health | Uptime Kuma availability | -- | Availability < 99.5% | PostgreSQL connection failure | ADR-005 |

## Observability (Demo Infrastructure)

| ID | Characteristic | Metric | Dev Check | Warning | Critical | Defining ADR |
|---|---|---|---|---|---|---|
| FF-022 | Sink Inspector | Before/After snapshots captured | Run Sink, verify Before and After JSON differ by Sink output | Snapshot capture > 5s | Snapshot capture failure | ADR-009 |
| FF-023 | SSE reconnection | Log replay on reconnect | Disconnect SSE, produce 5 log lines, reconnect with Last-Event-ID, verify replay | Reconnection rate > 10%/min | SSE endpoint errors | ADR-007 |
