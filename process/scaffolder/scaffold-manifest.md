# Scaffold Manifest — NexusFlow
**Version:** 1 | **Date:** 2026-03-26
**Artifact Weight:** Blueprint
**Profile:** Critical

---

## Structure Overview

```
nexusflow/
├── cmd/
│   ├── api/main.go                          — API server entry point
│   ├── worker/main.go                       — Worker process entry point
│   └── monitor/main.go                      — Monitor service entry point
├── api/
│   ├── server.go                            — HTTP server, route registration
│   ├── handlers_auth.go                     — POST /api/auth/login, /logout
│   ├── handlers_tasks.go                    — Task submission, query, cancel
│   ├── handlers_pipelines.go                — Pipeline CRUD
│   ├── handlers_workers.go                  — Worker fleet status
│   ├── handlers_sse.go                      — SSE streaming endpoints
│   └── handlers_health.go                   — GET /api/health
├── worker/
│   ├── worker.go                            — Worker main struct, execution loop
│   └── connectors.go                        — Connector interfaces + demo implementations
├── monitor/
│   └── monitor.go                           — Monitor service (Cycle 2)
├── internal/
│   ├── models/models.go                     — All domain types (shared by all services)
│   ├── config/config.go                     — Environment variable configuration loading
│   ├── auth/auth.go                         — Auth middleware, password hashing, token generation
│   ├── db/
│   │   ├── db.go                            — PostgreSQL pool, migration execution
│   │   ├── repository.go                    — Repository interfaces (User, Pipeline, Task, Worker, Log)
│   │   ├── migrations/
│   │   │   ├── 000001_initial_schema.up.sql — Initial schema migration
│   │   │   └── 000001_initial_schema.down.sql
│   │   └── queries/
│   │       ├── users.sql                    — sqlc query definitions: users
│   │       ├── tasks.sql                    — sqlc query definitions: tasks
│   │       ├── pipelines.sql                — sqlc query definitions: pipelines
│   │       ├── workers.sql                  — sqlc query definitions: workers
│   │       └── logs.sql                     — sqlc query definitions: task logs
│   ├── queue/
│   │   ├── queue.go                         — Queue interfaces: Producer, Consumer, PendingScanner, HeartbeatStore, EventPublisher, SessionStore
│   │   └── redis.go                         — Redis implementations of all queue interfaces
│   └── sse/
│       ├── broker.go                        — SSE Broker interface
│       └── redis_broker.go                  — Redis Pub/Sub-backed Broker implementation
├── web/
│   ├── package.json
│   ├── tsconfig.json
│   ├── vite.config.ts                       — Dev proxy: /api and /events -> Go server
│   └── src/
│       ├── main.tsx                         — React entry point
│       ├── App.tsx                          — Router + AuthProvider root
│       ├── styles/globals.css               — Design system tokens (DESIGN.md)
│       ├── types/domain.ts                  — TypeScript domain types (mirrors Go models)
│       ├── context/AuthContext.tsx          — Auth state, login/logout actions
│       ├── hooks/
│       │   ├── useSSE.ts                    — EventSource wrapper with reconnect
│       │   └── useAuth.ts                   — Re-export of useAuth hook
│       ├── api/client.ts                    — Typed REST API client functions
│       └── pages/
│           ├── LoginPage.tsx                — Login screen (UX spec)
│           ├── WorkerFleetDashboard.tsx     — Worker Fleet Dashboard (UX spec)
│           └── NotFoundPage.tsx             — 404 fallback
├── docker-compose.yml                       — Dev environment (all services + demo profile)
├── Dockerfile.api                           — Multi-stage Go build for API server
├── Dockerfile.worker                        — Multi-stage Go build for Worker
├── Dockerfile.monitor                       — Multi-stage Go build for Monitor
├── Dockerfile.web                           — Vite build + nginx serve for React app
├── nginx.conf                               — SPA routing for the web container
├── Makefile                                 — Common dev commands
├── .env.example                             — All environment variables documented
├── go.mod                                   — Go module definition
└── .github/workflows/ci.yml                 — CI: go build, vet, staticcheck, test; npm build
```

