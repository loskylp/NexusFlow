/**
 * SchemaMappingEditor — modal for defining field-to-field schema mappings
 * between two adjacent pipeline phases (TASK-023).
 *
 * Opened when the user clicks a mapping chip on the PipelineCanvas.
 * Presents:
 *   - Source phase output field list (left column, derived from outputSchema)
 *   - Target phase input field list (right column, derived on save as targetField values)
 *   - Per-row mapping rows: sourceField selector → targetField text input
 *   - Add Mapping button
 *   - Remove button per row (trash icon)
 *   - Save button (validates and closes on success)
 *   - Cancel button (closes without applying changes)
 *
 * Design-time validation (ADR-008): on Save, each mapping's sourceField must
 * be present in the source phase's outputSchema. Invalid mappings show a red
 * border on the sourceField selector with a tooltip message.
 *
 * See: TASK-023, REQ-007, ADR-008, UX Spec (Pipeline Builder — schema mapping editor)
 */

import React, { useEffect, useState } from 'react'
import type { SchemaMapping } from '@/types/domain'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SchemaMappingEditorProps {
  /** Whether the editor is open. */
  isOpen: boolean
  /** Label for the source phase (e.g. "DataSource → Process"). */
  title: string
  /**
   * Field names declared in the source phase's outputSchema.
   * Used to populate the sourceField selector dropdown.
   */
  sourceFields: string[]
  /** Current mappings to display and edit. */
  mappings: SchemaMapping[]
  /**
   * Called with the updated mappings array when the user clicks Save.
   * The parent updates the PipelineCanvasState and triggers API validation.
   */
  onSave: (mappings: SchemaMapping[]) => void
  /** Called when the user clicks Cancel or closes the modal. No state change. */
  onClose: () => void
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * SchemaMappingEditor renders the mapping editor in an accessible dialog.
 *
 * States handled:
 *   - Valid: all sourceFields are present in sourceFields prop; Save is enabled
 *   - Invalid: one or more sourceField values not in sourceFields prop; row shows
 *     red border on the selector and a tooltip; Save is disabled
 *   - Empty: no mappings yet; shows "No mappings defined" placeholder with
 *     an "Add Mapping" prompt
 *
 * Preconditions:
 *   - sourceFields is non-empty; otherwise the selector has no valid options.
 *   - onSave and onClose are stable callbacks.
 *
 * Postconditions:
 *   - On Save: client-side validation passes; onSave called with updated mappings.
 *   - On Cancel: onClose called; mappings prop unchanged.
 *   - Local edits are reset when isOpen transitions false → true (editor re-opens
 *     with the current mappings prop value).
 */
function SchemaMappingEditor({
  isOpen,
  title,
  sourceFields,
  mappings,
  onSave,
  onClose,
}: SchemaMappingEditorProps): React.ReactElement | null {
  const [localMappings, setLocalMappings] = useState<SchemaMapping[]>(mappings)

  // Reset local state each time the modal opens with the current mappings.
  useEffect(() => {
    if (isOpen) {
      setLocalMappings(mappings)
    }
  }, [isOpen, mappings])

  if (!isOpen) return null

  const sourceFieldSet = new Set(sourceFields)

  /**
   * isMappingValid returns true when the mapping's sourceField is present in
   * the source phase's declared outputSchema.
   */
  function isMappingValid(mapping: SchemaMapping): boolean {
    return sourceFields.length === 0 || sourceFieldSet.has(mapping.sourceField)
  }

  const hasInvalidMappings = localMappings.some(m => !isMappingValid(m))
  const canSave = !hasInvalidMappings

  /** addMapping appends a blank mapping row to the local list. */
  function addMapping(): void {
    setLocalMappings(prev => [
      ...prev,
      { sourceField: sourceFields[0] ?? '', targetField: '' },
    ])
  }

  /** removeMapping removes the mapping at the given index. */
  function removeMapping(index: number): void {
    setLocalMappings(prev => prev.filter((_, i) => i !== index))
  }

  /** updateSourceField updates the sourceField for the row at index. */
  function updateSourceField(index: number, value: string): void {
    setLocalMappings(prev =>
      prev.map((m, i) => (i === index ? { ...m, sourceField: value } : m))
    )
  }

  /** updateTargetField updates the targetField for the row at index. */
  function updateTargetField(index: number, value: string): void {
    setLocalMappings(prev =>
      prev.map((m, i) => (i === index ? { ...m, targetField: value } : m))
    )
  }

  /** handleSave validates and calls onSave when the mappings are valid. */
  function handleSave(): void {
    if (!canSave) return
    onSave(localMappings)
    onClose()
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={title}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundColor: 'rgba(0,0,0,0.4)',
      }}
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div
        style={{
          backgroundColor: 'var(--color-surface-panel)',
          border: '1px solid var(--color-border)',
          borderRadius: '8px',
          padding: '24px',
          minWidth: '520px',
          maxWidth: '680px',
          maxHeight: '80vh',
          overflowY: 'auto',
          boxShadow: '0 8px 32px rgba(0,0,0,0.24)',
        }}
      >
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
          <h2 style={{ margin: 0, fontSize: '16px', fontWeight: 600, color: 'var(--color-text-primary)' }}>
            {title}
          </h2>
          <button
            aria-label="Close mapping editor"
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              fontSize: '18px',
              cursor: 'pointer',
              color: 'var(--color-text-secondary)',
              padding: '4px',
            }}
          >
            ×
          </button>
        </div>

        {/* Column headers */}
        {localMappings.length > 0 && (
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 32px', gap: '8px', marginBottom: '8px' }}>
            <span style={{ fontSize: '12px', fontFamily: 'var(--font-label)', textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--color-text-secondary)' }}>
              Source Field
            </span>
            <span style={{ fontSize: '12px', fontFamily: 'var(--font-label)', textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--color-text-secondary)' }}>
              Target Field
            </span>
            <span />
          </div>
        )}

        {/* Mapping rows */}
        {localMappings.length === 0 ? (
          <div style={{ padding: '24px', textAlign: 'center', color: 'var(--color-text-secondary)', fontSize: '14px' }}>
            No mappings defined. Click "Add Mapping" to begin.
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginBottom: '16px' }}>
            {localMappings.map((mapping, index) => {
              const isValid = isMappingValid(mapping)
              return (
                <div
                  key={index}
                  style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 32px', gap: '8px', alignItems: 'center' }}
                >
                  {/* Source field selector */}
                  <div style={{ position: 'relative' }}>
                    <select
                      value={mapping.sourceField}
                      onChange={e => updateSourceField(index, e.target.value)}
                      aria-label={`Source field for mapping ${index + 1}`}
                      title={!isValid ? `"${mapping.sourceField}" is not in the source schema` : undefined}
                      style={{
                        width: '100%',
                        padding: '6px 8px',
                        fontSize: '13px',
                        fontFamily: 'var(--font-mono)',
                        border: `1px solid ${isValid ? 'var(--color-border)' : '#EF4444'}`,
                        borderRadius: '4px',
                        backgroundColor: isValid ? 'var(--color-surface-panel)' : '#FEF2F2',
                        color: 'var(--color-text-primary)',
                        cursor: 'pointer',
                      }}
                    >
                      {sourceFields.map(f => (
                        <option key={f} value={f}>{f}</option>
                      ))}
                      {/* Preserve a stale value that no longer exists in sourceFields */}
                      {!sourceFieldSet.has(mapping.sourceField) && mapping.sourceField && (
                        <option value={mapping.sourceField}>{mapping.sourceField} (invalid)</option>
                      )}
                    </select>
                    {!isValid && (
                      <span
                        role="alert"
                        style={{
                          position: 'absolute',
                          bottom: '-18px',
                          left: 0,
                          fontSize: '11px',
                          color: '#DC2626',
                        }}
                      >
                        Not in source schema
                      </span>
                    )}
                  </div>

                  {/* Target field input */}
                  <input
                    type="text"
                    value={mapping.targetField}
                    onChange={e => updateTargetField(index, e.target.value)}
                    placeholder="target field name"
                    aria-label={`Target field for mapping ${index + 1}`}
                    style={{
                      padding: '6px 8px',
                      fontSize: '13px',
                      fontFamily: 'var(--font-mono)',
                      border: '1px solid var(--color-border)',
                      borderRadius: '4px',
                      backgroundColor: 'var(--color-surface-panel)',
                      color: 'var(--color-text-primary)',
                    }}
                  />

                  {/* Remove button */}
                  <button
                    onClick={() => removeMapping(index)}
                    aria-label={`Remove mapping ${index + 1}`}
                    title="Remove mapping"
                    style={{
                      background: 'none',
                      border: 'none',
                      cursor: 'pointer',
                      fontSize: '16px',
                      color: '#DC2626',
                      padding: '4px',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    ×
                  </button>
                </div>
              )
            })}
          </div>
        )}

        {/* Add mapping button */}
        <button
          onClick={addMapping}
          style={{
            display: 'block',
            width: '100%',
            padding: '8px',
            marginBottom: '20px',
            marginTop: localMappings.length > 0 ? '12px' : '0',
            border: '1px dashed var(--color-border)',
            borderRadius: '4px',
            background: 'none',
            cursor: 'pointer',
            fontSize: '13px',
            color: 'var(--color-text-secondary)',
          }}
        >
          + Add Mapping
        </button>

        {/* Footer actions */}
        <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
          <button
            onClick={onClose}
            style={{
              padding: '8px 16px',
              border: '1px solid var(--color-border)',
              borderRadius: '6px',
              background: 'none',
              cursor: 'pointer',
              fontSize: '14px',
              color: 'var(--color-text-secondary)',
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={!canSave}
            style={{
              padding: '8px 16px',
              border: 'none',
              borderRadius: '6px',
              backgroundColor: canSave ? '#4F46E5' : '#94A3B8',
              color: '#FFFFFF',
              cursor: canSave ? 'pointer' : 'not-allowed',
              fontSize: '14px',
              fontWeight: 500,
              opacity: canSave ? 1 : 0.5,
            }}
          >
            Save Mappings
          </button>
        </div>
      </div>
    </div>
  )
}

export default SchemaMappingEditor
