# Brief -- NexusFlow
**Version:** 1
**Date:** 2026-03-25
**Artifact Weight:** Blueprint

## Problem Statement

Production applications at the organization depend on background operations -- billing reconciliation, report generation, data exports -- that block downstream teams when they run synchronously or fail silently. The cost of failure is SLA breaches and operational disruption to other teams. NexusFlow decouples these operations through an asynchronous task orchestration system: a Producer microservice receives and validates requests, deposits them into a Redis-backed persistent broker, and a self-managing fleet of Workers pulls and executes tasks based on capacity and capability. A monitoring layer provides full lifecycle traceability, and an auto-failover mechanism reassigns tasks from downed workers to healthy nodes.

The system serves two interaction surfaces: a web GUI for visual pipeline building, fleet monitoring, task tracking, and log streaming; and a REST API for programmatic task submission and integration by other teams.

## Context and Ground Truths

- Other teams in the organization depend on the output of tasks processed by this system. SLA obligations are real and pre-existing.
- Redis is the chosen persistent broker. This is a Nexus-stated constraint, not an open decision.
- The system is greenfield -- no existing codebase to integrate with.
- Single organization deployment. No multi-tenancy across organizations.
- The Nexus is a solo developer building this as a portfolio project, but the system models a production-grade scenario with real operational consequences.

## Scope and Boundaries

**In scope:**
- Task submission (via REST API and web GUI)
- Task validation and queuing through Redis broker
- Worker fleet: self-registration, heartbeat monitoring, tag-based capability matching
- Task execution via three-phase pipeline: DataSource, Process, Sink
- Linear pipeline chaining (A -> B -> C)
- Schema mapping between pipeline stages
- Task lifecycle tracking (submitted, queued, assigned, running, completed, failed, cancelled)
- Infrastructure-failure retry with per-task configuration and safe defaults
- Dead letter queue with cascading cancellation for chained pipelines
- Auto-failover: detect downed workers, reassign their tasks to healthy nodes
- Web GUI: Pipeline Builder (drag-and-drop), Worker Fleet Dashboard, Task Feed and Monitor, Log Streamer
- REST API: programmatic task submission and status querying
- Real-time log streaming (GUI and web stream API)
- Atomic sink operations with cleanup on failure
- User management: admin and user roles
- Visibility isolation: users see all workers but only their own tasks

**Out of scope:**
- Multi-organization tenancy
- Branching or fan-out pipelines (deferred to a future phase; phase 1 is linear only)
- Process/script error retry (only infrastructure failures trigger retry)
- Data-level isolation between tenants (visibility isolation only)
- Mobile or desktop clients
- Worker auto-scaling (workers self-register; fleet sizing is manual)

**Adjacent (conscious exclusion):**
- The applications that produce tasks (upstream systems) -- NexusFlow receives requests but does not own the callers
- The downstream systems that consume task output -- NexusFlow delivers results via Sink but does not own what happens next
- Infrastructure provisioning (Redis, networking, DNS) -- assumed to exist; DevOps configures the application layer, not the infrastructure layer

## Delivery Channel

**Channel:** Hybrid -- Web App + REST API
**Decision status:** Nexus-stated
**Implications:** The web GUI (Pipeline Builder, Dashboard, Task Feed, Log Streamer) requires UX design decisions. The REST API requires formal API surface design. Both are first-class interaction surfaces. The Architect must account for serving both a frontend and a backend API from the same system. Real-time log streaming implies WebSocket or SSE support.

**Note for Orchestrator/Methodologist:** The Methodology Manifest v1 marks the Designer agent as "Skipped" with rationale "No user-facing UI -- this is a backend services system." This contradicts the Nexus's stated requirements: the system includes a web GUI with four distinct views (Pipeline Builder, Worker Fleet Dashboard, Task Feed and Monitor, Log Streamer). The Designer agent should be re-activated. This is flagged for the Auditor to escalate.

## Stakeholders

| Role | Relationship to system | Needs | Authority over requirements |
|---|---|---|---|
| Nexus (system owner) | Sole developer and product owner | A working, demonstrable task orchestration system for their portfolio | Full |
| Dependent teams (modeled) | Consumers of task output; affected by SLA breaches | Reliable task completion within SLA; visibility into task status | None (represented by SLA requirements) |
| Admin users (modeled) | Manage users and monitor the worker fleet | User CRUD, fleet health visibility, system-wide task visibility | Partial (operational authority) |
| Regular users (modeled) | Submit and monitor their own tasks | Task submission, pipeline building, task status tracking, log access | None (use the system as provided) |

## User Roles

