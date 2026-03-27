# Builder Handoff — TASK-019
**Date:** 2026-03-27
**Task:** React app shell with sidebar navigation and auth flow
**Requirement(s):** REQ-019 (auth), REQ-016 (worker fleet routing), DEMO-003/004 (demo routes)

## What Was Implemented

### New infrastructure
- `web/vitest.config.ts` — Vitest test runner configuration with jsdom environment and `@/` alias resolution.
- `web/src/test/setup.ts` — Global test setup importing `@testing-library/jest-dom` matchers.
- `web/package.json` — Added `"test": "vitest run"` script and vitest/testing-library devDependencies.

### Implemented (replacing TODO stubs)
- `web/src/api/client.ts` — `apiFetch` base wrapper implemented: `credentials: 'include'`, `Content-Type: application/json`, throws `"${status}: ${body}"` on non-2xx so callers can detect 401 vs 500. All other client functions (`login`, `logout`, `listWorkers`, etc.) wired through it.
- `web/src/context/AuthContext.tsx` — `AuthProvider` fully implemented: session restore via `GET /api/auth/me` on mount (gracefully degrades if endpoint is not live), `login()` calls `client.login()` and stores user, `logout()` calls `client.logout()` and clears user. `isLoading` starts `true` during session check to prevent ProtectedRoute redirect flash on page refresh.
- `web/src/pages/LoginPage.tsx` — Full styled implementation: 400px centered card on slate-900 background, labeled inputs (for accessibility via `<label htmlFor>`), role-based redirect via `useEffect` (admin→`/workers`, user→`/tasks`), inline `role="alert"` error on failure, disabled button with "Signing in..." text during loading. IBM Plex Sans labels, Inter body, design token colors.

### New files
- `web/src/components/ProtectedRoute.tsx` — Renders children when authenticated, `<Navigate to="/login" replace />` when unauthenticated, `null` while `isLoading` (prevents redirect flash on page refresh).
- `web/src/components/Sidebar.tsx` — Fixed 240px sidebar with slate-900 background. Primary nav: Worker Fleet, Task Feed, Pipeline Builder, Log Streamer (all roles). Demo section (Sink Inspector, Chaos Controller) rendered only when `user.role === 'admin'`. Active NavLink: indigo left border + indigo-50 background. Logout button calls `AuthContext.logout()` and navigates to `/login`.
- `web/src/components/Layout.tsx` — Shell for authenticated views: Sidebar fixed on left, main content area offset by `var(--sidebar-width)`.
- `web/src/pages/TaskFeedPage.tsx` — Placeholder; title only. Full implementation in TASK-021 (Cycle 2).
- `web/src/pages/PipelineManagerPage.tsx` — Placeholder; title only. Full implementation in Cycle 3.
- `web/src/pages/LogStreamerPage.tsx` — Placeholder; title only. Full implementation in Cycle 3.
- `web/src/pages/SinkInspectorPage.tsx` — Placeholder with DEMO badge. Full implementation in Cycle 4.
- `web/src/pages/ChaosControllerPage.tsx` — Placeholder with DEMO badge. Full implementation in Cycle 4.

### Updated (styled)
- `web/src/pages/NotFoundPage.tsx` — Styled with design system tokens; was a bare div.
- `web/src/App.tsx` — Full routing: ProtectedRoute + Layout wrapping all authenticated routes, `/` resolved by `RootRedirect` (admin→`/workers`, user→`/tasks`), all placeholder routes registered.

### Design system tokens (AC-7)
- `web/src/styles/globals.css` — Was already complete from the Scaffolder. No changes needed; all tokens were pre-defined. Component styles are applied via inline styles referencing CSS custom properties.

