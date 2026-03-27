#!/usr/bin/env bash
# TASK-019 Acceptance Tests — React app shell with sidebar navigation and auth flow
# REQ-019: Role-based authentication and access control
# REQ-016: Worker Fleet routing after login
# DEMO-003/004: Demo nav visibility rules
#
# Mode: Pre-staging (local, no running server)
# These tests verify the implementation by:
#   1. Running the full vitest unit test suite (31 tests covering all 7 ACs)
#   2. Running the TypeScript build (type correctness + production bundle)
#   3. Running the TypeScript typecheck pass
#
# Usage (from project root):
#   bash tests/acceptance/TASK-019-acceptance.sh
#
# Requirements: Node 20+, npm, web/ dependencies installed

set -euo pipefail

WEB_DIR="/Users/pablo/projects/Nexus/NexusTests/NexusFlow/web"

PASS=0
FAIL=0
RESULTS=()

pass() {
  local name="$1"
  echo "  PASS: $name"
  PASS=$((PASS + 1))
  RESULTS+=("PASS | $name")
}

fail() {
  local name="$1"
  local detail="$2"
  echo "  FAIL: $name"
  echo "        $detail"
  FAIL=$((FAIL + 1))
  RESULTS+=("FAIL | $name | $detail")
}

echo ""
echo "=== TASK-019 Acceptance Tests ==="
echo "    Mode: Pre-staging (local, unit tests + build + typecheck)"
echo ""

# ---------------------------------------------------------------------------
# Phase 1: Vitest unit test suite
# The Builder's 31 unit tests cover all 7 acceptance criteria at the component
# and hook level. Each test is traced to a specific AC. A vitest pass is the
# primary acceptance gate for this pre-staging mode.
#
# AC coverage in the unit test suite:
#   AC-1 (LoginPage form): LoginPage.test.tsx — "form renders" describe block
#   AC-2 (role redirect): LoginPage.test.tsx — "success redirect" describe block
#   AC-3 (inline error): LoginPage.test.tsx — "invalid credentials" describe block
#   AC-4 (sidebar nav items): Sidebar.test.tsx — "primary nav items" describe block
#   AC-5 (demo nav hidden): Sidebar.test.tsx — "demo nav visibility" describe block
#   AC-6 (unauth redirect): ProtectedRoute.test.tsx — "redirects to /login when user is null"
#   AC-6 (auth loading): ProtectedRoute.test.tsx — "shows nothing while auth is loading"
#   AC-7 (design tokens): Covered by globals.css static analysis — see Phase 3
# ---------------------------------------------------------------------------
echo "--- Phase 1: Unit tests (vitest) ---"

VITEST_OUTPUT=$(npm --prefix "$WEB_DIR" run test 2>&1)
VITEST_EXIT=$?

echo "$VITEST_OUTPUT" | tail -20

if [ $VITEST_EXIT -eq 0 ]; then
  # Extract test counts from vitest output
  TEST_COUNT=$(echo "$VITEST_OUTPUT" | grep -o 'Tests  [0-9]* passed' | grep -o '[0-9]*' | head -1 || echo "?")
  pass "AC-1..6 [REQ-019]: vitest unit suite — ${TEST_COUNT} tests passing"
else
  FAILED_COUNT=$(echo "$VITEST_OUTPUT" | grep -o 'Tests  [0-9]* failed' | grep -o '[0-9]*' | head -1 || echo "?")
  fail "AC-1..6 [REQ-019]: vitest unit suite has failures" \
       "${FAILED_COUNT} test(s) failed — see vitest output above"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-1 (negative) [VERIFIER-ADDED]: LoginPage must have labeled username and password inputs.
# A component without labels would still render inputs but fail WCAG accessibility.
# We grep the LoginPage source to confirm htmlFor/label associations exist for both fields.
# ---------------------------------------------------------------------------
echo "--- AC-1 negative: labeled inputs present in LoginPage source ---"

# REQ-019: LoginPage — AC-1 (form renders with username/password per UX spec)
# Given: LoginPage.tsx exists
# When: we inspect the source for label associations
# Then: both htmlFor="username" and htmlFor="password" must be present

LP_SRC="$WEB_DIR/src/pages/LoginPage.tsx"

