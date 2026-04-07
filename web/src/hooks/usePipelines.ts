/**
 * usePipelines — hook for listing pipelines visible to the current user.
 *
 * Fetches GET /api/pipelines on mount. Provides a refresh function for
 * post-create or post-delete re-fetching. No SSE — pipeline changes are
 * low-frequency design-time operations and do not require real-time updates.
 *
 * See: TASK-023, TASK-024, REQ-022
 */

import type { Pipeline } from '@/types/domain'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface UsePipelinesReturn {
  /** Pipelines visible to the current user (role-filtered server-side). */
  pipelines: Pipeline[]
  /** Whether the fetch is in progress. */
  isLoading: boolean
  /** Non-null when the fetch failed. */
  error: string | null
  /** Re-fetch the pipeline list from the API. */
  refresh: () => void
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * usePipelines fetches and caches the list of pipelines accessible to the
 * current user.
 *
 * Preconditions:
 *   - User must be authenticated.
 *
 * Postconditions:
 *   - pipelines is populated from GET /api/pipelines on mount.
 *   - Calling refresh() re-fetches the list from the server.
 *   - User role sees only their own pipelines; Admin sees all (server-enforced).
 */
export function usePipelines(): UsePipelinesReturn {
  // TODO: implement
  throw new Error('Not implemented')
}
