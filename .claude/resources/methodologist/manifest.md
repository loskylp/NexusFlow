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

# Methodology Manifest
**Version:** v1 | **Date:** [date] | **Project:** [project name]
**Profile:** [Casual | Commercial | Critical | Vital]
**Artifact Weight:** [Sketch | Draft | Blueprint | Spec]

---

*Distribution template. On project start, the Methodologist copies this to `process/methodologist/manifest-v1.md` and fills it in. Subsequent versions are written as `manifest-v2.md`, `manifest-v3.md`, etc. — all in `process/methodologist/`. The current Manifest is always the highest-numbered file in that directory. Do not edit directly; invoke the Methodologist to produce or update.*

---

## Changelog
- v1: Initial configuration — [date]

## Profile Rationale
[One to three sentences: why this profile was assigned based on the Nexus's intake answers.]

## Agents

| Agent | Status | Notes |
|---|---|---|
| Methodologist | Active | |
| Orchestrator | Active | |
| Analyst | Active | |
| Auditor | Active | |
| Architect | Active | |
| Designer | [Active \| Skipped] | [reason if skipped] |
| Scaffolder | [Active \| Skipped] | [Active: invoked when ≥3 Builder tasks per cycle and profile is not Casual] |
| Planner | Active | |
| Builder | Active | |
| Verifier | Active | |
| Sentinel | [Active \| Skipped] | [Skipped at Casual — Builder applies common sense] |
| DevOps | [Active \| Skipped] | [Skipped at Casual — Builder absorbs infrastructure tasks] |
| Scribe | [Active \| Skipped] | [Skipped at Casual — Builder maintains README] |

### Acceptance criteria for skipped agents
[For each skipped or combined agent: what alternative mechanism provides equivalent coverage, and what the Nexus verifies instead. Remove section if all agents are active.]

## Documentation Requirements

| Agent | Produces | Depth |
|---|---|---|
| Analyst | Brief + Requirements List | [Sketch: informal / Blueprint: full DoD per REQ] |
| Architect | Architecture artifacts | [Sketch: system metaphor / Blueprint: ADRs + fitness functions] |
| Planner | Task Plan + Release Map | [Sketch: task list / Blueprint: full plan with risk matrix] |
| Verifier | Verification Reports + Demo Scripts | [Sketch: checklist / Blueprint: full structured report] |
| [others as needed] | | |

## Gate Configuration

| Gate | Status | Mode |
|---|---|---|
| Requirements Gate | Active | [Lightweight \| Formal] |
| Architecture Gate | Active | [Lightweight \| Formal] |
| Plan Gate | Active | [Lightweight \| Formal] |
| Demo Sign-off | Active | [Explore running software + retrospective question \| Formal sign-off with security review] |
| Go-Live | Active | [Continuous Deployment \| Continuous Delivery \| Business decision] |

## Iteration Model

**Max iterations per task:** [N]
**Convergence signal:** [N] consecutive iterations with non-decreasing failure count triggers escalation.
**Cycle scope:** Planner-defined — tasks are grouped into demonstrable increments at the Plan Gate. Each cycle ends with a Demo Sign-off. The Orchestrator executes only the tasks in the current cycle before moving to Demo Sign-off.
**CD philosophy:** [Continuous Deployment | Continuous Delivery | Business decision]

## Verifier Task Modes

*Remove this section at Casual — single verification mode only.*

The Verifier operates in one of two modes depending on whether DevOps Phase 2 (staging) is available. The Orchestrator declares the current mode at each Verifier invocation.

| Mode | When active | Test layers available |
|---|---|---|
| **Pre-staging** | Before DevOps Phase 2 completes (first Builder task only) | Unit tests + integration tests + acceptance tests. No system tests. |
| **Full** | After DevOps Phase 2 completes (all subsequent tasks) | All layers: unit, integration, acceptance, and system tests against staging. |

**Current mode:** [Pre-staging | Full — updated by Orchestrator as DevOps phases complete]

## Infrastructure Preconditions

**Before Builder tasks begin:**
[What CI pipeline, dev environment, and Environment Contract must be in place. At Casual: often none — Builder sets up the dev environment directly.]

**Before each Demo Sign-off:**
[Staging must be reachable at its health endpoint and the application must be running. DevOps confirms staging live before the Orchestrator opens the Demo Sign-off gate. At Casual: not applicable.]

## Deployment Workflow

*Remove this section at Casual — no DevOps agent, no pipeline.*

**Model:** [Tag-based | Branch-based — choose based on project scale and team preference. Tag-based is the default for single-branch projects at Commercial.]

### Tag-based model (default at Commercial)

Three triggers, three owners:

| Trigger | Who pushes | Pipeline action |
|---|---|---|
| Commit push to `main` (per task) | **Verifier** — after task PASS + local lint | Build + full regression test suite |
| Demo tag (e.g. `demo/v[cycle].[attempt]`) | **DevOps** — signalled by Orchestrator after all cycle tasks verified and CI green | Build + regression + Docker image build + push to staging |
| Release tag (e.g. `release/v[major].[minor]`) | **DevOps** — signalled by Orchestrator after Go-Live approved by Nexus | Retag the staging-validated Docker image to prod — no rebuild |

**Tag naming convention:**
- Demo tags: `demo/v[cycle].[attempt]` — e.g. `demo/v1.0` for Cycle 1 first attempt, `demo/v1.1` for a second attempt after a rejected demo
- Release tags: `release/v[major].[minor]` — e.g. `release/v1.0`; minor increments on cycle releases, major on breaking changes

**Image promotion rule:** the Docker image deployed to production is the exact image that passed Demo Sign-off — retag only, never rebuild from source. This guarantees the Nexus approved exactly what goes to prod.

### Branch-based model (for multi-branch workflows)

[Describe branch strategy, merge gates, and who triggers each environment promotion. Use this model at Critical and above when parallel feature development requires isolation.]

## Provisional Assumptions

[Assumptions made due to incomplete intake. Each is provisional and subject to revision. Remove section if intake was complete.]
