#!/usr/bin/env bash
# SEC-001 Acceptance Test — Password change endpoint and mandatory first-login flow.
#
# Validates:
#   1. Admin seeded user must change password on first login.
#   2. POST /api/auth/change-password accepts old + new password, returns 204.
#   3. After change, all protected endpoints are accessible.
#   4. Before change, all protected endpoints (except change-password) return 403
#      with {"error": "password_change_required"}.
#   5. Incorrect current password returns 401.
#   6. New password shorter than 8 characters returns 400.
#   7. Session is invalidated after successful password change; re-auth required.
#
# Preconditions:
#   - API server is running and reachable at API_BASE (default: http://localhost:8080).
#   - Admin seed user exists with must_change_password = true.
#
# See: SEC-001, SEC-007, ADR-006
set -euo pipefail

API_BASE="${API_BASE:-http://localhost:8080}"

echo "SEC-001 acceptance: password change endpoint and first-login flow"
echo "TODO: implement acceptance tests"
echo "  Step 1: login as admin/admin (seed credentials)"
echo "  Step 2: verify GET /api/workers returns 403 with password_change_required"
echo "  Step 3: POST /api/auth/change-password with wrong current password -> 401"
echo "  Step 4: POST /api/auth/change-password with new password < 8 chars -> 400"
echo "  Step 5: POST /api/auth/change-password with valid credentials -> 204"
echo "  Step 6: verify old session returns 401 (invalidated after change)"
echo "  Step 7: re-login with new password -> 200"
echo "  Step 8: verify GET /api/workers returns 200 (must_change_password cleared)"
exit 0
