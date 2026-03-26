# ADR-008: Data Model and Schema Migration Strategy

**Status:** Revised (supersedes v1)
**Date:** 2026-03-26
**Characteristic:** Maintainability, Data Persistence, Reliability

## Context

The system persists relational data in PostgreSQL (ADR-004): users, pipelines, pipeline chains, tasks, task state history, and logs. The data model must support the domain invariants from the Brief and the query patterns required by the GUI and API. The schema will evolve across development cycles as requirements are refined and features are added.

This ADR also resolves two audit deferrals:
- **AUDIT-005 (Log retention):** Specify concrete retention periods for task logs
- **AUDIT-007 (Schema mapping validation timing):** Specify when schema mappings are validated

**Revision note:** The backend stack is now Go (ADR-004 revised). Prisma is a Node.js/TypeScript-specific ORM and is incompatible with Go. This revision selects a Go-compatible migration tool and database access approach.

## Trade-off Analysis

### Schema Migration Tool

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| golang-migrate | Well-established; supports up and down migrations; CLI and Go library; plain SQL migration files; works with any database driver; large community | No schema generation -- migrations are hand-written SQL; no type generation | Standard tool for Go projects; low risk | Low -- migration files are plain SQL, portable to any tool |
| goose | Simple; supports Go-based and SQL-based migrations; embedded in Go binary; supports up/down | Smaller community than golang-migrate; less ecosystem tooling | Adequate but fewer integrations | Low -- SQL migrations are portable |
| Atlas (Ariga) | Declarative schema management (schema-as-code); auto-generates migrations from schema diff; built-in linting; supports Go | Newer tool; more complex setup; declarative model is a paradigm shift from imperative migrations; commercial features behind paywall | Learning curve; paradigm may not suit all developers | Medium -- declarative schema definitions are Atlas-specific |
| Raw SQL scripts | Full control; zero tool dependency | No migration tracking; must implement ordering and idempotency manually; error-prone | Fragile at scale; manual tracking leads to missed migrations | High -- no tooling to revert or track state |

### Database Access (replaces Prisma ORM)

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| pgx (direct driver) + sqlc (type-safe code generation) | pgx is the standard Go PostgreSQL driver; sqlc generates type-safe Go code from SQL queries; full SQL control; compile-time query validation | Must write SQL manually (but sqlc validates it); two tools to learn | Standard Go pattern; well-established | Medium -- sqlc-generated code is replaceable |
| pgx (direct driver) only | Maximum control; no code generation dependency; straightforward | Manual struct scanning; no compile-time query validation; verbose | More boilerplate but maximum flexibility | Low -- pgx is the standard driver regardless |
| GORM | ORM for Go; less SQL to write; active record pattern | Magic behavior; performance overhead; harder to optimize queries; hides PostgreSQL-specific features | ORM impedance mismatch; debugging generated queries | High -- ORM patterns pervade the codebase |

### Log Storage

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| PostgreSQL with TTL-based partition pruning | Single database; queryable; partitioned by date for efficient pruning; retention is a partition drop | Table bloat under high log volume; PostgreSQL is not optimized for append-only time-series data | Acceptable at 10K tasks/hour; logs are text lines, not high-cardinality metrics | Medium -- migrate to dedicated log store |
| Dedicated log store (Elasticsearch, Loki) | Purpose-built for log ingestion and search; better for high volume | Additional infrastructure; operational overhead; overkill for the current scale | Over-engineering | High -- additional system to operate |
| Redis Streams for logs (TTL-based) | Already in the stack; natural TTL via XTRIM; good write throughput | Not queryable for historical analysis; limited to MAXLEN-based retention; no full-text search | Acceptable for real-time streaming + recent history; not for long-term retention | Medium |

## Decision

1. **Schema migrations:** golang-migrate with plain SQL migration files committed to the repository; supports both up and down migrations
2. **Database access:** pgx as the PostgreSQL driver, with sqlc for type-safe Go code generation from SQL queries
3. **Log storage:** Dual storage
   - **Hot logs (0-72 hours):** Redis Streams per task (`logs:{taskId}`) with MAXLEN cap. Workers XADD log lines here. SSE streams from here. Provides real-time streaming and recent history.
   - **Cold logs (0-30 days):** PostgreSQL `task_logs` table, partitioned by week. A background goroutine copies log lines from Redis Streams to PostgreSQL periodically (every 60 seconds). After 30 days, weekly partitions are dropped.
4. **Schema mapping validation:** Both design-time and runtime
   - **Design-time:** When a pipeline definition is saved (via GUI Pipeline Builder or REST API), schema mappings are validated against the declared output schema of the preceding phase. Invalid mappings are rejected with a clear error.
   - **Runtime:** Schema mappings are re-validated at execution time against the actual output data. This catches cases where the DataSource output has changed since the pipeline was defined.

**Door type:** Two-way for log retention policy (durations are configurable). One-way for schema mapping validation approach (design-time validation is wired into the pipeline save flow).

**Cost to change later:** Low for retention durations (configuration). Medium for adding or removing design-time validation (it is a validation step in the save endpoint, not a deep architectural change). Low for switching migration tools (migration files are plain SQL, portable between golang-migrate, goose, and others).

## Rationale

### Migration tool (revised for Go)

