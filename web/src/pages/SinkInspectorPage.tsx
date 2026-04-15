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
 * Access: Admin only. Non-admin users see an access denied message.
 *
 * See: DEMO-003, UX Spec (Sink Inspector), TASK-032, TASK-033
 */

import React, { useCallback, useState } from 'react'
import { useAuth } from '@/context/AuthContext'
import { useSinkInspector } from '@/hooks/useSinkInspector'
import { useTasks } from '@/hooks/useTasks'
import type { SinkSnapshot } from '@/types/domain'
import type { UseSinkInspectorReturn } from '@/hooks/useSinkInspector'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SinkInspectorHeaderProps {
  sseStatus: UseSinkInspectorReturn['sseStatus']
}

interface TaskSelectorProps {
  selectedTaskId: string | null
  onChange: (taskId: string | null) => void
}

interface SnapshotPanelProps {
  title: string
  snapshot: SinkSnapshot | null
  /** When true, the panel shows the "Waiting for sink phase" spinner. */
  isWaiting?: boolean
  /** When true, new/changed items are highlighted with a green-50 tint. */
  highlightChanges?: boolean
  /** The Before snapshot used to detect changed keys in the After panel. */
  beforeSnapshotForDiff?: SinkSnapshot | null
  /** Empty state message shown when not waiting and no snapshot is available. */
  emptyMessage: string
}

interface AtomicityVerificationProps {
  beforeSnapshot: SinkSnapshot | null
  afterSnapshot: SinkSnapshot | null
  rolledBack: boolean
  writeError: string | null
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/**
 * SinkInspectorHeader renders the page title, DEMO badge, and SSE monitoring status.
 *
 * @param sseStatus - Current SSE connection status for the monitoring indicator.
 */
function SinkInspectorHeader({ sseStatus }: SinkInspectorHeaderProps): React.ReactElement {
  const isConnected = sseStatus === 'connected'
  const isReconnecting = sseStatus === 'reconnecting'

  const dotColor = isConnected
    ? '#16A34A'
    : isReconnecting
      ? '#DC2626'
      : sseStatus === 'idle'
        ? '#64748B'
        : '#D97706'

  const statusLabel = sseStatus === 'idle'
    ? 'No task selected'
    : isConnected
      ? 'Monitoring'
      : isReconnecting
        ? 'Reconnecting...'
        : 'Connecting...'

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        flexWrap: 'wrap',
      }}
    >
      <h1
        style={{
          fontSize: '20px',
          fontWeight: 600,
          color: 'var(--color-text-primary)',
          margin: 0,
        }}
      >
        Sink Inspector
      </h1>

      {/* DEMO badge */}
      <span
        style={{
          padding: '2px 8px',
          borderRadius: '4px',
          fontSize: '11px',
          fontWeight: 600,
          letterSpacing: '0.06em',
          textTransform: 'uppercase',
          backgroundColor: '#7C3AED20',
          color: '#7C3AED',
          border: '1px solid #7C3AED40',
        }}
      >
        DEMO
      </span>

      {/* Monitoring status indicator */}
      <span
        role="status"
        aria-live="polite"
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '6px',
          fontSize: '13px',
          color: 'var(--color-text-secondary)',
        }}
      >
        <span
          style={{
            display: 'inline-block',
            width: '8px',
            height: '8px',
            borderRadius: '50%',
            backgroundColor: dotColor,
          }}
        />
        {statusLabel}
      </span>
    </div>
  )
}

/**
 * TaskSelector renders the task dropdown for selecting which task to inspect.
 * Populates from the live task list via useTasks.
 *
 * @param selectedTaskId - Currently selected task ID or null.
 * @param onChange       - Called with the new task ID (null when cleared).
 */
