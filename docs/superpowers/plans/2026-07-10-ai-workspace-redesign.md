# AI Workspace Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a unified independent AI workspace with a resizable permanent sidebar, fixed conversation history, Skills and Automation pages, migrated AI settings, a right-docked terminal, five independent themes, and correct editor activity-bar visibility state.

**Architecture:** Keep `/ai-window` as the single primary shell and extract its navigation, settings, and automation surfaces into focused Vue components. Persist AI-window-only preferences through the existing settings service, apply AI theme attributes locally in the AI webview, and preserve the existing conversation, terminal, Skills, plan, goal, workflow, and snapshot stores as sources of truth. Add a visibility-specific backend query rather than changing the meaning of the existing window-existence query.

**Tech Stack:** Vue 3 Composition API, TypeScript, Vitest, Vue Test Utils, Wails v3, Go, Element Plus, CSS custom properties.

## Global Constraints

- Work in the existing checkout because the user explicitly authorized immediate changes and the working tree contains pre-existing overlapping user edits that must remain visible.
- Preserve all unrelated dirty-worktree changes; stage and commit only files belonging to each task.
- Because several implementation targets were already modified before this task, do not stage or commit implementation files during Tasks 1–7. Use diff and test checkpoints instead, then let the finishing workflow decide how to integrate the combined dirty-worktree result without claiming ownership of pre-existing hunks.
- Use test-driven development: each production behavior needs a failing test observed before implementation.
- AI sidebar width is clamped to 260–380px and is resizable in both pointer directions.
- Terminal is right-docked only, clamped to 340px–55% of available content width, and never replaces the active AI view.
- AI-window theme values are exactly `apple-dark`, `apple-light`, `claude-dark`, `claude-light`, and `system`.
- `system` always resolves to Apple light/dark and never Claude.
- AI-window theme changes must not mutate the editor `theme` or `designLanguage` values.
- Do not add decorative glow, animated gradients, or large blurred shadows.
- All motion must respect `prefers-reduced-motion`.

---

### Task 1: Persist AI-window Preferences and Apply Independent Themes

