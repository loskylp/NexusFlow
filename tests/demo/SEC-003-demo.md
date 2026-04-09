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

# Demo Script — SEC-003
**Feature:** Login Rate Limiting (Security Remediation)
**Requirement(s):** SEC-003
**Environment:** Staging — API base URL: `https://nexusflow-staging.internal/api`

---

## Scenario 1: 3 failed attempts trigger lockout
**REQ:** SEC-003 AC-1, AC-3

**Given:** The staging API is running and no prior login attempts have been made from the demo IP.

**When:** Send 3 consecutive POST requests to `/api/auth/login` with invalid credentials:
```
curl -s -o /dev/null -w "%{http_code}" -X POST https://nexusflow-staging.internal/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"nobody","password":"wrong"}'
```
Run this command 3 times. Then send a 4th identical request and capture headers:
```
curl -si -X POST https://nexusflow-staging.internal/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"nobody","password":"wrong"}'
```

**Then:**
- The first 3 requests return `HTTP 401 Unauthorized`.
- The 4th request returns `HTTP 429 Too Many Requests`.
- The 4th response includes the header `Retry-After: 60`.

---

## Scenario 2: Lockout persists for 1 minute
**REQ:** SEC-003 AC-2

**Given:** The IP used in Scenario 1 is currently locked out (the 4th attempt just returned 429).

**When:** Immediately send another POST to `/api/auth/login` from the same IP:
```
curl -si -X POST https://nexusflow-staging.internal/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"nobody","password":"wrong"}'
```

**Then:** The response is `HTTP 429 Too Many Requests` — the lockout is still active.

**Notes:** Wait 60 seconds and repeat the request. It should now return `HTTP 401` (lockout expired; normal failure handling resumes).

---

## Scenario 3: Successful login resets the counter
**REQ:** SEC-003 AC-5

**Given:** A valid user account exists in staging (e.g., `demo_user` / `DemoPass1!`). The demo IP has made 2 prior failed login attempts (counter = 2).

**When:** First, accumulate 2 failures:
```
for i in 1 2; do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST https://nexusflow-staging.internal/api/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"demo_user","password":"wrong"}'
done
```
Then send a successful login:
```
curl -si -X POST https://nexusflow-staging.internal/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"demo_user","password":"DemoPass1!"}'
```
Then send 2 more failed attempts:
```
for i in 1 2; do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST https://nexusflow-staging.internal/api/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"demo_user","password":"wrong"}'
done
```

**Then:**
- The successful login returns `HTTP 200 OK` with a token in the response body.
- The subsequent 2 failed attempts each return `HTTP 401` (not 429) — the counter was reset to 0 by the successful login.

---

## Scenario 4: Only 401 responses count toward the limit
**REQ:** SEC-003 AC-4

**Given:** A fresh IP with no prior login history.

**When:** Send 5 requests with a malformed body (missing username field):
```
for i in 1 2 3 4 5; do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST https://nexusflow-staging.internal/api/auth/login \
    -H "Content-Type: application/json" \
    -d '{"password":"anything"}'
done
```
Then send 1 well-formed but invalid request:
```
curl -s -o /dev/null -w "%{http_code}" -X POST https://nexusflow-staging.internal/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"nobody","password":"wrong"}'
```

**Then:**
- All 5 malformed requests return `HTTP 400 Bad Request`.
- The 6th (well-formed) request returns `HTTP 401 Unauthorized` — not 429 — confirming that 400 responses did not count toward the failure limit.

---

## Scenario 5: Other routes are not rate-limited
**REQ:** SEC-003 AC-6

**Given:** The IP from Scenario 1 is locked out of POST /api/auth/login.

**When:** Send a GET request to the health endpoint from the same IP:
```
curl -s -o /dev/null -w "%{http_code}" https://nexusflow-staging.internal/api/health
```

**Then:** The response is `HTTP 200 OK` — the health endpoint is not affected by the login rate limiter.

---
