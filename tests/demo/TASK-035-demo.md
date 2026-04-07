# Demo Script — TASK-035
**Feature:** Task Submission via GUI (complete flow)
**Requirement(s):** REQ-002
**Environment:** Staging — log in as a regular User (not Admin)

## Scenario 1: Submit a task with default settings
**REQ:** REQ-002

**Given:** You are logged in as a User. At least one pipeline definition exists. You are on the Task Feed page.

**When:** Click the "Submit Task" button in the filter bar. In the modal, the first available pipeline is already pre-selected. Leave all parameters and retry settings at their defaults. Click "Submit Task".

**Then:** The modal closes. The Task Feed refreshes and shows a new task card with status badge "submitted" and the pipeline name you selected.

---

## Scenario 2: Pipeline selector shows available pipelines
**REQ:** REQ-002

**Given:** You are logged in as a User. Multiple pipelines exist (at least two). You are on the Task Feed page.

**When:** Click "Submit Task". Observe the Pipeline dropdown in the modal.

**Then:** The dropdown lists all available pipelines by name. The first pipeline is pre-selected. You can change the selection to any other pipeline in the list.

---

## Scenario 3: Inline validation blocks submission with empty parameter key
**REQ:** REQ-002

**Given:** You are logged in as a User. At least one pipeline exists. The Submit Task modal is open.

**When:** Click "Add Parameter". Leave the Key field empty. Type any value in the Value field. Click "Submit Task".

**Then:** The task is NOT submitted. An inline validation message appears below the parameters section reading "Parameter key cannot be empty". The modal remains open.

**Notes:** The error clears as soon as you start typing in the Key field. Try submitting again after fixing the key — it should succeed.

---

## Scenario 4: Inline validation blocks submission with duplicate parameter keys
**REQ:** REQ-002

**Given:** The Submit Task modal is open.

**When:** Click "Add Parameter" twice. Type the same key (e.g. "source") in both Key fields. Click "Submit Task".

**Then:** The task is NOT submitted. An inline validation message appears reading "Duplicate parameter key — each key must be unique". The modal remains open.

---

## Scenario 5: Submit a task with custom input parameters
**REQ:** REQ-002

**Given:** The Submit Task modal is open.

**When:** Click "Add Parameter". Type "dataset" in the Key field and "s3://my-bucket/data.csv" in the Value field. Click "Submit Task".

**Then:** The task is created and appears in the Task Feed with status "submitted". The submitted task carries the input parameter you specified (verifiable via the API: GET /api/tasks/{id} should show input: {"dataset": "s3://my-bucket/data.csv"}).

---

## Scenario 6: Submit a task with custom retry configuration
**REQ:** REQ-002

**Given:** The Submit Task modal is open.

**When:** Set Max Retries to 3. Set Backoff Strategy to "Exponential". Click "Submit Task".

**Then:** The task is created with retryConfig {maxRetries: 3, backoff: "exponential"} (verifiable via GET /api/tasks/{id}). The task appears in the Task Feed with status "submitted".

---

## Scenario 7: GUI-created task is equivalent to API-created task
**REQ:** REQ-002

**Given:** A pipeline named "ETL Pipeline" exists. You have access to the REST API.

**When:** Submit a task via the GUI with pipeline "ETL Pipeline" and input parameter key "source", value "s3://test/data". Then submit an equivalent task directly via the API: POST /api/tasks with body {"pipelineId": "<ETL Pipeline ID>", "input": {"source": "s3://test/data"}}.

**Then:** Both tasks appear in the Task Feed with status "submitted". Both have the same pipeline reference and input parameters. Both follow the same lifecycle (submitted → queued → assigned → running → completed/failed).

---

## Scenario 8: Modal state resets on re-open
**REQ:** REQ-002

**Given:** The Submit Task modal has been opened, a parameter was added, and then Cancel was clicked.

**When:** Click "Submit Task" again to re-open the modal.

**Then:** The modal opens with a clean state: no parameter rows, Max Retries = 0, Backoff Strategy = "Fixed". Any prior error messages are gone.
