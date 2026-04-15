<!-- Copyright 2026 Pablo Ochendrowitsch — Apache License 2.0 -->

# Routing Instruction — Builder (REG-030 iteration 2)

**To:** @nexus-builder
**From:** Orchestrator
**Date:** 2026-04-15
**Cycle:** 4
**Task:** REG-030-4 — Suppress staticcheck U1000 violations in Cycle 4 scaffold stubs
**Iteration:** 2

## Context

REG-030 iteration 1 (commit 809e299) fixed REG-030-1 (`go vet` ChangePassword method), REG-030-2 (7 web test User fixtures), and REG-030-3 (4 web scaffold stubs unused locals). Verifier confirmed all three PASS.

Advancing past `go vet` exposed a fourth regression previously masked: `staticcheck ./...` reports 11 U1000 "declared and unused" violations in Cycle 4 scaffold stubs from commit 66c4bf0. CI remains red.

These declarations are intentional placeholders for not-yet-implemented tasks. The fix is inline `//nolint:U1000` suppressions that reference the implementing task, so the suppression is self-documenting and removable when the task lands.

Verifier report: [process/verifier/verification-reports/REG-030-verification.md](../verifier/verification-reports/REG-030-verification.md)

## Scope — exactly 11 declarations across 4 files

Add a line-level suppression comment directly above each declaration:

```
//nolint:U1000 // scaffold placeholder for <TASK-NNN>
```

The implementing task for each file:

| File | Implementing task | Declarations to suppress |
|---|---|---|
| `api/handlers_chaos.go` | TASK-034 (Chaos Controller GUI) | line 29 `killWorkerRequest`, line 36 `disconnectDBRequest`, line 43 `floodQueueRequest`, line 54 `chaosActivityEntry` |
| `api/handlers_password_change.go` | SEC-001 (Password change + mandatory first-login) | line 34 `changePasswordRequest` |
| `worker/connector_postgres.go` | TASK-031 (Mock-Postgres with seed data) | line 69 `db` field, line 131 `db` field, line 132 `dedup` field |
| `worker/snapshot.go` | TASK-033 (Sink Before/After snapshot capture) | line 45 `connector` field, line 46 `publisher` field, line 118 `sinkSnapshotEvent` type |

## Required Documents

- Verifier report (Failure Details §FAIL-001 has the line-by-line list): [process/verifier/verification-reports/REG-030-verification.md](../verifier/verification-reports/REG-030-verification.md)
- Project state: [process/orchestrator/project-state.md](./project-state.md)

## Acceptance Criteria

1. All 11 declarations carry an inline `//nolint:U1000 // scaffold placeholder for <TASK-NNN>` comment immediately preceding the declaration, with the correct task ID per the table above.
2. No other changes to the scaffold stubs (no logic, no struct-field renames, no signature changes).
3. Local verification:
   - `go vet ./...` — PASS
   - `go build ./...` — PASS
   - `staticcheck ./...` — PASS (0 violations)
   - `go test ./...` — PASS (previously blocked by staticcheck halt in CI; must run clean locally)
   - `npm --prefix web run typecheck` — PASS (still green from 809e299)
4. Commit message references REG-030-4 and the Verifier report, and describes the mechanical nature of the change.

## You Must Not

- Implement any of the placeholder types or fields — they must remain stubs.
- Change staticcheck configuration at the package or repo level (do not add `staticcheck.conf` disabling U1000). Per-declaration suppression is the required approach — it is self-documenting.
- Touch unrelated files.

## Handoff

On completion, return control with: commit SHA, local verification output, and a one-line status. The Orchestrator will re-dispatch the Verifier in regression-confirmation mode.

**Next:** Invoke @nexus-builder.
