# ADR-007: Real-Time Communication Architecture

**Status:** Proposed
**Date:** 2026-03-26
**Characteristic:** Performance, Usability

## Context

Multiple GUI views require real-time updates without page refresh:
- REQ-016: Worker Fleet Dashboard updates worker status within one heartbeat-timeout interval
- REQ-017: Task Feed updates task state in real time
- REQ-018: Log Streamer delivers log lines within 2 seconds of production (NFR-003)
- DEMO-003: Sink Inspector shows Before/After comparison
- DEMO-004: Chaos Controller effects are visible in real time

ADR-004 selected SSE as the real-time mechanism. This ADR specifies the SSE channel architecture: how many channels, what data flows through each, and how the backend publishes events to SSE connections.

## Trade-off Analysis

### Channel Architecture

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Single SSE channel (all events multiplexed) | One connection per client; simple connection management | Client receives all events, must filter; bandwidth waste for unused event types; no per-resource subscription | Inefficient for users who only care about one view; but acceptable for a single-org system with limited concurrent users | Low -- split channels later |
| Per-resource SSE channels | Client subscribes only to what it needs (e.g., /events/tasks/{id}/logs); minimal bandwidth; clean separation | More server-side connection management; more SSE endpoints to implement; connection count scales with open resources | Connection count could grow if many resources are open simultaneously | Medium -- merge channels |
| Hybrid (per-view channels) | One channel per GUI view (tasks, workers, logs:{taskId}); balances simplicity and efficiency | Moderate number of channels; some events delivered to clients that do not need every item in the view | Good balance for a system with 4-6 distinct views | Low -- add or merge channels |

### Backend Event Distribution

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Redis Pub/Sub for event fan-out | Decouples event producers (API, monitor, workers) from SSE consumers; supports multiple API instances; Redis is already in the stack | Additional Redis channels to manage; Pub/Sub is fire-and-forget (no persistence for missed events) | Missed events if SSE client reconnects and Redis Pub/Sub message was already sent | Low -- add event replay from database |
| In-process EventEmitter | Zero latency; no external dependency for event distribution | Only works with a single API instance; breaks if API is scaled horizontally | Single-instance constraint; acceptable for the stated scale | Medium -- replace with Redis Pub/Sub when scaling |
| PostgreSQL LISTEN/NOTIFY | Uses existing PostgreSQL; transactional consistency with data changes | Limited payload size (8KB); less throughput than Redis Pub/Sub; adds load to PostgreSQL | Performance ceiling under high event rates | Medium |

## Decision

**Hybrid SSE channels with Redis Pub/Sub for event distribution.**

### SSE Endpoints

| Endpoint | Purpose | Events | Access control |
|---|---|---|---|
| `GET /events/tasks` | Task Feed updates | task:created, task:state-changed, task:completed, task:failed | User: own tasks only; Admin: all tasks |
| `GET /events/tasks/{id}/logs` | Log streaming for a specific task | log:line | Owner or Admin only (REQ-018) |
| `GET /events/workers` | Worker Fleet Dashboard | worker:registered, worker:heartbeat, worker:down | All authenticated users |
| `GET /events/sink/{taskId}` | Sink Inspector updates (demo) | sink:before-snapshot, sink:after-result | Owner or Admin only |

### Redis Pub/Sub Channels

| Channel | Publisher | Subscriber |
|---|---|---|
| `events:tasks:{userId}` | API (on task state change) | SSE handler for that user's task feed |
| `events:tasks:all` | API (on any task state change) | SSE handler for admin task feeds |
| `events:logs:{taskId}` | Worker (on log line produced) | SSE handler for that task's log stream |
| `events:workers` | Monitor (on worker status change) | SSE handler for worker dashboard |
| `events:sink:{taskId}` | Worker (on sink operation) | SSE handler for sink inspector |

**Door type:** Two-way -- SSE endpoints and Pub/Sub channels can be added, merged, or split without changing the core architecture.

**Cost to change later:** Low -- adding a new SSE channel is a new endpoint + a new Pub/Sub subscription. Switching from Redis Pub/Sub to a different event bus changes the distribution layer, not the SSE endpoints.

## Rationale

**Hybrid channels** give each GUI view its own SSE connection, which aligns with how users interact with the system: they open one view at a time (Task Feed, Worker Dashboard, Log Streamer). Per-view channels mean the client receives only relevant events. Per-resource channels (e.g., per-task logs) are used where the resource identity matters (log streaming for a specific task).

**Redis Pub/Sub** decouples the event producers from SSE consumers. Workers produce log lines and publish them to `events:logs:{taskId}` -- they do not need to know which SSE connections are open. The API subscribes to Pub/Sub channels and fans out to connected SSE clients. This works for a single API instance (current deployment model per ADR-005) and supports horizontal scaling if needed later.

**Fire-and-forget is acceptable** because SSE has built-in reconnection. If a client disconnects and reconnects, it misses events during the gap. For task state, the client can fetch current state on reconnect. For logs, the client can fetch historical logs from the API and resume streaming. This is a conscious trade-off: implementing persistent event streams (like Kafka) for GUI updates is over-engineered for a system with limited concurrent users.

### SSE reconnection strategy

The `Last-Event-ID` header is supported on the log streaming endpoint. Each log line SSE event includes a monotonically increasing ID. On reconnect, the client sends `Last-Event-ID` and the server replays missed log lines from the database. For task state and worker status, reconnection fetches current state via REST (no replay needed -- only the latest state matters).

## Fitness Function
**Characteristic threshold:** State changes visible in GUI within 2 seconds (NFR-003); log lines delivered within 2 seconds of production (REQ-018)

| | Specification |
|---|---|
| **Dev check** | Integration test: change task state via API, assert SSE client receives event within 2 seconds. Log streaming test: worker produces log line, assert SSE client receives it within 2 seconds. Reconnection test: disconnect SSE, produce 5 log lines, reconnect with Last-Event-ID, assert all 5 are replayed. |
| **Prod metric** | SSE event delivery latency (publish-to-client); active SSE connection count; Pub/Sub channel count; reconnection rate. |
| **Warning threshold** | Event delivery latency p95 > 1.5s; reconnection rate > 10% of connections per minute |
| **Critical threshold** | Event delivery latency p95 > 2s (NFR-003 breach); any SSE endpoint returning errors |
| **Alarm meaning** | Warning: event delivery is approaching the SLA boundary -- investigate Redis Pub/Sub latency or SSE handler backpressure. Critical: real-time updates are slower than the SLA -- users are seeing stale data. |

## Consequences
**Easier:** Adding new real-time views (e.g., pipeline chain progress) is a new SSE endpoint + Pub/Sub channel; workers publish events without knowing about the GUI; SSE works through standard HTTP proxies.
**Harder:** Horizontal scaling of the API requires all instances to subscribe to the same Redis Pub/Sub channels (but Redis Pub/Sub supports this natively). Event ordering across channels is not guaranteed (but each channel is ordered).
**Newly required:** SSE handler in the Go HTTP server (streaming HTTP responses via `http.Flusher`); Redis Pub/Sub subscription management via go-redis; per-user event filtering for access control; Last-Event-ID replay logic for log streaming; SSE client in the React frontend (EventSource API is native to browsers).
