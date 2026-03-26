# Requirements -- NexusFlow
**Version:** 1
**Date:** 2026-03-25
**Artifact Weight:** Blueprint

## Functional Requirements

### REQ-001: Submit a task via REST API
**Statement:** The system exposes a REST API endpoint that accepts a task submission request containing a pipeline reference, input parameters, and optional retry configuration. The system validates the request and returns a task identifier.
**Origin:** Brief -- Problem Statement; Delivery Channel (REST API)
**Definition of Done:** A well-formed POST request creates a task, returns a unique task ID, and the task appears in the queue. A malformed request returns a structured validation error without creating a task.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a valid pipeline definition exists and the user is authenticated
When the user submits a task via POST /tasks with valid pipeline reference and input parameters
Then the system returns HTTP 201 with a unique task ID and the task state is "submitted"

Given a valid pipeline definition exists and the user is authenticated
When the user submits a task via POST /tasks with an invalid pipeline reference
Then the system returns HTTP 400 with a structured error describing the invalid reference and no task is created

Given a valid pipeline definition exists and the user is authenticated
When the user submits a task without specifying retry configuration
Then the system creates the task with safe default retry settings (infrastructure-only retry)
```

---

### REQ-002: Submit a task via web GUI
**Statement:** The web GUI provides a task submission interface where users can select a pipeline, provide input parameters, and optionally configure retry settings. Submission creates a task equivalent to the REST API path.
**Origin:** Brief -- Delivery Channel (Web App); Nexus-stated GUI requirement
**Definition of Done:** A user can select a pipeline, fill in parameters, and submit. The resulting task is identical in state and behavior to one submitted via the API.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given the user is logged in and at least one pipeline definition exists
When the user selects a pipeline, fills in required parameters, and clicks submit
Then a task is created with state "submitted" and appears in the user's Task Feed

Given the user is logged in and at least one pipeline definition exists
When the user attempts to submit a task with missing required parameters
Then the GUI shows inline validation errors and does not submit the task
```

---

### REQ-003: Task queuing via Redis broker
**Statement:** Upon successful validation, submitted tasks are deposited into the Redis broker queue. Queuing latency must be under 50ms at the 95th percentile.
**Origin:** Brief -- Context and Ground Truths (Redis as broker); SLA target (queuing < 50ms)
**Definition of Done:** A submitted task transitions from "submitted" to "queued" within 50ms (p95). The task is persisted in Redis such that it survives a broker restart.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a task has been submitted and validated
When the system queues the task to Redis
Then the task state transitions to "queued" and the queuing operation completes within 50ms (p95)

Given Redis is configured with persistence enabled
When a task is queued and Redis restarts before the task is picked up
Then the task is still present in the queue after Redis recovers
```

---

### REQ-004: Worker self-registration with heartbeat
**Statement:** Workers register themselves with the system on startup, advertising their capability tags. After registration, workers emit periodic heartbeats. A worker that stops sending heartbeats within the configured timeout is marked as down.
**Origin:** Brief -- Domain Model (Worker, Capability Tag); Nexus-stated self-registration requirement
**Definition of Done:** A worker can start, register with the system, appear in the worker fleet, and be marked as down when heartbeats stop.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given the system is running
When a new worker starts and sends a registration message with capability tags ["etl", "report"]
Then the worker appears in the Worker Fleet Dashboard as "online" with tags "etl" and "report"

Given a worker is registered and online
When the worker stops sending heartbeats for longer than the configured timeout
Then the system marks the worker as "down" and the Worker Fleet Dashboard reflects this status
```

---

### REQ-005: Tag-based task-to-worker matching
**Statement:** When a task is queued, the system matches it to an available worker whose capability tags satisfy the task's required tags. A task is only assigned to a worker that advertises all required tags.
**Origin:** Brief -- Domain Model (Capability Tag); Nexus-stated tag-based matching
**Definition of Done:** A queued task is assigned only to a worker whose tags are a superset of the task's required tags. A task with no matching worker remains queued.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a task requires tags ["etl"] and two workers are online: Worker-A with tags ["etl", "report"] and Worker-B with tags ["report"]
When the system assigns the task
Then the task is assigned to Worker-A (not Worker-B)

