/**
 * Unit tests for PipelineCanvas component and applyPhaseDropToState pure function.
 *
 * Rendering tests verify: empty state, partial states, full pipeline, validation
 * errors, read-only mode, and phase removal behavior.
 *
 * Linearity enforcement tests verify the applyPhaseDropToState pure function
 * directly, since dnd-kit drag interactions are difficult to simulate in jsdom.
 *
 * See: TASK-023, REQ-015, ADR-008
 */
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import PipelineCanvas, { applyPhaseDropToState } from './PipelineCanvas'
import type { PipelineCanvasProps, PipelineCanvasState, MappingValidationError } from './PipelineCanvas'

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

const EMPTY_STATE: PipelineCanvasState = {
  dataSource: null,
  process: null,
  sink: null,
  dataSourceToProcessMappings: [],
  processToSinkMappings: [],
}

const DS_ONLY: PipelineCanvasState = {
  dataSource: { connectorType: 'generic', config: {}, outputSchema: ['x'] },
  process: null,
  sink: null,
  dataSourceToProcessMappings: [],
  processToSinkMappings: [],
}

const DS_AND_PROCESS: PipelineCanvasState = {
  dataSource: { connectorType: 'generic', config: {}, outputSchema: ['x'] },
  process: { connectorType: 'generic', config: {}, inputMappings: [], outputSchema: ['y'] },
  sink: null,
  dataSourceToProcessMappings: [{ sourceField: 'x', targetField: 'x_in' }],
  processToSinkMappings: [],
}

const FULL_PIPELINE: PipelineCanvasState = {
  dataSource: { connectorType: 'generic', config: {}, outputSchema: ['x'] },
  process: { connectorType: 'generic', config: {}, inputMappings: [], outputSchema: ['y'] },
  sink: { connectorType: 'generic', config: {}, inputMappings: [] },
  dataSourceToProcessMappings: [{ sourceField: 'x', targetField: 'x_in' }],
  processToSinkMappings: [{ sourceField: 'y', targetField: 'y_out' }],
}

function renderCanvas(props: Partial<PipelineCanvasProps> = {}): {
  onChange: ReturnType<typeof vi.fn>
} {
  const onChange = vi.fn()
  render(
    <PipelineCanvas
      value={EMPTY_STATE}
      onChange={onChange}
      standalone={true}
      {...props}
    />
  )
  return { onChange }
}

// ---------------------------------------------------------------------------
// applyPhaseDropToState — pure function linearity enforcement tests
// ---------------------------------------------------------------------------

describe('applyPhaseDropToState()', () => {
  describe('DataSource', () => {
    it('places DataSource on empty canvas', () => {
      const result = applyPhaseDropToState(EMPTY_STATE, 'DataSource')
      expect(typeof result).not.toBe('string')
      if (typeof result !== 'string') {
        expect(result.dataSource).not.toBeNull()
      }
    })

    it('rejects duplicate DataSource with a string message', () => {
      const result = applyPhaseDropToState(DS_ONLY, 'DataSource')
      expect(typeof result).toBe('string')
      expect(result as string).toContain('DataSource')
    })
  })

  describe('Process', () => {
    it('places Process after DataSource is placed', () => {
      const result = applyPhaseDropToState(DS_ONLY, 'Process')
      expect(typeof result).not.toBe('string')
      if (typeof result !== 'string') {
        expect(result.process).not.toBeNull()
      }
    })

    it('rejects Process before DataSource with a string message', () => {
      const result = applyPhaseDropToState(EMPTY_STATE, 'Process')
      expect(typeof result).toBe('string')
      expect(result as string).toContain('DataSource')
    })

    it('rejects duplicate Process with a string message', () => {
      const result = applyPhaseDropToState(DS_AND_PROCESS, 'Process')
      expect(typeof result).toBe('string')
      expect(result as string).toContain('Process')
    })
  })

  describe('Sink', () => {
    it('places Sink after Process is placed', () => {
      const result = applyPhaseDropToState(DS_AND_PROCESS, 'Sink')
      expect(typeof result).not.toBe('string')
      if (typeof result !== 'string') {
        expect(result.sink).not.toBeNull()
      }
    })

    it('rejects Sink before Process with a string message', () => {
      const result = applyPhaseDropToState(DS_ONLY, 'Sink')
      expect(typeof result).toBe('string')
      expect(result as string).toContain('Process')
    })

    it('rejects duplicate Sink with a string message', () => {
      const result = applyPhaseDropToState(FULL_PIPELINE, 'Sink')
      expect(typeof result).toBe('string')
      expect(result as string).toContain('Sink')
    })
  })
})

// ---------------------------------------------------------------------------
// PipelineCanvas rendering
// ---------------------------------------------------------------------------

