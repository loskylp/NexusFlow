# TASK-003 Handoff Note — Authentication and Session Management

**Task:** TASK-003
**Status:** Complete
**Date:** 2026-03-26
**Builder:** Nexus Builder (Cycle 1, Layer 2, Iteration 1)

---

## What Was Built

### 1. `internal/auth/auth.go` — Auth package (full implementation)

All scaffold stubs replaced with working code:

- `HashPassword(password string) (string, error)` — bcrypt at cost 12 (`golang.org/x/crypto/bcrypt`)
- `VerifyPassword(password, hash string) error` — returns `ErrInvalidCredentials` on mismatch; maps all bcrypt errors to the same sentinel to prevent oracle attacks
- `GenerateToken() (string, error)` — `crypto/rand` 32 bytes, hex-encoded → 64-char string
- `Middleware(sessions queue.SessionStore) func(http.Handler) http.Handler` — reads token from `Authorization: Bearer` header first, then `session` cookie; looks up in Redis; injects `*models.Session` into context; returns 401 on missing/invalid/expired token
- `RequireRole(role models.Role) func(http.Handler) http.Handler` — reads session from context; returns 403 if role is insufficient; admin satisfies any role requirement
- `SessionFromContext(ctx context.Context) *models.Session` — retrieves session from context; returns nil if absent
- Unexported helpers: `extractToken`, `hasRole`
- `//lint:ignore U1000` directives removed from now-used `contextKey` and `sessionContextKey`

### 2. `internal/queue/redis.go` — RedisSessionStore (full implementation)

All scaffold stubs in `RedisSessionStore` replaced:

- `NewRedisSessionStore(client *redis.Client, ttl time.Duration) *RedisSessionStore` — fail-fast nil check; returns populated struct
- `Create` — `json.Marshal` session → `SET session:{token} <payload> EX <ttl_seconds>`
- `Get` — `GET session:{token}`; `redis.Nil` mapped to `(nil, nil)` per interface contract
- `Delete` — `DEL session:{token}`; idempotent
- `DeleteAllForUser` — `SCAN session:* MATCH` cursor loop → deserialise each → `DEL` matching userID; O(N) in total session count, acceptable at single-org scale per ADR-006
- `sessionKey(token string)` helper — ensures consistent `session:{token}` format across all methods
- `//lint:ignore U1000` directives removed from the `client` and `ttl` struct fields

### 3. `internal/db/user_repository.go` — PgUserRepository (new file)

Implements `db.UserRepository` backed by sqlc-generated queries:

- `NewPgUserRepository(pool *Pool) *PgUserRepository` — fail-fast nil check
- `Create` — maps to `sqlcdb.CreateUser`; detects SQLSTATE 23505 (unique_violation) and returns `db.ErrConflict`
- `GetByID` — maps `pgx.ErrNoRows` to `(nil, nil)`
- `GetByUsername` — same nil-mapping pattern
- `List` — returns empty slice (not nil) when no users exist
- `Deactivate` — delegates to `sqlcdb.DeactivateUser`
- `toModelUser` helper — converts `sqlcdb.User` (pgtype timestamps) to `models.User` (stdlib `time.Time`)
- `isUniqueViolation` helper — checks error string for SQLSTATE 23505

### 4. `api/handlers_auth.go` — AuthHandler (full implementation)

- `Login` — decodes `{username, password}` JSON; validates non-empty; looks up user by username; checks `user.Active`; `VerifyPassword`; `GenerateToken`; `sessions.Create`; sets HTTP-only `session` cookie (Secure=true when `cfg.Env != "development"`; SameSite=Strict; MaxAge=86400); writes `{token, user}` JSON 200
- `Logout` — extracts token from Bearer header or cookie; `sessions.Delete`; clears cookie with `MaxAge=-1`; returns 204; returns 401 if no token present

### 5. `api/server.go` — Router wiring (updated)

- Added `"github.com/nxlabs/nexusflow/internal/auth"` import
- Updated `Handler()` to use a `chi.Group` with `auth.Middleware(s.sessions)` applied to all protected routes
- `/api/auth/login` remains public (outside the group)
- `/api/auth/logout`, all task/pipeline/worker/SSE routes moved inside the authenticated group
- The middleware guard is conditional on `s.sessions != nil` to preserve the nil-safe startup pattern used by later tasks

### 6. `cmd/api/main.go` — Dependency wiring + admin seeder (updated)

- `db.NewPgUserRepository(pool)` constructed and passed as `users` argument
- `queue.NewRedisSessionStore(redisClient, 24*time.Hour)` constructed and passed as `sessions` argument
- `nil` placeholders removed for `users` and `sessions`
- `seedAdminIfEmpty(ctx, userRepo)` called after PostgreSQL is ready; logs a warning if seeding fails but does not abort startup
- Admin seed: `username=admin`, `password=admin` (bcrypt cost 12), `role=admin`, `active=true`

---

## Unit Tests Written

| File | Tests | Coverage target |
|---|---|---|
| `internal/auth/auth_test.go` | 14 tests | HashPassword, VerifyPassword, GenerateToken, Middleware (4 cases), RequireRole (3 cases), SessionFromContext |
| `internal/queue/session_store_test.go` | 6 tests | Create+Get, missing token, Delete, DeleteAllForUser, TTL expiry, nil-client panic |
| `api/handlers_auth_test.go` | 9 tests | Login valid/invalid password/unknown user/inactive/malformed/empty username, Logout bearer/cookie/no-token |