Given a task requires tags ["billing"] and no online worker has the "billing" tag
When the system attempts to assign the task
Then the task remains in "queued" state until a matching worker becomes available
```

---

### REQ-006: Three-phase pipeline execution (DataSource, Process, Sink)
**Statement:** A worker executes a task by running its pipeline in three sequential phases: DataSource (ingest data), Process (transform data), Sink (write output). Each phase runs to completion before the next begins.
**Origin:** Brief -- Domain Model (Pipeline, DataSource, Process, Sink)
**Definition of Done:** A task assigned to a worker executes all three phases in order. The output of each phase is available as input to the next. If any phase fails, subsequent phases do not execute.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a task is assigned to a worker with a valid pipeline definition
When the worker begins execution
Then DataSource runs first, then Process receives DataSource output, then Sink receives Process output, and the task state transitions to "completed"

Given a task is assigned to a worker and the Process phase fails with a script error
When the failure occurs
Then the Sink phase does not execute and the task state transitions to "failed"
```

---

### REQ-007: Schema mapping between pipeline phases
**Statement:** Pipeline definitions include schema mappings that define how output fields from one phase map to input fields of the next phase. The system applies these mappings at the boundary between DataSource->Process and Process->Sink.
**Origin:** Brief -- Domain Model (Schema Mapping); Nexus-stated schema mapping support
**Definition of Done:** A pipeline with defined schema mappings correctly transforms field names/structures between phases. A mapping that references a nonexistent source field produces a clear error.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a pipeline where DataSource outputs {"customer_id": 123, "amount": 50.0} and the schema mapping renames "customer_id" to "id" for the Process phase
When the pipeline executes
Then the Process phase receives {"id": 123, "amount": 50.0}

Given a schema mapping references a source field "nonexistent_field" that the DataSource does not produce
When the pipeline executes and DataSource completes
Then the system fails the task with a clear error indicating the missing field in the mapping
```

---

### REQ-008: Atomic sink operations with cleanup on failure
**Statement:** Sink operations are atomic. If a Sink phase fails partway through writing, all partial writes from that Sink execution are rolled back. There is no partial-success state.
**Origin:** Brief -- Domain Invariant 3 (Sink atomicity); Nexus-stated atomic sink
**Definition of Done:** A Sink that fails mid-write leaves no partial data at the destination. Verification: after a forced Sink failure, the destination state is identical to its state before the Sink began.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a Sink is writing 100 records to a destination
When the Sink fails after writing 50 records
Then all 50 partial records are cleaned up and the destination contains zero records from this execution

Given a Sink completes successfully
When all records are written
Then the destination contains exactly the expected records and the task state transitions to "completed"
```

---

### REQ-009: Task lifecycle state tracking
**Statement:** Every task has a lifecycle state visible to authorized users. States are: submitted, queued, assigned, running, completed, failed, cancelled. State transitions are monotonically forward except for failover reassignment (running/assigned -> queued).
**Origin:** Brief -- Domain Model (Task); Domain Invariant 1; Nexus-confirmed lifecycle states
**Definition of Done:** The system records state transitions with timestamps. The API and GUI show the current state. No invalid state transition is permitted.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a task is in state "running"
When the task completes successfully
Then the task state transitions to "completed" with a timestamp

Given a task is in state "completed"
When any operation attempts to transition it to "queued"
Then the system rejects the transition (completed is a terminal state)

