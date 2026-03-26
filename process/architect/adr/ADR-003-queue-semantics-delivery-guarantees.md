# ADR-003: Queue Semantics and Delivery Guarantees

**Status:** Proposed
**Date:** 2026-03-26
**Characteristic:** Reliability, Data Integrity

## Context

The system must guarantee that submitted tasks are processed despite infrastructure failures (REQ-011, NFR-004). The choice of delivery guarantee -- at-most-once, at-least-once, or exactly-once -- determines how the system handles the gap between a worker receiving a task and acknowledging completion. This decision interacts with ADR-001 (Redis Streams with XACK) and ADR-002 (failover via XCLAIM).

Sink operations must be atomic (REQ-008). Pipeline chains trigger downstream tasks on completion (REQ-014). These create side effects that are sensitive to duplicate delivery: a task processed twice could write to a Sink twice, or trigger a downstream chain task twice.

## Trade-off Analysis

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| At-most-once | Simplest; no duplicate handling needed | Tasks can be lost on worker failure (message consumed but not processed); violates NFR-002 and NFR-004 | Unacceptable -- SLA obligations require task completion | High -- fundamentally changes the reliability model |
| At-least-once with idempotency guards | Tasks are never lost; XCLAIM on failure means retried tasks are redelivered; idempotency guards prevent duplicate side effects | Workers must implement idempotency checks; Sink operations need deduplication; slight complexity increase | Duplicate side effects if idempotency is not implemented correctly | Medium -- idempotency logic is per-sink-type |
| Exactly-once (distributed transactions) | No duplicates, no loss -- the ideal | Requires two-phase commit or similar coordination across Redis, worker, and Sink destination; massive complexity; Redis does not natively support distributed XA transactions | Over-engineering; introduces latency; fragile under partial failure | Critical -- distributed transaction coordinator is a separate system |

## Decision

**At-least-once delivery with idempotency guards at the Sink boundary.**

The queue guarantees at-least-once: if a worker fails before XACK, the task is redelivered via XCLAIM (ADR-002). Idempotency is enforced at two points:

1. **Sink operations:** Each Sink execution is tagged with a unique execution ID (task ID + attempt number). The Sink checks whether this execution ID has already been applied before writing. This makes Sink writes idempotent regardless of redelivery.

2. **Pipeline chain triggers:** When a task completes and triggers a downstream chain task (REQ-014), the trigger is guarded by a Redis SET-NX with the triggering task's completion event ID. Duplicate completions (from redelivery) do not create duplicate downstream tasks.

**Door type:** One-way -- the delivery guarantee model shapes how every Sink, every chain trigger, and every worker completion handler is implemented.

**Cost to change later:** High -- switching to exactly-once would require a distributed transaction coordinator. Switching to at-most-once would violate core requirements.

## Rationale

**At-most-once is eliminated** by NFR-004 and REQ-011: the system must survive worker failures and retry tasks. Losing a task on worker failure is unacceptable when downstream teams depend on task completion.

**Exactly-once is eliminated** by engineering pragmatism: true exactly-once across Redis, the worker process, and an external Sink destination requires distributed transactions. Redis does not support XA. The complexity-to-value ratio is extreme, and the system's workload (batch ETL, reports, data exports) is naturally suited to idempotent operations.

**At-least-once with idempotency** is the standard pattern for reliable distributed task processing. It aligns with how Redis Streams work (XACK is the acknowledgment; no XACK means redelivery). The idempotency burden falls on the Sink implementation, which already has an atomicity requirement (REQ-008). Adding an execution ID check to the Sink's atomic boundary is a natural extension -- the Sink already wraps its writes in a transaction-like scope for rollback; adding a deduplication check at the start of that scope is minimal additional work.

### Ordering guarantees

Redis Streams provide FIFO ordering within a single stream. Per ADR-001, each tag has its own stream. This means:
- Tasks with the same tag are delivered in submission order (FIFO within the stream)
- Tasks with different tags have no ordering guarantee relative to each other (independent streams)
- Pipeline chain tasks are triggered sequentially by definition (A completes before B is submitted), so ordering is inherent

Priority queuing is not required by any current requirement. If needed in the future, it can be added via multiple streams per tag with priority-based consumption.

## Fitness Function
**Characteristic threshold:** Zero task loss; zero duplicate Sink writes; zero duplicate chain triggers

| | Specification |
|---|---|
| **Dev check** | Integration test: submit a task, kill the worker mid-execution (before XACK), verify the task is redelivered and completes exactly once at the Sink (check execution ID deduplication). Chain trigger test: complete a task in a chain, simulate duplicate completion event, verify only one downstream task is created. |
| **Prod metric** | Monitor: XCLAIM count (redeliveries); Sink deduplication rejection count; chain trigger deduplication count. |
| **Warning threshold** | Redelivery rate > 5% of total tasks (suggests worker instability); any Sink deduplication rejection (suggests a correctness issue worth investigating) |
| **Critical threshold** | Any duplicate Sink write detected (deduplication guard bypassed); any duplicate chain trigger detected |
| **Alarm meaning** | Warning: workers are failing frequently or idempotency guards are activating -- investigate worker stability. Critical: a duplicate side effect occurred -- data integrity may be compromised. |

## Consequences
**Easier:** Reliability -- tasks are never lost; Sink atomicity (REQ-008) already provides the transactional boundary where idempotency checks fit naturally; chain triggers are safe from duplication.
**Harder:** Every Sink implementation must include an execution ID check; workers must pass execution IDs through the pipeline; testing must cover the redelivery + idempotency path.
**Newly required:** Execution ID generation (task ID + attempt number); Sink-level deduplication check; chain trigger deduplication via Redis SET-NX; monitoring for deduplication rejections.
