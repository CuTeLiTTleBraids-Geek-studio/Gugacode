# Light Theme Implementation Plan (B-3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a complete light theme with dark/light/system mode switching. The light theme overrides all 40+ surface, text, border, and chrome tokens; Monaco editor gets 8 light-theme variants (one per accent); the SettingsView theme selector becomes functional.

**Architecture:** Add a `[data-mode="light"]` selector in `main.css` that overrides the dark tokens defined in `@theme`. Add a `createLightThemeData()` function in `monaco-themes.ts` that generates light Monaco themes. Add an `applyMode(mode)` function in `app.ts` that sets the `data-mode` attribute on `<html>` and switches Monaco between dark/light theme sets. The `handleThemeChange` in SettingsView calls `applyMode`.

**Tech Stack:** CSS custom properties, Monaco editor theming API, Vue 3 reactivity, `prefers-color-scheme` media query

---

## File Structure

- Modify: `frontend/src/assets/styles/main.css` — Task 1 (light tokens) + Task 2 (html color-scheme)
- Modify: `frontend/src/lib/monaco-themes.ts` — Task 3 (light Monaco themes)
- Modify: `frontend/src/stores/app.ts` — Task 4 (applyMode function)
- Modify: `frontend/src/views/SettingsView.vue` — Task 5 (wire theme change)
- Modify: `frontend/src/main.ts` — Task 6 (apply mode on startup)
- Test: `frontend/src/stores/app.test.ts` — Task 7 (new test file)

---

### Task 1: Add Light Theme CSS Tokens

**Files:**
- Modify: `frontend/src/assets/styles/main.css` (insert after line 211, before the Base reset section)

Add a `[data-mode="light"]` block that overrides all dark-theme tokens with light equivalents. The accent overrides (lines 132-211) remain unchanged — they set `--color-primary` which works in both modes. Only surface/text/border/chrome tokens need overriding.

- [x] **Step 1: Insert light theme token block**

Insert this block after line 211 (after the indigo accent block), before the "Base reset & global styles" comment:

```css
/* ─────────────────────────────────────────────────────────
   Light Mode Overrides
   Apply data-mode="light" on <html> to switch from dark to light.
   Accent colors (data-theme) remain the same in both modes.
   ───────────────────────────────────────────────────────── */

[data-mode="light"] {
  /* ── Surface hierarchy (Material You light tonal surfaces) ── */
  --color-bg-base: #fefcff;
  --color-bg-surface: #fefbff;
  --color-bg-surface-dim: #dbd9de;
  --color-bg-elevated: #ffffff;
  --color-bg-overlay: #ffffff;
  --color-bg-surface-container: #f4f3f8;
  --color-bg-surface-container-low: #eeeef3;
  --color-bg-surface-container-high: #e9e8ec;
  --color-bg-surface-container-highest: #e3e2e7;

  /* ── App chrome surfaces ── */
  --color-sidebar-bg: #f8f7fb;
  --color-sidebar-hover: #edeef1;
  --color-sidebar-active: #e3e3e8;
  --color-titlebar-bg: #f0f0f4;
  --color-activitybar-bg: #faf9fd;
  --color-terminal-bg: #ffffff;

  /* ── Text on surfaces ── */
  --color-on-background: #1b1b1f;
  --color-on-surface: #1b1b1f;
  --color-on-surface-variant: #44474e;

  --color-text-primary: #1b1b1f;
  --color-text-secondary: #44474e;
  --color-text-tertiary: #747678;
  --color-text-disabled: #c4c6c9;

  /* ── Inverse ── */
  --color-inverse-surface: #2e3036;
  --color-inverse-on-surface: #f0f0f4;

  /* ── Borders (dark opacity on light bg) ── */
  --color-border-subtle: rgba(0, 0, 0, 0.06);
  --color-border-default: rgba(0, 0, 0, 0.10);
  --color-border-strong: rgba(0, 0, 0, 0.16);
  --color-outline: rgba(0, 0, 0, 0.12);
  --color-outline-variant: rgba(0, 0, 0, 0.08);

  /* ── Scrollbar ── */
  --color-scrollbar: rgba(0, 0, 0, 0.08);
  --color-scrollbar-hover: rgba(0, 0, 0, 0.18);

  /* ── Physical shadows (lighter for light mode) ── */
  --shadow-1: 0 1px 2px rgba(0, 0, 0, 0.08), 0 1px 3px rgba(0, 0, 0, 0.04);
  --shadow-2: 0 2px 4px rgba(0, 0, 0, 0.08), 0 3px 6px rgba(0, 0, 0, 0.04);
  --shadow-3: 0 4px 8px rgba(0, 0, 0, 0.10), 0 6px 12px rgba(0, 0, 0, 0.05);
  --shadow-4: 0 8px 16px rgba(0, 0, 0, 0.12), 0 12px 24px rgba(0, 0, 0, 0.06);

  /* ── Semantic palette adjustments for light bg ── */
  --color-error: #ba1a1a;
  --color-error-container: rgba(186, 26, 26, 0.08);
  --color-warning: #7d5700;
  --color-warning-container: rgba(125, 87, 0, 0.08);
  --color-success: #1f6b3a;
  --color-success-container: rgba(31, 107, 58, 0.08);

  /* ── Element Plus overrides for light mode ── */
  --el-bg-color: var(--color-bg-base);
  --el-bg-color-overlay: var(--color-bg-overlay);
  --el-bg-color-page: var(--color-bg-base);
  --el-text-color-primary: var(--color-text-primary);
  --el-text-color-regular: var(--color-text-secondary);
  --el-text-color-secondary: var(--color-text-tertiary);
  --el-border-color: var(--color-border-default);
  --el-border-color-light: var(--color-border-subtle);
  --el-fill-color: var(--color-bg-surface-container);
  --el-fill-color-light: var(--color-bg-surface-container-low);
  --el-fill-color-lighter: var(--color-bg-surface);
  --el-fill-color-blank: var(--color-bg-base);
  --el-fill-color-extra-light: var(--color-bg-surface-dim);
}
```

