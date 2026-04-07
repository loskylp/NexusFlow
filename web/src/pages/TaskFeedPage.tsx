/**
 * TaskFeedPage — real-time task lifecycle feed with per-user visibility isolation (TASK-021).
 *
 * Layout (UX spec — Task Feed and Monitor):
 *   - Page header: "Task Feed" title, role indicator badge, SSE status
 *   - Filter bar: status dropdown, pipeline dropdown, search input, "Submit Task" button
 *   - Task list: vertical feed of TaskCard components, reverse chronological (newest first)
 *   - Pagination: "Showing X of Y tasks" with "Load More"
 *   - Empty states: no tasks / no filtered results
 *   - Loading: skeleton loader cards
 *
 * Real-time updates are handled by useTasks, which merges
 * GET /api/tasks (initial, filtered) with SSE /events/tasks (live).
 *
 * Role-based visibility:
 *   - Admin: sees all tasks with "Viewing: All Tasks" badge
 *   - User: sees own tasks with "Viewing: My Tasks" badge
 *   (Server-enforced — the SSE channel filters by userId for User role.)
 *
 * See: REQ-017, REQ-002, REQ-009, REQ-010, TASK-021, TASK-035,
 *      UX Spec (Task Feed and Monitor)
 */

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Task, TaskStatus, Pipeline } from '@/types/domain'
import type { SSEConnectionStatus } from '@/hooks/useSSE'
import { useTasks } from '@/hooks/useTasks'
import { usePipelines } from '@/hooks/usePipelines'
import { useAuth } from '@/context/AuthContext'
import { cancelTask } from '@/api/client'
import TaskCard from '@/components/TaskCard'
import SubmitTaskModal from '@/components/SubmitTaskModal'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Active filter values for the filter bar (controlled). */
interface ActiveFilters {
  status: TaskStatus | ''
  pipelineId: string
  search: string
}

const DEFAULT_FILTERS: ActiveFilters = { status: '', pipelineId: '', search: '' }

/** The statuses shown in the filter dropdown. */
const ALL_STATUSES: TaskStatus[] = ['submitted', 'queued', 'assigned', 'running', 'completed', 'failed', 'cancelled']

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface FilterBarProps {
  filters: ActiveFilters
  pipelines: Pipeline[]
  onFiltersChange: (filters: ActiveFilters) => void
  onSubmitTask: () => void
}

/**
 * FilterBar renders the status/pipeline/search filter controls and the
 * "Submit Task" button.
 *
 * @param filters         - Current active filter values (controlled).
 * @param pipelines       - Pipeline list for the pipeline selector dropdown.
 * @param onFiltersChange - Called when any filter value changes.
 * @param onSubmitTask    - Called when the "Submit Task" button is clicked.
 */
export function FilterBar({
  filters,
  pipelines,
  onFiltersChange,
  onSubmitTask,
}: FilterBarProps): React.ReactElement {
  const inputStyle: React.CSSProperties = {
    height: '36px',
    padding: '0 10px',
    fontSize: '13px',
    border: '1px solid var(--color-border)',
    borderRadius: '6px',
    backgroundColor: 'var(--color-surface-subtle)',
    color: 'var(--color-text-primary)',
    outline: 'none',
  }

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        flexWrap: 'wrap',
      }}
    >
      {/* Status filter */}
      <select
        aria-label="Filter by status"
        value={filters.status}
        onChange={e => onFiltersChange({ ...filters, status: e.target.value as TaskStatus | '' })}
        style={inputStyle}
      >
        <option value="">All Statuses</option>
        {ALL_STATUSES.map(s => (
          <option key={s} value={s}>{s.charAt(0).toUpperCase() + s.slice(1)}</option>
        ))}
      </select>

      {/* Pipeline filter */}
      <select
        aria-label="Filter by pipeline"
        value={filters.pipelineId}
        onChange={e => onFiltersChange({ ...filters, pipelineId: e.target.value })}
        style={inputStyle}
      >
        <option value="">All Pipelines</option>
        {pipelines.map(p => (
          <option key={p.id} value={p.id}>{p.name}</option>
        ))}
      </select>

      {/* Search input */}
      <input
        type="search"
        aria-label="Search tasks"
        placeholder="Search by task ID or pipeline name"
        value={filters.search}
        onChange={e => onFiltersChange({ ...filters, search: e.target.value })}
        style={{ ...inputStyle, width: '220px' }}
      />

      {/* Spacer */}
      <div style={{ flex: 1 }} />

      {/* Submit Task button */}
      <button
        type="button"
        onClick={onSubmitTask}
        style={{
          height: '36px',
          padding: '0 16px',
          backgroundColor: '#4F46E5',
          color: '#FFFFFF',
          border: 'none',
          borderRadius: '6px',
          fontSize: '13px',
          fontWeight: 500,
          cursor: 'pointer',
        }}
      >
        Submit Task
      </button>
    </div>
  )
}

