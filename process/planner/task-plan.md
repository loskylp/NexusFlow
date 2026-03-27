# Task Plan -- NexusFlow
**Version:** 2.1 | **Date:** 2026-03-26
**Requirements Version:** 5 | **Architecture Version:** 2
**Artifact Weight:** Blueprint

## Architecture Constraints

Go backend (ADR-004, one-way door) with React/TypeScript frontend. Redis Streams with per-tag consumer groups (ADR-001, one-way for queue structure). At-least-once delivery with idempotency guards at Sink boundary (ADR-003, one-way). Docker Compose on nxlabs.cc with Traefik, Watchtower, shared PostgreSQL, service-managed Redis (ADR-005). SSE per-view channels with Redis Pub/Sub (ADR-007). golang-migrate + sqlc for database access (ADR-008). Sink-type-specific atomicity wrappers (ADR-009, one-way).

Walking skeleton target: A user can log in, submit a task via the API, have it queued in Redis, picked up by a simulated worker with a demo connector, executed through a minimal pipeline, and see the task reach a terminal state -- all demonstrated with a demo source, a simulated worker, and a demo sink, in Docker Compose locally. Admin can view the Worker Fleet Dashboard in the GUI.

---

## Cycle 1 -- MVP Walking Skeleton

*The thinnest end-to-end slice: infrastructure, auth, worker registration, task submission, tag-based assignment, pipeline execution, and Admin login + Worker Fleet Dashboard GUI. Includes one demo source, one simulated worker, and one demo sink -- all managed via API.*

### Priority 1 -- Do First

### TASK-001: DevOps Phase 1 -- CI pipeline and dev environment
**Requirement(s):** FF-015, FF-020, ADR-004, ADR-005
**Description:** Set up the monorepo structure (api/, worker/, monitor/, web/, internal/), Go module initialization, Docker Compose for local dev (api, worker, monitor, redis, postgres), Dockerfiles for Go services (multi-stage builds), and a CI pipeline (GitHub Actions) that runs go build, go vet, staticcheck, and go test on every push to main. Include .env.example and docker-compose.yml for dev environment.
**Acceptance Criteria:**
- `docker compose up` starts all core services (api, worker, monitor, redis, postgres) and they pass health checks within 30 seconds
- CI pipeline runs on push to main: go build succeeds across all Go packages, go vet passes, staticcheck passes
- Monorepo directory layout matches ADR-004: api/, worker/, monitor/, web/, internal/
- .env.example documents all required environment variables
**Depends on:** none
**Risk:** L -- proven technology (Go, Docker, GitHub Actions), familiar patterns, no architectural unknowns
**Value:** H -- on walking skeleton critical path; unblocks all subsequent Builder tasks
**Priority:** P1 (LH -- quick win)
**DevOps Phase:** 1
**Demo Script:** tests/demo/TASK-001-demo.md
**Status:** Pending

### TASK-002: Database schema and migration foundation
**Requirement(s):** REQ-009, REQ-019, REQ-020, ADR-008
**Description:** Set up golang-migrate with the migration directory structure. Create initial migrations for the core data model: users table (id, username, password_hash, role, active, created_at), tasks table (id, pipeline_id, chain_id, user_id, status, retry_config, retry_count, execution_id, worker_id, input, created_at, updated_at), task_state_log table (id, task_id, from_state, to_state, reason, timestamp), workers table (id, tags, status, last_heartbeat, registered_at), pipelines table, pipeline_chains table. Set up sqlc configuration and generate initial Go query code. Include CHECK constraint on valid task state transitions via trigger.
**Acceptance Criteria:**
- Migrations apply cleanly to a fresh PostgreSQL database
- Down migrations roll back cleanly
- sqlc compile succeeds with zero errors
- Task state transition CHECK constraint rejects invalid transitions (e.g., completed -> queued)
- Schema matches the data model in ADR-008
**Depends on:** TASK-001
**Risk:** L -- proven technology (golang-migrate, sqlc, PostgreSQL), data model defined in ADR-008
**Value:** H -- Must Have for MVP; foundational for all task and user operations
**Priority:** P1 (LH -- quick win)
**Demo Script:** tests/demo/TASK-002-demo.md
**Status:** Pending

### TASK-004: Redis Streams queue infrastructure
**Requirement(s):** REQ-003, REQ-005, NFR-001, NFR-002, ADR-001
**Description:** Implement the Redis Streams queue layer: stream routing abstraction that maps task tags to per-tag streams (queue:{tag}), XADD for enqueuing tasks, consumer group initialization on service startup, XREADGROUP for consuming tasks, XACK for acknowledgment. Configure Redis with AOF+RDB hybrid persistence. Include dead letter stream (queue:dead-letter) setup.
**Acceptance Criteria:**
- A task enqueued with tags ["etl"] is added to stream queue:etl via XADD
- Consumer groups are created automatically on service startup if they do not exist
- XREADGROUP blocking read returns tasks to the appropriate consumer
- XACK removes the task from the pending entry list
- Enqueuing 1,000 tasks sequentially completes with p95 latency under 50ms
- After Redis restart, all previously enqueued but unacknowledged tasks are still in the stream
**Depends on:** TASK-001
**Risk:** M -- uses known technology (Redis Streams) but in a specific pattern (per-tag topology with consumer groups) not previously implemented
**Value:** H -- Must Have for MVP; core queue infrastructure; on walking skeleton critical path
**Priority:** P1 (MH -- schedule with HH tasks)
**Demo Script:** tests/demo/TASK-004-demo.md
**Status:** Pending

### TASK-003: Authentication and session management
**Requirement(s):** REQ-019, ADR-006
**Description:** Implement user authentication with bcrypt password hashing (cost 12), server-side Redis sessions (session:{token} with 24h TTL), login endpoint (POST /api/auth/login), logout endpoint (POST /api/auth/logout), and auth middleware that validates session on every protected request. Support HTTP-only secure cookies for web GUI and Bearer tokens for API clients. Include role extraction from session (Admin/User). Seed an initial admin user on first startup.
**Acceptance Criteria:**
- POST /api/auth/login with valid credentials returns 200 and sets session cookie + returns Bearer token
- POST /api/auth/login with invalid credentials returns 401
- Unauthenticated request to any protected endpoint returns 401
- Expired session returns 401
- POST /api/auth/logout invalidates the session; subsequent requests with that token return 401
- Auth middleware extracts user ID and role from session for downstream handlers
- Initial admin user (admin/admin) is created on first startup if no users exist
**Depends on:** TASK-001, TASK-002
**Risk:** M -- known technology in a new combination (Go + Redis sessions + bcrypt); ADR-006 specifies the approach clearly
**Value:** H -- Must Have for MVP; all API endpoints require authentication; on walking skeleton critical path
**Priority:** P1 (MH -- schedule with HH tasks)
**Demo Script:** tests/demo/TASK-003-demo.md
**Status:** Pending

### TASK-006: Worker self-registration and heartbeat
**Requirement(s):** REQ-004, ADR-002
**Description:** Implement worker startup registration: on start, the worker registers itself in PostgreSQL (workers table) and Redis (ZADD workers:active with timestamp), advertising capability tags. Implement periodic heartbeat emission (ZADD workers:active every 5 seconds). Worker ID is generated on startup (UUID or hostname-based).
**Acceptance Criteria:**
- Worker starts and appears in the workers table with status "online" and correct capability tags
- Worker heartbeat updates workers:active sorted set in Redis every 5 seconds
- Multiple workers can register simultaneously with different tags
- Worker record includes registration timestamp and tags
**Depends on:** TASK-001, TASK-002
**Risk:** L -- straightforward registration pattern; heartbeat is a simple ZADD timer
**Value:** H -- Must Have for MVP; workers must exist before tasks can be assigned; on walking skeleton critical path
**Priority:** P1 (LH -- quick win)
**Demo Script:** tests/demo/TASK-006-demo.md
**Status:** Pending

