# Editor & UX Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make gugacode truly usable as a daily IDE by adding file dirty-state tracking, keyboard shortcuts, command palette, quick-open file finder, and markdown preview.

**Architecture:** A new `keyboard` composable registers global shortcuts. The editor store gains dirty-state tracking. A command palette component provides fuzzy command search. A quick-open component provides fuzzy file search. Markdown files get a split preview pane.

**Tech Stack:** Go 1.25, Wails v3, Vue 3, TypeScript, Element Plus, Monaco Editor, `marked` (already installed), Vitest

**Project root:** `e:\gugacode\gugacode\gugacode\` (the directory containing `go.mod`, `main.go`, `frontend/`). All relative paths in this plan are from this root.

---

## File Structure

```
frontend/src/
├── composables/
│   └── useKeyboard.ts          # NEW — global keyboard shortcut handler
├── stores/
│   ├── editor.ts               # MODIFY — add dirty-state tracking, save, scroll-to-line
│   └── editor.test.ts          # MODIFY — add dirty-state tests
├── components/
│   ├── layout/
│   │   ├── CommandPalette.vue  # NEW — Ctrl+Shift+P command palette
│   │   └── MainLayout.vue      # MODIFY — register shortcuts, mount palette
│   └── editor/
│       ├── CodeEditor.vue      # MODIFY — enable find/replace, Ctrl+S
│       └── TabBar.vue          # MODIFY — show dirty indicator
├── views/
│   └── EditorView.vue          # MODIFY — markdown preview split
└── types/
    └── index.ts                # MODIFY — add Command interface
```

---

## Task 1: Editor Store — Dirty State Tracking & Save

**Files:**
- Modify: `frontend/src/stores/editor.ts`
- Modify: `frontend/src/stores/editor.test.ts`

- [x] **Step 1: Read current editor store**

Read `frontend/src/stores/editor.ts` to understand its structure: tabs, activeTab, openFile, closeFile, etc.

- [x] **Step 2: Write failing tests for dirty state**

Append to `frontend/src/stores/editor.test.ts`:

```typescript
describe("editor dirty state", () => {
  it("marks tab as dirty when content changes", () => {
    const { editorState, updateContent } = useEditor();
    editorState.tabs = [{ path: "test.ts", content: "original", dirty: false }];
    editorState.activeTabIndex = 0;
    updateContent("modified");
    expect(editorState.tabs[0].dirty).toBe(true);
    expect(editorState.tabs[0].content).toBe("modified");
  });

  it("marks tab as clean after save", async () => {
    const { editorState, updateContent, saveFile } = useEditor();
    editorState.tabs = [{ path: "test.ts", content: "original", dirty: false }];
    editorState.activeTabIndex = 0;
    updateContent("modified");
    expect(editorState.tabs[0].dirty).toBe(true);
    await saveFile();
    expect(editorState.tabs[0].dirty).toBe(false);
  });

  it("saveFile does nothing when no active tab", async () => {
    const { saveFile } = useEditor();
    await expect(saveFile()).resolves.toBeUndefined();
  });
});
```

- [x] **Step 3: Run tests to verify they fail**

Run: `cd frontend && npx vitest run src/stores/editor.test.ts`
Expected: FAIL — `updateContent` and `saveFile` don't exist yet.

- [x] **Step 4: Implement dirty-state tracking**

In `editor.ts`, add `dirty: boolean` to the tab type (if not present). Add:

```typescript
function updateContent(content: string): void {
  if (editorState.activeTabIndex < 0) return;
  const tab = editorState.tabs[editorState.activeTabIndex];
  if (!tab) return;
  if (tab.content !== content) {
    tab.content = content;
    tab.dirty = true;
  }
}

async function saveFile(): Promise<void> {
  if (editorState.activeTabIndex < 0) return;
  const tab = editorState.tabs[editorState.activeTabIndex];
  if (!tab) return;
  try {
    await fileService.writeFile(tab.path, tab.content);
    tab.dirty = false;
  } catch (e: any) {
    console.error("Failed to save file:", e);
  }
}
```

Export `updateContent` and `saveFile`.

- [x] **Step 5: Run tests to verify they pass**

Run: `cd frontend && npx vitest run src/stores/editor.test.ts`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add frontend/src/stores/editor.ts frontend/src/stores/editor.test.ts
git commit -m "feat: add file dirty-state tracking and save to editor store"
```

