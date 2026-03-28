# Security Review -- Cycle 1

**Project:** NexusFlow
**Profile:** Critical
**Reviewer:** Sentinel
**Date:** 2026-03-27
**Cycle:** 1 (14 tasks)
**Mode:** Code-level review (pre-staging deployment -- no live environment available for black-box testing)

---

## Verdict: PASS WITH CONDITIONS

Cycle 1 delivers a walking skeleton with a sound security foundation. No Critical-severity findings were identified. Three High-severity findings require remediation before production. The conditions below must be satisfied before Go-Live (v1.0.0); they do not block Cycle 1 Demo Sign-off because the system is not yet exposed to external users and the staging environment is access-restricted.

**Conditions for Demo Sign-off clearance:**
1. All HIGH findings must be tracked as backlog items with resolution deadlines no later than end of Cycle 3 (pre-Go-Live).
2. OBS-005 (npm audit vulnerabilities) must be evaluated and either resolved or documented as accepted risk.

---

## Summary of Findings

| ID | Severity | Title | Status |
|---|---|---|---|
| SEC-001 | HIGH | Default admin credentials (admin/admin) with no forced password change | Open |
| SEC-002 | HIGH | Redis instance has no authentication (requirepass not set) | Open |
| SEC-003 | HIGH | No rate limiting on login endpoint -- brute force viable | Open |
| SEC-004 | MEDIUM | No request body size limit -- potential denial of service | Open |
| SEC-005 | MEDIUM | No CORS policy configured -- cross-origin requests unrestricted | Open |
| SEC-006 | MEDIUM | No security headers (HSTS, X-Frame-Options, CSP, X-Content-Type-Options) | Open |
| SEC-007 | MEDIUM | No password complexity/length validation on login input | Open |
| SEC-008 | LOW | Health endpoint exposes infrastructure component status to unauthenticated callers | Open |
| SEC-009 | LOW | Session token logged in error messages (SessionStore.Create, SessionStore.Get) | Open |
| SEC-010 | LOW | Docker Compose dev environment exposes Redis (6379) and PostgreSQL (5432) on host ports | Open |
| SEC-011 | INFO | Staging Watchtower docker.sock mount (OBS-024) -- accepted risk for staging | Acknowledged |
| SEC-012 | INFO | SSE endpoints pass session by reference without nil check at handler level | Acknowledged |
| SEC-013 | INFO | Conditional auth middleware nil-safety pattern (OBS-013) | Acknowledged |

---

## Detailed Findings

### SEC-001: Default admin credentials with no forced password change

**Severity:** HIGH
**Category:** Authentication (OWASP A07:2021 -- Identification and Authentication Failures)
**Affected files:** `cmd/api/main.go` (seedAdminIfEmpty, line 166-195)

**Description:**
The API server seeds an admin user with username `admin` and password `admin` when the database is empty. The log message says "change password immediately in production" but there is no mechanism to enforce this. There is no user management endpoint to change passwords (planned for later cycles). An attacker who discovers a NexusFlow deployment before the operator changes the password has full admin access.

**Risk:**
An attacker with network access to the API can authenticate as admin with trivially guessable credentials and gain full system control: view all tasks, all pipelines, all worker data, and submit arbitrary tasks.

**Recommendation:**
1. Before Go-Live: implement a password change endpoint (already planned as part of user management).
2. Before Go-Live: either generate a random admin password at seed time and print it once to stdout, or require the admin password to be provided via environment variable (e.g., `ADMIN_SEED_PASSWORD`).
3. If the seed must remain `admin/admin` for development convenience, add a startup check that refuses to start in `staging` or `production` environments with the default password hash.

**Deferral acceptable for Cycle 1:** Yes -- staging is access-restricted and the seeded password is documented. Must be resolved before Go-Live.

---

### SEC-002: Redis instance has no authentication

**Severity:** HIGH
**Category:** Infrastructure Security (OWASP A05:2021 -- Security Misconfiguration)
**Affected files:** `docker-compose.yml` (lines 96-111), `deploy/staging/docker-compose.yml` (lines 124-140)

