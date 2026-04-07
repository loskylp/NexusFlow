/**
 * PipelineCanvas — drag-and-drop canvas for the Pipeline Builder (TASK-023).
 *
 * The canvas uses dnd-kit (@dnd-kit/core, @dnd-kit/sortable) for drag-and-drop
 * interactions. The component palette (left) contains draggable DataSource,
 * Process, and Sink cards. The canvas (right) accepts dropped components and
 * renders the linear pipeline: DataSource → [mapping chip] → Process → [mapping chip] → Sink.
 *
 * IMPORTANT: dnd-kit is required as a new dependency.
 * Add to package.json: "@dnd-kit/core": "^6.x", "@dnd-kit/utilities": "^3.x"
 * Install: npm --prefix web install @dnd-kit/core @dnd-kit/utilities
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

import React from 'react'
import type { DataSourceConfig, ProcessConfig, SinkConfig, SchemaMapping } from '@/types/domain'

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
}

// ---------------------------------------------------------------------------
// Sub-types for internal nodes
// ---------------------------------------------------------------------------

/** Phase names used internally for drag-and-drop identity and node labeling. */
export type PipelinePhase = 'DataSource' | 'Process' | 'Sink'

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * PipelineCanvas renders the visual drag-and-drop pipeline builder canvas.
 *
 * Internal structure:
 *   - ComponentPalette (left 200px): draggable phase cards and saved pipelines list
 *   - CanvasArea (remaining width): dot-grid, pipeline phase nodes, connector lines,
 *     schema mapping chips between phases
 *   - SchemaMappingEditor modal: opened when a mapping chip is clicked
 *
 * States handled:
 *   - Empty: centered placeholder "Drag components from the palette to build a pipeline"
 *   - Partial (1-2 phases placed): shows placed nodes with empty slots for remaining phases
 *   - Complete (all 3 phases): shows full pipeline with mapping chips
 *   - Validation error: mapping chip with error shows red border and tooltip
 *   - Read-only: all drag interactions disabled; schema mapping editor is not openable
 *
 * Preconditions:
 *   - onChange is a stable callback (wrapped in useCallback in parent).
 *   - value is the single source of truth for canvas state.
 *
 * Postconditions:
 *   - Dropping a duplicate phase does not call onChange; a tooltip rejection is shown.
 *   - Removing a phase clears its downstream mapping arrays (e.g., removing Process
 *     clears both mapping arrays).
 *   - Dropping a phase on an empty canvas positions it in the correct linear slot.
 */
function PipelineCanvas({
  value,
  onChange,
  validationErrors = [],
  readOnly = false,
}: PipelineCanvasProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

export default PipelineCanvas
