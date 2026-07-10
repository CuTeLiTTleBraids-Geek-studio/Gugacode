# AI Workspace Redesign Design

**Date:** 2026-07-10

**Status:** Approved for implementation planning

## Objective

Turn the independent AI window into a unified AI workspace with a wider, horizontally resizable sidebar, permanently visible conversation history, dedicated Skills and Automation areas, AI-only settings moved out of the editor settings page, and a terminal that docks only on the right. Fix the editor activity-bar AI active state so its highlight disappears when the AI window is hidden. Polish the UI using the Apple and Claude design references with restrained motion and no decorative glow.

## Scope

The AI workspace owns these capabilities:

- AI provider and model configuration
- Agent configuration
- MCP tools
- Skills
- Persona
- Plans, goals, workflows, and automation
- Model permissions and usage controls
- Diff confirmation and rollback configuration
- AI personalization
- Prompt and preset management
- Computer Use
- IM integrations
- The “open AI window on startup” preference
- AI-window-specific theme selection

The editor settings page retains only general editor concerns:

- General settings unrelated to AI-window startup
- Editor behavior
- Terminal settings
- Keyboard shortcuts
- Main-editor appearance
- Profiles

The existing `/ai-window` independent Wails window remains the primary AI surface. The legacy `/ai` route may remain as a compatibility route, but it must not introduce a second divergent AI information architecture.

## Chosen Approach

Use one persistent AI workspace shell instead of extending the current icon rail plus temporary drawers.

Rejected alternatives:

1. Keep the current 48px rail and only widen drawers. This cannot satisfy a permanently visible conversation list and leaves feature discovery dependent on tooltips.
2. Move the AI experience back into the editor layout. This reduces space for AI configuration and prevents a clean right-docked terminal inside the AI window.

The chosen shell has three regions:

```text
┌──────────── resizable AI sidebar ────────────┬──────── main workspace ────────┬── optional terminal ──┐
│ AI Assistant / Skills / Automation / Settings│ Chat or selected feature page │ right-docked only      │
│ Rollback and concise feature descriptions    │                               │ horizontally resizable │
│ permanent new/search/filter/conversation list│                               │                       │
└──────────────────────────────────────────────┴───────────────────────────────┴───────────────────────┘
```

## Layout and Interaction

### Resizable Sidebar

- Default width: 288px.
- Minimum width: 260px.
- Maximum width: 380px.
- A resize handle sits on the sidebar’s right edge.
- Dragging left makes the sidebar narrower; dragging right makes it wider.
- Width is persisted for the AI window and restored on the next launch.
- Keyboard accessibility is provided through a focusable separator using arrow keys in 8px increments, with Home and End moving to the minimum and maximum.
- At window widths below 760px, the sidebar uses a compact 260px layout and the terminal cannot consume more than 45% of the remaining workspace.

### Sidebar Navigation

Each primary item displays an icon, a short title, and one concise explanation:

- **AI Assistant** — “Chat, inspect code, and apply changes.”
- **Skills** — “Manage reusable AI instructions and tools.”
- **Automation** — “Run plans, goals, and workflows.”
- **AI Settings** — “Configure models, tools, permissions, and integrations.”
- **Smart Rollback** — “Review snapshots and restore workspace state.”

The label “gugacode AI chat” / “gugacode AI 对话” is removed. “AI Assistant” is the canonical name.

The selected item uses a subtle filled state and a small edge indicator. It does not use a glow or large shadow.

### Permanent Conversation Area

The conversation area is always part of the sidebar below the primary navigation. It contains:

- New conversation action
- Search
- Favorite and tag filters
- Scrollable conversation list
- Trash access

Conversation selection never closes or replaces the sidebar. Rename, favorite, tagging, grouping, export, trash, restore, and drag ordering remain available. The conversation list is visible while visiting Skills, Automation, AI Settings, and Rollback so the user can return to a conversation with one click.

### Main Workspace

The main workspace switches between focused views without replacing the outer shell:

- Assistant chat and composer
- Skills management
- Automation hub
- AI settings center
- Snapshot rollback timeline

The current chat toolbar continues to provide workspace, MCP, selected Skills, and model context. Dedicated Skills and Settings pages manage the underlying resources; toolbar controls only select resources for the active conversation.

### Right-Docked Terminal

