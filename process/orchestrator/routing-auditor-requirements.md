# Routing Instruction

**To:** Auditor
**Phase:** INGESTION -- Requirements Audit
**Task:** Audit Requirements v1 against Brief v1 for completeness, consistency, testability, and traceability. Flag any contradictions between the requirements and the Manifest configuration.
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| Requirements List v1 | [requirements.md](process/analyst/requirements.md) | The artifact being audited |
| Brief v1 | [brief.md](process/analyst/brief.md) | Source of truth for domain model, scope, and delivery channel |
| Methodology Manifest v1 | [manifest-v1.md](process/methodologist/manifest-v1.md) | Agent configuration -- Auditor should verify consistency between Manifest agent statuses and requirements |

---

## Context

The Analyst has flagged three items the Auditor should pay particular attention to:

1. **Designer agent contradiction (ESC-001):** The Manifest marks Designer as "Skipped" with rationale "No user-facing UI." The requirements include 4 web GUI views (REQ-015, REQ-016, REQ-017, REQ-018) and GUI-based task submission (REQ-002). The Brief's Delivery Channel is "Hybrid -- Web App + REST API." This is a configuration contradiction that the Auditor should formally flag.

2. **Uncertainty flags:** The Analyst notes uncertainty on:
   - REQ-007 (schema mapping depth) -- how deep/complex can schema mappings be?
   - REQ-019/REQ-020 (auth mechanism) -- no auth mechanism specified; does NexusFlow manage its own credentials or integrate with an external system?
   - REQ-008 (atomic sink abstraction) -- how is atomicity implemented across different sink types?

   The Auditor should assess whether these uncertainties make the affected requirements untestable or underspecified, or whether they can be deferred to the Architect as implementation decisions.

3. **Open context questions in the Brief** (Section "Open Context Questions") -- these are non-blocking but the Auditor should confirm no requirement depends on an unanswered question.
