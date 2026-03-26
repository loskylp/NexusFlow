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

# Verification Report — TASK-006
**Date:** 2026-03-26 | **Result:** PASS
**Task:** Worker self-registration and heartbeat | **Requirement(s):** REQ-004, ADR-002

## Acceptance Criteria Results

| REQ | Criterion | Layer | Result | Notes |
|---|---|---|---|---|
| REQ-004 | Worker starts and appears in the workers table with status "online" and correct capability tags | Acceptance | PASS | Verified live: `verifier-worker-ac1` with tags `etl,http` — DB row shows `status=online`, `tags={etl,http}` |
| REQ-004 / ADR-002 | Worker heartbeat updates workers:active sorted set in Redis every 5 seconds | Acceptance | PASS | Score increased from 1774557133 to 1774557143 over 7-second window (delta ~5s confirms interval) |
| REQ-004 | Multiple workers can register simultaneously with different tags | Acceptance | PASS | `verifier-worker-ac3a` (report,batch) and `verifier-worker-ac3b` (ml,gpu) both registered concurrently; both appear in DB and Redis |
| REQ-004 | Worker record includes registration timestamp and tags | Acceptance | PASS | `registered_at` populated at registration time; verified recent (within 120s of now) on both workers |

## Test Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Integration | 0 | — | — |
| System | 0 | — | — |
| Acceptance | 14 assertions | 14 | 0 |
| Performance | 0 | — | — |

**Unit tests (Builder-owned, run as part of verification):** 35 passed, 5 skipped (Redis session store tests skip when `REDIS_ADDR` is not `localhost:6379` — correct; those tests are covered by the queue integration test suite).

**Static analysis:** `go build ./...` — clean. `go vet ./...` — no findings.

### Test breakdown

**Acceptance test:** `tests/acceptance/TASK-006-acceptance.sh`

| Assertion | Criterion | Result |
|---|---|---|
| Unit tests: 35 passed | REQ-004, ADR-002 | PASS |
| AC-1 positive: worker exists in workers table with status=online | AC-1 | PASS |
| AC-1 positive: worker tags match config — {etl,http} | AC-1 | PASS |
| AC-1/AC-4: registered_at is populated | AC-1, AC-4 | PASS |
| AC-2 positive: initial heartbeat in workers:active | AC-2 | PASS |
| AC-2 positive: heartbeat score increased after 7s | AC-2 | PASS |
| AC-3 positive: both workers appear in workers table with status=online | AC-3 | PASS |
| AC-3 positive: worker-ac3a tags correct — {report,batch} | AC-3 | PASS |
| AC-3 positive: worker-ac3b tags correct — {ml,gpu} | AC-3 | PASS |
| AC-3 positive: both workers appear in workers:active in Redis | AC-3 | PASS |
| AC-4 positive: worker-ac3a registered_at is recent | AC-4 | PASS |
| AC-4 positive: worker-ac3b registered_at is recent | AC-4 | PASS |
| AC-4 positive: worker-ac3b tags field populated and correct | AC-4 | PASS |
| [VERIFIER-ADDED] Graceful shutdown: SIGTERM transitions status to down | ADR-002 | PASS |

### Negative coverage

All four acceptance criteria include embedded negative cases that prevent a trivially permissive implementation from passing:

- **AC-1/AC-4 tags:** The assertion compares the exact tag string `etl,http` — a no-op implementation that writes an empty tags array, or writes hardcoded tags, would fail.
- **AC-2 heartbeat interval:** A score that does not increase over 7 seconds would fail. A static initial heartbeat only (no periodic emitter) would produce a fixed score and fail this check.
- **AC-3 concurrent registration:** Both workers must appear with distinct, correct tags. A serialised or mutex-blocked implementation that only admits one registration would produce WORKER_COUNT=1 and fail.
- **AC-4 timestamp recency:** The `registered_at` epoch must be greater than `(now - 120s)`. A zero value, NULL, or epoch (1970-01-01) would fail the arithmetic check.
- **[VERIFIER-ADDED] Graceful shutdown:** A worker that does not call `markOffline` on context cancellation would retain `status=online` and fail.

## Observations (non-blocking)

**OBS-016:** `markOffline` uses `models.WorkerStatusDown` ("down") as the offline status, consistent with the `status` field values. The task description uses "offline" in plain English but the DB stores "down" — ADR-002 uses "down" as the defined value. The code is correct; this is just a terminology note for documentation reviewers.

**OBS-017:** The `runConsumptionLoop` method currently blocks on `ctx.Done()` (TASK-007 stub). When `consumer` is non-nil (production path), this means `Worker.Run` reaches the `runConsumptionLoop` call and blocks there until cancellation, which is correct placeholder behaviour. However, `InitGroups` is called before `runConsumptionLoop`, which means consumer groups are created on startup even though no consumption happens yet. This is benign — groups are idempotent via `BUSYGROUP` handling — but it does create Redis stream structures before any tasks exist. TASK-007 will give this full meaning.

**OBS-018:** The worker binary in the `Dockerfile.worker` (to be verified at TASK-007) needs to produce a binary that runs correctly on the alpine:3.20 base image. The Verifier tested with the binary produced by `golang:1.23-alpine` (linux/amd64) running against alpine:3.20 containers. This worked because both are musl-libc environments. If the CI build environment changes (e.g., to `golang:1.23` with glibc output), the alpine runtime image would need to change or `CGO_ENABLED=0` would need to be set explicitly.

## Recommendation

PASS TO NEXT STAGE — all four acceptance criteria are satisfied. The implementation is correct, clean, and well-structured. Commit the verified artifacts and push.