Given a task is in state "running" and its assigned worker is detected as down
When the system triggers failover
Then the task state transitions back to "queued" for reassignment
```

---

### REQ-010: Cancel a running task
**Statement:** The submitting user or an admin can cancel a task that is in a cancellable state (submitted, queued, assigned, running). Cancellation of a running task signals the worker to stop execution.
**Origin:** Brief -- Domain Invariant 8 (Cancel authority); Nexus-confirmed cancellation
**Definition of Done:** A cancel request from the task owner or admin transitions the task to "cancelled." A cancel request from a non-owner non-admin is rejected. Cancellation of a running task causes the worker to halt execution.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a task is in state "running" and belongs to User-A
When User-A sends a cancel request
Then the task state transitions to "cancelled" and the worker stops executing it

Given a task belongs to User-A
When User-B (a non-admin) sends a cancel request for that task
Then the system rejects the request with HTTP 403

Given a task is in state "completed"
When the owner sends a cancel request
Then the system rejects the request (completed is a terminal state)
```

---

### REQ-011: Infrastructure-failure retry with per-task configuration
**Statement:** When a task fails due to an infrastructure failure (downed worker, network issue), the system retries the task according to its retry configuration. Process/script errors do not trigger retry. Each task can specify retry count and backoff; unspecified values use safe system defaults.
**Origin:** Brief -- Domain Invariant 2 (infrastructure-only retry); Nexus-confirmed retry semantics
**Definition of Done:** An infrastructure-failed task is retried up to the configured limit. A process-error-failed task is not retried. Retry count and backoff are configurable per task with documented defaults.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a task has retry configuration {max_retries: 3, backoff: "exponential"} and the assigned worker goes down mid-execution
When the system detects the worker failure
Then the task is re-queued for assignment (retry count incremented) and eventually assigned to a healthy worker

Given a task is running and the Process phase exits with a non-zero error code (script error)
When the system processes the failure
Then the task is NOT retried and transitions to "failed" immediately

Given a task has exhausted its maximum retry count due to repeated infrastructure failures
When the next infrastructure failure occurs
Then the task transitions to "failed" and is placed in the Dead Letter Queue
```

---

### REQ-012: Dead letter queue with cascading cancellation
**Statement:** Tasks that exhaust retries or fail unrecoverably are placed in a dead letter queue. If the failed task is part of a pipeline chain, all downstream tasks in the chain are cancelled.
**Origin:** Brief -- Domain Model (Dead Letter Queue); Domain Invariant 4 (cascading cancellation)
**Definition of Done:** A terminally failed task appears in the dead letter queue. If it is part of a chain, all downstream tasks transition to "cancelled."
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a pipeline chain A -> B -> C and task A has exhausted all retries
When task A enters the Dead Letter Queue
Then task B and task C are cancelled with reason "upstream task failed"

Given a standalone task (not part of a chain) exhausts retries
When the task enters the Dead Letter Queue
Then no cascading cancellation occurs and the task is visible in the dead letter queue for inspection
```

---

### REQ-013: Auto-failover for downed workers
**Statement:** The system monitors worker heartbeats. When a worker is detected as down, all tasks assigned to or running on that worker are re-queued for assignment to healthy workers (subject to retry limits).
**Origin:** Brief -- Domain Model (Worker liveness); Domain Invariant 7; Nexus intake description (auto-reassignment)
**Definition of Done:** When a worker stops heartbeating, its in-flight tasks are re-queued within one heartbeat-timeout interval. Tasks are assigned to other healthy workers that match their tags.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given Worker-A is running Task-1 and Task-2
When Worker-A stops sending heartbeats and the timeout expires
Then Task-1 and Task-2 are re-queued with state "queued" and are eligible for assignment to other matching workers

Given Worker-A goes down and its Task-1 has already exhausted max retries
When the system processes the failover
Then Task-1 is sent to the Dead Letter Queue (not re-queued) and cascading cancellation applies if it is part of a chain
```

---

### REQ-014: Linear pipeline chaining
**Statement:** Users can define pipeline chains where the completion of one pipeline's task triggers the next pipeline in the chain. Chains are strictly linear (A -> B -> C). No branching or fan-out.
**Origin:** Brief -- Domain Model (Pipeline Chain); Domain Invariant 6; Nexus-confirmed linear pipeline for phase 1
**Definition of Done:** A user can define a chain of pipelines. When a task for pipeline A completes, a task for pipeline B is automatically submitted. Branching is not permitted.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given a pipeline chain A -> B -> C is defined
When task for pipeline A completes successfully
Then a task for pipeline B is automatically submitted with state "submitted"

Given a pipeline chain A -> B is defined
When a user attempts to define a branching chain where A triggers both B and C simultaneously
Then the system rejects the definition with an error indicating only linear chains are supported
```

