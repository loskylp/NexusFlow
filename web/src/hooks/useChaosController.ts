/**
 * useChaosController — hook that manages state and API calls for the
 * Chaos Controller demo view.
 *
 * Encapsulates:
 *   - Worker list for the Kill Worker selector (fetched on mount).
 *   - Pipeline list for the Flood Queue selector.
 *   - System health status derived from GET /api/health.
 *   - Kill worker action: POST /api/chaos/kill-worker.
 *   - Disconnect database action: POST /api/chaos/disconnect-db.
 *   - Flood queue action: POST /api/chaos/flood-queue.
 *   - Per-card activity log state.
 *   - Countdown timer for active database disconnect.
 *
 * State isolation: each card's activity log is maintained separately so the
 * ChaosControllerPage can pass the right log to each card component.
 *
 * See: DEMO-004, ADR-002, TASK-034
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import * as client from '@/api/client'
import type { Worker, Pipeline, ChaosActivityEntry, SystemHealthStatus } from '@/types/domain'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/**
 * UseChaosControllerReturn is the hook's complete public surface.
 */
export interface UseChaosControllerReturn {
  // ---- Data for selectors ----

  /** Online workers, fetched on mount. Used by Kill Worker selector. */
  workers: Worker[]

  /** All pipelines visible to admin, fetched on mount. Used by Flood Queue selector. */
  pipelines: Pipeline[]

  /** True while the initial workers/pipelines fetch is in progress. */
  isLoadingSelectors: boolean

  // ---- System health ----

  /**
   * Current system health status, derived from GET /api/health.
   * Refreshed after each chaos action completes.
   * 'nominal' when all dependencies are healthy.
   * 'degraded' when at least one dependency reports an error.
   * 'critical' when the health endpoint itself is unreachable.
   */
  systemStatus: SystemHealthStatus

  // ---- Kill Worker state ----

  /** Currently selected worker ID for the Kill Worker action. null if none. */
  selectedWorkerId: string | null

  /** Updates the selected worker ID. */
  setSelectedWorkerId: (workerId: string | null) => void

  /** True while the kill-worker API call is in progress. */
  isKilling: boolean

  /**
   * Calls POST /api/chaos/kill-worker for the selectedWorkerId.
   * Appends entries to killLog on success or failure.
   * Refreshes systemStatus after completion.
   *
   * Preconditions:
   *   - selectedWorkerId is non-null.
   *   - isKilling is false.
   *
   * Postconditions:
   *   - On success: at least one entry appended to killLog; worker container stopped.
   *   - On error: error entry appended to killLog; no container stopped.
   */
  killWorker: () => Promise<void>

  /** Activity log entries for the Kill Worker card. */
  killLog: ChaosActivityEntry[]

  // ---- Disconnect Database state ----

  /** Selected disconnect duration in seconds. Defaults to 15. */
  disconnectDurationSeconds: 15 | 30 | 60

  /** Updates the selected disconnect duration. */
  setDisconnectDurationSeconds: (seconds: 15 | 30 | 60) => void

  /** True while a database disconnect is active (timer is running). */
  isDisconnecting: boolean

  /** Remaining seconds of the active disconnect, or null when inactive. */
  disconnectRemainingSeconds: number | null

  /** True while the disconnect-db API call is in progress (not the same as isDisconnecting). */
  isSubmittingDisconnect: boolean

  /**
   * Calls POST /api/chaos/disconnect-db with the selected duration.
   * Starts the countdown timer. Appends entries to disconnectLog.
   * Refreshes systemStatus after the full disconnect duration elapses.
   *
   * Preconditions:
   *   - isDisconnecting is false.
   *   - isSubmittingDisconnect is false.
   *
   * Postconditions:
   *   - On success: isDisconnecting becomes true; countdown timer starts.
   *   - On 409 (already active): error entry appended to disconnectLog.
   *   - On error: error entry appended to disconnectLog.
   */
  disconnectDatabase: () => Promise<void>

