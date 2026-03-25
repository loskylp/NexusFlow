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

# Audit Report — [Project Name]
**Requirements Version Audited:** [N]
**Date:** [date]
**Artifact Weight:** [Sketch | Draft | Blueprint | Spec]
**Result:** [PASS | PASS WITH DEFERRALS | ISSUES FOUND]

## Summary
[N] requirements audited. [N] passed. [N] blocking issues found: [N] contradictions, [N] gaps, [N] ambiguous, [N] untraced, [N] regressions. [N] deferred items tracked (non-blocking).

## Blocking Issues

### AUDIT-[NNN]: [FLAG TYPE] — [Short description]
**Flag:** [CONTRADICTION | GAP | AMBIGUOUS | UNTRACED | REGRESSION | UNCOVERED | INCONSISTENCY | UNGROUNDED | INADEQUATE]
**Requirements involved:** [REQ-NNN, REQ-NNN]
**Description:** [Precise description of the issue]
**Resolution needed:** [What must happen to resolve this: Nexus decision / Analyst clarification / requirement revision]
**Nexus question (if applicable):** [The exact question to ask the Nexus, if domain knowledge is required]

[repeat for each blocking issue]

## Deferred Items (non-blocking)

### AUDIT-[NNN]: DEFERRED — [Short description]
**Flag:** DEFERRED
**Brief reference:** [Section of Brief that mentions this need]
**What is deferred:** [The specific need or decision left unaddressed]
**Why deferral is acceptable:** [Low risk / low value / dependency not yet available / Architect deferred decision — cite source]
**Resolve by:** [Before Gate 2 / before execution of TASK-NNN / before Release N planning / when demo feedback requests it]

[repeat for each deferred item]

## Passed Requirements
[REQ-NNN, REQ-NNN, ...] — all cleared all five checks.

## Recommendation
[PASS TO NEXUS CHECK | RETURN TO ANALYST WITH NEXUS INPUT | RETURN TO ANALYST FOR REVISION]