**golang-migrate** is chosen because: (a) it is the most widely adopted migration tool in the Go ecosystem; (b) migration files are plain SQL -- no tool-specific DSL, no lock-in; (c) it provides both a CLI for manual use and a Go library for embedding migrations in the application binary; (d) it tracks migration state in a `schema_migrations` table; (e) it supports both up and down migrations, enabling rollback when needed.

**sqlc** complements golang-migrate by generating type-safe Go structs and query functions from SQL. The developer writes SQL queries in `.sql` files, and sqlc generates Go code that uses pgx under the hood. This provides compile-time validation of queries against the schema -- similar to the type safety Prisma provided in the Node.js stack, but without an ORM's runtime overhead.

### Migration strategy

- Migrations are forward-only in production (down migrations exist for development rollback but are not used in production)
- Rollback procedure: deploy the previous application version, which is compatible with the current schema (backward-compatible migrations only)
- Migration testing: CI runs migrations against a fresh database and against a seeded database before accepting
- Migrations run automatically on application startup via the embedded golang-migrate library, or manually via CLI

### Log retention (AUDIT-005 resolution)

The dual-storage approach separates concerns:
- **Real-time streaming needs** are served by Redis Streams. Workers write log lines to `logs:{taskId}` via XADD. The SSE log streaming endpoint reads from this stream. Redis Streams support XRANGE for replay on reconnect (ADR-007, Last-Event-ID).
- **Historical query needs** (searching past task logs, debugging failed tasks) are served by PostgreSQL. Weekly partitioning makes retention enforcement trivial: `DROP` partitions older than 30 days.
- **30-day retention** is a reasonable default for a system where dependent teams may need to investigate task failures. It is configurable per deployment.
- **72-hour Redis hot window** keeps the most recent logs in the fastest store. After 72 hours, log lines are only in PostgreSQL.

### Schema mapping validation timing (AUDIT-007 resolution)

**Both design-time and runtime** is chosen because:
- **Design-time validation** catches obvious errors immediately (misspelled field names, missing fields). This is a UX enhancement that prevents users from saving pipelines that will always fail. The Pipeline Builder (REQ-015) already requires the user to define mappings interactively -- validating during save is a natural extension.
- **Runtime validation** is still required because DataSource output can change between pipeline definition and execution (e.g., an external data source adds or removes fields). REQ-007's acceptance scenarios test runtime behavior and remain the authoritative test.
- This does not contradict REQ-007 -- it supplements it. REQ-007 specifies what happens at runtime (mapping applied, error on missing field). Design-time validation is an additional check that prevents knowably-bad pipelines from being saved.

### Core data model

```
User           { id, username, passwordHash, role, active, createdAt }
Pipeline       { id, name, userId, dataSourceConfig, processConfig, sinkConfig, schemaMappings, createdAt, updatedAt }
PipelineChain  { id, name, userId, pipelineIds (ordered), createdAt }
Task           { id, pipelineId, chainId?, userId, status, retryConfig, retryCount, executionId, workerId?, input, createdAt, updatedAt }
TaskStateLog   { id, taskId, fromState, toState, reason, timestamp }
Worker         { id, tags, status, lastHeartbeat, registeredAt }
TaskLog        { id, taskId, line, level, timestamp }
```

Domain invariants enforced at the database level:
- Task status transitions: CHECK constraint on valid (fromState, toState) pairs in TaskStateLog, enforced by a trigger
- User-pipeline ownership: foreign key from Pipeline.userId to User.id
- Cascade behavior: deactivating a user does NOT cascade to tasks (REQ-020 -- "deactivation does not cancel in-flight tasks")

## Fitness Function
**Characteristic threshold:** Zero data loss during schema migration; log retention enforced within 1 day of threshold; design-time validation catches invalid mappings before save

| | Specification |
|---|---|
| **Dev check** | Migration test: apply all migrations to a fresh DB, seed data, verify schema matches sqlc-generated expectations. Down migration test: apply up then down, verify clean rollback. Log retention test: insert logs with timestamps > 30 days old, run partition pruning, verify old logs are removed. Schema validation test: attempt to save a pipeline with an invalid mapping, assert rejection with clear error message. sqlc compilation test: `sqlc compile` succeeds with zero errors. |
| **Prod metric** | Migration duration; partition count and size; Redis Stream length per task; schema validation rejection count. |
| **Warning threshold** | Any migration taking > 30 seconds; task_logs table total size > 10GB; Redis Stream for any task > 100K entries |
| **Critical threshold** | Migration failure in CI or staging; log partitions not pruned for > 7 days past retention; design-time validation allowing an invalid mapping to be saved |
| **Alarm meaning** | Warning: data growth is outpacing retention -- check partition pruning job. Critical: schema integrity or data retention is compromised. |

## Consequences
**Easier:** Log streaming from Redis is fast and does not load PostgreSQL; historical log queries are efficient with partitioned PostgreSQL; design-time validation gives users immediate feedback in the Pipeline Builder; sqlc provides compile-time query validation; plain SQL migrations are portable and auditable; golang-migrate is widely understood.
**Harder:** Dual log storage requires a background sync goroutine; partition management is an operational task; design-time validation requires each pipeline phase to declare its output schema; SQL queries must be hand-written (but sqlc validates them).
**Newly required:** golang-migrate setup with migration directory; sqlc configuration and query files; background log sync goroutine (Redis -> PostgreSQL); weekly partition creation/pruning cron; output schema declaration per pipeline phase type; design-time schema mapping validator in the pipeline save handler.