**Description:**
Both the development and staging Redis instances run without `--requirepass`. The Redis URL is `redis://redis:6379` with no password. While Redis is on an internal Docker network (not exposed to the internet in staging), any container or service on the same Docker network can connect to Redis without authentication.

The Redis instance stores:
- Session tokens (session:{token}) -- full session data including userID and role
- Task queue messages (queue:{tag}) -- task execution payloads
- Worker heartbeats (workers:active) -- worker liveness data
- SSE event distribution (Pub/Sub channels)

**Risk:**
A compromised container on the internal network can:
- Read or delete any user session (session hijacking or mass logout)
- Inject malicious messages into task queues (queue poisoning)
- Manipulate worker heartbeat scores (mark workers as down)
- Subscribe to all Pub/Sub channels (information disclosure)

**Recommendation:**
1. Add `--requirepass ${REDIS_PASSWORD}` to the Redis server command in both compose files.
2. Update `REDIS_URL` to include the password: `redis://:${REDIS_PASSWORD}@redis:6379`.
3. Add `REDIS_PASSWORD` to `.env.example` and `deploy/staging/.env.example`.
4. For staging: use a strong, randomly generated password.

**Deferral acceptable for Cycle 1:** Yes -- Redis is on an internal Docker network in both dev and staging. Must be resolved before Go-Live.

---

### SEC-003: No rate limiting on login endpoint

**Severity:** HIGH
**Category:** Authentication (OWASP A07:2021 -- Identification and Authentication Failures)
**Affected files:** `api/server.go` (line 113), `api/handlers_auth.go` (Login handler)

**Description:**
The `POST /api/auth/login` endpoint has no rate limiting. An attacker can submit unlimited login attempts per second. Combined with SEC-001 (default credentials), this makes brute-force attacks trivially fast even against non-default passwords.

The bcrypt cost factor of 12 provides some server-side slowdown (~250ms per attempt), but an attacker can parallelize requests. At 100 concurrent requests, an attacker can test ~400 passwords/second.

**Risk:**
Credential brute-forcing against any user account. The system provides no account lockout, no progressive delays, and no alerting on failed login attempts.

**Recommendation:**
1. Add IP-based rate limiting to the login endpoint (e.g., 5 attempts per IP per minute).
2. Consider account-level lockout after N consecutive failures (e.g., 10 failures triggers a 15-minute lockout).
3. Log failed login attempts with source IP for audit trail (currently only the username is logged on DB errors, not on credential failures).

**Deferral acceptable for Cycle 1:** Yes -- staging has restricted network access. Must be resolved before Go-Live. Rate limiting is noted in the release map as a Cycle 3 concern.

---

### SEC-004: No request body size limit

**Severity:** MEDIUM
**Category:** Availability (OWASP A05:2021 -- Security Misconfiguration)
**Affected files:** `api/server.go`, `api/handlers_auth.go`, `api/handlers_tasks.go`, `api/handlers_pipelines.go`

**Description:**
No handler uses `http.MaxBytesReader` or any equivalent to limit the size of incoming request bodies. The `json.NewDecoder(r.Body).Decode(&req)` pattern in all handlers will read the entire request body into memory.

**Risk:**
An authenticated attacker (or an unauthenticated attacker on the login endpoint) can send arbitrarily large request bodies to exhaust API server memory. A single request with a multi-GB body can crash the server process.

**Recommendation:**
Add a middleware or per-handler body size limit. A reasonable default is 1MB for API endpoints:
```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
```
Alternatively, use chi middleware to apply a global limit.

---

### SEC-005: No CORS policy configured

**Severity:** MEDIUM
**Category:** Access Control (OWASP A01:2021 -- Broken Access Control)
**Affected files:** `api/server.go`

**Description:**
The API server does not set any CORS headers. The configuration comments (`Env` field) mention "CORS policy" but no CORS middleware is applied. In the staging deployment, the web frontend and API are served through Traefik on the same domain (`nexusflow.staging.nxlabs.cc`), which mitigates cross-origin issues for the standard deployment.

