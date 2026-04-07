# Demo Script — TASK-024
**Feature:** Pipeline Management GUI (list/edit/delete)
**Requirement(s):** REQ-023, REQ-015
**Environment:** Staging — navigate to `/pipelines` after logging in

## Scenario 1: Pipeline list shows role-appropriate pipelines
**REQ:** REQ-023

**Given:** At least two pipelines have been created — one owned by a regular User account and one owned by a different user. You are logged in as the regular User.

**When:** Navigate to the Pipeline Builder page (`/pipelines`). Observe the "Saved Pipelines" section in the left panel.

**Then:** Only the pipelines owned by the logged-in User are listed. The other user's pipeline does not appear.

**Notes:** Log out and log back in as an Admin. The Admin's pipeline list should include pipelines from all users. Role filtering is enforced server-side; the frontend renders whatever the API returns.

---

## Scenario 2: Edit action loads pipeline onto the canvas
**REQ:** REQ-023

**Given:** You are logged in and at least one saved pipeline appears in the "Saved Pipelines" list.

**When:** Click the name of a saved pipeline in the left panel.

**Then:** The pipeline is loaded onto the canvas. The pipeline name input at the top of the canvas area shows the pipeline's name. The DataSource, Process, and Sink phase nodes are visible on the canvas, and any schema mappings between phases are shown on the connector chips.

**Notes:** If the canvas has unsaved changes when you click a pipeline name, a confirmation dialog appears ("You have unsaved changes. Load another pipeline?"). Clicking Cancel aborts the load and preserves the unsaved state. Clicking OK loads the selected pipeline.

---

## Scenario 3: Delete with confirmation dialog
**REQ:** REQ-023

**Given:** You are logged in and at least one saved pipeline appears in the "Saved Pipelines" list. The pipeline has no active tasks.

**When:** Click the × (delete) icon next to a pipeline in the "Saved Pipelines" list.

**Then:** An inline confirmation prompt appears within the list item: `Delete "[pipeline name]"?` with two buttons — Delete (red) and Cancel.

**When:** Click Delete.

**Then:** The pipeline is deleted. The pipeline disappears from the list. A green toast notification appears: "Pipeline deleted."

**Notes:** If the pipeline currently loaded on the canvas is deleted, the canvas clears to the empty state.

---

## Scenario 4: Cancel aborts deletion without removing the pipeline
**REQ:** REQ-023

**Given:** You are logged in and at least one saved pipeline appears in the "Saved Pipelines" list.

**When:** Click the × (delete) icon next to a pipeline. When the inline confirmation appears, click Cancel.

**Then:** The confirmation prompt disappears. The pipeline remains in the list. No deletion occurred.

---

## Scenario 5: Delete blocked when pipeline has active tasks (409)
**REQ:** REQ-023

**Given:** You are logged in. A pipeline has one or more tasks currently in a non-terminal state (submitted, queued, assigned, or running). This pipeline is visible in the "Saved Pipelines" list.

**When:** Click the × (delete) icon next to that pipeline. When the confirmation prompt appears, click Delete.

**Then:** A red toast notification appears: "Cannot delete pipeline: it has active tasks." The pipeline remains in the list. No deletion occurred.

**Notes:** To set up this scenario, submit a task using the pipeline via the Task Feed (or API) and ensure it is not yet in a terminal state before attempting deletion. The 409 protection is enforced server-side.
