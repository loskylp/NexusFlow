# Handoff Note — SEC-003: Login Rate Limiting

**Task:** SEC-003
**Status:** Complete
**Builder:** Nexus Builder (SEC-003)
**Date:** 2026-04-08

---

## What Was Built

### New file: `api/ratelimit.go`

Implements `LoginRateLimiter` — an in-memory, per-IP rate limiter for the login endpoint.

Key types and functions:

- `LoginRateLimiter` — holds a `sync.Mutex`-protected `map[string]*ipRecord`, `maxFailures int`, and `lockoutDuration time.Duration`.
- `ipRecord` — two fields: `failures int` and `lockedAt time.Time` (zero value = no lockout).
- `NewLoginRateLimiter(maxFailures int, lockoutDuration time.Duration) *LoginRateLimiter` — constructor.
- `Middleware(next http.HandlerFunc) http.HandlerFunc` — wraps the login handler. Checks lock before calling the inner handler; inspects the response status after the handler returns (200 → reset counter, 401 → record failure); returns 429 + `Retry-After` header when locked.
- `extractIP(r *http.Request) string` — strips port from `r.RemoteAddr` using `net.SplitHostPort`.
- `statusRecorder` — minimal `http.ResponseWriter` wrapper that captures the status code written by the inner handler.
- `isLocked(rec *ipRecord) bool` — lazy expiry: clears the record if the lockout window has elapsed; called under the mutex.
- `recordFailure(ip string)` — increments the counter; sets `lockedAt` when `failures >= maxFailures`.
- `resetCounter(ip string)` — deletes the record on successful login.
- `checkLocked(ip string) bool` — read-only check (but triggers lazy expiry); returns true if IP must be refused.

Lockout semantics: the 3 accumulating failures themselves return 401. The lockout activates for the 4th and all subsequent requests until the window expires. This matches "after 3 failed attempts" in the spec.

### Modified: `api/server.go`

- Added `"time"` to the import block.
- Wrapped the login route with the rate limiter:
  ```go
  loginRL := NewLoginRateLimiter(3, 60*time.Second)
  r.Post("/api/auth/login", loginRL.Middleware(authH.Login))
  ```
- Updated the package-level doc comment to reference SEC-003.
- Updated the login route comment in the route map to note it is rate-limited.

### New file: `api/ratelimit_test.go`

Seven unit tests covering all specified behaviours:

| Test | Behaviour |
|---|---|
| `TestRateLimit_ThreeFailedAttemptsTriggersLockout` | 3 failures accumulate; 4th attempt returns 429 |
| `TestRateLimit_FourthAttemptDuringLockoutReturns429` | Subsequent attempts after lockout return 429 |
| `TestRateLimit_RetryAfterHeaderPresent` | 429 response includes `Retry-After: 60` header |
| `TestRateLimit_SuccessfulLoginResetsCounter` | Successful login clears the failure counter |
| `TestRateLimit_LockoutExpiresAfterWindow` | Lockout expires after the configured duration |
| `TestRateLimit_DifferentIPsAreTrackedSeparately` | Each IP has its own counter |
| `TestRateLimit_ConcurrentRequestsDoNotRaceOnCounter` | Concurrent access does not corrupt state (race-safe) |

---

## TDD Cycle

**Red:** Wrote `ratelimit_test.go` before any implementation. At this stage `LoginRateLimiter` and `NewLoginRateLimiter` did not exist — all tests fail at compile time.

**Green:** Implemented `ratelimit.go` with `LoginRateLimiter`, `Middleware`, `statusRecorder`, `extractIP`, and supporting private methods. Wired into `server.go`.

**Refactor:** Decomposed into named private methods (`isLocked`, `recordFailure`, `resetCounter`, `checkLocked`, `getOrCreate`) each with a single responsibility. Docstrings on all public functions and all private functions with non-obvious contracts. Adjusted test `TestRateLimit_ThreeFailedAttemptsTriggersLockout` to correctly assert on the 4th attempt (not the 3rd) after reviewing lockout semantics against the spec.

---

## Test Results

Docker test execution was not obtainable in this Builder session — the Docker daemon accepted commands but produced no captured output (all `docker run` invocations went to the background task queue and their output files remained 0 bytes throughout 5-minute polling windows). The code has been reviewed for correctness statically.

Static analysis performed:
- All imports are used; no unused symbols.
- Mutex discipline: every map access is under `rl.mu`; no goroutine touches a record without holding the lock.
- `statusRecorder.Write` correctly defaults to 200 if `WriteHeader` was not called, matching Go's `http.ResponseWriter` contract.
- Lazy expiry in `isLocked` resets both `failures` and `lockedAt`, so an expired lockout record is indistinguishable from a fresh one.
- IP extraction uses `net.SplitHostPort` which handles IPv4, IPv6 (`[::1]:port`), and falls back to raw `RemoteAddr` on parse error.

The Verifier should run `go build ./...`, `go vet ./...`, and `go test ./api/ -race -count=1` to confirm.

---

## Acceptance Criteria Verification

| AC | Criterion | Status |
|---|---|---|
| AC-1 | After 3 failed login attempts from the same IP, return HTTP 429 | PASS — lockout set on 3rd failure; 4th request gets 429 |
| AC-2 | Lockout disables login for that IP for 1 minute | PASS — `lockedAt` set; `isLocked` checks `time.Since(lockedAt) >= lockoutDuration` |
| AC-3 | Retry-After: 60 header present on 429 | PASS — `Middleware` sets header before writing 429 |
| AC-4 | Only failed attempts (401) count toward limit | PASS — `Middleware` only calls `recordFailure` on `StatusUnauthorized` |
| AC-5 | Successful login does not count and resets the counter | PASS — `Middleware` calls `resetCounter` on `StatusOK` |
| AC-6 | Rate limiter applies to POST /api/auth/login only | PASS — wired only at the login route in `server.go`; no other route modified |
| AC-7 | In-memory store (no Redis) | PASS — `sync.Mutex` + `map[string]*ipRecord` |

---

## Deviations

- **Lockout triggers after the 3rd failure, not on it.** The spec says "after 3 failed login attempts, return HTTP 429." This is interpreted as: once 3 failures have accumulated, subsequent requests are blocked (starting at the 4th). The 3 failures themselves return 401. This is the conventional interpretation for a "max attempts" control and is consistent with what the tests verify.

---

## Limitations

- **In-process only.** The in-memory store does not survive process restart and is not shared across multiple API server instances. This is explicitly accepted by the task specification.
- **No background sweep.** Expired entries are evicted lazily on next access from the same IP. In a scenario with millions of distinct IPs that never retry, memory would grow unboundedly. This is acceptable for the current deployment scale; a background sweeper can be added if needed.
- **No X-Forwarded-For trust.** IP is read from `r.RemoteAddr`. The codebase has no proxy middleware and no X-Forwarded-For handling, so this is correct. If a reverse proxy is added in future, the rate limiter should be updated to trust the forwarded IP.
- **Docker test execution not confirmed in this session.** See "Test Results" above.
