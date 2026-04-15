/**
 * ChaosControllerPage — Admin-only demo view for injecting disturbances into the
 * running system to demonstrate auto-recovery capabilities.
 *
 * Layout:
 *   - Header: "Chaos Controller" title with "DEMO" and "DESTRUCTIVE" badges,
 *             system status indicator
 *   - Three action cards stacked vertically:
 *     1. Kill Worker: worker selector, kill button, expected result, activity log
 *     2. Disconnect Database: duration selector, disconnect button, expected result,
 *        activity log
 *     3. Flood Queue: task count input, pipeline selector, submit burst button,
 *        expected result, activity log
 *
 * All destructive actions (Kill Worker, Disconnect DB) require a confirmation dialog.
 * Flood Queue does not require confirmation (non-destructive).
 *
 * Access: Admin only. Non-admin users are shown an access denied message.
 *
 * See: DEMO-004, UX Spec (Chaos Controller), TASK-034
 */

import React, { useState } from 'react'
import { useAuth } from '@/context/AuthContext'
import { useChaosController } from '@/hooks/useChaosController'
import type { Worker, Pipeline, ChaosActivityEntry, SystemHealthStatus } from '@/types/domain'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ConfirmDialogProps {
  /** Dialog prompt shown to the user before a destructive action. */
  message: string
  /** Called when the user confirms the action. */
  onConfirm: () => void
  /** Called when the user cancels the action. */
  onCancel: () => void
}

interface ActivityLogProps {
  /** Log entries to display in reverse-chronological order (newest first). */
  entries: ChaosActivityEntry[]
}

interface ChaosHeaderProps {
  systemStatus: SystemHealthStatus
}

interface KillWorkerCardProps {
  workers: Worker[]
  selectedWorkerId: string | null
  onSelectWorker: (id: string | null) => void
  isKilling: boolean
  onKill: () => void
  log: ChaosActivityEntry[]
}

interface DisconnectDatabaseCardProps {
  disconnectDurationSeconds: 15 | 30 | 60
  onSelectDuration: (seconds: 15 | 30 | 60) => void
  isDisconnecting: boolean
  isSubmitting: boolean
  remainingSeconds: number | null
  onDisconnect: () => void
  log: ChaosActivityEntry[]
}

interface FloodQueueCardProps {
  pipelines: Pipeline[]
  selectedPipelineId: string | null
  onSelectPipeline: (id: string | null) => void
  taskCount: number
  onSetTaskCount: (count: number) => void
  isFlooding: boolean
  floodProgress: number | null
  onFlood: () => void
  log: ChaosActivityEntry[]
}

// ---------------------------------------------------------------------------
// Shared sub-components
// ---------------------------------------------------------------------------

/**
 * ConfirmDialog renders a modal confirmation dialog for destructive actions.
 * Rendered inline (not via a portal) for JSDOM test compatibility.
 *
 * @param message  - The action description to confirm.
 * @param onConfirm - Callback when the user clicks Confirm.
 * @param onCancel  - Callback when the user clicks Cancel.
 */