---

### REQ-015: Pipeline Builder (web GUI)
**Statement:** The web GUI provides a Pipeline Builder view where users can visually construct pipelines by dragging and dropping DataSource, Process, and Sink components and defining schema mappings between them.
**Origin:** Brief -- Delivery Channel (Web App); Nexus-stated Pipeline Builder
**Definition of Done:** A user can create a complete pipeline definition using drag-and-drop in the GUI. The resulting pipeline is equivalent to one defined via API.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given the user is logged in and on the Pipeline Builder view
When the user drags a DataSource, a Process, and a Sink onto the canvas and connects them with schema mappings
Then a valid pipeline definition is saved and is available for task submission

Given the user has placed a DataSource and a Process on the canvas
When the user defines a schema mapping between them using the mapping editor
Then the mapping is validated and persisted as part of the pipeline definition
```

---

### REQ-016: Worker Fleet Dashboard (web GUI)
**Statement:** The web GUI provides a Worker Fleet Dashboard showing all registered workers, their status (online/down), capability tags, and current task assignment.
**Origin:** Brief -- Delivery Channel (Web App); Nexus-stated Dashboard
**Definition of Done:** The dashboard displays all workers with real-time status. When a worker goes down, its status updates within one heartbeat-timeout interval.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given three workers are registered: Worker-A (online), Worker-B (online), Worker-C (down)
When the user views the Worker Fleet Dashboard
Then all three workers are displayed with their correct statuses and capability tags

Given Worker-A is shown as "online" on the dashboard
When Worker-A stops heartbeating and the timeout expires
Then the dashboard updates Worker-A's status to "down" without requiring a page refresh
```

---

### REQ-017: Task Feed and Monitor (web GUI)
**Statement:** The web GUI provides a Task Feed view showing the user's tasks (or all tasks for admins) with their current lifecycle state, submission time, and pipeline reference. Tasks update in real time as state transitions occur.
**Origin:** Brief -- Delivery Channel (Web App); Nexus-stated Task Feed and Monitor
**Definition of Done:** The task feed shows tasks with real-time state updates. Regular users see only their own tasks; admins see all tasks.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given User-A has submitted 5 tasks and User-B has submitted 3 tasks
When User-A views the Task Feed
Then User-A sees only their 5 tasks with current states

Given User-A has submitted 5 tasks and User-B has submitted 3 tasks
When an Admin views the Task Feed
Then the Admin sees all 8 tasks

Given a task is in state "running"
When the task transitions to "completed"
Then the Task Feed updates the task's state in real time without requiring a page refresh
```

---

### REQ-018: Real-time log streaming
**Statement:** During task execution, logs are streamed in real time. Users can view logs for their own tasks via the GUI Log Streamer or via a web stream API (SSE or WebSocket). Admins can view logs for any task.
**Origin:** Brief -- Delivery Channel; Nexus-stated Log Streamer and web stream API
**Definition of Done:** Logs appear in the GUI within 2 seconds of being produced by the worker. The stream API delivers the same logs programmatically. Access control is enforced (users see only their own task logs; admins see all).
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given Task-1 belongs to User-A and is running
When User-A opens the Log Streamer for Task-1
Then log lines appear in real time as the worker produces them

Given Task-1 belongs to User-A
When User-B (non-admin) attempts to stream logs for Task-1
Then the system denies access with HTTP 403

Given Task-1 is running and producing logs
When a client connects to the stream API for Task-1
Then the client receives log lines via SSE or WebSocket as they are produced
```

---

