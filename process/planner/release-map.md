# Release Map -- NexusFlow
**Version:** 1 | **Date:** 2026-03-26
**Task Plan Version:** 1
**CD Philosophy:** Cycle-based (Demo Sign-off at cycle boundary, Nexus Go-Live decision for production)
**Status:** Living document -- updated each planning cycle

## MVP -- v1.0.0: Users can build pipelines, submit tasks, and monitor execution with auto-recovery
**Confidence:** Firm
**Version target:** v1.0.0

**Business value proposition:** A fully functional task orchestration system where users can visually build data pipelines, submit tasks via API or GUI, monitor real-time execution with log streaming, and rely on automatic worker failover -- the complete value proposition for the portfolio project.

**Scope:**

| Requirement | Features | Task count |
|---|---|---|
| REQ-001 | Task submission via REST API | 1 (TASK-005) |
| REQ-002 | Task submission via web GUI | 1 (TASK-035) |
| REQ-003 | Task queuing via Redis broker | 1 (TASK-004) |
| REQ-004 | Worker self-registration with heartbeat | 1 (TASK-006) |
| REQ-005 | Tag-based task-to-worker matching | 1 (TASK-007, shared) |
| REQ-006 | Three-phase pipeline execution | 1 (TASK-007, shared) |
| REQ-007 | Schema mapping between pipeline phases | 2 (TASK-007 runtime, TASK-026 design-time) |
| REQ-008 | Atomic sink operations with cleanup | 1 (TASK-018) |
| REQ-009 | Task lifecycle state tracking | 2 (TASK-007 transitions, TASK-008 query API) |
| REQ-010 | Cancel a running task | 1 (TASK-012) |
| REQ-011 | Infrastructure-failure retry | 1 (TASK-010) |
| REQ-012 | Dead letter queue with cascading cancellation | 1 (TASK-011) |
| REQ-013 | Auto-failover for downed workers | 1 (TASK-009) |
| REQ-014 | Linear pipeline chaining | 1 (TASK-014) |
| REQ-015 | Pipeline Builder (GUI) | 1 (TASK-023) |
| REQ-016 | Worker Fleet Dashboard (GUI) | 2 (TASK-025 API, TASK-020 GUI) |
| REQ-017 | Task Feed and Monitor (GUI) | 1 (TASK-021) |
| REQ-018 | Real-time log streaming | 2 (TASK-016 backend, TASK-022 GUI) |
| REQ-019 | User authentication and role-based access | 1 (TASK-003) |
| REQ-020 | Admin user management | 1 (TASK-017) |
| REQ-021 | Throughput capacity (10K tasks/hour) | 1 (TASK-037) |
| REQ-022 | Pipeline CRUD via REST API | 1 (TASK-013) |
| REQ-023 | Pipeline management via GUI | 1 (TASK-024) |
| NFR-001 | Queuing latency SLA (<50ms p95) | Covered by TASK-004 + TASK-037 |
| NFR-002 | Redis persistence and recovery | Covered by TASK-004 (AOF+RDB config) |
| NFR-003 | Real-time update latency (<2s) | Covered by TASK-015 (SSE infrastructure) |
| NFR-004 | Graceful degradation under worker loss | Covered by TASK-009 + TASK-010 |
| Infrastructure | CI pipeline, dev env, health endpoint, OpenAPI | 3 (TASK-001, TASK-027, TASK-028) |
| Infrastructure | React app shell, sidebar nav, design system | 1 (TASK-019) |
| Database | Schema, migrations, sqlc | 1 (TASK-002) |
| SSE | Event infrastructure | 1 (TASK-015) |

**Total task count:** 28 (TASK-001 through TASK-028)

**Intentionally excluded from MVP:**
- Demo infrastructure (DEMO-001 through DEMO-004): MinIO Fake-S3, Mock-Postgres, Sink Inspector, Chaos Controller -- these are demonstration features, not production requirements. They enhance the portfolio presentation but the core system is fully functional without them.
- Staging and production deployment (TASK-029, TASK-036) -- the MVP is demonstrated in the dev environment and validated via the staging environment before Go-Live.
- Pipeline template sharing (AUDIT-006 deferred) -- private ownership is the safe default.
- Rate limiting -- single-org deployment mitigates risk.
- Priority queuing -- FIFO within tag streams is sufficient for stated requirements.

