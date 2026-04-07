<!--
Copyright 2026 Pablo Ochendrowitsch

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

---
task: TASK-023
title: Pipeline Builder (GUI)
requirements: REQ-015, REQ-007
environment: Staging — navigate to the Pipeline Builder page via the left sidebar
---

# Demo Script — TASK-023
**Feature:** Pipeline Builder (GUI)
**Requirement(s):** REQ-015, REQ-007
**Environment:** Staging — log in as an admin user and navigate to "Pipeline Builder" in the left sidebar nav

## Scenario 1: Build a complete pipeline by dragging three phases onto the canvas
**REQ:** REQ-015

Given   you are on the Pipeline Builder page with an empty canvas (no phases placed)

When    you drag the "DataSource" card from the left palette onto the canvas drop area, then drag "Process", then drag "Sink"

Then    the canvas shows three connected phase nodes in left-to-right order: DataSource → Process → Sink; two mapping chips appear between the phases showing "0 mappings" each; the pipeline name field is active

---

## Scenario 2: Linearity is enforced — duplicate phases are rejected
**REQ:** REQ-015

Given   you have placed DataSource on the canvas

When    you attempt to drag a second DataSource from the palette onto the canvas

Then    a tooltip overlay appears on the canvas explaining why the drop was rejected (the message mentions "DataSource"); the DataSource palette card shows a "placed" badge and is visually disabled; the canvas state is unchanged (still one DataSource, no duplicate)

---

## Scenario 3: Schema mapping editor opens on chip click and saves field mappings
**REQ:** REQ-007

Given   you have a complete pipeline on the canvas (DataSource, Process, Sink all placed)

When    you click the mapping chip between DataSource and Process (labelled "0 mappings")

Then    the Schema Mapping Editor dialog opens with title "DataSource → Process Mappings" (or similar boundary label); you can add a mapping row, select source and target fields, click Save, and the dialog closes; the chip now shows the updated mapping count

---

## Scenario 4: Save validates schema mappings — invalid mappings show red border
**REQ:** REQ-007

Given   the Schema Mapping Editor is open with at least one mapping row

When    you type a source field name that does not exist in the source schema and attempt to save

Then    the source field input shows a red border and a "Not in source schema" error message; the Save button is disabled until all source fields are valid

**Notes:** In staging, phases have empty output schemas by default (outputSchema: []), so all source field values will be considered valid until real schema configuration is added in a future task. Verify the red-border behaviour by observing it in the component's own validation state — type a value, then check the Save button state.

---

## Scenario 5: Saving a pipeline persists it and makes it available in the list
**REQ:** REQ-015

Given   you have a complete pipeline (all three phases placed) and have entered a pipeline name in the toolbar

When    you click the Save button

Then    a success toast appears; the asterisk (*) next to the pipeline name disappears; the pipeline appears in the "Saved Pipelines" list in the left palette; the Run button becomes enabled

---

## Scenario 6: Run button opens the task submission form pre-populated with the pipeline
**REQ:** REQ-015

Given   you have a saved pipeline loaded on the canvas (the Run button is enabled)

When    you click the Run button in the toolbar

Then    a task submission modal opens with the current pipeline pre-selected in the pipeline selector; clicking Cancel closes the modal without submitting

**Notes:** The submission form is minimal for this task — it submits with empty input parameters. Full parameter form is deferred to TASK-035.

---

## Scenario 7: Navigation guard warns about unsaved changes
**REQ:** REQ-015

Given   you have made changes to the canvas that have not been saved (the asterisk * is visible in the toolbar)

When    you attempt to navigate away from the Pipeline Builder page by clicking another nav link

Then    a confirmation dialog appears asking whether you want to leave; if you confirm, navigation proceeds; if you cancel, you remain on the Pipeline Builder page

**Notes:** Also verify the browser-level guard: make unsaved changes, then attempt to close the tab or press the browser back button. The browser should show a native "Leave site?" confirmation.

---

## Scenario 8: Saved pipelines list loads on page open; clicking a pipeline restores it onto the canvas
**REQ:** REQ-015

Given   at least one pipeline has been saved previously and you are on the Pipeline Builder page

When    the page finishes loading

Then    the saved pipelines appear in the "Saved Pipelines" section of the left palette; clicking a pipeline name loads its phases and name into the canvas and name field; the Run button becomes enabled (the pipeline has an ID)

**Notes:** If the canvas has unsaved changes when you click a saved pipeline, a confirmation dialog should appear first.

---
