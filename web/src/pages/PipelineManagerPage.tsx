/**
 * PipelineManagerPage — Pipeline Builder and pipeline management GUI (TASK-023, TASK-024).
 *
 * This page serves dual purpose:
 *   1. Pipeline Builder (TASK-023): drag-and-drop canvas with schema mapping editor.
 *      Accessed via /pipelines or /pipelines/new
 *   2. Pipeline management (TASK-024): list/edit/delete actions for saved pipelines.
 *      Accessed via /pipelines; the left panel shows the saved pipelines list.
 *
 * Layout (UX spec — Pipeline Builder):
 *   - Sidebar navigation (240px): global nav
 *   - Component palette (200px left panel):
 *       - Draggable phase cards: DataSource, Process, Sink
 *       - Saved pipelines list (for management: edit, delete)
 *   - Canvas (remaining width): dot-grid, pipeline nodes, connector lines,
 *       schema mapping chips (via PipelineCanvas component)
 *   - Canvas toolbar: pipeline name field, Save button, Run button, Clear button
 *
 * Pipeline Builder state machine:
 *   empty → editing (components placed) → saving → saved (toast) → editing
 *   unsaved changes: asterisk next to pipeline name, nav confirmation dialog
 *
 * Pipeline management (TASK-024):
 *   - Edit: clicking a saved pipeline loads it onto the canvas
 *   - Delete: confirmation dialog + DELETE /api/pipelines/{id}
 *     - Blocked with explanation when pipeline has active tasks (409 response)
 *   - List: shows user's own pipelines (User) or all pipelines (Admin)
 *
 * See: REQ-015, REQ-007, REQ-023, TASK-023, TASK-024, TASK-026,
 *      ADR-008, UX Spec (Pipeline Builder)
 */

import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useBlocker } from 'react-router-dom'
import type { Pipeline } from '@/types/domain'
import type { PipelineCanvasState, MappingValidationError, PipelinePhase } from '@/components/PipelineCanvas'
import PipelineCanvas, { DraggablePaletteCard, applyPhaseDropToState, CANVAS_DROP_ID } from '@/components/PipelineCanvas'
import { DndContext, type DragEndEvent, useSensor, useSensors, PointerSensor } from '@dnd-kit/core'
import { usePipelines } from '@/hooks/usePipelines'
import { useAuth } from '@/context/AuthContext'
import {
  createPipeline,
  updatePipeline,
  getPipeline,
  deletePipeline,
} from '@/api/client'
import SubmitTaskModal from '@/components/SubmitTaskModal'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Internal page state for the pipeline builder editor session. */
interface PipelineEditorState {
  /** null = new pipeline, non-null = editing existing pipeline */
  pipelineId: string | null
  name: string
  canvas: PipelineCanvasState
  /** True when canvas or name has changed since last save. */
  hasUnsavedChanges: boolean
  /** True when the save API call is in progress. */
  isSaving: boolean
  /** Validation errors returned by the API on save attempt. */
  validationErrors: MappingValidationError[]
}

/** Empty canvas state used when clearing or starting fresh. */
const EMPTY_CANVAS: PipelineCanvasState = {
  dataSource: null,
  process: null,
  sink: null,
  dataSourceToProcessMappings: [],
  processToSinkMappings: [],
}

