<!--
Copyright 2026 Pablo Ochendrowitsch

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

---
task: TASK-033
title: Sink Before/After snapshot capture
requirements: DEMO-003, ADR-009
environment: staging — https://nexusflow.nxlabs.cc (or local demo profile via make up-demo)
note: This is a backend capability. Full user-visible behaviour is demonstrated by TASK-032 (Sink Inspector GUI). These scenarios verify the SSE event stream directly using redis-cli and curl.
---

# Demo Script — TASK-033
**Feature:** Sink Before/After snapshot capture
**Requirement(s):** DEMO-003, ADR-009
**Environment:** staging (https://nexusflow.nxlabs.cc) or local demo profile (`make up-demo`)

## Scenario 1: Before snapshot published before Sink write begins

**REQ:** DEMO-003, ADR-009 / AC-1, AC-3

**Given:** the demo stack is running with the demo profile (MinIO and demo-postgres available); you are subscribed to the Redis Pub/Sub channel for a task that has not yet run

**When:** open a terminal and subscribe to the sink events channel, then submit a task whose Sink is a database connector targeting the demo-postgres instance — the command sequence is:

1. In terminal A: `redis-cli -u $REDIS_URL subscribe events:sink:<taskId>` (replace `<taskId>` with the UUID returned in the next step)
2. In terminal B: submit the task via the API — `curl -s -X POST $API_BASE/api/v1/tasks -H "Authorization: Bearer <admin-token>" -H "Content-Type: application/json" -d '{"pipelineId":"<demo-pipeline-uuid>","input":{}}'`
3. Note the task UUID returned and update the `redis-cli subscribe` channel in terminal A

**Then:**
- Terminal A receives a message with `"eventType":"sink:before-snapshot"` before the task reaches the `completed` status
- The message payload contains `"before":{"phase":"before","data":{"row_count":<N>},"capturedAt":"<RFC3339>"}` where `<N>` is the number of rows in the target table before the write
- The `"taskId"` field in the payload matches the submitted task UUID
- A second message with `"eventType":"sink:after-result"` arrives after the task completes, carrying both `"before"` and `"after"` snapshots; `"after".data.row_count` is greater than `"before".data.row_count`

**Notes:** If the demo pipeline uses an S3 sink instead of a database sink, the snapshot data carries `"object_count"` instead of `"row_count"` (AC-5). The channel name format is identical: `events:sink:{taskId}`.

---

## Scenario 2: After snapshot on successful write — Before and After differ

**REQ:** DEMO-003, ADR-009 / AC-2, AC-4

**Given:** the demo stack is running; the target database table has at least one existing row (check with `SELECT COUNT(*) FROM <table>` on demo-postgres)

**When:** subscribe to `events:sink:<taskId>` as in Scenario 1 and submit a task that writes at least one new record to the database sink

**Then:**
- The `sink:after-result` event payload has `"rolledBack":false` (or the field is absent)
- `after.data.row_count` is strictly greater than `before.data.row_count` (the write committed new rows)
- `writeError` is absent or empty in the payload
- Task status transitions to `completed`

---

## Scenario 3: Rollback — After snapshot matches Before snapshot

**REQ:** DEMO-003, ADR-009 / AC-6

**Given:** the demo stack is running; a pipeline is configured with a Sink that is set to fail (for example, by pointing the database sink at a non-existent table or by using the Chaos Controller to disconnect the database)

**When:** subscribe to `events:sink:<taskId>` and submit the task

**Then:**
- The `sink:after-result` event payload contains `"rolledBack":true`
- `after.data.row_count` (or `after.data.object_count` for S3) equals `before.data.row_count` — the destination is unchanged
- `writeError` contains a non-empty error message describing the failure
- Task status transitions to `failed`

**Notes:** This scenario is most conveniently demonstrated using the Chaos Controller (TASK-034) to trigger a mid-write database disconnection, or by submitting a task with an invalid Sink configuration. The snapshot equality check (Before == After on rollback) is the atomicity proof.

---

## Scenario 4: S3 sink — object_count in target prefix

**REQ:** DEMO-003, ADR-009 / AC-5

**Given:** the demo stack is running with MinIO (demo profile); the target S3 bucket has at least one existing object in the output prefix

**When:** subscribe to `events:sink:<taskId>` and submit a task using the S3 sink connector targeting the demo MinIO bucket (e.g. `s3://demo-bucket/exports/`)

**Then:**
- The `sink:before-snapshot` event payload contains `"before":{"data":{"object_count":<N>}}` where `<N>` is the number of objects under the `exports/` prefix before the write
- The `sink:after-result` event carries `"after":{"data":{"object_count":<N+1>}}` after a successful write
- The `"before"` snapshot data does not contain a `"row_count"` key — that key is exclusive to database sinks

**Notes:** Both the database and S3 scenarios publish to the same `events:sink:{taskId}` channel with the same event type names. The only difference is the shape of the `data` map inside each snapshot.