---

## Components

### Domain Models — `internal/models/models.go`
**Responsibility:** Defines all shared domain types used by the API, Worker, and Monitor services.
**Architectural source:** Architecture v2 (Component Map), Analyst Brief v2 (Domain Model), ADR-008

#### Exported types

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `TaskStatus` | type (string enum) | `"submitted" \| "queued" \| ...` | Valid lifecycle states for a Task; transitions enforced by DB trigger |
| `WorkerStatus` | type (string enum) | `"online" \| "down"` | Liveness state of a Worker |
| `Role` | type (string enum) | `"admin" \| "user"` | Access level of a User |
| `RetryConfig` | struct | `{ MaxRetries int, Backoff BackoffStrategy }` | Per-task retry settings; safe defaults via DefaultRetryConfig() |
| `SchemaMapping` | struct | `{ SourceField, TargetField string }` | Field mapping between pipeline phases |
| `DataSourceConfig` | struct | connector type + config + output schema | Phase 1 of a Pipeline |
| `ProcessConfig` | struct | connector type + input mappings + output schema | Phase 2 of a Pipeline |
| `SinkConfig` | struct | connector type + input mappings | Phase 3 of a Pipeline (atomic writes, ADR-009) |
| `User` | struct | id, username, passwordHash, role, active, createdAt | Authenticated user; passwordHash is never serialised to JSON |
| `Pipeline` | struct | id, name, userId, three phase configs, timestamps | Owned by User; linear DataSource->Process->Sink |
| `PipelineChain` | struct | id, name, userId, pipelineIds (ordered) | Linear chain; failure cascades cancellation downstream |
| `Task` | struct | id, pipelineId, chainId?, userId, status, retryConfig, ... | Primary unit of work |
| `TaskStateLog` | struct | id, taskId, fromState, toState, reason, timestamp | Audit trail of Task lifecycle |
| `Worker` | struct | id, tags, status, lastHeartbeat, registeredAt | Compute node; emits heartbeats every 5s |
| `TaskLog` | struct | id, taskId, line, level, timestamp | Log line; hot in Redis, cold in PostgreSQL |
| `Session` | struct | userId, role, createdAt | Server-side session payload stored in Redis |
| `SSEEvent` | struct | channel, type, payload, id | SSE event envelope for Redis Pub/Sub fan-out |
| `SinkSnapshot` | struct | taskId, phase, data, capturedAt | Before/After Sink state for Sink Inspector (ADR-009) |

---

### Configuration — `internal/config/config.go`
**Responsibility:** Loads all runtime configuration from environment variables with validation and defaults.
**Architectural source:** ADR-005 (12-factor config)

#### Exported interface

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `Config` | struct | All runtime parameters | Fully populated or error returned |
| `Load()` | function | `() -> (*Config, error)` | Returns populated Config or error listing all missing variables |

---

### Auth — `internal/auth/auth.go`
**Responsibility:** HTTP middleware for session validation, password hashing, and token generation.
**Architectural source:** ADR-006

#### Exported interface

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `HashPassword(password)` | function | `string -> (string, error)` | Returns bcrypt hash with cost 12; VerifyPassword(input, hash) returns nil |
| `VerifyPassword(password, hash)` | function | `(string, string) -> error` | Returns ErrInvalidCredentials on mismatch; never leaks timing info |
| `GenerateToken()` | function | `() -> (string, error)` | Returns cryptographically random 256-bit hex token |
| `Middleware(sessions)` | function | `SessionStore -> func(http.Handler) http.Handler` | Reads cookie or Bearer token; injects Session into ctx; 401 on failure |
| `RequireRole(role)` | function | `Role -> func(http.Handler) http.Handler` | Must compose after Middleware; 403 on insufficient role |
| `SessionFromContext(ctx)` | function | `context.Context -> *Session` | Returns nil if no session in context |
| `ErrInvalidCredentials` | var | `error` | Sentinel for password mismatch; map to 401 in handlers |