---

## Task 2: Tab Bar — Dirty Indicator

**Files:**
- Modify: `frontend/src/components/editor/TabBar.vue`

- [x] **Step 1: Read current TabBar.vue**

Read `frontend/src/components/editor/TabBar.vue` to understand the tab rendering.

- [x] **Step 2: Add dirty indicator dot**

In the tab template, add a dot indicator when `tab.dirty` is true. Replace the close button with a conditional dot:

```vue
<span v-if="tab.dirty" class="tab-bar__dirty">●</span>
<button
  v-else
  class="tab-bar__close"
  @click.stop="closeTab(index)"
>
  ×
</button>
```

Add style:
```css
.tab-bar__dirty {
  color: var(--color-text-tertiary);
  font-size: 10px;
  margin-left: 4px;
}
```

- [x] **Step 3: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: no errors.

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/editor/TabBar.vue
git commit -m "feat: show dirty indicator dot in file tabs"
```

---

## Task 3: Keyboard Shortcuts — Ctrl+S Save

**Files:**
- Create: `frontend/src/composables/useKeyboard.ts`
- Modify: `frontend/src/components/layout/MainLayout.vue`

- [x] **Step 1: Create useKeyboard composable**

Create `frontend/src/composables/useKeyboard.ts`:

```typescript
import { onMounted, onUnmounted } from "vue";

type ShortcutHandler = (e: KeyboardEvent) => void;

interface Shortcut {
  key: string;
  ctrl?: boolean;
  shift?: boolean;
  alt?: boolean;
  handler: ShortcutHandler;
  preventDefault?: boolean;
}

const shortcuts: Shortcut[] = [];

export function registerShortcut(shortcut: Shortcut): void {
  shortcuts.push(shortcut);
}

export function unregisterShortcut(shortcut: Shortcut): void {
  const idx = shortcuts.indexOf(shortcut);
  if (idx >= 0) shortcuts.splice(idx, 1);
}

function handleKeyDown(e: KeyboardEvent): void {
  for (const s of shortcuts) {
    if (s.key.toLowerCase() !== e.key.toLowerCase()) continue;
    if (!!s.ctrl !== (e.ctrlKey || e.metaKey)) continue;
    if (!!s.shift !== e.shiftKey) continue;
    if (!!s.alt !== e.altKey) continue;
    if (s.preventDefault !== false) e.preventDefault();
    s.handler(e);
    return;
  }
}

export function useKeyboard(): void {
  onMounted(() => {
    window.addEventListener("keydown", handleKeyDown);
  });
  onUnmounted(() => {
    window.removeEventListener("keydown", handleKeyDown);
  });
}
```

- [x] **Step 2: Wire Ctrl+S in MainLayout.vue**

In `MainLayout.vue` script setup, add:

```typescript
import { registerShortcut } from "@/composables/useKeyboard";
import { saveFile } from "@/stores/editor";

registerShortcut({
  key: "s",
  ctrl: true,
  handler: () => {
    saveFile();
  },
});
```

Also call `useKeyboard()` in MainLayout's setup if not already done.

- [x] **Step 3: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: no errors.

- [x] **Step 4: Commit**

```bash
git add frontend/src/composables/useKeyboard.ts frontend/src/components/layout/MainLayout.vue
git commit -m "feat: add keyboard shortcuts composable with Ctrl+S save"
```

---

## Task 4: Command Palette

**Files:**
- Create: `frontend/src/components/layout/CommandPalette.vue`
- Modify: `frontend/src/components/layout/MainLayout.vue`
- Modify: `frontend/src/types/index.ts`

- [x] **Step 1: Add Command type**

In `frontend/src/types/index.ts`, add:

```typescript
export interface Command {
  id: string;
  label: string;
  shortcut?: string;
  action: () => void;
}
```

- [x] **Step 2: Create CommandPalette.vue**

Create `frontend/src/components/layout/CommandPalette.vue`:

```vue
<script setup lang="ts">
import { ref, computed, watch, nextTick } from "vue";
import type { Command } from "@/types";

