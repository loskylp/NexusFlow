# Release Map -- NexusFlow
**Version:** 2.1 | **Date:** 2026-03-26
**Task Plan Version:** 2.1
**CD Philosophy:** Cycle-based (Demo Sign-off at cycle boundary, Nexus Go-Live decision for production)
**Status:** Living document -- updated each planning cycle

---

## Cycle and Version Relationship

Cycles are internal development iterations. Production versions are release milestones. They are not 1:1. Multiple cycles compose a single version.

| Version | Cycles | Description |
|---|---|---|
| v1.0.0 | Cycle 1, Cycle 2, Cycle 3 | Full core system -- all Must Have requirements |
| v1.1.0 | Cycle 4, Cycle 5 | Demo infrastructure + production deployment |

---

## MVP -- v1.0.0: Users can build pipelines, submit tasks, and monitor execution with auto-recovery
**Confidence:** Firm
**Version target:** v1.0.0
**Composed of:** Cycle 1 (walking skeleton), Cycle 2 (core system), Cycle 3 (GUI + infra)

**Business value proposition:** A fully functional task orchestration system where users can visually build data pipelines, submit tasks via API or GUI, monitor real-time execution with log streaming, and rely on automatic worker failover -- the complete value proposition for the portfolio project.

**Scope:**

| Requirement | Features | Cycle | Task(s) |
|---|---|---|---|
| REQ-001 | Task submission via REST API | 1 | TASK-005 |
| REQ-002 | Task submission via web GUI | 3 | TASK-035 |
| REQ-003 | Task queuing via Redis broker | 1 | TASK-004 |
| REQ-004 | Worker self-registration with heartbeat | 1 | TASK-006 |
| REQ-005 | Tag-based task-to-worker matching | 1 | TASK-007 |
| REQ-006 | Three-phase pipeline execution | 1 | TASK-007 |
| REQ-007 | Schema mapping between pipeline phases | 1, 2 | TASK-007 (runtime), TASK-026 (design-time) |
| REQ-008 | Atomic sink operations with cleanup | 2 | TASK-018 |
| REQ-009 | Task lifecycle state tracking | 1, 2 | TASK-007 (transitions), TASK-008 (query API) |
| REQ-010 | Cancel a running task | 2 | TASK-012 |
| REQ-011 | Infrastructure-failure retry | 2 | TASK-010 |
| REQ-012 | Dead letter queue with cascading cancellation | 2 | TASK-011 |
| REQ-013 | Auto-failover for downed workers | 2 | TASK-009 |
| REQ-014 | Linear pipeline chaining | 2 | TASK-014 |
| REQ-015 | Pipeline Builder (GUI) | 3 | TASK-023 |
| REQ-016 | Worker Fleet Dashboard (GUI) | 1 | TASK-020 (GUI), TASK-025 (API) |
| REQ-017 | Task Feed and Monitor (GUI) | 3 | TASK-021 |
| REQ-018 | Real-time log streaming | 2, 3 | TASK-016 (backend), TASK-022 (GUI) |
| REQ-019 | User authentication and role-based access | 1 | TASK-003 |
| REQ-020 | Admin user management | 2 | TASK-017 |
| REQ-021 | Throughput capacity (10K tasks/hour) | 5 | TASK-037 |
| REQ-022 | Pipeline CRUD via REST API | 1 | TASK-013 |
| REQ-023 | Pipeline management via GUI | 3 | TASK-024 |
| NFR-001 | Queuing latency SLA (<50ms p95) | 1 | Covered by TASK-004 + TASK-037 |
| NFR-002 | Redis persistence and recovery | 1 | Covered by TASK-004 (AOF+RDB config) |
| NFR-003 | Real-time update latency (<2s) | 1 | Covered by TASK-015 (SSE infrastructure) |
| NFR-004 | Graceful degradation under worker loss | 2 | Covered by TASK-009 + TASK-010 |
| Infrastructure | CI pipeline, dev env | 1 | TASK-001 |
| Infrastructure | Staging environment, CD pipeline | 1 | TASK-029 |
| Infrastructure | Health endpoint, OpenAPI | 3 | TASK-027 |
| Infrastructure | React app shell, sidebar nav, design system | 1 | TASK-019 |
| Infrastructure | Log retention | 3 | TASK-028 |
| Database | Schema, migrations, sqlc | 1 | TASK-002 |
| SSE | Event infrastructure | 1 | TASK-015 |
| Walking skeleton | Demo connectors | 1 | TASK-042 |

**Total task count:** 31 (across Cycles 1-3, plus TASK-037 in Cycle 5)

**Intentionally excluded from MVP:**
- Demo infrastructure (DEMO-001 through DEMO-004): MinIO Fake-S3, Mock-Postgres, Sink Inspector, Chaos Controller -- these are demonstration features in v1.1.0
- Production deployment (TASK-036) -- in v1.1.0 (staging moved to Cycle 1 per Nexus directive)
- Rate limiting -- single-org deployment mitigates risk
- Priority queuing -- FIFO within tag streams is sufficient for stated requirements

**Release criterion:**
1. All Cycle 1, 2, and 3 tasks pass Verifier checks
2. Walking skeleton demo: end-to-end task submission, execution, and monitoring works in dev environment
3. All fitness function dev checks pass in CI (instrumented in Cycle 4)
4. Demo Sign-off from Nexus at each cycle boundary (Cycles 1, 2, 3)

**Ships when:** On Nexus Go-Live decision after Cycle 5 completion (production deployment in Cycle 5)

---

