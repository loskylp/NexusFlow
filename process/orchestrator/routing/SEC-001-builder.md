# Routing Instruction — Builder — SEC-001

**From:** Orchestrator
**To:** @nexus-builder
**Date:** 2026-04-15
**Cycle:** 4
**Task:** SEC-001 -- Password change endpoint + UI + mandatory first-login change

---

## Objective

Remediate Sentinel finding SEC-001 (HIGH): default admin credentials (admin/admin) with
no forced password change. The remediation consists of three welded parts:

1. A backend password-change endpoint (`POST /api/auth/change-password`).
2. A frontend ChangePasswordPage that forces the change flow on first login.
3. Auth middleware enforcement: sessions carrying `MustChangePassword=true` are refused
   access to every protected endpoint except the change-password endpoint itself.

The scaffold already laid down the full surface (handler stub, React page stub, model
field, migration, middleware hook, `UpdateUserPassword` sqlc query, `ChangePassword`
repository method, router wiring, acceptance test stubs). Your job is to fill in the
bodies, run `sqlc generate`, wire the seed path to set `must_change_password = TRUE` on
the default admin, and make the 7 acceptance steps pass.

## Acceptance Criteria

From `tests/acceptance/SEC-001-acceptance.sh` -- these are the canonical ACs:

- **AC-1:** Admin seed user is created with `must_change_password = TRUE`; first login
  as admin/admin succeeds (returns a session) but the session is flagged.
- **AC-2:** Any protected endpoint (e.g. `GET /api/workers`) called with a
  `must_change_password` session returns **403** with body
  `{"error": "password_change_required"}`. Only `POST /api/auth/change-password` is
  exempt.
- **AC-3:** `POST /api/auth/change-password` with an **incorrect current password**
  returns **401**.
- **AC-4:** `POST /api/auth/change-password` with a new password **shorter than 8
  characters** returns **400**.
- **AC-5:** `POST /api/auth/change-password` with valid current password and valid new
  password returns **204**; the user's `must_change_password` is cleared atomically
  with the password hash update.
- **AC-6:** After a successful password change, the **old session is invalidated** --
  subsequent requests with the old session token return **401**. All other sessions for
  that user are also invalidated.
- **AC-7:** Re-login with the new password succeeds; subsequent calls to protected
  endpoints return **200** (the flag is cleared).

Frontend acceptance (`tests/acceptance/SEC-001-change-password-page.test.tsx`):

- ChangePasswordPage renders three fields (current, new, confirm), validates
  client-side, shows a spinner on submit, maps 401 -> inline "current password is
  incorrect", 400 -> inline length error, 204 -> redirect to `/login`.
- When `user.mustChangePassword === true`, all other routes redirect to
  `/change-password` (via `ProtectedRoute.allowMustChangePassword`). When the flag is
  cleared, visiting `/change-password` redirects away.

## Required Documents

- Scaffold manifest (SEC-001 surface map): [process/scaffolder/scaffold-manifest.md](../../scaffolder/scaffold-manifest.md) -- lines 41-73 (files) and 484-565 (component contracts)
- Sentinel finding: [process/sentinel/cycle-3-security-report.md](../../sentinel/cycle-3-security-report.md) -- SEC-001 entry, remediation condition (lines 19-24)
- Backend acceptance test: [tests/acceptance/SEC-001-acceptance.sh](../../../tests/acceptance/SEC-001-acceptance.sh)
- Frontend acceptance test: [tests/acceptance/SEC-001-change-password-page.test.tsx](../../../tests/acceptance/SEC-001-change-password-page.test.tsx)
- Handler stub: [api/handlers_password_change.go](../../../api/handlers_password_change.go)
- Page stub: [web/src/pages/ChangePasswordPage.tsx](../../../web/src/pages/ChangePasswordPage.tsx)
- Middleware hook: [internal/auth/auth.go](../../../internal/auth/auth.go)
- DB layer: [internal/db/repository.go](../../../internal/db/repository.go), [internal/db/user_repository.go](../../../internal/db/user_repository.go), [internal/db/queries/users.sql](../../../internal/db/queries/users.sql), [internal/db/sqlc/models.go](../../../internal/db/sqlc/models.go), [internal/db/sqlc/users.sql.go](../../../internal/db/sqlc/users.sql.go)
- Migration: [internal/db/000007_must_change_password.up.sql](../../../internal/db/000007_must_change_password.up.sql), [internal/db/000007_must_change_password.down.sql](../../../internal/db/000007_must_change_password.down.sql)
- Session model: [internal/models/models.go](../../../internal/models/models.go) -- Session, User
- Rate-limiting precedent (SEC-003): [process/builder/handoff-notes/SEC-003-handoff.md](../../builder/handoff-notes/SEC-003-handoff.md) -- follow the same "security remediation as a first-class handler" pattern
- Router: [api/server.go](../../../api/server.go) -- `/api/auth/change-password` route already wired to the middleware exemption
- Frontend guard: [web/src/components/ProtectedRoute.tsx](../../../web/src/components/ProtectedRoute.tsx), [web/src/App.tsx](../../../web/src/App.tsx)

