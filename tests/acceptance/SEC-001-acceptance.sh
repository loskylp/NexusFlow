#!/usr/bin/env bash
# SEC-001 Acceptance Test — Password change endpoint and mandatory first-login flow.
#
# Validates:
#   AC-1: Admin seed user created with must_change_password=true; login succeeds with flagged session.
#   AC-2: Protected endpoints return 403 {"error":"password_change_required"} with flagged session.
#   AC-3: POST /api/auth/change-password with incorrect current password returns 401.
#   AC-4: POST /api/auth/change-password with new password < 8 chars returns 400.
#   AC-5: POST /api/auth/change-password with valid credentials returns 204; flag cleared atomically.
#   AC-6: After successful change, old session is invalidated (401 on subsequent requests).
#   AC-7: Re-login with new password succeeds; protected endpoints return 200.
#
# Preconditions:
#   - API server is running and reachable at API_BASE (default: http://localhost:8080).
#   - Database has been reset so seedAdminIfEmpty runs and creates admin/admin with must_change_password=true.
#   - jq is available.
#
# Usage:
#   API_BASE=http://localhost:8080 bash tests/acceptance/SEC-001-acceptance.sh
#
# See: SEC-001, SEC-007, ADR-006
set -euo pipefail

API_BASE="${API_BASE:-http://localhost:8080}"
PASS_COUNT=0
FAIL_COUNT=0

pass() { echo "  PASS: $1"; ((PASS_COUNT++)); }
fail() { echo "  FAIL: $1"; ((FAIL_COUNT++)); }

require_status() {
  local label="$1" expected="$2" actual="$3"
  if [ "$actual" -eq "$expected" ]; then
    pass "$label (HTTP $actual)"
  else
    fail "$label — expected HTTP $expected, got HTTP $actual"
  fi
}

require_body_contains() {
  local label="$1" expected="$2" actual="$3"
  if echo "$actual" | grep -qF "$expected"; then
    pass "$label (body contains '$expected')"
  else
    fail "$label — expected body to contain '$expected', got: $actual"
  fi
}

echo "SEC-001 acceptance test — password change endpoint and first-login flow"
echo "API_BASE: $API_BASE"
echo ""

# ---------------------------------------------------------------------------
# AC-1: Login as admin/admin — session returned; flag is set
# ---------------------------------------------------------------------------
echo "AC-1: Login as admin/admin (seed credentials)"
LOGIN_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}')
LOGIN_BODY=$(echo "$LOGIN_RESPONSE" | head -1)
LOGIN_STATUS=$(echo "$LOGIN_RESPONSE" | tail -1)

require_status "AC-1: login returns 200" 200 "$LOGIN_STATUS"

ADMIN_TOKEN=$(echo "$LOGIN_BODY" | jq -r '.token // empty')
if [ -z "$ADMIN_TOKEN" ]; then
  fail "AC-1: token not present in login response"
else
  pass "AC-1: token present in login response"
fi

MUST_CHANGE=$(echo "$LOGIN_BODY" | jq -r '.user.mustChangePassword // empty')
if [ "$MUST_CHANGE" = "true" ]; then
  pass "AC-1: mustChangePassword=true in login response"
else
  fail "AC-1: expected mustChangePassword=true in login response, got: $MUST_CHANGE"
fi

# ---------------------------------------------------------------------------
# AC-2: Protected endpoints return 403 with flagged session
# ---------------------------------------------------------------------------
echo ""
echo "AC-2: Protected endpoint returns 403 with flagged session"
WORKERS_RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$API_BASE/api/workers" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
WORKERS_BODY=$(echo "$WORKERS_RESPONSE" | head -1)
WORKERS_STATUS=$(echo "$WORKERS_RESPONSE" | tail -1)

require_status "AC-2: GET /api/workers returns 403" 403 "$WORKERS_STATUS"
require_body_contains "AC-2: body contains password_change_required" "password_change_required" "$WORKERS_BODY"

