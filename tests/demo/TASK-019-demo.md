# Demo Script — TASK-019
**Feature:** React app shell with sidebar navigation and auth flow
**Requirement(s):** REQ-019, REQ-016, DEMO-003, DEMO-004
**Environment:** Staging — https://nexusflow.staging (or local: http://localhost:3000)

---

## Scenario 1: Login screen renders with username/password form
**REQ:** REQ-019

**Given:** you have navigated to the NexusFlow application URL and are not logged in

**When:** the page loads at `/login`

**Then:** a centered white card (400px wide) on a dark slate background is visible; the card shows the "NexusFlow" wordmark at the top; below the wordmark are two clearly labeled fields: "USERNAME" and "PASSWORD" (labels in small uppercase IBM Plex Sans); a "Sign In" button appears below the password field; no dashboard content is visible

---

## Scenario 2: Invalid credentials show inline error message
**REQ:** REQ-019

**Given:** you are on the `/login` page with the form showing

**When:** you type any username (e.g. "wronguser") and any password (e.g. "badpass") into the fields, then click "Sign In"

**Then:** the button label changes to "Signing in..." and becomes non-clickable while the request is in flight; after the request fails, an inline red error message reading "Invalid username or password." appears below the password field within the card; you remain on the login page; the form inputs retain the values you typed; no navigation occurs

**Notes:** The error text must appear within the card, not as a popup or toast. This is the UX spec's inline error pattern.

---

## Scenario 3: Admin login redirects to Worker Fleet Dashboard
**REQ:** REQ-019, REQ-016

**Given:** you are on the `/login` page; you have Admin credentials (default seed: username `admin`, password `admin`)

**When:** you enter the admin credentials and click "Sign In"

**Then:** after a brief moment, the browser navigates to `/workers`; the Worker Fleet Dashboard placeholder page is visible; the left sidebar is visible with "NexusFlow" at the top; all four primary nav items are listed: "Worker Fleet", "Task Feed", "Pipeline Builder", "Log Streamer"; below the primary items, under a "DEMO" label, two additional links appear: "Sink Inspector" and "Chaos Controller"; a "Log out" button appears at the bottom of the sidebar

---

## Scenario 4: User login redirects to Task Feed
**REQ:** REQ-019, REQ-016

**Given:** you are on the `/login` page; you have User-role credentials (create one if needed via the database, or use a seeded test user)

**When:** you enter the user credentials and click "Sign In"

**Then:** the browser navigates to `/tasks`; the Task Feed placeholder page is visible; the sidebar is visible with the four primary nav items ("Worker Fleet", "Task Feed", "Pipeline Builder", "Log Streamer"); the DEMO section (Sink Inspector, Chaos Controller) does NOT appear in the sidebar

**Notes:** The sidebar content is the primary observable difference between Admin and User sessions. Inspect carefully — the DEMO label and its two links must be absent.

---

## Scenario 5: Sidebar navigation visible on all authenticated views
**REQ:** REQ-019

**Given:** you are logged in as Admin and are on the Worker Fleet Dashboard (`/workers`)

**When:** you click each nav item in the sidebar in turn: "Task Feed", "Pipeline Builder", "Log Streamer", "Sink Inspector", "Chaos Controller", then "Worker Fleet" again

**Then:** each click navigates to the corresponding route (`/tasks`, `/pipelines`, `/tasks/logs`, `/demo/sink-inspector`, `/demo/chaos`, `/workers`); on every page, the sidebar remains fixed on the left side at 240px width with the dark slate background; the active nav item has an indigo left border highlight and slightly brighter text; all other items remain visible in the sidebar

---

## Scenario 6: Unauthenticated users redirected to /login
**REQ:** REQ-019

**Given:** you are not logged in (clear browser cookies/session storage to ensure no session exists)

**When:** you attempt to navigate directly to any protected route, e.g. `/workers` or `/tasks`

**Then:** the browser immediately redirects to `/login`; the login form is shown; no dashboard content flashes briefly before the redirect

**Notes:** To clear session: open DevTools, go to Application > Cookies, delete the `session` cookie for the NexusFlow domain. Then navigate directly to `/workers` in the address bar.

---

## Scenario 7: Design tokens applied globally — visual spot check
**REQ:** REQ-019

**Given:** you are logged in and on any authenticated page (e.g. `/workers`)

**When:** you open browser DevTools, select the `<body>` element, and inspect its Computed Styles; also inspect the `:root` element to view CSS custom properties

**Then:** the body `background-color` is `rgb(250, 250, 250)` (which is `#FAFAFA`, the `--color-surface-base` token); the sidebar background is `rgb(15, 23, 42)` (which is `#0F172A`, the `--sidebar-bg` token matching slate-900); the primary action color `--color-primary` is `#4F46E5` (indigo); the body font-family references Inter; the `:root` block lists spacing variables `--space-1` through `--space-16`

**Notes:** You can also type `getComputedStyle(document.documentElement).getPropertyValue('--color-primary')` in the browser console; the result must be `#4F46E5` (with possible surrounding whitespace).

---
