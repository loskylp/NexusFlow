# Builder Handoff — HOTFIX: App.tsx router conversion
**Date:** 2026-04-09
**Task:** HOTFIX — App.tsx crashes on Pipeline Builder page in staging
**Requirement(s):** N/A (hotfix for staging crash)

## What Was Implemented

Single file changed: `web/src/App.tsx`

Converted the router setup from the legacy `BrowserRouter` + `Routes` pattern to
`createBrowserRouter` + `RouterProvider` (data router). This is the minimal mechanical
change required to resolve the runtime crash on `/pipelines`.

Specific changes:
- Replaced `import { BrowserRouter, Route, Routes, Navigate }` with
  `import { createBrowserRouter, RouterProvider, Navigate }`
- Defined a module-scope `router` constant using `createBrowserRouter([...])` with the
  same eight route definitions (paths, elements, wrappers) as before — no routes added,
  removed, or reordered
- `App` now returns `<AuthProvider><RouterProvider router={router} /></AuthProvider>`
  instead of the `BrowserRouter`/`Routes` tree
- Updated the file-level and `router` constant docstrings to explain why the data router
  is required (`useBlocker` on `PipelineManagerPage` requires data router context)

## Unit Tests

- Tests written: 0 (no new test logic — this is a router wiring change, not behavioral)
- All passing: yes — 574 tests across 28 test files pass without modification
- The existing `PipelineManagerPage.test.tsx` already uses `createMemoryRouter` /
  `RouterProvider`, confirming the page's tests were already written against the data
  router pattern; the production wiring now matches.

## Deviations from Task Description

None. All routes preserved exactly. `AuthProvider` wraps `RouterProvider`. `ProtectedRoute`
and `Layout` wrappers unchanged on every protected route.

## Known Limitations

None. The conversion is complete.

## For the Verifier

- Navigate to `/pipelines` in staging — the blank-page crash should be gone
- All other routes (`/login`, `/workers`, `/tasks`, `/tasks/logs`, `/demo/sink-inspector`,
  `/demo/chaos`, and the `*` 404 catch-all) should behave identically to before
- The `useBlocker` unsaved-changes guard on the Pipeline Builder should now function
  (previously it threw before even rendering)
