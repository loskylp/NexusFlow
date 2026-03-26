# ADR-006: Authentication and Session Management

**Status:** Proposed
**Date:** 2026-03-26
**Characteristic:** Security, Maintainability

## Context

The Nexus decided that NexusFlow manages its own credentials -- username/password with session tokens (ESC-002 resolution). REQ-019 specifies role-based access control with Admin and User roles. REQ-020 specifies admin user management (create, view, deactivate). This ADR decides the session token implementation: how tokens are issued, validated, stored, and invalidated.

## Trade-off Analysis

| Option | Gains | Costs | Risk if wrong | Cost to change later |
|---|---|---|---|---|
| JWT (stateless tokens) | No server-side session storage; scales horizontally; self-contained claims (role, user ID) | Cannot revoke a token before expiry without a blocklist (which reintroduces server-side state); token size larger than opaque IDs | Deactivated users retain access until token expires; REQ-020 requires immediate session invalidation on deactivation | Medium -- switching token format requires client changes |
| Server-side sessions (Redis-backed) | Immediate revocation (delete session from Redis); small token (opaque session ID); simple to implement | Requires Redis lookup on every authenticated request; session state is server-side | Redis is already in the stack (ADR-001) -- no additional infrastructure | Medium -- same |
| JWT with Redis revocation blocklist | Self-contained claims + revocability via blocklist | Complexity of two mechanisms; every request checks the blocklist anyway, negating the "stateless" benefit of JWT | Over-engineered -- if you need server-side state for revocation, just use server-side sessions | Medium |

## Decision

**Server-side sessions stored in Redis**, with opaque session tokens issued as HTTP-only secure cookies (web GUI) and Bearer tokens (API).

- Password hashing: bcrypt with cost factor 12
- Session token: cryptographically random 256-bit value, hex-encoded
- Session storage: Redis key `session:{token}` with TTL (24 hours default, configurable)
- Session data: `{ userId, role, createdAt }`
- Deactivation: deleting all `session:*` keys for the user (REQ-020 -- immediate invalidation)

**Door type:** Two-way -- session storage mechanism can be changed without altering the API contract (the token is opaque to clients).

**Cost to change later:** Low -- the session store is behind an abstraction. Switching from Redis sessions to JWT (or vice versa) changes the auth middleware, not the API surface.

## Rationale

**Server-side sessions** are chosen because REQ-020 requires immediate session invalidation when an admin deactivates a user account. JWT cannot satisfy this without a revocation blocklist, which reintroduces the server-side state that JWT was designed to avoid. Since Redis is already in the stack (ADR-001), storing sessions in Redis adds zero infrastructure cost. The lookup on each request is a single Redis GET, well under 1ms.

**bcrypt with cost factor 12** is the industry standard for password hashing. It is deliberately slow (to resist brute force) but fast enough for login flows. The cost factor can be increased in the future as hardware improves.

**HTTP-only secure cookies for the web GUI** prevent XSS-based token theft. **Bearer tokens for the API** support programmatic integration by other teams (Brief -- REST API as first-class surface). Both map to the same server-side session.

### Trust boundaries

- The API server is the only component that validates sessions. Workers do not handle authentication -- they receive tasks from Redis, not from users.
- Redis is a trusted internal component. Session data in Redis is not encrypted at rest (performance over defense-in-depth for an internal deployment).
- The web frontend never stores the session token in localStorage (cookie-only for CSRF resistance).

## Fitness Function
**Characteristic threshold:** Unauthenticated requests rejected; deactivated users lose access immediately; session lookup < 5ms p95

| | Specification |
|---|---|
| **Dev check** | Auth test suite: unauthenticated request returns 401; valid session returns 200; expired session returns 401; deactivated user's session returns 401 immediately after deactivation; role-based access returns 403 for unauthorized roles. |
| **Prod metric** | Session lookup latency; login failure rate; active session count; 401/403 response rate. |
| **Warning threshold** | Session lookup p95 > 5ms; login failure rate > 20% (possible brute force); active sessions > 1000 |
| **Critical threshold** | Any request bypassing auth (returns 200 without valid session); session lookup > 50ms (Redis performance issue) |
| **Alarm meaning** | Warning: possible brute force attack or Redis session store performance degradation. Critical: auth bypass detected -- security incident. |

## Consequences
**Easier:** Immediate session revocation on user deactivation; simple auth middleware (one Redis GET per request); no JWT library complexity; session extension on activity is trivial (EXPIRE command).
**Harder:** Horizontal scaling of the API server requires all instances to share the same Redis session store (but they already share Redis for the queue). Stateless API consumers (other teams) must manage session token lifecycle.
**Newly required:** Auth middleware in the Go HTTP router; login endpoint (POST /auth/login); logout endpoint (POST /auth/logout); session Redis key management via go-redis; bcrypt dependency (golang.org/x/crypto/bcrypt); CORS and cookie configuration for the web GUI.