## Release 2 -- v1.1.0: Demo infrastructure and production deployment
**Confidence:** Planned
**Version target:** v1.1.0
**Composed of:** Cycle 4 (demo infrastructure), Cycle 5 (production deployment)
**Depends on:** v1.0.0

**Business value proposition:** Demo-ready system with Chaos Controller, Sink Inspector, and realistic demo scenarios (MinIO, Mock-Postgres). Production deployment live at nexusflow.nxlabs.cc. This is what makes the portfolio project presentable.

| Requirement | Features | Cycle | Rough size |
|---|---|---|---|
| DEMO-001 | MinIO Fake-S3 integration | 4 | M |
| DEMO-002 | Mock-Postgres with 10K seed rows | 4 | M |
| DEMO-003 | Sink Inspector with Before/After snapshots | 4 | M (TASK-033 + TASK-032) |
| DEMO-004 | Chaos Controller (Kill Worker, Disconnect DB, Flood Queue) | 4 | L |
| ADR-005 | Production deployment and monitoring | 5 | M |
| REQ-021 | Throughput load test (validation) | 5 | S |
| FF-* | Fitness function instrumentation (comprehensive) | 4 | M |

**Task count:** 8 (TASK-030, TASK-031, TASK-032, TASK-033, TASK-034, TASK-036, TASK-037, TASK-038)

**Release criterion:**
1. All Cycle 4 and Cycle 5 tasks pass Verifier checks
2. Full demo scenario works: Chaos Controller triggers auto-recovery, Sink Inspector shows Before/After, audience can observe the full resilience story
3. Production deployment live at nexusflow.nxlabs.cc
4. Staging and production image SHAs match after release
5. Throughput test passes: 10K tasks/hour sustained
6. Demo Sign-off from Nexus at Cycle 4 and Cycle 5 boundaries

---

## Release 3+ -- Tentative
**Confidence:** Tentative -- scope pending feedback from production usage

Potential feature groups (not yet decomposed):
- API rate limiting
- Priority queuing (multiple priority levels within tag streams)
- User deletion (vs. deactivation only)
- Pipeline versioning and rollback
- Dashboard customization and saved views
- Webhook notifications for task completion/failure

---

## Unplaced Requirements

| Requirement | Reason not yet placed |
|---|---|
| (none) | All approved requirements are placed. AUDIT-006 (pipeline template sharing) was DISMISSED by the Nexus -- it is not a requirement. |

---

## Rolling Confidence Assessment

### Cycle 1 (Walking Skeleton) -- Confidence: HIGH
**Rationale:** Focused scope (14 tasks) targets the walking skeleton plus staging deployment. All architectural decisions are made. Technology stack confirmed. DevOps Phase 1 is standard Docker + CI setup. Auth, worker registration, and task submission are well-understood patterns. Pipeline execution (TASK-007) is the highest-risk task but architecture specifies it clearly. Demo connectors (TASK-042) are lightweight by design. Worker Fleet Dashboard GUI has a complete UX spec. TASK-029 (staging deployment) is scheduled last in the cycle, after the walking skeleton exists, and follows documented nxlabs.cc conventions (ADR-005).

**Risk factors:**
- Pipeline execution with schema mapping (TASK-007) is the most complex task in the cycle
- SSE infrastructure (TASK-015) requires careful goroutine management in Go
- Staging deployment (TASK-029) is the first deployment to nxlabs.cc -- conventions must be followed precisely
- All are well-specified by the Architect; no unknowns remain

### Cycle 2 (Core System) -- Confidence: HIGH
**Rationale:** Builds on working walking skeleton. Monitor/failover (TASK-009) and sink atomicity (TASK-018) are the highest-risk tasks but both have clear ADR specifications. Remaining tasks (retry, DLQ, cancellation, chaining, log production, user management, task query API, schema validation) are well-understood patterns.

**Risk factors:**
- Monitor service (TASK-009) integrates heartbeat detection, XCLAIM, retry counting, and dead-letter routing
- Sink atomicity (TASK-018) is a one-way door with per-sink-type wrappers
- Pipeline chain idempotency (TASK-014) requires SET-NX guard

### Cycle 3 (GUI Completion) -- Confidence: HIGH
**Rationale:** All backend APIs exist from Cycles 1-2. GUI views have complete UX specifications and approved screen designs. React app shell with SSE utilities exists from Cycle 1. The most complex frontend task (Pipeline Builder TASK-023 with drag-and-drop) uses mature React ecosystem libraries.

**Risk factors:**
- Pipeline Builder drag-and-drop (TASK-023) is the highest complexity frontend task
- Task Feed (TASK-021) integrates multiple backend features in one view

### Cycle 4 (Demo Infrastructure) -- Confidence: HIGH
**Rationale:** Demo infrastructure is well-scoped. MinIO and Mock-Postgres are standard Docker containers. Chaos Controller requires container management from the API, which is the main risk. Sink Inspector depends on snapshot capture, which is a bounded addition to the existing Sink execution flow.

**Risk factors:**
- Chaos Controller's container management (docker exec or equivalent) and DB disconnection simulation are novel
- Sink snapshot capture adds latency to Sink execution

### Cycle 5 (Production Deployment) -- Confidence: HIGH
**Rationale:** Two tasks only. Staging environment already established in Cycle 1. nxlabs.cc deployment conventions are documented in ADR-005 and validated during Cycle 1 staging setup. Production deployment follows the same pattern. Throughput load test is the primary validation step.

**Risk factors:**
- Load test (10K tasks/hour) may reveal performance issues requiring optimization
- Production deployment follows patterns already proven in staging (Cycle 1)