---

### Database — `internal/db/db.go` + `internal/db/repository.go`
**Responsibility:** PostgreSQL connection pool, migration execution, and repository interfaces for all domain entities.
**Architectural source:** ADR-004, ADR-008

#### Exported interface — `db.go`

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `New(ctx, dsn)` | function | `(Context, string) -> (*Pool, error)` | Opens pool, runs all pending migrations; returns error on failure |
| `RunMigrations(dsn)` | function | `string -> error` | Idempotent; skips already-applied migrations |

#### Exported interface — `repository.go` (interfaces)

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `UserRepository` | interface | 5 methods | Create, GetByID, GetByUsername, List, Deactivate |
| `PipelineRepository` | interface | 7 methods | CRUD + HasActiveTasks; Delete returns ErrActiveTasks if non-terminal tasks exist |
| `TaskRepository` | interface | 7 methods | Create, GetByID, ListByUser, List, UpdateStatus, IncrementRetryCount, Cancel |
| `WorkerRepository` | interface | 4 methods | Register (upsert), GetByID, List (with CurrentTaskID), UpdateStatus |
| `TaskLogRepository` | interface | 2 methods | BatchInsert (cold sync), ListByTask (with afterID for Last-Event-ID replay) |
| `ErrNotFound` | var | `error` | Sentinel for missing entity; map to 404 |
| `ErrActiveTasks` | var | `error` | Sentinel for pipeline-with-active-tasks; map to 409 |
| `ErrConflict` | var | `error` | Sentinel for unique constraint violations; map to 409 |

---

### Queue — `internal/queue/queue.go` + `internal/queue/redis.go`
**Responsibility:** Redis Streams abstraction for task enqueuing, consumption, pending entry scanning, heartbeat management, event publishing, and session storage.
**Architectural source:** ADR-001, ADR-002, ADR-003, ADR-006, ADR-007

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `Producer` | interface | `Enqueue, EnqueueDeadLetter` | Routes tasks to queue:{tag} streams via XADD; creates groups on first use |
| `Consumer` | interface | `ReadTasks, Acknowledge, InitGroups` | XREADGROUP blocking read; XACK on completion; group creation on startup |
| `PendingScanner` | interface | `ListPendingOlderThan, Claim` | XPENDING + XCLAIM for Monitor failover reclamation |
| `HeartbeatStore` | interface | `RecordHeartbeat, ListExpired, Remove` | ZADD / ZRANGEBYSCORE on workers:active |
| `EventPublisher` | interface | `Publish` | Redis Pub/Sub PUBLISH to SSE distribution channels |
| `SessionStore` | interface | `Create, Get, Delete, DeleteAllForUser` | Redis session:{token} with TTL; DeleteAllForUser for immediate revocation |
| `RedisQueue` | struct | Implements all queue interfaces | Single instance; injected via interface where needed |
| `RedisSessionStore` | struct | Implements SessionStore | Separate from RedisQueue; injected into auth middleware |
| Stream key constants | `TaskQueueStream(tag)`, `DeadLetterStream`, `WorkersActiveKey`, `ConsumerGroupName`, `NewLogStream(taskID)` | Helper functions/constants | Centralise stream naming per ADR-001 |

---

### SSE Broker — `internal/sse/broker.go` + `internal/sse/redis_broker.go`
**Responsibility:** SSE event distribution layer — subscribes to Redis Pub/Sub and fans events to connected HTTP clients.
**Architectural source:** ADR-007

#### Exported interface

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `Broker` | interface | 8 methods | Start, ServeTaskEvents, ServeWorkerEvents, ServeLogEvents, ServeSinkEvents, PublishTaskEvent, PublishWorkerEvent, PublishLogLine, PublishSinkSnapshot |
| `RedisBroker` | struct | Implements Broker | Redis Pub/Sub subscriber; in-process fan-out to SSE clients |

