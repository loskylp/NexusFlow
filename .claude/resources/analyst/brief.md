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

# Brief — [Project Name]
**Version:** [N]
**Date:** [date]
**Artifact Weight:** [Sketch | Draft | Blueprint | Spec]

## Problem Statement
[What problem does this system solve? For whom? What is the cost of not solving it?]

## Context and Ground Truths
[What is true about the world this system operates in, independent of the system itself?
Business rules, constraints, existing systems, regulatory context, organizational facts.]

## Scope and Boundaries
**In scope:** [What the system is responsible for]
**Out of scope:** [What is explicitly excluded — name it, don't leave it implied]
**Adjacent (conscious exclusion):** [Things that touch the system but are owned elsewhere —
integrations, upstream data sources, downstream consumers]

## Delivery Channel
**Channel:** [One of: Web App | Mobile Native (iOS / Android) | Desktop | CLI with menus / TUI (ncurses style) | CLI (commands and flags only) | REST API / Service | GraphQL API | Hybrid — specify]
**Decision status:** [Nexus-stated | Nexus-confirmed | OPEN — blocking]
**Implications:** [What this channel means for the project — e.g. "Web App: UX Design phase required before architecture. UI framework decision belongs to Architect." or "REST API: no UX phase. API surface design belongs to Architect. Developer experience is the UX concern."]

## Stakeholders
| Role | Relationship to system | Needs | Authority over requirements |
|---|---|---|---|
| [role] | [affected / approves / funds] | [what they need] | [yes / no / partial] |

## User Roles
| Role | Description | Goals | Permissions needed |
|---|---|---|---|
| [role name] | [who this is] | [what they want to accomplish] | [what actions they need] |

## Domain Model
[Profile-dependent — see Profile Variants. Captures the key concepts in the problem domain,
their relationships, and the shared vocabulary for the project.]

### Key Concepts
| Term | Definition | Relationships |
|---|---|---|
| [concept] | [what it means in this domain] | [relates to: ...] |

### Domain Invariants
[Rules that are always true in this domain, regardless of what the system does.
Example: "An order cannot be fulfilled if any line item is out of stock."]

## Open Context Questions
[Things the Analyst still needs to understand. Will shrink each cycle.]
