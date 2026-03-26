# Routing Instruction

**To:** Architect
**Phase:** Design
**Task:** Produce system architecture (component diagrams, deployment model), ADRs for key decisions, and fitness functions for NexusFlow. Resolve three deferred audit items that have Architecture Gate as their deadline.
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| Approved requirements | [Requirements v5](../analyst/requirements.md) | The 31 approved requirements that the architecture must satisfy |
| Brief (Domain Model) | [Brief v1](../analyst/brief.md) | Domain vocabulary, scope boundaries, delivery channel, stakeholder context |
| Audit report (deferrals) | [Audit v4](../auditor/requirements-audit-v4.md) | Three deferrals the Architect must resolve (AUDIT-005, AUDIT-007, AUDIT-009) |
| Methodology Manifest | [Manifest v2](../methodologist/manifest-v2.md) | Profile (Critical), documentation depth (Blueprint), agent configuration, deployment workflow |

---

## Context

### Deferrals requiring resolution at Architecture Gate

The Auditor flagged three items as DEFERRED with a deadline of Architecture Gate. The Architect must address each in the architecture artifacts:

1. **AUDIT-005 -- Log retention policy.** REQ-018 covers real-time log streaming but no retention duration or storage TTL is specified. The Architect must include a log retention strategy in the architecture (duration, storage mechanism, TTL/rotation policy).

2. **AUDIT-007 -- Schema mapping validation timing.** REQ-007 specifies runtime schema mapping behavior. The Auditor deferred the question of whether schema mappings should also be validated at pipeline definition time (design-time). The Architect must decide and document whether design-time validation is in scope for Cycle 1 or deferred.

3. **AUDIT-009 -- Sink Inspector "Before" state capture.** DEMO-003 requires a Before/After comparison in the Sink Inspector but does not define when or how the "Before" snapshot is taken. The Architect must specify the capture mechanism (e.g., snapshot before Sink execution, CDC, shadow copy).

### Key architectural decisions expected

The Manifest identifies the following as requiring formal ADRs at Blueprint weight:

- Broker choice (Redis is a Nexus constraint -- ADR should document the rationale and configuration: persistence mode, queue semantics)
- Failover strategy (heartbeat detection, reassignment protocol, interaction with retry configuration)
- Queue semantics (FIFO vs. priority, at-least-once vs. exactly-once delivery guarantees)
- Worker implementation language and framework (not yet specified -- Architect determines)
- Technology stack for the web GUI and REST API
- Real-time update mechanism (WebSocket, SSE, or other for GUI live updates and log streaming)
- Deployment model (Docker orchestration per Manifest; the Manifest specifies a tag-based single-branch deployment workflow)

### Delivery channel

Hybrid -- Web App + REST API. The web GUI includes four views (Pipeline Builder with drag-and-drop canvas, Worker Fleet Dashboard, Task Feed and Monitor, Log Streamer) plus two demo panels (Sink Inspector, Chaos Controller). The REST API is a first-class interaction surface for programmatic integration by other teams.

### Deployment workflow (from Manifest)

Tag-based on a single branch (`main`). Three triggers: commit push (CI), demo tag (staging deploy), release tag (production deploy via image retag). The Architect should design the system to support this workflow -- containerized services, health endpoints, configuration management.

### Fourth deferral (not for Architect)

AUDIT-006 (pipeline template sharing between users) is deferred to before Cycle 2 planning. The Architect does not need to resolve this now but should ensure the pipeline ownership model does not preclude a future sharing capability.
