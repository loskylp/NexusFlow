# Dependency Graph -- NexusFlow Task Plan
**Version:** 2.2 | **Date:** 2026-03-27

## Cycle 1 -- MVP Walking Skeleton

```mermaid
flowchart TD
    classDef infra    fill:#e8d4b8,stroke:#9e6b2d,color:#2e1a0a,font-weight:bold
    classDef backend  fill:#b8e8c9,stroke:#2d9e5a,color:#0a1e0a,font-weight:bold
    classDef frontend fill:#b8d4e8,stroke:#2d6b9e,color:#0a1a2e,font-weight:bold
    classDef p1       fill:#c9b8e8,stroke:#6b3fa0,color:#1a0a2e,font-weight:bold

    T001["TASK-001<br/>DevOps Phase 1<br/>CI + Dev Env"]:::infra

    T002["TASK-002<br/>DB Schema +<br/>Migrations"]:::infra
    T004["TASK-004<br/>Redis Streams<br/>Queue Infra"]:::backend

    T003["TASK-003<br/>Auth + Sessions"]:::backend
    T006["TASK-006<br/>Worker Registration<br/>+ Heartbeat"]:::backend

    T005["TASK-005<br/>Task Submission<br/>REST API"]:::backend
    T013["TASK-013<br/>Pipeline CRUD<br/>REST API"]:::backend

    T007["TASK-007<br/>Task Assignment +<br/>Pipeline Execution"]:::p1

    T042["TASK-042<br/>Demo Connectors<br/>(Source + Worker + Sink)"]:::backend

    T019["TASK-019<br/>React App Shell +<br/>Sidebar + Auth"]:::frontend
    T025["TASK-025<br/>Worker Fleet<br/>Status API"]:::backend
    T015["TASK-015<br/>SSE Event<br/>Infrastructure"]:::p1
    T020["TASK-020<br/>Worker Fleet<br/>Dashboard GUI"]:::frontend

    T001 --> T002
    T001 --> T004

    T002 --> T003
    T002 --> T006
    T002 --> T005
    T002 --> T013

    T003 --> T005
    T003 --> T013
    T003 --> T019
    T003 --> T025
    T003 --> T015

    T004 --> T005
    T004 --> T007
    T004 --> T015

    T005 --> T007
    T006 --> T007
    T013 --> T007

    T007 --> T042
    T013 --> T042

    T006 --> T025
    T006 --> T020

    T019 --> T020
    T025 --> T020
    T015 --> T020

    T008["TASK-008<br/>Task Lifecycle<br/>Query API"]:::backend

    T002 --> T008
    T003 --> T008
    T005 --> T008

    T029["TASK-029<br/>DevOps Phase 2<br/>Staging + CD"]:::infra

    T001 --> T029
    T042 --> T029
```

## Cycle 2 -- Core System Completion

```mermaid
flowchart TD
    classDef backend  fill:#b8e8c9,stroke:#2d9e5a,color:#0a1e0a,font-weight:bold
    classDef p1       fill:#c9b8e8,stroke:#6b3fa0,color:#1a0a2e,font-weight:bold

    C1["Cycle 1<br/>(complete)"]:::p1

    T009["TASK-009<br/>Monitor Service<br/>Heartbeat + Failover"]:::p1
    T010["TASK-010<br/>Infrastructure<br/>Retry + Backoff"]:::backend
    T011["TASK-011<br/>Dead Letter Queue +<br/>Cascading Cancel"]:::backend
    T012["TASK-012<br/>Task<br/>Cancellation"]:::backend
    T014["TASK-014<br/>Pipeline Chain<br/>Definition"]:::backend
    T018["TASK-018<br/>Sink Atomicity +<br/>Idempotency"]:::p1
    T016["TASK-016<br/>Log Production +<br/>Dual Storage"]:::backend
    T017["TASK-017<br/>Admin User<br/>Management"]:::backend
    T026["TASK-026<br/>Schema Validation<br/>Design-time"]:::backend

    C1 --> T009
    C1 --> T012
    C1 --> T014
    C1 --> T018
    C1 --> T016
    C1 --> T017
    C1 --> T026

    T009 --> T010
    T010 --> T011
    T014 --> T011
```

## Cycle 3 -- GUI Completion and Infrastructure

