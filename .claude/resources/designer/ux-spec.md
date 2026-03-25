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

# UX Specification — [Project Name]
**Version:** [N] | **Date:** [date]
**Delivery Channel:** [Web App | Mobile Native (iOS/Android) | Desktop | CLI with menus / TUI]
**Artifact Weight:** [Sketch | Draft | Blueprint | Spec]

## Personas
[Casual: omitted — user roles from Brief apply directly]
[Commercial+: one entry per user role]

### [Role Name]
**Based on:** [Brief — User Roles: role name]
**Goals:** [What they are trying to accomplish]
**Behaviors:** [How they typically approach this kind of task]
**Frustrations:** [What gets in their way]
**Context of use:** [Where, when, on what device]
**Research basis:** [Casual: assumed | Commercial: informed assumption | Critical+: cite source or study]

## Information Architecture

### Screen Inventory
| Screen | Purpose | Accessible to |
|---|---|---|
| [screen name] | [what it does] | [user roles] |

### Navigation Structure
[Mermaid diagram or structured description of how screens connect and what navigation
elements are always present vs. contextual]

## User Flows

[One flow per role per key scenario. Use Mermaid flowchart.]

### Flow: [Role] — [Scenario]
```mermaid
flowchart TD
    ...
```
**Precondition:** [What must be true before this flow starts]
**Postcondition:** [What is true when this flow ends successfully]
**Error paths:** [What can go wrong and where the flow goes]

## Wireframes

[Fidelity per profile: Sketch = described in prose or rough ASCII, Commercial = low-fi, Critical = mid-fi, Vital = high-fi]
[For TUI channels: all fidelity levels use ASCII layout notation — this is not a limitation, it is the correct medium]

### [Screen / View / Panel name]
**Purpose:** [What this screen or panel is for]
**User roles with access:** [which roles see this]

**Layout zones (GUI):**
- [Zone name]: [content/components in this zone, hierarchy note]

**Layout zones (TUI — ASCII notation):**
```
┌──────────────────────┬──────────────────────┐
│ [Panel name]         │ [Panel name]         │
│ [content description]│ [content description]│
│                      │                      │
└──────────────────────┴──────────────────────┘
[ F1:Help   F5:Action   F9:Menu   F10:Quit    ]
```
[Describe panel roles, active panel indicator, selection highlight]

**States:**
- Default: [description]
- Empty: [what the user sees when there is no data]
- Loading: [how loading is communicated — spinner character, progress bar, status line message]
- Error: [how errors are surfaced — status line, modal dialog, inline — and what the user can do]

**Key interactions:**
- [Element or key]: [what happens]

**UX notes:**
- [Fitts's Law / Hick's Law / cognitive load considerations for GUI — or — key binding conflicts, information density decisions for TUI]

## Interaction Specification

### Patterns in use
| Pattern | Where used | Rationale |
|---|---|---|
| [pattern name] | [screens] | [why this pattern — Jakob's Law, convention, etc.] |

### Mode system (TUI channels only)
**Model:** [Modeless (Norton Commander style — all keys active at all times) | Modal (vim style — key meaning depends on current mode)]

[If modal: document each mode, how to enter it, how to exit it, and what keys are active within it]

| Mode | Entry key | Exit key | Purpose |
|---|---|---|---|
| [mode name] | [key] | [key] | [what the user does in this mode] |

### Key binding map (TUI channels only)
[Document every key binding. Conflicts must be resolved here, not during implementation.]

| Context | Key | Action | Notes |
|---|---|---|---|
| Global | F1 | Help | Always available |
| Global | F10 / q | Quit | With confirmation if unsaved state |
| [Panel/view name] | Arrow keys | Move selection | |
| [Panel/view name] | Enter | Open / confirm | |
| [Panel/view name] | [key] | [action] | |

### Transitions and feedback
[GUI: How the system communicates state changes, confirmations, and errors across the product]
[TUI: Status line content and position, dialog box usage, inline vs. modal error reporting, progress indicators (spinner chars, progress bar ASCII)]

## Visual Specification
[Casual: omitted | Commercial: partial | Critical+: full]

### Hierarchy system (GUI)
- **Size scale:** [heading levels and relative sizes]
- **Weight usage:** [when bold/medium/regular is used]
- **Color as signal:** [primary action, secondary action, destructive, disabled, error, success]

### Hierarchy system (TUI)
- **Attributes:** [bold = primary/active, dim = secondary/inactive, reverse-video = selection highlight, underline = focused field]
- **Color as signal:** [map terminal color names to semantic roles — e.g. red = error/destructive, green = success, yellow = warning, cyan = active panel border]
- **Border weight:** [single-line borders for panels, double-line or bold for active/focused panel]
- **Color depth target:** [8 colors (safest) | 16 | 256 | true color — per Architect's terminal target decision]

### Spacing and grid (GUI)
[Grid columns, gutter, margin, and spacing scale — reference Architect's framework decision
for implementation tokens]

### Spacing and grid (TUI)
[Column widths, panel proportions, status line position (top/bottom), menu bar position.
All values in character cells, not pixels.]

### Accessibility notes
[GUI: Contrast ratios, focus order, ARIA roles for non-standard components, touch target minimums for mobile]
[TUI: Keyboard-only operability (no flow requires a mouse), visible focus indicator at all times,
color-blind safe palette (do not use color as the only signal — pair with bold or a symbol)]

## Design Hypotheses
[Casual: omitted | Commercial+: one per significant design decision]

| Decision | Hypothesis | Signal |
|---|---|---|
| [what was decided] | We believe this will [outcome] for [role] | We will know when [measurable] |
