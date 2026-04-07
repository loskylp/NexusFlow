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

import React from 'react'
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
  // TODO: implement
  throw new Error('Not implemented')
}

export default SchemaMappingEditor
