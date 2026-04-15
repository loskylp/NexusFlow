<!-- Copyright 2026 Pablo Ochendrowitsch — Apache License 2.0 -->

# Routing Instruction — Builder — REG-030 CI Regression Fixes
**Date:** 2026-04-15 | **From:** Orchestrator | **To:** Builder
**Cycle:** 4 | **Task:** REG-030 (bundled: REG-030-1, REG-030-2, REG-030-3)

## Context

TASK-030 (MinIO Fake-S3) Verifier returned PASS — all 4 ACs PASS. During the CI regression check, the Verifier identified three **pre-existing CI regressions** introduced by the Cycle 4 scaffold commit (66c4bf0). These failures are present on every commit since the scaffold was pushed and are **outside TASK-030's scope** — they affect code owned by SEC-001, TASK-032, and TASK-034 scaffolds.

CI is currently red on `main`. This must be restored to green before the next verification in Cycle 4 (TASK-033), otherwise subsequent Verifier CI reports will be polluted by pre-existing noise and may mask new regressions.

All three fixes are small and mechanical. Bundle them in a single Builder dispatch and single commit.

## What to do

Apply the three fixes described below. Do not modify TASK-030 / MinIO code. Do not modify behaviour of SEC-001, TASK-032, or TASK-034 scaffolds — these fixes are purely to make the scaffolds type-check and pass `go vet` against pre-existing tests.

### REG-030-1 — Go vet: `stubUserRepo` missing `ChangePassword`

**File:** `api/handlers_auth_test.go`
**Fix:** Add a no-op `ChangePassword(ctx context.Context, userID string, newHash string) error` method to `stubUserRepo` (signature must match the `db.UserRepository` interface as defined in `internal/db/user_repository.go`). Return `nil`. The stub is for tests that do not exercise password change.

**Verification:** `go vet ./...` must return clean. `go test ./api/...` must pass.

### REG-030-2 — TypeScript: `mustChangePassword` missing from User fixtures

**Files (7):**
- `web/src/components/ProtectedRoute.test.tsx`
- `web/src/components/Sidebar.test.tsx`
- `web/src/context/AuthContext.test.tsx`
- `web/src/pages/LogStreamerPage.test.tsx`
- `web/src/pages/LoginPage.test.tsx`
- `web/src/pages/PipelineManagerPage.test.tsx`
- `web/src/pages/TaskFeedPage.test.tsx`

**Fix:** Add `mustChangePassword: false` to every inline `User` mock object in these files. Do not modify the `User` type in `web/src/types/domain.ts`.

**Verification:** `npm run typecheck` (or `tsc --noEmit`) in `web/` must return clean. `npm test` in `web/` must pass.

### REG-030-3 — TypeScript: unused variables in scaffold stubs

**Files (4):**
- `web/src/hooks/useSinkInspector.ts`
- `web/src/pages/ChangePasswordPage.tsx`
- `web/src/pages/ChaosControllerPage.tsx`
- `web/src/pages/SinkInspectorPage.tsx`

**Fix:** Preferred approach — **remove** the unused destructured names/imports from the scaffold stubs. They will be re-introduced when TASK-032, TASK-034, and SEC-001 are implemented and actually wire the components into JSX. Do not use `_`-prefix suppression; remove rather than hide, since the placeholder intent is clearer when the future implementer sees an empty component body rather than an unused `_Foo` binding.

Preserve any file-level comments or TODO markers that signal the stub's intent. If a scaffold stub becomes effectively empty, leave a single line comment `// Stub — see TASK-<nnn> (scaffold: process/scaffolder/...)` so the next Builder knows where to start.

**Verification:** `npm run typecheck` in `web/` must return clean.

## Required documents

- Verifier report (source of the regression descriptions): [`process/verifier/verification-reports/TASK-030-verification.md`](../verifier/verification-reports/TASK-030-verification.md) — CI Regression Report section
- Interface definition for REG-030-1: [`internal/db/user_repository.go`](../../internal/db/user_repository.go) — `UserRepository` interface, `ChangePassword` signature
- User type for REG-030-2: [`web/src/types/domain.ts`](../../web/src/types/domain.ts) — `User` type, `mustChangePassword` field

## Out of scope

- Do NOT implement `ChangePassword` persistence logic. The real implementation is SEC-001's responsibility.
- Do NOT render the Sink Inspector, Chaos Controller, or Change Password pages. Those are TASK-032, TASK-034, SEC-001 responsibilities respectively.
- Do NOT change test assertions or add new tests. This is a compile/typecheck restoration only.

## Acceptance signals

Before handing back to the Orchestrator, confirm ALL of the following locally:

1. `go vet ./...` — clean
2. `go build ./...` — clean
3. `go test ./...` — all pre-existing tests pass (no new failures)
4. `cd web && npm run typecheck` — clean (no TS errors)
5. `cd web && npm test` — all pre-existing tests pass

Commit the fixes as a single commit with message `fix(cycle-4): restore CI green — REG-030-1/2/3 scaffold regressions` (follow commit-discipline skill).

## Handoff

When complete, report back with:
- Commit SHA
- Files changed (list)
- Confirmation of all five acceptance signals above
- Any deviations from the fix descriptions above (there should be none — these are mechanical)

**Next after Builder:** Orchestrator will route to Verifier in **regression-confirmation mode** (run CI locally/remote; no new tests written) to confirm CI green. On Verifier PASS, Orchestrator will dispatch Builder for TASK-033 (next task in Cycle 4 sequence).