## Dependencies (all satisfied)

- TASK-003 -- Authentication and session management (COMPLETE Cycle 1; bcrypt cost 12, session store, RequireAuth middleware)
- TASK-017 -- Admin user management (COMPLETE Cycle 2; UserRepository, role model)
- Scaffold v3 (2026-04-09) -- full SEC-001 surface in place

## Reminders

- **sqlc regeneration:** The scaffold hand-edited `internal/db/sqlc/models.go` and
  `users.sql.go` and added `UpdateUserPassword` to the query file. Run `sqlc generate`
  as the first Builder step and reconcile. Do **not** leave hand-edited sqlc output in
  the final commit.
- **Atomic clear-and-hash:** `must_change_password = FALSE` and the new `password_hash`
  must be written in the same `UPDATE` statement. Do not split into two round trips --
  a crash between them leaves a user locked out or unflagged.
- **Session invalidation on change:** After a successful password change, invalidate
  **all** sessions for the user (not just the current one). Reuse the session-store
  deletion path from TASK-017 admin deactivation. This is AC-6.
- **Middleware exemption scope:** Only `POST /api/auth/change-password` is exempt from
  the `MustChangePassword` block. `/api/auth/logout` and `/api/auth/me` should **also**
  be exempt (the frontend needs them during the forced flow) -- confirm by reading the
  scaffold middleware hook; if not exempt there, add them to the allowlist.
- **Seed flag:** `seedAdminIfEmpty` must set `must_change_password = TRUE` on the
  initial admin. Verify the seed path writes the flag (AC-1 depends on this).
- **Password length only:** AC-4 is length-only (>=8). Do not over-implement SEC-007
  (complexity) in this task -- it is tracked separately as a MEDIUM finding already
  accepted by the Nexus.
- **Frontend redirect semantics:** `ProtectedRoute.allowMustChangePassword` is a prop,
  not a new route wrapper. Routes that need to be reachable while flagged opt in with
  the prop; everything else redirects to `/change-password`. See the scaffold for the
  exact pattern.
- **Error shape:** The 403 body must be exactly `{"error": "password_change_required"}`
  -- the frontend keys on this string.
- **CI hygiene:** TASK-034 tripped on `staticcheck SA4006` (dead assignments). Run
  `staticcheck ./...` locally before pushing. Run the full Go test suite plus the web
  test suite. Scaffolded SEC-001 files had TODO bodies -- ensure no TODOs remain in
  shipped code.
- **No rate-limit interaction:** The SEC-003 login rate limiter applies to
  `POST /api/auth/login`, not to `/api/auth/change-password`. Do not add rate limiting
  to the change-password path in this task.
- Commit working increments. Report final commit SHA, acceptance pass summary, and
  explicit confirmation that (1) `sqlc generate` was run, (2) seed sets the flag,
  (3) old sessions are invalidated on change, (4) CI is green.

## Exit Criteria for Your Handoff

- All 7 backend acceptance steps in `SEC-001-acceptance.sh` pass against a running stack.
- Frontend acceptance test `SEC-001-change-password-page.test.tsx` passes.
- `sqlc generate` run; generated files match queries; no hand-edited sqlc output remains.
- Web + Go CI green (including `staticcheck`).
- No TODO stubs remain in the SEC-001 surface.
- Final commit SHA reported.

---

**Next:** Invoke @nexus-orchestrator -- on completion, report commit SHA, acceptance
summary, sqlc regeneration confirmation, and CI run ID so Verifier can be dispatched.
