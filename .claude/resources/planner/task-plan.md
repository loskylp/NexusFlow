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

# Task Plan — [Project Name]
**Version:** [N] | **Date:** [date]
**Requirements Version:** [N] | **Architecture Version:** [N]
**Artifact Weight:** [Sketch | Draft | Blueprint | Spec]

## Architecture Constraints
[One-line summary of architectural constraints from the Architect that affect task ordering.
For Casual: repeat the metaphor. For Commercial+: reference the Overview or ADRs.]

## Priority 1 — Do This Cycle
*High risk + high value. Dependencies respected within group.*

### TASK-[NNN]: [Short title]
**Requirement(s):** [REQ-NNN]
**Description:** [What must be done. Not how.]
**Acceptance Criteria:**
- [Specific, testable condition]
**Depends on:** [TASK-NNN | none]
**Risk:** [H/M/L — cite the rubric criterion: e.g. "H — one-way door, no spike yet"]
**Value:** [H/M/L — cite the rubric criterion: e.g. "H — Must Have for MVP, on walking skeleton critical path"]
**Status:** [Pending | In Progress | Done | Superseded]

### SPIKE-[NNN]: [Short title of unknown]
**Resolves:** [The unknown, as stated by the Architect]
**Needed before:** [TASK-NNN, TASK-NNN]
**Acceptance Criterion:** [The specific question that defines done — copied from Architect]
**Finding goes to:** [Architect | Planner — copied from Architect's spike spec]
**Risk:** High
**Value:** [Derived from the value of the blocked tasks]
**Status:** [Pending | In Progress | Complete]

## Priority 2 — Do This Cycle
*Low risk + high value. Quick wins.*
[tasks]

## Priority 3 — Next Cycle
*High risk + low value. Spike first, then reassess.*
[tasks]

## Deferred — Below Cut Line
*Low risk + low value. Nexus decides: defer or cut.*

| Task | What is lost if cut | Cost to include |
|---|---|---|
| [TASK-NNN: title] | [impact] | [rough sizing] |

## Open Technical Questions
[Unknowns not yet resolved by a spike — for Nexus awareness]

## Revision Delta (Plan vN+ only)
*Present only on revised plans. Omit on the initial plan.*

### New Tasks
[Tasks created from new requirements this revision]

### Revised Tasks
| Task | Version | Requirement revised | What changed in acceptance criteria |
|---|---|---|---|
| TASK-NNN | v1 → v2 | REQ-NNN v1 → v2 | [summary of criteria change] |

### Superseded Tasks
| Task | Original Req | Revised Req | Why superseded | Replacement task |
|---|---|---|---|---|
| TASK-NNN | REQ-NNN v1 | REQ-NNN v2 | [reason existing implementation cannot be extended] | TASK-NNN |

### Unaffected Tasks (checked)
*Tasks traced to revised requirements but confirmed unaffected.*
[TASK-NNN, TASK-NNN — acceptance criteria still satisfy revised requirement]
