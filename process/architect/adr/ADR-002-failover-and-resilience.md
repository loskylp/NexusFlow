# ADR-002: Failover and Resilience Strategy

**Status:** Proposed
**Date:** 2026-03-26
**Characteristic:** Reliability, Availability, Resilience

## Context

The system must detect downed workers and reassign their tasks without manual intervention (REQ-013, NFR-004). Workers self-register and emit heartbeats (REQ-004). When 50% of the worker fleet goes down simultaneously, all affected tasks must be re-queued and eventually processed (NFR-004). Infrastructure failures trigger retry with per-task configuration (REQ-011). Tasks that exhaust retries enter the dead letter queue with cascading cancellation for pipeline chains (REQ-012).

This decision defines: (a) how worker liveness is determined, (b) how tasks are reclaimed from downed workers, (c) how retry interacts with failover, and (d) what the heartbeat timeout should be.

## Trade-off Analysis

### Liveness Detection

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Heartbeat polling (workers push heartbeats, server checks expiry) | Simple; workers are stateless pushers; detection latency = heartbeat interval + timeout | False positives if network latency causes missed heartbeats; detection is not instantaneous | Premature failover triggers unnecessary re-queuing; too-slow detection delays recovery | Low -- timeout tuning is config |
| Lease-based (workers hold a lease on tasks, lease expires on failure) | Detection is per-task not per-worker; no separate heartbeat mechanism | More complex; every task assignment is a lease renewal; lease management overhead | Lease storms under high task throughput | Medium -- lease logic is intertwined with assignment |
| Peer-to-peer failure detection (workers monitor each other) | No central monitor; faster detection via gossip | Complex; requires worker-to-worker networking; split-brain risk | False positives from network partitions between workers | High -- fundamental architecture change |

### Task Reclamation

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Pending entry scanner (Redis Streams XPENDING + XCLAIM) | Native to Redis Streams; XCLAIM atomically reassigns a message to a new consumer; no custom state tracking | Scanning interval adds to detection-to-reassignment latency; requires periodic polling | Scanning interval too long delays reassignment; too short wastes CPU | Low -- interval is config |
| Event-driven reclamation (worker-down event triggers immediate reassignment) | Faster reassignment; no polling delay | Requires a reliable event bus for worker-down events; event loss means orphaned tasks | If the event is lost, tasks are never reclaimed | Medium -- need fallback scanner anyway |

### Heartbeat Timeout

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| 10 seconds (heartbeat every 3s, timeout at 10s) | Fast detection; quick failover | Higher false-positive rate from transient network issues; more unnecessary re-queuing | Flapping workers cause task churn | Low -- config change |
| 30 seconds (heartbeat every 10s, timeout at 30s) | Low false-positive rate; tolerant of network jitter | Slower detection; up to 30s of tasks stuck on a dead worker | Acceptable for batch workloads; poor for latency-sensitive tasks | Low -- config change |
| 60 seconds (heartbeat every 20s, timeout at 60s) | Very low false positives | Up to 60s detection delay; potentially unacceptable for SLA compliance | Too slow for a system with SLA obligations | Low -- config change |

## Decision

1. **Liveness detection:** Heartbeat polling -- workers push heartbeats to Redis; a monitor service checks expiry
2. **Task reclamation:** Pending entry scanner using XPENDING + XCLAIM, running on a configurable interval
3. **Heartbeat configuration:** Heartbeat interval 5 seconds, timeout 15 seconds (3 missed heartbeats = down), configurable per deployment
4. **Scanning interval:** Pending entry scanner runs every 10 seconds

**Door type:** Two-way -- all parameters are configurable; the mechanism (heartbeat + scanner) is a one-way choice but aligns with ADR-001's Redis Streams decision.

**Cost to change later:** Low for parameter tuning. Medium to switch from heartbeat polling to a fundamentally different detection mechanism (would require reworking the monitor service).

## Rationale

**Heartbeat polling** is the simplest mechanism that satisfies REQ-004. Workers already self-register (REQ-004); adding a periodic heartbeat push to Redis (SET with TTL or ZADD with timestamp) is trivial. The monitor checks for expired heartbeats, which is a single Redis operation. Peer-to-peer detection is over-engineered for a system where the central broker (Redis) is already the coordination point.

**XPENDING + XCLAIM** leverages ADR-001's Redis Streams decision. XPENDING returns messages read but not ACKed, with idle time per consumer. When a consumer's idle time exceeds the heartbeat timeout, XCLAIM atomically reassigns its pending messages to a healthy consumer. This is the exact primitive Redis Streams provides for this use case -- implementing it manually would duplicate what Streams already does.

**15-second timeout** (3 missed heartbeats at 5-second intervals) balances detection speed against false positives. For a system processing batch-style tasks (billing, reports, ETL), 15 seconds of detection delay is acceptable. The pending entry scanner adds up to 10 more seconds before reassignment, giving a worst-case 25-second detection-to-reassignment latency. This is well within the Brief's operational model (batch tasks, not interactive requests).

### Failover-retry interaction

When a worker is detected as down and its tasks are reclaimed:
1. The task's retry counter is incremented (REQ-011)
2. If retries remain, XCLAIM reassigns the message to the consumer group -- the next available matching worker picks it up
3. If retries are exhausted, the task is XADDed to `queue:dead-letter` and cascading cancellation fires for any downstream pipeline chain tasks (REQ-012)
4. The task state transitions: running/assigned -> queued (if retries remain) or running/assigned -> failed (if exhausted)

### Worker heartbeat storage

Workers write heartbeats as: `ZADD workers:active <timestamp> <worker-id>`

The monitor service runs `ZRANGEBYSCORE workers:active -inf <now - 15s>` to find expired workers. This is O(log N) per check, negligible even at fleet sizes of hundreds of workers.

## Fitness Function
**Characteristic threshold:** Failover detection within 25 seconds; zero task loss during worker failure; automatic recovery without operator intervention

| | Specification |
|---|---|
| **Dev check** | Integration test: start 2 workers, submit 5 tasks, kill worker with 3 in-flight tasks, verify all 3 are reassigned to surviving worker and complete. Assert no operator action required. Assert task retry counter incremented. |
| **Prod metric** | Monitor: workers:active sorted set size; XCLAIM count per scan interval; failover event count; detection-to-reassignment latency histogram. |
| **Warning threshold** | Detection-to-reassignment latency > 30s; more than 3 failover events in 5 minutes (possible flapping) |
| **Critical threshold** | Detection-to-reassignment latency > 60s; any task orphaned (pending > 60s with no XCLAIM); worker count drops below 1 |
| **Alarm meaning** | Warning: failover is slower than expected or workers are unstable -- check network and worker health. Critical: tasks are stuck or the fleet is depleted -- immediate investigation required. |

## Consequences
**Easier:** Worker implementation (just push heartbeats on a timer); failover is automatic; retry configuration per-task gives operators control.
**Harder:** Tuning heartbeat/timeout parameters for different environments (dev vs. staging vs. production may need different values). Debugging failover storms if heartbeat timeout is too aggressive.
**Newly required:** Monitor service (or monitor loop within the Producer) that runs the heartbeat check and pending entry scan. Worker heartbeat push logic. Failover event logging for observability.