**Files:**
- Modify: `services/settings_service.go`
- Modify: `services/settings_service_test.go`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/stores/app.ts`
- Modify: `frontend/src/stores/app.test.ts`
- Create: `frontend/src/stores/aiWindow.ts`
- Create: `frontend/src/stores/aiWindow.test.ts`

**Interfaces:**
- Produces: `export type AIWindowTheme = "apple-dark" | "apple-light" | "claude-dark" | "claude-light" | "system"`.
- Produces: `normalizeAIWindowTheme(value: unknown): AIWindowTheme`.
- Produces: `resolveAIWindowTheme(theme, prefersLight): { designLanguage: "apple" | "claude"; mode: "dark" | "light" }`.
- Produces: `applyAIWindowTheme(theme, prefersLight?): void` which writes only DOM attributes in the current webview.
- Produces: `aiWindowState` with `activeView`, `sidebarWidth`, `terminalVisible`, `terminalWidth`, and `theme`.
- Settings fields: `aiWindowTheme`, `aiSidebarWidth`, and `aiTerminalWidth`.

- [ ] **Step 1: Write failing Go settings round-trip/default tests**

Add tests that save and reload:

```go
func TestSettingsService_AIWindowPreferencesRoundTrip(t *testing.T) {
    configPath := filepath.Join(t.TempDir(), "settings.json")
    svc := &SettingsService{configPath: configPath}
    s := defaultSettings()
    s.AIWindowTheme = "claude-light"
    s.AISidebarWidth = 336
    s.AITerminalWidth = 512
    if err := svc.SaveSettings(s); err != nil { t.Fatal(err) }
    got, err := svc.LoadSettings()
    if err != nil { t.Fatal(err) }
    if got.AIWindowTheme != "claude-light" { t.Fatalf("theme=%q", got.AIWindowTheme) }
    if got.AISidebarWidth != 336 { t.Fatalf("sidebar=%d", got.AISidebarWidth) }
    if got.AITerminalWidth != 512 { t.Fatalf("terminal=%d", got.AITerminalWidth) }
}
```

Also add a legacy-settings test asserting missing fields load as `apple-dark`, `288`, and `440`.

- [ ] **Step 2: Run the targeted Go tests and verify RED**

Run: `go test ./services -run 'AIWindowPreferences' -count=1`

Expected: compile failure because the new fields do not exist.

- [ ] **Step 3: Add backend settings fields and defaults**

Add to `services.Settings`:

```go
AIWindowTheme  string `json:"aiWindowTheme"`
AISidebarWidth int    `json:"aiSidebarWidth"`
AITerminalWidth int   `json:"aiTerminalWidth"`
```

Set default values in the existing default-settings constructor. Normalize invalid or missing values during load with explicit helpers:

```go
func normalizeAIWindowTheme(value string) string
func clampInt(value, fallback, min, max int) int
```

Valid theme values are the five global-constraint strings. Width bounds are 260–380 for sidebar and 340–960 for persisted terminal width; runtime layout applies the additional 55% bound.

- [ ] **Step 4: Run targeted Go tests and verify GREEN**

Run: `go test ./services -run 'AIWindowPreferences' -count=1`

Expected: PASS.

- [ ] **Step 5: Write failing frontend theme/state tests**

Create `frontend/src/stores/aiWindow.test.ts` covering:

```ts
expect(normalizeAIWindowTheme("claude-dark")).toBe("claude-dark");
expect(normalizeAIWindowTheme("bad-value")).toBe("apple-dark");
expect(resolveAIWindowTheme("system", true)).toEqual({ designLanguage: "apple", mode: "light" });
expect(resolveAIWindowTheme("system", false)).toEqual({ designLanguage: "apple", mode: "dark" });
expect(resolveAIWindowTheme("claude-light", false)).toEqual({ designLanguage: "claude", mode: "light" });
```

Add a DOM test that seeds editor values, calls `applyAIWindowTheme("claude-dark")`, and asserts `appState.theme` and `appState.designLanguage` are unchanged while `<html>` receives `data-mode="dark"` and `data-design-language="claude"`.

Extend `app.test.ts` to assert the three new settings fields load and save.

- [ ] **Step 6: Run targeted frontend tests and verify RED**

Run: `node node_modules/vitest/vitest.mjs run src/stores/aiWindow.test.ts src/stores/app.test.ts`

Expected: FAIL because the store and settings properties do not exist.

- [ ] **Step 7: Implement frontend types, state, persistence, and theme application**

Add to the frontend `Settings` type and `AppState`:

```ts
aiWindowTheme: AIWindowTheme;
aiSidebarWidth: number;
aiTerminalWidth: number;
```

Create `aiWindow.ts` with constants and clamping functions:

```ts
export const AI_SIDEBAR_MIN = 260;
export const AI_SIDEBAR_MAX = 380;
export const AI_TERMINAL_MIN = 340;
export const AI_TERMINAL_MAX_PERSISTED = 960;
export type AIWorkspaceView = "assistant" | "skills" | "automation" | "settings" | "rollback";
```

`applyAIWindowTheme` must set/remove `data-design-language`, set `data-mode`, and set `data-ai-window-theme` without calling `applyMode` or `applyDesignLanguage`, because those mutate global editor preference state.

Wire load/save fields through `app.ts` and synchronize `aiWindowState` after settings load.

- [ ] **Step 8: Run targeted frontend tests and verify GREEN**

Run: `node node_modules/vitest/vitest.mjs run src/stores/aiWindow.test.ts src/stores/app.test.ts`

Expected: PASS.

- [ ] **Step 9: Record the Task 1 diff checkpoint**

```bash
git diff -- services/settings_service.go services/settings_service_test.go frontend/src/types/index.ts frontend/src/stores/app.ts frontend/src/stores/app.test.ts frontend/src/stores/aiWindow.ts frontend/src/stores/aiWindow.test.ts
```

Do not stage these files.

### Task 2: Fix AI-window Visibility Semantics and Editor Highlight

**Files:**
- Modify: `services/window_service.go`
- Modify: `services/window_service_test.go`
- Modify: `frontend/src/api/services.ts`
- Modify: `frontend/src/components/layout/ActivityBar.vue`
- Create: `frontend/src/components/layout/ActivityBar.test.ts`
- Modify through the project binding generator: `frontend/bindings/gugacode/services/windowservice.ts`

**Interfaces:**
- Produces: backend `IsAIWindowVisible() bool`.
- Produces: frontend `windowService.isAIWindowVisible(): Promise<boolean>`.
- ActivityBar active state consumes visibility rather than existence.

- [ ] **Step 1: Write failing visibility and ActivityBar tests**

Extend the nil-window backend test:

```go
if w.IsAIWindowVisible() {
    t.Error("expected nil AI window not visible")
}
```

Create `ActivityBar.test.ts` with a mocked `windowService`. Mount on `/editor`, return `true` then `false` from `isAIWindowVisible`, click the AI button, and assert the active class is removed after the toggle. Assert `openAIWindow` is not called as a fallback when toggle intentionally hides the window.

- [ ] **Step 2: Run targeted tests and verify RED**

Run: `go test ./services -run WindowService -count=1`

Run: `node node_modules/vitest/vitest.mjs run src/components/layout/ActivityBar.test.ts`

Expected: FAIL because visibility API is absent and ActivityBar uses `isAIWindowOpen`.

- [ ] **Step 3: Implement visibility API and correct click flow**

Add:

```go
func (w *WindowService) IsAIWindowVisible() bool {
    w.mu.RLock()
    aiWin := w.aiWindow
    w.mu.RUnlock()
    return aiWin != nil && aiWin.IsVisible()
}
```

Expose it in frontend services/bindings. Change ActivityBar to:

```ts
async function refreshAiWindowState() {
  try { aiWindowVisible.value = await windowService.isAIWindowVisible(); }
  catch { aiWindowVisible.value = false; }
}