- Opening Terminal creates a panel on the right side of the AI workspace.
- Terminal never replaces the chat, Skills, Automation, Settings, or Rollback view.
- Default width: 38% of available main-workspace width, with a practical default near 440px.
- Minimum width: 340px.
- Maximum width: 55% of the available content width.
- The left edge is draggable and keyboard-resizable.
- Closing Terminal returns all width to the main workspace.
- Opening Terminal preserves the current AI view and input draft.
- Existing terminal sessions and tabs are reused; no second terminal state store is created.

## Feature Pages

### Skills

The Skills sidebar item opens the existing Skills management experience in the main workspace. It supports listing, viewing, creating, editing, enabling, disabling, and approving skills. The conversation composer continues to reference the same skills store, so changes are immediately available to the active conversation.

### Automation

Automation is a top-level hub with three internal tabs:

- Plans
- Goals
- Workflows

The existing Plan, Goal, and Workflow components and stores remain the source of truth. The hub provides a short overview, current running state, and direct access to each management surface. It does not duplicate workflow data.

### AI Settings

AI Settings uses grouped secondary navigation inside the main workspace:

1. **Models and behavior** — provider/model configuration, Agent, Persona, model permissions.
2. **Context and tools** — MCP, Skills, prompts, presets.
3. **Execution and safety** — Computer Use, Diff, Rollback configuration.
4. **Integrations** — IM and personalization.
5. **Window** — startup behavior, always-on-top, sidebar width, terminal width, and AI-window theme.

Skills appears both as a top-level workspace and as a direct settings link. Both render the same component and state rather than separate implementations.

## Independent AI Window Themes

### Theme Choices

The AI window exposes exactly five theme choices:

1. Apple Dark
2. Apple Light
3. Claude Dark
4. Claude Light
5. Follow System

“Follow System” always uses the Apple design language. It resolves only between Apple Dark and Apple Light based on `prefers-color-scheme`. It never resolves to a Claude theme.

### Independence from the Editor

- Add a separately persisted `aiWindowTheme` setting with values `apple-dark`, `apple-light`, `claude-dark`, `claude-light`, or `system`.
- Changing the AI-window theme does not change the main editor’s `theme` or `designLanguage` settings.
- The independent AI webview applies its theme locally after settings load.
- Main-editor appearance remains controlled by the existing Appearance page.
- Existing Apple/Claude CSS tokens are reused. New AI-shell tokens may map to those base tokens but must not duplicate the full global palette.

### Visual Direction

Apple themes provide:

- Low-chrome layout and clear hierarchy
- SF/system font stack
- White/parchment or neutral near-black surfaces
- Action blue for interactive focus
- Hairline dividers and almost no card shadow

Claude themes provide:

- Warm cream or warm charcoal surfaces
- Coral as the restrained primary accent
- Editorial display type only for section headings; dense controls stay in the sans stack
- Dark code and terminal surfaces with warm neutral framing

Both design languages share the same component geometry, focus behavior, spacing scale, and accessibility rules. Theme changes alter tokens, not component structure.

## Motion and Effects

Motion is functional and restrained:

- Sidebar selection: 150ms color and indicator transition.
- Workspace view change: 180–220ms fade with a maximum 6px vertical offset.
- Terminal open/close: 220–250ms width and opacity transition.
- Dropdown and context menu: 150ms fade/scale from 0.98.
- Message insertion: 180ms fade/translate for newly appended messages only.
- Button press: subtle `scale(0.97)` where appropriate.

No animated gradients, persistent glow, neon borders, or large blurred shadows are introduced. `prefers-reduced-motion: reduce` removes transforms and collapses transitions to near-instant state changes.

## AI Activity-Bar State Fix

### Root Cause

`ToggleAIWindow()` hides an existing window, while `IsAIWindowOpen()` reports whether a window object exists. A hidden window therefore continues to report “open,” leaving the editor activity-bar AI item highlighted.

### Design

- Preserve `IsAIWindowOpen()` as an existence check for compatibility.
- Add a visibility-aware backend query, `IsAIWindowVisible()`.
- The editor ActivityBar uses visibility, not existence, for its active style and indicator.
- After a toggle, the frontend immediately refreshes visibility and does not call the open fallback merely because the window became hidden.
- Polling may remain as a defensive synchronization fallback, but visibility is also refreshed on click and window lifecycle events where available.
- Hiding, closing, and reopening the AI window each produce distinct, testable states.

