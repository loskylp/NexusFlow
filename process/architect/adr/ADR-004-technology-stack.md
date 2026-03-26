# ADR-004: Technology Stack

**Status:** Revised (supersedes v1)
**Date:** 2026-03-26
**Characteristic:** Maintainability, Performance, Deployability

## Context

The Manifest identifies the technology stack as an open decision (Provisional Assumptions: "Worker implementation language and framework are not yet specified -- the Architect will determine these"). The system has two interaction surfaces: a web GUI with real-time updates (REQ-015 through REQ-018, DEMO-001 through DEMO-004) and a REST API for programmatic integration (REQ-001, REQ-022). The backend must coordinate with Redis Streams (ADR-001), manage worker communication, and support real-time streaming (log streaming, state updates).

The Nexus is a solo developer building this as a portfolio project (Brief -- Stakeholders). Technology choices must balance production-grade capability with solo-developer productivity.

**Revision note:** The Nexus directed the backend stack to use Go instead of Node.js/TypeScript. This is a value-based technology preference from the Nexus, not a contested architectural decision. The Architect records the trade-offs and adapts dependent decisions accordingly.

## Trade-off Analysis

### Backend Language/Framework

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Go (stdlib net/http or chi/echo) | Excellent concurrency via goroutines; fast runtime; single binary deployment; strong Redis client ecosystem (go-redis); native support for SSE via streaming HTTP responses; low memory footprint; strong static typing | No shared types with React frontend; slower iteration velocity vs. interpreted languages; more verbose error handling; smaller web framework ecosystem than Node.js | Development velocity -- more code per feature; but the architecture is service-oriented and Go excels at services | Critical -- full rewrite |
| Node.js (Express/Fastify) + TypeScript | Excellent Redis client ecosystem (ioredis); native async/event-loop model; TypeScript adds type safety; shared types with React frontend; fast development velocity | Single-threaded (CPU-bound work blocks event loop); callback/promise complexity | CPU-bound tasks need process isolation (already the architecture) | Critical -- full rewrite |
| Python (FastAPI) | Clean async support; fast prototyping; strong ETL/data processing ecosystem | Slower runtime; GIL; async ecosystem less mature for streaming | Performance ceiling for high-throughput queue operations | Critical -- full rewrite |
| Java (Spring Boot) | Enterprise-grade Redis support; strong typing; mature ecosystem | Heavy framework; slow startup; high memory; verbose | Over-engineering for a portfolio project | Critical -- full rewrite |

### Frontend Framework

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| React + TypeScript | Dominant ecosystem; excellent component libraries; strong drag-and-drop libraries (react-dnd, dnd-kit) for Pipeline Builder | Bundle size; state management complexity for real-time updates | Minimal -- React is the safe choice for this kind of application | High -- full frontend rewrite |
| Vue.js | Simpler mental model; good reactivity system; smaller bundle | Smaller ecosystem for drag-and-drop; fewer enterprise component libraries | Less ecosystem support for complex interactive UIs like the Pipeline Builder | High -- full frontend rewrite |
| Svelte/SvelteKit | Smallest bundle; excellent reactivity; fast | Smallest ecosystem; fewer drag-and-drop libraries; riskier for a complex UI like Pipeline Builder | Ecosystem gaps for drag-and-drop canvas | High -- full frontend rewrite |

### Real-time Communication

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| Server-Sent Events (SSE) | Simple; HTTP-based; automatic reconnection; sufficient for server-to-client streaming; works through proxies and Traefik | Unidirectional (server to client only); limited to text data | Sufficient for this use case -- all real-time needs are server-to-client pushes | Low -- SSE and WebSocket can coexist |
| WebSocket | Bidirectional; binary support; lower overhead per message | More complex setup; requires sticky sessions or Redis pub/sub for multi-instance | Over-engineered for server-to-client streaming | Medium -- WebSocket infrastructure is more involved |
| Long polling | Universal compatibility; simple server implementation | Higher latency; more HTTP overhead | Unacceptable latency for log streaming (REQ-018 requires within-2-second updates) | Low |

## Decision

