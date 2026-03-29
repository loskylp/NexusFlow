---
task: TASK-017
title: Admin user management
requirement: REQ-020
status: PASS
date: 2026-03-29
---

# Demo Script — TASK-017: Admin user management

**Audience:** Nexus (staging environment walkthrough)
**Prerequisites:** Docker Compose stack running; admin/admin credentials seeded; API accessible at the staging base URL.

---

## Scenario 1 — Admin creates a new user account

Requirement: REQ-020

Step | Action | Expected result
---|---|---
Given | An admin is logged in (POST /api/auth/login with admin/admin; retain the token) | 200 OK; token in response body
When | POST /api/users with body `{"username":"demo-operator","password":"Secure#Demo!1","role":"user"}` and `Authorization: Bearer <admin-token>` | 201 Created
Then | Response body contains: `id` (UUID), `username: "demo-operator"`, `role: "user"`, `active: true`, `createdAt` (timestamp) | Fields present and correct values
And | Response body does NOT contain any field named `password`, `passwordHash`, or `password_hash` | No password field present

---

## Scenario 2 — Admin lists all user accounts

Requirement: REQ-020

Step | Action | Expected result
---|---|---
Given | Admin is logged in (token from Scenario 1) | Token is valid
When | GET /api/users with `Authorization: Bearer <admin-token>` | 200 OK
Then | Response body is a JSON array | Array notation (starts with `[`)
And | The array includes the `demo-operator` user created in Scenario 1 | Entry with `"username":"demo-operator"` present
And | The array includes the `admin` seed user | Entry with `"username":"admin"` present
And | No entry in the array contains `passwordHash` or `password_hash` | Sensitive field absent from all items
And | Each entry includes an `active` field showing the user's status | `"active":true` visible on active users

---

## Scenario 3 — Admin deactivates a user account

Requirement: REQ-020

Step | Action | Expected result
---|---|---
Given | Admin is logged in; `demo-operator` user was created in Scenario 1 and their UUID is known | UUID captured from Scenario 1 response
Given | Log in as `demo-operator` (POST /api/auth/login) and retain their session token | 200 OK; token captured
When | PUT /api/users/{demo-operator-id}/deactivate with `Authorization: Bearer <admin-token>` | 204 No Content
Then | Response body is empty | No body content
And | GET /api/users still lists `demo-operator` (soft deactivation, not deletion) | User appears in list with `"active":false`

---

## Scenario 4 — Deactivated user's active session is immediately invalidated

Requirement: REQ-020

Step | Action | Expected result
---|---|---
Given | `demo-operator` has been deactivated (Scenario 3); their pre-deactivation session token was captured | Token from Scenario 3 "Given" step
When | Make any authenticated request (e.g., GET /api/tasks) using the pre-deactivation token | 401 Unauthorized
Then | The session is no longer accepted by the API | 401 response with no task data

---

## Scenario 5 — Deactivated user cannot log in

Requirement: REQ-020

Step | Action | Expected result
---|---|---
Given | `demo-operator`'s account has been deactivated (Scenario 3) | active=false in database
When | POST /api/auth/login with `{"username":"demo-operator","password":"Secure#Demo!1"}` | 401 Unauthorized
Then | Login is rejected even with the correct password | 401 response; no token issued

---

## Scenario 6 — Non-admin user is denied access to user management endpoints

Requirement: REQ-020

Step | Action | Expected result
---|---|---
Given | A logged-in user with the `user` role (any non-admin account) | Token has role=user
When | POST /api/users with the user token | 403 Forbidden
When | GET /api/users with the user token | 403 Forbidden
When | PUT /api/users/{any-id}/deactivate with the user token | 403 Forbidden
Then | All three endpoints return 403 to the non-admin caller | No user management operation succeeds

---

## Scenario 7 — Deactivated user's previously submitted tasks continue executing

Requirement: REQ-020

Step | Action | Expected result
---|---|---
Given | A user has submitted tasks that are in running or queued state | Tasks visible in task table with status not `cancelled`
When | Admin deactivates that user (PUT /api/users/{id}/deactivate) | 204 No Content
Then | GET /api/tasks (as admin) shows the deactivated user's tasks still in their original non-cancelled state | Task status unchanged; no cancellation triggered by deactivation
