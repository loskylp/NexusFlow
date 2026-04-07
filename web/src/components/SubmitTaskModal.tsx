/**
 * SubmitTaskModal — modal dialog for submitting a new task (TASK-021, TASK-035).
 *
 * Contains:
 *   - Pipeline selector dropdown (populated from GET /api/pipelines)
 *   - Input parameters form (key-value pairs; inline validation for required fields)
 *   - Retry configuration (max retries integer input, backoff strategy selector)
 *   - Submit button (shows spinner during submission, disabled on invalid form)
 *   - Cancel button (closes modal without submitting)
 *
 * On successful submission, calls onSuccess with the new task ID and closes.
 * Inline validation prevents submission with missing required parameters.
 *
 * See: TASK-021, TASK-035, REQ-002, UX Spec (Task Feed and Monitor — key interactions)
 */

import React from 'react'
import type { Pipeline, RetryConfig } from '@/types/domain'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SubmitTaskModalProps {
  /** Whether the modal is currently visible. */
  isOpen: boolean
  /** Called to close the modal (user clicked Cancel or submission succeeded). */
  onClose: () => void
  /**
   * Called with the new task ID after a successful POST /api/tasks.
   * The parent should update the task list (refresh or optimistic add).
   */
  onSuccess: (taskId: string) => void
  /**
   * Pipelines to populate the pipeline selector.
   * Passed in from the parent (which holds usePipelines) to avoid double fetching.
   */
  pipelines: Pipeline[]
  /**
   * When set, the modal opens with this pipeline pre-selected.
   * Used when "Run" is clicked from the Pipeline Builder (TASK-023).
   */
  initialPipelineId?: string
}

/** The form state managed inside the modal. */
interface SubmitTaskFormState {
  pipelineId: string
  input: Record<string, string>
  retryConfig: RetryConfig
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * SubmitTaskModal renders the task submission form in an accessible dialog.
 *
 * States handled:
 *   - Idle: form with empty or pre-filled values, Submit button enabled if valid
 *   - Submitting: Submit button shows inline spinner, all inputs disabled
 *   - Validation error: inline red error messages below offending fields
 *   - API error: toast notification via the toast system (non-blocking)
 *
 * Preconditions:
 *   - pipelines list is already fetched by the parent.
 *   - onSuccess is a stable callback.
 *
 * Postconditions:
 *   - On success: onClose is called; onSuccess is called with new task ID.
 *   - On cancel: onClose is called; no API call is made.
 *   - Form state is reset when the modal is re-opened (isOpen transitions false -> true).
 */
function SubmitTaskModal({
  isOpen,
  onClose,
  onSuccess,
  pipelines,
  initialPipelineId,
}: SubmitTaskModalProps): React.ReactElement | null {
  // TODO: implement
  throw new Error('Not implemented')
}

export default SubmitTaskModal