  /** Activity log entries for the Disconnect Database card. */
  disconnectLog: ChaosActivityEntry[]

  // ---- Flood Queue state ----

  /** Currently selected pipeline ID for the Flood Queue action. null if none. */
  selectedFloodPipelineId: string | null

  /** Updates the selected pipeline ID. */
  setSelectedFloodPipelineId: (pipelineId: string | null) => void

  /** Current task count for the flood burst. Controlled by the Flood Queue card. */
  floodTaskCount: number

  /** Updates the task count. Clamped to [1, 1000]. */
  setFloodTaskCount: (count: number) => void

  /** True while the flood-queue API call is in progress. */
  isFlooding: boolean

  /**
   * Flood submission progress as a percentage [0, 100], or null when idle.
   * Set to 100 when the call completes successfully; null otherwise.
   */
  floodProgress: number | null

  /**
   * Calls POST /api/chaos/flood-queue with the selected pipeline and task count.
   * Appends progress and completion entries to floodLog.
   *
   * Preconditions:
   *   - selectedFloodPipelineId is non-null.
   *   - isFlooding is false.
   *   - floodTaskCount is between 1 and 1000 inclusive.
   *
   * Postconditions:
   *   - On success: tasks submitted; completion entry in floodLog with count.
   *   - On error: error entry appended to floodLog.
   */
  floodQueue: () => Promise<void>

