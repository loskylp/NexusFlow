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
 * NOTE: This is a minimal implementation for TASK-023 (Run button flow).
 * Full implementation with parameter form and retry config is in TASK-035.
 *
 * See: TASK-021, TASK-035, REQ-002, UX Spec (Task Feed and Monitor — key interactions)
 */

import React, { useEffect, useState } from 'react'
import type { Pipeline } from '@/types/domain'
import { submitTask } from '@/api/client'

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
  isSubmitting: boolean
  error: string | null
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * SubmitTaskModal renders the task submission form in an accessible dialog.
 *
 * Minimal implementation for TASK-023:
 *   - Pipeline pre-selected when initialPipelineId is provided
 *   - Submit button calls POST /api/tasks with empty input
 *   - Spinner state during submission
 *   - Error display on API failure
 *
 * Full TASK-035 implementation adds: parameter form, retry configuration,
 * inline field validation.
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
  const [formState, setFormState] = useState<SubmitTaskFormState>({
    pipelineId: initialPipelineId ?? pipelines[0]?.id ?? '',
    isSubmitting: false,
    error: null,
  })

  // Reset form state each time the modal opens.
  useEffect(() => {
    if (isOpen) {
      setFormState({
        pipelineId: initialPipelineId ?? pipelines[0]?.id ?? '',
        isSubmitting: false,
        error: null,
      })
    }
  }, [isOpen, initialPipelineId, pipelines])

  if (!isOpen) return null

  /** handleSubmit sends POST /api/tasks with the selected pipeline. */
  async function handleSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault()
    if (!formState.pipelineId || formState.isSubmitting) return

    setFormState(prev => ({ ...prev, isSubmitting: true, error: null }))
    try {
      const result = await submitTask({
        pipelineId: formState.pipelineId,
        input: {},
      })
      onSuccess(result.taskId)
      onClose()
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Failed to submit task'
      setFormState(prev => ({ ...prev, isSubmitting: false, error: message }))
    }
  }

  const canSubmit = Boolean(formState.pipelineId) && !formState.isSubmitting

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Submit Task"
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundColor: 'rgba(0,0,0,0.4)',
      }}
      onClick={(e) => { if (e.target === e.currentTarget && !formState.isSubmitting) onClose() }}
    >
      <div
        style={{
          backgroundColor: 'var(--color-surface-panel)',
          border: '1px solid var(--color-border)',
          borderRadius: '8px',
          padding: '24px',
          width: '420px',
          boxShadow: '0 8px 32px rgba(0,0,0,0.24)',
        }}
      >
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
          <h2 style={{ margin: 0, fontSize: '16px', fontWeight: 600, color: 'var(--color-text-primary)' }}>
            Submit Task
          </h2>
          <button
            aria-label="Close"
            onClick={() => { if (!formState.isSubmitting) onClose() }}
            style={{
              background: 'none',
              border: 'none',
              fontSize: '18px',
              cursor: formState.isSubmitting ? 'not-allowed' : 'pointer',
              color: 'var(--color-text-secondary)',
              padding: '4px',
            }}
          >
            ×
          </button>
        </div>

        <form onSubmit={handleSubmit}>
          {/* Pipeline selector */}
          <div style={{ marginBottom: '16px' }}>
            <label
              htmlFor="submit-task-pipeline"
              style={{
                display: 'block',
                fontSize: '12px',
                fontFamily: 'var(--font-label)',
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
                color: 'var(--color-text-secondary)',
                marginBottom: '6px',
              }}
            >
              Pipeline
            </label>
            <select
              id="submit-task-pipeline"
              value={formState.pipelineId}
              onChange={e => setFormState(prev => ({ ...prev, pipelineId: e.target.value }))}
              disabled={formState.isSubmitting}
              style={{
                width: '100%',
                padding: '8px 10px',
                fontSize: '14px',
                border: '1px solid var(--color-border)',
                borderRadius: '6px',
                backgroundColor: 'var(--color-surface-panel)',
                color: 'var(--color-text-primary)',
                cursor: formState.isSubmitting ? 'not-allowed' : 'pointer',
              }}
            >
              {pipelines.length === 0 && (
                <option value="">No pipelines available</option>
              )}
              {pipelines.map(p => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
          </div>

          {/* Minimal notice about full form in TASK-035 */}
          <p style={{ fontSize: '12px', color: 'var(--color-text-secondary)', marginBottom: '16px' }}>
            Task will be submitted with empty input parameters. Full parameter form available in a future update.
          </p>

          {/* Error display */}
          {formState.error && (
            <div
              role="alert"
              style={{
                padding: '8px 12px',
                marginBottom: '16px',
                backgroundColor: '#FEF2F2',
                border: '1px solid #FECACA',
                borderRadius: '6px',
                fontSize: '13px',
                color: '#DC2626',
              }}
            >
              {formState.error}
            </div>
          )}

          {/* Footer actions */}
          <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
            <button
              type="button"
              onClick={() => { if (!formState.isSubmitting) onClose() }}
              disabled={formState.isSubmitting}
              style={{
                padding: '8px 16px',
                border: '1px solid var(--color-border)',
                borderRadius: '6px',
                background: 'none',
                cursor: formState.isSubmitting ? 'not-allowed' : 'pointer',
                fontSize: '14px',
                color: 'var(--color-text-secondary)',
                opacity: formState.isSubmitting ? 0.5 : 1,
              }}
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              style={{
                padding: '8px 16px',
                border: 'none',
                borderRadius: '6px',
                backgroundColor: canSubmit ? '#4F46E5' : '#94A3B8',
                color: '#FFFFFF',
                cursor: canSubmit ? 'pointer' : 'not-allowed',
                fontSize: '14px',
                fontWeight: 500,
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                opacity: canSubmit ? 1 : 0.6,
              }}
            >
              {formState.isSubmitting ? (
                <>
                  <span
                    style={{
                      display: 'inline-block',
                      width: '12px',
                      height: '12px',
                      border: '2px solid rgba(255,255,255,0.3)',
                      borderTopColor: '#FFFFFF',
                      borderRadius: '50%',
                      animation: 'spin 0.8s linear infinite',
                    }}
                  />
                  Submitting...
                </>
              ) : (
                'Submit Task'
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default SubmitTaskModal