### TASK-005: Task submission via REST API
**Requirement(s):** REQ-001, REQ-003, REQ-009
**Description:** Implement POST /api/tasks endpoint that accepts a task submission (pipeline reference, input parameters, optional retry configuration), validates the request, inserts the task into PostgreSQL (status: submitted), enqueues it in the appropriate Redis stream (status: queued), and returns 201 with the task ID. Apply safe default retry settings (max_retries: 3, backoff: exponential) when not specified. Enforce auth via middleware.
**Acceptance Criteria:**
- POST /api/tasks with valid payload returns 201 with unique task ID
- Task record exists in PostgreSQL with status "submitted" then "queued"
- Task message exists in the appropriate Redis stream (queue:{tag})
- POST /api/tasks with invalid pipeline reference returns 400 with structured error
- POST /api/tasks without retry config creates task with default retry settings
- Unauthenticated request returns 401
**Depends on:** TASK-002, TASK-003, TASK-004
**Risk:** L -- well-understood REST endpoint pattern; all dependencies are clear
**Value:** H -- Must Have for MVP; primary entry point for tasks; on walking skeleton critical path
**Priority:** P1 (LH -- quick win)
**Demo Script:** tests/demo/TASK-005-demo.md
**Status:** Pending

### TASK-013: Pipeline CRUD via REST API
**Requirement(s):** REQ-022
**Description:** Implement pipeline management endpoints: POST /api/pipelines (create), GET /api/pipelines (list), GET /api/pipelines/{id} (retrieve), PUT /api/pipelines/{id} (update), DELETE /api/pipelines/{id} (delete). Pipelines are owned by the creating user. Users manage their own pipelines; admins can manage all. Deletion is rejected if the pipeline has active (non-terminal) tasks.
**Acceptance Criteria:**
- POST /api/pipelines creates a pipeline with DataSource, Process, Sink config and schema mappings; returns 201
- GET /api/pipelines returns user's own pipelines (User role) or all pipelines (Admin)
- PUT /api/pipelines/{id} updates pipeline config; returns 200
- DELETE /api/pipelines/{id} deletes pipeline if no active tasks reference it; returns 204
- DELETE /api/pipelines/{id} returns 409 if active tasks exist
- Non-owner non-admin operations on another user's pipeline return 403
**Depends on:** TASK-002, TASK-003
**Risk:** L -- standard CRUD pattern with ownership enforcement
**Value:** H -- Must Have for MVP; pipelines must exist before tasks can reference them
**Priority:** P1 (LH -- quick win)
**Demo Script:** tests/demo/TASK-013-demo.md
**Status:** Pending

### TASK-007: Tag-based task assignment and pipeline execution
**Requirement(s):** REQ-005, REQ-006, REQ-007, REQ-009
**Description:** Implement the worker task consumption loop: XREADGROUP blocking read from streams matching the worker's capability tags. On receiving a task, update task status to "assigned" then "running" in PostgreSQL. Execute the three-phase pipeline: DataSource (data ingestion), Process (transformation with schema mapping applied), Sink (output). Apply schema mappings at phase boundaries (DataSource->Process and Process->Sink). On completion, update task to "completed" and XACK the message. On failure, update task to "failed". Emit task state transition events to Redis Pub/Sub (events:tasks:{userId}).
**Acceptance Criteria:**
- Worker with tags ["etl"] consumes tasks from queue:etl and not from queue:report
- Task with required tags ["etl"] is assigned only to a worker with matching tags
- Task with no matching worker remains in "queued" state
- Pipeline executes DataSource, then Process (with schema mapping from DataSource output), then Sink (with schema mapping from Process output)
- Schema mapping correctly renames fields between phases
- Schema mapping referencing a nonexistent source field fails the task with a clear error
- Task state transitions are recorded in task_state_log with timestamps
- Completed task has status "completed" and is XACKed
- Failed task (Process script error) has status "failed" and does not retry
**Depends on:** TASK-004, TASK-005, TASK-006, TASK-013
**Risk:** H -- core pipeline execution with schema mapping is the most complex single task; integrates queue consumption, database updates, and pipeline orchestration; touches ADR-001, ADR-003, ADR-008
**Value:** H -- Must Have for MVP; the heart of the system; on walking skeleton critical path
**Priority:** P1 (HH -- do first)
**Demo Script:** tests/demo/TASK-007-demo.md
**Status:** Pending

### TASK-042: Demo connectors -- demo source, simulated worker, demo sink
**Requirement(s):** Nexus walking skeleton directive (Cycle 1 scope)
**Description:** Implement one demo DataSource connector (e.g., in-memory JSON array or CSV generator), one simulated worker configuration with demo capability tags, and one demo Sink connector (e.g., in-memory store or stdout logger). All three are managed via the pipeline CRUD API -- no separate infrastructure required. The demo source produces deterministic sample data, the simulated worker processes it through a trivial Process phase, and the demo sink records the output. This enables end-to-end walking skeleton demonstration without MinIO or Mock-Postgres.
**Acceptance Criteria:**
- A demo DataSource connector can be configured in a pipeline definition via POST /api/pipelines
- A demo Sink connector can be configured in a pipeline definition via POST /api/pipelines
- A worker can start with demo capability tags and execute pipelines using the demo connectors
- End-to-end test: create a pipeline with demo source + trivial process + demo sink, submit a task, task completes successfully with data flowing through all three phases
- Demo connectors are lightweight (no external infrastructure dependency beyond Redis and PostgreSQL)
**Depends on:** TASK-007, TASK-013
**Risk:** L -- simple connector implementations wrapping in-memory or stdout; pipeline execution framework is established by TASK-007
**Value:** H -- required for Cycle 1 walking skeleton demo; enables end-to-end demonstration without demo infrastructure (MinIO/Mock-Postgres)
**Priority:** P1 (LH -- quick win; unblocks walking skeleton demo)
**Demo Script:** tests/demo/TASK-042-demo.md
**Status:** Pending

### Priority 2 -- Do After P1

### TASK-019: React app shell with sidebar navigation and auth flow
**Requirement(s):** REQ-019, REQ-016, UX Spec (Navigation Structure)
**Description:** Set up the React + TypeScript frontend: project initialization, routing (react-router), sidebar navigation component (240px dark slate-900), auth context with login/logout flow, protected route wrapper, role-based nav item visibility (hide demo views for User role). Implement the Login screen per UX spec. Set up SSE client utilities (EventSource wrapper with reconnection). Apply design system (DESIGN.md): color tokens, typography (Inter, IBM Plex Sans, JetBrains Mono), spacing scale.
**Acceptance Criteria:**
- Login screen renders with username/password form per UX spec
- Successful login redirects to Worker Fleet Dashboard (Admin) or Task Feed (User)
- Invalid credentials show inline error message
- Sidebar navigation visible on all authenticated views with correct items
- Demo nav items (Sink Inspector, Chaos Controller) hidden for User role
- Unauthenticated users redirected to /login
- Design system tokens (colors, typography, spacing) applied globally
**Depends on:** TASK-003
**Risk:** L -- standard React SPA setup; design system is fully specified
**Value:** M -- Should Have for this release; enables GUI-based interaction; not on the walking skeleton API critical path but needed for the Worker Fleet Dashboard GUI
**Priority:** P2 (LM -- reliable value)
**Demo Script:** tests/demo/TASK-019-demo.md
**Status:** Pending

