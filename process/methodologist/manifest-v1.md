# Methodology Manifest
**Version:** v1 | **Date:** 2026-03-25 | **Project:** NexusFlow
**Profile:** Critical
**Artifact Weight:** Blueprint

---

## Changelog
- v1: Initial configuration — 2026-03-25

## Profile Rationale
The system processes background tasks (billing reconciliation, reports, data exports) that other teams depend on operationally. Failure causes SLA breaches and blocks downstream teams — not merely inconvenient, but disruptive to essential operations. The distributed architecture (Redis broker, worker fleet, auto-failover) adds deployment and observability complexity that warrants formal process controls. Nature drives the profile to Critical; Scale and architectural complexity reinforce it.

## Agents

| Agent | Status | Notes |
|---|---|---|
| Methodologist | Active | |
| Orchestrator | Active | |
| Analyst | Active | |
| Auditor | Active | |
| Architect | Active | |
| Designer | Skipped | No user-facing UI — this is a backend services system |
| Scaffolder | Active | Invoked when >=3 Builder tasks per cycle |
| Planner | Active | |
| Builder | Active | |
| Verifier | Active | |
| Sentinel | Active | Security review required at Critical — Redis access, queue poisoning, worker isolation |
| DevOps | Active | CI/CD pipeline, staging environment, Docker orchestration |
| Scribe | Active | Operational runbooks and API documentation required for a system other teams depend on |

### Acceptance criteria for skipped agents
- **Designer:** No user-facing interface exists. If a monitoring dashboard or admin UI is added in a future cycle, the Nexus should re-activate this agent via retrospective. The Orchestrator will flag if any task implies UI work.

## Documentation Requirements

| Agent | Produces | Depth |
|---|---|---|
| Analyst | Brief + Requirements List | Blueprint: full Definition of Done per REQ, acceptance criteria, SLA targets |
| Architect | ADRs + System Diagrams + Fitness Functions | Blueprint: formal ADRs for key decisions (broker choice, failover strategy, queue semantics), component diagrams, fitness functions for resilience and throughput |
| Planner | Task Plan + Release Map + Risk Matrix | Blueprint: full plan with dependency graph, risk matrix, and rollback considerations |
| Verifier | Verification Reports + Demo Scripts | Blueprint: full structured report per task, regression suite, system-level tests against staging |
| Sentinel | Security Assessment | Blueprint: threat model for Redis exposure, queue injection, worker trust boundaries |
| DevOps | Environment Contract + Pipeline Config + Runbook | Blueprint: full environment contract, CI/CD pipeline definition, deployment runbook |
| Scribe | API Docs + Operational Runbook + Architecture Guide | Blueprint: comprehensive operational documentation for dependent teams |

## Gate Configuration

| Gate | Status | Mode |
|---|---|---|
| Requirements Gate | Active | Formal: Nexus approves requirements and acceptance criteria before proceeding |
| Architecture Gate | Active | Formal: Nexus approves architecture decisions and ADRs before proceeding |
| Plan Gate | Active | Formal: Nexus approves task plan and release map before proceeding |
| Demo Sign-off | Active | Formal sign-off with security review: Nexus explores running software, Sentinel findings reviewed, retrospective question |
| Go-Live | Active | Continuous Delivery: deploy at Demo Sign-off after Nexus approval |

## Iteration Model

**Max iterations per task:** 4
**Convergence signal:** 2 consecutive iterations with non-decreasing failure count triggers escalation to Nexus rather than continuing the loop.
**Cycle scope:** Planner-defined — tasks are grouped into demonstrable increments at the Plan Gate. Each cycle ends with a Demo Sign-off. The Orchestrator executes only the tasks in the current cycle before moving to Demo Sign-off.
**CD philosophy:** Continuous Delivery — deploy at Demo Sign-off after Nexus approval. Given SLA obligations and downstream team dependencies, releases are deliberate, not automatic.

## Verifier Task Modes

The Verifier operates in one of two modes depending on whether DevOps Phase 2 (staging) is available. The Orchestrator declares the current mode at each Verifier invocation.

| Mode | When active | Test layers available |
|---|---|---|
| **Pre-staging** | Before DevOps Phase 2 completes (first Builder task only) | Unit tests + integration tests + acceptance tests. No system tests. |
| **Full** | After DevOps Phase 2 completes (all subsequent tasks) | All layers: unit, integration, acceptance, and system tests against staging. |

**Current mode:** Pre-staging

## Infrastructure Preconditions

**Before Builder tasks begin:**
- CI pipeline passing (DevOps Phase 1 complete)
- Dev environment accessible with Redis instance available
- Environment Contract produced by DevOps specifying Redis connection, queue names, worker configuration
- Docker build pipeline operational for local development

**Before each Demo Sign-off:**
- Staging must be reachable at its health endpoint and the application must be running
- DevOps confirms staging live before the Orchestrator opens the Demo Sign-off gate
- All Verifier system tests must have run against staging

## Deployment Workflow

**Model:** Tag-based (single-branch project)

Three triggers, three owners:

| Trigger | Who pushes | Pipeline action |
|---|---|---|
| Commit push to `main` (per task) | **Verifier** — after task PASS + local lint | Build + full regression test suite |
| Demo tag (e.g. `demo/v[cycle].[attempt]`) | **DevOps** — signalled by Orchestrator after all cycle tasks verified and CI green | Build + regression + Docker image build + push to staging |
| Release tag (e.g. `release/v[major].[minor]`) | **DevOps** — signalled by Orchestrator after Go-Live approved by Nexus | Retag the staging-validated Docker image to prod — no rebuild |

**Tag naming convention:**
- Demo tags: `demo/v[cycle].[attempt]` — e.g. `demo/v1.0` for Cycle 1 first attempt, `demo/v1.1` for a second attempt after a rejected demo
- Release tags: `release/v[major].[minor]` — e.g. `release/v1.0`; minor increments on cycle releases, major on breaking changes

**Image promotion rule:** the Docker image deployed to production is the exact image that passed Demo Sign-off — retag only, never rebuild from source. This guarantees the Nexus approved exactly what goes to prod.

## Provisional Assumptions
- **Team size assumed solo:** The Nexus described this as a portfolio project. If additional contributors join, the retrospective should reassess gate formality and coordination mechanisms.
- **No existing codebase:** Assumed greenfield. If there is existing code to integrate, the Analyst and Architect need to account for it.
- **Technology stack:** Redis is specified as the broker. Worker implementation language and framework are not yet specified — the Architect will determine these.

## Nexus Intake Note

The following is the Nexus's project description, preserved verbatim for the Analyst to structure into the Brief and Requirements List:

> This system solves the problem of blocking operations in high-demand applications through total decoupling. A 'Producer' microservice receives requests and deposits them into a high-speed data bus after validation. The heart of the orchestration is a Redis instance acting as a persistent broker. A fleet of 'Workers' monitors the queue and pulls tasks based on available capacity, allowing for traffic spikes without degradation of user experience. It implements a monitoring layer where each task has a traceable lifecycle. The orchestrator detects downed workers and automatically reassigns unfinished tasks to healthy nodes in the cluster to ensure resilience.
>
> In real life, other teams on the company would be blocked and SLA would not be met if this system fails.

---
**Next:** Invoke @nexus-orchestrator — the Manifest is ready and the swarm is configured.
