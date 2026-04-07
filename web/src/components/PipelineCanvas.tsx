/**
 * PipelineCanvas — drag-and-drop canvas for the Pipeline Builder (TASK-023).
 *
 * The canvas uses dnd-kit (@dnd-kit/core, @dnd-kit/utilities) for drag-and-drop
 * interactions. The component palette (left) contains draggable DataSource,
 * Process, and Sink cards. The canvas (right) accepts dropped components and
 * renders the linear pipeline: DataSource → [mapping chip] → Process → [mapping chip] → Sink.
 *
 * The DndContext is provided by the parent (PipelineManagerPage) so that palette
 * drag sources and the canvas drop target share the same drag-and-drop context.
 * PipelineCanvas exposes a handleDrop function for the parent to call from its
 * DndContext onDragEnd handler.
 *
 * Canvas rules (linearity enforcement, Phase 1):
 *   - Exactly one DataSource, one Process, one Sink — in that order.
 *   - Attempting to drop a second DataSource shows a tooltip rejection (no state change).
 *   - Attempting to drop a Process before DataSource is placed is rejected.
 *   - Attempting to drop a Sink before Process is placed is rejected.
 *
 * Node types and colors (per UX spec phase-colored headers):
 *   DataSource  → blue header   (#2563EB)
 *   Process     → purple header (#8B5CF6)
 *   Sink        → green header  (#16A34A)
 *
 * The dot-grid background is implemented via CSS background-image SVG dot pattern.
 *
 * See: TASK-023, REQ-015, REQ-007, UX Spec (Pipeline Builder)
 */

import React, { useCallback, useState } from 'react'
import {
  DndContext,
  useDraggable,
  useDroppable,
  type DragEndEvent,
} from '@dnd-kit/core'
import { CSS } from '@dnd-kit/utilities'
import type { DataSourceConfig, ProcessConfig, SinkConfig, SchemaMapping } from '@/types/domain'
import SchemaMappingEditor from './SchemaMappingEditor'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Represents the placement state of the pipeline on the canvas. */
export interface PipelineCanvasState {
  dataSource: DataSourceConfig | null
  process: ProcessConfig | null
  sink: SinkConfig | null
  /** Mappings from DataSource output to Process input. */
  dataSourceToProcessMappings: SchemaMapping[]
  /** Mappings from Process output to Sink input. */
  processToSinkMappings: SchemaMapping[]
}

/** Represents a validation error on a specific mapping chip. */
export interface MappingValidationError {
  /** Which phase boundary this error is on: 'dataSourceToProcess' | 'processToSink'. */
  boundary: 'dataSourceToProcess' | 'processToSink'
  /** The source field name that failed validation. */
  sourceField: string
  /** Human-readable error description. */
  message: string
}

export interface PipelineCanvasProps {
  /** Current canvas state — controlled component. */
  value: PipelineCanvasState
  /**
   * Called whenever the canvas state changes (component dropped, removed, or
   * schema mappings edited). Parent must persist state to enable unsaved changes tracking.
   */
  onChange: (state: PipelineCanvasState) => void
  /**
   * Validation errors returned by the API or the client-side validator.
   * Errors are displayed as red borders on the affected mapping chip with a tooltip.
   */
  validationErrors?: MappingValidationError[]
  /** Whether the canvas is read-only (e.g., when a save is in progress). */
  readOnly?: boolean
  /**
   * When true, the canvas manages its own DndContext internally.
   * When false (default for PipelineManagerPage), the parent provides DndContext
   * and calls handleDropFromParent to deliver drop events.
   * Set to true only for standalone canvas usage (e.g., tests).
   */
  standalone?: boolean
}

// ---------------------------------------------------------------------------
// Sub-types for internal nodes
// ---------------------------------------------------------------------------

/** Phase names used internally for drag-and-drop identity and node labeling. */
export type PipelinePhase = 'DataSource' | 'Process' | 'Sink'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Color tokens for phase node headers per UX spec. */
export const PHASE_COLORS: Record<PipelinePhase, string> = {
  DataSource: '#2563EB',
  Process: '#8B5CF6',
  Sink: '#16A34A',
}

