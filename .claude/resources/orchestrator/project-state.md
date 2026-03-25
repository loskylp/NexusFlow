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

# Project State
**Manifest version:** v[N] | **Profile:** [Casual | Commercial | Critical | Vital]
**Current phase:** [INGESTION | DECOMPOSITION | EXECUTION | VERIFICATION | DEMO SIGN-OFF | GO-LIVE | CLOSED]
**Current cycle:** [N]
**Last updated:** [date]

---

*Distribution template. On project start, copy this to `process/orchestrator/project-state.md`. The Orchestrator overwrites it in place before and after every agent handoff. Git history is the audit trail. Do not edit directly; invoke the Orchestrator to update.*

---

## Where We Are

[One sentence: what is happening right now, or what is waiting for Nexus action.]

## Active Work

**Agent in control:** [Agent name | NEXUS]
**Current task:** [What the agent is doing, or what decision the Nexus must make]
**Waiting for:** [If NEXUS: the specific approval or decision needed to proceed. If agent: expected output.]

---

## Cycle [N] — Task Status

| Task | Status | Iterations | Verifier |
|---|---|---|---|
| TASK-NNN: [title] | [PENDING \| IN PROGRESS \| COMPLETE \| BLOCKED] | [N of max N] | [— \| PASS \| FAIL \| PARTIAL] |

**Cycle summary:**
- Tasks complete: [N] of [N]
- Requirements satisfied this cycle: [N] of [N]
- Sentinel: [Not invoked | PASS | PENDING | BLOCKED — N Critical/High findings]

---

## Nexus Gate Log

| Gate | Date | Decision | Notes |
|---|---|---|---|
| Requirements Gate | — | — | |
| Architecture Gate | — | — | |
| Plan Gate | — | — | |
| Demo Sign-off — Cycle 1 | — | — | |
| Go-Live — v[N.N.N] | — | — | |

---

## Pending Decisions

NONE

---

## Iterate Loop State

NONE — not currently in an iterate loop.

<!-- When in an iterate loop, replace the line above with:
**Task:** TASK-NNN
**Iteration:** [N] of [max per Manifest]
**Failure counts per iteration:** [iter 1: N, iter 2: N, ...]
**Convergence status:** [Progressing | Stalled — N consecutive non-decreasing]
-->

---

## Process Metrics — Cycle [N]

*Commercial and above. At Casual, omit this section.*

| Metric | Value |
|---|---|
| Auditor passes — requirements | [N] |
| Auditor passes — architecture | [N] |
| Gate rejections this cycle | [N] ([which gates]) |
| Tasks completed | [N] of [N] planned |
| Average iterations to PASS | [N.N] |
| Tasks that hit max iterations | [N] |
| Escalations to Nexus | [N] |
| Backward cascade triggered | [Yes / No] |

---

## Standing Routing Rules (Cycle [N])

NONE

---

## All Nexus Decisions (Complete)

| Decision | Date | Outcome |
|---|---|---|
| [e.g. Ratify Manifest v1] | — | — |
| [e.g. Requirements Gate — Cycle 1] | — | — |
