/**
 * SinkInspectorPage — Admin-only demo view showing Before/After sink destination
 * state comparison with atomicity verification.
 *
 * Layout:
 *   - Header: "Sink Inspector" title, "DEMO" badge, monitoring status
 *   - Task selector: dropdown of recent tasks
 *   - Split panel (50/50): Before Snapshot (left), After Result (right)
 *   - Atomicity verification section below panels
 *
 * SSE subscription: when a task is selected, subscribes to
 *   GET /events/sink/{taskId}
 * and populates panels from incoming sink:before-snapshot and sink:after-result events.
 *
 * Access: Admin only. The SSE endpoint returns 403 for non-admin callers; this is
 * surfaced as an access error message in the view.
 *
 * See: DEMO-003, UX Spec (Sink Inspector), TASK-032, TASK-033
 */

import React from 'react'
import type { Task, SinkSnapshot } from '@/types/domain'
import type { UseSinkInspectorReturn } from '@/hooks/useSinkInspector'

// ---------------------------------------------------------------------------
// Sub-component contracts
// ---------------------------------------------------------------------------

/**
 * SinkInspectorHeaderProps feeds the page header.
 */
interface SinkInspectorHeaderProps {
  /** SSE connection status for the currently selected task. */
  sseStatus: UseSinkInspectorReturn['sseStatus']
}

/**
 * SinkInspectorHeader renders the page title with DEMO badge and monitoring
 * status indicator.
 *
 * @param sseStatus - Used to show a connection status dot next to the title.
 *
 * Postconditions:
 *   - DEMO badge is always visible.
 *   - Status dot color reflects sseStatus: green=connected, yellow=reconnecting,
 *     red=error, grey=idle/connecting.
 */
function SinkInspectorHeader({ sseStatus }: SinkInspectorHeaderProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------

/**
 * TaskSelectorProps feeds the task selector dropdown.
 */
interface TaskSelectorProps {
  /** Available tasks for selection (recent tasks visible to admin). */
  tasks: Task[]
  /** The currently selected task ID, or null if none is selected. */
  selectedTaskId: string | null
  /** Called when the user selects a task from the dropdown. */
  onSelect: (taskId: string | null) => void
  /** True while the task list is loading. */
  isLoading: boolean
}

/**
 * TaskSelector renders the task selection dropdown above the split panels.
 *
 * @param tasks        - List of tasks to show in the dropdown.
 * @param selectedTaskId - Currently selected task ID.
 * @param onSelect     - Callback when selection changes.
 * @param isLoading    - Disables the dropdown while loading.
 *
 * Postconditions:
 *   - Includes a "Select a task..." placeholder option with null value.
 *   - Disabled when isLoading is true.
 */
function TaskSelector({ tasks, selectedTaskId, onSelect, isLoading }: TaskSelectorProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------

/**
 * SnapshotPanelProps feeds a single Before or After snapshot panel.
 */
interface SnapshotPanelProps {
  /** Panel title: "Before Snapshot" or "After Result". */
  title: string
  /** The snapshot to display. null means not yet received. */
  snapshot: SinkSnapshot | null
  /** True while waiting for this snapshot (shows spinner + waiting message). */
  isWaiting: boolean
  /** When true, new/changed items are highlighted with green-50 background. */
  highlightChanges?: boolean
  /** When true, displays a "ROLLED BACK" badge in red. */
  rolledBack?: boolean
}

/**
 * SnapshotPanel renders a scrollable data table of snapshot key-value pairs.
 *
 * Default state (no task selected):
 *   - Shows "Select a task to inspect its sink operation" placeholder text.
 *
 * Waiting state (isWaiting is true):
 *   - Shows "Waiting for sink phase to begin..." with a spinner.
 *
 * Snapshot received state:
 *   - Renders snapshot.data as a table with monospace font.
 *   - When highlightChanges is true, new/changed rows have a green-50 background.
 *   - When rolledBack is true, displays "ROLLED BACK" badge above the table.
 *
 * Postconditions:
 *   - Data values rendered in monospace font.
 *   - Table is scrollable when content overflows.
 */
function SnapshotPanel({
  title,
  snapshot,
  isWaiting,
  highlightChanges,
  rolledBack,
}: SnapshotPanelProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------

/**
 * AtomicityVerificationProps feeds the atomicity verification section.
 */
interface AtomicityVerificationProps {
  /**
   * The verification state:
   *   - 'pending': neither snapshot has been received yet.
   *   - 'success': After snapshot received, write succeeded.
   *   - 'rolled-back': After snapshot received, write failed and rolled back.
   *   - 'waiting': Before snapshot received, waiting for After.
   */
  state: 'pending' | 'success' | 'rolled-back' | 'waiting'
  /** The write error message, if any. Empty string or null on success. */
  writeError: string | null
}

/**
 * AtomicityVerification renders the atomicity verification result below the
 * split panels.
 *
 * Success state:
 *   - Green checkmark icon + "Atomicity verified: write committed successfully"
 *
 * Rolled-back state:
 *   - Red X icon + "Atomicity verified: write rolled back — destination unchanged"
 *   - Shows writeError detail if non-empty.
 *
 * Waiting state:
 *   - Spinner + "Waiting for sink phase to complete..."
 *
 * Pending state:
 *   - No content (returns null or empty).
 *
 * Postconditions:
 *   - Never returns an error. Renders the appropriate state regardless of
 *     snapshot content.
 */
function AtomicityVerification({ state, writeError }: AtomicityVerificationProps): React.ReactElement | null {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

/**
 * SinkInspectorPage is the root component for the Sink Inspector demo view.
 *
 * Orchestrates:
 *   - Task list fetch via useTasks (for the selector dropdown)
 *   - SSE snapshot subscription via useSinkInspector
 *   - Before/After panel rendering
 *   - Atomicity verification display
 *
 * Access guard: if the current user is not an Admin (surfaced via useSinkInspector
 * accessError), renders a 403 access denied message instead of the view content.
 *
 * Route: /demo/sink-inspector
 *
 * Postconditions:
 *   - Selecting a task immediately subscribes to the SSE channel.
 *   - Changing the selected task resets all snapshot state.
 *   - Non-admin users see an access denied message, not the data panels.
 */
function SinkInspectorPage(): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

export default SinkInspectorPage