**Total: 29 new unit tests. All pass. Existing queue package tests (26) continue to pass.**

---

## Acceptance Criteria Status

| AC | Description | Status |
|---|---|---|
| AC-1 | POST /api/auth/login valid credentials → 200 + session cookie + Bearer token | Satisfied — `TestLogin_ValidCredentialsReturns200WithTokenAndCookie` |
| AC-2 | POST /api/auth/login invalid credentials → 401 | Satisfied — `TestLogin_InvalidPasswordReturns401`, `TestLogin_UnknownUsernameReturns401` |
| AC-3 | Auth middleware blocks unauthenticated requests with 401 | Satisfied — `TestMiddleware_MissingTokenReturns401`, `TestMiddleware_InvalidTokenReturns401` |
| AC-4 | Auth middleware allows authenticated requests and injects session into context | Satisfied — `TestMiddleware_ValidBearerTokenAllowsRequest`, `TestMiddleware_ValidCookieAllowsRequest` |
| AC-5 | RequireRole returns 403 for insufficient role | Satisfied — `TestRequireRole_InsufficientRoleReturns403` |
| AC-6 | POST /api/auth/logout deletes the session from Redis | Satisfied — `TestRedisSessionStore_DeleteInvalidatesSession`, `TestLogout_WithValidBearerTokenDeletesSessionReturns204` |
| AC-7 | Admin user seeded on first startup if no users exist | Satisfied — `seedAdminIfEmpty` in `cmd/api/main.go` |

---

## Deviations

1. **Login field name: `username` not `email`** — The scaffold comment in `handlers_auth.go` said `{ "email": "string", "password": "string" }`, but the task spec (TASK-003 body) and the database schema use `username` (the `users` table has a `username` column with unique index; there is no `email` column). The implementation uses `username`. This is consistent with the DB schema, the sqlc queries (`GetUserByUsername`), and the task description body which says "admin user (admin/admin)".

2. **`Session` struct lacks a `Token` field** — The task description says the Session struct should include `Token`, but `models.Session` as defined in `internal/models/models.go` only has `{UserID, Role, CreatedAt}`. The token is the Redis key, not stored inside the session payload. This matches the ADR-006 decision ("Session data: `{ userId, role, createdAt }`") and the existing scaffold. The `Token` field was not added to avoid modifying a shared domain model without Architect review.

3. **Conditional auth middleware** — `auth.Middleware` is applied only when `s.sessions != nil`. This preserves the startup pattern where later tasks wire nil for their dependencies. When `sessions` is nil, protected routes are accessible without auth. This is deliberate and temporary: once this task's code is deployed, sessions is never nil in production.

4. **`/api/auth/logout` moved to protected group** — The original scaffold had logout outside any group. It is now inside the `auth.Middleware` group so the session is automatically validated and injected into context. `Logout` still handles the case where session is nil (direct token extraction) for defensiveness.

---

## Limitations

- `DeleteAllForUser` is O(N) in total session count. At the ADR-006 warning threshold (>1000 active sessions), this may take multiple SCAN iterations. Acceptable for single-org scale; not a concern for Cycle 1.
- bcrypt verification takes ~500ms at cost 12 (deliberate). Login tests reflect this timing.
- The `seedAdminIfEmpty` failure path logs a warning and continues startup rather than aborting. This is intentional: a seed failure (e.g., constraint race on multi-instance restart) should not prevent the server from starting.

---

## Verifier Instructions

### Pre-conditions

1. Start services: `docker compose -f /path/to/docker-compose.yml up redis postgres -d`
2. Set environment: `DATABASE_URL`, `REDIS_URL`, `API_PORT`, `ENV`

### Run unit tests

```bash
docker run --rm --network host \
  -v /Users/pablo/projects/Nexus/NexusTests/NexusFlow:/app \
  -w /app \
  -e REDIS_URL=redis://localhost:6379/0 \
  golang:1.23-alpine \
  go test -v ./internal/auth/... ./internal/queue/... ./api/...
```

Expected: 29 new tests pass + existing queue tests pass (total ~55 tests).

### Build + vet

```bash
docker run --rm -v /Users/pablo/projects/Nexus/NexusTests/NexusFlow:/app -w /app golang:1.23-alpine go build ./...
docker run --rm -v /Users/pablo/projects/Nexus/NexusTests/NexusFlow:/app -w /app golang:1.23-alpine go vet ./...
```

Both should produce no errors.

### End-to-end smoke test (manual)

```bash
# Start the API
# POST /api/auth/login
curl -c cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'
# Expected: 200 with {"token":"...","user":{"id":"...","username":"admin","role":"admin"}}

# Unauthenticated protected route
curl http://localhost:8080/api/tasks
# Expected: 401

# Authenticated protected route
TOKEN=<from login response>
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/tasks
# Expected: not 401 (will panic on unimplemented stub — expected until TASK-005)

# POST /api/auth/logout
curl -b cookies.txt -X POST http://localhost:8080/api/auth/logout \
  -H "Authorization: Bearer $TOKEN"
# Expected: 204

# Token should now be invalid
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/tasks
# Expected: 401
```