/** Initial editor state (blank). */
const INITIAL_EDITOR: PipelineEditorState = {
  pipelineId: null,
  name: '',
  canvas: EMPTY_CANVAS,
  hasUnsavedChanges: false,
  isSaving: false,
  validationErrors: [],
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface ComponentPaletteProps {
  savedPipelines: Array<{ id: string; name: string }>
  isAdmin: boolean
  onLoadPipeline: (pipelineId: string) => void
  onDeletePipeline: (pipelineId: string) => void
  isLoadingPipelines: boolean
  /** Whether a DataSource, Process, or Sink is already placed on the canvas. */
  isDataSourcePlaced: boolean
  isProcessPlaced: boolean
  isSinkPlaced: boolean
  readOnly: boolean
}

/**
 * ComponentPalette renders the left panel with draggable phase cards
 * and the saved pipelines list.
 *
 * Phase cards are drag sources for dnd-kit. Dropping them onto the canvas
 * triggers the PipelineCanvas onChange handler.
 *
 * Saved pipelines list:
 *   - Click a pipeline: calls onLoadPipeline (loads onto canvas)
 *   - Delete icon on a pipeline: shows confirmation dialog, then calls onDeletePipeline
 *   - Delete is disabled when pipeline has active tasks (surfaced as 409 from API)
 *
 * Preconditions:
 *   - onLoadPipeline and onDeletePipeline are stable callbacks.
 */
function ComponentPalette({
  savedPipelines,
  onLoadPipeline,
  onDeletePipeline,
  isLoadingPipelines,
  isDataSourcePlaced,
  isProcessPlaced,
  isSinkPlaced,
  readOnly,
}: ComponentPaletteProps): React.ReactElement {
  const [deletingId, setDeletingId] = useState<string | null>(null)

  return (
    <div
      style={{
        width: '200px',
        flexShrink: 0,
        display: 'flex',
        flexDirection: 'column',
        gap: '0',
        backgroundColor: 'var(--color-surface-panel)',
        border: '1px solid var(--color-border)',
        borderRadius: '8px',
        overflow: 'hidden',
      }}
    >
      {/* Phase cards section */}
      <div style={{ padding: '12px' }}>
        <div
          style={{
            fontSize: '11px',
            fontFamily: 'var(--font-label)',
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
            color: 'var(--color-text-secondary)',
            marginBottom: '10px',
          }}
        >
          Components
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <DraggablePaletteCard
            phase="DataSource"
            disabled={readOnly || isDataSourcePlaced}
            disabledReason={isDataSourcePlaced ? 'DataSource already placed — one per pipeline' : undefined}
          />
          <DraggablePaletteCard
            phase="Process"
            disabled={readOnly || isProcessPlaced || !isDataSourcePlaced}
            disabledReason={
              isProcessPlaced
                ? 'Process already placed — one per pipeline'
                : !isDataSourcePlaced
                  ? 'Place a DataSource first'
                  : undefined
            }
          />
          <DraggablePaletteCard
            phase="Sink"
            disabled={readOnly || isSinkPlaced || !isProcessPlaced}
            disabledReason={
              isSinkPlaced
                ? 'Sink already placed — one per pipeline'
                : !isProcessPlaced
                  ? 'Place a Process first'
                  : undefined
            }
          />
        </div>
      </div>

      {/* Divider */}
      <div style={{ borderTop: '1px solid var(--color-border)' }} />

      {/* Saved pipelines section */}
      <div style={{ flex: 1, overflow: 'auto', padding: '12px' }}>
        <div
          style={{
            fontSize: '11px',
            fontFamily: 'var(--font-label)',
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
            color: 'var(--color-text-secondary)',
            marginBottom: '10px',
          }}
        >
          Saved Pipelines
        </div>

        {isLoadingPipelines ? (
          <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)' }}>Loading...</div>
        ) : savedPipelines.length === 0 ? (
          <div style={{ fontSize: '12px', color: 'var(--color-text-tertiary)', fontStyle: 'italic' }}>
            No saved pipelines
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
            {savedPipelines.map(pipeline => (
              <div
                key={pipeline.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  borderRadius: '4px',
                  overflow: 'hidden',
                }}
              >
                {deletingId === pipeline.id ? (
                  // Confirmation inline
                  <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '4px', padding: '4px' }}>
                    <span style={{ fontSize: '11px', color: '#DC2626' }}>Delete "{pipeline.name}"?</span>
                    <div style={{ display: 'flex', gap: '4px' }}>
                      <button
                        onClick={() => { onDeletePipeline(pipeline.id); setDeletingId(null) }}
                        style={{
                          flex: 1,
                          padding: '3px 6px',
                          fontSize: '11px',
                          backgroundColor: '#DC2626',
                          color: '#FFFFFF',
                          border: 'none',
                          borderRadius: '3px',
                          cursor: 'pointer',
                        }}
                      >
                        Delete
                      </button>
                      <button
                        onClick={() => setDeletingId(null)}
                        style={{
                          flex: 1,
                          padding: '3px 6px',
                          fontSize: '11px',
                          backgroundColor: 'none',
                          border: '1px solid var(--color-border)',
                          borderRadius: '3px',
                          cursor: 'pointer',
                          color: 'var(--color-text-secondary)',
                        }}
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : (
                  <>
                    <button
                      onClick={() => onLoadPipeline(pipeline.id)}
                      title={`Load pipeline: ${pipeline.name}`}
                      style={{
                        flex: 1,
                        padding: '6px 8px',
                        textAlign: 'left',
                        fontSize: '12px',
                        background: 'none',
                        border: 'none',
                        cursor: 'pointer',
                        color: 'var(--color-text-primary)',
                        borderRadius: '4px',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                      onMouseEnter={e => { (e.currentTarget as HTMLElement).style.backgroundColor = '#EFF6FF' }}
                      onMouseLeave={e => { (e.currentTarget as HTMLElement).style.backgroundColor = 'transparent' }}
                    >
                      {pipeline.name}
                    </button>
                    <button
                      onClick={() => setDeletingId(pipeline.id)}
                      aria-label={`Delete pipeline ${pipeline.name}`}
                      title="Delete pipeline"
                      style={{
                        background: 'none',
                        border: 'none',
                        cursor: 'pointer',
                        fontSize: '14px',
                        color: '#94A3B8',
                        padding: '4px',
                        flexShrink: 0,
                      }}
                      onMouseEnter={e => { (e.currentTarget as HTMLElement).style.color = '#DC2626' }}
                      onMouseLeave={e => { (e.currentTarget as HTMLElement).style.color = '#94A3B8' }}
                    >
                      ×
                    </button>
                  </>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

interface CanvasToolbarProps {
  pipelineName: string
  hasUnsavedChanges: boolean
  isSaving: boolean
  canRun: boolean
  onNameChange: (name: string) => void
  onSave: () => void
  onRun: () => void
  onClear: () => void
}

/**
 * CanvasToolbar renders the top toolbar of the canvas area.
 *
 * Elements:
 *   - Pipeline name input (shows asterisk when hasUnsavedChanges)
 *   - Save button (inline spinner when isSaving, checkmark on success)
 *   - Run button (opens SubmitTaskModal with current pipeline; disabled when no pipelineId)
 *   - Clear button (resets canvas to empty state; shows confirmation if hasUnsavedChanges)
 *
 * Preconditions:
 *   - onSave, onRun, onClear, onNameChange are stable callbacks.
 *
 * Postconditions:
 *   - Save button is disabled when isSaving === true.
 *   - Run button is disabled when the pipeline has not been saved yet (canRun === false).
 */
function CanvasToolbar({
  pipelineName,
  hasUnsavedChanges,
  isSaving,
  canRun,
  onNameChange,
  onSave,
  onRun,
  onClear,
}: CanvasToolbarProps): React.ReactElement {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        padding: '10px 16px',
        backgroundColor: 'var(--color-surface-panel)',
        border: '1px solid var(--color-border)',
        borderRadius: '8px',
        marginBottom: '12px',
      }}
    >
      {/* Pipeline name input */}
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: '4px' }}>
        <input
          type="text"
          value={pipelineName}
          onChange={e => onNameChange(e.target.value)}
          placeholder="Pipeline name"
          aria-label="Pipeline name"
          disabled={isSaving}
          style={{
            flex: 1,
            padding: '6px 10px',
            fontSize: '14px',
            border: '1px solid var(--color-border)',
            borderRadius: '6px',
            backgroundColor: 'var(--color-surface-panel)',
            color: 'var(--color-text-primary)',
          }}
        />
        {hasUnsavedChanges && (
          <span
            title="Unsaved changes"
            style={{ color: '#D97706', fontSize: '16px', fontWeight: 600 }}
          >
            *
          </span>
        )}
      </div>

      {/* Save button */}
      <button
        onClick={onSave}
        disabled={isSaving}
        aria-label="Save pipeline"
        style={{
          padding: '6px 14px',
          border: 'none',
          borderRadius: '6px',
          backgroundColor: isSaving ? '#94A3B8' : '#4F46E5',
          color: '#FFFFFF',
          cursor: isSaving ? 'not-allowed' : 'pointer',
          fontSize: '13px',
          fontWeight: 500,
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          minWidth: '72px',
          justifyContent: 'center',
        }}
      >
        {isSaving ? (
          <>
            <span
              style={{
                display: 'inline-block',
                width: '10px',
                height: '10px',
                border: '2px solid rgba(255,255,255,0.3)',
                borderTopColor: '#FFFFFF',
                borderRadius: '50%',
                animation: 'spin 0.8s linear infinite',
              }}
            />
            Saving...
          </>
        ) : (
          'Save'
        )}
      </button>

      {/* Run button */}
      <button
        onClick={onRun}
        disabled={!canRun}
        aria-label="Run pipeline"
        title={canRun ? 'Submit a task using this pipeline' : 'Save the pipeline first to enable Run'}
        style={{
          padding: '6px 14px',
          border: '1px solid var(--color-border)',
          borderRadius: '6px',
          backgroundColor: 'var(--color-surface-panel)',
          color: canRun ? 'var(--color-text-primary)' : '#94A3B8',
          cursor: canRun ? 'pointer' : 'not-allowed',
          fontSize: '13px',
          fontWeight: 500,
          opacity: canRun ? 1 : 0.5,
        }}
      >
        Run
      </button>

      {/* Clear button */}
      <button
        onClick={onClear}
        aria-label="Clear pipeline canvas"
        title="Clear canvas"
        style={{
          padding: '6px 14px',
          border: '1px solid var(--color-border)',
          borderRadius: '6px',
          backgroundColor: 'var(--color-surface-panel)',
          color: 'var(--color-text-secondary)',
          cursor: 'pointer',
          fontSize: '13px',
        }}
      >
        Clear
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Toast notification (lightweight local implementation)
// ---------------------------------------------------------------------------

interface ToastState {
  message: string
  type: 'success' | 'error'
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

/**
 * PipelineManagerPage is the Pipeline Builder and pipeline management view.
 * Composes PipelineCanvas, ComponentPalette, CanvasToolbar, SchemaMappingEditor,
 * and SubmitTaskModal.
 *
 * Navigation guard:
 *   - When the user navigates away with unsaved changes, a browser confirmation
 *     dialog is shown ("You have unsaved changes. Leave?"). Implemented via
 *     the beforeunload event and react-router's useBlocker hook.
 *
 * Save flow:
 *   1. Click Save: client-side canvas completeness check (all 3 phases placed)
 *   2. POST /api/pipelines (new) or PUT /api/pipelines/{id} (edit)
 *   3. On success: toast "Pipeline saved", hasUnsavedChanges = false
 *   4. On 400 (validation errors): validationErrors set, shown on canvas mapping chips
 *
 * Edit flow (TASK-024):
 *   1. User clicks a pipeline in ComponentPalette
 *   2. GET /api/pipelines/{id} fetched
 *   3. Canvas populated from pipeline.dataSourceConfig, processConfig, sinkConfig
 *   4. pipelineId and name set in editor state
 *
 * Preconditions:
 *   - User must be authenticated.
 *
 * Postconditions:
 *   - Saved pipeline is available via GET /api/pipelines.
 *   - Deleted pipeline no longer appears in the saved pipelines list.
 *   - After successful save, Run button becomes enabled.
 */
function PipelineManagerPage(): React.ReactElement {
  const { user } = useAuth()
  const { pipelines, isLoading: isPipelinesLoading, refresh: refreshPipelines } = usePipelines()

  const [editor, setEditor] = useState<PipelineEditorState>(INITIAL_EDITOR)
  const [toast, setToast] = useState<ToastState | null>(null)
  const [isRunModalOpen, setIsRunModalOpen] = useState(false)
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  /**
   * showToast displays a transient notification that auto-dismisses after 5 seconds.
   */
  function showToast(message: string, type: 'success' | 'error'): void {
    setToast({ message, type })
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current)
    toastTimerRef.current = setTimeout(() => setToast(null), 5000)
  }

  // Navigation guard: block react-router navigation when there are unsaved changes.
  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      editor.hasUnsavedChanges && currentLocation.pathname !== nextLocation.pathname
  )

  // Handle blocker state changes.
  useEffect(() => {
    if (blocker.state === 'blocked') {
      const confirmed = window.confirm('You have unsaved changes. Leave this page?')
      if (confirmed) {
        blocker.proceed()
      } else {
        blocker.reset()
      }
    }
  }, [blocker])

  // beforeunload guard for browser-level navigation (refresh, tab close).
  useEffect(() => {
    function handleBeforeUnload(e: BeforeUnloadEvent): void {
      if (editor.hasUnsavedChanges) {
        e.preventDefault()
        e.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [editor.hasUnsavedChanges])

  // Clean up toast timer on unmount.
  useEffect(() => {
    return () => {
      if (toastTimerRef.current) clearTimeout(toastTimerRef.current)
    }
  }, [])

  /**
   * handleCanvasChange updates the canvas state and marks unsaved changes.
   */
  const handleCanvasChange = useCallback((canvas: PipelineCanvasState) => {
    setEditor(prev => ({ ...prev, canvas, hasUnsavedChanges: true, validationErrors: [] }))
  }, [])

  /**
   * handleNameChange updates the pipeline name and marks unsaved changes.
   */
  const handleNameChange = useCallback((name: string) => {
    setEditor(prev => ({ ...prev, name, hasUnsavedChanges: true }))
  }, [])

  /**
   * handleSave performs client-side completeness check, then calls POST or PUT.
   * On 400 (schema validation failure), parses the error and populates validationErrors.
   */
  const handleSave = useCallback(async () => {
    if (editor.isSaving) return

    // Completeness check: all three phases must be placed.
    if (!editor.canvas.dataSource || !editor.canvas.process || !editor.canvas.sink) {
      showToast('Pipeline must have a DataSource, Process, and Sink before saving.', 'error')
      return
    }

    if (!editor.name.trim()) {
      showToast('Pipeline name is required.', 'error')
      return
    }

    setEditor(prev => ({ ...prev, isSaving: true, validationErrors: [] }))

    const payload = {
      name: editor.name.trim(),
      dataSourceConfig: editor.canvas.dataSource,
      processConfig: {
        ...editor.canvas.process,
        inputMappings: editor.canvas.dataSourceToProcessMappings,
      },
      sinkConfig: {
        ...editor.canvas.sink,
        inputMappings: editor.canvas.processToSinkMappings,
      },
    }

    try {
      let saved: Pipeline
      if (editor.pipelineId) {
        saved = await updatePipeline(editor.pipelineId, payload)
      } else {
        saved = await createPipeline(payload)
      }

      setEditor(prev => ({
        ...prev,
        pipelineId: saved.id,
        isSaving: false,
        hasUnsavedChanges: false,
        validationErrors: [],
      }))
      showToast('Pipeline saved.', 'success')
      refreshPipelines()
    } catch (err: unknown) {
      setEditor(prev => ({ ...prev, isSaving: false }))

      const errMsg = err instanceof Error ? err.message : 'Save failed'

      // Surface 400 validation errors as mapping chip annotations.
      if (errMsg.startsWith('400:')) {
        const detail = errMsg.slice(4).trim()
        // Parse the error message from the backend (TASK-026 format):
        // "process input mapping: source field 'X' not found in datasource output schema"
        // "sink input mapping: source field 'X' not found in process output schema"
        const errors = parseValidationErrors(detail)
        if (errors.length > 0) {
          setEditor(prev => ({ ...prev, validationErrors: errors }))
          showToast('Schema mapping validation failed. Check highlighted mappings.', 'error')
        } else {
          showToast(detail || 'Validation error. Check your pipeline configuration.', 'error')
        }
      } else {
        showToast(errMsg, 'error')
      }
    }
  }, [editor, refreshPipelines])

  /**
   * parseValidationErrors converts a backend 400 error message into
   * MappingValidationError objects for the canvas chips.
   *
   * Backend format (from TASK-026):
   *   "process input mapping: source field '<field>' not found in datasource output schema"
   *   "sink input mapping: source field '<field>' not found in process output schema"
   */
  function parseValidationErrors(message: string): MappingValidationError[] {
    const errors: MappingValidationError[] = []

    const processMatch = message.match(/process input mapping: source field '([^']+)' not found/)
    if (processMatch) {
      errors.push({
        boundary: 'dataSourceToProcess',
        sourceField: processMatch[1],
        message,
      })
    }

    const sinkMatch = message.match(/sink input mapping: source field '([^']+)' not found/)
    if (sinkMatch) {
      errors.push({
        boundary: 'processToSink',
        sourceField: sinkMatch[1],
        message,
      })
    }

    return errors
  }

  /**
   * handleLoadPipeline fetches a saved pipeline by ID and populates the canvas.
   */
  const handleLoadPipeline = useCallback(async (pipelineId: string) => {
    // Warn if there are unsaved changes before loading.
    if (editor.hasUnsavedChanges) {
      const confirmed = window.confirm('You have unsaved changes. Load another pipeline?')
      if (!confirmed) return
    }

    try {
      const pipeline = await getPipeline(pipelineId)

      const canvas: PipelineCanvasState = {
        dataSource: pipeline.dataSourceConfig,
        process: pipeline.processConfig,
        sink: pipeline.sinkConfig,
        dataSourceToProcessMappings: pipeline.processConfig.inputMappings,
        processToSinkMappings: pipeline.sinkConfig.inputMappings,
      }

      setEditor({
        pipelineId: pipeline.id,
        name: pipeline.name,
        canvas,
        hasUnsavedChanges: false,
        isSaving: false,
        validationErrors: [],
      })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load pipeline'
      showToast(msg, 'error')
    }
  }, [editor.hasUnsavedChanges])

  /**
   * handleDeletePipeline calls DELETE /api/pipelines/{id} and refreshes the list.
   * Shows a toast on 409 (active tasks) and on success.
   */
  const handleDeletePipeline = useCallback(async (pipelineId: string) => {
    try {
      await deletePipeline(pipelineId)

      // If the deleted pipeline is currently loaded, clear the canvas.
      if (editor.pipelineId === pipelineId) {
        setEditor(INITIAL_EDITOR)
      }
      showToast('Pipeline deleted.', 'success')
      refreshPipelines()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Delete failed'
      if (msg.startsWith('409:')) {
        showToast('Cannot delete pipeline: it has active tasks.', 'error')
      } else {
        showToast(msg, 'error')
      }
    }
  }, [editor.pipelineId, refreshPipelines])

  /**
   * handleClear resets the canvas to empty state.
   * Asks for confirmation when there are unsaved changes.
   */
  const handleClear = useCallback(() => {
    if (editor.hasUnsavedChanges) {
      const confirmed = window.confirm('Clear canvas? Unsaved changes will be lost.')
      if (!confirmed) return
    }
    setEditor(INITIAL_EDITOR)
  }, [editor.hasUnsavedChanges])

  /**
   * handleRun opens the SubmitTaskModal with the current pipeline pre-selected.
   * Only available when the pipeline has been saved (pipelineId is set).
   */
  const handleRun = useCallback(() => {
    if (!editor.pipelineId) return
    setIsRunModalOpen(true)
  }, [editor.pipelineId])

  const [canvasRejectionMsg, setCanvasRejectionMsg] = useState<string | null>(null)
  const rejectionTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  /**
   * handlePageDragEnd is the DndContext drag-end handler at the page level.
   * It connects ComponentPalette drag sources with the PipelineCanvas drop target.
   * Calls applyPhaseDropToState for linearity enforcement and updates editor state.
   */
  const handlePageDragEnd = useCallback((event: DragEndEvent) => {
    if (!event.over || event.over.id !== CANVAS_DROP_ID) return

    const phase = event.active.data.current?.phase as PipelinePhase | undefined
    if (!phase) return

    const result = applyPhaseDropToState(editor.canvas, phase)
    if (typeof result === 'string') {
      setCanvasRejectionMsg(result)
      if (rejectionTimerRef.current) clearTimeout(rejectionTimerRef.current)
      rejectionTimerRef.current = setTimeout(() => setCanvasRejectionMsg(null), 3000)
    } else {
      setEditor(prev => ({
        ...prev,
        canvas: result,
        hasUnsavedChanges: true,
        validationErrors: [],
      }))
    }
  }, [editor.canvas])

  // Clean up rejection timer on unmount.
  useEffect(() => {
    return () => {
      if (rejectionTimerRef.current) clearTimeout(rejectionTimerRef.current)
    }
  }, [])

  // PointerSensor for dnd-kit (required for jsdom compatibility in tests)
  const sensors = useSensors(useSensor(PointerSensor))

  const isAdmin = user?.role === 'admin'
  const savedPipelineListItems = pipelines.map(p => ({ id: p.id, name: p.name }))
  const canRun = Boolean(editor.pipelineId)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '0', height: '100%' }}>
      {/* Page header */}
      <h1
        style={{
          fontSize: '20px',
          fontWeight: 600,
          color: 'var(--color-text-primary)',
          margin: '0 0 16px 0',
        }}
      >
        Pipeline Builder
      </h1>

      {/* Main layout: palette left, canvas right — all within one DndContext */}
      <DndContext sensors={sensors} onDragEnd={handlePageDragEnd}>
        <div style={{ display: 'flex', gap: '16px', flex: 1, minHeight: 0 }}>
          {/* Component Palette (left panel) — drag sources live here */}
          <ComponentPalette
            savedPipelines={savedPipelineListItems}
            isAdmin={isAdmin ?? false}
            onLoadPipeline={handleLoadPipeline}
            onDeletePipeline={handleDeletePipeline}
            isLoadingPipelines={isPipelinesLoading}
            isDataSourcePlaced={editor.canvas.dataSource !== null}
            isProcessPlaced={editor.canvas.process !== null}
            isSinkPlaced={editor.canvas.sink !== null}
            readOnly={editor.isSaving}
          />

          {/* Canvas area: toolbar + canvas */}
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
            <CanvasToolbar
              pipelineName={editor.name}
              hasUnsavedChanges={editor.hasUnsavedChanges}
              isSaving={editor.isSaving}
              canRun={canRun}
              onNameChange={handleNameChange}
              onSave={handleSave}
              onRun={handleRun}
              onClear={handleClear}
            />

            {/* Pipeline canvas — drop target lives here; no inner DndContext */}
            <div style={{ flex: 1, position: 'relative' }}>
              <PipelineCanvas
                value={editor.canvas}
                onChange={handleCanvasChange}
                validationErrors={editor.validationErrors}
                readOnly={editor.isSaving}
                standalone={false}
              />
              {/* Rejection toast from page-level drag handling */}
              {canvasRejectionMsg && (
                <div
                  role="alert"
                  style={{
                    position: 'absolute',
                    bottom: '24px',
                    left: '50%',
                    transform: 'translateX(-50%)',
                    backgroundColor: '#1E293B',
                    color: '#F8FAFC',
                    padding: '8px 16px',
                    borderRadius: '6px',
                    fontSize: '13px',
                    maxWidth: '400px',
                    textAlign: 'center',
                    boxShadow: '0 4px 12px rgba(0,0,0,0.2)',
                    zIndex: 10,
                  }}
                >
                  {canvasRejectionMsg}
                </div>
              )}
            </div>
          </div>
        </div>
      </DndContext>

      {/* Toast notification */}
      {toast && (
        <div
          role="alert"
          aria-live="assertive"
          style={{
            position: 'fixed',
            bottom: '24px',
            right: '24px',
            zIndex: 2000,
            backgroundColor: toast.type === 'success' ? '#16A34A' : '#DC2626',
            color: '#FFFFFF',
            padding: '12px 20px',
            borderRadius: '8px',
            fontSize: '14px',
            fontWeight: 500,
            boxShadow: '0 4px 16px rgba(0,0,0,0.2)',
            maxWidth: '360px',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <span>{toast.type === 'success' ? '✓' : '✕'}</span>
          <span>{toast.message}</span>
          <button
            onClick={() => setToast(null)}
            aria-label="Dismiss notification"
            style={{
              background: 'none',
              border: 'none',
              color: 'rgba(255,255,255,0.8)',
              cursor: 'pointer',
              fontSize: '16px',
              marginLeft: '8px',
              padding: '0',
            }}
          >
            ×
          </button>
        </div>
      )}

      {/* Submit task modal (Run button) */}
      <SubmitTaskModal
        isOpen={isRunModalOpen}
        onClose={() => setIsRunModalOpen(false)}
        onSuccess={(taskId) => {
          setIsRunModalOpen(false)
          showToast(`Task submitted successfully. Task ID: ${taskId}`, 'success')
        }}
        pipelines={pipelines}
        initialPipelineId={editor.pipelineId ?? undefined}
      />
    </div>
  )
}

export default PipelineManagerPage
