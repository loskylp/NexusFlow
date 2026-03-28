# Handoff Note — OBS-023

**Date:** 2026-03-26
**Builder session:** targeted bugfix (not a plan task)
**Observation fixed:** OBS-023 — race condition in task submit handler

---

## What was built

Fixed the race condition in `api/handlers_tasks.go` `Submit` where `Enqueue`
(Redis XADD) was called before `UpdateStatus(queued)` (PostgreSQL write). A
fast worker could pick up the task from the stream while it was still in
"submitted" state and fail the submitted→assigned state transition.

### Change summary

**`api/handlers_tasks.go`** — lines ~146–160 reordered:

- `UpdateStatus(queued)` is now called **first**. If it fails, 500 is returned
  without enqueueing; task stays in "submitted" (recoverable by operator).
- `Enqueue` is called **after** the status is durable. If it fails, 500 is
  returned; task stays in "queued" in PostgreSQL, which is a recoverable state.
- Inline comments document the rationale for both error paths.
- `Submit` docstring updated to reflect the corrected operation order and both
  failure postconditions.

**`api/handlers_tasks_test.go`** — new spy and test added:

- `orderingProducer` — a `queue.Producer` spy that snapshots the task's status
  from the `stubTaskRepo` at the instant `Enqueue` is called.
- `TestSubmit_StatusQueuedBeforeEnqueue` — fails with the old code (status is
  "submitted" at enqueue time), passes with the fix (status is "queued").

### TDD cycle

1. **Red** — added `TestSubmit_StatusQueuedBeforeEnqueue`; confirmed failure:
   `expected task status "queued" at enqueue time, got "submitted"`.
2. **Green** — swapped `UpdateStatus` before `Enqueue` in the handler.
3. **Refactor** — updated `Submit` docstring (postcondition ordering, failure
   paths); added inline comments explaining the rationale.

---

## Test results

```
go test ./api/... -v
ok  github.com/nxlabs/nexusflow/api  1.392s  (50 tests, all PASS)
```

No existing tests required modification — the stub ordering is transparent to
all prior tests because they only check the final state after `Submit` returns.

---

## Deviations

None. The fix is exactly as specified in the OBS-023 description.

---

## Limitations / notes for Verifier

- The fix is visible at the unit-test level via the `orderingProducer` spy. An
  integration test with a real Redis consumer would provide stronger assurance
  but is outside the Builder's scope.
- If `Enqueue` fails after `UpdateStatus`, the task is left in "queued" with no
  corresponding stream entry. A reconciler to re-enqueue orphaned "queued" tasks
  is not part of this bugfix and should be tracked separately if required.