  /** Activity log entries for the Flood Queue card. */
  floodLog: ChaosActivityEntry[]
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * makeLogEntry creates a timestamped activity log entry at the given level.
 *
 * @param level   - 'info', 'warn', or 'error'
 * @param message - Human-readable description of the event.
 * @returns A ChaosActivityEntry stamped with the current UTC time.
 */
function makeLogEntry(level: ChaosActivityEntry['level'], message: string): ChaosActivityEntry {
  return {
    timestamp: new Date().toISOString(),
    message,
    level,
  }
}

/**
 * deriveHealthStatus maps a raw health API response to a SystemHealthStatus.
 * Returns 'nominal' when all checks pass, 'degraded' when any check fails.
 *
 * @param body - The parsed JSON body from GET /api/health.
 * @returns 'nominal' | 'degraded'
 */
function deriveHealthStatus(body: Record<string, unknown>): SystemHealthStatus {
  // The health endpoint returns { status: string, checks: { db: string, redis: string } }.
  // Any non-"ok" check value is considered degraded.
  const checks = body['checks'] as Record<string, string> | undefined
  if (!checks) return 'nominal'
  const hasError = Object.values(checks).some(v => v !== 'ok')
  return hasError ? 'degraded' : 'nominal'
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * useChaosController manages all state and API interactions for the Chaos
 * Controller demo view.
 *
 * @returns The full chaos controller state and action callbacks.
 *
 * @throws Never — all errors are captured into activity log entries and
 *   surfaced via the log arrays and loading flags.
 *
 * Preconditions:
 *   - Called from a component rendered within the AuthContext tree.
 *   - Caller must be an Admin (enforced at route level; this hook does not
 *     enforce access itself).
 *
 * Postconditions:
 *   - Workers and pipelines are fetched on mount.
 *   - systemStatus is refreshed after each chaos action completes.
 *   - All activity log arrays grow monotonically; entries are never removed.
 */
export function useChaosController(): UseChaosControllerReturn {
  // ---- Data for selectors ----
  const [workers, setWorkers] = useState<Worker[]>([])
  const [pipelines, setPipelines] = useState<Pipeline[]>([])
  const [isLoadingSelectors, setIsLoadingSelectors] = useState(true)

  // ---- System health ----
  const [systemStatus, setSystemStatus] = useState<SystemHealthStatus>('nominal')

  // ---- Kill Worker state ----
  const [selectedWorkerId, setSelectedWorkerId] = useState<string | null>(null)
  const [isKilling, setIsKilling] = useState(false)
  const [killLog, setKillLog] = useState<ChaosActivityEntry[]>([])

  // ---- Disconnect Database state ----
  const [disconnectDurationSeconds, setDisconnectDurationSeconds] = useState<15 | 30 | 60>(15)
  const [isDisconnecting, setIsDisconnecting] = useState(false)
  const [disconnectRemainingSeconds, setDisconnectRemainingSeconds] = useState<number | null>(null)
  const [isSubmittingDisconnect, setIsSubmittingDisconnect] = useState(false)
  const [disconnectLog, setDisconnectLog] = useState<ChaosActivityEntry[]>([])
  // Countdown timer interval ref — cleaned up on unmount.
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // ---- Flood Queue state ----
  const [selectedFloodPipelineId, setSelectedFloodPipelineId] = useState<string | null>(null)
  const [floodTaskCount, setFloodTaskCountRaw] = useState(10)
  const [isFlooding, setIsFlooding] = useState(false)
  const [floodProgress, setFloodProgress] = useState<number | null>(null)
  const [floodLog, setFloodLog] = useState<ChaosActivityEntry[]>([])

  // ---- Health fetch ----

  /**
   * refreshHealth fetches GET /api/health and updates systemStatus.
   * On network failure, sets status to 'critical'.
   */
  const refreshHealth = useCallback(async () => {
    try {
      const res = await fetch('/api/health', { credentials: 'include' })
      if (!res.ok) {
        setSystemStatus('degraded')
        return
      }
      const body = await res.json() as Record<string, unknown>
      setSystemStatus(deriveHealthStatus(body))
    } catch {
      setSystemStatus('critical')
    }
  }, [])

  // ---- Mount: fetch selectors and initial health ----
  useEffect(() => {
    let cancelled = false

    async function fetchSelectors() {
      try {
        const [fetchedWorkers, fetchedPipelines] = await Promise.all([
          client.listWorkers(),
          client.listPipelines(),
        ])
        if (!cancelled) {
          setWorkers(fetchedWorkers)
          setPipelines(fetchedPipelines)
        }
      } catch {
        // Selector fetch failure is not fatal; the user can retry via page reload.
      } finally {
        if (!cancelled) setIsLoadingSelectors(false)
      }
    }

    fetchSelectors()
    refreshHealth()

    return () => {
      cancelled = true
    }
  }, [refreshHealth])

  // ---- Cleanup countdown on unmount ----
  useEffect(() => {
    return () => {
      if (countdownRef.current !== null) {
        clearInterval(countdownRef.current)
      }
    }
  }, [])

  // ---- Kill Worker action ----

  /**
   * killWorker calls POST /api/chaos/kill-worker for the selected worker.
   * Appends the API response log entries to killLog. Refreshes system health.
   *
   * Preconditions: selectedWorkerId is non-null; isKilling is false.
   */
  const killWorker = useCallback(async () => {
    if (!selectedWorkerId || isKilling) return

    setIsKilling(true)
    setKillLog(prev => [...prev, makeLogEntry('info', `Requesting kill of worker ${selectedWorkerId}...`)])

    try {
      const result = await client.killWorker(selectedWorkerId)
      setKillLog(prev => [...prev, ...result.log])
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setKillLog(prev => [...prev, makeLogEntry('error', `Kill request failed: ${message}`)])
    } finally {
      setIsKilling(false)
      await refreshHealth()
    }
  }, [selectedWorkerId, isKilling, refreshHealth])

  // ---- Disconnect Database action ----

  /**
   * startCountdown starts the per-second countdown timer for the active disconnect.
   * Clears isDisconnecting and disconnectRemainingSeconds when the timer reaches zero.
   * Also triggers a health refresh at completion.
   *
   * @param durationSeconds - Total seconds to count down from.
   */
  const startCountdown = useCallback((durationSeconds: number) => {
    setDisconnectRemainingSeconds(durationSeconds)
    setIsDisconnecting(true)

    if (countdownRef.current !== null) {
      clearInterval(countdownRef.current)
    }

    let remaining = durationSeconds
    countdownRef.current = setInterval(() => {
      remaining -= 1
      setDisconnectRemainingSeconds(remaining)
      if (remaining <= 0) {
        clearInterval(countdownRef.current!)
        countdownRef.current = null
        setIsDisconnecting(false)
        setDisconnectRemainingSeconds(null)
        setDisconnectLog(prev => [
          ...prev,
          makeLogEntry('info', 'Disconnect duration elapsed. Database should be reconnecting.'),
        ])
        refreshHealth()
      }
    }, 1000)
  }, [refreshHealth])

  /**
   * disconnectDatabase calls POST /api/chaos/disconnect-db with the selected duration.
   * On success, starts the countdown timer. Appends entries to disconnectLog.
   *
   * Preconditions: isDisconnecting is false; isSubmittingDisconnect is false.
   */
  const disconnectDatabase = useCallback(async () => {
    if (isDisconnecting || isSubmittingDisconnect) return

    setIsSubmittingDisconnect(true)
    setDisconnectLog(prev => [
      ...prev,
      makeLogEntry('info', `Requesting DB disconnect for ${disconnectDurationSeconds}s...`),
    ])

    try {
      const result = await client.disconnectDatabase(disconnectDurationSeconds)
      setDisconnectLog(prev => [...prev, ...result.log])
      startCountdown(result.durationSeconds)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      // Surface 409 Conflict as a specific message for the UI.
      const userMessage = message.startsWith('409')
        ? 'A database disconnect is already active. Wait for it to complete.'
        : `Disconnect request failed: ${message}`
      setDisconnectLog(prev => [...prev, makeLogEntry('error', userMessage)])
    } finally {
      setIsSubmittingDisconnect(false)
    }
  }, [isDisconnecting, isSubmittingDisconnect, disconnectDurationSeconds, startCountdown])

  // ---- Flood Queue action ----

  /**
   * setFloodTaskCount clamps the task count to [1, 1000] before storing.
   * Precondition: count is a finite integer.
   */
  const setFloodTaskCount = useCallback((count: number) => {
    setFloodTaskCountRaw(Math.min(1000, Math.max(1, Math.trunc(count))))
  }, [])

  /**
   * floodQueue calls POST /api/chaos/flood-queue with the selected pipeline and count.
   * Appends the response log entries to floodLog. Sets floodProgress on completion.
   *
   * Preconditions: selectedFloodPipelineId is non-null; isFlooding is false.
   */
  const floodQueue = useCallback(async () => {
    if (!selectedFloodPipelineId || isFlooding) return

    setIsFlooding(true)
    setFloodProgress(null)
    setFloodLog(prev => [
      ...prev,
      makeLogEntry('info', `Submitting burst of ${floodTaskCount} tasks to pipeline ${selectedFloodPipelineId}...`),
    ])

    try {
      const result = await client.floodQueue(selectedFloodPipelineId, floodTaskCount)
      setFloodLog(prev => [...prev, ...result.log])
      setFloodProgress(100)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setFloodLog(prev => [...prev, makeLogEntry('error', `Flood request failed: ${message}`)])
      setFloodProgress(null)
    } finally {
      setIsFlooding(false)
    }
  }, [selectedFloodPipelineId, isFlooding, floodTaskCount])

  return {
    // Selectors
    workers,
    pipelines,
    isLoadingSelectors,

    // Health
    systemStatus,

    // Kill Worker
    selectedWorkerId,
    setSelectedWorkerId,
    isKilling,
    killWorker,
    killLog,

    // Disconnect DB
    disconnectDurationSeconds,
    setDisconnectDurationSeconds,
    isDisconnecting,
    disconnectRemainingSeconds,
    isSubmittingDisconnect,
    disconnectDatabase,
    disconnectLog,

    // Flood Queue
    selectedFloodPipelineId,
    setSelectedFloodPipelineId,
    floodTaskCount,
    setFloodTaskCount,
    isFlooding,
    floodProgress,
    floodQueue,
    floodLog,
  }
}