### TASK-025: Worker fleet status API
**Requirement(s):** REQ-016
**Description:** Implement GET /api/workers endpoint returning all registered workers with their status, tags, current task assignment, and last heartbeat timestamp. All authenticated users can see all workers (Domain Invariant 5).
**Acceptance Criteria:**
- GET /api/workers returns all registered workers regardless of caller role
- Each worker includes: id, status (online/down), capability tags, current task ID (if assigned), last heartbeat
- Unauthenticated request returns 401
**Depends on:** TASK-003, TASK-006
**Risk:** L -- simple read-only endpoint
**Value:** M -- Should Have; required for Worker Fleet Dashboard GUI
**Priority:** P2 (LM -- reliable value)
**Demo Script:** tests/demo/TASK-025-demo.md
**Status:** Pending

### TASK-015: SSE event infrastructure
**Requirement(s):** REQ-016, REQ-017, REQ-018, NFR-003, ADR-007
**Description:** Implement the SSE event distribution layer: Redis Pub/Sub subscription management in the API server, SSE endpoint handler using Go's http.Flusher for streaming responses. Implement four SSE endpoints: GET /events/tasks (task state updates, role-filtered), GET /events/workers (worker status updates), GET /events/tasks/{id}/logs (log streaming for specific task), GET /events/sink/{taskId} (sink inspector events). Include Last-Event-ID support on the log streaming endpoint for reconnection replay.
**Acceptance Criteria:**
- GET /events/tasks streams task state change events to connected clients
- User role receives only their own task events; Admin receives all
- GET /events/workers streams worker status changes to all authenticated users
- GET /events/tasks/{id}/logs streams log lines in real time for the specified task
- Reconnection with Last-Event-ID replays missed log lines
- SSE events are delivered within 2 seconds of the backend state change (NFR-003)
- Access control enforced: user cannot stream logs for another user's task (403)
**Depends on:** TASK-003, TASK-004
**Risk:** H -- SSE with Redis Pub/Sub in Go requires careful goroutine management, connection lifecycle, and backpressure handling; one-way door decisions in ADR-007
**Value:** H -- Must Have for MVP; all real-time GUI views depend on SSE; satisfies NFR-003
**Priority:** P2 (HH forced to P2 -- depends on TASK-003/TASK-004 in P1; cannot schedule until queue infrastructure exists)
**Demo Script:** tests/demo/TASK-015-demo.md
**Status:** Pending

### TASK-020: Worker Fleet Dashboard (GUI)
**Requirement(s):** REQ-016, REQ-004, UX Spec (Worker Fleet Dashboard)
**Description:** Implement the Worker Fleet Dashboard view: summary cards row (Total Workers, Online, Down, Avg Load), full-width data table with sortable columns (Status, Worker ID, Hostname, Tags, Current Task, CPU%, Memory%, Last Heartbeat). SSE connection to GET /events/workers for real-time updates. Down workers sorted to top. Status bar with SSE connection indicator. Skeleton loaders during initial fetch. Empty state when no workers registered.
**Acceptance Criteria:**
- Dashboard shows all registered workers with correct status indicators (green dot = online, red = down)
- Summary cards show accurate counts (Total, Online, Down)
- Worker going down updates in real time without page refresh (within 15s heartbeat timeout)
- Worker coming online updates in real time
- Table columns are sortable by click
- Down workers sorted to top by default
- SSE disconnection shows "Reconnecting..." in status bar
- Empty state message shown when no workers registered
**Depends on:** TASK-019, TASK-025, TASK-015, TASK-006
**Risk:** L -- standard data table with SSE; UX spec fully defined
**Value:** M -- Should Have; Admin landing page; included in Cycle 1 per Nexus directive
**Priority:** P2 (LM -- reliable value)
**Demo Script:** tests/demo/TASK-020-demo.md
**Status:** Pending

### TASK-029: DevOps Phase 2 -- staging environment and CD pipeline
**Requirement(s):** ADR-005, FF-021
**Description:** Set up staging environment on nxlabs.cc (nexusflow.staging.nxlabs.cc). Configure CI to build and push Docker images to container registry on demo/vN.N tag. Configure Watchtower on staging to auto-deploy from registry. Set up Traefik labels and Uptime Kuma labels for staging. Verify staging topology matches production.
**Acceptance Criteria:**
- demo/vN.N tag triggers CI build and image push to registry
- Watchtower on staging detects new images and redeploys within 5 minutes
- staging accessible at nexusflow.staging.nxlabs.cc with TLS via Traefik
- Uptime Kuma monitors staging health endpoints
- Staging runs same Docker images that will go to production
**Depends on:** TASK-001, TASK-042
**Risk:** M -- nxlabs.cc infrastructure conventions must be followed precisely; first deployment to remote server
**Value:** M -- Should Have for this release; enables pre-production validation; Nexus directive to include in Cycle 1 so the walking skeleton is shown deployed
**Priority:** P2 (MM -- solid middle; scheduled last in Cycle 1 so there is a walking skeleton to deploy)
**DevOps Phase:** 2
**Demo Script:** tests/demo/TASK-029-demo.md
**Status:** Pending

---

## Cycle 1 Demo Sign-off Criteria

A user can log in (Admin), submit a task via the REST API using a demo pipeline (demo source -> trivial process -> demo sink), have it queued in Redis, picked up by a simulated worker, executed through the three-phase pipeline with schema mapping, and see the task reach "completed" state. The Worker Fleet Dashboard shows the demo worker online with real-time SSE updates. One demo source, one simulated worker, one demo sink -- all managed via the pipeline CRUD API. The walking skeleton is deployed to staging at nexusflow.staging.nxlabs.cc and accessible via TLS.

---

## Cycle 2 -- Core System Completion

*Complete the core system: monitor/failover, retry, DLQ, cancellation, pipeline CRUD, pipeline chaining, SSE log production, admin user management, sink atomicity. Builds on the walking skeleton from Cycle 1.*

### Priority 1 -- Do First

### TASK-009: Monitor service -- heartbeat checking and failover
**Requirement(s):** REQ-004, REQ-013, REQ-011, ADR-002
**Description:** Implement the Monitor service: periodic heartbeat check (ZRANGEBYSCORE workers:active for expired entries, 15s timeout), worker status update to "down" in PostgreSQL and Redis Pub/Sub. Pending entry scanner (XPENDING + XCLAIM every 10s) to reclaim tasks from downed workers. Increment retry counter on failover. If retries exhausted, route task to dead letter stream and update status to "failed".
**Acceptance Criteria:**
- Worker that stops heartbeating for >15 seconds is marked "down" in PostgreSQL
- Worker down event published to events:workers via Redis Pub/Sub
- Tasks pending on a downed worker are reclaimed via XCLAIM and re-queued
- Task retry counter is incremented on each failover reassignment
- Reclaimed task is picked up by a healthy matching worker
- Task with exhausted retries (default 3) is moved to queue:dead-letter and status set to "failed"
**Depends on:** TASK-004, TASK-006, TASK-007
**Risk:** H -- integrates heartbeat detection, XCLAIM reclamation, retry counting, and dead-letter routing; failure here means orphaned tasks
**Value:** H -- Must Have for MVP; system cannot self-heal without the Monitor; critical for NFR-004
**Priority:** P1 (HH -- do first)
**Demo Script:** tests/demo/TASK-009-demo.md
**Status:** Pending

