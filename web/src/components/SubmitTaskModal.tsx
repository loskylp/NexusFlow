/**
 * SubmitTaskModal — modal dialog for submitting a new task (TASK-021, TASK-035).
 *
 * Contains:
 *   - Pipeline selector dropdown (populated from props, sourced from GET /api/pipelines)
 *   - Input parameters form: dynamic key-value rows; users add/remove rows; inline
 *     validation checks for empty keys and duplicate keys before submission
 *   - Retry configuration: max retries (integer, default 0) and backoff strategy selector
 *   - Submit button: shows spinner during submission, disabled while submitting or invalid
 *   - Cancel button: closes modal without submitting
 *
 * On successful submission calls onSuccess with the new task ID, then calls onClose.
 * Inline validation prevents submission with empty parameter keys or duplicate keys.
 * API errors are displayed inline below the form.
 *
 * See: TASK-021, TASK-035, REQ-002, UX Spec (Task Feed and Monitor — key interactions)
 */

import React, { useEffect, useId, useState } from 'react'
import type { BackoffStrategy, Pipeline } from '@/types/domain'
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

/**
 * A single row in the parameter editor.
 * Each row tracks its own key, value, and a stable React key for list rendering.
 */
interface ParamRow {
  /** Stable React list key — assigned at row creation; never changes. */
  rowId: string
  key: string
  value: string
}

/** Full form state owned by the modal. */
interface SubmitFormState {
  pipelineId: string
  params: ParamRow[]
  maxRetries: number
  backoff: BackoffStrategy
  isSubmitting: boolean
  apiError: string | null
  validationError: string | null
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

/**
 * buildInitialState constructs the default form state for a given pipeline selection.
 * Returns params=[], maxRetries=0, backoff='fixed', with no errors.
 */
function buildInitialState(
  initialPipelineId: string | undefined,
  pipelines: Pipeline[]
): SubmitFormState {
  const pipelineId = initialPipelineId ?? pipelines[0]?.id ?? ''
  return {
    pipelineId,
    params: [],
    maxRetries: 0,
    backoff: 'fixed',
    isSubmitting: false,
    apiError: null,
    validationError: null,
  }
}

/**
 * validateParams checks the parameter rows for empty keys and duplicate keys.
 * Returns an error message string if invalid, or null if valid.
 *
 * Precondition: rows is a non-null array.
 * Postcondition: Returns null when all keys are non-empty and unique.
 */
function validateParams(rows: ParamRow[]): string | null {
  for (const row of rows) {
    if (row.key.trim() === '') {
      return 'Parameter key cannot be empty'
    }
  }
  const keys = rows.map(r => r.key.trim())
  const uniqueKeys = new Set(keys)
  if (uniqueKeys.size !== keys.length) {
    return 'Duplicate parameter key — each key must be unique'
  }
  return null
}

/**
 * buildInputRecord converts a list of ParamRow entries into the Record<string, unknown>
 * expected by the POST /api/tasks payload.
 */
function buildInputRecord(rows: ParamRow[]): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const row of rows) {
    result[row.key.trim()] = row.value
  }
  return result
}

// ---------------------------------------------------------------------------
// Sub-component: ParameterRow
// ---------------------------------------------------------------------------

interface ParameterRowProps {
  row: ParamRow
  onChange: (rowId: string, field: 'key' | 'value', value: string) => void
  onRemove: (rowId: string) => void
  disabled: boolean
}

/**
 * ParameterRow renders a single key-value input pair with a remove button.
 * Pure presentational — all state is managed by SubmitTaskModal.
 */
