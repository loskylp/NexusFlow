# Skill — Graphic Design with Stitch MCP

UI/screen design is handled through the **Stitch MCP tools**. This skill covers the full Designer lifecycle: from generation through Nexus approval to handoff artifacts that the Builder can use directly.

---

## Designer lifecycle

```
Generate → Propose → [Nexus Review] → Revise? → [Nexus Approval] → Finalize → Handoff
```

### 1. Generate
Create screens in Stitch using `generate_screen_from_text`, `edit_screens`, and `generate_variants`.

### 2. Propose
Output a design proposal document at `process/designer/proposal.md` listing every screen with its Stitch ID, title, and the Stitch project link. Open the project in the browser using Playwright so the Nexus can inspect it live — do not ask the Nexus to navigate manually.

```python
# Open Stitch project for Nexus inspection
browser_navigate("https://stitch.withgoogle.com")
# Navigate to the project by its ID if a direct URL is known
```

### 3. Nexus Review ← human checkpoint
The Nexus inspects screens in the open browser. This checkpoint is **non-negotiable** — the Designer cannot self-approve. The Nexus either approves or requests changes.

### 4. Revise (if needed)
Use `edit_screens` or `generate_variants` on flagged screens. Update the proposal document. Re-open in Playwright for re-review.

### 5. Finalize (on approval)
Download all approved screens locally:
- **HTML** via `curl` from `htmlCode.downloadUrl`
- **Screenshot** via `curl` from `screenshot.downloadUrl`
- **DESIGN.md** — extract `designTheme.designMd` from the `list_projects` response and save as `process/designer/DESIGN.md`

Save to `process/designer/`:
```
process/designer/
  DESIGN.md              ← the design system: tokens, typography, components, do's/don'ts
  proposal.md            ← the handoff document with embedded screenshots
  screens/
    landing-page/
      screen.html
      screenshot.png
    sign-up/
      screen.html
      screenshot.png
    workspace/
      screen.html
      screenshot.png
```

Update the proposal document: embed screenshots and link local HTML files.

### 6. Handoff
The Builder reads the proposal document, opens local HTML files directly — no Stitch calls needed at build time. The Builder also reads `DESIGN.md` for any UI component not explicitly covered by a screen: it is the authoritative reference for colors, typography, component patterns, and do's/don'ts.

---

## Tools at a glance

| Tool | Purpose | Returns |
|---|---|---|
| `create_project` | Create a new project container | Project ID (sync) |
| `list_projects` | List all accessible projects | Project list (sync) |
| `list_screens` | List all screens in a project | Screen list (sync) |
| `get_screen` | Retrieve a screen with download URLs | Screen data (sync) |
| `generate_screen_from_text` | Generate a new screen from a prompt | Nothing — **async** |
| `edit_screens` | Edit existing screens | Nothing — **async** |
| `generate_variants` | Generate 1–5 variants of a screen | Full variant data (sync) |

---

## Async behaviour — do not retry

`generate_screen_from_text` and `edit_screens` complete with no output. Generation happens in the background.

- **Do not retry** — duplicates will land silently later
- Call `list_screens` after a short wait to see what appeared
- `generate_variants` is the exception: returns immediately with full screen data including IDs, screenshot URLs, and the sub-prompt used per variant

---

## Downloading artifacts

Both URLs are publicly accessible signed URLs — no authentication required. Download at finalization time, not before (URLs may expire):

```bash
# HTML — the Builder's implementation scaffold
curl -s "<htmlCode.downloadUrl>" -o process/designer/screens/<slug>/screen.html

# Screenshot — embedded in the proposal document
curl -s "<screenshot.downloadUrl>" -o process/designer/screens/<slug>/screenshot.png
```

---

## Proposal document format

`process/designer/proposal.md` is the source of truth for the Nexus review and the Builder handoff.

```markdown
# Design Proposal — <Project Name>

**Stitch Project:** projects/<project-id>
**Status:** [Pending Review | Approved | Revising]

## Screens

### <Screen Title>
- **Stitch ID:** `<screen-id>`
- **HTML:** [screen.html](screens/<slug>/screen.html)

![<Screen Title>](screens/<slug>/screenshot.png)

---
```