function TaskSelector({ selectedTaskId, onChange }: TaskSelectorProps): React.ReactElement {
  const { tasks } = useTasks()

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
      <label
        htmlFor="task-selector"
        style={{
          fontSize: '13px',
          color: 'var(--color-text-secondary)',
          fontWeight: 500,
          flexShrink: 0,
        }}
      >
        Task:
      </label>
      <select
        id="task-selector"
        aria-label="Select task"
        value={selectedTaskId ?? ''}
        onChange={e => onChange(e.target.value || null)}
        style={{
          height: '34px',
          padding: '0 10px',
          fontSize: '13px',
          border: '1px solid var(--color-border)',
          borderRadius: '6px',
          backgroundColor: 'var(--color-surface-subtle)',
          color: 'var(--color-text-primary)',
          outline: 'none',
          minWidth: '240px',
        }}
      >
        <option value="">Select a task to inspect...</option>
        {tasks.map(t => (
          <option key={t.id} value={t.id}>
            {t.id.slice(0, 8)}... — {t.status}
          </option>
        ))}
      </select>
    </div>
  )
}

/**
 * SnapshotDataTable renders the key-value data from a SinkSnapshot in a
 * scrollable monospace table. Changed keys are highlighted with green-50 tint
 * when highlightChanges is true and beforeSnapshotForDiff is provided.
 *
 * @param data                - The snapshot data to display.
 * @param highlightChanges    - Whether to highlight changed/new keys.
 * @param beforeSnapshotForDiff - Before snapshot used to detect changed keys.
 */