### TASK-010: Infrastructure retry with backoff
**Requirement(s):** REQ-011
**Description:** Implement per-task retry configuration (max_retries, backoff strategy). When the Monitor detects an infrastructure failure (worker down), the task is re-queued with backoff delay applied. Process/script errors do not trigger retry -- only infrastructure failures. Include retry count tracking and exhaustion detection.
**Acceptance Criteria:**
- Task with {max_retries: 3, backoff: "exponential"} is retried up to 3 times on infrastructure failure
- Backoff delay is applied between retries (exponential: 1s, 2s, 4s)
- Task failing due to Process script error is NOT retried and transitions to "failed" immediately
- Task that exhausts retries transitions to "failed" and is placed in dead letter queue
- Retry count is visible in task state
**Depends on:** TASK-009
**Risk:** M -- retry logic is well-understood but must correctly distinguish infrastructure vs. script errors
**Value:** H -- Must Have for MVP; production resilience depends on correct retry behavior
**Priority:** P1 (MH -- schedule with HH tasks)
**Demo Script:** tests/demo/TASK-010-demo.md
**Status:** Pending

### TASK-011: Dead letter queue with cascading cancellation
**Requirement(s):** REQ-012, REQ-014
**Description:** Implement dead letter queue handling: tasks that exhaust retries or fail unrecoverably are added to queue:dead-letter. If the failed task is part of a pipeline chain, all downstream tasks in the chain are cancelled with reason "upstream task failed". Include GET /api/tasks?status=failed endpoint for dead letter inspection.
**Acceptance Criteria:**
- Task exhausting retries appears in queue:dead-letter stream
- Pipeline chain A -> B -> C: when task A enters dead letter queue, tasks B and C are cancelled with reason "upstream task failed"
- Standalone task (not in a chain) enters dead letter queue without cascading cancellation
- Dead letter tasks are visible via the task API with status "failed"
**Depends on:** TASK-009, TASK-010, TASK-014
**Risk:** M -- chain cancellation requires traversing pipeline chain definitions; logic is clear but integration is non-trivial
**Value:** H -- Must Have for MVP; prevents silent data loss and ensures chain integrity
**Priority:** P1 (MH -- schedule with HH tasks)
**Demo Script:** tests/demo/TASK-011-demo.md
**Status:** Pending

### TASK-012: Task cancellation
**Requirement(s):** REQ-010
**Description:** Implement POST /api/tasks/{id}/cancel endpoint. Only the task owner or an admin can cancel. Cancellable states: submitted, queued, assigned, running. Cancellation of a running task signals the worker to stop execution (via a cancellation flag in Redis). Terminal states (completed, failed, cancelled) reject cancellation requests.
**Acceptance Criteria:**
- Task owner can cancel their own task in any cancellable state; task transitions to "cancelled"
- Admin can cancel any user's task
- Non-owner non-admin receives 403
- Cancellation of a running task causes the worker to halt execution
- Cancellation of a completed task returns 409 (conflict -- terminal state)
- Cancelled task cannot be transitioned to any other state
**Depends on:** TASK-005, TASK-007
**Risk:** M -- worker cancellation signaling (via Redis flag checked during execution) requires coordination between API and Worker
**Value:** H -- Must Have for MVP; operational necessity for task management
**Priority:** P1 (MH -- schedule with HH tasks)
**Demo Script:** tests/demo/TASK-012-demo.md
**Status:** Pending

### TASK-014: Pipeline chain definition
**Requirement(s):** REQ-014
**Description:** Implement pipeline chain creation and management: POST /api/chains (create linear chain of pipeline IDs), GET /api/chains, GET /api/chains/{id}. Chains are strictly linear (A -> B -> C). On task completion for pipeline A in a chain, automatically submit a task for pipeline B. Branching chains are rejected.
**Acceptance Criteria:**
- POST /api/chains with ordered pipeline IDs creates a linear chain; returns 201
- POST /api/chains with a branching structure (A -> B and A -> C simultaneously) returns 400
- When a task for pipeline A in chain completes, a task for pipeline B is automatically submitted
- Chain trigger is idempotent (ADR-003): duplicate completion events do not create duplicate downstream tasks
- GET /api/chains/{id} returns the chain definition with pipeline ordering
**Depends on:** TASK-013, TASK-007
**Risk:** M -- chain trigger idempotency (SET-NX guard per ADR-003) requires careful implementation
**Value:** H -- Must Have for MVP; pipeline chaining is a core workflow
**Priority:** P1 (MH -- schedule with HH tasks)
**Demo Script:** tests/demo/TASK-014-demo.md
**Status:** Pending

### TASK-018: Sink atomicity with idempotency
**Requirement(s):** REQ-008, ADR-003, ADR-009
**Description:** Implement sink-type-specific transaction wrappers: database sinks use BEGIN/COMMIT/ROLLBACK, S3-compatible sinks use multipart upload with abort, file sinks use temp file with rename. Add execution ID (task ID + attempt number) deduplication check at the Sink boundary -- if execution ID already applied, skip write and return success. This enforces at-least-once delivery with idempotent Sink writes.
**Acceptance Criteria:**
- Database Sink: forced failure mid-write rolls back all partial records; destination has zero records from this execution
- S3 Sink: forced failure mid-write aborts multipart upload; no partial objects at destination
- Successful Sink write commits all records atomically
- Duplicate execution (same task ID + attempt number) is detected and skipped (no duplicate writes)
- Execution ID is recorded at the destination for deduplication
**Depends on:** TASK-007
**Risk:** H -- one-way door (ADR-009); sink atomicity is a core invariant; each sink type needs its own wrapper; idempotency guard is critical for correctness
**Value:** H -- Must Have for MVP; Domain Invariant 3 (sink atomicity) is non-negotiable
**Priority:** P1 (HH -- do first)
**Demo Script:** tests/demo/TASK-018-demo.md
**Status:** Pending

### TASK-016: Log production and dual storage
**Requirement(s):** REQ-018, ADR-008
**Description:** Implement worker log production: during pipeline execution, workers publish log lines to Redis Streams (logs:{taskId}) via XADD and to Redis Pub/Sub (events:logs:{taskId}) for real-time streaming. Include phase tagging on each log line (datasource/process/sink). Implement background goroutine in the API server that copies log lines from Redis Streams to PostgreSQL task_logs table periodically (every 60 seconds). Include GET /api/tasks/{id}/logs REST endpoint for historical log retrieval.
**Acceptance Criteria:**
- During task execution, log lines appear in Redis Stream logs:{taskId} with phase tags
- Log lines are published to events:logs:{taskId} for SSE consumption
- Background sync copies logs from Redis to PostgreSQL task_logs table
- GET /api/tasks/{id}/logs returns historical log lines from PostgreSQL
- Log lines include timestamp, level (INFO/WARN/ERROR), phase, and message
- Access control: user can only retrieve logs for their own tasks; admin for all
**Depends on:** TASK-007, TASK-015
**Risk:** M -- dual storage (Redis hot + PostgreSQL cold) with background sync requires careful ordering and reliability
**Value:** H -- Must Have for MVP; log streaming is a core feature
**Priority:** P1 (MH -- schedule with HH tasks)
**Demo Script:** tests/demo/TASK-016-demo.md
**Status:** Pending