---

### API Server — `api/server.go` + handlers
**Responsibility:** HTTP server, route registration, and handler implementations for all REST and SSE endpoints.
**Architectural source:** Architecture v2 (Resource Topology), ADR-004, ADR-006, ADR-007

#### Route surface (all handlers are TODO stubs)

| Handler | Task | Endpoint |
|---|---|---|
| `AuthHandler.Login` | TASK-003 | POST /api/auth/login |
| `AuthHandler.Logout` | TASK-003 | POST /api/auth/logout |
| `HealthHandler.Health` | TASK-001 | GET /api/health |
| `TaskHandler.Submit` | TASK-005 | POST /api/tasks |
| `TaskHandler.List` | TASK-008 (Cycle 2) | GET /api/tasks |
| `TaskHandler.Get` | TASK-008 (Cycle 2) | GET /api/tasks/{id} |
| `TaskHandler.Cancel` | TASK-012 (Cycle 2) | POST /api/tasks/{id}/cancel |
| `PipelineHandler.Create` | TASK-013 | POST /api/pipelines |
| `PipelineHandler.List` | TASK-013 | GET /api/pipelines |
| `PipelineHandler.Get` | TASK-013 | GET /api/pipelines/{id} |
| `PipelineHandler.Update` | TASK-013 | PUT /api/pipelines/{id} |
| `PipelineHandler.Delete` | TASK-013 | DELETE /api/pipelines/{id} |
| `WorkerHandler.List` | TASK-025 | GET /api/workers |
| `SSEHandler.Tasks` | TASK-015 | GET /events/tasks |
| `SSEHandler.Workers` | TASK-015 | GET /events/workers |
| `SSEHandler.Logs` | TASK-015 | GET /events/tasks/{id}/logs |
| `SSEHandler.Sink` | TASK-015 | GET /events/sink/{taskId} |

---

### Worker — `worker/worker.go` + `worker/connectors.go`
**Responsibility:** Task consumption loop, pipeline execution (DataSource -> Process -> Sink), schema mapping application, heartbeat emission, and worker registration.
**Architectural source:** Architecture v2 (Worker responsibilities), ADR-001, ADR-002, ADR-003, ADR-009

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `Worker` | struct | Main worker struct | Manages registration, heartbeat, consumption loop |
| `Worker.Run(ctx)` | method | `Context -> error` | Blocks until ctx cancelled; starts all goroutines |
| `Worker.Register(ctx)` | method | `Context -> error` | Upserts worker record in PostgreSQL + Redis |
| `DataSourceConnector` | interface | `Type(), Fetch(ctx, config, input)` | Returns records or error; error = task failed, no retry |
| `ProcessConnector` | interface | `Type(), Transform(ctx, config, records)` | Returns transformed records; error = task failed, no retry |
| `SinkConnector` | interface | `Type(), Snapshot(ctx, config, taskID), Write(ctx, config, records, executionID)` | Atomic writes with idempotency guard; Snapshot for Sink Inspector |
| `ConnectorRegistry` | interface | `DataSource, Process, Sink, Register` | Resolves connector type names to implementations |
| `DemoDataSource` | struct | Implements DataSourceConnector | type="demo"; deterministic sample data |
| `DemoProcessConnector` | struct | Implements ProcessConnector | type="demo"; pass-through |
| `DemoSinkConnector` | struct | Implements SinkConnector | type="demo"; in-memory store + stdout |
| `ErrAlreadyApplied` | var | `error` | Sink idempotency; caller treats as no-op success |

---

### Monitor — `monitor/monitor.go`
**Responsibility:** Heartbeat timeout detection and pending task reclamation via XCLAIM. Cycle 2 implementation.
**Architectural source:** ADR-002

#### Exported interface

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `Monitor` | struct | Main monitor struct | Runs heartbeat checker and pending scanner |
| `Monitor.Run(ctx)` | method | `Context -> error` | Blocks until ctx cancelled; runs both periodic loops |

