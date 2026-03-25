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

# Routing Instruction

**To:** [Agent name]
**Phase:** [Current lifecycle phase]
**Task:** [What the agent should do — one sentence]
**Iteration:** [N of max N — omit if not in an iterate loop]
**Verifier mode:** [Initial verification | Iterate-loop re-verification | Iterate-loop re-verification + requirement change: REQ-NNN changed/superseded/cancelled — required on all Verifier routings; omit for other agents]
**Return to:** Orchestrator when complete

---

## Required documents

*Link directly to the specific file and anchor the receiving agent needs. The agent must not search for documents not listed here. All paths are relative to the project root. Follow [`.claude/skills/traceability-links.md`](.claude/skills/traceability-links.md) when filling this section.*

| Document | Link | Why needed |
|---|---|---|
| [document type] | [display text](relative/path/to/file.md#anchor) | [one phrase] |

**By agent — what to include:**

- **Builder:** task acceptance criteria, relevant REQ-NNN (linked to anchor), relevant ADR-NNN (linked to file), scaffold manifest section for this task
- **Verifier:** task acceptance criteria, relevant REQ-NNN, Builder's handoff note
- **Analyst:** prior requirements version (if revising), Auditor flags document (if incorporating feedback)
- **Auditor:** requirements document being audited (full file), prior audit report (if regression check)
- **Architect:** approved requirements document, any prior ADRs that may be affected
- **Planner:** approved requirements document, architecture overview, prior task plan (if revising)
- **Sentinel:** architecture overview, environment contract, prior Sentinel report (if cycle re-review)
- **DevOps:** architecture deployment model, environment contract (if updating), task plan DevOps section

*Remove the "By agent" guidance block when filling in a real routing instruction — it is a template aid, not part of the output.*

---

## Skills required

*List the skills the receiving agent must read before starting. Link to `.claude/skills/<skill>.md`. Omit section if no skills apply.*

- [`.claude/skills/bash-execution.md`](.claude/skills/bash-execution.md) — always include for Builder and DevOps
- [`.claude/skills/commit-discipline.md`](.claude/skills/commit-discipline.md) — include for Verifier and DevOps tasks that end with a commit
- [`.claude/skills/demo-script-execution.md`](.claude/skills/demo-script-execution.md) — include for Verifier (demo script authorship)
- [`.claude/skills/traceability-links.md`](.claude/skills/traceability-links.md) — include for Orchestrator self-reference when producing routing slips

*Remove skills not applicable to this routing. Remove this section if empty.*

---

## Context

[Any additional context the receiving agent needs that is not captured in the linked documents — prior decisions, known constraints, escalation history relevant to this task. Keep brief. If there is nothing to add, remove this section.]
