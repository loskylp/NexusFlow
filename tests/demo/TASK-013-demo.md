# Demo Script — TASK-013
**Feature:** Pipeline CRUD via REST API
**Requirement(s):** REQ-022
**Environment:** Staging API — `https://<staging-host>/api` (substitute actual staging URL)

---

## Scenario 1: Create a pipeline
**REQ:** REQ-022

**Given:** You are logged in as any authenticated user (admin or regular user). You have obtained a session token from `POST /api/auth/login`.

**When:** Send `POST /api/pipelines` with the following JSON body, setting `Authorization: Bearer <token>`:
```json
{
  "name": "demo-pipeline-001",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {"rows": 10},
    "outputSchema": ["id", "name", "value"]
  },
  "processConfig": {
    "connectorType": "passthrough",
    "config": {},
    "inputMappings": [
      {"sourceField": "id",    "targetField": "record_id"},
      {"sourceField": "name",  "targetField": "label"},
      {"sourceField": "value", "targetField": "amount"}
    ],
    "outputSchema": ["record_id", "label", "amount"]
  },
  "sinkConfig": {
    "connectorType": "demo-sink",
    "config": {"target": "stdout"},
    "inputMappings": [
      {"sourceField": "record_id", "targetField": "id"},
      {"sourceField": "label",     "targetField": "name"},
      {"sourceField": "amount",    "targetField": "value"}
    ]
  }
}
```

**Then:** The response is `201 Created`. The body contains an `id` (UUID), `name` matching the submitted name, `userId` matching the authenticated user's ID (not any caller-supplied value), `dataSourceConfig`, `processConfig`, `sinkConfig`, `createdAt`, and `updatedAt`. Record the returned `id` for subsequent scenarios.

**Notes:** Omitting `name` returns `400 Bad Request`. Submitting without an `Authorization` header returns `401 Unauthorized`.

---

## Scenario 2: List pipelines (role-based visibility)
**REQ:** REQ-022

**Given:** Pipeline created in Scenario 1 exists and is owned by the admin user. A second pipeline owned by a regular user also exists.

**When (admin):** Send `GET /api/pipelines` with the admin token.

**Then:** Response is `200 OK`. The JSON array contains both the admin's pipeline and the regular user's pipeline. Both `id` fields are present.

**When (regular user):** Send `GET /api/pipelines` with the regular user's token.

**Then:** Response is `200 OK`. The JSON array contains only the pipelines owned by that user. The admin's pipeline does not appear.

**Notes:** An unauthenticated request returns `401 Unauthorized`.

---

## Scenario 3: Retrieve a single pipeline
**REQ:** REQ-022

**Given:** The pipeline from Scenario 1 exists. You have its `id`.

**When:** Send `GET /api/pipelines/<id>` with the owner's token.

**Then:** Response is `200 OK` with the full pipeline JSON including all three phase config fields.

**Notes:** `GET /api/pipelines/00000000-0000-0000-0000-000000000099` (a valid UUID that does not exist) returns `404 Not Found`. `GET /api/pipelines/not-a-uuid` returns `400 Bad Request`.

---

## Scenario 4: Update a pipeline
**REQ:** REQ-022

**Given:** The pipeline from Scenario 1 exists. You are the owner.

**When:** Send `PUT /api/pipelines/<id>` with the owner's token and the same body structure as Scenario 1 but with `"name": "demo-pipeline-001-updated"` and any config change (e.g. `"rows": 20` in `dataSourceConfig`).

**Then:** Response is `200 OK`. The `name` field in the response body is `demo-pipeline-001-updated`. The `userId` is unchanged (still the original owner). A subsequent `GET /api/pipelines/<id>` reflects the new name.

**Notes:** `PUT` on a non-existent UUID returns `404 Not Found`.

---

## Scenario 5: Delete a pipeline with no active tasks
**REQ:** REQ-022

**Given:** A pipeline exists that either has no associated tasks, or has only tasks in terminal states (`completed`, `failed`, or `cancelled`).

**When:** Send `DELETE /api/pipelines/<id>` with the owner's token.

**Then:** Response is `204 No Content`. A subsequent `GET /api/pipelines/<id>` returns `404`. Any historical task rows that referenced this pipeline remain in the database with `pipeline_id` set to `NULL` (the pipeline reference is orphaned, not cascaded).

**Notes:** `DELETE` on a non-existent UUID returns `404 Not Found`.

---

## Scenario 6: Delete blocked by active tasks
**REQ:** REQ-022

**Given:** A pipeline exists and has at least one task in a non-terminal state (e.g. `running`, `queued`, `submitted`, or `assigned`).

**When:** Send `DELETE /api/pipelines/<id>` with the owner's token.

**Then:** Response is `409 Conflict`. The pipeline row is still present in the database. The active task is unaffected.

**Notes:** Once all tasks for the pipeline reach a terminal state, the same `DELETE` request will succeed with `204`. This can be verified by completing or cancelling the blocking task and retrying.

---

## Scenario 7: Access control — non-owner blocked, admin permitted
**REQ:** REQ-022

**Given:** A pipeline is owned by User A. User B is a regular (non-admin) user who does not own the pipeline. An admin user exists.

**When (non-owner GET):** User B sends `GET /api/pipelines/<id>` for User A's pipeline.

**Then:** `403 Forbidden`.

**When (non-owner PUT):** User B sends `PUT /api/pipelines/<id>` for User A's pipeline.

**Then:** `403 Forbidden`.

**When (non-owner DELETE):** User B sends `DELETE /api/pipelines/<id>` for User A's pipeline.

**Then:** `403 Forbidden`.

**When (admin GET):** Admin sends `GET /api/pipelines/<id>` for User A's pipeline.

**Then:** `200 OK` — admin can read any pipeline regardless of ownership.

**When (admin DELETE):** Admin sends `DELETE /api/pipelines/<id>` for User A's pipeline (with no active tasks).

**Then:** `204 No Content` — admin can delete any pipeline regardless of ownership.