---

### Frontend — `web/src/`
**Responsibility:** React + TypeScript SPA consuming the REST API and SSE endpoints.
**Architectural source:** ADR-004, DESIGN.md, UX spec

#### Exported contracts

| Element | Kind | File | Contract |
|---|---|---|---|
| Domain types | TypeScript types | `types/domain.ts` | Mirror of Go models; must stay in sync with API responses |
| `AuthProvider` | React component | `context/AuthContext.tsx` | Provides auth state; wraps entire app |
| `useAuth()` | hook | `hooks/useAuth.ts` | Returns { user, login, logout, isLoading }; must be inside AuthProvider |
| `useSSE(options)` | hook | `hooks/useSSE.ts` | Manages EventSource lifecycle; returns { status, close } |
| API client functions | functions | `api/client.ts` | login, logout, listWorkers, listPipelines, createPipeline, submitTask, listTasks |
| `LoginPage` | React component | `pages/LoginPage.tsx` | Public; login form per UX spec |
| `WorkerFleetDashboard` | React component | `pages/WorkerFleetDashboard.tsx` | Admin landing page; real-time via SSE |

---

## Dependencies between components

| Component | Depends on | Nature of dependency |
|---|---|---|
| `cmd/api/main.go` | `internal/config`, `internal/db`, `internal/queue`, `internal/auth`, `internal/sse`, `api/` | wires dependencies and calls server.Handler() |
| `cmd/worker/main.go` | `internal/config`, `internal/db`, `internal/queue`, `internal/sse`, `worker/` | wires dependencies and calls worker.Run() |
| `cmd/monitor/main.go` | `internal/config`, `internal/db`, `internal/queue`, `internal/sse`, `monitor/` | wires dependencies and calls monitor.Run() |
| `api/server.go` | `internal/auth`, `internal/db`, `internal/queue`, `internal/sse` | injects all repositories and services |
| `api/handlers_*.go` | `api/server.go`, `internal/models`, `internal/auth` | accesses server.users, server.tasks, etc. |
| `worker/worker.go` | `internal/db`, `internal/queue`, `internal/sse`, `internal/models` | reads tasks from queue, writes status to db, publishes events |
| `worker/connectors.go` | `internal/models` | uses SchemaMapping, SinkSnapshot types |
| `monitor/monitor.go` | `internal/db`, `internal/queue`, `internal/sse` | reads workers/tasks from db, claims via queue, publishes events |
| `internal/auth/auth.go` | `internal/queue` (SessionStore), `internal/models` | reads sessions from Redis |
| `internal/sse/redis_broker.go` | `internal/models`, `internal/queue` (EventPublisher) | publishes via Redis Pub/Sub |
| `internal/db/repository.go` | `internal/models` | all repository methods use domain types |
| `web/src/context/AuthContext.tsx` | `web/src/api/client.ts`, `web/src/types/domain.ts` | calls login/logout; stores User |
| `web/src/pages/WorkerFleetDashboard.tsx` | `web/src/api/client.ts`, `web/src/hooks/useSSE.ts` | seeds from REST, live updates from SSE |

---

## Builder task surface

### Cycle 1 — All unimplemented elements

