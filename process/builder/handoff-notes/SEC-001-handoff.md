# Handoff Note — SEC-001: Password Change Endpoint + ChangePasswordPage + Mandatory First-Login

**Task:** SEC-001
**Status:** Complete
**Builder:** Nexus Builder (SEC-001)
**Date:** 2026-04-15

---

## What Was Built

### Backend — `api/handlers_password_change.go` (implemented)

- `PasswordChangeHandler.ChangePassword` — full implementation replacing the scaffold stub.
- Validates: nil guard (`users` / `sessions` not nil), session presence in context (403), JSON decode (400), empty fields (400), new password length >= 8 (400), bcrypt current-password verification (401).
- On success: atomically updates `password_hash` + `must_change_password=FALSE` via `users.ChangePassword`, then invalidates all sessions via `sessions.DeleteAllForUser` (AC-6).
- Returns 204 No Content on success; all error paths return before any write.
- `writeJSONError` helper extracted for consistent `{"error":"..."}` envelope.

### Backend — `api/handlers_auth.go` (modified)

- `publicUser` struct extended with `MustChangePassword bool` field and `json:"mustChangePassword"` tag so the frontend receives the flag in the login response.
- `Login` handler: session now carries `MustChangePassword: user.MustChangePassword` (was zero-value false before this change, breaking AC-1/AC-2).

### Backend — `cmd/api/main.go` (modified)

- `seedAdminIfEmpty`: now sets `MustChangePassword: true` on the seeded admin user (AC-1).

### Backend — `internal/auth/auth.go` (modified)

- Middleware `MustChangePassword` block refactored to call `isMustChangePasswordExempt(r)`.
- `isMustChangePasswordExempt` added: allows `POST /api/auth/change-password`, `POST /api/auth/logout`, `GET /api/auth/me` through without the 403 block.

### Database — migrations and sqlc

- `internal/db/migrations/000007_must_change_password.up.sql` — copied from `internal/db/` to the embedded `migrations/` directory (required for both `golang-migrate` and `sqlc generate` to see the column).
- `internal/db/migrations/000007_must_change_password.down.sql` — same.
- `internal/db/queries/users.sql` — `CreateUser` query updated to include `must_change_password` as column `$6` (was omitted before; relied on DB default of FALSE, which prevented seed from setting it TRUE).
- `internal/db/queries/tasks.sql` — added `SetTaskRetryAfterAndTags` and `ListRetryReadyTasks` queries (these were missing from the query file; `task_repository.go` referenced them causing a build failure).
- `sqlc generate` run via Docker (`sqlc/sqlc:1.30.0`); all generated files are clean output.

### Database — `internal/db/user_repository.go` (modified)

- `Create`: now passes `MustChangePassword` in `CreateUserParams`.

### Database — `internal/db/task_repository.go` (modified)

- `ListByPipelineAndStatuses`: field name `Statuses` corrected to `Column2` after sqlc regeneration changed the struct field name.

### Frontend — `web/src/pages/ChangePasswordPage.tsx` (implemented)

Full implementation of the forced password change page:
- Three password fields: current, new, confirm.
- Client-side validation: length < 8 → inline error below new-password field; mismatch → inline error below confirm field.
- Submit button disabled until all fields non-empty.
- Loading state: button text → "Changing...", all inputs disabled.
- Server error mapping: 401 → "Current password is incorrect" below current-password field; 400 → length error below new-password field; other → network error below button.
- On success: calls `logout()` to clear auth context, then `navigate('/login', { replace: true })`.
- Style: matches NexusFlow design system (slate-900 background, indigo-600 button, slate labels).

### Frontend routing — no changes needed

`ProtectedRoute` was already correctly implemented by the scaffold with `allowMustChangePassword` prop. `App.tsx` routes were already correct. No changes required.

### Unit tests