### REQ-019: User authentication and role-based access
**Statement:** The system authenticates users and enforces role-based access control. Two roles exist: Admin and User. Admins can manage users and view all tasks. Users can manage their own pipelines and tasks and view all workers.
**Origin:** Brief -- User Roles; Domain Invariant 5 (visibility isolation)
**Definition of Done:** Unauthenticated requests are rejected. Authenticated users can only perform actions permitted by their role. Role checks are enforced on all API endpoints and GUI views.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given an unauthenticated request
When the request hits any protected endpoint
Then the system returns HTTP 401

Given a user with role "User"
When the user attempts to access the admin user management endpoint
Then the system returns HTTP 403

Given a user with role "Admin"
When the admin accesses the user management endpoint
Then the admin can list, create, and deactivate user accounts
```

---

### REQ-020: Admin user management
**Statement:** Admins can create, view, and deactivate user accounts. Admins manage users from other teams within the organization.
**Origin:** Brief -- User Roles (Admin); Nexus-stated admin manages users from other teams
**Definition of Done:** An admin can create a new user account, view all user accounts, and deactivate an account. Deactivated users cannot log in or submit tasks.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given an admin is logged in
When the admin creates a new user account with valid details
Then the user account is created and the new user can log in

Given an admin deactivates User-A's account
When User-A attempts to log in
Then the login is rejected

Given an admin deactivates User-A's account
When viewing User-A's previously submitted tasks
Then the tasks remain visible and continue executing (deactivation does not cancel in-flight tasks)
```

---

### REQ-021: Throughput capacity
**Statement:** The system must sustain processing of 10,000 tasks per hour under normal operating conditions.
**Origin:** Nexus-stated SLA target
**Definition of Done:** A load test demonstrates 10,000 tasks processed end-to-end in one hour with no tasks dropped or lost.
**Priority:** Must Have
**Status:** Draft

**Acceptance Scenarios:**
```
Given the system is running with a sufficient worker fleet
When 10,000 tasks are submitted within a one-hour window
Then all 10,000 tasks reach a terminal state (completed or failed) within that hour and no tasks are lost from the queue
```

---

## Non-Functional Requirements

### NFR-001: Queuing latency SLA
**Statement:** Task queuing latency (time from validated submission to "queued" state in Redis) must be under 50ms at the 95th percentile.
**Origin:** Nexus-stated SLA target (queuing < 50ms)
**Definition of Done:** Under sustained load (10,000 tasks/hour), p95 queuing latency is measured and remains below 50ms.
**Priority:** Must Have
**Status:** Draft

---

### NFR-002: Redis persistence and recovery
**Statement:** The Redis broker must be configured for persistence such that queued tasks survive a broker restart. No queued task is lost due to a Redis restart.
**Origin:** Brief -- Context and Ground Truths (Redis as persistent broker)
**Definition of Done:** After a Redis restart, all previously queued but unprocessed tasks are still present in the queue and are eventually processed.
**Priority:** Must Have
**Status:** Draft

---

### NFR-003: Real-time update latency
**Statement:** State changes (task state transitions, worker status changes) must be reflected in the GUI within 2 seconds of occurring.
**Origin:** Brief -- Delivery Channel (real-time GUI); log streaming requirement
**Definition of Done:** Measured end-to-end: a task state change on the backend appears in the GUI within 2 seconds. Worker status changes appear within one heartbeat-timeout interval.
**Priority:** Should Have
**Status:** Draft

---

### NFR-004: Graceful degradation under worker loss
**Statement:** The system continues operating when workers go down. Tasks are reassigned; no manual intervention is required to recover from worker failures.
**Origin:** Brief -- Domain Invariant 7 (Worker liveness); auto-failover requirement
**Definition of Done:** When 50% of the worker fleet goes down simultaneously, all affected tasks are re-queued and eventually processed by remaining workers (within retry limits). No operator intervention is required.
**Priority:** Must Have
**Status:** Draft

---

## Superseded Requirements
(none -- this is version 1)
