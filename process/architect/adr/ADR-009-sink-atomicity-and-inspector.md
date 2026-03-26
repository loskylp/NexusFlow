# ADR-009: Sink Atomicity and Sink Inspector "Before" State Capture

**Status:** Proposed
**Date:** 2026-03-26
**Characteristic:** Data Integrity, Observability

## Context

REQ-008 requires atomic Sink operations: if a Sink fails partway through writing, all partial writes are rolled back. DEMO-003 requires a Sink Inspector that shows Before/After comparison of destination data. The Auditor deferred AUDIT-009 (how the "Before" state is captured) to the Architecture Gate.

This ADR decides: (a) the atomicity mechanism for Sink operations, and (b) the "Before" state capture mechanism for the Sink Inspector.

## Trade-off Analysis

### Sink Atomicity Mechanism

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Sink-type-specific transactions (e.g., DB transaction for DB sinks, S3 multipart abort for S3 sinks) | Uses native atomicity of each destination; most reliable; leverages destination guarantees | Each Sink type needs its own rollback implementation; more code per sink type | If a Sink type lacks native transactions, a fallback is needed | Medium -- adding a new Sink type requires implementing its rollback |
| Staging pattern (write to temp location, move on success, delete on failure) | Universal across Sink types; single pattern for all destinations | Requires temp storage; move operation must be atomic; not all destinations support atomic move | Temp-to-final move is not atomic in all systems (e.g., S3 has no atomic rename) | Medium |
| Compensating writes (write normally, delete on failure) | Simple implementation; works anywhere you can delete | Race condition: downstream consumers may read partial data before rollback; not truly atomic | Data consumers see partial state between write and rollback | Low -- but the approach is fundamentally flawed for atomicity |

### "Before" State Capture (AUDIT-009)

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Pre-execution snapshot (query destination state before Sink runs) | Simple; captures exactly what was there before; works for any readable destination | Adds a read operation before every Sink execution; snapshot can be large for big destinations; slight latency increase | Snapshot is stale if another process writes to the destination between snapshot and Sink execution (acceptable for demo infrastructure) | Low -- it is a read operation before the Sink |
| Change Data Capture (CDC) from destination | No pre-read needed; captures every change | Requires CDC support from the destination; complex setup; over-engineered for demo infrastructure | Over-engineering a demo feature | High |
| Diff-based (capture full state before and after, compute diff) | Shows exactly what changed; most informative for demo | Two full reads of destination data; expensive for large datasets; diff computation complexity | Performance for large destinations | Low -- but expensive |

## Decision

1. **Sink atomicity:** Sink-type-specific transaction wrappers
   - **Database sinks:** Use database transactions (BEGIN/COMMIT/ROLLBACK)
   - **S3-compatible sinks (including MinIO for DEMO-001):** Use multipart upload; abort on failure; write to a staging prefix, then copy to final location on success
   - **File sinks:** Write to temp file; rename on success; delete temp on failure

2. **"Before" state capture (AUDIT-009 resolution):** Pre-execution snapshot
   - Before the Sink phase begins, the worker queries the destination to capture the current state relevant to the Sink's output scope
   - The snapshot is stored as a JSON document attached to the task execution record
   - After Sink completion (or rollback), the "After" state is similarly captured
   - Both snapshots are published to the `events:sink:{taskId}` SSE channel (ADR-007) for the Sink Inspector to display

**Door type:** One-way for the atomicity pattern (each Sink type is implemented with its own transaction wrapper). Two-way for the "Before" capture mechanism (pre-execution snapshot is a read hook, easily changed).

**Cost to change later:** Medium for atomicity (adding a new Sink type requires implementing its transaction wrapper, but existing Sink types are unaffected). Low for "Before" capture (it is a hook in the Sink execution flow).

## Rationale

**Sink-type-specific transactions** are chosen because atomicity means different things for different destinations. A database Sink uses database transactions -- the most reliable atomicity mechanism available. An S3 Sink uses multipart upload with abort. Attempting a universal pattern (staging or compensating writes) would either not work for all destinations or provide weaker guarantees than the native mechanisms. The cost is per-Sink-type implementation, but the Brief defines a limited set of Sink types (the demo infrastructure uses S3 via MinIO and PostgreSQL), so the implementation burden is bounded.

**Pre-execution snapshot** for AUDIT-009 is chosen because:
- It is the simplest mechanism that satisfies DEMO-003's Before/After requirement
- The Sink Inspector is demo infrastructure, not production telemetry -- it does not need CDC-level sophistication
- The snapshot scope is limited to what the Sink will write (e.g., "SELECT * FROM target_table WHERE [scope condition]" for a DB sink, or list objects in the target S3 prefix)
- The snapshot is stored as JSON, making it easy to render in the Sink Inspector GUI as a Before/After comparison
- The staleness risk (another process writing to the destination between snapshot and Sink execution) is acceptable for a demo context where the destination is controlled (DEMO-001 Fake-S3, DEMO-002 Mock-Postgres)

### Execution flow with snapshot

```
1. Worker receives task
2. DataSource phase executes
3. Process phase executes
4. Sink phase begins:
   a. Worker queries destination -> "Before" snapshot (JSON)
   b. Worker stores "Before" snapshot in task execution record
   c. Worker publishes sink:before-snapshot event via Redis Pub/Sub
   d. Worker executes Sink writes within transaction wrapper
   e. On success: COMMIT; capture "After" snapshot; publish sink:after-result
   f. On failure: ROLLBACK; capture "After" snapshot (should match "Before"); publish sink:after-result with error
5. Task state transitions to completed or failed
```

### Execution ID and idempotency (ADR-003 integration)

The Sink transaction wrapper checks the execution ID (task ID + attempt number) before writing. If this execution ID has already been applied (checked via a deduplication table or idempotency key in the destination), the Sink skips the write and returns success. This integrates ADR-003's at-least-once delivery guarantee with the Sink's atomic boundary.

## Fitness Function
**Characteristic threshold:** Zero partial writes at Sink destinations; "Before" snapshot captured for every Sink execution in demo mode

| | Specification |
|---|---|
| **Dev check** | Atomicity test: force Sink failure mid-write (e.g., after 50 of 100 records), verify destination has zero records from this execution. Snapshot test: run a Sink, verify "Before" and "After" JSON documents are stored and differ by exactly the Sink's output. Rollback test: force Sink failure, verify "After" snapshot matches "Before" snapshot. |
| **Prod metric** | Sink failure count; rollback count; snapshot capture latency; Sink execution duration histogram. |
| **Warning threshold** | Sink failure rate > 5% of executions; snapshot capture taking > 5 seconds |
| **Critical threshold** | Any partial write detected at a Sink destination (atomicity violation); snapshot capture failure blocking Sink execution |
| **Alarm meaning** | Warning: Sinks are failing frequently -- investigate destination health. Critical: atomicity guarantee is broken -- data integrity at the Sink destination may be compromised. |

## Consequences
**Easier:** Each Sink type uses its native atomicity mechanism, which is the most reliable approach; Sink Inspector has structured Before/After data to render; idempotency is enforced at the atomic boundary.
**Harder:** Each new Sink type requires implementing both a transaction wrapper and a snapshot reader; snapshot scope must be defined per Sink type.
**Newly required:** Sink transaction wrapper interface (per Sink type); snapshot reader interface (per Sink type); execution ID deduplication check at Sink boundary; snapshot storage in task execution record; SSE events for sink:before-snapshot and sink:after-result.