### TASK-017: Admin user management
**Requirement(s):** REQ-020
**Description:** Implement admin user management endpoints: POST /api/users (create user with username, initial password, role), GET /api/users (list all users), PUT /api/users/{id}/deactivate (deactivate account). Deactivation immediately invalidates all of the user's active sessions (delete session:* keys for that user). Deactivation does NOT cancel the user's in-flight tasks.
**Acceptance Criteria:**
- POST /api/users (admin only) creates a user with hashed password and assigned role; returns 201
- GET /api/users (admin only) lists all user accounts with status
- PUT /api/users/{id}/deactivate (admin only) deactivates the user
- After deactivation, the user's existing sessions are immediately invalidated (returns 401)
- After deactivation, the user cannot log in
- Deactivated user's previously submitted tasks continue executing (not cancelled)
- Non-admin accessing these endpoints receives 403
**Depends on:** TASK-003
**Risk:** L -- standard admin CRUD with session invalidation; session cleanup is a Redis DEL operation
**Value:** H -- Must Have for MVP; admin must be able to manage users
**Priority:** P1 (LH -- quick win)
**Demo Script:** tests/demo/TASK-017-demo.md
**Status:** Pending

### TASK-008: Task lifecycle state tracking and query API
**Requirement(s):** REQ-009, REQ-017
**Description:** Implement GET /api/tasks (list tasks with filtering by status, pipeline, user), GET /api/tasks/{id} (single task with state history). Enforce visibility isolation: regular users see only their own tasks, admins see all. Return task state, timestamps, worker assignment, retry count, and state transition history.
**Acceptance Criteria:**
- GET /api/tasks returns the authenticated user's tasks (for User role) or all tasks (for Admin)
- GET /api/tasks?status=running filters to running tasks only
- GET /api/tasks/{id} returns full task detail including state transition history from task_state_log
- User-A cannot see User-B's tasks (returns empty list, not 403)
- Admin can see all users' tasks
- Unauthenticated request returns 401
**Depends on:** TASK-002, TASK-003, TASK-005
**Risk:** L -- standard CRUD query endpoint with role-based filtering
**Value:** H -- Must Have for MVP; required for Task Feed GUI and API consumers
**Priority:** P1 (LH -- quick win)
**Demo Script:** tests/demo/TASK-008-demo.md
**Status:** Pending

### Priority 2 -- Do After P1

### TASK-026: Schema mapping validation at design time
**Requirement(s):** REQ-007, ADR-008
**Description:** Implement design-time schema mapping validation in the pipeline save handler: when a pipeline definition is saved, validate schema mappings against the declared output schema of each preceding phase. Reject invalid mappings (missing source fields, type mismatches) with clear error messages. This supplements runtime validation (which remains in TASK-007).
**Acceptance Criteria:**
- Saving a pipeline with a schema mapping referencing a nonexistent source field returns 400 with clear error
- Saving a pipeline with valid schema mappings succeeds
- Validation checks all mappings: DataSource->Process and Process->Sink
- Error messages identify the specific field and mapping that failed
**Depends on:** TASK-013
**Risk:** L -- validation logic is well-defined in ADR-008; design-time validation is an additional check on save
**Value:** M -- Should Have; addresses User persona frustration about runtime-only validation errors
**Priority:** P2 (LM -- reliable value)
**Demo Script:** tests/demo/TASK-026-demo.md
**Status:** Pending

---

## Cycle 2 Demo Sign-off Criteria

Full backend core system works end-to-end: admin can monitor heartbeats and observe failover when a worker goes down, tasks are retried on infrastructure failure with exponential backoff, exhausted tasks land in the dead letter queue with cascading chain cancellation, tasks can be cancelled, pipelines can be chained (A -> B triggers automatically), logs stream in real time via SSE, sink operations are atomic with idempotency, and admin can manage users. All Must Have backend requirements satisfied.

---

## Cycle 3 -- GUI Completion and Infrastructure

*Remaining GUI views, remaining APIs, log retention, health/OpenAPI. Completes v1.0.0 scope.*

### Priority 1 -- Do First

### TASK-021: Task Feed and Monitor (GUI)
**Requirement(s):** REQ-017, REQ-002, REQ-009, REQ-010, UX Spec (Task Feed and Monitor)
**Description:** Implement the Task Feed view: vertical card feed of tasks with status badges (using task state color map), filter bar (status/pipeline/search), "Submit Task" button opening a modal with pipeline selector and parameter form, "View Logs" action button per task, "Cancel" button (visible only for cancellable states on own tasks or admin), retry button for failed tasks. SSE connection to GET /events/tasks for real-time state updates. Role-based visibility: Admin sees all tasks with owner display, User sees own tasks only.
**Acceptance Criteria:**
- Task Feed shows tasks in reverse chronological order with correct status badges
- Task state changes update in real time via SSE (badge transition with 200ms highlight)
- "Submit Task" modal allows pipeline selection, parameter input, and retry config; submission creates a task via API
- "Cancel" button visible only on cancellable states for task owner or admin
- "View Logs" navigates to Log Streamer with task pre-selected
- Admin sees all tasks with "Viewing: All Tasks" badge; User sees own tasks with "Viewing: My Tasks"
- Filter by status, pipeline, and search (task ID or pipeline name) works correctly
- Empty state and loading skeleton shown appropriately
**Depends on:** TASK-019, TASK-005, TASK-008, TASK-012, TASK-013, TASK-015
**Risk:** M -- integrates task submission, cancellation, SSE updates, and role-based filtering in one view
**Value:** H -- Must Have for MVP; primary user interaction surface for task management
**Priority:** P1 (MH -- schedule first)
**Demo Script:** tests/demo/TASK-021-demo.md
**Status:** Pending

### TASK-022: Log Streamer (GUI)
**Requirement(s):** REQ-018, UX Spec (Log Streamer)
**Description:** Implement the Log Streamer view: task selector dropdown, phase filter toggles (All/DataSource/Process/Sink), auto-scroll toggle, dark terminal-style log panel (monospace text, dark background). SSE connection to GET /events/tasks/{id}/logs for real-time streaming. Phase-colored tags ([datasource] blue, [process] purple, [sink] green). Download Logs button (fetches from REST API). Clear button (clears visual buffer). Status bar with Last-Event-ID and line count. Support Last-Event-ID reconnection.
**Acceptance Criteria:**
- Selecting a task initiates SSE connection and streams log lines in real time
- Phase filter toggles show/hide log lines by pipeline phase (client-side)
- Phase tags are color-coded per design system
- Auto-scroll follows new lines; toggling off allows scroll-back
- Download Logs fetches full log history from REST API and triggers browser download
- SSE disconnection reconnects with Last-Event-ID; missed lines are replayed
- Access denied (403) for non-owner non-admin shows error in log panel
- Log lines include timestamp, level, phase tag, and message text
**Depends on:** TASK-019, TASK-015, TASK-016
**Risk:** M -- SSE reconnection with Last-Event-ID replay requires careful state management
**Value:** M -- Should Have; essential for task debugging and operational visibility
**Priority:** P1 (MM -- solid middle)
**Demo Script:** tests/demo/TASK-022-demo.md
**Status:** Pending

