/**
 * Unit tests for SchemaMappingEditor component.
 * Tests modal visibility, local state reset on re-open, row add/remove,
 * client-side validation, and save/cancel callbacks.
 *
 * See: TASK-023, REQ-007, ADR-008
 */
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import SchemaMappingEditor from './SchemaMappingEditor'
import type { SchemaMappingEditorProps } from './SchemaMappingEditor'
import type { SchemaMapping } from '@/types/domain'

function renderEditor(overrides: Partial<SchemaMappingEditorProps> = {}): {
  onSave: ReturnType<typeof vi.fn>
  onClose: ReturnType<typeof vi.fn>
} {
  const onSave = vi.fn()
  const onClose = vi.fn()

  const props: SchemaMappingEditorProps = {
    isOpen: true,
    title: 'DataSource → Process Mapping',
    sourceFields: ['field_a', 'field_b'],
    mappings: [],
    onSave,
    onClose,
    ...overrides,
  }

  render(<SchemaMappingEditor {...props} />)

  return { onSave, onClose }
}

describe('SchemaMappingEditor', () => {
  it('returns null when isOpen is false', () => {
    const { container } = render(
      <SchemaMappingEditor
        isOpen={false}
        title="Test"
        sourceFields={['x']}
        mappings={[]}
        onSave={vi.fn()}
        onClose={vi.fn()}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders the dialog when isOpen is true', () => {
    renderEditor()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('DataSource → Process Mapping')).toBeInTheDocument()
  })

  it('shows empty state placeholder when no mappings', () => {
    renderEditor({ mappings: [] })
    expect(screen.getByText(/No mappings defined/i)).toBeInTheDocument()
  })

  it('renders existing mappings as rows', () => {
    const mappings: SchemaMapping[] = [
      { sourceField: 'field_a', targetField: 'dest_x' },
    ]
    renderEditor({ mappings })
    expect(screen.getByDisplayValue('field_a')).toBeInTheDocument()
    expect(screen.getByDisplayValue('dest_x')).toBeInTheDocument()
  })

  it('adds a new mapping row when Add Mapping is clicked', () => {
    renderEditor()
    fireEvent.click(screen.getByText('+ Add Mapping'))
    // A new select and a target input should appear
    expect(screen.getByRole('combobox')).toBeInTheDocument()
  })

  it('removes a mapping row when remove button is clicked', () => {
    const mappings: SchemaMapping[] = [
      { sourceField: 'field_a', targetField: 'dest_x' },
    ]
    renderEditor({ mappings })
    fireEvent.click(screen.getByLabelText('Remove mapping 1'))
    expect(screen.queryByDisplayValue('dest_x')).not.toBeInTheDocument()
  })

  it('calls onSave with current mappings when Save is clicked', () => {
    const mappings: SchemaMapping[] = [
      { sourceField: 'field_a', targetField: 'result' },
    ]
    const { onSave } = renderEditor({ mappings })
    fireEvent.click(screen.getByText('Save Mappings'))
    expect(onSave).toHaveBeenCalledOnce()
    expect(onSave).toHaveBeenCalledWith(mappings)
  })

  it('calls onClose when Cancel is clicked', () => {
    const { onClose } = renderEditor()
    fireEvent.click(screen.getByText('Cancel'))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('disables Save and shows validation error when sourceField is not in sourceFields', () => {
    // Render with a mapping that has an invalid sourceField
    const mappings: SchemaMapping[] = [
      { sourceField: 'nonexistent', targetField: 'y' },
    ]
    const { onSave } = renderEditor({ mappings, sourceFields: ['field_a'] })

    const saveButton = screen.getByText('Save Mappings')
    expect(saveButton).toBeDisabled()

    // Validation error text should be visible
    expect(screen.getByText(/Not in source schema/i)).toBeInTheDocument()

    fireEvent.click(saveButton)
    expect(onSave).not.toHaveBeenCalled()
  })

  it('enables Save when all sourceFields are valid', () => {
    const mappings: SchemaMapping[] = [
      { sourceField: 'field_a', targetField: 'out' },
    ]
    const { onSave } = renderEditor({ mappings, sourceFields: ['field_a'] })

    const saveButton = screen.getByText('Save Mappings')
    expect(saveButton).not.toBeDisabled()

    fireEvent.click(saveButton)
    expect(onSave).toHaveBeenCalledWith(mappings)
  })

  it('resets local state when isOpen transitions from false to true', () => {
    const initialMappings: SchemaMapping[] = [
      { sourceField: 'field_a', targetField: 'x' },
    ]
    const { rerender } = render(
      <SchemaMappingEditor
        isOpen={false}
        title="Test"
        sourceFields={['field_a']}
        mappings={initialMappings}
        onSave={vi.fn()}
        onClose={vi.fn()}
      />
    )

    // Close → open with fresh mappings prop
    rerender(
      <SchemaMappingEditor
        isOpen={true}
        title="Test"
        sourceFields={['field_a']}
        mappings={initialMappings}
        onSave={vi.fn()}
        onClose={vi.fn()}
      />
    )

    expect(screen.getByDisplayValue('x')).toBeInTheDocument()
  })

  it('calls onClose when clicking the overlay background', () => {
    const { onClose } = renderEditor()
    const dialog = screen.getByRole('dialog')
    fireEvent.click(dialog)
    expect(onClose).toHaveBeenCalledOnce()
  })
})
