---
task: TASK-025
title: Worker fleet status API
result: PASS
date: 2026-03-27
requirements: REQ-016
environment: staging API at https://api.nexusflow.staging (or http://localhost:8080 for local)
---

# Demo Script — TASK-025
**Feature:** Worker fleet status API
**Requirement(s):** REQ-016
**Environment:** Staging API server. Admin credentials: username `admin`, password `admin`. Requires `curl` and `jq`.

---

## Scenario 1: Authenticated admin retrieves list of all registered workers

**REQ:** REQ-016

**Given:** the API server is running; at least one worker process is connected and has sent a heartbeat; you have logged in as admin to obtain a session token (see setup below)

**When:** `GET /api/workers` with `Authorization: Bearer <token>`

**Then:** the response status is 200 OK; `Content-Type` is `application/json`; the body is a JSON array; each element in the array contains the fields `id`, `status`, `tags`, `currentTaskId`, and `lastHeartbeat`; `status` is either `"online"` or `"down"`; `tags` is an array (may be empty); `currentTaskId` is a UUID string or `null`; `lastHeartbeat` is a non-empty timestamp string

**Notes:** Run `docker compose ps` to confirm at least one worker container is up before running this scenario. The response should show all workers that have registered with the API, regardless of how many the caller expects. No pagination is required for this endpoint.

Sample command:
```bash
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/workers | jq .
```

Sample response shape:
```json
[
  {
    "id": "a1b2c3d4-...",
    "status": "online",
    "tags": ["etl", "report"],
    "currentTaskId": null,
    "lastHeartbeat": "2026-03-27T10:00:00Z"
  }
]
```

---

## Scenario 2: Empty worker registry returns an empty array, not null or an error

**REQ:** REQ-016

**Given:** no worker containers are running (stop workers via `docker compose stop worker`)

**When:** `GET /api/workers` with a valid admin session token

**Then:** the response status is 200 OK; the body is `[]` (an empty JSON array), not `null`, not `{}`, and not an error object

**Notes:** Restart the worker after this scenario: `docker compose up -d worker`. This scenario confirms that an empty fleet is a valid state, not an error condition.

---

## Scenario 3: Unauthenticated request is rejected with 401

**REQ:** REQ-016

**Given:** no `Authorization` header is included

**When:** `GET /api/workers` without any auth

**Then:** the response status is 401 Unauthorized; the body is `{"error":"unauthorized"}`; a worker list is not disclosed

Sample command:
```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/workers
# Expected: 401
```

---

## Scenario 4: Invalid or expired token is rejected with 401

**REQ:** REQ-016

**Given:** a syntactically plausible but non-existent 64-character hex token

**When:** `GET /api/workers` with `Authorization: Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`

**Then:** the response status is 401 Unauthorized; no worker data is returned

---

## Setup: obtain admin session token

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' \
  | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
echo "Token: $TOKEN"
```
