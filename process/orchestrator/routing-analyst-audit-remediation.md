# Routing Instruction

**To:** Analyst
**Phase:** INGESTION (audit remediation)
**Task:** Revise Requirements v1 to resolve AUDIT-002 (pipeline CRUD gap) and AUDIT-004 (REQ-003/NFR-001 duplication ambiguity). Do NOT revise REQ-019/REQ-020 yet -- AUDIT-003 awaits Nexus decision on the auth mechanism.
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| Requirements v1 | [Requirements List v1](process/analyst/requirements-v1.md) | Base document to revise |
| Brief v1 | [Brief v1](process/analyst/brief-v1.md) | Domain context, User Roles table (pipeline ownership), Open Context Questions |
| Audit Report | [Requirements Audit v1](process/auditor/requirements-audit-v1.md) | Full audit findings -- AUDIT-002 and AUDIT-004 details |

---

## Context

Two audit findings to resolve in this pass. A third (AUDIT-003, auth mechanism) is pending Nexus decision and will be routed separately once answered.

**AUDIT-002 -- Pipeline CRUD gap:**
The Brief's User Roles table states Users can "Create/read/update own pipelines" and Admins can "manage pipelines." REQ-015 covers drag-and-drop pipeline construction but no requirement covers the full pipeline CRUD lifecycle: listing, editing, deleting pipelines, or the REST API surface for pipeline management. Tasks cannot be submitted (REQ-001) without pipelines existing as persistent named entities. Add requirement(s) covering pipeline CRUD for both GUI and REST API, including ownership and access control rules. Assign new REQ IDs following the existing numbering sequence. Include full Definition of Done and acceptance scenarios at Blueprint weight.

**AUDIT-004 -- REQ-003/NFR-001 duplication:**
REQ-003 states "Queuing latency must be under 50ms at p95" inline. NFR-001 restates the same target but adds a load condition ("under sustained load of 10,000 tasks/hour"). This creates ambiguity about whether REQ-003's 50ms target applies at any load or only under NFR-001's load condition. Resolution: REQ-003 should reference NFR-001 for the latency SLA (removing its inline 50ms claim) or the two should be explicitly scoped to different conditions. This is an Analyst clarification -- no Nexus input needed.

**Important:** Produce the output as Requirements v2. Do not modify REQ-019 or REQ-020 in this version -- those will be revised in a subsequent pass after the Nexus answers the auth mechanism question.