| Element | Location | Complexity signal |
|---|---|---|
| `config.Load()` | `internal/config/config.go` | Low — read env vars, apply defaults, validate required |
| `db.New(ctx, dsn)` | `internal/db/db.go` | Low — pgxpool.New + RunMigrations |
| `db.RunMigrations(dsn)` | `internal/db/db.go` | Low — golang-migrate embed + Apply |
| Migration 000001 up SQL | `internal/db/migrations/000001_initial_schema.up.sql` | Medium — all tables, indexes, trigger for state transition enforcement |
| Migration 000001 down SQL | `internal/db/migrations/000001_initial_schema.down.sql` | Low — DROP tables in order |
| All sqlc query files | `internal/db/queries/*.sql` | Low — standard CRUD SQL; sqlc validates |
| `HashPassword` | `internal/auth/auth.go` | Low — bcrypt.GenerateFromPassword |
| `VerifyPassword` | `internal/auth/auth.go` | Low — bcrypt.CompareHashAndPassword |
| `GenerateToken` | `internal/auth/auth.go` | Low — crypto/rand |
| `auth.Middleware` | `internal/auth/auth.go` | Medium — cookie + Bearer token extraction, Redis lookup, ctx injection |
| `auth.RequireRole` | `internal/auth/auth.go` | Low — read session from ctx, compare role |
| `auth.SessionFromContext` | `internal/auth/auth.go` | Low — ctx.Value |
| `RedisQueue.NewRedisQueue` | `internal/queue/redis.go` | Low — struct construction |
| `RedisQueue.Enqueue` | `internal/queue/redis.go` | Medium — XADD per tag, XGROUP CREATE MKSTREAM on first call |
| `RedisQueue.InitGroups` | `internal/queue/redis.go` | Low — XGROUP CREATE MKSTREAM, handle BUSYGROUP error |
| `RedisSessionStore.Create/Get/Delete/DeleteAllForUser` | `internal/queue/redis.go` | Medium — JSON marshal/unmarshal; DeleteAllForUser requires SCAN |
| `api.Server.NewServer` | `api/server.go` | Low — struct assignment |
| `api.Server.Handler` | `api/server.go` | Medium — chi router setup, middleware chain, route registration |
| `HealthHandler.Health` | `api/handlers_health.go` | Low — Redis PING + PostgreSQL SELECT 1 |
| `AuthHandler.Login` | `api/handlers_auth.go` | Medium — credential check, token generation, cookie + JSON response |
| `AuthHandler.Logout` | `api/handlers_auth.go` | Low — session delete, cookie clear |
| `PipelineHandler.Create` | `api/handlers_pipelines.go` | Low — JSON decode, insert, return 201 |
| `PipelineHandler.List` | `api/handlers_pipelines.go` | Low — role-based query selection |
| `PipelineHandler.Get` | `api/handlers_pipelines.go` | Low — ownership check, return 200/403/404 |
| `PipelineHandler.Update` | `api/handlers_pipelines.go` | Low — ownership check, update, return 200 |
| `PipelineHandler.Delete` | `api/handlers_pipelines.go` | Low — ownership check, HasActiveTasks guard, delete, return 204/409 |
| `TaskHandler.Submit` | `api/handlers_tasks.go` | Medium — validate pipeline ref, insert task, enqueue, publish SSE event |
| `WorkerHandler.List` | `api/handlers_workers.go` | Low — query all workers, return 200 |
| `Worker.Register` | `worker/worker.go` | Low — upsert in PostgreSQL + RecordHeartbeat |
| `Worker.emitHeartbeats` | `worker/worker.go` | Low — ticker + RecordHeartbeat |
| `Worker.Run` | `worker/worker.go` | Medium — goroutine coordination: registration, heartbeat, consumption loop |
| `Worker.runConsumptionLoop` | `worker/worker.go` | Medium — blocking XREADGROUP, dispatch to executeTask, error handling |
| `Worker.executeTask` | `worker/worker.go` | **High** — integrates all three pipeline phases, schema mapping, Sink atomicity, XACK, SSE events, idempotency guard |
| `Worker.applySchemaMapping` | `worker/worker.go` | Low — field rename using SchemaMapping slice; error on missing source field |
| `RedisQueue.ReadTasks` | `internal/queue/redis.go` | Medium — XREADGROUP across multiple streams, parse message to TaskMessage |
| `RedisQueue.Acknowledge` | `internal/queue/redis.go` | Low — XACK |
| `DemoDataSource.Fetch` | `worker/connectors.go` | Low — return hard-coded sample records |
| `DemoProcessConnector.Transform` | `worker/connectors.go` | Low — pass-through, return records unchanged |
| `DemoSinkConnector.Snapshot` | `worker/connectors.go` | Low — return in-memory store contents |
| `DemoSinkConnector.Write` | `worker/connectors.go` | Low — idempotency guard on executionID, append to in-memory store |
| `RedisBroker.NewRedisBroker` | `internal/sse/redis_broker.go` | Low — struct init with subscriber map |
| `RedisBroker.Start` | `internal/sse/redis_broker.go` | **High** — goroutine management, Pub/Sub subscription lifecycle, fan-out to client channels |
| `RedisBroker.ServeTaskEvents` | `internal/sse/redis_broker.go` | Medium — role-based channel selection, SSE write loop, client cleanup |
| `RedisBroker.ServeWorkerEvents` | `internal/sse/redis_broker.go` | Medium — SSE write loop |
| `RedisBroker.ServeLogEvents` | `internal/sse/redis_broker.go` | **High** — Last-Event-ID replay from PostgreSQL + live Pub/Sub; goroutine management and backpressure |
| `RedisBroker.ServeSinkEvents` | `internal/sse/redis_broker.go` | Medium — ownership check, SSE write loop |
| `RedisBroker.Publish*` | `internal/sse/redis_broker.go` | Low — marshal event + PUBLISH |
| `writeSSEEvent` | `internal/sse/redis_broker.go` | Low — format SSE wire protocol, http.Flusher |
| `SSEHandler.Tasks/Workers/Logs/Sink` | `api/handlers_sse.go` | Low — extract params, delegate to broker |
| `AuthContext.AuthProvider` | `web/src/context/AuthContext.tsx` | Medium — session state management, login/logout API calls, navigate on role |
| `useSSE hook` | `web/src/hooks/useSSE.ts` | Medium — EventSource lifecycle, exponential backoff reconnect, status tracking |
| `api/client.ts` functions | `web/src/api/client.ts` | Low — fetch wrappers, JSON body, credentials: include |
| `LoginPage` (styled) | `web/src/pages/LoginPage.tsx` | Low — form, error state, DESIGN.md styling |
| `WorkerFleetDashboard` (styled) | `web/src/pages/WorkerFleetDashboard.tsx` | Medium — summary cards, sortable data table, SSE integration, DESIGN.md styling |
| `cmd/api/main.go` | `cmd/api/main.go` | Medium — wiring all dependencies, admin seed, graceful shutdown |
| `cmd/worker/main.go` | `cmd/worker/main.go` | Medium — wiring all dependencies, connector registry, graceful shutdown |