**Release criterion:**
1. All 28 MVP tasks pass Verifier checks
2. Walking skeleton demo: end-to-end task submission, execution, and monitoring works in dev environment
3. Throughput test (TASK-037) passes: 10K tasks/hour sustained
4. All fitness function dev checks pass in CI
5. Demo Sign-off from Nexus at Cycle 1 boundary

**Ships when:** On Nexus Go-Live decision after Cycle 2 Demo Sign-off (production deployment happens in Cycle 2)

---

## Release 2 -- v1.1.0: Demo infrastructure and production deployment
**Confidence:** Planned
**Version target:** v1.1.0
**Depends on:** MVP (v1.0.0)

**Business value proposition:** Demo-ready system with Chaos Controller, Sink Inspector, and realistic demo scenarios (MinIO, Mock-Postgres). Production deployment live at nexusflow.nxlabs.cc. This is what makes the portfolio project presentable.

| Requirement | Features | Rough size |
|---|---|---|
| DEMO-001 | MinIO Fake-S3 integration | M |
| DEMO-002 | Mock-Postgres with 10K seed rows | M |
| DEMO-003 | Sink Inspector with Before/After snapshots | M |
| DEMO-004 | Chaos Controller (Kill Worker, Disconnect DB, Flood Queue) | L |
| ADR-005 | Staging environment, CD pipeline, production deployment | M |
| FF-010 | Throughput load test (if not completed in Cycle 1) | S |
| FF-* | Fitness function instrumentation (comprehensive) | M |

**Task count:** 10 (TASK-029 through TASK-038)

**Release criterion:**
1. All Cycle 2 tasks pass Verifier checks
2. Full demo scenario works: Chaos Controller triggers auto-recovery, Sink Inspector shows Before/After, audience can observe the full resilience story
3. Production deployment live at nexusflow.nxlabs.cc
4. Staging and production image SHAs match after release
5. Demo Sign-off from Nexus at Cycle 2 boundary

---

## Release 3+ -- Tentative
**Confidence:** Tentative -- scope pending feedback from production usage

Potential feature groups (not yet decomposed):
- Pipeline template sharing between users
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
| AUDIT-006: Pipeline template sharing | Deferred by Nexus decision; safe default (private ownership) is in place; additive feature for Release 3+ |

---

## Rolling Confidence Assessment

### Cycle 1 (MVP) -- Confidence: HIGH
**Rationale:** All architectural decisions are made (9 ADRs, no unresolved spikes). Technology stack is confirmed (Go + React + Redis + PostgreSQL). UX specification is complete with 7 approved screens. 28 tasks decomposed with testable acceptance criteria. The walking skeleton path is clear and dependencies are satisfiable. The solo developer has the Nexus's full authority to execute.

**Risk factors:**
- Pipeline Builder drag-and-drop (TASK-023) is the highest complexity frontend task -- but React ecosystem (dnd-kit/react-flow) is mature
- Pipeline execution with schema mapping (TASK-007) is the highest complexity backend task -- but the architecture specifies it clearly
- No external dependencies or integrations outside the team's control

### Cycle 2 (Demo + Production) -- Confidence: HIGH
**Rationale:** Demo infrastructure is well-scoped (MinIO and PostgreSQL containers). Chaos Controller requires container management from the API which is somewhat novel but bounded in scope. nxlabs.cc deployment conventions are documented. Staging-to-production workflow is straightforward (image retag + Watchtower).

**Risk factors:**
- First deployment to nxlabs.cc -- infrastructure conventions must be followed precisely
- Chaos Controller's "Disconnect Database" action requires novel simulation approach
- Load test (10K tasks/hour) may reveal performance issues requiring optimization