interface FeedStatusBarProps {
  sseStatus: SSEConnectionStatus
  isAdmin: boolean
  taskCount: number
}

/**
 * FeedStatusBar renders the role indicator badge and SSE connection status.
 *
 * @param sseStatus  - Current SSE connection status for the dot indicator.
 * @param isAdmin    - True if the current user is Admin (shows "Viewing: All Tasks").
 * @param taskCount  - Number of tasks currently visible (for the count display).
 */
export function FeedStatusBar({
  sseStatus,
  isAdmin,
  taskCount,
}: FeedStatusBarProps): React.ReactElement {
  const isReconnecting = sseStatus === 'reconnecting'
  const isConnected = sseStatus === 'connected'

  const dotColor = isConnected
    ? 'var(--color-success)'
    : isReconnecting
      ? 'var(--color-error)'
      : 'var(--color-warning)'

  const statusLabel = isReconnecting
    ? 'Reconnecting...'
    : isConnected
      ? 'Connected'
      : 'Connecting...'

  return (
    <div
      role="status"
      aria-live="polite"
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '16px',
        padding: '8px 12px',
        backgroundColor: 'var(--color-surface-panel)',
        border: '1px solid var(--color-border)',
        borderRadius: '6px',
        marginTop: '16px',
        fontSize: '12px',
        color: 'var(--color-text-secondary)',
      }}
    >
      {/* SSE dot + label */}
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
        <span
          style={{
            display: 'inline-block',
            width: '6px',
            height: '6px',
            borderRadius: '50%',
            backgroundColor: dotColor,
          }}
        />
        <span style={{ color: isReconnecting ? 'var(--color-error)' : 'inherit' }}>
          {statusLabel}
        </span>
      </span>

      {/* Task count */}
      <span>{taskCount} task{taskCount !== 1 ? 's' : ''}</span>

      {/* Role indicator badge */}
      <span
        style={{
          padding: '2px 8px',
          borderRadius: '9999px',
          backgroundColor: isAdmin ? '#EEF2FF' : '#F0FDF4',
          color: isAdmin ? '#4F46E5' : '#16A34A',
          fontSize: '11px',
          fontWeight: 500,
          fontFamily: 'var(--font-label)',
          textTransform: 'uppercase' as const,
          letterSpacing: '0.04em',
        }}
      >
        Viewing: {isAdmin ? 'All Tasks' : 'My Tasks'}
      </span>
    </div>
  )
}

interface SkeletonTaskCardProps {
  /** Unique key for React list rendering. */
  index: number
}

/**
 * SkeletonTaskCard renders a placeholder task card during initial loading.
 */
export function SkeletonTaskCard({ index: _index }: SkeletonTaskCardProps): React.ReactElement {
  return (
    <div
      aria-busy="true"
      style={{
        backgroundColor: 'var(--color-surface-panel)',
        border: '1px solid var(--color-border)',
        borderRadius: '8px',
        padding: '16px',
      }}
    >
      {/* Title line */}
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '10px' }}>
        <div
          style={{
            height: '14px',
            width: '160px',
            backgroundColor: 'var(--color-surface-subtle)',
            borderRadius: '4px',
            animation: 'pulse 1.5s ease-in-out infinite',
          }}
        />
        <div
          style={{
            height: '20px',
            width: '64px',
            backgroundColor: 'var(--color-surface-subtle)',
            borderRadius: '9999px',
            animation: 'pulse 1.5s ease-in-out infinite',
          }}
        />
      </div>
      {/* Subtitle line */}
      <div
        style={{
          height: '12px',
          width: '120px',
          backgroundColor: 'var(--color-surface-subtle)',
          borderRadius: '4px',
          marginBottom: '12px',
          animation: 'pulse 1.5s ease-in-out infinite',
        }}
      />
      {/* Meta line */}
      <div
        style={{
          height: '12px',
          width: '200px',
          backgroundColor: 'var(--color-surface-subtle)',
          borderRadius: '4px',
          marginBottom: '12px',
          animation: 'pulse 1.5s ease-in-out infinite',
        }}
      />
      {/* Buttons row */}
      <div style={{ display: 'flex', gap: '8px' }}>
        <div
          style={{
            height: '28px',
            width: '72px',
            backgroundColor: 'var(--color-surface-subtle)',
            borderRadius: '6px',
            animation: 'pulse 1.5s ease-in-out infinite',
          }}
        />
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