### Cycle 2 — Scaffolded for structure only (not Cycle 1 scope)

| Element | Location | Complexity signal |
|---|---|---|
| `TaskHandler.List` | `api/handlers_tasks.go` | Low |
| `TaskHandler.Get` | `api/handlers_tasks.go` | Low |
| `TaskHandler.Cancel` | `api/handlers_tasks.go` | Medium — cancel authority check |
| `Monitor.Run` | `monitor/monitor.go` | Medium — dual ticker coordination |
| `Monitor.checkHeartbeats` | `monitor/monitor.go` | Medium — ZRANGEBYSCORE + N PostgreSQL updates |
| `Monitor.scanPendingEntries` | `monitor/monitor.go` | **High** — XPENDING, XCLAIM, retry logic, dead-letter routing, cascading cancel |
| `Monitor.reclaimTask` | `monitor/monitor.go` | Medium — XCLAIM + status transition |
| `Monitor.deadLetterTask` | `monitor/monitor.go` | Medium — dead-letter + cascading cancel |
| `RedisQueue.EnqueueDeadLetter` | `internal/queue/redis.go` | Low — XADD to queue:dead-letter |
| `RedisQueue.ListPendingOlderThan` | `internal/queue/redis.go` | Medium — XPENDING IDLE |
| `RedisQueue.Claim` | `internal/queue/redis.go` | Low — XCLAIM |
| `RedisQueue.ListExpired` | `internal/queue/redis.go` | Low — ZRANGEBYSCORE |
| `RedisQueue.Remove` | `internal/queue/redis.go` | Low — ZREM |
| `cmd/monitor/main.go` | `cmd/monitor/main.go` | Medium — wiring |

