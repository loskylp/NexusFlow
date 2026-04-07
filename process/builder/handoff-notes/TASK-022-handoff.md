# Builder Handoff — TASK-022
**Date:** 2026-04-07
**Task:** Log Streamer (GUI)
**Requirement(s):** REQ-018, UX Spec (Log Streamer), ADR-007

## What Was Implemented

### `web/src/hooks/useLogs.ts`
Full implementation replacing the scaffold stub. Streams log lines via SSE from
`GET /events/tasks/{taskId}/logs`. Key behaviors:

- Idle when `taskId` is undefined (SSE disabled, no connection)
- Accumulates `log:line` SSE events in order into the `lines` array
- Tracks `lastEventId` from each event's `id` field for status bar display
  (the browser's native `EventSource` handles `Last-Event-ID` header automatically
  on reconnection — no manual injection needed)
- Resets the buffer (lines + lastEventId) when `taskId` changes
- `clearLines()` empties the visual buffer while preserving `lastEventId`
- Surfaces `log:error` SSE events as `accessError` (403/access denied scenario)
- `enabled=false` prop disconnects without clearing lines

### `web/src/pages/LogStreamerPage.tsx`
Full implementation replacing the scaffold stub. Composes `useLogs`, `useTasks`, and
the download trigger.

**Exported functions and components:**
- `filterLogLines(lines, phase)` — pure function; filters by `[datasource]`, `[process]`,
  or `[sink]` tag in the log line text; returns same reference when `phase='all'`
- `LogLine` — renders a single log line with colored phase badge, timestamp, and message;
  ERROR level lines rendered in red
- `LogPanel` — dark terminal panel; auto-scrolls to bottom sentinel when `autoScroll=true`
  and new lines arrive; guards `scrollIntoView` call for test environments
- `LogStatusBar` — bottom status bar with SSE dot, status label, line count, Last-Event-ID
- `LogStreamerPage` (default export) — root page; reads `?taskId` query param on mount to
  pre-select a task; task selector dropdown populated from `useTasks`
- `PhaseFilterToggleGroup` (internal) — All / DataSource / Process / Sink toggle buttons

**AC coverage:**
1. Selecting a task initiates SSE connection and streams in real time — useLogs + LogPanel
2. Phase filter toggles show/hide lines client-side — filterLogLines + PhaseFilterToggleGroup
3. Phase tags color-coded — PHASE_COLORS constants per DESIGN.md
4. Auto-scroll follows new lines; toggling off allows scroll-back — autoScroll state + LogPanel
5. Download Logs fetches from REST and triggers browser download — handleDownload
6. SSE disconnection reconnects with Last-Event-ID replay — native EventSource behavior; lastEventId displayed in status bar
7. 403 access denied shows error in log panel, not a redirect — accessError from useLogs
8. Log lines include timestamp, level phase tag, and message — LogLine component

**Route:** Uses existing `/tasks/logs?taskId=<uuid>` route from App.tsx (scaffold alignment
per scaffold-manifest.md boundary ambiguity note).

## Unit Tests

- Tests written: 60 (22 for `useLogs`, 38 for `LogStreamerPage`)
- All passing: yes
- Key behaviors covered:
  - `useLogs`: idle state, SSE URL construction, log line accumulation, Last-Event-ID tracking,
    `clearLines` preserves lastEventId, taskId change resets buffer, access error surfacing,
    enabled prop, SSE status passthrough
  - `filterLogLines`: all-phases passthrough, datasource/process/sink filtering, empty input,
    no-match case, reference identity for `all` phase
  - `LogLine`: phase tag rendering and color, timestamp presence, error level
  - `LogStatusBar`: connected/reconnecting/complete states, line count, Last-Event-ID display
  - `LogStreamerPage`: URL query param pre-selection, log line rendering, phase filter toggles,
    access error in panel, download trigger, clear button, auto-scroll toggle, SSE status bar

## Deviations from Task Description

1. **No REST seed for initial log history.** The task description mentioned "REST seed: fetch
   existing logs from `GET /api/tasks/{id}/logs`". After reviewing the backend (ADR-007 and
   the `downloadTaskLogs` function), the REST endpoint returns all logs as raw text (not JSON
   array), making it unsuitable as a seed for individual line rendering. The SSE endpoint
   `GET /events/tasks/{id}/logs` replays all historical log lines on initial connection
   (via Last-Event-ID replay on first connect). The download button fetches the raw text for
   file download only. This is consistent with ADR-007's reconnection strategy. If the Verifier
   needs per-line historical seed via REST, a JSON variant of `GET /api/tasks/{id}/logs` would
   need to be implemented in the backend first.

2. **Phase detection via line text, not a dedicated field.** The `TaskLog` domain type has no
   `phase` field — only `id`, `taskId`, `line`, `level`, and `timestamp`. Phase is detected
   by presence of `[datasource]`, `[process]`, or `[sink]` tags in the `line` text. This
   matches the log format documented in the scaffold manifest (LogLine comment: "Format:
   `<timestamp> [<phase>] [<level>] <message>`") and the UX spec.

3. **log:error for 403 surfacing.** The `useLogs` hook surfaces `403` errors via a synthetic
   `log:error` SSE event type. The actual backend behavior when access is denied (whether it
   returns HTTP 403 on the SSE endpoint or sends a `log:error` event) is tested at the
   integration level. The hook handles both: if the EventSource itself fails (HTTP 403),
   `useSSE` will set status to `reconnecting`; the access error message in the panel is
   triggered by a `log:error` event type.

## Known Limitations

1. **Task selector dropdown is populated from `useTasks`.** This fetches the user's task list.
   For the Log Streamer when opened directly from the sidebar (not from Task Feed), the
   dropdown may show limited tasks depending on filter state. The URL query param pre-selection
   works correctly regardless.

2. **scrollIntoView is guarded for test environments.** jsdom does not implement
   `scrollIntoView`, so the auto-scroll behavior is guarded with a `typeof` check. The
   auto-scroll behavior is functional in a real browser.

3. **No REST seed for initial log history.** See deviation #1 above.

## For the Verifier

- Route is `/tasks/logs?taskId=<uuid>` (query param approach, not `/tasks/:id/logs`). This
  aligns with the existing App.tsx route table and the scaffold manifest boundary ambiguity note.
- The "View Logs" button in TaskFeedPage navigates to `/tasks/logs?taskId=${taskId}` — the
  Log Streamer reads this param on mount and pre-selects the task.
- 403 access denied is tested at the hook level via the `log:error` SSE event convention;
  integration test should verify the actual backend sends either HTTP 403 on the stream or
  a `log:error` SSE event.
- All 411 unit tests in the web/ test suite pass. The 1 error in
  `tests/acceptance/TASK-023-acceptance.test.tsx` is pre-existing and outside TASK-022 scope.
- TypeScript compilation (`npm run build`) is clean with no errors or warnings on project source.
