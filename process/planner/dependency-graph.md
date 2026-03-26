# Dependency Graph -- NexusFlow Task Plan
**Version:** 1 | **Date:** 2026-03-26

## Cycle 1 -- Walking Skeleton + Core System

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
    T017["TASK-017<br/>Admin User<br/>Management"]:::backend
    T025["TASK-025<br/>Worker Fleet<br/>Status API"]:::backend
    T027["TASK-027<br/>Health Endpoint<br/>+ OpenAPI"]:::backend

    T007["TASK-007<br/>Task Assignment +<br/>Pipeline Execution"]:::p1
    T008["TASK-008<br/>Task Lifecycle<br/>Query API"]:::backend
    T015["TASK-015<br/>SSE Event<br/>Infrastructure"]:::backend
    T026["TASK-026<br/>Schema Validation<br/>Design-time"]:::backend

    T009["TASK-009<br/>Monitor Service<br/>Heartbeat + Failover"]:::p1
    T012["TASK-012<br/>Task<br/>Cancellation"]:::backend
    T014["TASK-014<br/>Pipeline Chain<br/>Definition"]:::backend
    T016["TASK-016<br/>Log Production +<br/>Dual Storage"]:::backend
    T018["TASK-018<br/>Sink Atomicity +<br/>Idempotency"]:::p1

    T010["TASK-010<br/>Infrastructure<br/>Retry + Backoff"]:::backend
    T011["TASK-011<br/>Dead Letter Queue +<br/>Cascading Cancel"]:::backend

    T019["TASK-019<br/>React App Shell +<br/>Sidebar + Auth"]:::frontend
    T020["TASK-020<br/>Worker Fleet<br/>Dashboard GUI"]:::frontend
    T021["TASK-021<br/>Task Feed +<br/>Monitor GUI"]:::frontend
    T022["TASK-022<br/>Log Streamer<br/>GUI"]:::frontend
    T023["TASK-023<br/>Pipeline Builder<br/>GUI"]:::frontend
    T024["TASK-024<br/>Pipeline Mgmt<br/>GUI"]:::frontend
    T028["TASK-028<br/>Log Retention +<br/>Partition Pruning"]:::infra

    T001 --> T002
    T001 --> T004
    T001 --> T027

    T002 --> T003
    T002 --> T006
    T002 --> T005
    T002 --> T013
    T002 --> T008
    T002 --> T028

    T003 --> T005
    T003 --> T013
    T003 --> T017
    T003 --> T025
    T003 --> T019
    T003 --> T027
    T003 --> T015
    T003 --> T008

    T004 --> T005
    T004 --> T007
    T004 --> T009
    T004 --> T015

    T005 --> T007
    T005 --> T008
    T005 --> T012

    T006 --> T007
    T006 --> T009
    T006 --> T025

    T007 --> T009
    T007 --> T012
    T007 --> T014
    T007 --> T016
    T007 --> T018

    T009 --> T010
    T009 --> T011

    T010 --> T011

    T013 --> T014
    T013 --> T023
    T013 --> T024
    T013 --> T026

    T015 --> T016
    T015 --> T020
    T015 --> T022

    T016 --> T028
    T016 --> T022

    T019 --> T020
    T019 --> T021
    T019 --> T022
    T019 --> T023
    T019 --> T024

    T005 --> T021
    T008 --> T021
    T012 --> T021
    T013 --> T021
    T015 --> T021
    T006 --> T020
    T025 --> T020

    T023 --> T024

    T013 --> T035
    T021 --> T035
```

## Cycle 2 -- Demo Infrastructure + Production Readiness

```mermaid
flowchart TD
    classDef infra    fill:#e8d4b8,stroke:#9e6b2d,color:#2e1a0a,font-weight:bold
    classDef demo     fill:#e8e8b8,stroke:#9e9e2d,color:#1e1e0a,font-weight:bold
    classDef frontend fill:#b8d4e8,stroke:#2d6b9e,color:#0a1a2e,font-weight:bold
    classDef test     fill:#c9b8e8,stroke:#6b3fa0,color:#1a0a2e,font-weight:bold

    C1["Cycle 1<br/>(all tasks complete)"]:::infra

    T029["TASK-029<br/>DevOps Phase 2<br/>Staging + CD"]:::infra
    T030["TASK-030<br/>MinIO Fake-S3<br/>Demo Infra"]:::demo
    T031["TASK-031<br/>Mock-Postgres<br/>Demo Infra"]:::demo
    T033["TASK-033<br/>Sink Before/After<br/>Snapshot Capture"]:::demo
    T032["TASK-032<br/>Sink Inspector<br/>GUI"]:::frontend
    T034["TASK-034<br/>Chaos Controller<br/>GUI"]:::frontend
    T035["TASK-035<br/>Task Submission<br/>GUI Flow"]:::frontend
    T036["TASK-036<br/>DevOps Phase 3<br/>Production + Monitoring"]:::infra
    T037["TASK-037<br/>Throughput<br/>Load Test"]:::test
    T038["TASK-038<br/>Fitness Function<br/>Instrumentation"]:::test

    C1 --> T029
    C1 --> T030
    C1 --> T031
    C1 --> T033
    C1 --> T035
    C1 --> T038

    T029 --> T036
    T029 --> T037

    T030 --> T032
    T031 --> T032
    T033 --> T032

    C1 --> T034
```

## Legend

| Color | Category |
|---|---|
| Orange | Infrastructure / DevOps |
| Green | Backend services |
| Blue | Frontend / GUI |
| Purple | Critical path / high-risk |
| Yellow | Demo infrastructure |

## Critical Path (Walking Skeleton)

The walking skeleton critical path through Cycle 1 is:

```
TASK-001 (DevOps) -> TASK-002 (DB Schema) -> TASK-003 (Auth)
                  -> TASK-004 (Redis Streams)
                                             -> TASK-005 (Task Submission API)
                  -> TASK-006 (Worker Registration)
                                             -> TASK-007 (Pipeline Execution)
                                             -> TASK-009 (Monitor/Failover)
```

This chain produces the walking skeleton: a user can authenticate, submit a task, have it queued, assigned to a worker, executed through a pipeline, and observe the result -- with auto-failover if the worker goes down.