However, if the API is ever accessed from a different origin (e.g., during development with `localhost:3000` frontend and `localhost:8080` API), browsers will block requests. More importantly, the absence of CORS headers means the API does not actively restrict which origins can make credentialed requests.

**Risk:**
Without explicit CORS restrictions, if a user visits a malicious website while logged in to NexusFlow, the malicious site can make cross-origin requests to the NexusFlow API. The `SameSite=Strict` cookie setting mitigates cookie-based CSRF for GET-initiated flows, but Bearer token authentication in JavaScript is not protected by SameSite.

**Recommendation:**
Add CORS middleware that explicitly allows only the expected frontend origin(s):
- Development: `http://localhost:3000`
- Staging: `https://nexusflow.staging.nxlabs.cc`
- Production: the production domain

---

### SEC-006: No security headers

**Severity:** MEDIUM
**Category:** Security Misconfiguration (OWASP A05:2021)
**Affected files:** `api/server.go`, `nginx.conf`

**Description:**
Neither the Go API server nor the nginx frontend configuration sets standard security headers:
- `Strict-Transport-Security` (HSTS)
- `X-Frame-Options` or `Content-Security-Policy: frame-ancestors`
- `X-Content-Type-Options: nosniff`
- `Content-Security-Policy`
- `Referrer-Policy`

**Risk:**
- Without HSTS: users may be downgraded from HTTPS to HTTP on first visit (Traefik provides TLS but does not guarantee HSTS).
- Without X-Frame-Options: the application can be embedded in iframes (clickjacking).
- Without X-Content-Type-Options: browsers may MIME-sniff responses.

**Recommendation:**
Add a middleware in the Go API server and appropriate headers in `nginx.conf`:
```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Strict-Transport-Security: max-age=31536000; includeSubDomains
Referrer-Policy: strict-origin-when-cross-origin
```

---

### SEC-007: No password complexity validation

**Severity:** MEDIUM
**Category:** Authentication (OWASP A07:2021)
**Affected files:** `api/handlers_auth.go` (Login handler, line 81)

**Description:**
The login handler checks that `username` and `password` are non-empty but does not enforce any password length or complexity requirements. This applies to the seed password and (when user management is implemented) to user-created passwords.

Note: bcrypt has a 72-byte input limit. Passwords longer than 72 bytes are silently truncated, which means extremely long passwords do not provide additional security.

**Risk:**
Users (especially the admin) can use trivially weak passwords (e.g., "a", "1"). Combined with the absence of rate limiting (SEC-003), weak passwords are rapidly brute-forceable.

**Recommendation:**
When user management / password change endpoints are implemented, enforce a minimum password length of 8 characters. For the admin seed, require a minimum of 12 characters when sourced from an environment variable.

---

### SEC-008: Health endpoint exposes infrastructure status

**Severity:** LOW
**Category:** Information Disclosure (OWASP A01:2021)
**Affected files:** `api/handlers_health.go`

**Description:**
The `GET /api/health` endpoint is unauthenticated and returns the connectivity status of both Redis and PostgreSQL. This tells an unauthenticated attacker whether the backend databases are reachable and which infrastructure components are degraded.

**Risk:**
Low. The information disclosed is limited ("ok" vs "error" per component). It helps an attacker understand the infrastructure but does not directly enable exploitation. The endpoint is standard practice for container orchestration health checks.

**Recommendation:**
Consider returning a simple 200/503 without component-level detail on the public endpoint. Expose the detailed breakdown on an internal-only endpoint or behind authentication.

---

### SEC-009: Session token included in log messages

**Severity:** LOW
**Category:** Sensitive Data Exposure (OWASP A02:2021)
**Affected files:** `internal/queue/redis.go` (lines 422, 437, 456)

**Description:**
The `RedisSessionStore` methods include the session token in error log messages via `fmt.Errorf` format strings:
```go
fmt.Errorf("queue.SessionStore.Create: SET session:%s: %w", token, err)
fmt.Errorf("queue.SessionStore.Get: GET session:%s: %w", token, err)
```

Session tokens are 256-bit cryptographic secrets. Including them in log output means they appear in container logs, centralized log aggregation, and potentially monitoring dashboards.