function ParameterRow({ row, onChange, onRemove, disabled }: ParameterRowProps): React.ReactElement {
  return (
    <div
      style={{
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
        marginBottom: '8px',
      }}
    >
      <input
        type="text"
        placeholder="Key"
        value={row.key}
        onChange={e => onChange(row.rowId, 'key', e.target.value)}
        disabled={disabled}
        aria-label={`Parameter key ${row.rowId}`}
        style={{
          flex: 1,
          padding: '6px 8px',
          fontSize: '13px',
          border: '1px solid #E2E8F0',
          borderRadius: '6px',
          backgroundColor: '#F1F5F9',
          color: '#0F172A',
          fontFamily: 'JetBrains Mono, monospace',
        }}
      />
      <input
        type="text"
        placeholder="Value"
        value={row.value}
        onChange={e => onChange(row.rowId, 'value', e.target.value)}
        disabled={disabled}
        aria-label={`Parameter value ${row.rowId}`}
        style={{
          flex: 2,
          padding: '6px 8px',
          fontSize: '13px',
          border: '1px solid #E2E8F0',
          borderRadius: '6px',
          backgroundColor: '#F1F5F9',
          color: '#0F172A',
          fontFamily: 'JetBrains Mono, monospace',
        }}
      />
      <button
        type="button"
        onClick={() => onRemove(row.rowId)}
        disabled={disabled}
        aria-label="Remove parameter"
        style={{
          padding: '6px 8px',
          border: '1px solid #E2E8F0',
          borderRadius: '6px',
          background: 'none',
          cursor: disabled ? 'not-allowed' : 'pointer',
          color: '#64748B',
          fontSize: '14px',
          lineHeight: 1,
        }}
      >
        ×
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Label style constant
// ---------------------------------------------------------------------------

const LABEL_STYLE: React.CSSProperties = {
  display: 'block',
  fontSize: '12px',
  fontFamily: 'IBM Plex Sans, sans-serif',
  textTransform: 'uppercase',
  letterSpacing: '0.05em',
  color: '#64748B',
  marginBottom: '6px',
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

let _rowCounter = 0
/** generateRowId creates a unique, stable string ID for a new parameter row. */
function generateRowId(): string {
  _rowCounter += 1
  return `param-row-${_rowCounter}`
}

/**
 * SubmitTaskModal renders the task submission form in an accessible dialog.
 *
 * Pipeline selector, parameter form (key-value rows), retry configuration,
 * inline validation, and submission to POST /api/tasks.
 *
 * Preconditions:
 *   - pipelines list is already fetched by the parent.
 *   - onSuccess and onClose are stable callbacks.
 *
 * Postconditions:
 *   - On success: onSuccess(taskId) and onClose() are called. Form is NOT reset
 *     here because the component will be re-mounted/re-opened to reset.
 *   - On cancel: onClose() is called; no API call is made.
 *   - Form state is reset when isOpen transitions false -> true (useEffect).
 *   - Returns null when isOpen is false (avoids DOM cost of hidden modal).
 */
function SubmitTaskModal({
  isOpen,
  onClose,
  onSuccess,
  pipelines,
  initialPipelineId,
}: SubmitTaskModalProps): React.ReactElement | null {
  const pipelineLabelId = useId()
  const maxRetriesLabelId = useId()
  const backoffLabelId = useId()

  const [form, setForm] = useState<SubmitFormState>(() =>
    buildInitialState(initialPipelineId, pipelines)
  )

  // Reset all form state each time the modal opens (isOpen: false -> true).
  useEffect(() => {
    if (isOpen) {
      setForm(buildInitialState(initialPipelineId, pipelines))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen])

  if (!isOpen) return null

  // -------------------------------------------------------------------------
  // Event handlers
  // -------------------------------------------------------------------------

  /** handlePipelineChange updates the selected pipeline ID. */
  function handlePipelineChange(e: React.ChangeEvent<HTMLSelectElement>): void {
    setForm(prev => ({ ...prev, pipelineId: e.target.value }))
  }

  /** handleAddParam appends a new empty parameter row. */
  function handleAddParam(): void {
    setForm(prev => ({
      ...prev,
      params: [...prev.params, { rowId: generateRowId(), key: '', value: '' }],
      validationError: null,
    }))
  }

  /** handleParamChange updates a single field in one parameter row, clearing any validation error. */
  function handleParamChange(rowId: string, field: 'key' | 'value', value: string): void {
    setForm(prev => ({
      ...prev,
      params: prev.params.map(r => r.rowId === rowId ? { ...r, [field]: value } : r),
      validationError: null,
    }))
  }

  /** handleRemoveParam removes a parameter row by its stable row ID. */
  function handleRemoveParam(rowId: string): void {
    setForm(prev => ({
      ...prev,
      params: prev.params.filter(r => r.rowId !== rowId),
      validationError: null,
    }))
  }

  /** handleMaxRetriesChange parses the integer value, clamping negatives to 0. */
  function handleMaxRetriesChange(e: React.ChangeEvent<HTMLInputElement>): void {
    const parsed = parseInt(e.target.value, 10)
    const value = isNaN(parsed) ? 0 : Math.max(0, parsed)
    setForm(prev => ({ ...prev, maxRetries: value }))
  }

  /** handleBackoffChange updates the backoff strategy. */
  function handleBackoffChange(e: React.ChangeEvent<HTMLSelectElement>): void {
    setForm(prev => ({ ...prev, backoff: e.target.value as BackoffStrategy }))
  }

  /**
   * handleSubmit validates the form and sends POST /api/tasks.
   * Validation errors are shown inline and prevent submission.
   * Network/API errors are shown in the error alert below the form.
   */
  async function handleSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault()

    // Precondition: a pipeline must be selected.
    if (!form.pipelineId || form.isSubmitting) return

    // Validate parameters before sending the request.
    const validationError = validateParams(form.params)
    if (validationError !== null) {
      setForm(prev => ({ ...prev, validationError }))
      return
    }

    setForm(prev => ({ ...prev, isSubmitting: true, apiError: null, validationError: null }))

    try {
      // Only include retryConfig when the user has specified a non-default value.
      // The API treats an absent retryConfig as "use system defaults" (REQ-001).
      // Default is maxRetries=0, backoff='fixed' — equivalent to no retry config.
      const retryConfig =
        form.maxRetries > 0
          ? { maxRetries: form.maxRetries, backoff: form.backoff }
          : undefined

      const result = await submitTask({
        pipelineId: form.pipelineId,
        input: buildInputRecord(form.params),
        ...(retryConfig !== undefined ? { retryConfig } : {}),
      })
      // Postcondition: notify parent with new task ID, then close.
      onSuccess(result.taskId)
      onClose()
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Failed to submit task'
      setForm(prev => ({ ...prev, isSubmitting: false, apiError: message }))
    }
  }

  /** handleClose closes the modal unless a submission is in progress. */
  function handleClose(): void {
    if (!form.isSubmitting) onClose()
  }

  const canSubmit = Boolean(form.pipelineId) && !form.isSubmitting && form.validationError === null

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

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
        backgroundColor: 'rgba(15, 23, 42, 0.5)',
      }}
      onClick={(e) => { if (e.target === e.currentTarget) handleClose() }}
    >
      <div
        style={{
          backgroundColor: '#FFFFFF',
          border: '1px solid #E2E8F0',
          borderRadius: '8px',
          padding: '24px',
          width: '480px',
          maxWidth: 'calc(100vw - 48px)',
          maxHeight: 'calc(100vh - 96px)',
          overflowY: 'auto',
        }}
      >
        {/* ---------------------------------------------------------------- */}
        {/* Header                                                            */}
        {/* ---------------------------------------------------------------- */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginBottom: '20px',
          }}
        >
          <h2
            style={{
              margin: 0,
              fontSize: '16px',
              fontWeight: 600,
              color: '#0F172A',
              fontFamily: 'Inter, sans-serif',
            }}
          >
            Submit Task
          </h2>
          <button
            type="button"
            aria-label="Close"
            onClick={handleClose}
            disabled={form.isSubmitting}
            style={{
              background: 'none',
              border: 'none',
              fontSize: '18px',
              cursor: form.isSubmitting ? 'not-allowed' : 'pointer',
              color: '#64748B',
              padding: '4px',
              lineHeight: 1,
            }}
          >
            ×
          </button>
        </div>

        <form onSubmit={handleSubmit}>
          {/* -------------------------------------------------------------- */}
          {/* Pipeline selector                                               */}
          {/* -------------------------------------------------------------- */}
          <div style={{ marginBottom: '16px' }}>
            <label id={pipelineLabelId} htmlFor="submit-task-pipeline" style={LABEL_STYLE}>
              Pipeline
            </label>
            <select
              id="submit-task-pipeline"
              aria-labelledby={pipelineLabelId}
              value={form.pipelineId}
              onChange={handlePipelineChange}
              disabled={form.isSubmitting}
              style={{
                width: '100%',
                padding: '8px 10px',
                fontSize: '14px',
                border: '1px solid #E2E8F0',
                borderRadius: '6px',
                backgroundColor: '#F1F5F9',
                color: '#0F172A',
                cursor: form.isSubmitting ? 'not-allowed' : 'pointer',
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

          {/* -------------------------------------------------------------- */}
          {/* Parameters                                                      */}
          {/* -------------------------------------------------------------- */}
          <div style={{ marginBottom: '16px' }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                marginBottom: '8px',
              }}
            >
              <span style={LABEL_STYLE}>Parameters</span>
              <button
                type="button"
                onClick={handleAddParam}
                disabled={form.isSubmitting}
                aria-label="Add parameter"
                style={{
                  padding: '4px 10px',
                  fontSize: '12px',
                  border: '1px solid #E2E8F0',
                  borderRadius: '6px',
                  backgroundColor: '#FFFFFF',
                  color: '#4F46E5',
                  cursor: form.isSubmitting ? 'not-allowed' : 'pointer',
                  fontWeight: 500,
                }}
              >
                + Add Parameter
              </button>
            </div>

            {form.params.length === 0 && (
              <p
                style={{
                  fontSize: '13px',
                  color: '#94A3B8',
                  margin: 0,
                  fontStyle: 'italic',
                }}
              >
                No parameters. Click "Add Parameter" to supply input key-value pairs.
              </p>
            )}

            {form.params.map(row => (
              <ParameterRow
                key={row.rowId}
                row={row}
                onChange={handleParamChange}
                onRemove={handleRemoveParam}
                disabled={form.isSubmitting}
              />
            ))}

            {/* Inline validation error for parameters */}
            {form.validationError !== null && (
              <p
                style={{
                  fontSize: '12px',
                  color: '#DC2626',
                  margin: '4px 0 0',
                }}
              >
                {form.validationError}
              </p>
            )}
          </div>

          {/* -------------------------------------------------------------- */}
          {/* Retry configuration                                             */}
          {/* -------------------------------------------------------------- */}
          <div style={{ marginBottom: '20px' }}>
            <span style={{ ...LABEL_STYLE, display: 'block', marginBottom: '12px' }}>
              Retry Configuration
            </span>
            <div style={{ display: 'flex', gap: '16px' }}>
              <div style={{ flex: 1 }}>
                <label
                  id={maxRetriesLabelId}
                  htmlFor="submit-task-max-retries"
                  style={{ ...LABEL_STYLE, marginBottom: '4px' }}
                >
                  Max Retries
                </label>
                <input
                  id="submit-task-max-retries"
                  aria-labelledby={maxRetriesLabelId}
                  type="number"
                  min={0}
                  max={10}
                  value={form.maxRetries}
                  onChange={handleMaxRetriesChange}
                  disabled={form.isSubmitting}
                  style={{
                    width: '100%',
                    padding: '7px 8px',
                    fontSize: '14px',
                    border: '1px solid #E2E8F0',
                    borderRadius: '6px',
                    backgroundColor: '#F1F5F9',
                    color: '#0F172A',
                    boxSizing: 'border-box',
                  }}
                />
              </div>
              <div style={{ flex: 2 }}>
                <label
                  id={backoffLabelId}
                  htmlFor="submit-task-backoff"
                  style={{ ...LABEL_STYLE, marginBottom: '4px' }}
                >
                  Backoff Strategy
                </label>
                <select
                  id="submit-task-backoff"
                  aria-labelledby={backoffLabelId}
                  value={form.backoff}
                  onChange={handleBackoffChange}
                  disabled={form.isSubmitting}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    fontSize: '14px',
                    border: '1px solid #E2E8F0',
                    borderRadius: '6px',
                    backgroundColor: '#F1F5F9',
                    color: '#0F172A',
                    cursor: form.isSubmitting ? 'not-allowed' : 'pointer',
                  }}
                >
                  <option value="fixed">Fixed</option>
                  <option value="linear">Linear</option>
                  <option value="exponential">Exponential</option>
                </select>
              </div>
            </div>
          </div>

          {/* -------------------------------------------------------------- */}
          {/* API error                                                       */}
          {/* -------------------------------------------------------------- */}
          {form.apiError !== null && (
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
              {form.apiError}
            </div>
          )}

          {/* -------------------------------------------------------------- */}
          {/* Footer actions                                                  */}
          {/* -------------------------------------------------------------- */}
          <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
            <button
              type="button"
              onClick={handleClose}
              disabled={form.isSubmitting}
              style={{
                padding: '8px 16px',
                border: '1px solid #E2E8F0',
                borderRadius: '6px',
                background: 'none',
                cursor: form.isSubmitting ? 'not-allowed' : 'pointer',
                fontSize: '14px',
                color: '#64748B',
                opacity: form.isSubmitting ? 0.5 : 1,
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
              {form.isSubmitting ? (
                <>
                  <span
                    aria-hidden="true"
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
