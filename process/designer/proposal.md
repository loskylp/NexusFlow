# Design Proposal -- NexusFlow Web GUI

**Stitch Project:** projects/14608407312724823932
**Status:** Pending Review

## Screens

### 1. NexusFlow Login Screen
- **Stitch ID:** `4a0e7124d95b472f867e40bdfc632029`
- **Requirement:** REQ-019 (User authentication and role-based access)
- **Description:** Centered login card on light gray background. Username/password form with indigo primary action. No decorative elements -- operational tool aesthetic.

---

### 2. Worker Fleet Dashboard
- **Stitch ID:** `0a5771cf643c4de6ac720b66fe6ba593`
- **Requirement:** REQ-016 (Worker Fleet Dashboard)
- **Description:** Dark sidebar navigation, summary cards (Total/Online/Down/Avg Load), full data table with worker status, capability tags as colored pills, real-time SSE connection indicator. Down workers highlighted with red-50 background.

---

### 3. Task Feed and Monitor
- **Stitch ID:** `ba2a20e0e74d4c7bb20ea261bdb4deb0`
- **Requirement:** REQ-017 (Task Feed and Monitor), REQ-002 (Submit task via GUI), REQ-010 (Cancel task)
- **Description:** Vertical card feed of tasks with status badges using the full task state color map. Filter bar with status/pipeline/search. Submit Task button. Each task card shows ID, pipeline, status badge, worker, timing, and action buttons (View Logs, Cancel, Retry). Role-based visibility: Admin sees all tasks, User sees own tasks.

---

### 4. Pipeline Builder
- **Stitch ID:** `cfbc8cf895f04d7eb468d7aa93d2cbf8`
- **Requirement:** REQ-015 (Pipeline Builder), REQ-007 (Schema mapping)
- **Description:** Component palette on the left with draggable DataSource/Process/Sink cards. Canvas with dot-grid background showing connected pipeline nodes. Each node has a phase-colored header (blue/purple/green). Schema mapping chips between nodes. Toolbar with Save/Run/Clear actions. Saved pipelines list in the palette.

---

### 5. Log Streamer Dashboard
- **Stitch ID:** `b466c2648ab245008d3b6e20f06256b2`
- **Requirement:** REQ-018 (Real-time log streaming)
- **Description:** Task selector bar with phase filter toggles and auto-scroll control. Dark terminal-style log output panel with monospace text. Log lines color-coded by level (INFO/WARN/ERROR) and phase (datasource/process/sink/mapping). SSE connection status in bottom bar with Last-Event-ID.

---

### 6. Sink Inspector Dashboard
- **Stitch ID:** `4ccd7b8c9a2a45beb930be5a5ce93e38`
- **Requirement:** DEMO-003 (Sink Inspector)
- **Description:** Side-by-side Before/After comparison panels. Task selector at top. Before panel shows pre-execution destination state. After panel shows post-execution state with new items highlighted in green-50. Atomicity verification section with checkmark/cross and details. DEMO badge in header.

---

### 7. Chaos Controller
- **Stitch ID:** `6a6ec21065d2417bad277dcd401c3626`
- **Requirement:** DEMO-004 (Chaos Controller)
- **Description:** Three action cards: Kill Worker (worker selector + red kill button), Disconnect Database (duration selector + red disconnect button), Flood Queue (task count + pipeline selector + amber submit burst button). Each card has expected result description and activity log. DEMO and DESTRUCTIVE badges in header. Confirmation required on destructive actions.

---

## Review Checklist

The Nexus should verify:

1. **Consistent sidebar navigation** across all views (same dark sidebar, same nav items, active state highlighting)
2. **Status badge color consistency** with the defined task state color map (submitted=violet, queued=amber, running=blue, completed=green, failed=red, cancelled=gray)
3. **Worker status indicators** (green dot = online, red = down)
4. **Information density** -- operational dashboards should be dense and scannable, not spacious
5. **Phase color coding** in Pipeline Builder (DataSource=blue, Process=purple, Sink=green) carries through to Log Streamer phase tags
6. **Demo views** clearly labeled with DEMO badge to distinguish from production views
7. **Log Streamer** dark terminal aesthetic with monospace text
8. **Overall visual consistency** -- same border style (1px #E2E8F0), same card radius (8px), same font pairing (Inter + IBM Plex Sans) across all screens