| File | Tests |
|---|---|
| `api/handlers_password_change_test.go` | 8 tests: 204 happy path, 401 wrong current pw, 400 too short, 400 boundary-exact (8 chars accepted), all sessions invalidated (AC-6), 403 missing session, 400 malformed body, 400 empty fields |
| `internal/auth/auth_test.go` | 4 new tests: 403 on protected endpoint with flagged session, change-password endpoint exempt, logout exempt, normal session passes through |
| `web/src/pages/ChangePasswordPage.test.tsx` | 11 tests: form renders, button disabled/enabled, client-side validation (length + mismatch), 401 server error, 400 server error, API called with correct args, success redirect, loading state disables inputs |

### Acceptance tests

| File | Tests |
|---|---|
| `tests/acceptance/SEC-001-acceptance.sh` | Full bash script for all 7 backend ACs against a running stack |
| `tests/acceptance/SEC-001-change-password-page.test.tsx` | 10 frontend tests covering all 9 listed ACs |

---

## TDD Cycle

**Red:** Wrote `handlers_password_change_test.go` before implementing the handler. The handler panicked with "not implemented".

**Green:** Implemented `ChangePassword` handler to make all 8 tests pass. Implemented `ChangePasswordPage` to make 11 frontend tests pass.

**Refactor:** Extracted `writeJSONError` as a named helper (replaces inline JSON writes). Extracted `isMustChangePasswordExempt` in auth middleware to make the intent explicit and testable. Updated all docstrings to reflect final behaviour.

---

## Test Results

| Suite | Files | Tests | Status |
|---|---|---|---|
| Go (api + internal) | 8 packages | All pass | GREEN |
| staticcheck | all packages | 0 issues | GREEN |
| Web (vitest) | 35 files | 691 pass | GREEN |

---

## Acceptance Criteria Mapping

| AC | Implementation |
|---|---|
| AC-1 | `seedAdminIfEmpty` sets `MustChangePassword=true`; `Login` carries flag into session + response |
| AC-2 | `auth.Middleware` — `isMustChangePasswordExempt` gate; 403 + `{"error":"password_change_required"}` |
| AC-3 | `ChangePassword` handler — `auth.VerifyPassword` → 401 on mismatch |
| AC-4 | `ChangePassword` handler — `len(req.NewPassword) < 8` → 400 |
| AC-5 | `users.ChangePassword` calls `UpdateUserPassword` which atomically sets `password_hash` + `must_change_password=FALSE` |
| AC-6 | `sessions.DeleteAllForUser(userID)` after successful change |
| AC-7 | Re-login with new password: `users.GetByUsername` returns cleared flag; new session has `MustChangePassword=false` |
| Frontend AC | `ChangePasswordPage`: renders 3 fields, validates, maps 401/400/success, disables during submit; `ProtectedRoute` redirect logic unchanged (already correct in scaffold) |

---

## Confirmations

1. `sqlc generate` was run via Docker (`sqlc/sqlc:1.30.0`); no hand-edited sqlc output remains.
2. Seed sets `must_change_password=TRUE` on the initial admin.
3. All sessions are invalidated via `DeleteAllForUser` on a successful password change.
4. Go tests, web tests, and staticcheck are all green.

---

## Deviations

None from the task specification. One scope addition: `SetTaskRetryAfterAndTags` and `ListRetryReadyTasks` SQL queries were added to `tasks.sql` to fix a pre-existing build failure introduced by the scaffold (the queries were referenced in `task_repository.go` but never added to the query file). The `ListTasksByPipelineAndStatusesParams.Statuses` field reference was also corrected to `Column2` after regeneration. These changes were required to compile; they do not change any existing behaviour.

---

## Limitations

- `/api/auth/me` is called by the frontend AuthProvider for session restore but is not implemented in the API server. The `MustChangePassword` middleware exempts it, so once it is implemented it will work correctly in the forced-change flow.
- The bash acceptance script (`SEC-001-acceptance.sh`) requires a running stack with a fresh database (so `seedAdminIfEmpty` runs). It also requires `jq`. Run with `API_BASE=http://localhost:8080 bash tests/acceptance/SEC-001-acceptance.sh`.
