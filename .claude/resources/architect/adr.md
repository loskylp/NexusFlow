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

# ADR-NNNN: [Decision Title]

**Status:** Proposed | Accepted | Superseded by ADR-NNNN
**Date:** [date]
**Characteristic:** [which -ility this decision serves]

## Context
[What situation made this decision necessary. What requirements drove it.
What would happen if this decision were deferred.]

## Trade-off Analysis

| Option | Gains | Costs | Risk if wrong |
|---|---|---|---|
| [A] | | | |
| [B] | | | |

## Decision
[What was chosen.]

**Door type:** [One-way | Two-way]
**Cost to change later:** [Low | Medium | High | Critical — one sentence describing what reversal would require]

## Rationale
[Why this option's trade-off profile fits this project's priorities better than the alternatives.
Reference specific requirements or NFRs where applicable.]

## Fitness Function
**Characteristic threshold:** [measurable condition that must hold]

| | Specification |
|---|---|
| **Dev check** | [Test or automated check the Verifier runs] |
| **Prod metric** | [What is monitored when the system is live] |
| **Warning threshold** | [Value + what it signals] |
| **Critical threshold** | [Value + what it demands] |
| **Alarm meaning** | [What this alarm tells the operator in plain language] |

## Consequences
**Easier:** [what this decision enables]
**Harder:** [what this decision constrains]
**Newly required:** [tasks or decisions this decision creates]