if grep -q 'htmlFor="username"' "$LP_SRC" && grep -q 'htmlFor="password"' "$LP_SRC"; then
  pass "AC-1 [REQ-019]: LoginPage has labeled username and password inputs (htmlFor associations)"
else
  fail "AC-1 [REQ-019]: LoginPage missing labeled inputs" \
       "Expected htmlFor=\"username\" and htmlFor=\"password\" in LoginPage.tsx"
fi

# [VERIFIER-ADDED] A component that renders a form without a submit button would not satisfy AC-1.
# Confirm the submit button type exists.
if grep -q 'type="submit"' "$LP_SRC"; then
  pass "AC-1 [VERIFIER-ADDED] [REQ-019]: LoginPage has type=\"submit\" button"
else
  fail "AC-1 [VERIFIER-ADDED] [REQ-019]: LoginPage missing submit button" \
       "Expected type=\"submit\" in LoginPage.tsx"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-2 negative [VERIFIER-ADDED]: Role-based redirect logic must distinguish admin from user.
# A trivially permissive implementation might always redirect to /workers.
# We inspect the redirect logic to confirm both roles are handled.
# ---------------------------------------------------------------------------
echo "--- AC-2 negative: role-based redirect covers both roles ---"

# REQ-019: LoginPage — AC-2 (successful login redirects to correct page per role)
# Given: LoginPage.tsx with role-based redirect
# When: we inspect the redirect logic
# Then: both '/workers' and '/tasks' destinations must be present (one per role)

if grep -q "'/workers'" "$LP_SRC" && grep -q "'/tasks'" "$LP_SRC"; then
  pass "AC-2 [REQ-019]: LoginPage redirect logic handles both admin (/workers) and user (/tasks)"