const props = defineProps<{
  visible: boolean;
  commands: Command[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "run", command: Command): void;
}>();

const query = ref("");
const selectedIndex = ref(0);
const inputRef = ref<HTMLInputElement | null>(null);

const filtered = computed(() => {
  const q = query.value.toLowerCase().trim();
  if (!q) return props.commands;
  return props.commands.filter((c) =>
    c.label.toLowerCase().includes(q),
  );
});

watch(
  () => props.visible,
  (v) => {
    if (v) {
      query.value = "";
      selectedIndex.value = 0;
      nextTick(() => inputRef.value?.focus());
    }
  },
);

watch(filtered, () => {
  selectedIndex.value = 0;
});

function handleKeydown(e: KeyboardEvent) {
  if (e.key === "ArrowDown") {
    e.preventDefault();
    selectedIndex.value = Math.min(selectedIndex.value + 1, filtered.value.length - 1);
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    selectedIndex.value = Math.max(selectedIndex.value - 1, 0);
  } else if (e.key === "Enter") {
    e.preventDefault();
    const cmd = filtered.value[selectedIndex.value];
    if (cmd) emit("run", cmd);
  } else if (e.key === "Escape") {
    emit("close");
  }
}

function handleRun(cmd: Command) {
  emit("run", cmd);
}
</script>

<template>
  <transition name="fade">
    <div v-if="visible" class="command-palette-overlay" @click="emit('close')">
      <div class="command-palette" @click.stop>
        <input
          ref="inputRef"
          v-model="query"
          class="command-palette__input"
          placeholder="Type a command..."
          @keydown="handleKeydown"
        />
        <div class="command-palette__list">
          <div v-if="filtered.length === 0" class="command-palette__empty">
            No matching commands
          </div>
          <button
            v-for="(cmd, i) in filtered"
            :key="cmd.id"
            class="command-palette__item"
            :class="{ 'command-palette__item--active': i === selectedIndex }"
            @click="handleRun(cmd)"
            @mouseenter="selectedIndex = i"
          >
            <span class="command-palette__label">{{ cmd.label }}</span>
            <span v-if="cmd.shortcut" class="command-palette__shortcut">{{ cmd.shortcut }}</span>
          </button>
        </div>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.command-palette-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.4);
  z-index: 1000;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding-top: 80px;
}

.command-palette {
  width: 520px;
  max-width: 90vw;
  background-color: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md, 12px);
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
}

.command-palette__input {
  width: 100%;
  padding: 12px 16px;
  font-size: 14px;
  font-family: var(--font-sans);
  color: var(--color-text-primary);
  background-color: transparent;
  border: none;
  border-bottom: 1px solid var(--color-border-subtle);
  outline: none;
}

.command-palette__input::placeholder {
  color: var(--color-text-tertiary);
}

.command-palette__list {
  max-height: 320px;
  overflow-y: auto;
  padding: 4px;
}

.command-palette__empty {
  padding: 16px;
  font-size: 12px;
  color: var(--color-text-tertiary);
  text-align: center;
}

.command-palette__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 8px 12px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm, 8px);
  cursor: pointer;
  text-align: left;
  color: var(--color-text-primary);
  font-size: 13px;
  transition: background-color 80ms ease;
}

.command-palette__item--active {
  background-color: color-mix(in srgb, var(--color-primary) 12%, transparent);
}