Add screenshots only **after Nexus approval** — not during proposal.

---

## Project types

`create_project` always creates a `PROJECT_DESIGN` type. The Stitch UI creates `TEXT_TO_UI_PRO` projects. Generation works in both, but prefer generating into an existing `TEXT_TO_UI_PRO` project retrieved via `list_projects` when available.

---

## Choosing a model (`modelId`)

| Model | Use when |
|---|---|
| `GEMINI_3_1_PRO` | Default — complex layouts, dashboards, production candidates |
| `GEMINI_3_FLASH` | Speed — rapid wireframes, quick iterations |
| `GEMINI_3_PRO` | Deprecated — do not use |

Always specify `modelId` explicitly.

---

## Device types

| Type | Layout behaviour |
|---|---|
| `MOBILE` | Vertical scroll, bottom-tab nav, stacked content, thumb zones |
| `DESKTOP` | Top nav, multi-column grids, horizontal sprawl |
| `TABLET` | Intermediate |
| `AGNOSTIC` | Not device-tied |

Do not mix device types within a project without intent. To translate mobile → desktop, create a new project or prompt explicitly covering: navigation restructure, hero section split, and grid density increase.

**Hidden content tip:** When translating App → Web within a project, Stitch may generate the full layout but clip it to the original frame height. Check `height` in `get_screen` — if it matches the original, content may be hidden. Instruct the Nexus to drag the frame handle down in Stitch to reveal it.

---

## Prompting strategy

### Structure every screen prompt in this order

1. **What it is** — screen name and app context
2. **Layout** — panels, sections, proportions, what goes where
3. **Content** — realistic placeholder content, not lorem ipsum
4. **Visual system** — surface colors (hex), accent, typography pairing, shadow/border rules
5. **Negative rules** — explicitly what to avoid: `"no box borders"`, `"no dark surfaces"`, `"no drop shadows — tonal depth only"`

### Adjectives set the vibe
They influence colors, fonts, and imagery globally. Use deliberately:
- `"editorial"`, `"brutalist"`, `"glassmorphic"`, `"warm and minimal"`, `"clinical precision"`

### Define a visual system once, reference it across all screens
- **Surface hierarchy by hex:** base `#FAFAF7` → panel `#F2F0EB` → card `#FFFFFF`
- **The No-Line Rule:** ban 1px borders explicitly — use tonal background shifts and whitespace only
- **Shadows:** specify blur + opacity: `"ambient 40px blur, 5% opacity warm charcoal"` — default shadows look generic
- **Typography:** name the pairing and where each applies: `"DM Serif Display for headings, DM Sans for all UI chrome"`

### Iterating with `edit_screens`
- Creates a **new screen version** — the original is preserved in the project
- One or two changes per call — easier to evaluate
- Be explicit about removals: `"Remove the right panel entirely. Do not replace it with anything."`
- Reference elements by location: `"primary CTA in the hero section"` not `"the button"`
- If a change misses, rephrase with more specificity — do not retry the same prompt

---

## Using `generate_variants`

Variants are for **exploration and pivoting**, not incremental edits. Use when getting unstuck, exploring layout concepts, or overhauling the vibe in one go.

```
generate_variants(
  projectId,
  selectedScreenIds: ["<screen-id>"],
  prompt: "direction brief",
  variantOptions: {
    variantCount: 1–5,          // default 3
    creativeRange: "REFINE" | "EXPLORE" | "REIMAGINE",
    aspects: ["LAYOUT", "COLOR_SCHEME", "IMAGES", "TEXT_FONT", "TEXT_CONTENT"]
  },
  modelId: "GEMINI_3_1_PRO"
)
```

### Creative range

| Range | Effect |
|---|---|
| `REFINE` | Subtle polish — fonts, spacing, color tweaks. Structure intact. |
| `EXPLORE` | Balanced exploration — layout and theme may shift. Default. |
| `REIMAGINE` | Radical — full restructure, new imagery, theme overhaul. |