### TASK-023: Pipeline Builder (GUI)
**Requirement(s):** REQ-015, REQ-007, UX Spec (Pipeline Builder)
**Description:** Implement the Pipeline Builder view: component palette (draggable DataSource/Process/Sink cards), dot-grid canvas, pipeline node rendering with phase-colored headers, connector lines between nodes, schema mapping chips between phases. Schema mapping editor (modal or slide-out panel) for field-to-field mapping definition. Design-time schema mapping validation (ADR-008): validate mappings against declared phase output schemas on save. Save/Run/Clear toolbar actions. Saved pipelines list in palette. Unsaved changes warning on navigation.
**Acceptance Criteria:**
- User can drag DataSource, Process, and Sink components onto canvas
- Canvas enforces linearity: exactly one DataSource, one Process, one Sink in sequence
- Attempting to add a second DataSource is rejected with tooltip explanation
- Schema mapping editor opens on clicking the mapping chip; allows field-to-field mapping
- Save validates all schema mappings at design time; invalid mappings show red border and tooltip with error
- Saved pipeline is available via GET /api/pipelines
- Run button opens task submission form pre-populated with this pipeline
- Browser navigation with unsaved changes triggers confirmation dialog
- Saved pipelines list loads from API; clicking a pipeline loads it onto canvas
**Depends on:** TASK-019, TASK-013, TASK-026
**Risk:** H -- drag-and-drop canvas with schema mapping editor is the most complex frontend component; requires dnd-kit or react-flow integration
**Value:** H -- Must Have for MVP; primary pipeline creation interface
**Priority:** P1 (HH -- do first)
**Demo Script:** tests/demo/TASK-023-demo.md
**Status:** Pending

### TASK-035: Task submission via GUI (complete flow)
**Requirement(s):** REQ-002
**Description:** Ensure the full GUI task submission flow works end-to-end: user selects a pipeline in the Task Feed submission modal, fills in required parameters, optionally configures retry settings, and submits. The resulting task is identical to one submitted via the API. Inline validation prevents submission with missing required parameters.
**Acceptance Criteria:**
- User can submit a task via the Task Feed "Submit Task" modal
- Pipeline selector shows available pipelines from GET /api/pipelines
- Missing required parameters show inline validation errors
- Submitted task appears in the Task Feed with status "submitted"
- Task created via GUI is identical in state and behavior to one created via API
**Depends on:** TASK-021, TASK-013
**Risk:** L -- TASK-021 builds most of this; this task ensures the complete end-to-end flow works
**Value:** H -- Must Have for MVP; second entry path for tasks alongside API
**Priority:** P1 (LH -- quick win)
**Demo Script:** tests/demo/TASK-035-demo.md
**Status:** Pending

### TASK-024: Pipeline management GUI
**Requirement(s):** REQ-023
**Description:** Implement pipeline list/edit/delete functionality in the GUI. Users see their own pipelines; admins see all. Edit navigates to Pipeline Builder with the pipeline loaded. Delete shows confirmation dialog and calls DELETE /api/pipelines/{id}. Active task protection: deletion blocked with explanation if pipeline has active tasks.
**Acceptance Criteria:**
- Pipeline list shows user's own pipelines (User) or all pipelines (Admin)
- Edit action loads the pipeline in the Pipeline Builder
- Delete action shows confirmation dialog; on confirm, deletes via API
- Delete blocked with explanation when pipeline has active tasks
**Depends on:** TASK-023, TASK-013
**Risk:** L -- standard list/edit/delete UI pattern
**Value:** M -- Should Have; pipeline lifecycle management in GUI
**Priority:** P1 (LM but elevated to P1 -- dependency of complete GUI)
**Demo Script:** tests/demo/TASK-024-demo.md
**Status:** Pending

### Priority 2 -- Do After P1

### TASK-028: Log retention and partition pruning
**Requirement(s):** ADR-008, FF-018
**Description:** Implement weekly partitioning on the task_logs PostgreSQL table. Implement a background job (cron or goroutine) that drops partitions older than 30 days. Implement Redis Streams XTRIM or MAXLEN cap on logs:{taskId} streams for 72-hour hot retention.
**Acceptance Criteria:**
- task_logs table is partitioned by week
- Partitions older than 30 days are dropped automatically
- Redis log streams are trimmed to enforce 72-hour retention
- Log insertion continues correctly across partition boundaries
- Pruning job runs without blocking normal operations
**Depends on:** TASK-002, TASK-016
**Risk:** L -- PostgreSQL partitioning and Redis XTRIM are well-documented patterns
**Value:** M -- Should Have; prevents unbounded storage growth; fitness function FF-018
**Priority:** P2 (LM -- reliable value)
**Demo Script:** N/A -- pure infrastructure; no user-visible behavior; demonstrated via partition count and pruning log
**Status:** Pending

### TASK-027: Health endpoint and OpenAPI specification
**Requirement(s):** ADR-005, ADR-004, FF-011, FF-020
**Description:** Implement GET /api/health endpoint that checks Redis and PostgreSQL connectivity. Generate OpenAPI specification for all REST API endpoints. Generate TypeScript types from OpenAPI spec for the React frontend (using a code generator such as openapi-typescript).
**Acceptance Criteria:**
- GET /api/health returns 200 when Redis and PostgreSQL are reachable
- GET /api/health returns 503 with details when either dependency is unreachable
- OpenAPI spec covers all implemented endpoints with request/response schemas
- TypeScript types generated from OpenAPI spec compile without errors in the frontend
**Depends on:** TASK-001, TASK-003
**Risk:** L -- standard health check pattern; OpenAPI generation is well-tooled
**Value:** M -- Should Have; required for Uptime Kuma monitoring and frontend type safety
**Priority:** P2 (LM -- reliable value)
**Demo Script:** tests/demo/TASK-027-demo.md
**Status:** Pending

---

## Cycle 3 Demo Sign-off Criteria

All GUI views functional: Pipeline Builder with drag-and-drop canvas and schema mapping editor, Task Feed with real-time SSE updates and task submission/cancellation, Log Streamer with phase filtering and reconnection, Pipeline management with list/edit/delete. Health endpoint responds correctly. OpenAPI spec generated. Log retention pruning operational. All v1.0.0 requirements satisfied. System ready for demo infrastructure and production deployment.

---

## Cycle 4 -- Demo Infrastructure

*Demo connectors (MinIO, Mock-Postgres), Sink Inspector, Chaos Controller, fitness function instrumentation. Builds presentation-ready demo capability.*

### Priority 1 -- Do First

### TASK-030: Demo infrastructure -- MinIO Fake-S3
**Requirement(s):** DEMO-001
**Description:** Configure MinIO container in Docker Compose (demo profile). Pre-seed with sample buckets and objects. Implement an S3-compatible DataSource that reads from MinIO and an S3-compatible Sink that writes to MinIO. Wire into the pipeline execution flow.
**Acceptance Criteria:**
- MinIO starts via `docker compose --profile demo up`
- S3 DataSource can read objects from MinIO buckets
- S3 Sink can write objects to MinIO buckets
- A demo pipeline can be defined using MinIO as DataSource and Sink
**Depends on:** TASK-007, TASK-018
**Risk:** M -- S3-compatible sink atomicity (multipart upload/abort) is a new pattern
**Value:** M -- Should Have; enables realistic demo scenarios with S3 storage
**Priority:** P1 (MM -- solid middle)
**Demo Script:** tests/demo/TASK-030-demo.md
**Status:** Pending

### TASK-031: Demo infrastructure -- Mock-Postgres with seed data
**Requirement(s):** DEMO-002
**Description:** Configure demo-postgres container in Docker Compose (demo profile). Pre-seed with 10K rows of sample data. Implement a PostgreSQL-compatible DataSource and Sink that work against demo-postgres. Wire into the pipeline execution flow.
**Acceptance Criteria:**
- demo-postgres starts via `docker compose --profile demo up` with 10K pre-seeded rows
- PostgreSQL DataSource can query data from demo-postgres
- PostgreSQL Sink can write data to demo-postgres
- A demo pipeline can use demo-postgres as both DataSource and Sink
**Depends on:** TASK-007, TASK-018
**Risk:** M -- database sink atomicity (transaction wrapper) needs to work against a separate PostgreSQL instance
**Value:** M -- Should Have; enables realistic demo scenarios with database storage
**Priority:** P1 (MM -- solid middle)
**Demo Script:** tests/demo/TASK-031-demo.md
**Status:** Pending