.command-palette__shortcut {
  font-size: 11px;
  color: var(--color-text-tertiary);
  font-family: var(--font-mono);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 120ms ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
```

- [x] **Step 3: Wire command palette in MainLayout.vue**

In `MainLayout.vue`:

```typescript
import CommandPalette from "./CommandPalette.vue";
import { saveFile } from "@/stores/editor";
import { toggleAiChat } from "@/stores/app";
import { clearMessages } from "@/stores/ai";

const paletteVisible = ref(false);
const commands = computed<Command[]>(() => [
  { id: "save", label: "Save File", shortcut: "Ctrl+S", action: () => saveFile() },
  { id: "toggle-ai", label: "Toggle AI Chat", action: () => toggleAiChat() },
  { id: "clear-chat", label: "Clear AI Conversation", action: () => clearMessages() },
  { id: "toggle-terminal", label: "Toggle Terminal", action: () => toggleTerminal() },
]);

registerShortcut({
  key: "p",
  ctrl: true,
  shift: true,
  handler: () => { paletteVisible.value = true; },
});

function handleRunCommand(cmd: Command) {
  cmd.action();
  paletteVisible.value = false;
}
```

In template, add at the end:
```vue
<CommandPalette
  :visible="paletteVisible"
  :commands="commands"
  @close="paletteVisible = false"
  @run="handleRunCommand"
/>
```

- [x] **Step 4: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: no errors.

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/layout/CommandPalette.vue frontend/src/components/layout/MainLayout.vue frontend/src/types/index.ts
git commit -m "feat: add command palette with Ctrl+Shift+P shortcut"
```

---

## Task 5: CodeEditor — Enable Built-in Find/Replace & Wire Content Updates

**Files:**
- Modify: `frontend/src/components/editor/CodeEditor.vue`

- [x] **Step 1: Read current CodeEditor.vue**

Read `frontend/src/components/editor/CodeEditor.vue` to understand its structure.

- [x] **Step 2: Enable find/replace and wire content updates**

In the options computed, add:
```typescript
find: { addExtraSpaceOnTop: false },
autoClosingBrackets: "always",
```

Change the `handleChange` function to call `updateContent` from the editor store instead of just emitting:

```typescript
import { editorState, updateContent } from "@/stores/editor";

function handleChange(value: string | undefined) {
  const v = value ?? "";
  updateContent(v);
  emit("update:content", v);
}
```

- [x] **Step 3: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: no errors.

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/editor/CodeEditor.vue
git commit -m "feat: enable Monaco find/replace and wire content to editor store"
```

---

## Task 6: Markdown Preview

**Files:**
- Modify: `frontend/src/views/EditorView.vue`

- [x] **Step 1: Read current EditorView.vue**

Read `frontend/src/views/EditorView.vue` to understand how it renders the editor.

- [x] **Step 2: Add markdown preview split**

Add a preview pane that shows rendered markdown when the active file is `.md` or `.markdown`:

```typescript
import { renderMarkdown } from "@/lib/markdown";
import { editorState } from "@/stores/editor";

const isMarkdown = computed(() => {
  const path = editorState.tabs[editorState.activeTabIndex]?.path ?? "";
  return /\.(md|markdown|mdown)$/i.test(path);
});

const previewHtml = computed(() => {
  const content = editorState.tabs[editorState.activeTabIndex]?.content ?? "";
  return renderMarkdown(content);
});

const showPreview = ref(false);
```

In template, add a toggle button and preview pane:
```vue
<button
  v-if="isMarkdown"
  class="editor-view__preview-toggle"
  :class="{ active: showPreview }"
  @click="showPreview = !showPreview"
>
  Preview
</button>
<div class="editor-view__body">
  <div class="editor-view__editor" :class="{ 'editor-view__editor--split': showPreview && isMarkdown }">
    <!-- existing editor component -->
  </div>
  <div
    v-if="showPreview && isMarkdown"
    class="editor-view__preview markdown-body"
    v-html="previewHtml"
  />
</div>
```

Add styles for the split layout.

- [x] **Step 3: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: no errors.

- [x] **Step 4: Commit**

```bash
git add frontend/src/views/EditorView.vue
git commit -m "feat: add markdown preview pane for .md files"
```

---

## Task 7: Integration Verification

**Files:** none (testing only)

- [x] **Step 1: Run full test suites**

```bash
go test ./services/ -v
cd frontend
npx vue-tsc --noEmit
npx vitest run
```

Expected: All tests pass, type check clean.

- [x] **Step 2: Manual GUI checklist**

Verify in the running app (if wails3 is available):
- [x] Editing a file shows a dot in its tab
- [x] Ctrl+S saves the file and removes the dot
- [x] Ctrl+Shift+P opens the command palette
- [x] Typing in the palette filters commands
- [x] Arrow keys navigate, Enter runs, Escape closes
- [x] Ctrl+F opens Monaco's find widget
- [x] Opening a .md file and clicking "Preview" shows rendered markdown