- [x] **Step 2: Verify CSS parses without errors**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 2: Update HTML Color-Scheme

**Files:**
- Modify: `frontend/src/assets/styles/main.css:225-235`

The `html` selector has `color-scheme: dark` hardcoded. This must be dynamic based on `data-mode`.

- [x] **Step 1: Update html selector**

In `frontend/src/assets/styles/main.css`, find the `html` block (around line 225):
```css
html {
  font-family: var(--font-sans);
  color-scheme: dark;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-rendering: optimizeLegibility;
  font-size: 14px;
  line-height: 1.5;
  background: var(--color-bg-base);
  color: var(--color-on-background);
}
```

Change `color-scheme: dark;` to `color-scheme: dark light;`:
```css
html {
  font-family: var(--font-sans);
  color-scheme: dark light;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-rendering: optimizeLegibility;
  font-size: 14px;
  line-height: 1.5;
  background: var(--color-bg-base);
  color: var(--color-on-background);
}
```

- [x] **Step 2: Add explicit color-scheme for each mode**

Add after the `html` block (after line 235):
```css
[data-mode="dark"] {
  color-scheme: dark;
}

[data-mode="light"] {
  color-scheme: light;
}
```

- [x] **Step 3: Update .dark class marker**

Find the `.dark` block (around line 359):
```css
.dark {
  color-scheme: dark;
}
```

Change to:
```css
.dark {
  color-scheme: dark;
}

.light {
  color-scheme: light;
}
```

---

### Task 3: Add Light Monaco Themes

**Files:**
- Modify: `frontend/src/lib/monaco-themes.ts`

