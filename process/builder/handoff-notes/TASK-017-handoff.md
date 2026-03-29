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

# Builder Handoff — TASK-017
**Date:** 2026-03-29
**Task:** Admin user management
**Requirement(s):** REQ-020

## What Was Implemented

### New file: `api/handlers_users.go`
- `UserHandler` — handler type that holds a reference to the Server.
- `CreateUser` — `POST /api/users`: parses JSON body, validates username/password/role, bcrypt-hashes the password at cost 12 (same as login), inserts via `UserRepository.Create`, returns 201 with `userResponse` (no password hash).
- `ListUsers` — `GET /api/users`: calls `UserRepository.List`, returns 200 with slice of `userResponse`.
- `DeactivateUser` — `PUT /api/users/{id}/deactivate`: validates UUID path param, checks user exists (404 if not), calls `UserRepository.Deactivate`, then calls `SessionStore.DeleteAllForUser` to invalidate all active sessions. Returns 204.
- `userResponse` — public projection of `models.User` that explicitly excludes `PasswordHash`.
- `buildUserRoutes` — helper used in tests to construct an isolated chi router with `auth.Middleware` + `auth.RequireRole("admin")` applied, mirroring the production middleware stack.

### Modified file: `api/server.go`
- Added `models` import (needed for `models.RoleAdmin`).
- Registered the three user management routes inside a new `admin` sub-group within the existing `protected` chi group. The sub-group applies `auth.RequireRole(models.RoleAdmin)` so only authenticated admins can reach these endpoints.
- Updated the route map comment to include the three new routes.

### Modified file: `api/handlers_auth_test.go`
- Added `db` import.
- Updated `stubUserRepo.Create` to return `db.ErrConflict` when the username already exists, matching the real `PgUserRepository` contract and enabling the 409 test.

### New file: `api/handlers_users_test.go`
- 15 unit tests covering all three handlers and the middleware enforcement path.

### Pre-existing (no changes needed)
- `internal/db/repository.go` — `UserRepository` interface already declared `Create`, `List`, `Deactivate`, and `GetByID`.
- `internal/db/user_repository.go` — `PgUserRepository` already implemented all four methods.
- `internal/db/queries/users.sql` and `internal/db/sqlc/users.sql.go` — all required queries (`CreateUser`, `ListUsers`, `DeactivateUser`) already present.
- `internal/queue/redis.go` — `RedisSessionStore.DeleteAllForUser` already implemented.
- `api/handlers_auth.go` — login guard for inactive users (`if !user.Active`) was already present; no change required.

## Unit Tests
- Tests written: 15
- All passing: yes
- Key behaviors covered:
  - Admin can create a user; response contains id, username, role, active, createdAt; password hash is not present.
  - Validation rejects empty username, empty password, invalid role.
  - Duplicate username returns 409.
  - Admin can list all users; password hash absent from every item.
  - Empty user store returns an empty array (not null).
  - Admin can deactivate a user; user.Active becomes false; victim's Redis session is deleted.
  - Deactivating a non-existent user returns 404.
  - Non-UUID path parameter returns 400.
  - Non-admin authenticated caller receives 403 on all three endpoints.
  - Unauthenticated caller receives 401 on all three endpoints.

## Deviations from Task Description
None. All acceptance criteria map directly to implemented behavior and passing tests.

## Known Limitations
- `DeleteAllForUser` is O(N) in the total session count (SCAN-based). The existing implementation includes an ADR-006 warning threshold of >1000 active sessions. This is the pre-existing design decision, not introduced by this task.
- Session invalidation errors are logged but do not cause the 204 response to change to 500. The user is already deactivated in the database at that point, so future login attempts are blocked regardless of stale Redis entries. This matches the fail-safe direction: deactivation is durable; session expiry is eventually consistent.

## For the Verifier
- Acceptance criterion 4 ("After deactivation, the user's existing sessions are immediately invalidated — returns 401") requires a live Redis instance to verify end-to-end. The unit test proves the stub session store is cleared; integration verification should use a real Redis.
- Acceptance criterion 6 ("Deactivated user's previously submitted tasks continue executing — not cancelled") is a domain invariant, not enforced by code in this task. No code path in DeactivateUser touches the tasks table. The Verifier can confirm by inspecting the deactivate handler — it calls only `Deactivate` and `DeleteAllForUser`.
- The login guard for deactivated users (`if !user.Active { return 401 }`) was already implemented in `handlers_auth.go` before this task. No code change was made there; the existing test `TestLogin_InactiveUserReturns401` covers it.