**Risk:**
An operator or attacker with access to log files can extract valid session tokens and impersonate users.

**Recommendation:**
Truncate or hash the token in log messages. For example, log only the first 8 characters: `session:%s...: %w`, `token[:8]`.

---

### SEC-010: Dev compose exposes database ports on host

**Severity:** LOW
**Category:** Security Misconfiguration
**Affected files:** `docker-compose.yml` (lines 109, 131)

**Description:**
The development `docker-compose.yml` maps Redis port 6379 and PostgreSQL port 5432 to the host. This is standard for development but means any process on the developer's machine can connect to these databases with no authentication (Redis) or default credentials (PostgreSQL: nexusflow/nexusflow_dev).

**Risk:**
Low in development context. Other processes or malware on the developer's machine can access the databases. The staging compose correctly does NOT expose these ports.

**Recommendation:**
No action required for development. Ensure staging and production never expose database ports externally. This is already correct in `deploy/staging/docker-compose.yml`.

---

### SEC-011: Watchtower docker.sock mount on staging (OBS-024)

**Severity:** INFO
**Category:** Infrastructure Security
**Affected files:** `deploy/staging/docker-compose.yml` (line 153)

**Description:**
Watchtower requires Docker socket access to monitor and redeploy containers. Mounting `/var/run/docker.sock` gives the Watchtower container root-equivalent access to the Docker daemon on the staging host. This is a known, documented risk (OBS-024).

**Assessment:**
This is standard practice for Watchtower deployments. The risk is limited to staging. The Watchtower container is from the official `containrrr/watchtower` image. Acceptable for staging; for production, consider Watchtower alternatives or image-based deployment triggers (e.g., CD pipeline SSH deploy).

---

### SEC-012: SSE handler nil check gap

**Severity:** INFO
**Category:** Robustness
**Affected files:** `api/handlers_sse.go` (lines 35, 51)