Add a `createLightThemeData(accent)` function and register light variants for all 8 accents. The light theme uses `base: "vs"` (Monaco's built-in light theme) instead of `vs-dark`.

- [x] **Step 1: Add light theme data function**

In `frontend/src/lib/monaco-themes.ts`, add this function after `createThemeData()` (after line 96):

```typescript
function createLightThemeData(accent: string): monaco.editor.IStandaloneThemeData {
  return {
    base: "vs",
    inherit: true,
    rules: [
      { token: "comment",             foreground: "747678", fontStyle: "italic" },
      { token: "keyword",            foreground: accent },
      { token: "keyword.control",    foreground: accent },
      { token: "string",             foreground: "1f6b3a" },
      { token: "string.escape",      foreground: "8e24aa", fontStyle: "bold" },
      { token: "number",             foreground: "b25f00" },
      { token: "regexp",             foreground: "006777" },
      { token: "type",               foreground: "006a6a" },
      { token: "class",              foreground: "9a4a00" },
      { token: "function",           foreground: "1858b4" },
      { token: "variable",          foreground: "1b1b1f" },
      { token: "variable.predefined",foreground: "b0146f" },
      { token: "constant",           foreground: "8e24aa" },
      { token: "tag",                foreground: "b0146f" },
      { token: "attribute.name",      foreground: "1858b4" },
      { token: "attribute.value",     foreground: "1f6b3a" },
      { token: "delimiter",           foreground: "44474e" },
      { token: "delimiter.bracket",  foreground: "44474e" },
      { token: "operator",           foreground: accent },
      { token: "meta",               foreground: "44474e" },
      { token: "meta.tag",           foreground: "44474e" },
    ],
    colors: {
      "editor.background":             "#fefcff",
      "editor.foreground":             "#1b1b1f",
      "editor.lineHighlightBackground": "#f4f3f8",
      "editor.selectionBackground":     accent + "30",
      "editor.inactiveSelectionBackground": accent + "18",
      "editorLineNumber.foreground":    "#c4c6c9",
      "editorLineNumber.activeForeground": "#44474e",
      "editorCursor.foreground":       "#1b1b1f",
      "editorWhitespace.foreground":   "#dbd9de",
      "editorIndentGuide.background":  "#eeeef3",
      "editorIndentGuide.activeBackground": "#c4c6c9",
      "editorBracketMatch.background": accent + "25",
      "editorBracketMatch.border":     accent + "60",
      "editorGutter.background":       "#fefcff",
      "editorWidget.background":       "#ffffff",
      "editorWidget.border":           "#e3e2e7",
      "editorSuggestWidget.background": "#ffffff",
      "editorSuggestWidget.border":     "#e3e2e7",
      "editorSuggestWidget.selectedBackground": accent + "20",
      "editorHoverWidget.background":  "#ffffff",
      "editorHoverWidget.border":      "#e3e2e7",
      "peekViewEditor.background":     "#f4f3f8",
      "peekViewResult.background":      "#f4f3f8",
      "peekViewTitle.background":       "#ffffff",
      "minimap.background":             "#fefcff",
      "scrollbarSlider.background":     "#c4c6c960",
      "scrollbarSlider.hoverBackground": "#74767890",
      "scrollbarSlider.activeBackground": accent + "80",
      "input.background":               "#f4f3f8",
      "input.border":                   "#e3e2e7",
      "inputOption.activeBackground":   accent + "20",
      "focusBorder":                     accent + "60",
      "list.activeSelectionBackground": accent + "18",
      "list.hoverBackground":           "#f4f3f8",
      "list.highlightForeground":       accent,
      "findMatchBackground":            accent + "35",
      "findMatchHighlightBackground":    accent + "25",
      "findRangeHighlightBackground":    accent + "10",
    },
  };
}
```

- [x] **Step 2: Add light theme names to accentThemes**

Update the `accentThemes` object (lines 17-26) to add `monacoLightTheme` field. Change:

```typescript
export interface ThemeMeta {
  label: string;
  color: string;
  monacoTheme: string;
}

export const accentThemes: Record<AccentTheme, ThemeMeta> = {
  blue:   { label: "Blue",   color: "#4285f4", monacoTheme: "nknk-blue" },
  teal:   { label: "Teal",   color: "#26a69a", monacoTheme: "nknk-teal" },
  green:  { label: "Green",  color: "#66bb6a", monacoTheme: "nknk-green" },
  amber:  { label: "Amber",  color: "#ffa726", monacoTheme: "nknk-amber" },
  pink:   { label: "Pink",   color: "#ec407a", monacoTheme: "nknk-pink" },
  purple: { label: "Purple", color: "#ab47bc", monacoTheme: "nknk-purple" },
  cyan:   { label: "Cyan",   color: "#26c6da", monacoTheme: "nknk-cyan" },
  indigo: { label: "Indigo", color: "#5c6bc0", monacoTheme: "nknk-indigo" },
};
```

to:

```typescript
export interface ThemeMeta {
  label: string;
  color: string;
  monacoTheme: string;
  monacoLightTheme: string;
}

export const accentThemes: Record<AccentTheme, ThemeMeta> = {
  blue:   { label: "Blue",   color: "#4285f4", monacoTheme: "nknk-blue",   monacoLightTheme: "nknk-light-blue" },
  teal:   { label: "Teal",   color: "#26a69a", monacoTheme: "nknk-teal",   monacoLightTheme: "nknk-light-teal" },
  green:  { label: "Green",  color: "#66bb6a", monacoTheme: "nknk-green",  monacoLightTheme: "nknk-light-green" },
  amber:  { label: "Amber",  color: "#ffa726", monacoTheme: "nknk-amber",  monacoLightTheme: "nknk-light-amber" },
  pink:   { label: "Pink",   color: "#ec407a", monacoTheme: "nknk-pink",   monacoLightTheme: "nknk-light-pink" },
  purple: { label: "Purple", color: "#ab47bc", monacoTheme: "nknk-purple", monacoLightTheme: "nknk-light-purple" },
  cyan:   { label: "Cyan",   color: "#26c6da", monacoTheme: "nknk-cyan",   monacoLightTheme: "nknk-light-cyan" },
  indigo: { label: "Indigo", color: "#5c6bc0", monacoTheme: "nknk-indigo", monacoLightTheme: "nknk-light-indigo" },
};
```

- [x] **Step 3: Register light themes in registerAllThemes**

Update `registerAllThemes()` (lines 102-107) from:

```typescript
export function registerAllThemes(): void {
  for (const [key, meta] of Object.entries(accentThemes)) {
    const data = createThemeData(meta.color);
    monaco.editor.defineTheme(meta.monacoTheme, data);
  }
}
```

to:

```typescript
export function registerAllThemes(): void {
  for (const [key, meta] of Object.entries(accentThemes)) {
    const darkData = createThemeData(meta.color);
    monaco.editor.defineTheme(meta.monacoTheme, darkData);
    const lightData = createLightThemeData(meta.color);
    monaco.editor.defineTheme(meta.monacoLightTheme, lightData);
  }
}
```

- [x] **Step 4: Add light theme application function**

Add after `applyMonacoTheme()` (after line 117):

```typescript
/**
 * Set Monaco editor theme to match current accent and mode.
 */
export function applyMonacoThemeForMode(accent: AccentTheme, mode: "dark" | "light"): void {
  const theme = accentThemes[accent];
  if (theme) {
    const themeName = mode === "light" ? theme.monacoLightTheme : theme.monacoTheme;
    monaco.editor.setTheme(themeName);
  }
}

/**
 * Get the Monaco theme name for an accent and mode.
 */
export function getMonacoThemeNameForMode(accent: AccentTheme, mode: "dark" | "light"): string {
  const theme = accentThemes[accent];
  if (!theme) return "nknk-blue";
  return mode === "light" ? theme.monacoLightTheme : theme.monacoTheme;
}
```

- [x] **Step 5: Type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 4: Add applyMode Function to app.ts

**Files:**
- Modify: `frontend/src/stores/app.ts`

Add an `applyMode(mode)` function that sets `data-mode` on `<html>` and applies the matching Monaco theme. Handle "system" mode by checking `prefers-color-scheme`.

- [x] **Step 1: Update imports**

In `frontend/src/stores/app.ts` line 5, change:
```typescript
import { accentThemes, applyMonacoTheme, registerAllThemes } from "@/lib/monaco-themes";
```
to:
```typescript
import { accentThemes, applyMonacoTheme, applyMonacoThemeForMode, registerAllThemes } from "@/lib/monaco-themes";
```

- [x] **Step 2: Add type for theme mode**

After line 7 (`export type PanelTab = ...`), add:
```typescript
export type ThemeMode = "dark" | "light" | "system";
```

- [x] **Step 3: Add helper to resolve system mode**

After the `applyAccentTheme` function (after line 129), add:

```typescript
/**
 * Resolve "system" theme mode to actual "dark" or "light" based on OS preference.
 */
export function resolveSystemMode(): "dark" | "light" {
  if (typeof window !== "undefined" && window.matchMedia) {
    return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  return "dark";
}

/**
 * Apply theme mode (dark/light/system) to DOM and Monaco editor.
 * Sets data-mode attribute on <html> and switches Monaco theme set.
 */
export function applyMode(mode: ThemeMode): void {
  appState.theme = mode;
  const resolved: "dark" | "light" = mode === "system" ? resolveSystemMode() : mode;
  document.documentElement.setAttribute("data-mode", resolved);
  applyMonacoThemeForMode(appState.accentTheme, resolved);
}
```

- [x] **Step 4: Update initThemes to apply mode**

Update `initThemes()` (lines 135-139) from:
```typescript
export function initThemes(): void {
  registerAllThemes();
  document.documentElement.setAttribute("data-theme", appState.accentTheme);
  applyMonacoTheme(appState.accentTheme);
}
```
to:
```typescript
export function initThemes(): void {
  registerAllThemes();
  document.documentElement.setAttribute("data-theme", appState.accentTheme);
  const resolved: "dark" | "light" = appState.theme === "system" ? resolveSystemMode() : (appState.theme as "dark" | "light");
  document.documentElement.setAttribute("data-mode", resolved);
  applyMonacoThemeForMode(appState.accentTheme, resolved);
}
```

- [x] **Step 5: Add system mode change listener**

After `initThemes()`, add:
```typescript
/**
 * Start listening for OS color scheme changes.
 * Only applies when theme mode is "system".
 * Call once at app startup.
 */
let systemModeCleanup: (() => void) | null = null;

export function startSystemModeListener(): void {
  if (typeof window === "undefined" || !window.matchMedia) return;
  const mq = window.matchMedia("(prefers-color-scheme: light)");
  const handler = () => {
    if (appState.theme === "system") {
      const resolved = resolveSystemMode();
      document.documentElement.setAttribute("data-mode", resolved);
      applyMonacoThemeForMode(appState.accentTheme, resolved);
    }
  };
  mq.addEventListener("change", handler);
  systemModeCleanup = () => mq.removeEventListener("change", handler);
}

export function stopSystemModeListener(): void {
  if (systemModeCleanup) {
    systemModeCleanup();
    systemModeCleanup = null;
  }
}
```

- [x] **Step 6: Type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 5: Wire Theme Change in SettingsView

**Files:**
- Modify: `frontend/src/views/SettingsView.vue:77-79`

The `handleThemeChange` function currently only saves settings. It must also call `applyMode`.

- [x] **Step 1: Update imports**

In `frontend/src/views/SettingsView.vue` line 3, change:
```typescript
import { appState, saveSettings, applyAccentTheme } from "@/stores/app";
```
to:
```typescript
import { appState, saveSettings, applyAccentTheme, applyMode } from "@/stores/app";
import type { ThemeMode } from "@/stores/app";
```

- [x] **Step 2: Update handleThemeChange**

Change the `handleThemeChange` function (lines 77-79) from:
```typescript
function handleThemeChange() {
  saveSettings();
}
```
to:
```typescript
function handleThemeChange() {
  applyMode(appState.theme as ThemeMode);
  saveSettings();
}
```

- [x] **Step 3: Type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 6: Apply Mode on Startup

**Files:**
- Modify: `frontend/src/main.ts`

The app must apply the saved theme mode on startup, and start the system mode listener.

- [x] **Step 1: Update main.ts**

In `frontend/src/main.ts`, update the imports (line 10) from:
```typescript
import { loadSettings, initThemes } from "@/stores/app";
```
to:
```typescript
import { loadSettings, initThemes, startSystemModeListener } from "@/stores/app";
```

Then update the startup sequence (lines 24-26) from:
```typescript
initThemes();
loadSettings();
app.mount("#app");
```
to:
```typescript
initThemes();
startSystemModeListener();
loadSettings();
app.mount("#app");
```

**Note:** `loadSettings` is async but not awaited — this is existing behavior. The `initThemes` runs first with default `theme: "dark"`, then `loadSettings` updates `appState.theme`. We need to apply the mode after settings load. Add a watcher in app.ts instead.

- [x] **Step 2: Add watcher for theme changes in app.ts**

In `frontend/src/stores/app.ts`, add `watch` to the imports (line 1):
```typescript
import { reactive, computed, watch } from "vue";
```

Then add a watcher after `applyMode` function:
```typescript
// Apply mode whenever theme changes (e.g. after loadSettings populates appState)
watch(
  () => appState.theme,
  (newMode) => {
    applyMode(newMode as ThemeMode);
  }
);
```

- [x] **Step 3: Type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 7: Add Theme Mode Tests

**Files:**
- Create: `frontend/src/stores/app.test.ts`

- [x] **Step 1: Write test file**

Create `frontend/src/stores/app.test.ts`:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/lib/monaco-themes", () => ({
  accentThemes: {
    blue: { label: "Blue", color: "#4285f4", monacoTheme: "nknk-blue", monacoLightTheme: "nknk-light-blue" },
  },
  applyMonacoTheme: vi.fn(),
  applyMonacoThemeForMode: vi.fn(),
  registerAllThemes: vi.fn(),
}));