describe('PipelineCanvas', () => {
  describe('empty state', () => {
    it('shows the placeholder text when no phases are placed', () => {
      renderCanvas()
      expect(screen.getByText(/Drag components from the palette to build a pipeline/i)).toBeInTheDocument()
    })
  })

  describe('with DataSource placed', () => {
    it('renders the DataSource node header', () => {
      renderCanvas({ value: DS_ONLY })
      // The PhaseNode header renders the phase name in white text.
      // Multiple elements with "DataSource" text may exist (e.g. placeholder slot).
      expect(screen.getAllByText('DataSource').length).toBeGreaterThan(0)
    })

    it('shows Process placeholder slot when only DataSource is placed', () => {
      renderCanvas({ value: DS_ONLY })
      expect(screen.getAllByText('Process').length).toBeGreaterThan(0)
    })
  })

  describe('with DataSource and Process placed', () => {
    it('renders both DataSource and Process nodes', () => {
      renderCanvas({ value: DS_AND_PROCESS })
      expect(screen.getAllByText('DataSource').length).toBeGreaterThan(0)
      expect(screen.getAllByText('Process').length).toBeGreaterThan(0)
    })

    it('renders a mapping chip showing the mapping count', () => {
      renderCanvas({ value: DS_AND_PROCESS })
      expect(screen.getByText(/1 mapping/i)).toBeInTheDocument()
    })
  })

  describe('with full pipeline (DS + Process + Sink)', () => {
    it('renders all three phase nodes', () => {
      renderCanvas({ value: FULL_PIPELINE })
      expect(screen.getAllByText('DataSource').length).toBeGreaterThan(0)
      expect(screen.getAllByText('Process').length).toBeGreaterThan(0)
      expect(screen.getAllByText('Sink').length).toBeGreaterThan(0)
    })

    it('renders two mapping chips (DS→Process and Process→Sink)', () => {
      renderCanvas({ value: FULL_PIPELINE })
      const chips = screen.getAllByText(/mapping/i)
      expect(chips.length).toBeGreaterThanOrEqual(2)
    })
  })

  describe('validation errors', () => {
    it('renders a warning symbol on the mapping chip when validation errors exist for that boundary', () => {
      const errors: MappingValidationError[] = [
        {
          boundary: 'dataSourceToProcess',
          sourceField: 'bad_field',
          message: 'source field "bad_field" not found in datasource output schema',
        },
      ]
      renderCanvas({ value: DS_AND_PROCESS, validationErrors: errors })
      expect(screen.getByText(/⚠/)).toBeInTheDocument()
    })
  })

  describe('read-only mode', () => {
    it('does not render remove buttons when readOnly is true', () => {
      renderCanvas({ value: FULL_PIPELINE, readOnly: true })
      expect(screen.queryByLabelText(/Remove DataSource from pipeline/i)).not.toBeInTheDocument()
      expect(screen.queryByLabelText(/Remove Process from pipeline/i)).not.toBeInTheDocument()
      expect(screen.queryByLabelText(/Remove Sink from pipeline/i)).not.toBeInTheDocument()
    })
  })

  describe('phase removal', () => {
    it('calls onChange with all phases cleared when DataSource is removed', () => {
      const { onChange } = renderCanvas({ value: FULL_PIPELINE })

      screen.getByLabelText('Remove DataSource from pipeline').click()

      expect(onChange).toHaveBeenCalledOnce()
      const newState: PipelineCanvasState = onChange.mock.calls[0][0]
      expect(newState.dataSource).toBeNull()
      expect(newState.process).toBeNull()
      expect(newState.sink).toBeNull()
      expect(newState.dataSourceToProcessMappings).toEqual([])
      expect(newState.processToSinkMappings).toEqual([])
    })

    it('calls onChange with Process and Sink cleared when Process is removed', () => {
      const { onChange } = renderCanvas({ value: FULL_PIPELINE })

      screen.getByLabelText('Remove Process from pipeline').click()

      expect(onChange).toHaveBeenCalledOnce()
      const newState: PipelineCanvasState = onChange.mock.calls[0][0]
      expect(newState.dataSource).not.toBeNull()
      expect(newState.process).toBeNull()
      expect(newState.sink).toBeNull()
      expect(newState.dataSourceToProcessMappings).toEqual([])
      expect(newState.processToSinkMappings).toEqual([])
    })

    it('calls onChange with only Sink cleared when Sink is removed', () => {
      const { onChange } = renderCanvas({ value: FULL_PIPELINE })

      screen.getByLabelText('Remove Sink from pipeline').click()

      expect(onChange).toHaveBeenCalledOnce()
      const newState: PipelineCanvasState = onChange.mock.calls[0][0]
      expect(newState.dataSource).not.toBeNull()
      expect(newState.process).not.toBeNull()
      expect(newState.sink).toBeNull()
      expect(newState.processToSinkMappings).toEqual([])
      expect(newState.dataSourceToProcessMappings).toEqual(FULL_PIPELINE.dataSourceToProcessMappings)
    })
  })
})