### TASK-033: Sink Before/After snapshot capture
**Requirement(s):** DEMO-003, ADR-009
**Description:** Implement pre-execution snapshot in the Worker: before Sink phase begins, query the destination to capture current state (scoped to Sink output). Store as JSON in the task execution record. After Sink completion (or rollback), capture After state. Publish both snapshots to events:sink:{taskId} via Redis Pub/Sub.
**Acceptance Criteria:**
- Before snapshot is captured and stored as JSON before Sink writes begin
- After snapshot is captured after Sink completion or rollback
- Snapshots are published to events:sink:{taskId} for SSE consumption
- For database sinks: snapshot queries the target table within the Sink's output scope
- For S3 sinks: snapshot lists objects in the target prefix
- On rollback, After snapshot matches Before snapshot
**Depends on:** TASK-018, TASK-015
**Risk:** M -- snapshot scope must be correctly defined per sink type; adds latency to Sink execution
**Value:** M -- Should Have; enables Sink Inspector demo feature
**Priority:** P1 (MM -- solid middle)
**Demo Script:** N/A -- backend capability; user-visible behavior demonstrated via TASK-032 (Sink Inspector GUI)
**Status:** Pending

### TASK-032: Sink Inspector (GUI)
**Requirement(s):** DEMO-003, ADR-009, UX Spec (Sink Inspector)
**Description:** Implement the Sink Inspector view: task selector dropdown, side-by-side Before/After panels, SSE connection to GET /events/sink/{taskId} for real-time snapshot delivery. Before panel populated on sink:before-snapshot event, After panel populated on sink:after-result event. Highlight new/changed items in green-50. Show atomicity verification status (checkmark for success, rollback confirmation for failure). Admin-only access.
**Acceptance Criteria:**
- Selecting a task subscribes to SSE channel for sink events
- Before snapshot displayed when sink phase begins
- After result displayed when sink phase completes (or rolls back)
- Successful completion: delta summary shows new/changed items highlighted
- Rollback: After panel matches Before panel; "ROLLED BACK" badge shown
- Admin-only: User role cannot access this view
**Depends on:** TASK-019, TASK-015, TASK-033, TASK-030 or TASK-031
**Risk:** M -- integrates SSE sink events with Before/After rendering; requires working demo infrastructure
**Value:** M -- Should Have; demonstrates sink atomicity for portfolio presentations
**Priority:** P1 (MM -- solid middle)
**Demo Script:** tests/demo/TASK-032-demo.md
**Status:** Pending

### TASK-034: Chaos Controller (GUI)
**Requirement(s):** DEMO-004, UX Spec (Chaos Controller)
**Description:** Implement the Chaos Controller view: three action cards (Kill Worker, Disconnect Database, Flood Queue). Kill Worker: worker selector dropdown, kill button with confirmation dialog, activity log. Disconnect Database: duration selector (15s/30s/60s), disconnect button with confirmation. Flood Queue: task count input, pipeline selector, submit burst button. System status indicator. Admin-only access. Backend endpoints for each chaos action.
**Acceptance Criteria:**
- Kill Worker: selecting a worker and clicking Kill (after confirmation) stops that worker container; activity log shows timeline
- Disconnect Database: clicking Disconnect (after confirmation) simulates DB unavailability for selected duration
- Flood Queue: submitting a burst creates the specified number of tasks rapidly
- System status indicator reflects current system health (nominal/degraded)
- Admin-only: User role cannot access this view
- All destructive actions require confirmation dialog
**Depends on:** TASK-019, TASK-020, TASK-021, TASK-009
**Risk:** H -- requires container management from the API (docker exec or equivalent); database disconnection simulation is novel
**Value:** M -- Should Have; key differentiator for portfolio demos showing auto-recovery
**Priority:** P1 (HM -- spike first, then schedule)
**Demo Script:** tests/demo/TASK-034-demo.md
**Status:** Pending

### Priority 2 -- Do After P1

### TASK-038: Fitness function instrumentation
**Requirement(s):** FF-001 through FF-025
**Description:** Implement automated fitness function checks as CI test targets or monitoring scripts. Include: Redis persistence test (FF-001), queuing latency benchmark (FF-002), queue backlog monitoring (FF-003), delivery guarantee test (FF-004, FF-005), sink atomicity test (FF-006), failover detection test (FF-007, FF-008), fleet resilience test (FF-009), auth enforcement test (FF-013), schema migration test (FF-017), schema validation test (FF-019), service startup test (FF-020), Redis persistence on container restart (FF-024).
**Acceptance Criteria:**
- Each fitness function has an automated test or monitoring check
- Tests are runnable in CI (dev checks) and reportable
- Tests cover the critical thresholds defined in the fitness functions index
- CI pipeline includes fitness function tests in the test suite
**Depends on:** TASK-001, TASK-004, TASK-007, TASK-009, TASK-018
**Risk:** L -- fitness function tests are integration tests against well-defined thresholds
**Value:** M -- Should Have; ensures ongoing architectural compliance; required before Go-Live
**Priority:** P2 (LM -- reliable value)
**Demo Script:** N/A -- CI infrastructure; no user-visible behavior
**Status:** Pending

---

## Cycle 4 Demo Sign-off Criteria

Full demo scenario works: MinIO Fake-S3 and Mock-Postgres configured as demo data sources and sinks. Sink Inspector shows Before/After comparison with atomicity verification. Chaos Controller can kill a worker, disconnect database, and flood queue -- audience observes auto-recovery via Worker Fleet Dashboard and Task Feed. Fitness function tests pass in CI. System is demo-presentation ready.

---

## Cycle 5 -- Production Deployment

*Production deployment, throughput validation. Staging environment already established in Cycle 1.*

### Priority 1 -- Do First

### TASK-037: Throughput load test
**Requirement(s):** REQ-021, NFR-001, FF-010
**Description:** Create a load test that submits 10,000 tasks within a one-hour window and verifies all reach a terminal state. Measure p95 queuing latency. Run against staging with a sufficient worker fleet. Include test report with throughput metrics.
**Acceptance Criteria:**
- 10,000 tasks submitted within one hour
- All 10,000 tasks reach a terminal state (completed or failed) within that hour
- No tasks lost from the queue
- p95 queuing latency remains below 50ms under load
- Test report documents throughput, latency percentiles, and any failures
**Depends on:** TASK-005, TASK-007, TASK-029
**Risk:** M -- load testing requires sufficient infrastructure and correct test design
**Value:** H -- Must Have for MVP; validates the SLA target
**Priority:** P1 (MH -- schedule with high-value tasks)
**Demo Script:** N/A -- test infrastructure; results documented in test report
**Status:** Pending