```mermaid
flowchart TD
    classDef frontend fill:#b8d4e8,stroke:#2d6b9e,color:#0a1a2e,font-weight:bold
    classDef infra    fill:#e8d4b8,stroke:#9e6b2d,color:#2e1a0a,font-weight:bold
    classDef p1       fill:#c9b8e8,stroke:#6b3fa0,color:#1a0a2e,font-weight:bold

    C2["Cycle 2<br/>(complete)"]:::p1

    T021["TASK-021<br/>Task Feed +<br/>Monitor GUI"]:::frontend
    T022["TASK-022<br/>Log Streamer<br/>GUI"]:::frontend
    T023["TASK-023<br/>Pipeline Builder<br/>GUI"]:::p1
    T035["TASK-035<br/>Task Submission<br/>GUI Flow"]:::frontend
    T024["TASK-024<br/>Pipeline Mgmt<br/>GUI"]:::frontend
    T028["TASK-028<br/>Log Retention +<br/>Partition Pruning"]:::infra
    T027["TASK-027<br/>Health Endpoint<br/>+ OpenAPI"]:::infra

    C2 --> T021
    C2 --> T022
    C2 --> T023
    C2 --> T028
    C2 --> T027

    T021 --> T035
    T023 --> T024
```

## Cycle 4 -- Demo Infrastructure

```mermaid
flowchart TD
    classDef demo     fill:#e8e8b8,stroke:#9e9e2d,color:#1e1e0a,font-weight:bold
    classDef frontend fill:#b8d4e8,stroke:#2d6b9e,color:#0a1a2e,font-weight:bold
    classDef test     fill:#c9b8e8,stroke:#6b3fa0,color:#1a0a2e,font-weight:bold

    C3["Cycle 3<br/>(complete)"]:::test

    T030["TASK-030<br/>MinIO Fake-S3<br/>Demo Infra"]:::demo
    T031["TASK-031<br/>Mock-Postgres<br/>Demo Infra"]:::demo
    T033["TASK-033<br/>Sink Before/After<br/>Snapshot Capture"]:::demo
    T032["TASK-032<br/>Sink Inspector<br/>GUI"]:::frontend
    T034["TASK-034<br/>Chaos Controller<br/>GUI"]:::frontend
    T038["TASK-038<br/>Fitness Function<br/>Instrumentation"]:::test

    C3 --> T030
    C3 --> T031
    C3 --> T033
    C3 --> T034
    C3 --> T038

    T030 --> T032
    T031 --> T032
    T033 --> T032
```

## Cycle 5 -- Production Deployment

```mermaid
flowchart TD
    classDef infra    fill:#e8d4b8,stroke:#9e6b2d,color:#2e1a0a,font-weight:bold
    classDef test     fill:#c9b8e8,stroke:#6b3fa0,color:#1a0a2e,font-weight:bold

    C4["Cycle 4<br/>(complete)"]:::test
    C1_029["TASK-029<br/>(Cycle 1)"]:::infra

    T037["TASK-037<br/>Throughput<br/>Load Test"]:::test
    T036["TASK-036<br/>DevOps Phase 3<br/>Production + Monitoring"]:::infra

    C4 --> T037
    C4 --> T036

    C1_029 --> T037
    C1_029 --> T036
```

## Legend

| Color | Category |
|---|---|
| Orange | Infrastructure / DevOps |
| Green | Backend services |
| Blue | Frontend / GUI |
| Purple | Critical path / high-risk |
| Yellow | Demo infrastructure |

## Critical Path (Walking Skeleton -- Cycle 1)

The walking skeleton critical path through Cycle 1:

```
TASK-001 (DevOps) -> TASK-002 (DB Schema) -> TASK-003 (Auth)
                  -> TASK-004 (Redis Streams)
                                             -> TASK-005 (Task Submission API)
                  -> TASK-006 (Worker Registration)
                  -> TASK-013 (Pipeline CRUD API)
                                             -> TASK-007 (Pipeline Execution)
                                             -> TASK-042 (Demo Connectors)
```

This chain produces the walking skeleton: an admin can log in, create a demo pipeline via API, submit a task, have it queued, assigned to a simulated worker, executed through a three-phase pipeline with demo connectors, and see the task reach "completed" state. TASK-029 (staging deployment) depends on TASK-001 + TASK-042 and deploys the walking skeleton to nexusflow.staging.nxlabs.cc.

## Critical Path (Full v1.0.0 -- Cycles 1 through 3)

```
Cycle 1 -> TASK-009 (Monitor) -> TASK-010 (Retry) -> TASK-011 (DLQ)
        -> TASK-018 (Sink Atomicity)
        -> TASK-016 (Log Production)
        -> TASK-008 (Task Query API, Cycle 1) -> TASK-021 (Task Feed GUI) -> TASK-035 (GUI Submission)
        -> TASK-026 (Schema Validation) -> TASK-023 (Pipeline Builder) -> TASK-024 (Pipeline Mgmt)
```