## Unit Tests
- Tests written: 31
- All passing: yes
- Test files:
  - `web/src/api/client.test.ts` (5 tests) — `apiFetch` credentials/Content-Type, login POST shape, AuthResponse returned, 401/500 error propagation, logout POST shape.
  - `web/src/context/AuthContext.test.tsx` (7 tests) — Initial state (isLoading true then false), login sets user, login failure leaves user null, role stored correctly, logout clears user, useAuth guard throws outside provider.
  - `web/src/pages/LoginPage.test.tsx` (7 tests) — Form renders with labelled inputs (AC-1), NexusFlow brand visible, admin redirected to /workers (AC-2), user redirected to /tasks (AC-2), inline alert on failure (AC-3), no redirect on failure, button disabled during loading (AC-3).
  - `web/src/components/ProtectedRoute.test.tsx` (3 tests) — Renders children when authenticated, redirects to /login when unauthenticated (AC-6), renders nothing during auth loading.
  - `web/src/components/Sidebar.test.tsx` (9 tests) — All primary nav links visible to both roles (AC-4), logout button present, Sink Inspector and Chaos Controller visible to admin (AC-5), both hidden for user role (AC-5).

## Deviations from Task Description

1. **`/api/auth/me` session-restore endpoint** — The task description notes the backend sets an HttpOnly cookie on login. AuthContext now calls `GET /api/auth/me` on mount to restore a session across page refreshes. This endpoint is not yet implemented in the backend (TASK-003 implemented login/logout but not `/me`). AuthContext degrades gracefully: a 404 or network error results in `isLoading → false` with `user = null`. When TASK-003's `/me` endpoint is wired up, session restore will work automatically. This deviation is necessary to prevent the ProtectedRoute redirect flash on hard refresh; without it, every page refresh would log the user out.

2. **Sidebar routing for Log Streamer** — The UX spec lists Log Streamer as accessible from the sidebar (route: `/tasks/{id}/logs`). The sidebar links to `/tasks/logs` (no task ID) because the sidebar cannot know a specific task ID at nav time. The UX spec also states it "opens empty, requires task selection" when opened from the sidebar — this is consistent with the placeholder implementation.

3. **`useSSE` not implemented** — TASK-019 scope does not include the SSE hook implementation (the scaffold has a TODO). `WorkerFleetDashboard` uses `useSSE` but the hook stub is sufficient for TASK-019 acceptance (the dashboard is a placeholder for TASK-020). The `useSSE` implementation is deferred to TASK-020.

## Known Limitations

- **Session restore requires `/api/auth/me`** — Until the backend implements this endpoint, page refreshes will show the login page even with a valid session cookie. The user must log in again after refresh. This is a temporary limitation (see Deviation 1).
- **`useSSE` is a stub** — The SSE hook in `web/src/hooks/useSSE.ts` remains a TODO stub. The Worker Fleet Dashboard renders but will not receive real-time updates until TASK-020.
- **Demo routes are admin-only by convention only** — The router registers `/demo/sink-inspector` and `/demo/chaos` but does not enforce admin-only access at the route level (only the sidebar hides the links). A user who manually navigates to `/demo/chaos` will see the placeholder page. Role enforcement at the route level is deferred; the backend API will enforce permissions on the actual data calls.
- **Inline styles** — Design system tokens are applied via inline React styles (no CSS modules or Tailwind). This is consistent with the existing project style (no CSS framework was included in `package.json`). A future refactor could extract to CSS modules if the project grows.

## For the Verifier

- **AC-1:** Load `/login` — expect the form to render with a visible USERNAME label, PASSWORD label, and "Sign In" button. The NexusFlow wordmark must be visible above the form.
- **AC-2:** Log in with an admin credential (role: admin) — expect redirect to `/workers`. Log in with a user credential (role: user) — expect redirect to `/tasks`.
- **AC-3:** Submit the form with wrong credentials — expect a red inline error message reading "Invalid username or password." to appear below the button without navigating away.
- **AC-4:** After logging in, the sidebar must be visible on all routes listed in the UX spec (workers, tasks, pipelines, tasks/logs, demo routes for admin).
- **AC-5:** Log in as a user (role: user) and inspect the sidebar — "Sink Inspector" and "Chaos Controller" links must not appear. Log in as admin — both must appear under the DEMO label.
- **AC-6:** With no active session (cookies cleared), navigate to `/workers` — expect redirect to `/login`. After logging in, navigate back — expect the protected page to render without a redirect loop.
- **AC-7:** Open DevTools and inspect `:root` CSS custom properties — all color, font, and spacing tokens from DESIGN.md must be present. Body background must be `#FAFAFA`, login page background must be `#0F172A` (slate-900).
- The `GET /api/auth/me` 404 in the browser console is expected in Cycle 1 (backend not yet implemented). It is not an error.