/** Number of skeleton cards shown during initial loading. */
const SKELETON_COUNT = 4

/**
 * TaskFeedPage is the primary user interaction surface for task management.
 * It composes useTasks (live data), usePipelines (filter selector), and
 * SubmitTaskModal (task creation).
 *
 * States rendered:
 *   - Loading: skeleton task cards (isLoading === true)
 *   - Empty (no tasks ever): empty state with "Submit Task" CTA
 *   - Empty (filtered): "No tasks match your filters." with "Clear Filters"
 *   - Populated: task cards in reverse chronological order
 *   - SSE reconnecting: status bar shows "Reconnecting..."
 *
 * Preconditions:
 *   - User must be authenticated.
 *   - App routes this component behind ProtectedRoute.
 */
function TaskFeedPage(): React.ReactElement {
  const navigate = useNavigate()
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'

  const [filters, setFilters] = useState<ActiveFilters>(DEFAULT_FILTERS)
  const [isModalOpen, setIsModalOpen] = useState(false)

  // Build TaskFilters from active filter state for the hook.
  const taskFilters = useMemo(() => ({
    ...(filters.status ? { status: filters.status } : {}),
    ...(filters.pipelineId ? { pipelineId: filters.pipelineId } : {}),
    ...(filters.search ? { search: filters.search } : {}),
  }), [filters])

  const { tasks, isLoading, error, sseStatus, refresh } = useTasks(
    Object.keys(taskFilters).length > 0 ? taskFilters : undefined
  )
  const { pipelines } = usePipelines()

  // Track recently-updated task IDs for the 200ms highlight flash.
  // A Set of task IDs that have been updated via SSE since last render.
  const [recentlyUpdated, setRecentlyUpdated] = useState<Set<string>>(new Set())
  const recentlyUpdatedTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())

  // Build pipeline name lookup for TaskCard.
  const pipelineNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const p of pipelines) {
      map.set(p.id, p.name)
    }
    return map
  }, [pipelines])

  // Determine whether any filter is active (for empty state disambiguation).
  const hasActiveFilters = filters.status !== '' || filters.pipelineId !== '' || filters.search !== ''

  /** handleViewLogs navigates to Log Streamer with the task pre-selected. */
  const handleViewLogs = useCallback((taskId: string) => {
    navigate(`/tasks/logs?taskId=${taskId}`)
  }, [navigate])

  /** handleCancel sends the cancel request and refreshes the task list on success. */
  const handleCancel = useCallback(async (taskId: string) => {
    try {
      await cancelTask(taskId)
      refresh()
    } catch {
      // Error surfaced by the server response; a future toast notification would go here.
    }
  }, [refresh])

  /** handleRetry re-submits the task via the submit modal with the same pipeline. */
  const handleRetry = useCallback((task: Task) => {
    // For now, open the submit modal. Full retry-with-same-config in TASK-035.
    void task
    setIsModalOpen(true)
  }, [])

  /**
   * markRecentlyUpdated adds a task ID to the recently-updated set and schedules
   * its removal after 200ms so the highlight flash is transient.
   */
  function markRecentlyUpdated(taskId: string): void {
    setRecentlyUpdated(prev => new Set(prev).add(taskId))
    const existing = recentlyUpdatedTimers.current.get(taskId)
    if (existing) clearTimeout(existing)
    const timer = setTimeout(() => {
      setRecentlyUpdated(prev => {
        const next = new Set(prev)
        next.delete(taskId)
        return next
      })
      recentlyUpdatedTimers.current.delete(taskId)
    }, 200)
    recentlyUpdatedTimers.current.set(taskId, timer)
  }

  // Clear all timers on unmount to prevent state updates on unmounted component.
  useEffect(() => {
    return () => {
      for (const timer of recentlyUpdatedTimers.current.values()) {
        clearTimeout(timer)
      }
    }
  }, [])

  // When a task's updatedAt changes (detected by comparing references from useTasks),
  // mark it as recently updated. We track previous task states by ID.
  const prevTasksRef = useRef<Map<string, string>>(new Map())
  useEffect(() => {
    for (const task of tasks) {
      const prevUpdatedAt = prevTasksRef.current.get(task.id)
      if (prevUpdatedAt !== undefined && prevUpdatedAt !== task.updatedAt) {
        markRecentlyUpdated(task.id)
      }
      prevTasksRef.current.set(task.id, task.updatedAt)
    }
  })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
      {/* Page header */}
      <h1
        style={{
          fontSize: '20px',
          fontWeight: 600,
          color: 'var(--color-text-primary)',
          margin: 0,
        }}
      >
        Task Feed
      </h1>

      {/* Filter bar */}
      <FilterBar
        filters={filters}
        pipelines={pipelines}
        onFiltersChange={setFilters}
        onSubmitTask={() => setIsModalOpen(true)}
      />

      {/* Content area */}
      {isLoading ? (
        // Loading state: skeleton cards
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          {Array.from({ length: SKELETON_COUNT }, (_, i) => (
            <SkeletonTaskCard key={i} index={i} />
          ))}
        </div>
      ) : error ? (
        // Error state
        <div
          role="alert"
          style={{
            padding: '16px',
            backgroundColor: '#FEF2F2',
            border: '1px solid #FECACA',
            borderRadius: '8px',
            color: '#DC2626',
            fontSize: '14px',
          }}
        >
          {error}
        </div>
      ) : tasks.length === 0 && !hasActiveFilters ? (
        // Empty state: no tasks submitted yet
        <div
          style={{
            backgroundColor: 'var(--color-surface-panel)',
            border: '1px solid var(--color-border)',
            borderRadius: '8px',
            padding: '48px 24px',
            textAlign: 'center',
          }}
        >
          <p style={{ color: 'var(--color-text-secondary)', fontSize: '14px', marginBottom: '16px' }}>
            No tasks found. Submit your first task to get started.
          </p>
          <button
            type="button"
            onClick={() => setIsModalOpen(true)}
            style={{
              padding: '8px 16px',
              backgroundColor: '#4F46E5',
              color: '#FFFFFF',
              border: 'none',
              borderRadius: '6px',
              fontSize: '14px',
              fontWeight: 500,
              cursor: 'pointer',
            }}
          >
            Submit Task
          </button>
        </div>
      ) : tasks.length === 0 && hasActiveFilters ? (
        // Empty state: filters returned no results
        <div
          style={{
            backgroundColor: 'var(--color-surface-panel)',
            border: '1px solid var(--color-border)',
            borderRadius: '8px',
            padding: '48px 24px',
            textAlign: 'center',
          }}
        >
          <p style={{ color: 'var(--color-text-secondary)', fontSize: '14px', marginBottom: '12px' }}>
            No tasks match your filters.
          </p>
          <button
            type="button"
            onClick={() => setFilters(DEFAULT_FILTERS)}
            style={{
              background: 'none',
              border: 'none',
              color: '#4F46E5',
              fontSize: '14px',
              cursor: 'pointer',
              textDecoration: 'underline',
            }}
          >
            Clear Filters
          </button>
        </div>
      ) : (
        // Task list: reverse chronological (newest first, sorted by createdAt desc)
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          {sortTasksNewestFirst(tasks).map(task => (
            <TaskCard
              key={task.id}
              task={task}
              pipelineName={pipelineNameById.get(task.pipelineId) ?? task.pipelineId}
              isAdmin={isAdmin}
              isOwner={user?.id === task.userId}
              onViewLogs={handleViewLogs}
              onCancel={handleCancel}
              onRetry={handleRetry}
              isRecentlyUpdated={recentlyUpdated.has(task.id)}
            />
          ))}
        </div>
      )}

      {/* SSE status bar */}
      <FeedStatusBar
        sseStatus={sseStatus}
        isAdmin={isAdmin}
        taskCount={tasks.length}
      />

      {/* Submit Task modal */}
      <SubmitTaskModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSuccess={() => refresh()}
        pipelines={pipelines}
      />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Sort helper
// ---------------------------------------------------------------------------

/**
 * sortTasksNewestFirst returns a new array sorted by createdAt descending
 * (newest task first) to match the UX spec reverse chronological order.
 *
 * @param tasks - Task list to sort.
 * @returns New array with newest tasks at the front.
 */
function sortTasksNewestFirst(tasks: Task[]): Task[] {
  return [...tasks].sort((a, b) => b.createdAt.localeCompare(a.createdAt))
}

export default TaskFeedPage