## State and Persistence

Persist these AI-window preferences through the existing settings service:

- `aiWindowTheme`
- `aiSidebarWidth`
- `aiTerminalWidth`
- Existing `openAIWindowOnStartup`
- Existing AI always-on-top preference if persistence is not already durable across application restarts

Bounds are enforced when reading persisted widths so values from older window sizes cannot make panels inaccessible.

The AI workspace uses one navigation state and one terminal visibility state. Navigating between feature pages does not reset the active conversation, selected context, terminal sessions, or composer draft.

## Error Handling

- Failed settings load falls back to Apple Dark, 288px sidebar, and a 440px terminal.
- Invalid persisted theme values fall back to `system` only when the stored value explicitly requested system; otherwise they fall back to Apple Dark.
- Failed conversation loading leaves the list visible and reports an inline or notification error without switching views.
- Failed terminal initialization leaves the right panel open with an actionable error and retry control.
- Failed AI-window visibility queries clear the active highlight rather than displaying stale active state.
- Settings components retain their existing backend error handling after migration.

## Accessibility

- Sidebar and terminal resize handles use `role="separator"`, orientation, value attributes, and keyboard controls.
- Primary sidebar items remain real buttons with `aria-current` or `aria-pressed` as appropriate.
- Theme cards form a radiogroup with exactly one selected option.
- Focus rings use the active design language’s focus token and remain visible in all four explicit themes.
- Touch/click targets are at least 40px in the desktop window and 44px when the layout enters its narrow mode.
- Reduced-motion preferences are respected globally.

## Settings Migration

The editor Settings view removes navigation entries, imports, and mounted instances for all AI-owned components listed in Scope. GeneralSection loses the AI-window startup row. The migrated components are reused in the AI workspace rather than copied.

Direct links or commands that previously opened an AI settings section should open `/ai-window` and select the equivalent AI Settings subsection. Existing saved data and backend service contracts remain compatible.

## Testing Strategy

### Frontend Unit and Component Tests

- Sidebar width clamps, pointer dragging, and keyboard resizing.
- Conversation list remains mounted while switching all primary workspace views.
- Skills and Automation navigation select the expected content.
- Terminal opens on the right, preserves the active workspace view, resizes within bounds, and closes cleanly.
- Five AI-window themes map to the correct design-language and mode attributes.
- Follow System switches only between Apple Light and Apple Dark.
- AI theme changes do not mutate main-editor theme settings.
- Migrated AI settings no longer render in SettingsView and do render in the AI settings center.
- ActivityBar highlight follows `IsAIWindowVisible()` and clears after hiding the window.
- Existing conversation, composer, terminal, skills, workflow, and AI store tests remain green.

### Backend Tests

- `IsAIWindowOpen()` distinguishes existence from nil.
- `IsAIWindowVisible()` reports false for nil/hidden and true for visible windows where the Wails test boundary permits.
- Window toggle behavior does not recreate a hidden window.
- New settings fields round-trip, default correctly, and reject invalid values safely.
- Existing service concurrency and race-oriented tests remain green.

### Full Verification

Run fresh full gates after implementation:

- Frontend Vitest suite
- ESLint
- `vue-tsc --noEmit`
- Production frontend build
- Bindings and documentation checks
- `go test ./...`
- Go race tests for services where supported
- Existing dual-window and end-to-end smoke tests
- Manual Wails smoke check covering open, hide, reopen, theme switching, sidebar resizing, conversation switching, settings migration, Skills, Automation, and right-docked terminal

## Acceptance Criteria

- AI sidebar is visibly wider and can be dragged both left and right within safe bounds.
- Every primary sidebar feature has a concise visible description.
- Conversation history is permanently present in the sidebar.
- “gugacode AI chat” is absent from the UI.
- Skills and Automation are top-level AI sidebar entries.
- All agreed AI settings are removed from editor Settings and available in the AI window.
- Terminal only appears as a right-side dock and never replaces the current AI content.
- The AI window has exactly five independent theme choices with the specified system behavior.
- Changing AI themes does not alter the editor theme.
- Hiding the AI window clears the editor AI-button highlight.
- Motion is restrained, reduced-motion compliant, and free of decorative glow.
- Full frontend and backend verification gates pass, with any environment-limited manual checks explicitly reported.
