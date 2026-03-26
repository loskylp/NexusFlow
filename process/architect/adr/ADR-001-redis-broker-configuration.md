# ADR-001: Redis Broker Configuration and Queue Topology

**Status:** Proposed
**Date:** 2026-03-26
**Characteristic:** Reliability, Data Persistence, Performance

## Context

Redis is a Nexus-stated constraint as the persistent broker (Brief -- Context and Ground Truths). This ADR does not decide *whether* to use Redis -- that is settled. It decides *how* Redis is configured: persistence mode, queue data structure, and queue topology.

The system must satisfy:
- NFR-001: Queuing latency under 50ms at p95 under sustained 10,000 tasks/hour load
- NFR-002: Queued tasks survive a Redis restart with zero loss
- REQ-003: Tasks transition to "queued" reliably after validation
- REQ-012: Dead letter queue as a distinct holding area for terminally failed tasks
- REQ-005: Tag-based routing requires either per-tag queues or filtered consumption

If Redis is misconfigured, queued tasks are lost on restart (violating NFR-002), or queuing latency exceeds SLA (violating NFR-001).

## Trade-off Analysis

### Persistence Mode

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| AOF (appendfsync everysec) | Near-zero data loss (max 1s window); fast writes; standard production config | Up to 1 second of queued tasks could be lost on crash; larger disk footprint than RDB | Acceptable loss window for task queue -- tasks can be resubmitted | Low -- Redis config change, no application change |
| AOF (appendfsync always) | Zero data loss -- every write flushed to disk | Significant write latency penalty; risks violating NFR-001 under sustained load | Latency SLA breach | Low -- Redis config change |
| RDB snapshots only | Minimal write overhead; fastest throughput | Minutes of data loss on crash; violates NFR-002 for any task queued between snapshots | Unacceptable data loss | Low -- Redis config change |
| AOF + RDB (hybrid) | Fast recovery (RDB for bulk restore, AOF for recent ops); near-zero data loss | Slightly higher disk I/O; more complex backup strategy | Acceptable -- standard production pattern | Low -- Redis config change |

### Queue Data Structure

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Redis Streams (XADD/XREADGROUP) | Consumer groups for worker coordination; built-in acknowledgment (XACK); message persistence; replay capability; at-least-once delivery native | Slightly more complex than LIST ops; requires consumer group management | Minimal -- Streams are purpose-built for this pattern | Medium -- changing queue structure requires migration of in-flight tasks |
| Redis LIST (LPUSH/BRPOP) | Simple; low latency; well-understood | No built-in acknowledgment; manual tracking of in-flight tasks; no consumer groups; manual dead-letter routing | Lost tasks on worker crash between POP and completion; violates NFR-004 | Medium -- same migration concern |
| Redis Pub/Sub | Real-time fan-out; simple subscription model | No persistence -- messages lost if no subscriber is listening; fundamentally incompatible with NFR-002 | Complete data loss for any undelivered message | High -- architectural rethink |

### Queue Topology

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Single stream + tag filtering | Simple topology; one stream to monitor | All workers read all messages; filtering overhead at consumer level; bottleneck under high throughput | Performance degradation as task variety grows | Medium -- split streams later |
| Per-tag streams (queue:etl, queue:report, etc.) | Workers only consume relevant streams; natural load isolation; parallelism by tag | More streams to manage; workers with multiple tags consume from multiple streams; dynamic stream creation needed | Stream proliferation if tags are fine-grained | Medium -- merge or split streams |
| Priority-tagged single stream with consumer groups | One stream; consumer group handles distribution; priority via score-based pending list | Priority ordering not native to Streams; requires custom logic | Complexity for limited benefit at current scale | Medium |

## Decision

1. **Persistence:** AOF + RDB hybrid (appendfsync everysec)
2. **Queue structure:** Redis Streams with consumer groups
3. **Queue topology:** Per-tag streams with a routing layer

**Door type:** Two-way for persistence config; One-way for queue data structure (switching from Streams to LIST after implementation would require significant rework of consumer group logic, acknowledgment handling, and dead-letter routing)

**Cost to change later:** Persistence: Low (config change). Queue structure: High (consumer group logic, acknowledgment patterns, and dead-letter routing are all coupled to the Streams API). Queue topology: Medium (adding or merging streams is manageable with a routing abstraction).

## Rationale

**AOF + RDB hybrid** gives near-zero data loss (max 1 second window with everysec) while maintaining write throughput well within NFR-001's 50ms p95 target. RDB snapshots provide fast recovery on restart. The 1-second loss window is acceptable because: (a) task submission is idempotent from the client's perspective -- the client received a task ID and can query status; (b) the Brief models SLA-sensitive but not life-critical operations.

**Redis Streams** are the natural fit because they provide exactly the primitives this system needs: consumer groups for coordinating a worker fleet (REQ-004, REQ-005), built-in acknowledgment via XACK for at-least-once delivery (critical for NFR-004 failover), message persistence (NFR-002), and pending entry tracking for detecting tasks assigned to downed workers (REQ-013). LIST-based queues would require reimplementing all of these primitives manually.

**Per-tag streams** align with REQ-005 (tag-based task-to-worker matching). Workers subscribe only to streams matching their capability tags, avoiding wasted reads. A routing layer in the Producer maps task tags to the appropriate stream at enqueue time, and handles stream creation for new tags. This keeps the topology manageable while providing natural parallelism.

### Stream naming convention
- Task queues: `queue:{tag}` -- one stream per capability tag
- Dead letter queue: `queue:dead-letter` -- single stream for all terminally failed tasks (REQ-012)
- Worker heartbeats: `workers:heartbeat` -- used by the monitoring layer (REQ-004)

### Consumer group configuration
- One consumer group per stream, named `workers`
- Each worker is a consumer within the group, identified by its worker ID
- XACK on task completion or terminal failure
- Pending entries (tasks read but not ACKed) older than heartbeat timeout are candidates for reassignment (REQ-013)

## Fitness Function
**Characteristic threshold:** Zero task loss across Redis restart; queuing latency under 50ms p95

| | Specification |
|---|---|
| **Dev check** | Integration test: enqueue 100 tasks, restart Redis, verify all 100 are recoverable from the stream. Latency test: enqueue 1,000 tasks sequentially, assert p95 queuing time < 50ms. |
| **Prod metric** | Monitor `queue:{tag}` stream length, pending entry count, and XADD latency via Redis INFO and Slowlog. |
| **Warning threshold** | Pending entries > 100 for any stream (tasks assigned but not ACKed); XADD p95 latency > 30ms |
| **Critical threshold** | Pending entries > 500; XADD p95 latency > 45ms; any task loss detected after Redis restart |
| **Alarm meaning** | Warning: workers are falling behind or slowing down -- investigate fleet capacity. Critical: approaching SLA breach on queuing latency or tasks may be lost -- immediate investigation required. |

## Consequences
**Easier:** Worker coordination (consumer groups handle distribution); dead-letter routing (failed tasks XADD to dead-letter stream); failover detection (pending entries with stale consumers); at-least-once delivery (XACK-based).
**Harder:** Schema evolution of stream entries (field changes require careful versioning); operational monitoring requires Redis Streams familiarity.
**Newly required:** Stream routing abstraction in the Producer service; consumer group initialization on service startup; pending entry scanner for failover detection; Redis Streams monitoring dashboards.