---

## Component dependency order for Builder sequencing

The following order must be observed when Builder tasks are assigned:

1. **TASK-001** — Infrastructure first: monorepo layout, Go module, Docker Compose, CI pipeline, Dockerfiles. Unblocks everything.
2. **TASK-002** — Database schema and sqlc queries. Unblocks TASK-003, TASK-006, TASK-005, TASK-013.
3. **TASK-004** — Redis Streams queue layer (RedisQueue interfaces). Unblocks TASK-005, TASK-007, TASK-015.
4. **TASK-003 + TASK-006** — Auth/sessions and Worker registration. Both depend on TASK-001 + TASK-002; can run in parallel.
5. **TASK-005 + TASK-013** — Task submission API and Pipeline CRUD. Both depend on TASK-002 + TASK-003 + TASK-004 (TASK-005). Can run in parallel.
6. **TASK-007** — Pipeline execution (Worker consumption loop). Depends on TASK-004 + TASK-005 + TASK-006 + TASK-013. Highest implementation complexity in Cycle 1.
7. **TASK-042** — Demo connectors. Depends on TASK-007 + TASK-013. Low complexity once TASK-007 is done.
8. **TASK-019** — React app shell and auth flow. Depends on TASK-003. Can run in parallel with TASK-005–007.
9. **TASK-025** — Worker fleet status API. Depends on TASK-003 + TASK-006.
10. **TASK-015** — SSE event infrastructure. Depends on TASK-003 + TASK-004. High complexity (RedisBroker.Start, ServeLogEvents).
11. **TASK-020** — Worker Fleet Dashboard GUI. Depends on TASK-019 + TASK-025 + TASK-015 + TASK-006.
12. **TASK-029** — Staging environment and CD pipeline. Depends on TASK-001 + TASK-042. Last Cycle 1 task.

---

## Component boundary ambiguity note

One ambiguity was discovered during scaffolding and is flagged for the Architect:

**Schema mapping validation (ADR-008, TASK-026):** The scaffold places a comment in PipelineHandler.Create noting that design-time validation is a Cycle 2 concern (TASK-026). However, ADR-008 states that design-time validation runs when a pipeline is saved. This means TASK-013 (Create/Update pipeline, Cycle 1) and TASK-026 (design-time validation, Cycle 2) are tightly coupled. The Builder implementing TASK-013 must leave a clear extension point for TASK-026 to wire in the validator. This is not an ambiguity that blocks Cycle 1, but the Builder for TASK-013 should be aware of it.

---

## Build and run instructions

### Prerequisites
- Go 1.23+
- Docker and Docker Compose
- Node.js 20+ (for frontend development)
- `golang-migrate` CLI (for manual migrations): `go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest`
- `sqlc` CLI (for query code generation): `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- `staticcheck` (for CI lint): `go install honnef.co/go/tools/cmd/staticcheck@latest`

### Development setup
```bash
cp .env.example .env
# Edit .env with your local settings (defaults work for docker compose)
make up
# Services start at:
#   API:    http://localhost:8080
#   Web:    http://localhost:3000
#   Redis:  localhost:6379
#   PG:     localhost:5432
```

### Frontend development (with hot reload)
```bash
# Start backend services only
docker compose up redis postgres api -d
# Run frontend dev server
npm --prefix web run dev
# Frontend at http://localhost:5173 with /api proxy to Go server
```

### Common tasks
```bash
make build          # Compile all Go packages
make test           # Run tests
make vet            # go vet
make lint           # staticcheck
make sqlc           # Regenerate sqlc query code
make migrate-up     # Run pending migrations
make scale-workers N=3  # Start 3 worker instances
```