1. **Backend:** Go (standard library net/http with a lightweight router such as chi or echo)
2. **Frontend:** React with TypeScript
3. **Real-time:** Server-Sent Events (SSE) for log streaming, task state updates, and worker status updates
4. **Data access:** PostgreSQL via pgx (native Go PostgreSQL driver); Redis via go-redis
5. **Repository structure:** Monorepo with directory-based separation -- `api/` (Go backend), `worker/` (Go worker process), `monitor/` (Go monitor service), `web/` (React frontend), `internal/` (shared Go packages: domain types, Redis client wrappers, database queries)

**Door type:** One-way -- language and framework choices pervade every file in the codebase.

**Cost to change later:** Critical -- any of these choices requires a full rewrite of the affected layer.

## Rationale

**Go** is chosen by Nexus directive. The architectural analysis confirms it is a sound choice for this system because: (a) go-redis provides full Redis Streams support (XADD, XREADGROUP, XCLAIM, XACK, consumer group management); (b) goroutines provide natural concurrency for queue consumption, SSE streaming, and heartbeat monitoring -- each connection gets its own goroutine without the complexity of an event loop; (c) single binary deployment simplifies Docker images (small Alpine-based or scratch images); (d) Go's standard library includes robust HTTP server and SSE capabilities with no framework dependency required; (e) strong static typing catches integration errors at compile time.

**The trade-off accepted:** No shared types between frontend and backend. The API contract must be enforced through OpenAPI specification or equivalent, with the React frontend generating TypeScript types from the API spec. This replaces the TypeScript-everywhere type sharing that Node.js would have provided. The cost is manageable -- API contract drift is caught by generated types and integration tests, not by shared source code.

**React + TypeScript** because the Pipeline Builder (REQ-015) requires a drag-and-drop canvas with complex interaction state. React has the strongest ecosystem for this (dnd-kit for drag-and-drop, react-flow for node-based editors). The four GUI views plus two demo panels are component-heavy -- React's component model is well-suited.

**SSE over WebSocket** because every real-time need is server-to-client: log lines streaming to GUI (REQ-018), task state transitions pushed to the feed (REQ-017, NFR-003), worker status changes pushed to the dashboard (REQ-016). SSE is simpler, reconnects automatically, and works through Traefik reverse proxy without special configuration.

**pgx** is the standard high-performance PostgreSQL driver for Go. It provides connection pooling, prepared statements, and direct access to PostgreSQL-specific features. Unlike an ORM, it requires writing SQL directly, but this gives full control over queries and schema migrations -- appropriate for a Go backend.

**go-redis** is the dominant Redis client for Go, with native support for all Redis Streams commands, Pub/Sub, and pipelining.

**Monorepo with directory layout** because the Go backend services share domain types and infrastructure clients (Redis wrappers, database queries). Go's package system handles internal code sharing without a registry. The React frontend is a separate build artifact in the same repository.

## Fitness Function
**Characteristic threshold:** Compile-time type safety across Go packages; API contract consistency between Go backend and React frontend; development velocity maintainable by a solo developer

| | Specification |
|---|---|
| **Dev check** | Go build succeeds with zero compilation errors across all packages; `go vet` and `staticcheck` pass with no findings; API contract types are generated from OpenAPI spec and used in both Go handlers and React frontend; shared Go packages are imported (not duplicated) across services. |
| **Prod metric** | API response time p95; SSE connection count; frontend bundle size; Go binary memory footprint. |
| **Warning threshold** | API p95 response time > 100ms (non-queuing endpoints); SSE connection count > 500; frontend bundle > 2MB; Go process RSS > 256MB |
| **Critical threshold** | API p95 > 500ms; SSE connections dropped without reconnect; Go build failures in CI |
| **Alarm meaning** | Warning: performance is degrading or resource usage is growing -- investigate before it becomes user-facing. Critical: the system is too slow for interactive use or the build is broken. |

## Consequences
**Easier:** Goroutine-per-connection model simplifies concurrent SSE streaming and queue consumption; single binary deployment produces minimal Docker images; go-redis provides direct Redis Streams support; Go's compile-time checks catch type errors before runtime; low memory footprint allows more headroom on a single VPS.
**Harder:** No shared types between frontend and backend -- requires OpenAPI spec as contract and generated TypeScript types; Go is more verbose than TypeScript for JSON handling and validation; no ORM -- SQL is written directly (but this gives full control over queries and migrations).
**Newly required:** OpenAPI specification for the REST API; TypeScript type generation from OpenAPI for the React frontend; pgx-based database access layer; go-redis client wrappers for stream operations; Go project layout with internal packages for shared code.