function SnapshotDataTable({
  data,
  highlightChanges,
  beforeSnapshotForDiff,
}: {
  data: Record<string, unknown>
  highlightChanges?: boolean
  beforeSnapshotForDiff?: SinkSnapshot | null
}): React.ReactElement {
  const entries = Object.entries(data)

  if (entries.length === 0) {
    return (
      <p
        style={{
          fontSize: '13px',
          color: '#64748B',
          fontFamily: 'var(--font-mono, "JetBrains Mono", monospace)',
          margin: 0,
        }}
      >
        (empty)
      </p>
    )
  }

  return (
    <div
      style={{
        overflowX: 'auto',
        overflowY: 'auto',
        maxHeight: '300px',
      }}
    >
      <table
        style={{
          width: '100%',
          borderCollapse: 'collapse',
          fontFamily: 'var(--font-mono, "JetBrains Mono", monospace)',
          fontSize: '12px',
        }}
      >
        <thead>
          <tr>
            <th
              style={{
                textAlign: 'left',
                padding: '4px 8px',
                borderBottom: '1px solid var(--color-border)',
                color: 'var(--color-text-secondary)',
                fontWeight: 600,
                fontSize: '11px',
                textTransform: 'uppercase',
                letterSpacing: '0.04em',
              }}
            >
              Key
            </th>
            <th
              style={{
                textAlign: 'left',
                padding: '4px 8px',
                borderBottom: '1px solid var(--color-border)',
                color: 'var(--color-text-secondary)',
                fontWeight: 600,
                fontSize: '11px',
                textTransform: 'uppercase',
                letterSpacing: '0.04em',
              }}
            >
              Value
            </th>
          </tr>
        </thead>
        <tbody>
          {entries.map(([key, value]) => {
            const isChanged = highlightChanges && beforeSnapshotForDiff
              ? JSON.stringify(beforeSnapshotForDiff.data[key]) !== JSON.stringify(value)
              : false
            const isNew = highlightChanges && beforeSnapshotForDiff
              ? !(key in beforeSnapshotForDiff.data)
              : false
            const shouldHighlight = isChanged || isNew

            return (
              <tr
                key={key}
                style={{
                  backgroundColor: shouldHighlight ? '#F0FDF4' : 'transparent',
                }}
              >
                <td
                  style={{
                    padding: '4px 8px',
                    color: shouldHighlight ? '#15803D' : 'var(--color-text-primary)',
                    borderBottom: '1px solid var(--color-border)',
                    verticalAlign: 'top',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {key}
                  {isNew && (
                    <span
                      style={{
                        marginLeft: '4px',
                        fontSize: '10px',
                        color: '#15803D',
                        fontWeight: 600,
                      }}
                    >
                      NEW
                    </span>
                  )}
                </td>
                <td
                  style={{
                    padding: '4px 8px',
                    color: shouldHighlight ? '#15803D' : 'var(--color-text-secondary)',
                    borderBottom: '1px solid var(--color-border)',
                    wordBreak: 'break-all',
                  }}
                >
                  {JSON.stringify(value)}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

/**
 * SnapshotPanel renders one half of the split panel view (Before or After).
 * Handles all four panel states per the UX spec:
 *   - Default: placeholder text
 *   - Waiting: spinner with "Waiting for sink phase to begin..."
 *   - Populated: snapshot data table
 *
 * @param title                - "Before Snapshot" or "After Result"
 * @param snapshot             - The SinkSnapshot to render, or null.
 * @param isWaiting            - Whether to show the waiting spinner.
 * @param highlightChanges     - Whether to highlight new/changed keys.
 * @param beforeSnapshotForDiff - Before snapshot for diff computation.
 * @param emptyMessage         - Placeholder text when no snapshot is available.
 */
export function SnapshotPanel({
  title,
  snapshot,
  isWaiting,
  highlightChanges,
  beforeSnapshotForDiff,
  emptyMessage,
}: SnapshotPanelProps): React.ReactElement {
  return (
    <div
      style={{
        flex: 1,
        border: '1px solid var(--color-border)',
        borderRadius: '8px',
        overflow: 'hidden',
        backgroundColor: 'var(--color-surface-panel)',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {/* Panel header */}
      <div
        style={{
          padding: '12px 16px',
          borderBottom: '1px solid var(--color-border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          backgroundColor: 'var(--color-surface-subtle)',
        }}
      >
        <span
          style={{
            fontSize: '13px',
            fontWeight: 600,
            color: 'var(--color-text-primary)',
            textTransform: 'uppercase',
            letterSpacing: '0.04em',
          }}
        >
          {title}
        </span>
        {snapshot && (
          <span
            style={{
              fontSize: '11px',
              color: '#64748B',
              fontFamily: 'var(--font-mono, "JetBrains Mono", monospace)',
            }}
          >
            {new Date(snapshot.capturedAt).toLocaleTimeString('en-US', { hour12: false })}
          </span>
        )}
      </div>

      {/* Panel body */}
      <div
        style={{
          padding: '16px',
          flex: 1,
          overflowY: 'auto',
          minHeight: '200px',
        }}
      >
        {/* Waiting state */}
        {isWaiting && !snapshot && (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: '10px',
              height: '100%',
              minHeight: '160px',
              color: '#64748B',
              fontSize: '13px',
            }}
          >
            <span
              aria-label="Loading"
              role="progressbar"
              style={{
                display: 'inline-block',
                width: '16px',
                height: '16px',
                borderRadius: '50%',
                border: '2px solid #CBD5E1',
                borderTopColor: '#4F46E5',
                animation: 'spin 0.8s linear infinite',
              }}
            />
            Waiting for sink phase to begin...
          </div>
        )}

        {/* Empty / default state */}
        {!isWaiting && !snapshot && (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              minHeight: '160px',
              color: '#64748B',
              fontSize: '13px',
              textAlign: 'center',
            }}
          >
            {emptyMessage}
          </div>
        )}

        {/* Populated state */}
        {snapshot && (
          <SnapshotDataTable
            data={snapshot.data}
            highlightChanges={highlightChanges}
            beforeSnapshotForDiff={beforeSnapshotForDiff}
          />
        )}
      </div>
    </div>
  )
}

/**
 * AtomicityVerification renders the result section below the split panels.
 * Shows a green checkmark on success, a "ROLLED BACK" badge on rollback,
 * or a neutral placeholder when the after result has not arrived yet.
 *
 * @param beforeSnapshot - The Before snapshot (null until received).
 * @param afterSnapshot  - The After snapshot (null until received).
 * @param rolledBack     - Whether the write was rolled back.
 * @param writeError     - The error message from a failed write, or null.
 */
export function AtomicityVerification({
  beforeSnapshot,
  afterSnapshot,
  rolledBack,
  writeError,
}: AtomicityVerificationProps): React.ReactElement {
  const hasResult = afterSnapshot !== null

  // Delta summary: count changed and new keys for display.
  const deltaCount = (() => {
    if (!beforeSnapshot || !afterSnapshot) return { changed: 0, added: 0 }
    const beforeKeys = Object.keys(beforeSnapshot.data)
    const afterEntries = Object.entries(afterSnapshot.data)
    let changed = 0
    let added = 0
    for (const [key, value] of afterEntries) {
      if (!(key in beforeSnapshot.data)) {
        added++
      } else if (JSON.stringify(beforeSnapshot.data[key]) !== JSON.stringify(value)) {
        changed++
      }
    }
    // Also count keys removed (present in before, absent in after).
    for (const key of beforeKeys) {
      if (!(key in afterSnapshot.data)) changed++
    }
    return { changed, added }
  })()

  return (
    <div
      style={{
        border: '1px solid var(--color-border)',
        borderRadius: '8px',
        padding: '16px',
        backgroundColor: 'var(--color-surface-panel)',
      }}
    >
      <h2
        style={{
          fontSize: '13px',
          fontWeight: 600,
          color: 'var(--color-text-secondary)',
          textTransform: 'uppercase',
          letterSpacing: '0.04em',
          margin: '0 0 12px 0',
        }}
      >
        Atomicity Verification
      </h2>

      {/* No result yet — neutral placeholder */}
      {!hasResult && (
        <p
          style={{
            fontSize: '13px',
            color: '#64748B',
            margin: 0,
          }}
        >
          Awaiting sink phase completion...
        </p>
      )}

      {/* Success */}
      {hasResult && !rolledBack && (
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px', flexWrap: 'wrap' }}>
          <span
            aria-label="Atomicity verified"
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: '28px',
              height: '28px',
              borderRadius: '50%',
              backgroundColor: '#D1FAE5',
              color: '#059669',
              fontSize: '16px',
              flexShrink: 0,
            }}
          >
            ✓
          </span>
          <div>
            <p
              style={{
                fontSize: '13px',
                fontWeight: 600,
                color: '#059669',
                margin: '0 0 4px 0',
              }}
            >
              Write committed successfully
            </p>
            {(deltaCount.changed > 0 || deltaCount.added > 0) && (
              <p
                style={{
                  fontSize: '12px',
                  color: 'var(--color-text-secondary)',
                  margin: 0,
                }}
              >
                Delta: {deltaCount.changed} changed, {deltaCount.added} new
              </p>
            )}
          </div>
        </div>
      )}

      {/* Rollback */}
      {hasResult && rolledBack && (
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px', flexWrap: 'wrap' }}>
          <span
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              padding: '4px 10px',
              borderRadius: '4px',
              backgroundColor: '#FEE2E2',
              color: '#DC2626',
              fontSize: '12px',
              fontWeight: 700,
              letterSpacing: '0.06em',
              textTransform: 'uppercase',
              flexShrink: 0,
              border: '1px solid #FECACA',
            }}
            role="status"
            aria-label="ROLLED BACK"
          >
            ROLLED BACK
          </span>
          <div>
            <p
              style={{
                fontSize: '13px',
                fontWeight: 600,
                color: '#DC2626',
                margin: '0 0 4px 0',
              }}
            >
              Write rolled back — destination restored to Before state
            </p>
            {writeError && (
              <p
                style={{
                  fontSize: '12px',
                  color: 'var(--color-text-secondary)',
                  margin: 0,
                  fontFamily: 'var(--font-mono, "JetBrains Mono", monospace)',
                }}
              >
                Error: {writeError}
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  )
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
 * Access guard: if the current user is not an Admin, renders a 403 access
 * denied message instead of the view content. The SSE endpoint also enforces
 * admin-only access server-side and surfaces 403 via accessError.
 *
 * Route: /demo/sink-inspector
 *
 * Postconditions:
 *   - Selecting a task immediately subscribes to the SSE channel.
 *   - Changing the selected task resets all snapshot state.
 *   - Non-admin users see an access denied message, not the data panels.
 */
function SinkInspectorPage(): React.ReactElement {
  const { user } = useAuth()
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)

  // Admin-only access guard: non-admin users see an access denied message.
  if (user?.role !== 'admin') {
    return (
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          minHeight: '400px',
          gap: '12px',
        }}
      >
        <p
          style={{
            fontSize: '16px',
            fontWeight: 600,
            color: '#DC2626',
            margin: 0,
          }}
        >
          Access denied
        </p>
        <p
          style={{
            fontSize: '13px',
            color: 'var(--color-text-secondary)',
            margin: 0,
          }}
        >
          The Sink Inspector is only available to Admin users.
        </p>
      </div>
    )
  }

  return <SinkInspectorContent selectedTaskId={selectedTaskId} onTaskChange={setSelectedTaskId} />
}

interface SinkInspectorContentProps {
  selectedTaskId: string | null
  onTaskChange: (taskId: string | null) => void
}

/**
 * SinkInspectorContent renders the main Sink Inspector view for authenticated Admin users.
 * Extracted from SinkInspectorPage so hooks are always called unconditionally at the
 * same component level, satisfying the Rules of Hooks.
 *
 * @param selectedTaskId - Currently selected task ID, or null.
 * @param onTaskChange   - Called when the user picks a different task.
 */
function SinkInspectorContent({
  selectedTaskId,
  onTaskChange,
}: SinkInspectorContentProps): React.ReactElement {
  const {
    beforeSnapshot,
    afterSnapshot,
    rolledBack,
    isWaitingForSinkPhase,
    sseStatus,
    accessError,
    writeError,
  } = useSinkInspector({ taskId: selectedTaskId })

  const handleTaskChange = useCallback((taskId: string | null) => {
    onTaskChange(taskId)
  }, [onTaskChange])

  // If the SSE endpoint returned a 403 (non-admin), surface an access error.
  if (accessError) {
    return (
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          minHeight: '400px',
          gap: '12px',
        }}
      >
        <p
          style={{
            fontSize: '16px',
            fontWeight: 600,
            color: '#DC2626',
            margin: 0,
          }}
        >
          Access denied
        </p>
        <p
          style={{
            fontSize: '13px',
            color: 'var(--color-text-secondary)',
            margin: 0,
          }}
        >
          {accessError}
        </p>
      </div>
    )
  }

  const noTaskSelected = selectedTaskId === null

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: '16px',
      }}
    >
      {/* Spin keyframe — inlined so no CSS file is needed */}
      <style>{`@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }`}</style>

      {/* Page header */}
      <SinkInspectorHeader sseStatus={sseStatus} />

      {/* Task selector */}
      <TaskSelector selectedTaskId={selectedTaskId} onChange={handleTaskChange} />

      {/* Split panel (50/50) */}
      <div
        style={{
          display: 'flex',
          gap: '16px',
          alignItems: 'stretch',
        }}
      >
        {/* Before Snapshot — left panel */}
        <SnapshotPanel
          title="Before Snapshot"
          snapshot={beforeSnapshot}
          isWaiting={isWaitingForSinkPhase}
          emptyMessage={
            noTaskSelected
              ? 'Select a task to inspect its sink operation'
              : 'Awaiting sink:before-snapshot event...'
          }
        />

        {/* After Result — right panel */}
        <SnapshotPanel
          title="After Result"
          snapshot={afterSnapshot}
          highlightChanges={!rolledBack}
          beforeSnapshotForDiff={beforeSnapshot}
          emptyMessage={
            noTaskSelected
              ? 'Select a task to inspect its sink operation'
              : 'Awaiting sink:after-result event...'
          }
        />
      </div>

      {/* Atomicity verification section */}
      <AtomicityVerification
        beforeSnapshot={beforeSnapshot}
        afterSnapshot={afterSnapshot}
        rolledBack={rolledBack}
        writeError={writeError}
      />
    </div>
  )
}

export default SinkInspectorPage