vi.mock("@/api/services", () => ({
  settingsService: {
    loadSettings: vi.fn().mockResolvedValue({}),
    saveSettings: vi.fn().mockResolvedValue(undefined),
  },
}));

vi.mock("@wailsio/runtime", () => ({
  Events: { On: vi.fn() },
}));

import { appState, applyMode, resolveSystemMode } from "./app";

describe("Theme Mode", () => {
  beforeEach(() => {
    document.documentElement.removeAttribute("data-mode");
    appState.theme = "dark";
    appState.accentTheme = "blue";
  });

  it("resolveSystemMode returns 'dark' or 'light'", () => {
    const mode = resolveSystemMode();
    expect(["dark", "light"]).toContain(mode);
  });

  it("applyMode('dark') sets data-mode to dark", () => {
    applyMode("dark");
    expect(document.documentElement.getAttribute("data-mode")).toBe("dark");
  });

  it("applyMode('light') sets data-mode to light", () => {
    applyMode("light");
    expect(document.documentElement.getAttribute("data-mode")).toBe("light");
  });

  it("applyMode('system') sets data-mode to resolved system mode", () => {
    applyMode("system");
    const resolved = resolveSystemMode();
    expect(document.documentElement.getAttribute("data-mode")).toBe(resolved);
  });

  it("applyMode updates appState.theme", () => {
    applyMode("light");
    expect(appState.theme).toBe("light");
  });
});
```

- [x] **Step 2: Run tests**

Run: `cd frontend && npx vitest run src/stores/app.test.ts`
Expected: All 5 tests pass

---

### Task 8: Full Verification

- [x] **Step 1: Go tests (unchanged, sanity check)**

Run: `cd e:\gugacode\gugacode\gugacode && go test ./services/...`
Expected: ok gugacode/services

- [x] **Step 2: Frontend type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [x] **Step 3: Frontend tests**

Run: `cd frontend && npx vitest run`
Expected: All tests pass (82 existing + 5 new = 87)

- [x] **Step 4: Verify light tokens exist in CSS**

Run: `grep "data-mode=\"light\"" frontend/src/assets/styles/main.css`
Expected: At least one match

- [x] **Step 5: Verify light Monaco themes registered**

Run: `grep "createLightThemeData" frontend/src/lib/monaco-themes.ts`
Expected: At least 2 matches (function definition + call in registerAllThemes)

- [x] **Step 6: Verify applyMode exists in app.ts**

Run: `grep "export function applyMode" frontend/src/stores/app.ts`
Expected: `export function applyMode(mode: ThemeMode): void {`

- [x] **Step 7: Verify SettingsView calls applyMode**

Run: `grep "applyMode" frontend/src/views/SettingsView.vue`
Expected: At least 2 matches (import + call in handleThemeChange)