# ---------------------------------------------------------------------------
# AC-3: Wrong current password returns 401
# ---------------------------------------------------------------------------
echo ""
echo "AC-3: Wrong current password returns 401"
WRONG_PW_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/api/auth/change-password" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"currentPassword":"wrongpassword","newPassword":"newpassword8"}')
WRONG_PW_STATUS=$(echo "$WRONG_PW_RESPONSE" | tail -1)

require_status "AC-3: wrong current password returns 401" 401 "$WRONG_PW_STATUS"

# ---------------------------------------------------------------------------
# AC-4: New password too short returns 400
# ---------------------------------------------------------------------------
echo ""
echo "AC-4: New password shorter than 8 chars returns 400"
SHORT_PW_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/api/auth/change-password" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"currentPassword":"admin","newPassword":"short7!"}')
SHORT_PW_STATUS=$(echo "$SHORT_PW_RESPONSE" | tail -1)

require_status "AC-4: short new password returns 400" 400 "$SHORT_PW_STATUS"

# ---------------------------------------------------------------------------
# AC-5: Valid change returns 204; flag cleared
# ---------------------------------------------------------------------------
echo ""
echo "AC-5: Valid password change returns 204"
NEW_PASSWORD="nexusflow-secure-2024"
CHANGE_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/api/auth/change-password" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"currentPassword\":\"admin\",\"newPassword\":\"$NEW_PASSWORD\"}")
CHANGE_STATUS=$(echo "$CHANGE_RESPONSE" | tail -1)

require_status "AC-5: valid change returns 204" 204 "$CHANGE_STATUS"

# ---------------------------------------------------------------------------
# AC-6: Old session is invalidated after change
# ---------------------------------------------------------------------------
echo ""
echo "AC-6: Old session is invalidated after password change"
OLD_SESSION_RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$API_BASE/api/workers" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
OLD_SESSION_STATUS=$(echo "$OLD_SESSION_RESPONSE" | tail -1)

require_status "AC-6: old session returns 401 after change" 401 "$OLD_SESSION_STATUS"

# ---------------------------------------------------------------------------
# AC-7: Re-login with new password; protected endpoints accessible
# ---------------------------------------------------------------------------
echo ""
echo "AC-7: Re-login with new password; access protected endpoints"
NEW_LOGIN_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"admin\",\"password\":\"$NEW_PASSWORD\"}")
NEW_LOGIN_BODY=$(echo "$NEW_LOGIN_RESPONSE" | head -1)
NEW_LOGIN_STATUS=$(echo "$NEW_LOGIN_RESPONSE" | tail -1)

require_status "AC-7: re-login with new password returns 200" 200 "$NEW_LOGIN_STATUS"

NEW_TOKEN=$(echo "$NEW_LOGIN_BODY" | jq -r '.token // empty')
NEW_MUST_CHANGE=$(echo "$NEW_LOGIN_BODY" | jq -r '.user.mustChangePassword // empty')

if [ "$NEW_MUST_CHANGE" = "false" ]; then
  pass "AC-7: mustChangePassword=false after successful change"
else
  fail "AC-7: expected mustChangePassword=false, got: $NEW_MUST_CHANGE"
fi

WORKERS_NEW_RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$API_BASE/api/workers" \
  -H "Authorization: Bearer $NEW_TOKEN")
WORKERS_NEW_STATUS=$(echo "$WORKERS_NEW_RESPONSE" | tail -1)

require_status "AC-7: GET /api/workers returns 200 with new session" 200 "$WORKERS_NEW_STATUS"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "─────────────────────────────────────────────"
echo "SEC-001 acceptance: $PASS_COUNT passed, $FAIL_COUNT failed"
echo "─────────────────────────────────────────────"

if [ "$FAIL_COUNT" -gt 0 ]; then
  exit 1
fi
exit 0
