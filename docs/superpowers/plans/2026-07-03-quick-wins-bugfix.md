# Quick Wins & Bug Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 9 P0/P1 bugs and code quality issues identified in prompt.md (B-2, B-5, B-6, B-8, B-9, Q-2, Q-3, Q-7, Q-10) to bring the IDE to a stable, usable baseline.

**Architecture:** Direct file edits following TDD where applicable. Backend Go changes use `go test` verification; frontend changes use `vue-tsc --noEmit` + `vitest run`. Each task is independent and produces a self-contained commit.

**Tech Stack:** Go 1.25, Vue 3 + TypeScript, Element Plus, xterm.js, Wails v3

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `greetservice.go` | Legacy demo service | Delete |
| `main.go` | Service registration | Remove GreetService registration |
| `frontend/bindings/changeme/greetservice.js` | Legacy binding | Delete |
| `services/project_service.go` | Project ID + sorting | Modify (Q-2, Q-3) |
| `services/project_service_test.go` | Project service tests | Add tests |
| `frontend/src/components/explorer/FileTree.vue` | File tree node | Modify (Q-7) |
| `frontend/src/components/layout/TerminalPanel.vue` | Terminal UI | Modify (B-8, B-9) |
| `frontend/src/components/layout/TitleBar.vue` | Menu bar | Modify (B-6) |
| `frontend/src/views/ProjectsView.vue` | Projects page | Modify CSS tokens (B-5) |
| `frontend/src/views/PluginsView.vue` | Plugins page | Modify CSS tokens (B-5) |
| `frontend/src/views/SettingsView.vue` | Settings page | Modify accent selector (B-2) |

---

### Task 1: Delete GreetService (Q-10)

**Files:**
- Delete: `greetservice.go`
- Delete: `frontend/bindings/changeme/greetservice.js`
- Modify: `main.go` (remove registration line)

- [ ] **Step 1: Delete greetservice.go**

Delete the file `greetservice.go` at project root.

- [ ] **Step 2: Delete the binding file**

Delete `frontend/bindings/changeme/greetservice.js`.

- [ ] **Step 3: Remove GreetService registration from main.go**

In `main.go`, find the line that registers GreetService:

```go
application.NewService(&GreetService{}),
```

Remove that line from the services slice.

- [ ] **Step 4: Verify build**

Run: `go build .`
Expected: success (no errors)

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success (no errors)

- [ ] **Step 5: Commit**

```bash
git add greetservice.go frontend/bindings/changeme/greetservice.js main.go
git commit -m "chore: remove legacy GreetService (Q-10)"
```

---

### Task 2: Replace bubble sort with sort.Slice (Q-2)

**Files:**
- Modify: `services/project_service.go:127-135`
- Test: `services/project_service_test.go`

- [ ] **Step 1: Write the failing test**

Add to `services/project_service_test.go`:

```go
func TestSortProjectsByRecency_DescendingOrder(t *testing.T) {
	projects := []Project{
		{ID: "a", LastOpened: 100},
		{ID: "b", LastOpened: 300},
		{ID: "c", LastOpened: 200},
	}
	sortProjectsByRecency(projects)
	if projects[0].ID != "b" {
		t.Errorf("expected 'b' (300) first, got %s", projects[0].ID)
	}
	if projects[1].ID != "c" {
		t.Errorf("expected 'c' (200) second, got %s", projects[1].ID)
	}
	if projects[2].ID != "a" {
		t.Errorf("expected 'a' (100) third, got %s", projects[2].ID)
	}
}

func TestSortProjectsByRecency_EmptyAndSingle(t *testing.T) {
	sortProjectsByRecency(nil)
	sortProjectsByRecency([]Project{{ID: "x", LastOpened: 1}})
}
```

- [ ] **Step 2: Run test to verify it passes (bubble sort already works)**

Run: `go test ./services/ -run TestSortProjectsByRecency -v`
Expected: PASS (current bubble sort already produces correct order; this test locks behavior before refactor)

- [ ] **Step 3: Replace bubble sort with sort.Slice**

In `services/project_service.go`, replace the `sortProjectsByRecency` function:

```go
func sortProjectsByRecency(projects []Project) {
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastOpened > projects[j].LastOpened
	})
}
```