### Aspects
Omit to vary everything. Narrow to preserve specific dimensions:
- `["LAYOUT"]` — keeps colors/fonts, explores structure
- `["COLOR_SCHEME"]` — keeps layout, explores palette

### Reading variant output
Each variant includes:
- `id` — use with `edit_screens` or as a base for further generation
- `screenshot.downloadUrl` — rendered preview
- `htmlCode.downloadUrl` — exportable HTML
- `prompt` — the sub-instruction the model used for this variant — read this to understand the direction taken and use it to pursue a direction further

### Iterating on a winner
1. Pick the variant closest to the target — note its `id`
2. Run `generate_variants` again on it with `creativeRange: "REFINE"` to polish
3. Or pass its `id` to `edit_screens` for targeted adjustments

---

## Style word bank

Use these terms in prompts to direct aesthetic precisely. Combine across categories for a distinct visual language.

**Layout:** Bento Grid, Editorial, Swiss Style, Split-Screen
**Texture & Depth:** Glassmorphism, Claymorphism, Skeuomorphic, Grainy/Noise
**Atmosphere:** Brutalist, Cyberpunk, Y2K, Retro-Futurism
**Color:** Duotone, Monochromatic, Pastel Goth, Dark Mode OLED

Example: `"Brutalist layout with Duotone color and Grainy texture"`

---

## DESIGN.md — the design system document

`DESIGN.md` is the design counterpart to `AGENTS.md`. It defines how the project looks and feels in a format both humans and agents can read and enforce.

| File | Read by | Defines |
|---|---|---|
| `AGENTS.md` | Coding agents | How to build the project |
| `DESIGN.md` | Design agents + Builder | How the project should look and feel |

### Where it comes from
It is generated by Stitch automatically and lives in `designTheme.designMd` on every `list_projects` response — no extra tool call needed. Extract and save it at finalization:

```python
projects = list_projects()
design_md = projects[0]["designTheme"]["designMd"]
# write to process/designer/DESIGN.md
```

### What it contains
- Creative north star and design philosophy
- Full color token system with hex values and roles
- Surface hierarchy rules (which surface tier to use where)
- Typography pairing — which font for headlines vs body vs labels
- Component patterns — buttons, inputs, cards, chips
- Explicit do's and don'ts

### Why it matters
- **Consistency across sessions:** feed it back to Stitch in future generations and every new screen inherits the same rules automatically
- **Builder reference:** when implementing a component not explicitly designed in Stitch, DESIGN.md is the authoritative spec — not the HTML files
- **Living artifact:** the Nexus or Analyst can hand-edit it to encode design constraints before generation starts, steering the output before a single screen is generated
- **Can be authored manually:** write it by hand to define exact design preferences upfront — no Stitch generation needed first

---

## What the Builder receives

Each downloaded `screen.html` is a fully production-ready, self-contained file:
- **Tailwind CSS** via CDN with a complete custom config (all design tokens as Tailwind classes)
- **Google Fonts** — exact fonts from the design
- **Semantic HTML** — `<nav>`, `<aside>`, `<section>`, `<header>`
- **Real placeholder content** — not lorem ipsum
- **Interactive states** — hover, active, focus, transitions already coded
- **Design tokens** as Tailwind classes (`bg-surface`, `text-primary`) — swappable by editing the config object

The Builder uses the HTML as a pixel-perfect implementation scaffold — adapting it to the target framework or using it as-is.

---

## Common failure patterns

| Symptom | Fix |
|---|---|
| Screen generated but not in `list_screens` | Wait — it's async. Call `list_screens` again |
| `edit_screens` seems to do nothing | It created a new screen — check `list_screens` for a new entry |
| Panel removed but something fills its space | Add `"Do not replace it with anything"` to the prompt |
| Design looks generic despite detailed prompt | Add a negative rules section to the prompt |
| Editor panel shows rendered preview, not raw source | Explicitly describe visible raw Markdown syntax, line numbers, blinking cursor |
| Too many duplicate screens from retried calls | Expected — delete duplicates in the Stitch UI; identify canonical by title or `list_screens` order |
| Download URL returns 403 | URL has expired — call `get_screen` again to get a fresh URL, then download immediately |
