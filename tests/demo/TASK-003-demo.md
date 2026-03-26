# Demo Script — TASK-003
**Feature:** Authentication and session management
**Requirement(s):** REQ-019, ADR-006
**Environment:** Staging — API at https://api.nexusflow.nxlabs.cc (or http://localhost:8080 for local)

## Scenario 1: Admin login returns token and session cookie

**REQ:** REQ-019

**Given:** The system has just started fresh (or the admin user exists in the database). No prior session is held.

**When:** Send `POST /api/auth/login` with body `{"username":"admin","password":"admin"}` and `Content-Type: application/json`.

**Then:** The response status is 200 OK. The response body is a JSON object with a `token` field (64-character hex string) and a `user` object with `username: "admin"` and `role: "admin"`. The response includes a `Set-Cookie` header with an HTTP-only `session` cookie containing the same token value.

**Notes:** Save the `token` value — it is needed for Scenarios 3, 4, and 5. Save the cookie for Scenario 3 (cookie-based auth). The `Secure` flag on the cookie will be present in staging (HTTPS) but absent in development (HTTP).

---

## Scenario 2: Login with wrong password is rejected

**REQ:** REQ-019

**Given:** The admin user exists in the database.

**When:** Send `POST /api/auth/login` with body `{"username":"admin","password":"wrongpassword"}`.

**Then:** The response status is 401 Unauthorized. No session cookie is set. The response body contains an error message (not the internal error — credential mismatch is not logged in detail to prevent oracle attacks).

**Notes:** Try also with a non-existent username — the response must also be 401 (same status, same response time to prevent username enumeration timing attacks).

---

## Scenario 3: Unauthenticated request to protected endpoint is blocked

**REQ:** REQ-019

**Given:** No session token or cookie is present in the request.

**When:** Send `GET /api/tasks` with no `Authorization` header and no `session` cookie.

**Then:** The response status is 401 Unauthorized.

**Notes:** This demonstrates that the auth middleware is wired on all protected routes. Try also `GET /api/pipelines` and `GET /api/workers` — all must return 401.

---

## Scenario 4: Authenticated request passes through to the handler

**REQ:** REQ-019

**Given:** A valid session token obtained from Scenario 1.

**When:** Send `GET /api/tasks` with header `Authorization: Bearer <token from Scenario 1>`.

**Then:** The response status is not 401. (At this stage TASK-005 is not yet implemented, so the response may be 500 from an unimplemented stub — but the middleware passed the request through, confirming the session was found in Redis and injected into the request context.)

**Notes:** The same test applies using the `session` cookie from Scenario 1 instead of the Bearer header — both authentication paths must work.

---

## Scenario 5: Logout invalidates the session

**REQ:** REQ-019

**Given:** A valid session token obtained from a fresh login (repeat Scenario 1 to get a new token for this test).

**When:** First confirm the token is valid: send `GET /api/tasks` with the token — expect non-401. Then send `POST /api/auth/logout` with `Authorization: Bearer <token>`.

**Then:** The logout response status is 204 No Content. Immediately after, send `GET /api/tasks` again with the same token — the response must now be 401 Unauthorized. The session is gone from Redis.

**Notes:** This demonstrates immediate session revocation — a core requirement (REQ-020). The session key `session:{token}` is deleted from Redis on logout. There is no grace period.

---

## Scenario 6: Admin user is present after first startup

**REQ:** REQ-019

**Given:** The API has been started against a fresh database (no users exist). This can be confirmed by the startup log line: `api: admin user seeded (username=admin)`.

**When:** Send `POST /api/auth/login` with `{"username":"admin","password":"admin"}`.

**Then:** The response status is 200 OK. The admin user was created automatically by the seed function on first startup. This is the bootstrap mechanism — no manual database setup is required.

**Notes:** If the database already has users (from a prior run), the seed is skipped. To demonstrate the seed in staging, the database would need to be fresh. The startup log is the evidence artifact for this scenario in a non-fresh environment.

---