| Role | Description | Goals | Permissions needed |
|---|---|---|---|
| Admin | System administrator from any team; manages users and has full visibility | Manage user accounts; view all tasks across all users; monitor worker fleet health; configure system-level settings | Create/read/update/delete users; read all tasks; read all workers; access all logs; manage pipelines |
| User | A team member who submits and monitors tasks | Define pipelines; submit tasks via GUI or API; monitor their own tasks; stream logs for their tasks; cancel their own running tasks | Create/read/update own pipelines; submit tasks; read own tasks; cancel own running tasks; read all workers (visibility); stream own task logs |

## Domain Model

### Key Concepts

| Term | Definition | Relationships |
|---|---|---|
| Task | A unit of work submitted to the system for asynchronous execution. Has a lifecycle (submitted -> queued -> assigned -> running -> completed/failed/cancelled). | Belongs to a User; assigned to a Worker; executes a Pipeline; produces Logs; may be part of a Pipeline Chain |
| Pipeline | A linear sequence of three phases -- DataSource, Process, Sink -- that defines how a Task's data flows from ingestion through transformation to output. | Composed of a DataSource, a Process, and a Sink; connected by Schema Mappings; owned by a User |
| DataSource | The first phase of a Pipeline. Responsible for ingesting data from an external source. | Part of a Pipeline; produces output conforming to a schema |
| Process | The second phase of a Pipeline. Applies transformation logic (script/operation) to the data received from the DataSource. | Part of a Pipeline; receives input from DataSource via Schema Mapping; produces output conforming to a schema |
| Sink | The third phase of a Pipeline. Writes the processed data to an external destination. Operations are atomic -- on failure, partial writes are cleaned up. | Part of a Pipeline; receives input from Process via Schema Mapping; atomic execution with rollback |
| Schema Mapping | A definition of how data fields from one pipeline phase map to input fields of the next phase. | Connects DataSource to Process and Process to Sink within a Pipeline |
| Pipeline Chain | A linear sequence of Pipelines where the completion of one triggers the next (A -> B -> C). If any Pipeline in the chain fails, downstream Pipelines are cancelled (cascading cancellation). | Composed of ordered Pipelines; failure cascades forward |
| Worker | A compute node that pulls Tasks from the queue and executes them. Self-registers with the system, emits heartbeats, and advertises capability tags. | Executes Tasks; has Capability Tags; monitored by heartbeat |
| Capability Tag | A label on a Worker indicating what types of work it can perform. Tasks are matched to Workers based on required tags. | Belongs to a Worker; used for task-to-worker matching |
| Dead Letter Queue | A holding area for Tasks that have exhausted retry attempts or failed in an unrecoverable way. | Receives failed Tasks; triggers cascading cancellation for Pipeline Chains |
| Retry Configuration | Per-task settings controlling how many times and with what backoff an infrastructure-failure retry is attempted. Has system-wide safe defaults. | Attached to a Task; governs infrastructure-failure retry only |
| Log | A stream of output produced during Task execution. Available in real-time via the GUI and web stream API. | Belongs to a Task; streamed to users |
| User | A person who interacts with the system. Either an Admin or a regular User. | Owns Tasks and Pipelines; managed by Admin |

### Domain Invariants

1. **Task lifecycle is monotonically forward.** A Task cannot return to a prior state (e.g., a "running" task cannot become "queued" again). The only exception is reassignment during failover, which returns an "assigned" or "running" task to "queued."
2. **Retry is infrastructure-only.** A Task that fails because its Process script errors does not retry. Only infrastructure failures (downed worker, network partition) trigger retry.
3. **Sink atomicity.** A Sink operation either completes fully or rolls back all partial writes. There is no partial-success state for a Sink.
4. **Cascading cancellation.** When a Task in a Pipeline Chain fails and enters the Dead Letter Queue, all downstream Tasks in the chain are cancelled.
5. **Visibility isolation.** A regular User can see all Workers but can only see their own Tasks. An Admin can see all Tasks and all Workers.
6. **Pipeline linearity (Phase 1).** Pipelines are strictly linear: one DataSource -> one Process -> one Sink. No branching or fan-out.
7. **Worker liveness.** A Worker that stops sending heartbeats within the configured timeout is considered down. Its assigned Tasks are eligible for reassignment.
8. **Cancel authority.** Only the submitting User (or an Admin) can cancel a running Task.

## Open Context Questions

1. **Log retention:** How long should task logs be retained? Is there a storage budget or TTL?
2. **Admin user management details:** Is admin user management full CRUD, or just invite/deactivate? Can admins delete users who own tasks?
3. **Pipeline template sharing:** Can users share pipeline definitions with other users, or are pipelines strictly private?
4. **Heartbeat timeout threshold:** What is an acceptable heartbeat timeout before a worker is declared down? (May be deferred to Architect as a configurable parameter.)
5. **Authentication mechanism:** Is there an existing auth system to integrate with, or does NexusFlow manage its own credentials?
6. **Schema mapping validation:** Should schema mappings be validated at pipeline definition time (design-time) or only at execution time?