### TASK-036: DevOps Phase 3 -- production environment and monitoring
**Requirement(s):** ADR-005, FF-020, FF-021, FF-025
**Description:** Set up production environment on nxlabs.cc (nexusflow.nxlabs.cc). Provision PostgreSQL database via nxlabs.cc provisioning script. Configure production docker-compose.yml with Traefik labels, Watchtower labels, Uptime Kuma labels. Set up release/vN.N tag workflow: retag staging images as latest, Watchtower auto-deploys to production. Verify image SHA consistency between staging and production.
**Acceptance Criteria:**
- PostgreSQL database provisioned via nxlabs.cc provisioning script
- Production accessible at nexusflow.nxlabs.cc with TLS
- release/vN.N tag triggers image retag from staging to production
- Watchtower deploys new production images within 5 minutes
- Uptime Kuma monitors nexusflow.nxlabs.cc/api/health and nexusflow.nxlabs.cc
- Image SHAs match between staging and production after release
**Depends on:** TASK-029
**Risk:** M -- first production deployment; infrastructure conventions must be exact
**Value:** M -- required for Go-Live
**Priority:** P1 (MM -- solid middle)
**DevOps Phase:** 3
**Demo Script:** tests/demo/TASK-036-demo.md
**Status:** Pending

---

## Cycle 5 Demo Sign-off Criteria

Production environment live at nexusflow.nxlabs.cc. Throughput load test passes (10K tasks/hour) on staging. Image SHAs match between staging and production. Uptime Kuma monitoring active on production. System ready for Go-Live. (Staging environment established in Cycle 1.)

---

## Deferred -- Below Cut Line

*Low risk + low value. Nexus decides: defer or cut.*

| Task | What is lost if cut | Cost to include |
|---|---|---|
| TASK-040: Rate limiting on API | No protection against API abuse. Single-org deployment with authenticated users mitigates risk. | Small -- middleware addition; but no current requirement drives it |
| TASK-041: Priority queuing | All tasks processed FIFO within tag streams. No priority differentiation. | Medium -- requires multiple streams per tag or custom consumption logic |

**Removed from plan (Nexus directive):** TASK-039 (Pipeline template sharing / AUDIT-006) -- AUDIT-006 was DISMISSED, not deferred. Template sharing is not in scope for any release.

---

## Open Technical Questions

None. All architectural unknowns have been resolved by the Architect in v2. No spikes are needed.

---

## Cycle Boundaries

### Cycle 1 -- MVP Walking Skeleton
**Tasks:** TASK-001, TASK-002, TASK-003, TASK-004, TASK-005, TASK-006, TASK-007, TASK-013, TASK-042, TASK-019, TASK-025, TASK-015, TASK-020, TASK-029
**Task count:** 14
**Demo Sign-off criteria:** Admin can log in, create a demo pipeline (demo source + trivial process + demo sink) via API, submit a task, have it queued, assigned to a simulated worker, executed through all three phases, and see it reach "completed" state. Worker Fleet Dashboard shows the demo worker online with real-time SSE updates. Walking skeleton deployed to staging at nexusflow.staging.nxlabs.cc.

### Cycle 2 -- Core System Completion
**Tasks:** TASK-009, TASK-010, TASK-011, TASK-012, TASK-014, TASK-018, TASK-016, TASK-017, TASK-008, TASK-026
**Task count:** 10
**Demo Sign-off criteria:** Monitor detects downed workers and reassigns tasks. Retry with backoff works. Dead letter queue with cascading cancellation works. Task cancellation works. Pipeline chaining triggers downstream tasks. Sink atomicity verified. Log streaming via SSE works. Admin can manage users. Design-time schema validation works.

### Cycle 3 -- GUI Completion and Infrastructure
**Tasks:** TASK-021, TASK-022, TASK-023, TASK-035, TASK-024, TASK-028, TASK-027
**Task count:** 7
**Demo Sign-off criteria:** All GUI views functional (Task Feed, Log Streamer, Pipeline Builder, Pipeline Management). Task submission via GUI. Health endpoint. OpenAPI spec. Log retention. All v1.0.0 requirements satisfied.

### Cycle 4 -- Demo Infrastructure
**Tasks:** TASK-030, TASK-031, TASK-033, TASK-032, TASK-034, TASK-038
**Task count:** 6
**Demo Sign-off criteria:** Full demo scenario with MinIO, Mock-Postgres, Sink Inspector, Chaos Controller. Fitness functions instrumented.

### Cycle 5 -- Production Deployment
**Tasks:** TASK-037, TASK-036
**Task count:** 2
**Demo Sign-off criteria:** Production live. Throughput validated on staging. Go-Live ready. (Staging established in Cycle 1.)

---

## Revision Delta (Plan v1 -> v2)

### Summary of Changes

**Nexus directives applied:**
1. TASK-039 removed entirely -- AUDIT-006 was DISMISSED, not deferred
2. Cycles separated from production versions -- cycles are internal iterations, versions are release milestones, not 1:1
3. Original Cycle 1 (28 tasks) broken into three cycles: Cycle 1 (13 tasks -- walking skeleton), Cycle 2 (10 tasks -- core system), Cycle 3 (7 tasks -- GUI + infra). All three compose v1.0.0
4. Original Cycle 2 (10 tasks) split into Cycle 4 (6 tasks -- demo infra) and Cycle 5 (3 tasks -- production deployment)

### New Tasks
| Task | Description | Rationale |
|---|---|---|
| TASK-042 | Demo connectors (demo source, simulated worker, demo sink) | Nexus directive: Cycle 1 must include demo connectors for walking skeleton demonstration without requiring MinIO/Mock-Postgres infrastructure |

### Removed Tasks
| Task | Reason |
|---|---|
| TASK-039: Pipeline template sharing | AUDIT-006 was DISMISSED by Nexus, not deferred. Removed from all releases and backlog. |

### Restructured (no content change, only cycle assignment)
All existing tasks retain their v1 acceptance criteria unchanged. Changes are limited to:
- Cycle assignment (from 2 cycles to 5 cycles)
- TASK-007 gains dependency on TASK-013 (pipeline definitions must exist before execution)
- TASK-011 gains dependency on TASK-014 (cascading cancellation requires chain definitions)
- TASK-029 gains dependency on TASK-027 (health endpoint needed for Uptime Kuma monitoring)
- TASK-023 gains dependency on TASK-026 (design-time validation needed before Pipeline Builder integrates it)
- Priority adjustments within cycles to reflect reduced cycle scope

---

## Revision Delta (Plan v2 -> v2.1)

### Summary of Changes

**Nexus directive applied at Plan Gate v2:**
TASK-029 (DevOps Phase 2 -- staging environment and CD pipeline) moved from Cycle 5 to Cycle 1. The Nexus wants the walking skeleton shown deployed to staging, not just running locally.

### Changes

| Item | Change |
|---|---|
| TASK-029 | Moved from Cycle 5 P1 to Cycle 1 P2 (last task in cycle, after walking skeleton is complete) |
| TASK-029 dependencies | Changed from TASK-001 + TASK-027 to TASK-001 + TASK-042. TASK-027 (health endpoint, Cycle 3) removed as a dependency -- Uptime Kuma monitoring can initially use a basic HTTP check; TASK-027's health endpoint will enhance it later. TASK-042 added so there is a working walking skeleton to deploy. |
| Cycle 1 task count | 13 -> 14 |
| Cycle 5 task count | 3 -> 2 |
| Cycle 1 demo criteria | Updated to include staging deployment at nexusflow.staging.nxlabs.cc |
| Cycle 5 demo criteria | Updated to reflect staging already exists from Cycle 1 |
| Release Map v2.1 | Updated task counts and cycle assignments |
| Dependency Graph v2.1 | TASK-029 moved to Cycle 1 diagram; Cycle 5 diagram updated |

### Unaffected
- All other tasks retain their cycle assignment, dependencies, and acceptance criteria unchanged
- Release Map release boundaries unchanged (v1.0.0 = Cycles 1-3, v1.1.0 = Cycles 4-5)
- Cut line unchanged
