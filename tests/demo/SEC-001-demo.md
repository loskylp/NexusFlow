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

---
task: SEC-001
title: Password Change Endpoint + Mandatory First-Login Flow
requirements: REQ-019
environment: Staging — https://nexusflow.staging (fresh database, seedAdminIfEmpty has run)
---

# Demo Script — SEC-001
**Feature:** Password Change Endpoint + Mandatory First-Login Flow
**Requirement(s):** REQ-019
**Environment:** Staging — fresh database so the admin seed user is created with mustChangePassword=true

## Scenario 1: First login with seed credentials is blocked from protected routes
**REQ:** REQ-019 / AC-1, AC-2

**Given:** The database has been reset and the application has been started — the seed admin account (username: admin, password: admin) has been created with mustChangePassword=true.

**When:** Log in via `POST /api/auth/login` with `{"username":"admin","password":"admin"}`. Then use the returned token to call `GET /api/workers`.

**Then:** The login returns HTTP 200 with a `token` and `user.mustChangePassword: true` in the response body. The `GET /api/workers` call returns HTTP 403 with body `{"error":"password_change_required"}`.

**Notes:** The `mustChangePassword: true` flag in the login response is what the frontend uses to redirect to the Change Password page. The 403 on `/api/workers` confirms the enforcement gate is active.

---

## Scenario 2: Change-password endpoint rejects wrong current password and short new password
**REQ:** REQ-019 / AC-3, AC-4

**Given:** The admin is logged in with the flagged session token from Scenario 1.

**When (AC-3):** Call `POST /api/auth/change-password` with `{"currentPassword":"wrongpassword","newPassword":"newpassword8"}`.

**Then (AC-3):** HTTP 401 is returned. No change is made to the account.

**When (AC-4):** Call `POST /api/auth/change-password` with `{"currentPassword":"admin","newPassword":"short7!"}` (7 characters).

**Then (AC-4):** HTTP 400 is returned. No change is made to the account.

---

## Scenario 3: Successful password change invalidates old session and clears flag
**REQ:** REQ-019 / AC-5, AC-6

**Given:** The admin is logged in with the flagged session token from Scenario 1.

**When:** Call `POST /api/auth/change-password` with `{"currentPassword":"admin","newPassword":"nexusflow-secure-2024"}`.

**Then:** HTTP 204 is returned. Immediately after, use the same (old) token to call `GET /api/workers` — it returns HTTP 401, confirming the session was invalidated.

**Notes:** The 204 response and subsequent 401 on the old token together confirm: (AC-5) the password was changed and the flag cleared, and (AC-6) all sessions for the user were invalidated.

---

## Scenario 4: Re-login with new password grants full access
**REQ:** REQ-019 / AC-7

**Given:** The password has been changed to `nexusflow-secure-2024` and the old session is invalid.

**When:** Log in via `POST /api/auth/login` with `{"username":"admin","password":"nexusflow-secure-2024"}`. Then use the new token to call `GET /api/workers`.

**Then:** Login returns HTTP 200 with `user.mustChangePassword: false`. The `GET /api/workers` call returns HTTP 200 with the worker list.

**Notes:** `mustChangePassword: false` in the new login response confirms the flag was cleared atomically on password change. Access to a previously-blocked protected endpoint confirms the full enforcement cycle completed correctly.

---

## Scenario 5: Change Password page in the web GUI — first-login forced flow
**REQ:** REQ-019 / Frontend ACs

**Given:** The database has been reset and the seed admin exists with mustChangePassword=true. Navigate to the NexusFlow web GUI and log in as admin/admin.

**When:** After login, the browser is redirected to the `/change-password` route.

**Then:** The Change Password page renders with three fields: Current Password, New Password, Confirm New Password. The "Change Password" button is disabled. The page does not show the sidebar navigation.

**When:** Leave the fields empty and observe — the button remains disabled. Fill in only the Current Password field — the button remains disabled. Fill in all three fields.

**Then:** The button becomes enabled.

**When:** Enter Current Password = `admin`, New Password = `short`, Confirm New Password = `short`. Click "Change Password".

**Then:** An inline error appears below the New Password field: "Password must be at least 8 characters." The form is not submitted.

**When:** Enter New Password = `nexusflow-secure-2024`, Confirm New Password = `differentpassword`. Click "Change Password".

**Then:** An inline error appears below the Confirm New Password field: "Passwords do not match." The form is not submitted.

**When:** Enter Current Password = `wrongpassword`, New Password = `nexusflow-secure-2024`, Confirm New Password = `nexusflow-secure-2024`. Click "Change Password".

**Then:** An inline error appears below the Current Password field: "Current password is incorrect."

**When:** Enter Current Password = `admin`, New Password = `nexusflow-secure-2024`, Confirm New Password = `nexusflow-secure-2024`. Click "Change Password".

**Then:** The browser redirects to the `/login` page. The user must log in again with the new password.

**Notes:** After the successful change the user can log in with `nexusflow-secure-2024` and navigate freely. The `/change-password` route will redirect away (to `/workers`) for users whose mustChangePassword flag is false.
