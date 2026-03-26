# Routing Instruction

**To:** Methodologist
**Phase:** INGESTION (audit remediation)
**Task:** Update Manifest to re-activate the Designer agent -- Nexus has decided that the Designer must be active for UX design of the four GUI views.
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| Current Manifest | [Manifest v1](process/methodologist/manifest-v1.md) | Base document to revise |
| Audit Report | [Requirements Audit v1](process/auditor/requirements-audit-v1.md#audit-001-contradiction----manifest-marks-designer-skipped-but-requirements-demand-web-gui) | AUDIT-001 details the contradiction |
| Brief | [Brief v1](process/analyst/brief-v1.md) | Delivery channel and GUI view descriptions |

---

## Context

The Nexus has explicitly decided: "Re-activate the Designer agent. The Manifest must be corrected to include the Designer for UX design of the four GUI views."

Changes required in Manifest v2:
1. Designer agent status: change from "Skipped" to "Active" with appropriate notes (Web GUI: Pipeline Builder, Worker Fleet Dashboard, Task Feed and Monitor, Log Streamer).
2. Remove or revise the "Acceptance criteria for skipped agents" section for Designer.
3. Add Designer to the Documentation Requirements table (UX specifications for the four views).
4. Update the Changelog to record v2 and the reason.

No other Manifest changes are needed at this time. The profile, gates, iteration model, and all other agent configurations remain unchanged.