async function handleAiWindowClick() {
  await windowService.toggleAIWindow();
  await refreshAiWindowState();
}
```

Retain error notification, but remove the fallback that reopens a window immediately after a deliberate hide.

- [ ] **Step 4: Regenerate/check Wails bindings**

Run from the repository root: `task common:generate:bindings`.

Then run: `node scripts/check-bindings.mjs`.

Expected: WindowService bindings export `IsAIWindowVisible` and bindings check passes.

- [ ] **Step 5: Run targeted tests and verify GREEN**

Run both commands from Step 2.

Expected: PASS.

- [ ] **Step 6: Record the Task 2 diff checkpoint**

```bash
git diff -- services/window_service.go services/window_service_test.go frontend/src/api/services.ts frontend/src/components/layout/ActivityBar.vue frontend/src/components/layout/ActivityBar.test.ts frontend/bindings/gugacode/services/windowservice.ts
```

Do not stage these files.

### Task 3: Build the Resizable Permanent AI Sidebar

**Files:**
- Create: `frontend/src/components/ai-window/AiWorkspaceSidebar.vue`
- Create: `frontend/src/components/ai-window/AiWorkspaceSidebar.test.ts`
- Modify: `frontend/src/components/ai-assistant/ConversationSidebar.vue`
- Modify: `frontend/src/components/ai-assistant/ConversationSidebar.test.ts`
- Modify: `frontend/src/views/AiWindowView.vue`
- Modify: `frontend/src/stores/aiWindow.ts`
- Modify: `frontend/src/stores/aiWindow.test.ts`

**Interfaces:**
- `AiWorkspaceSidebar` props: `activeView: AIWorkspaceView`, `width: number`.
- Emits: `select-view(view)`, `select-conversation(id)`, `new-conversation`, `resize(width)` and `resize-commit(width)`.
- `ConversationSidebar` gains presentation props such as `embedded?: boolean` and does not create a nested outer sidebar border when embedded.

- [ ] **Step 1: Write failing sidebar component tests**

Tests must assert:

- Five visible nav titles and five concise descriptions render.
- “gugacode AI 对话” does not render.
- ConversationSidebar is mounted regardless of active view.
- Pointer drag from x=288 to x=348 emits a larger width; dragging back emits a smaller width.
- ArrowRight/ArrowLeft resize, Home clamps to 260, and End clamps to 380.
- Selecting Skills emits `skills`; selecting a conversation emits its ID without hiding the sidebar.

- [ ] **Step 2: Run targeted tests and verify RED**

Run: `node node_modules/vitest/vitest.mjs run src/components/ai-window/AiWorkspaceSidebar.test.ts src/components/ai-assistant/ConversationSidebar.test.ts`

Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement the sidebar using `useDragResize`**

Use:

```ts
const resize = useDragResize({
  direction: "horizontal",
  sign: "positive-increases",
  min: AI_SIDEBAR_MIN,
  max: AI_SIDEBAR_MAX,
  getStartSize: () => props.width,
  onResize: (width) => emit("resize", width),
  onCommit: (width) => emit("resize-commit", width),
});
```

Render the resize handle with `role="separator"`, `aria-orientation="vertical"`, `aria-valuemin`, `aria-valuemax`, and `aria-valuenow`.

Keep ConversationSidebar mounted below the nav and add a compact embedded mode so it fills remaining height without setting its own fixed width.

- [ ] **Step 4: Replace AiWindowView’s activity rail/drawers with the new sidebar**

Remove `DrawerKind`, drawer toggles, drawer resize code, and the temporary conversation/settings drawers. Bind `aiWindowState.activeView` and `aiWindowState.sidebarWidth`; persist committed width through `appState.aiSidebarWidth` and `saveSettings()`.

- [ ] **Step 5: Run targeted tests and verify GREEN**

Run the command from Step 2 plus: `node node_modules/vitest/vitest.mjs run src/stores/aiWindow.test.ts`.

Expected: PASS.

- [ ] **Step 6: Record the Task 3 diff checkpoint**

```bash
git diff -- frontend/src/components/ai-window/AiWorkspaceSidebar.vue frontend/src/components/ai-window/AiWorkspaceSidebar.test.ts frontend/src/components/ai-assistant/ConversationSidebar.vue frontend/src/components/ai-assistant/ConversationSidebar.test.ts frontend/src/views/AiWindowView.vue frontend/src/stores/aiWindow.ts frontend/src/stores/aiWindow.test.ts
```

Do not stage these files.

### Task 4: Dock the Terminal on the Right

**Files:**
- Create: `frontend/src/components/ai-window/AiTerminalDock.vue`
- Create: `frontend/src/components/ai-window/AiTerminalDock.test.ts`
- Modify: `frontend/src/views/AiWindowView.vue`
- Modify: `frontend/src/stores/aiWindow.ts`
- Modify: `frontend/src/stores/aiWindow.test.ts`

**Interfaces:**
- `AiTerminalDock` props: `visible`, `width`, `maxWidth`.
- Emits: `close`, `resize(width)`, `resize-commit(width)`.
- Store helper: `getTerminalMaxWidth(contentWidth: number): number` returns `Math.max(340, Math.floor(contentWidth * 0.55))`.

- [ ] **Step 1: Write failing dock/store tests**

Assert:

- Invisible dock does not render TerminalPanel.
- Visible dock renders on the right and leaves the active workspace slot mounted.
- Its left-edge drag uses `positive-decreases`: dragging left increases width and dragging right decreases width.
- Width clamps to 340 and the calculated 55% maximum.
- Closing emits `close` without changing `activeView`.
- Toggling terminal in store preserves the active view and composer state reference.

- [ ] **Step 2: Run targeted tests and verify RED**

Run: `node node_modules/vitest/vitest.mjs run src/components/ai-window/AiTerminalDock.test.ts src/stores/aiWindow.test.ts`

Expected: FAIL because the dock does not exist.

- [ ] **Step 3: Implement the right dock**

Use a flex row inside AiWindowView’s content region:

```vue
<section class="ai-window__workspace-row">
  <div class="ai-window__workspace-main">...</div>
  <AiTerminalDock
    :visible="aiWindowState.terminalVisible"
    :width="aiWindowState.terminalWidth"
    :max-width="terminalMaxWidth"
    @close="aiWindowState.terminalVisible = false"
    @resize="setAITerminalWidth"
    @resize-commit="persistAITerminalWidth"
  />