Add `"sort"` to the import block.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestSortProjectsByRecency -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/project_service.go services/project_service_test.go
git commit -m "refactor: replace bubble sort with sort.Slice (Q-2)"
```

---

### Task 3: Project.ID with crypto/rand (Q-3)

**Files:**
- Modify: `services/project_service.go:88-104` (AddProject method)
- Test: `services/project_service_test.go`

- [ ] **Step 1: Write the failing test**

Add to `services/project_service_test.go`:

```go
func TestProjectService_AddProjectGeneratesUniqueIDs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "projects.json")
	svc := &ProjectService{configPath: configPath}

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	p1, err := svc.AddProject(dir1)
	if err != nil {
		t.Fatalf("AddProject dir1 failed: %v", err)
	}
	p2, err := svc.AddProject(dir2)
	if err != nil {
		t.Fatalf("AddProject dir2 failed: %v", err)
	}

	if p1.ID == p2.ID {
		t.Errorf("expected unique IDs, got duplicate %s", p1.ID)
	}
	if len(p1.ID) < 16 {
		t.Errorf("expected ID length >= 16, got %d", len(p1.ID))
	}
}
```

- [ ] **Step 2: Run test to verify it fails or is flaky**

Run: `go test ./services/ -run TestProjectService_AddProjectGeneratesUniqueIDs -v`
Expected: may PASS or FAIL depending on timing (millisecond timestamp IDs can collide)

- [ ] **Step 3: Replace timestamp ID with crypto/rand hex**

In `services/project_service.go`, add a helper function after the imports:

```go
func generateProjectID() string {
	b := make([]byte, 8)
	_, _ = crypto_rand.Read(b)
	return hex.EncodeToString(b)
}
```

Add imports `"crypto/rand"` (aliased as `crypto_rand` to avoid conflict) and `"encoding/hex"`.

In the `AddProject` method, replace:
```go
proj := Project{
	ID:         fmt.Sprintf("%d", now),
```
with:
```go
proj := Project{
	ID:         generateProjectID(),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestProjectService_AddProjectGeneratesUniqueIDs -v`
Expected: PASS (crypto/rand guarantees uniqueness)

- [ ] **Step 5: Run full test suite**

Run: `go test ./services/ -count=1`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add services/project_service.go services/project_service_test.go
git commit -m "fix: use crypto/rand for Project.ID instead of timestamp (Q-3)"
```

---

### Task 4: FileTree isFolder to computed (Q-7)

**Files:**
- Modify: `frontend/src/components/explorer/FileTree.vue:27`

- [ ] **Step 1: Change isFolder from plain assignment to computed**

In `frontend/src/components/explorer/FileTree.vue`, update the import:

```typescript
import { ref, computed } from "vue";
```

Replace line 27:
```typescript
const isFolder = props.depth === 0 || props.isDir;
```

with:
```typescript
const isFolder = computed(() => props.depth === 0 || props.isDir);
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [ ] **Step 3: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/explorer/FileTree.vue
git commit -m "fix: make FileTree isFolder reactive with computed (Q-7)"
```

---

### Task 5: TerminalPanel fontFamily fix (B-9)

**Files:**
- Modify: `frontend/src/components/layout/TerminalPanel.vue:43`

- [ ] **Step 1: Replace CSS variable with literal font name**

In `frontend/src/components/layout/TerminalPanel.vue`, find line 43:
```typescript
fontFamily: "var(--font-mono)",
```

Replace with:
```typescript
fontFamily: "JetBrains Mono, Consolas, 'Courier New', monospace",
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/layout/TerminalPanel.vue
git commit -m "fix: use literal font name for xterm fontFamily (B-9)"
```

---

### Task 6: TerminalPanel activeTab to ref (B-8)

**Files:**
- Modify: `frontend/src/components/layout/TerminalPanel.vue:29,112-118`

- [ ] **Step 1: Change activeTab from const string to ref**

In `frontend/src/components/layout/TerminalPanel.vue`, find line 29:
```typescript
const activeTab = "terminal";
```

Replace with:
```typescript
const activeTab = ref("terminal");
```

- [ ] **Step 2: Add click handler to tab buttons**

Find the tab button template (around line 108-118):
```html
<button
  v-for="tab in tabs"
  :key="tab.key"
  class="terminal-panel__tab"
  :class="{ 'terminal-panel__tab--active': activeTab === tab.key }"
  role="tab"
  :aria-selected="activeTab === tab.key"
  :aria-label="tab.label + ' tab'"
>
  {{ tab.label }}
</button>
```

Replace with:
```html
<button
  v-for="tab in tabs"
  :key="tab.key"
  class="terminal-panel__tab"
  :class="{ 'terminal-panel__tab--active': activeTab === tab.key }"
  role="tab"
  :aria-selected="activeTab === tab.key"
  :aria-label="tab.label + ' tab'"
  @click="activeTab = tab.key"
>
  {{ tab.label }}
</button>
```

- [ ] **Step 3: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/TerminalPanel.vue
git commit -m "fix: make TerminalPanel activeTab reactive ref (B-8)"
```

---

### Task 7: TitleBar Edit/View menu handlers (B-6)

**Files:**
- Modify: `frontend/src/components/layout/TitleBar.vue:17-29`

- [ ] **Step 1: Add Edit and View menu handlers**

In `frontend/src/components/layout/TitleBar.vue`, replace the `handleMenu` function:

```typescript
function handleMenu(action: string) {
  switch (action) {
    case "file":
      router.push("/welcome");
      break;
    case "edit":
      // Focus the editor for editing commands
      router.push("/editor");
      break;
    case "view":
      // Toggle sidebar visibility
      toggleSidebar();
      break;
    case "terminal":
      router.push("/editor");
      toggleTerminal();
      break;
    case "help":
      window.open("https://v3.wails.io/", "_blank");
      break;
  }
}
```

Add `toggleSidebar` and `toggleTerminal` to the import from `@/stores/app`:

```typescript
import { appState, toggleSidebar, toggleTerminal } from "@/stores/app";
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [ ] **Step 3: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/TitleBar.vue
git commit -m "fix: wire TitleBar Edit/View menu handlers (B-6)"
```

---

### Task 8: Fix CSS token mismatch in ProjectsView (B-5)

**Files:**
- Modify: `frontend/src/views/ProjectsView.vue` (lines 114, 141, 211-212, 262-263)

- [ ] **Step 1: Replace --color-background with --color-bg-base**

In `frontend/src/views/ProjectsView.vue`, replace all occurrences of `var(--color-background, #111111)` with `var(--color-bg-base)`:

Line 114:
```css
background-color: var(--color-bg-base);
```

Line 141:
```css
color: var(--color-bg-base) !important;
```

- [ ] **Step 2: Replace --duration-fast and --ease-out-expo with --transition-fast**

Lines 211-212:
```css
transition: border-color var(--transition-fast),
            background-color var(--transition-fast);
```

Lines 262-263:
```css
transition: color var(--transition-fast),
            background-color var(--transition-fast);
```

- [ ] **Step 3: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/ProjectsView.vue
git commit -m "fix: use correct CSS design tokens in ProjectsView (B-5)"
```

---

### Task 9: Fix CSS token mismatch in PluginsView (B-5)

**Files:**
- Modify: `frontend/src/views/PluginsView.vue` (lines 115, 255, 279)

- [ ] **Step 1: Replace --color-background with --color-bg-base**

In `frontend/src/views/PluginsView.vue`, replace all occurrences of `var(--color-background, #111111)` with `var(--color-bg-base)`:

Line 115:
```css
background-color: var(--color-bg-base);
```

Line 255:
```css
color: var(--color-bg-base) !important;
```

Line 279:
```css
color: var(--color-bg-base) !important;
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/PluginsView.vue
git commit -m "fix: use correct CSS design tokens in PluginsView (B-5)"
```

---

### Task 10: Fix Color Accent selector (B-2)

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

- [ ] **Step 1: Import accentThemes and applyAccentTheme**

In `frontend/src/views/SettingsView.vue`, update the script imports:

```typescript
import { ref } from "vue";
import { appState, saveSettings, applyAccentTheme } from "@/stores/app";
import { accentThemes } from "@/lib/monaco-themes";
import type { AccentTheme } from "@/lib/monaco-themes";
import { fileService, aiService, aiServiceV2 } from "@/api/services";
import { Folder, Hide, View, Pointer } from "@element-plus/icons-vue";
```

- [ ] **Step 2: Replace accentColors array with accentThemes-derived list**

Remove the `accentColors` array (lines ~39-48):
```typescript
const accentColors = [
  "#a0c4ff",
  "#ff6b6b",
  "#51cf66",
  "#ffd43b",
  "#cc5de8",
  "#ff922b",
  "#20c997",
  "#748ffc",
];
```

Replace with:
```typescript
const accentColorList = Object.entries(accentThemes).map(([key, meta]) => ({
  key: key as AccentTheme,
  label: meta.label,
  color: meta.color,
}));
```

- [ ] **Step 3: Add selectAccent function**

Add this function after `handleThemeChange`:

```typescript
function selectAccent(key: AccentTheme) {
  applyAccentTheme(key);
  saveSettings();
}
```

- [ ] **Step 4: Update the color swatch template**

Find the color swatches template (around line 585-596) and replace:

```html
<div class="color-swatches">
  <button
    v-for="item in accentColorList"
    :key="item.key"
    class="color-swatch"
    :class="{ 'is-selected': appState.accentTheme === item.key }"
    :style="{ backgroundColor: item.color }"
    :aria-label="'Select accent color ' + item.label"
    :aria-pressed="appState.accentTheme === item.key"
    @click="selectAccent(item.key)"
  />
</div>
```

- [ ] **Step 5: Remove the old accentColor ref**

Remove this line from the script:
```typescript
const accentColor = ref("#a0c4ff");
```

- [ ] **Step 6: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [ ] **Step 7: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add frontend/src/views/SettingsView.vue
git commit -m "fix: wire Color Accent selector to accentThemes + applyAccentTheme (B-2)"
```

---

### Task 11: Full verification

- [ ] **Step 1: Run Go tests**

Run: `go test ./services/... -count=1 -timeout 60s`
Expected: all PASS

- [ ] **Step 2: Run Go build**

Run: `go build .`
Expected: success

- [ ] **Step 3: Run frontend type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [ ] **Step 4: Run frontend tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [ ] **Step 5: Commit if any fixup needed**

```bash
git add -A
git commit -m "chore: verification fixes for Plan 9"
```