**Description:**
The SSE handlers `Tasks` and `Workers` call `auth.SessionFromContext(r.Context())` and pass the result directly to the broker without checking for nil. These handlers are behind the auth middleware which guarantees a non-nil session, but the absence of a defensive nil check means a misconfiguration (e.g., OBS-013's conditional middleware pattern) could lead to a nil pointer dereference and a 500 error rather than a clean 401.

**Assessment:**
The `chi.Recoverer` middleware will catch the panic and return 500, preventing a crash. The auth middleware is the contractual guarantee. This is a defense-in-depth observation, not an exploitable vulnerability.

---

### SEC-013: Conditional auth middleware pattern (OBS-013)

**Severity:** INFO
**Category:** Authentication bypass risk
**Affected files:** `api/server.go` (line 118)

**Description:**
The `if s.sessions != nil` guard on auth middleware means that if `sessions` is nil (e.g., during testing or misconfiguration), all "protected" routes become publicly accessible. The Verifier has already flagged this as OBS-013.

**Assessment:**
In production, `sessions` is always non-nil because `NewRedisSessionStore` panics on nil client. The risk is limited to test configurations. The pattern should be hardened in a later cycle by removing the nil check and always applying auth middleware.

---

## Auth Model Assessment

| Aspect | Assessment | Verdict |
|---|---|---|
| Password hashing | bcrypt cost 12 -- industry standard | PASS |
| Session token generation | crypto/rand, 256-bit, hex-encoded -- strong | PASS |
| Session storage | Redis with 24h TTL, server-side -- correct | PASS |
| Cookie security | HttpOnly, Secure (non-dev), SameSite=Strict, Path=/ | PASS |
| Token extraction | Bearer header preferred, cookie fallback -- correct dual-channel | PASS |
| Logout | Deletes Redis key + clears cookie -- complete | PASS |
| Role enforcement | RequireRole middleware with admin > user hierarchy | PASS |
| Session invalidation on deactivation | DeleteAllForUser scans all sessions -- O(N) but correct | PASS |

---

## Redis Access Pattern Assessment

| Aspect | Assessment | Verdict |
|---|---|---|
| Queue stream isolation | Per-tag streams (queue:{tag}) with single consumer group | PASS |
| Dead letter stream | Separate stream (queue:dead-letter) with reason field | PASS |
| Consumer group semantics | XREADGROUP with explicit XACK -- at-least-once delivery | PASS |
| Heartbeat mechanism | ZADD sorted set with Unix timestamp scores | PASS |
| Session key namespace | session:{token} -- no overlap with queue keys | PASS |
| Redis authentication | No requirepass set -- see SEC-002 | CONDITIONAL |

---

## API Security Assessment (OWASP Top 10 Coverage)

| OWASP Category | Assessment | Findings |
|---|---|---|
| A01: Broken Access Control | Pipeline CRUD enforces ownership. Task submission ties to session userID. SSE log/sink streams verify task ownership. Worker list is all-authenticated (by design). | PASS |
| A02: Cryptographic Failures | bcrypt cost 12, crypto/rand tokens, HTTPS via Traefik in staging. | PASS |
| A03: Injection | All database queries use sqlc-generated parameterized queries. No string concatenation in SQL. Redis commands use structured go-redis API (no raw command string building). | PASS |
| A04: Insecure Design | Session architecture is sound. Role model is simple and correct. Pipeline ownership is enforced at handler level. | PASS |
| A05: Security Misconfiguration | No Redis auth (SEC-002), no security headers (SEC-006), no CORS (SEC-005), no body size limits (SEC-004). | CONDITIONAL |
| A06: Vulnerable Components | See Dependency Audit below. | CONDITIONAL |
| A07: Auth Failures | Default creds (SEC-001), no rate limiting (SEC-003), no password policy (SEC-007). | CONDITIONAL |
| A08: Software/Data Integrity | Docker images built from source in CI. Watchtower uses image tags. No deserialization of untrusted data beyond JSON API payloads. | PASS |
| A09: Logging/Monitoring | Failed logins logged with username. Health monitoring via Uptime Kuma. Session tokens in logs (SEC-009). | CONDITIONAL |
| A10: SSRF | No server-side HTTP requests to user-controlled URLs in Cycle 1. Demo connectors are in-memory only. | PASS |

---

## Worker Isolation Assessment

| Aspect | Assessment | Verdict |
|---|---|---|
| Task assignment | Workers only consume from their configured tag streams | PASS |
| Cross-worker data access | Workers share the same PostgreSQL connection and can read any task by ID | CONDITIONAL -- by design (workers need to load pipeline defs and task input); no user-facing data leak |
| Worker registration | Self-registration with configurable ID -- no mutual authentication between worker and API | INFO -- acceptable for internal network |
| Task state transitions | Enforced by database trigger on task_state_log -- workers cannot skip states | PASS |
| Connector execution | Connectors receive only the pipeline config and task input -- no access to other tasks | PASS |

---

## Dependency Audit

### Go Dependencies (go.mod)

| Package | Version | License | Maintenance | CVEs | Verdict |
|---|---|---|---|---|---|
| github.com/go-chi/chi/v5 | v5.0.12 | MIT | Active (last release 2024) | None known | APPROVE |
| github.com/golang-migrate/migrate/v4 | v4.18.1 | MIT | Active | None known | APPROVE |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | Active (Google-maintained) | None known | APPROVE |
| github.com/jackc/pgx/v5 | v5.5.5 | MIT | Active (last release 2024) | None known | APPROVE |
| github.com/redis/go-redis/v9 | v9.5.1 | BSD-2-Clause | Active (official Redis client) | None known | APPROVE |
| golang.org/x/crypto | v0.27.0 | BSD-3-Clause | Active (Go team) | None known for v0.27.0 | APPROVE |
| golang.org/x/sync | v0.8.0 | BSD-3-Clause | Active (Go team) | None known | APPROVE |
| golang.org/x/text | v0.18.0 | BSD-3-Clause | Active (Go team) | None known | APPROVE |

**Go dependency summary:** All direct and indirect Go dependencies are from well-maintained, reputable sources. All use permissive licenses (MIT, BSD). No known CVEs at the pinned versions. Transitive dependency tree is minimal (go-redis and pgx pull in a few indirect deps, all low-risk). The Go module system with go.sum provides integrity verification.

### npm Dependencies (web/package.json)

| Package | Version Range | License | Category | Verdict |
|---|---|---|---|---|
| react | ^18.3.1 | MIT | Runtime | APPROVE |
| react-dom | ^18.3.1 | MIT | Runtime | APPROVE |
| react-router-dom | ^6.23.0 | MIT | Runtime | APPROVE |
| vite | ^5.2.10 | MIT | Dev-only | APPROVE |
| typescript | ^5.4.5 | Apache-2.0 | Dev-only | APPROVE |
| eslint | ^8.57.0 | MIT | Dev-only | APPROVE |
| vitest | ^4.1.2 | MIT | Dev-only | APPROVE |
| @vitejs/plugin-react | ^4.2.1 | MIT | Dev-only | APPROVE |
| @testing-library/* | various | MIT | Dev-only | APPROVE |
| jsdom | ^29.0.1 | MIT | Dev-only | APPROVE |
| esbuild | ^0.27.4 | MIT | Dev-only | APPROVE |

**npm dependency summary:** All runtime dependencies are from the React ecosystem -- well-maintained and widely used. All dev dependencies are standard tooling. Licenses are all MIT or Apache-2.0 -- fully compatible.

**OBS-005 (npm audit vulnerabilities):** The Verifier flagged 2 moderate vulnerabilities during TASK-001 verification. These are in dev-only dependencies (build tooling) and do not affect the production bundle. They should be reviewed when upgrading dev dependencies but do not represent a production risk.

**Recommendation:** Run `npm audit` in CI and address any moderate+ vulnerabilities in runtime dependencies before Go-Live. Current dev-only vulnerabilities are acceptable.

---

## Staging Environment Security Posture

| Aspect | Assessment |
|---|---|
| TLS | HTTPS via Traefik with Let's Encrypt -- correct |
| Network isolation | Redis and worker on internal network only -- correct |
| Database exposure | PostgreSQL on external "postgres" network (shared nxlabs.cc) -- acceptable for staging |
| Docker socket | Mounted by Watchtower (OBS-024) -- accepted risk for staging |
| Image provenance | Built in GitHub Actions CI, pushed to ghcr.io -- traceable |
| Secrets management | .env file on staging host -- acceptable for staging; production should use a secrets manager |
| SSE endpoint routing | /events/* routed through Traefik alongside /api/* -- correct |

---

## Recommendations Priority Matrix

| Priority | Finding | Target Cycle |
|---|---|---|
| Must-fix before Go-Live | SEC-001: Admin seed credentials | Cycle 2-3 |
| Must-fix before Go-Live | SEC-002: Redis authentication | Cycle 2-3 |
| Must-fix before Go-Live | SEC-003: Login rate limiting | Cycle 2-3 |
| Should-fix before Go-Live | SEC-004: Request body size limits | Cycle 2-3 |
| Should-fix before Go-Live | SEC-005: CORS policy | Cycle 2-3 |
| Should-fix before Go-Live | SEC-006: Security headers | Cycle 2-3 |
| Should-fix before Go-Live | SEC-007: Password complexity | Cycle 2-3 |
| Fix when convenient | SEC-009: Token in logs | Cycle 2-3 |

---

## Conclusion

The Cycle 1 implementation demonstrates good security fundamentals: proper use of bcrypt for password hashing, cryptographically secure session tokens, parameterized database queries (sqlc), server-side session management, ownership-based access control on pipelines and SSE streams, and a clean role hierarchy.

The three HIGH findings (SEC-001, SEC-002, SEC-003) are standard hardening items that are expected to be absent in a walking skeleton and must be addressed before Go-Live. None of them represent an immediate risk in the current staging-only deployment context.

No Critical findings. No Demo Sign-off blockers at Critical profile severity threshold (Critical and High findings may be deferred for one cycle when the system is not yet publicly exposed).

**Verdict: PASS WITH CONDITIONS -- Cycle 1 Demo Sign-off may proceed. HIGH findings are tracked for resolution in Cycles 2-3, before Go-Live.**