function ConfirmDialog({ message, onConfirm, onCancel }: ConfirmDialogProps): React.ReactElement {
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Confirm action"
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 100,
      }}
    >
      <div
        style={{
          backgroundColor: 'var(--color-surface-elevated)',
          border: '1px solid var(--color-border)',
          borderRadius: '8px',
          padding: '24px',
          maxWidth: '400px',
          width: '90%',
        }}
      >
        <h3 style={{ color: 'var(--color-text-primary)', marginTop: 0, marginBottom: '12px', fontSize: '16px' }}>
          Confirm Action
        </h3>
        <p style={{ color: 'var(--color-text-secondary)', marginBottom: '20px', fontSize: '14px' }}>
          {message}
        </p>
        <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
          <button
            onClick={onCancel}
            style={{
              padding: '8px 16px',
              background: 'transparent',
              border: '1px solid var(--color-border)',
              borderRadius: '6px',
              color: 'var(--color-text-secondary)',
              fontSize: '14px',
              cursor: 'pointer',
            }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            style={{
              padding: '8px 16px',
              backgroundColor: '#DC2626',
              border: 'none',
              borderRadius: '6px',
              color: '#FFFFFF',
              fontSize: '14px',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Confirm
          </button>
        </div>
      </div>
    </div>
  )
}

/**
 * ActivityLog renders a scrollable list of ChaosActivityEntry items in
 * reverse-chronological order (newest first).
 *
 * @param entries - The log entries to render.
 */
function ActivityLog({ entries }: ActivityLogProps): React.ReactElement {
  if (entries.length === 0) {
    return (
      <p style={{ color: 'var(--color-text-tertiary)', fontSize: '13px', fontStyle: 'italic', margin: 0 }}>
        No activity yet.
      </p>
    )
  }

  const colorForLevel = (level: string): string => {
    if (level === 'error') return '#EF4444'
    if (level === 'warn') return '#F59E0B'
    return '#94A3B8'
  }

  return (
    <div
      style={{
        maxHeight: '160px',
        overflowY: 'auto',
        fontFamily: 'var(--font-mono)',
        fontSize: '12px',
        backgroundColor: 'var(--color-surface-base)',
        borderRadius: '4px',
        padding: '8px',
        border: '1px solid var(--color-border)',
      }}
    >
      {[...entries].reverse().map((entry, idx) => (
        <div key={idx} style={{ marginBottom: '4px', color: colorForLevel(entry.level) }}>
          <span style={{ opacity: 0.6 }}>{entry.timestamp}</span>{' '}
          <span>[{entry.level.toUpperCase()}]</span>{' '}
          <span>{entry.message}</span>
        </div>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------------------
// ChaosHeader
// ---------------------------------------------------------------------------

/**
 * ChaosHeader renders the page title, "DEMO" and "DESTRUCTIVE" badges, and
 * the system status indicator dot.
 *
 * @param systemStatus - The current health status from GET /api/health.
 */
function ChaosHeader({ systemStatus }: ChaosHeaderProps): React.ReactElement {
  const dotColor = systemStatus === 'nominal' ? '#16A34A'
    : systemStatus === 'degraded' ? '#D97706'
    : '#DC2626'

  const dotLabel = systemStatus === 'nominal' ? 'Nominal'
    : systemStatus === 'degraded' ? 'Degraded'
    : 'Critical'

  return (
    <div style={{ marginBottom: '24px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
        <h1 style={{ color: 'var(--color-text-primary)', fontSize: '24px', fontWeight: 700, margin: 0 }}>
          Chaos Controller
        </h1>
        <span
          style={{
            backgroundColor: 'rgba(99,102,241,0.15)',
            color: '#818CF8',
            borderRadius: '4px',
            padding: '2px 8px',
            fontSize: '11px',
            fontWeight: 600,
            textTransform: 'uppercase',
            letterSpacing: '0.06em',
          }}
        >
          DEMO
        </span>
        <span
          style={{
            backgroundColor: 'rgba(220,38,38,0.15)',
            color: '#F87171',
            borderRadius: '4px',
            padding: '2px 8px',
            fontSize: '11px',
            fontWeight: 600,
            textTransform: 'uppercase',
            letterSpacing: '0.06em',
          }}
        >
          DESTRUCTIVE
        </span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginTop: '8px' }}>
        <span
          role="status"
          aria-label={`System status: ${dotLabel}`}
          style={{
            display: 'inline-block',
            width: '10px',
            height: '10px',
            borderRadius: '50%',
            backgroundColor: dotColor,
          }}
        />
        <span style={{ color: 'var(--color-text-secondary)', fontSize: '13px' }}>
          System status: {dotLabel}
        </span>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// KillWorkerCard
// ---------------------------------------------------------------------------

/**
 * KillWorkerCard allows the admin to select a worker and kill its container.
 * A confirmation dialog is shown before the kill action is dispatched.
 *
 * @param workers          - List of registered workers for the selector.
 * @param selectedWorkerId - Currently selected worker ID (null = none selected).
 * @param onSelectWorker   - Callback when the dropdown selection changes.
 * @param isKilling        - True while the kill API call is in flight.
 * @param onKill           - Callback to dispatch the kill action (post-confirm).
 * @param log              - Activity log entries for this card.
 */
function KillWorkerCard({
  workers,
  selectedWorkerId,
  onSelectWorker,
  isKilling,
  onKill,
  log,
}: KillWorkerCardProps): React.ReactElement {
  const [showConfirm, setShowConfirm] = useState(false)

  const handleKillClick = () => {
    if (!selectedWorkerId) return
    setShowConfirm(true)
  }

  const handleConfirm = () => {
    setShowConfirm(false)
    onKill()
  }

  const handleCancel = () => {
    setShowConfirm(false)
  }

  return (
    <>
      {showConfirm && (
        <ConfirmDialog
          message={`This will send SIGKILL to worker container "${selectedWorkerId}". The Monitor will reclaim any in-flight tasks. Continue?`}
          onConfirm={handleConfirm}
          onCancel={handleCancel}
        />
      )}
      <div style={cardStyle}>
        <h2 style={cardTitleStyle}>Kill Worker</h2>
        <p style={cardDescriptionStyle}>
          Terminate a worker container to demonstrate automatic task recovery (ADR-002).
        </p>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center', marginBottom: '12px' }}>
          <select
            aria-label="Select worker"
            value={selectedWorkerId ?? ''}
            onChange={e => onSelectWorker(e.target.value || null)}
            disabled={isKilling}
            style={selectStyle}
          >
            <option value="">Select a worker...</option>
            {workers.map(w => (
              <option key={w.id} value={w.id}>
                {w.id} ({w.status})
              </option>
            ))}
          </select>
          <button
            onClick={handleKillClick}
            disabled={!selectedWorkerId || isKilling}
            aria-label="Kill selected worker"
            style={destructiveButtonStyle(!selectedWorkerId || isKilling)}
          >
            {isKilling ? 'Killing...' : 'Kill Worker'}
          </button>
        </div>
        <p style={expectedResultStyle}>
          Expected: Worker goes down; Monitor detects missed heartbeat and reclaims tasks within ~30s.
        </p>
        <ActivityLog entries={log} />
      </div>
    </>
  )
}

// ---------------------------------------------------------------------------
// DisconnectDatabaseCard
// ---------------------------------------------------------------------------

/**
 * DisconnectDatabaseCard allows the admin to simulate a database outage for
 * 15, 30, or 60 seconds. A confirmation dialog is shown before dispatch.
 * A countdown timer is displayed while the disconnect is active.
 *
 * @param disconnectDurationSeconds - Selected duration in seconds (15 | 30 | 60).
 * @param onSelectDuration          - Callback when the duration selector changes.
 * @param isDisconnecting           - True while the countdown is active.
 * @param isSubmitting              - True while the API call is in flight.
 * @param remainingSeconds          - Countdown value, or null when inactive.
 * @param onDisconnect              - Callback to dispatch the disconnect (post-confirm).
 * @param log                       - Activity log entries for this card.
 */
function DisconnectDatabaseCard({
  disconnectDurationSeconds,
  onSelectDuration,
  isDisconnecting,
  isSubmitting,
  remainingSeconds,
  onDisconnect,
  log,
}: DisconnectDatabaseCardProps): React.ReactElement {
  const [showConfirm, setShowConfirm] = useState(false)

  const handleDisconnectClick = () => {
    if (isDisconnecting || isSubmitting) return
    setShowConfirm(true)
  }

  const handleConfirm = () => {
    setShowConfirm(false)
    onDisconnect()
  }

  const handleCancel = () => {
    setShowConfirm(false)
  }

  const isBlocked = isDisconnecting || isSubmitting

  return (
    <>
      {showConfirm && (
        <ConfirmDialog
          message={`This will stop the PostgreSQL container for ${disconnectDurationSeconds} seconds. Services will fail DB operations during this window. Continue?`}
          onConfirm={handleConfirm}
          onCancel={handleCancel}
        />
      )}
      <div style={cardStyle}>
        <h2 style={cardTitleStyle}>Disconnect Database</h2>
        <p style={cardDescriptionStyle}>
          Stop the PostgreSQL container temporarily to trigger DB error handling and auto-recovery.
        </p>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center', marginBottom: '12px' }}>
          <select
            aria-label="Select disconnect duration"
            value={disconnectDurationSeconds}
            onChange={e => onSelectDuration(Number(e.target.value) as 15 | 30 | 60)}
            disabled={isBlocked}
            style={selectStyle}
          >
            <option value={15}>15 seconds</option>
            <option value={30}>30 seconds</option>
            <option value={60}>60 seconds</option>
          </select>
          <button
            onClick={handleDisconnectClick}
            disabled={isBlocked}
            aria-label="Disconnect database"
            style={destructiveButtonStyle(isBlocked)}
          >
            {isSubmitting ? 'Disconnecting...' : 'Disconnect DB'}
          </button>
        </div>
        {isDisconnecting && remainingSeconds !== null && (
          <div
            aria-live="polite"
            aria-label={`Disconnect active: ${remainingSeconds} seconds remaining`}
            style={{
              padding: '8px 12px',
              backgroundColor: 'rgba(220,38,38,0.1)',
              border: '1px solid rgba(220,38,38,0.3)',
              borderRadius: '6px',
              color: '#F87171',
              fontSize: '14px',
              fontWeight: 600,
              marginBottom: '12px',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
            }}
          >
            <span>DB DISCONNECTED</span>
            <span data-testid="disconnect-countdown">{remainingSeconds}s remaining</span>
          </div>
        )}
        <p style={expectedResultStyle}>
          Expected: API and workers log DB errors; auto-reconnect when container restarts.
        </p>
        <ActivityLog entries={log} />
      </div>
    </>
  )
}

// ---------------------------------------------------------------------------
// FloodQueueCard
// ---------------------------------------------------------------------------

/**
 * FloodQueueCard allows the admin to submit a burst of tasks to a pipeline.
 * No confirmation dialog — flooding is non-destructive (tasks can be cancelled).
 *
 * @param pipelines          - List of pipelines for the selector.
 * @param selectedPipelineId - Currently selected pipeline ID (null = none).
 * @param onSelectPipeline   - Callback when the pipeline selector changes.
 * @param taskCount          - Current task count input value.
 * @param onSetTaskCount     - Callback when the task count input changes.
 * @param isFlooding         - True while the flood API call is in flight.
 * @param floodProgress      - Completion percentage (0-100) or null when idle.
 * @param onFlood            - Callback to dispatch the flood action.
 * @param log                - Activity log entries for this card.
 */
function FloodQueueCard({
  pipelines,
  selectedPipelineId,
  onSelectPipeline,
  taskCount,
  onSetTaskCount,
  isFlooding,
  floodProgress,
  onFlood,
  log,
}: FloodQueueCardProps): React.ReactElement {
  const canSubmit = !!selectedPipelineId && !isFlooding && taskCount >= 1 && taskCount <= 1000

  return (
    <div style={cardStyle}>
      <h2 style={cardTitleStyle}>Flood Queue</h2>
      <p style={cardDescriptionStyle}>
        Submit a burst of tasks to demonstrate auto-scaling and queue backlog handling.
      </p>
      <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap', marginBottom: '12px' }}>
        <input
          type="number"
          aria-label="Task count"
          value={taskCount}
          min={1}
          max={1000}
          disabled={isFlooding}
          onChange={e => onSetTaskCount(Number(e.target.value))}
          style={{
            width: '100px',
            padding: '6px 10px',
            backgroundColor: 'var(--color-surface-base)',
            border: '1px solid var(--color-border)',
            borderRadius: '6px',
            color: 'var(--color-text-primary)',
            fontSize: '14px',
          }}
        />
        <select
          aria-label="Select pipeline for flood"
          value={selectedPipelineId ?? ''}
          onChange={e => onSelectPipeline(e.target.value || null)}
          disabled={isFlooding}
          style={selectStyle}
        >
          <option value="">Select a pipeline...</option>
          {pipelines.map(p => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        <button
          onClick={onFlood}
          disabled={!canSubmit}
          aria-label="Submit burst"
          style={primaryButtonStyle(!canSubmit)}
        >
          {isFlooding ? 'Flooding...' : 'Submit Burst'}
        </button>
      </div>
      {floodProgress !== null && (
        <div
          aria-label="Flood complete"
          style={{
            padding: '6px 12px',
            backgroundColor: 'rgba(22,163,74,0.1)',
            border: '1px solid rgba(22,163,74,0.3)',
            borderRadius: '6px',
            color: '#4ADE80',
            fontSize: '13px',
            marginBottom: '12px',
          }}
        >
          Flood complete ({floodProgress}%)
        </div>
      )}
      <p style={expectedResultStyle}>
        Expected: Task Feed shows burst of submitted tasks; workers process them in parallel.
      </p>
      <ActivityLog entries={log} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Shared styles
// ---------------------------------------------------------------------------

const cardStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-elevated)',
  border: '1px solid var(--color-border)',
  borderRadius: '8px',
  padding: '20px',
  marginBottom: '16px',
}

const cardTitleStyle: React.CSSProperties = {
  color: 'var(--color-text-primary)',
  fontSize: '16px',
  fontWeight: 600,
  marginTop: 0,
  marginBottom: '8px',
}

const cardDescriptionStyle: React.CSSProperties = {
  color: 'var(--color-text-secondary)',
  fontSize: '13px',
  marginTop: 0,
  marginBottom: '12px',
}

const expectedResultStyle: React.CSSProperties = {
  color: 'var(--color-text-tertiary)',
  fontSize: '12px',
  fontStyle: 'italic',
  marginTop: 0,
  marginBottom: '12px',
}

const selectStyle: React.CSSProperties = {
  padding: '6px 10px',
  backgroundColor: 'var(--color-surface-base)',
  border: '1px solid var(--color-border)',
  borderRadius: '6px',
  color: 'var(--color-text-primary)',
  fontSize: '14px',
  flex: 1,
  minWidth: '160px',
}

function destructiveButtonStyle(disabled: boolean): React.CSSProperties {
  return {
    padding: '7px 14px',
    backgroundColor: disabled ? '#374151' : '#DC2626',
    border: 'none',
    borderRadius: '6px',
    color: disabled ? '#6B7280' : '#FFFFFF',
    fontSize: '14px',
    fontWeight: 600,
    cursor: disabled ? 'not-allowed' : 'pointer',
    whiteSpace: 'nowrap',
  }
}

function primaryButtonStyle(disabled: boolean): React.CSSProperties {
  return {
    padding: '7px 14px',
    backgroundColor: disabled ? '#374151' : '#4F46E5',
    border: 'none',
    borderRadius: '6px',
    color: disabled ? '#6B7280' : '#FFFFFF',
    fontSize: '14px',
    fontWeight: 600,
    cursor: disabled ? 'not-allowed' : 'pointer',
    whiteSpace: 'nowrap',
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

/**
 * ChaosControllerPage is the root component for the Chaos Controller demo view.
 *
 * Orchestrates:
 *   - Worker list fetch (for Kill Worker selector)
 *   - Pipeline list fetch (for Flood Queue selector)
 *   - System health polling via GET /api/health (refreshed after chaos actions)
 *   - API calls for each chaos action:
 *     - POST /api/chaos/kill-worker     — Kill Worker action
 *     - POST /api/chaos/disconnect-db   — Disconnect Database action
 *     - POST /api/chaos/flood-queue     — Flood Queue action
 *   - Activity log state management per card
 *
 * Route: /demo/chaos
 *
 * Postconditions:
 *   - Each destructive action (Kill Worker, Disconnect DB) appends timeline entries
 *     to the relevant card's activity log.
 *   - All confirmation dialogs are handled by the respective card components.
 *   - Non-admin users see an access denied message (admin guard applies at route level).
 */
function ChaosControllerPage(): React.ReactElement {
  const { user } = useAuth()

  // Admin-only enforcement in the UI: show access denied for non-admin users.
  // The route is also guarded at the API layer via RequireRole in server.go (avoids OBS-032-1).
  if (!user || user.role !== 'admin') {
    return (
      <div role="alert" style={{ color: 'var(--color-text-primary)', padding: '24px' }}>
        <h2>Access Denied</h2>
        <p>The Chaos Controller is only available to admin users.</p>
      </div>
    )
  }

  return <ChaosControllerContent />
}

/**
 * ChaosControllerContent renders the main page content for authenticated admins.
 * Extracted into a separate component so that useChaosController is only called
 * after the admin check passes (hooks must not be called conditionally).
 */
function ChaosControllerContent(): React.ReactElement {
  const chaos = useChaosController()

  return (
    <div style={{ maxWidth: '760px' }}>
      <ChaosHeader systemStatus={chaos.systemStatus} />

      {chaos.isLoadingSelectors ? (
        <p style={{ color: 'var(--color-text-secondary)' }}>Loading...</p>
      ) : (
        <>
          <KillWorkerCard
            workers={chaos.workers}
            selectedWorkerId={chaos.selectedWorkerId}
            onSelectWorker={chaos.setSelectedWorkerId}
            isKilling={chaos.isKilling}
            onKill={chaos.killWorker}
            log={chaos.killLog}
          />

          <DisconnectDatabaseCard
            disconnectDurationSeconds={chaos.disconnectDurationSeconds}
            onSelectDuration={chaos.setDisconnectDurationSeconds}
            isDisconnecting={chaos.isDisconnecting}
            isSubmitting={chaos.isSubmittingDisconnect}
            remainingSeconds={chaos.disconnectRemainingSeconds}
            onDisconnect={chaos.disconnectDatabase}
            log={chaos.disconnectLog}
          />

          <FloodQueueCard
            pipelines={chaos.pipelines}
            selectedPipelineId={chaos.selectedFloodPipelineId}
            onSelectPipeline={chaos.setSelectedFloodPipelineId}
            taskCount={chaos.floodTaskCount}
            onSetTaskCount={chaos.setFloodTaskCount}
            isFlooding={chaos.isFlooding}
            floodProgress={chaos.floodProgress}
            onFlood={chaos.floodQueue}
            log={chaos.floodLog}
          />
        </>
      )}
    </div>
  )
}

export default ChaosControllerPage