</section>
```

Terminal action becomes a sidebar or header utility toggle. Delete the old `activeView === "terminal"` replacement path.

- [ ] **Step 4: Add restrained dock transition and reduced-motion rule**

Animate width/opacity for 220–250ms. Do not use glow or a large shadow. Under reduced motion, disable width animation.

- [ ] **Step 5: Run targeted tests and verify GREEN**

Run the Step 2 command and the existing `src/components/layout/TerminalPanel.test.ts` suite.

Expected: PASS.

- [ ] **Step 6: Record the Task 4 diff checkpoint**

```bash
git diff -- frontend/src/components/ai-window/AiTerminalDock.vue frontend/src/components/ai-window/AiTerminalDock.test.ts frontend/src/views/AiWindowView.vue frontend/src/stores/aiWindow.ts frontend/src/stores/aiWindow.test.ts
```

Do not stage these files.

### Task 5: Add Skills and Automation Workspace Pages

**Files:**
- Create: `frontend/src/components/ai-window/AiSkillsView.vue`
- Create: `frontend/src/components/ai-window/AiAutomationView.vue`
- Create: `frontend/src/components/ai-window/AiAutomationView.test.ts`
- Modify: `frontend/src/views/AiWindowView.vue`
- Reuse without copying: `frontend/src/components/settings/ai/SkillsSection.vue`
- Reuse without copying: `frontend/src/components/settings/ai/PlanSection.vue`
- Reuse without copying: `frontend/src/components/settings/ai/GoalSection.vue`
- Reuse without copying: `frontend/src/components/settings/ai/WorkflowSection.vue`

**Interfaces:**
- `AiSkillsView` renders the existing SkillsSection.
- `AiAutomationView` owns local tab type `"plans" | "goals" | "workflows"` and renders exactly one existing section at a time.

- [ ] **Step 1: Write failing Automation tests**

Mount with stubbed existing sections. Assert default Plans content, tab switching to Goals and Workflows, keyboard-accessible tab roles, and no duplicated store/data layer.

- [ ] **Step 2: Run targeted test and verify RED**

Run: `node node_modules/vitest/vitest.mjs run src/components/ai-window/AiAutomationView.test.ts`

Expected: FAIL because the view does not exist.

- [ ] **Step 3: Implement Skills and Automation views**

Use a shared workspace page header and internal tablist. Keep the existing sections intact except for small container-style compatibility changes if needed.

- [ ] **Step 4: Wire workspace view selection in AiWindowView**

Render Assistant, Skills, Automation, Settings placeholder, and Rollback with `v-show` or a keyed transition that does not unmount the conversation sidebar or terminal. Rollback uses the existing SnapshotTimeline.

- [ ] **Step 5: Run targeted tests and verify GREEN**

Run Step 2 plus the existing Skills, plan, goal, and workflow tests selected by file/store names.

Expected: PASS.

- [ ] **Step 6: Record the Task 5 diff checkpoint**

```bash
git diff -- frontend/src/components/ai-window/AiSkillsView.vue frontend/src/components/ai-window/AiAutomationView.vue frontend/src/components/ai-window/AiAutomationView.test.ts frontend/src/views/AiWindowView.vue
```

Do not stage these files.

### Task 6: Build AI Settings Center and Remove AI Settings from Editor

**Files:**
- Create: `frontend/src/components/ai-window/AiSettingsView.vue`
- Create: `frontend/src/components/ai-window/AiSettingsView.test.ts`
- Create: `frontend/src/components/ai-window/AiWindowThemePicker.vue`
- Create: `frontend/src/components/ai-window/AiWindowThemePicker.test.ts`
- Modify: `frontend/src/views/AiWindowView.vue`
- Modify: `frontend/src/views/SettingsView.vue`
- Create or modify: `frontend/src/views/SettingsView.test.ts`
- Modify: `frontend/src/components/settings/GeneralSection.vue`
- Create: `frontend/src/components/settings/GeneralSection.test.ts`
- Reuse existing AI settings components under `frontend/src/components/settings/` and `frontend/src/components/settings/ai/`.

**Interfaces:**
- `AiSettingsView` group type: `"models" | "context" | "execution" | "integrations" | "window"`.
- `AiWindowThemePicker` uses `v-model:theme` of `AIWindowTheme` and emits one of exactly five values.

- [ ] **Step 1: Write failing settings migration tests**

Assert editor SettingsView contains only General, Editor, Terminal, Shortcuts, Appearance, and Profiles navigation. Assert it does not mount AI, Agent, MCP, Skills, Persona, Plan, Goal, Workflow, Model Permission, Diff, Rollback, Personalization, Presets, Prompts, Computer Use, or IM.

Assert AI Settings group navigation renders the existing components in these groups:

```ts
models: [AiSection, AgentSection, PersonaSection, ModelPermissionSection]
context: [McpSection, SkillsSection, PromptsSection, PresetsSection]
execution: [ComputerUseSection, DiffSection, RollbackSection]
integrations: [ImSection, PersonalizationSection]
window: [AiWindowThemePicker, startup/always-on-top/layout controls]
```

- [ ] **Step 2: Write failing theme picker tests**

Assert exactly five radio buttons, labels for four explicit themes plus Follow System, and emitted values. Assert selecting a theme calls the local AI theme helper and does not mutate `appState.theme` or `appState.designLanguage`.

- [ ] **Step 3: Run targeted tests and verify RED**

Run: `node node_modules/vitest/vitest.mjs run src/components/ai-window/AiSettingsView.test.ts src/components/ai-window/AiWindowThemePicker.test.ts src/views/SettingsView.test.ts`

Expected: FAIL because the components/migration do not exist.

- [ ] **Step 4: Implement AI Settings and theme picker**

Use existing settings components directly. Window controls bind to `appState.openAIWindowOnStartup`, `aiWindowState.theme`, widths, and the current always-on-top service. Theme card previews reuse the visual grammar of AppearanceSection but save through `aiWindowTheme` only.

- [ ] **Step 5: Remove AI-owned settings from editor SettingsView**

Remove imports, union keys, nav items, and mounted component instances. Remove the startup AI row from GeneralSection. Keep AppearanceSection for the editor’s own theme.

- [ ] **Step 6: Run targeted tests and verify GREEN**

Run the Step 3 command plus existing tests for migrated settings components.

Expected: PASS.

- [ ] **Step 7: Record the Task 6 diff checkpoint**

```bash
git diff -- frontend/src/components/ai-window/AiSettingsView.vue frontend/src/components/ai-window/AiSettingsView.test.ts frontend/src/components/ai-window/AiWindowThemePicker.vue frontend/src/components/ai-window/AiWindowThemePicker.test.ts frontend/src/views/AiWindowView.vue frontend/src/views/SettingsView.vue frontend/src/views/SettingsView.test.ts frontend/src/components/settings/GeneralSection.vue frontend/src/components/settings/GeneralSection.test.ts
```

Do not stage these files.

### Task 7: Apply Apple/Claude Visual Polish, Motion, and Localized Copy

**Files:**
- Modify: `frontend/src/views/AiWindowView.vue`
- Modify: `frontend/src/components/ai-window/AiWorkspaceSidebar.vue`
- Modify: `frontend/src/components/ai-window/AiTerminalDock.vue`
- Modify: `frontend/src/components/ai-window/AiAutomationView.vue`
- Modify: `frontend/src/components/ai-window/AiSettingsView.vue`
- Modify: `frontend/src/components/ai-window/AiWindowThemePicker.vue`
- Modify: `frontend/src/components/ai-assistant/ConversationSidebar.vue`
- Modify: `frontend/src/assets/styles/main.css`
- Modify: `frontend/src/lib/locales/zh.ts`
- Modify: `frontend/src/lib/locales/en.ts`
- Modify: `frontend/src/lib/locales/ja.ts`
- Modify: `frontend/src/lib/i18n.test.ts`
- Modify: `frontend/src/router/index.ts`

**Interfaces:**
- New i18n keys share identical key sets across zh/en/ja.
- `/ai-window` document title uses “AI Assistant” instead of “gugacode AI”.

- [ ] **Step 1: Write failing copy/i18n tests**

Add i18n assertions for sidebar titles/descriptions, Automation, five themes, Window settings, terminal controls, and resize labels. Add a source-level or mounted-view assertion that the removed “gugacode AI chat” key/value no longer appears in the AI window UI.

- [ ] **Step 2: Run targeted tests and verify RED**

Run: `node node_modules/vitest/vitest.mjs run src/lib/i18n.test.ts src/components/ai-window/AiWorkspaceSidebar.test.ts`

Expected: FAIL for missing keys/copy.

- [ ] **Step 3: Add localized copy and title changes**

Use natural localized descriptions. Update router title to “AI Assistant”. Remove or stop using `aiWindow.actChat` values that name the product chat surface.

- [ ] **Step 4: Polish shared tokens and AI shell styling**

Reuse existing `[data-design-language="claude"]` and `[data-mode]` tokens. Add only focused AI shell tokens such as:

```css
--ai-sidebar-width-default: 288px;
--ai-shell-transition: 220ms var(--ease-standard);
--ai-panel-border: var(--color-border-default);
```

Apple uses white/parchment or neutral near-black plus Action Blue. Claude uses warm cream/charcoal plus restrained coral. Keep shadows limited to menus/floating panels and remove the current heavy AI dropdown shadow if it exceeds the shared floating shadow.

- [ ] **Step 5: Add restrained transitions and reduced-motion coverage**

Add view fade/translate, selected indicator, menu, and message-entry transitions. Add a single reduced-motion block covering all new AI classes.

- [ ] **Step 6: Run targeted tests and verify GREEN**

Run Step 2 plus all new AI window component tests.

Expected: PASS.

- [ ] **Step 7: Record the Task 7 diff checkpoint**

```bash
git diff -- frontend/src/views/AiWindowView.vue frontend/src/components/ai-window frontend/src/components/ai-assistant/ConversationSidebar.vue frontend/src/assets/styles/main.css frontend/src/lib/locales/zh.ts frontend/src/lib/locales/en.ts frontend/src/lib/locales/ja.ts frontend/src/lib/i18n.test.ts frontend/src/router/index.ts
```

Do not stage these files.

### Task 8: Full Verification and Regression Cleanup

**Files:**
- Modify only files needed to fix failures caused by Tasks 1–7.
- Update documentation only if bindings or settings schema documentation requires it.

**Interfaces:**
- Produces a clean full frontend/backend verification result.

- [ ] **Step 1: Run all frontend tests**

Run: `npm.cmd test`

Expected: all Vitest files and tests pass with zero failures.

- [ ] **Step 2: Run lint and type checking**

Run: `node node_modules/eslint/bin/eslint.js src`

Run: `node node_modules/vue-tsc/bin/vue-tsc.js --noEmit`

Expected: exit 0 for both.

- [ ] **Step 3: Run production frontend build and repository checks**

Run: `npm.cmd run build`

Run from repository root: `node scripts/check-bindings.mjs`

Run from repository root: `node scripts/check-doc-numbers.mjs`

Expected: exit 0 for every command.

- [ ] **Step 4: Run all Go tests**

Run: `go test ./... -count=1`

Expected: all packages pass.

- [ ] **Step 5: Run service race tests if supported on Windows**

Run: `go test -race ./services -count=1`

Expected: PASS, or explicitly report if the installed toolchain/platform cannot run the race detector.

- [ ] **Step 6: Run focused smoke tests**

Run: `node node_modules/vitest/vitest.mjs run src/lib/dual-window-smoke.test.ts src/lib/e2e-smoke.test.ts`

Expected: PASS.

- [ ] **Step 7: Review acceptance criteria against the diff**

Confirm each criterion from `docs/superpowers/specs/2026-07-10-ai-workspace-redesign-design.md` has code and test evidence. Inspect `git diff --check` and `git status --short` to ensure unrelated user changes were not staged or overwritten.

- [ ] **Step 8: Route any verification failure back to its owning task**

Do not create a catch-all verification commit. If a gate fails, return to the task that introduced the behavior, add or correct the focused regression test there, and amend that task with a narrowly scoped follow-up commit.
