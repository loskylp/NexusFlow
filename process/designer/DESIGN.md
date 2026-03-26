# NexusFlow Design System

## Philosophy
Operational clarity over decoration. Every pixel serves a monitoring or action purpose. Dense information display with clear visual hierarchy. The system manages background tasks for production applications -- the interface must inspire confidence and trust.

## Color Tokens
- **Primary:** Indigo #4F46E5 -- primary actions, active states, navigation highlights
- **Surface Base:** #FAFAFA -- page background
- **Surface Panel:** #FFFFFF -- cards, panels, elevated content
- **Surface Subtle:** #F1F5F9 -- table row alternation, input backgrounds
- **Border:** #E2E8F0 -- panel borders, dividers
- **Text Primary:** #0F172A -- headings, primary content
- **Text Secondary:** #64748B -- labels, secondary info, timestamps
- **Text Tertiary:** #94A3B8 -- placeholders, disabled text

## Semantic Colors
- **Success / Online / Completed:** #16A34A (green-600)
- **Warning / Queued / Assigned:** #D97706 (amber-600)
- **Error / Failed / Down:** #DC2626 (red-600)
- **Info / Running:** #2563EB (blue-600)
- **Cancelled:** #64748B (slate-500)
- **Submitted:** #8B5CF6 (violet-500)

## Task State Color Map
- submitted: violet-500 #8B5CF6
- queued: amber-600 #D97706
- assigned: amber-500 #F59E0B
- running: blue-600 #2563EB (with pulse animation)
- completed: green-600 #16A34A
- failed: red-600 #DC2626
- cancelled: slate-500 #64748B

## Worker Status Color Map
- online: green-600 #16A34A (solid dot)
- down: red-600 #DC2626 (hollow dot or X)

## Typography
- Headlines: Inter SemiBold, 24/20/18/16px
- Body: Inter Regular, 14px, line-height 1.5
- Labels/Captions: IBM Plex Sans, 12px, uppercase tracking for section labels
- Monospace (logs, code, JSON): JetBrains Mono or system monospace, 13px

## Spacing Scale
4px base unit: 4, 8, 12, 16, 24, 32, 48, 64

## Component Patterns

### Status Badge
Small rounded pill with semantic background color at 10% opacity and full-color text. Always pair color with a text label -- never color alone.

### Data Table
Full-width, alternating row backgrounds (#FAFAFA / #FFFFFF). Sticky header. Sortable columns indicated by caret icon. Row hover: subtle blue-50 background.

### Sidebar Navigation
Fixed 240px sidebar. Dark slate-900 background. White text. Active item: indigo-500 left border + indigo-50 background on the text. Icons + labels. Collapsible to icon-only 64px width.

### Cards
White background, 1px border slate-200, 8px radius, 16px padding. No box shadows -- use border only for elevation.

### Real-time Indicators
- Streaming data: pulsing dot next to title
- SSE connected: green dot in status bar
- SSE disconnected: red dot with 'Reconnecting...' text

### Pipeline Builder Canvas
Light gray #F8FAFC background with subtle dot grid pattern. Pipeline nodes are cards with distinct header colors per phase: DataSource (blue-100 header), Process (purple-100 header), Sink (green-100 header). Connection lines: 2px slate-400 with directional arrow.

### Toast Notifications
Fixed position bottom-right. White background, 1px border, 8px radius. Auto-dismiss after 5s (8s for errors). Left accent border: green for success, red for error, amber for warning.

### Confirmation Dialog
Modal overlay with semi-transparent slate-900 background. Centered white card, 480px max-width. Title, description, Cancel (secondary) and Confirm (primary or destructive) buttons. Focus trapped within dialog. Escape key closes.

### Form Inputs
Full-width within container. 40px height. #F1F5F9 background, 1px #E2E8F0 border, 8px radius. Label above in IBM Plex Sans 12px uppercase. Focus: white background, 2px indigo-500 ring. Error: red-500 border, red-50 background, red-600 error text below.

## Do's
- Always show task state as badge with both color and text label
- Use monospace font for all log output, task IDs, and JSON data
- Show real-time connection status in the bottom status bar
- Use skeleton loaders during initial data fetch
- Show empty state illustrations with actionable text when no data
- Include confirmation dialogs on all destructive actions
- Maintain consistent sidebar navigation across all authenticated views
- Use phase-specific colors consistently: DataSource=blue, Process=purple, Sink=green

## Don'ts
- Never use color as the only indicator of state -- always pair with text
- No box shadows -- use 1px borders for elevation
- No rounded-full on cards or panels -- only on badges and avatars
- No decorative animations -- only functional transitions (state changes, loading)
- Never hide critical operational information behind progressive disclosure
- No lorem ipsum -- use realistic placeholder data in all components
- No gradient backgrounds or decorative patterns on functional surfaces