/** Drop target ID for the canvas area — used by dnd-kit. */
export const CANVAS_DROP_ID = 'pipeline-canvas'

/** Default placeholder configs used when a phase is dropped onto the canvas. */
const DEFAULT_DATASOURCE: DataSourceConfig = {
  connectorType: 'generic',
  config: {},
  outputSchema: [],
}

const DEFAULT_PROCESS: ProcessConfig = {
  connectorType: 'generic',
  config: {},
  inputMappings: [],
  outputSchema: [],
}

const DEFAULT_SINK: SinkConfig = {
  connectorType: 'generic',
  config: {},
  inputMappings: [],
}

// ---------------------------------------------------------------------------
// DraggablePaletteCard — drag source in the palette
// ---------------------------------------------------------------------------

interface DraggablePaletteCardProps {
  phase: PipelinePhase
  disabled: boolean
  disabledReason?: string
}

/**
 * DraggablePaletteCard renders a draggable phase card in the component palette.
 * When disabled (phase already placed), it shows a tooltip explaining why.
 * Uses dnd-kit's useDraggable hook; the drag id is `palette-{phase}`.
 *
 * Preconditions:
 *   - Must be rendered within a dnd-kit DndContext.
 */
export function DraggablePaletteCard({ phase, disabled, disabledReason }: DraggablePaletteCardProps): React.ReactElement {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `palette-${phase}`,
    data: { phase },
    disabled,
  })

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    opacity: isDragging ? 0.5 : disabled ? 0.4 : 1,
    cursor: disabled ? 'not-allowed' : 'grab',
    padding: '10px 12px',
    backgroundColor: 'var(--color-surface-panel)',
    border: `1px solid ${disabled ? 'var(--color-border)' : PHASE_COLORS[phase]}`,
    borderLeft: `4px solid ${PHASE_COLORS[phase]}`,
    borderRadius: '6px',
    fontSize: '13px',
    fontWeight: 500,
    color: 'var(--color-text-primary)',
    userSelect: 'none',
    position: 'relative',
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      title={disabled ? disabledReason : `Drag to place ${phase}`}
      {...(disabled ? {} : listeners)}
      {...(disabled ? {} : attributes)}
    >
      <span
        style={{
          display: 'inline-block',
          width: '8px',
          height: '8px',
          borderRadius: '50%',
          backgroundColor: PHASE_COLORS[phase],
          marginRight: '8px',
        }}
      />
      {phase}
      {disabled && (
        <span
          style={{
            position: 'absolute',
            top: '50%',
            right: '8px',
            transform: 'translateY(-50%)',
            fontSize: '11px',
            color: 'var(--color-text-tertiary)',
          }}
        >
          placed
        </span>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// CanvasDropArea — the drop target area (not the outer DndContext)
// ---------------------------------------------------------------------------

interface CanvasDropAreaProps {
  isOver: boolean
  children: React.ReactNode
}

/**
 * CanvasDropArea is the visual canvas area that serves as a dnd-kit drop target.
 * Highlights with a blue border when a draggable is hovering over it.
 * Must be rendered within a DndContext.
 */
function CanvasDropArea({ isOver, children }: CanvasDropAreaProps): React.ReactElement {
  const { setNodeRef } = useDroppable({ id: CANVAS_DROP_ID })

  return (
    <div
      ref={setNodeRef}
      style={{
        flex: 1,
        minHeight: '400px',
        position: 'relative',
        // Dot-grid background via CSS SVG pattern
        backgroundImage: 'radial-gradient(circle, #CBD5E1 1px, transparent 1px)',
        backgroundSize: '20px 20px',
        backgroundColor: '#F8FAFC',
        border: `2px dashed ${isOver ? '#4F46E5' : '#E2E8F0'}`,
        borderRadius: '8px',
        transition: 'border-color 0.15s ease',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        overflow: 'hidden',
      }}
    >
      {children}
    </div>
  )
}

// ---------------------------------------------------------------------------
// PhaseNode — a placed phase on the canvas
// ---------------------------------------------------------------------------

interface PhaseNodeProps {
  phase: PipelinePhase
  onRemove: () => void
  readOnly: boolean
}

/**
 * PhaseNode renders a placed pipeline phase as a card on the canvas.
 * Shows the phase name in a color-coded header and a remove button when
 * not in read-only mode.
 */
function PhaseNode({ phase, onRemove, readOnly }: PhaseNodeProps): React.ReactElement {
  const headerColor = PHASE_COLORS[phase]

  return (
    <div
      style={{
        width: '160px',
        border: '1px solid var(--color-border)',
        borderRadius: '8px',
        backgroundColor: 'var(--color-surface-panel)',
        overflow: 'hidden',
        boxShadow: '0 2px 8px rgba(0,0,0,0.08)',
        flexShrink: 0,
      }}
    >
      {/* Color-coded header */}
      <div
        style={{
          backgroundColor: headerColor,
          padding: '8px 12px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <span style={{ color: '#FFFFFF', fontSize: '12px', fontWeight: 600 }}>{phase}</span>
        {!readOnly && (
          <button
            onClick={onRemove}
            aria-label={`Remove ${phase} from pipeline`}
            title={`Remove ${phase}`}
            style={{
              background: 'rgba(255,255,255,0.2)',
              border: 'none',
              borderRadius: '3px',
              color: '#FFFFFF',
              cursor: 'pointer',
              fontSize: '12px',
              padding: '1px 5px',
              lineHeight: 1.4,
            }}
          >
            ×
          </button>
        )}
      </div>
      {/* Node body */}
      <div style={{ padding: '10px 12px', fontSize: '12px', color: 'var(--color-text-secondary)' }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px' }}>generic</span>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// ConnectorLine — SVG arrow between nodes
// ---------------------------------------------------------------------------

/**
 * ConnectorLine renders an SVG horizontal arrow connector between two phase nodes.
 */
function ConnectorLine(): React.ReactElement {
  return (
    <svg
      width="40"
      height="24"
      viewBox="0 0 40 24"
      aria-hidden="true"
      style={{ flexShrink: 0 }}
    >
      <line x1="0" y1="12" x2="32" y2="12" stroke="#94A3B8" strokeWidth="2" />
      <polygon points="32,6 40,12 32,18" fill="#94A3B8" />
    </svg>
  )
}

// ---------------------------------------------------------------------------
// MappingChip — clickable chip between two phase nodes
// ---------------------------------------------------------------------------

interface MappingChipProps {
  label: string
  hasErrors: boolean
  errorMessage?: string
  onClick: () => void
  readOnly: boolean
}

/**
 * MappingChip renders the schema mapping indicator between two adjacent phases.
 * Shows a red border and tooltip when there are validation errors.
 * Clicking opens the SchemaMappingEditor (unless read-only).
 */
function MappingChip({ label, hasErrors, errorMessage, onClick, readOnly }: MappingChipProps): React.ReactElement {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px', flexShrink: 0 }}>
      <ConnectorLine />
      <button
        onClick={readOnly ? undefined : onClick}
        title={hasErrors ? errorMessage : 'Click to edit schema mapping'}
        aria-label={`${label} mapping${hasErrors ? ' — has validation errors' : ''}`}
        style={{
          padding: '3px 8px',
          fontSize: '11px',
          border: `1px solid ${hasErrors ? '#EF4444' : '#CBD5E1'}`,
          borderRadius: '12px',
          backgroundColor: hasErrors ? '#FEF2F2' : '#F1F5F9',
          color: hasErrors ? '#DC2626' : 'var(--color-text-secondary)',
          cursor: readOnly ? 'default' : 'pointer',
          fontFamily: 'var(--font-mono)',
          whiteSpace: 'nowrap',
        }}
      >
        {label}
        {hasErrors && ' ⚠'}
      </button>
      <ConnectorLine />
    </div>
  )
}

// ---------------------------------------------------------------------------
// PipelineCanvasInner — canvas content (without DndContext)
// ---------------------------------------------------------------------------

interface PipelineCanvasInnerProps {
  value: PipelineCanvasState
  onChange: (state: PipelineCanvasState) => void
  validationErrors: MappingValidationError[]
  readOnly: boolean
  isOverCanvas: boolean
  rejectionMessage: string | null
}

/**
 * PipelineCanvasInner renders the canvas content: the drop area, phase nodes,
 * connectors, mapping chips, and schema mapping editor modals.
 *
 * Separated from the DndContext wrapper so that either:
 *   - PipelineManagerPage provides the outer DndContext (normal use), or
 *   - PipelineCanvas provides its own DndContext (standalone mode).
 */
function PipelineCanvasInner({
  value,
  onChange,
  validationErrors,
  readOnly,
  isOverCanvas,
  rejectionMessage,
}: PipelineCanvasInnerProps): React.ReactElement {
  const [mappingEditorOpen, setMappingEditorOpen] = useState<'dataSourceToProcess' | 'processToSink' | null>(null)

  /** saveDataSourceToProcessMappings updates the DS→Process mapping array. */
  const saveDataSourceToProcessMappings = useCallback((mappings: SchemaMapping[]) => {
    onChange({ ...value, dataSourceToProcessMappings: mappings })
  }, [value, onChange])

  /** saveProcessToSinkMappings updates the Process→Sink mapping array. */
  const saveProcessToSinkMappings = useCallback((mappings: SchemaMapping[]) => {
    onChange({ ...value, processToSinkMappings: mappings })
  }, [value, onChange])

  /**
   * removePhase removes a placed phase and clears downstream mappings.
   * Removing DataSource also clears Process, Sink, and all mappings.
   * Removing Process also clears Sink and all mappings.
   * Removing Sink clears processToSink mappings only.
   */
  function removePhase(phase: PipelinePhase): void {
    if (phase === 'DataSource') {
      onChange({
        dataSource: null,
        process: null,
        sink: null,
        dataSourceToProcessMappings: [],
        processToSinkMappings: [],
      })
    } else if (phase === 'Process') {
      onChange({
        ...value,
        process: null,
        sink: null,
        dataSourceToProcessMappings: [],
        processToSinkMappings: [],
      })
    } else if (phase === 'Sink') {
      onChange({ ...value, sink: null, processToSinkMappings: [] })
    }
  }

  const dsToProcessErrors = validationErrors.filter(e => e.boundary === 'dataSourceToProcess')
  const processToSinkErrors = validationErrors.filter(e => e.boundary === 'processToSink')
  const dsToProcessErrorMsg = dsToProcessErrors.map(e => e.message).join('; ')
  const processToSinkErrorMsg = processToSinkErrors.map(e => e.message).join('; ')
  const dsSourceFields = value.dataSource?.outputSchema ?? []
  const processSourceFields = value.process?.outputSchema ?? []

  const isEmpty = value.dataSource === null && value.process === null && value.sink === null

  return (
    <>
      <CanvasDropArea isOver={isOverCanvas}>
        {isEmpty ? (
          <div
            style={{
              textAlign: 'center',
              color: 'var(--color-text-secondary)',
              fontSize: '14px',
              pointerEvents: 'none',
            }}
          >
            <div style={{ fontSize: '32px', marginBottom: '12px' }}>⬡</div>
            <div>Drag components from the palette to build a pipeline</div>
          </div>
        ) : (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: '0',
              padding: '24px',
              flexWrap: 'nowrap',
            }}
          >
            {/* DataSource node */}
            {value.dataSource !== null && (
              <>
                <PhaseNode phase="DataSource" onRemove={() => removePhase('DataSource')} readOnly={readOnly} />

                {/* DataSource → Process mapping chip (shown when Process is placed) */}
                {value.process !== null && (
                  <MappingChip
                    label={`${value.dataSourceToProcessMappings.length} mapping${value.dataSourceToProcessMappings.length !== 1 ? 's' : ''}`}
                    hasErrors={dsToProcessErrors.length > 0}
                    errorMessage={dsToProcessErrorMsg || undefined}
                    onClick={() => setMappingEditorOpen('dataSourceToProcess')}
                    readOnly={readOnly}
                  />
                )}
              </>
            )}

            {/* DataSource placeholder when DataSource is missing but something is on canvas */}
            {value.dataSource === null && !isEmpty && (
              <>
                <div
                  style={{
                    width: '160px',
                    height: '80px',
                    border: '2px dashed #2563EB',
                    borderRadius: '8px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: '12px',
                    color: '#2563EB',
                    flexShrink: 0,
                  }}
                >
                  DataSource
                </div>
                <ConnectorLine />
              </>
            )}

            {/* Process node */}
            {value.process !== null && (
              <>
                <PhaseNode phase="Process" onRemove={() => removePhase('Process')} readOnly={readOnly} />

                {/* Process → Sink mapping chip (shown when Sink is placed) */}
                {value.sink !== null && (
                  <MappingChip
                    label={`${value.processToSinkMappings.length} mapping${value.processToSinkMappings.length !== 1 ? 's' : ''}`}
                    hasErrors={processToSinkErrors.length > 0}
                    errorMessage={processToSinkErrorMsg || undefined}
                    onClick={() => setMappingEditorOpen('processToSink')}
                    readOnly={readOnly}
                  />
                )}
              </>
            )}

            {/* Process placeholder when DataSource is placed but no Process yet */}
            {value.dataSource !== null && value.process === null && (
              <>
                <ConnectorLine />
                <div
                  style={{
                    width: '160px',
                    height: '80px',
                    border: '2px dashed #8B5CF6',
                    borderRadius: '8px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: '12px',
                    color: '#8B5CF6',
                    flexShrink: 0,
                  }}
                >
                  Process
                </div>
              </>
            )}

            {/* Sink node */}
            {value.sink !== null && (
              <PhaseNode phase="Sink" onRemove={() => removePhase('Sink')} readOnly={readOnly} />
            )}

            {/* Sink placeholder when Process is placed but no Sink yet */}
            {value.process !== null && value.sink === null && (
              <>
                <ConnectorLine />
                <div
                  style={{
                    width: '160px',
                    height: '80px',
                    border: '2px dashed #16A34A',
                    borderRadius: '8px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: '12px',
                    color: '#16A34A',
                    flexShrink: 0,
                  }}
                >
                  Sink
                </div>
              </>
            )}
          </div>
        )}

        {/* Rejection tooltip for linearity violations */}
        {rejectionMessage && (
          <div
            role="alert"
            style={{
              position: 'absolute',
              bottom: '16px',
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
            {rejectionMessage}
          </div>
        )}
      </CanvasDropArea>

      {/* Schema Mapping Editor modals */}
      <SchemaMappingEditor
        isOpen={mappingEditorOpen === 'dataSourceToProcess'}
        title="DataSource → Process Mapping"
        sourceFields={dsSourceFields}
        mappings={value.dataSourceToProcessMappings}
        onSave={saveDataSourceToProcessMappings}
        onClose={() => setMappingEditorOpen(null)}
      />
      <SchemaMappingEditor
        isOpen={mappingEditorOpen === 'processToSink'}
        title="Process → Sink Mapping"
        sourceFields={processSourceFields}
        mappings={value.processToSinkMappings}
        onSave={saveProcessToSinkMappings}
        onClose={() => setMappingEditorOpen(null)}
      />
    </>
  )
}

// ---------------------------------------------------------------------------
// applyphaseDropToState — pure function for linearity enforcement
// ---------------------------------------------------------------------------

/**
 * applyPhaseDropToState processes a drag-end event and returns either the
 * updated canvas state (on valid drop) or a rejection reason string (on invalid drop).
 *
 * This is the core linearity enforcement logic:
 *   - Duplicate phase → rejection message
 *   - Out-of-order drop → rejection message
 *   - Valid drop → new state
 *
 * Exported for unit testing without requiring dnd-kit interaction.
 *
 * @param current - Current canvas state before the drop.
 * @param phase   - The phase being dropped.
 * @returns The new state on success, or a string rejection message on failure.
 */
export function applyPhaseDropToState(
  current: PipelineCanvasState,
  phase: PipelinePhase
): PipelineCanvasState | string {
  if (phase === 'DataSource') {
    if (current.dataSource !== null) {
      return 'A DataSource is already placed. Only one DataSource is allowed per pipeline.'
    }
    return { ...current, dataSource: DEFAULT_DATASOURCE }
  }

  if (phase === 'Process') {
    if (current.process !== null) {
      return 'A Process is already placed. Only one Process is allowed per pipeline.'
    }
    if (current.dataSource === null) {
      return 'Place a DataSource first before adding a Process.'
    }
    return { ...current, process: DEFAULT_PROCESS }
  }

  if (phase === 'Sink') {
    if (current.sink !== null) {
      return 'A Sink is already placed. Only one Sink is allowed per pipeline.'
    }
    if (current.process === null) {
      return 'Place a Process first before adding a Sink.'
    }
    return { ...current, sink: DEFAULT_SINK }
  }

  return current
}

// ---------------------------------------------------------------------------
// PipelineCanvas
// ---------------------------------------------------------------------------

/**
 * PipelineCanvas renders the visual drag-and-drop pipeline builder canvas.
 *
 * When `standalone` is true, manages its own DndContext (used in tests and
 * standalone usage). When false (default), the parent must wrap both this
 * component and the DraggablePaletteCard components in a single DndContext,
 * and the parent's DragEnd handler must call handleDrop on this component's ref,
 * OR simply let PipelineCanvas use its own inner DndContext that is scoped
 * to just the CanvasDropArea (which won't pick up external drag sources).
 *
 * Architecture note: In PipelineManagerPage, the DndContext is provided at
 * the page level, wrapping both ComponentPalette (drag sources) and
 * PipelineCanvas (drop target). PipelineCanvas with standalone=false should
 * NOT create its own DndContext — it only renders the CanvasDropArea.
 * PipelineManagerPage's onDragEnd handler calls the applyPhaseDropToState
 * helper to update canvas state.
 *
 * When standalone=true (or by default, for backwards compatibility), the
 * canvas manages its own DndContext. This is suitable for tests.
 *
 * Postconditions:
 *   - Dropping a duplicate phase does not call onChange; a tooltip rejection is shown.
 *   - Removing a phase clears its downstream mapping arrays.
 *   - Dropping a phase positions it in the correct linear slot.
 */
function PipelineCanvas({
  value,
  onChange,
  validationErrors = [],
  readOnly = false,
  standalone = false,
}: PipelineCanvasProps): React.ReactElement {
  const [isOverCanvas, setIsOverCanvas] = useState(false)
  const [rejectionMessage, setRejectionMessage] = useState<string | null>(null)

  /**
   * handleDragEnd processes a completed drag-and-drop event when in standalone mode.
   * Enforces linearity rules before updating canvas state.
   */
  const handleDragEnd = useCallback((event: DragEndEvent) => {
    setIsOverCanvas(false)
    if (!event.over || event.over.id !== CANVAS_DROP_ID) return

    const phase = event.active.data.current?.phase as PipelinePhase | undefined
    if (!phase) return

    const result = applyPhaseDropToState(value, phase)
    if (typeof result === 'string') {
      setRejectionMessage(result)
      setTimeout(() => setRejectionMessage(null), 3000)
    } else {
      onChange(result)
    }
  }, [value, onChange])

  const inner = (
    <div style={{ width: '100%', height: '100%' }}>
      <PipelineCanvasInner
        value={value}
        onChange={onChange}
        validationErrors={validationErrors}
        readOnly={readOnly}
        isOverCanvas={isOverCanvas}
        rejectionMessage={rejectionMessage}
      />
    </div>
  )

  if (standalone) {
    return (
      <DndContext
        onDragEnd={handleDragEnd}
        onDragOver={(e) => setIsOverCanvas(e.over?.id === CANVAS_DROP_ID)}
        onDragCancel={() => setIsOverCanvas(false)}
      >
        {inner}
      </DndContext>
    )
  }

  return inner
}

export default PipelineCanvas