else
  fail "AC-2 [REQ-019]: LoginPage redirect logic is not role-differentiated" \
       "Expected both '/workers' and '/tasks' paths in LoginPage.tsx"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-3 negative [VERIFIER-ADDED]: Error display must use role="alert" (required by UX spec
# and AC-3: "inline error message"). A component that uses a plain <div> without role="alert"
# would not satisfy the UX spec's accessibility requirement.
# ---------------------------------------------------------------------------
echo "--- AC-3 negative: error element uses role=alert ---"

# REQ-019: LoginPage — AC-3 (invalid credentials show inline error message)
# Given: LoginPage.tsx with error state
# When: we inspect the error element markup
# Then: role="alert" must be present on the error container

if grep -q 'role="alert"' "$LP_SRC"; then
  pass "AC-3 [REQ-019]: LoginPage error uses role=\"alert\" (inline error per UX spec)"
else
  fail "AC-3 [REQ-019]: LoginPage error element missing role=\"alert\"" \
       "Expected role=\"alert\" on the error <p> in LoginPage.tsx"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-4 negative [VERIFIER-ADDED]: Sidebar must list all four primary nav items.
# A trivially permissive Sidebar that renders an empty nav would pass unit tests
# only if the tests themselves are comprehensive — this static check provides
# an independent second layer of assurance.
# ---------------------------------------------------------------------------
echo "--- AC-4 negative: Sidebar source lists all four primary nav items ---"

# REQ-019: Sidebar — AC-4 (sidebar visible on all authenticated views with correct items)
# Given: Sidebar.tsx with PRIMARY_NAV array
# When: we inspect the source
# Then: all four UX spec nav items must appear: Worker Fleet, Task Feed, Pipeline Builder, Log Streamer

SIDEBAR_SRC="$WEB_DIR/src/components/Sidebar.tsx"

for ITEM in "Worker Fleet" "Task Feed" "Pipeline Builder" "Log Streamer"; do
  if grep -q "$ITEM" "$SIDEBAR_SRC"; then
    pass "AC-4 [REQ-019]: Sidebar PRIMARY_NAV contains \"$ITEM\""
  else
    fail "AC-4 [REQ-019]: Sidebar PRIMARY_NAV missing \"$ITEM\"" \
         "Expected \"$ITEM\" in Sidebar.tsx PRIMARY_NAV"
  fi
done

echo ""

# ---------------------------------------------------------------------------
# AC-5 negative [VERIFIER-ADDED]: Demo nav must be gated on role === 'admin'.
# A trivially permissive implementation might always render demo items.
# We confirm the conditional render uses the admin role guard.
# ---------------------------------------------------------------------------
echo "--- AC-5 negative: demo section rendered only for admin role ---"

# REQ-019: Sidebar — AC-5 (demo nav items hidden for User role)
# Given: Sidebar.tsx with conditional demo section
# When: we inspect the conditional render logic
# Then: the guard must reference role === 'admin'

if grep -q "role === 'admin'" "$SIDEBAR_SRC"; then
  pass "AC-5 [REQ-019]: Sidebar demo section is gated on user.role === 'admin'"
else
  fail "AC-5 [REQ-019]: Sidebar demo section is not gated on admin role" \
       "Expected role === 'admin' guard in Sidebar.tsx demo section"
fi

# Confirm both demo items are listed in DEMO_NAV
for ITEM in "Sink Inspector" "Chaos Controller"; do
  if grep -q "$ITEM" "$SIDEBAR_SRC"; then
    pass "AC-5 [REQ-019]: DEMO_NAV contains \"$ITEM\""
  else
    fail "AC-5 [REQ-019]: DEMO_NAV missing \"$ITEM\"" \
         "Expected \"$ITEM\" in Sidebar.tsx DEMO_NAV"
  fi
done

echo ""

# ---------------------------------------------------------------------------
# AC-6 negative [VERIFIER-ADDED]: ProtectedRoute must redirect to /login (not another path)
# when unauthenticated. A route that redirects to / would create an infinite redirect loop.
# ---------------------------------------------------------------------------
echo "--- AC-6 negative: ProtectedRoute redirects specifically to /login ---"

# REQ-019: ProtectedRoute — AC-6 (unauthenticated users redirected to /login)
# Given: ProtectedRoute.tsx with unauthenticated guard
# When: we inspect the Navigate destination
# Then: Navigate must point to "/login", not "/" or any other path

PR_SRC="$WEB_DIR/src/components/ProtectedRoute.tsx"

if grep -q 'to="/login"' "$PR_SRC"; then
  pass "AC-6 [REQ-019]: ProtectedRoute redirects to \"/login\" when unauthenticated"
else
  fail "AC-6 [REQ-019]: ProtectedRoute redirect destination is not \"/login\"" \
       "Expected Navigate to=\"/login\" in ProtectedRoute.tsx"
fi

# [VERIFIER-ADDED] Confirm ProtectedRoute does NOT redirect during isLoading
# (otherwise users would always see /login briefly on hard refresh).
# A trivially broken implementation would redirect on isLoading OR when user is null.
if grep -q 'isLoading' "$PR_SRC"; then
  pass "AC-6 [VERIFIER-ADDED] [REQ-019]: ProtectedRoute checks isLoading to suppress redirect flash"
else
  fail "AC-6 [VERIFIER-ADDED] [REQ-019]: ProtectedRoute does not handle isLoading state" \
       "Expected isLoading check in ProtectedRoute.tsx to prevent redirect flash on refresh"
fi

echo ""

# ---------------------------------------------------------------------------
# AC-7: Design system tokens applied globally (globals.css static analysis)
# REQ-019: design tokens (colors, typography, spacing) applied globally
# Given: globals.css exists at web/src/styles/globals.css
# When: we check for required CSS custom property declarations
# Then: all DESIGN.md token categories must be present in :root
# ---------------------------------------------------------------------------
echo "--- AC-7: Design system tokens in globals.css ---"

CSS_SRC="$WEB_DIR/src/styles/globals.css"

# Color tokens (from DESIGN.md)
for TOKEN in \
  "--color-primary" \
  "--color-surface-base" \
  "--color-surface-panel" \
  "--color-surface-subtle" \
  "--color-border" \
  "--color-text-primary" \
  "--color-text-secondary" \
  "--color-text-tertiary" \
  "--color-success" \
  "--color-warning" \
  "--color-error" \
  "--color-info"; do
  if grep -qF -- "$TOKEN" "$CSS_SRC"; then
    pass "AC-7 [REQ-019]: globals.css defines $TOKEN"
  else
    fail "AC-7 [REQ-019]: globals.css missing $TOKEN" \
         "Expected $TOKEN in web/src/styles/globals.css :root block"
  fi
done

# Typography tokens
for TOKEN in "--font-sans" "--font-label" "--font-mono"; do
  if grep -qF -- "$TOKEN" "$CSS_SRC"; then
    pass "AC-7 [REQ-019]: globals.css defines $TOKEN"
  else
    fail "AC-7 [REQ-019]: globals.css missing $TOKEN" \
         "Expected $TOKEN in web/src/styles/globals.css :root block"
  fi
done

# Spacing tokens (4px base scale)
for TOKEN in "--space-1" "--space-2" "--space-4" "--space-6" "--space-8"; do
  if grep -qF -- "$TOKEN" "$CSS_SRC"; then
    pass "AC-7 [REQ-019]: globals.css defines $TOKEN"
  else
    fail "AC-7 [REQ-019]: globals.css missing $TOKEN" \
         "Expected $TOKEN in web/src/styles/globals.css :root block"
  fi
done

# Sidebar layout tokens (referenced by Layout and Sidebar components)
for TOKEN in "--sidebar-width" "--sidebar-bg"; do
  if grep -qF -- "$TOKEN" "$CSS_SRC"; then
    pass "AC-7 [REQ-019]: globals.css defines $TOKEN"
  else
    fail "AC-7 [REQ-019]: globals.css missing $TOKEN" \
         "Expected $TOKEN in web/src/styles/globals.css :root block"
  fi
done

# Body background must use --color-surface-base (#FAFAFA per DESIGN.md)
if grep -q "background-color: var(--color-surface-base)" "$CSS_SRC"; then
  pass "AC-7 [REQ-019]: body background-color uses --color-surface-base token"
else
  fail "AC-7 [REQ-019]: body background-color does not reference --color-surface-base" \
       "Expected 'background-color: var(--color-surface-base)' in body rule of globals.css"
fi

# Font families imported (Google Fonts import line)
if grep -q "Inter" "$CSS_SRC" && grep -q "IBM Plex Sans" "$CSS_SRC" && grep -q "JetBrains Mono" "$CSS_SRC"; then
  pass "AC-7 [REQ-019]: globals.css imports Inter, IBM Plex Sans, and JetBrains Mono fonts"
else
  fail "AC-7 [REQ-019]: globals.css missing required font imports" \
       "Expected @import for Inter, IBM Plex Sans, JetBrains Mono in globals.css"
fi

echo ""

# ---------------------------------------------------------------------------
# Phase 2: TypeScript build
# A clean production build confirms that all imports resolve, all types are
# satisfied end-to-end, and the app tree is self-consistent.
# ---------------------------------------------------------------------------
echo "--- Phase 2: Production build (tsc + vite build) ---"

BUILD_OUTPUT=$(npm --prefix "$WEB_DIR" run build 2>&1)
BUILD_EXIT=$?

if [ $BUILD_EXIT -eq 0 ]; then
  pass "All ACs [REQ-019]: production build succeeds (tsc + vite) — all imports resolve and types are valid"
else
  fail "All ACs [REQ-019]: production build failed" \
       "Build errors detected — see output: $(echo "$BUILD_OUTPUT" | tail -20)"
fi

echo ""

# ---------------------------------------------------------------------------
# Phase 3: TypeScript typecheck
# tsc --noEmit with strict mode verifies no type errors exist independently of
# whether vite's build (which can tolerate some TS errors) passes.
# ---------------------------------------------------------------------------
echo "--- Phase 3: TypeScript typecheck (tsc --noEmit) ---"

TC_OUTPUT=$(npm --prefix "$WEB_DIR" run typecheck 2>&1)
TC_EXIT=$?

if [ $TC_EXIT -eq 0 ]; then
  pass "All ACs [REQ-019]: TypeScript typecheck passes with zero errors"
else
  fail "All ACs [REQ-019]: TypeScript typecheck has errors" \
       "$(echo "$TC_OUTPUT" | tail -20)"
fi

echo ""

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo "=== Results ==="
for r in "${RESULTS[@]}"; do
  echo "  $r"
done
echo ""
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo ""

if [ "$FAIL" -eq 0 ]; then
  echo "VERDICT: PASS"
  exit 0
else
  echo "VERDICT: FAIL"
  exit 1
fi
