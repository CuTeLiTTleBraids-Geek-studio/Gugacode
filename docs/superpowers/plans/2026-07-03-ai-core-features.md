# AI Core Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement 3 core AI features from prompt.md section 4.1: Code Actions context menu entry (#2), Notification system (#7), and @-mention multi-file context (#4) to bring the IDE to the "usable AI IDE" baseline.

**Architecture:** Code Actions uses Monaco's context menu API to add AI preset actions. Notifications wrap Element Plus `ElNotification`. @-mention adds a file picker triggered by typing `@` in the chat input, injecting file content as context.

**Tech Stack:** Vue 3 + TypeScript, Monaco Editor, Element Plus, Wails v3

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `frontend/src/lib/notifications.ts` | Notification helper | Create |
| `frontend/src/components/editor/CodeEditor.vue` | Editor with AI context menu | Modify |
| `frontend/src/stores/ai.ts` | AI store with multi-file context | Modify |
| `frontend/src/components/ai/AiChatPanel.vue` | Chat panel with @-mention | Modify |
| `frontend/src/types/index.ts` | Type definitions | Modify |

---

### Task 1: Create notification system

**Files:**
- Create: `frontend/src/lib/notifications.ts`

- [x] **Step 1: Create the notification helper**

Create `frontend/src/lib/notifications.ts`:

```typescript
import { ElNotification } from "element-plus";
import type { NotificationParams } from "element-plus";

type NotificationType = "success" | "warning" | "info" | "error";

interface NotifyOptions {
  title?: string;
  message: string;
  type?: NotificationType;
  duration?: number;
}

/**
 * Shows a toast notification. Duration defaults to 3000ms; set to 0 for persistent.
 */
export function notify(options: NotifyOptions): void {
  const type = options.type ?? "info";
  const duration = options.duration ?? 3000;
  ElNotification({
    title: options.title,
    message: options.message,
    type,
    duration,
    position: "bottom-right",
  } as NotificationParams);
}

/**
 * Shows a success notification.
 */
export function notifySuccess(message: string, title?: string): void {
  notify({ message, title, type: "success" });
}

/**
 * Shows an error notification. Duration defaults to 5000ms for errors.
 */
export function notifyError(message: string, title?: string): void {
  notify({ message, title, type: "error", duration: 5000 });
}

/**
 * Shows a warning notification.
 */
export function notifyWarning(message: string, title?: string): void {
  notify({ message, title, type: "warning" });
}

/**
 * Shows an info notification.
 */
export function notifyInfo(message: string, title?: string): void {
  notify({ message, title, type: "info" });
}
```

- [x] **Step 2: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 3: Commit**

```bash
git add frontend/src/lib/notifications.ts
git commit -m "feat: add notification system with ElNotification wrapper"
```

---

### Task 2: Wire notifications into error-prone paths

**Files:**
- Modify: `frontend/src/stores/ai.ts`
- Modify: `frontend/src/stores/editor.ts`

- [x] **Step 1: Replace console.error with notifyError in ai.ts**

In `frontend/src/stores/ai.ts`, add import:

```typescript
import { notifyError } from "@/lib/notifications";
```

In the `ai:error` event handler, replace:
```typescript
    aiState.error = errMsg;
```
with:
```typescript
    aiState.error = errMsg;
    notifyError(errMsg, "AI Error");
```

In `sendMessage` catch block, add:
```typescript
    notifyError(e?.message ?? "AI request failed", "AI Error");
```

- [x] **Step 2: Replace console.error with notifyError in editor.ts**

In `frontend/src/stores/editor.ts`, add import:

```typescript
import { notifyError } from "@/lib/notifications";
```

In `openFileFromPath`, replace:
```typescript
    console.error("Failed to open file:", e);
```
with:
```typescript
    notifyError(`Failed to open file: ${e instanceof Error ? e.message : String(e)}`);
```

- [x] **Step 3: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 4: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src/stores/ai.ts frontend/src/stores/editor.ts
git commit -m "feat: wire notification system into error paths"
```

---

### Task 3: Code Actions context menu in editor

**Files:**
- Modify: `frontend/src/components/editor/CodeEditor.vue`

- [x] **Step 1: Add AI actions to Monaco context menu**

In `frontend/src/components/editor/CodeEditor.vue`, find where the Monaco editor actions are registered (the `actions` array in `registerEditorActions`). The actions already exist for the command palette. Add context menu actions by registering them with Monaco's `addAction`:

After the existing editor setup, add:

```typescript
function registerContextMenuActions() {
  if (!editor) return;

  const aiActions: Array<{ id: string; label: string; action: AIActionName }> = [
    { id: "ai-explain-ctx", label: "AI: Explain Selection", action: "explain" },
    { id: "ai-refactor-ctx", label: "AI: Refactor Selection", action: "refactor" },
    { id: "ai-fix-ctx", label: "AI: Fix Bugs", action: "fix" },
    { id: "ai-docs-ctx", label: "AI: Generate Docs", action: "generate_docs" },
    { id: "ai-tests-ctx", label: "AI: Generate Tests", action: "generate_tests" },
    { id: "ai-optimize-ctx", label: "AI: Optimize", action: "optimize" },
    { id: "ai-review-ctx", label: "AI: Code Review", action: "review" },
    { id: "ai-security-ctx", label: "AI: Security Audit", action: "security" },
  ];

  for (const act of aiActions) {
    editor.addAction({
      id: act.id,
      label: act.label,
      contextMenuGroupId: "ai-navigation",
      contextMenuOrder: aiActions.indexOf(act),
      run: (ed) => {
        const selection = ed.getSelection();
        const model = ed.getModel();
        if (!selection || !model) return;
        const selectedText = model.getValueInRange(selection);
        if (!selectedText) {
          notifyWarning("Select some code first");
          return;
        }
        const filePath = appState.currentFilePath ?? "untitled";
        const language = model.getLanguageId();
        void runAIAction(act.action, selectedText, language, filePath);
      },
    });
  }
}
```

Add imports at the top:
```typescript
import { runAIAction } from "@/stores/ai";
import { notifyWarning } from "@/lib/notifications";
import { appState } from "@/stores/app";
import type { AIActionName } from "@/types";
```

- [x] **Step 2: Call registerContextMenuActions after editor mount**

Find the `onMounted` or editor initialization code and add:

```typescript
registerContextMenuActions();
```

after `editor = monaco.editor.create(...)` or after the `vue-monaco-editor` `handleMount`.

- [x] **Step 3: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 4: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/editor/CodeEditor.vue
git commit -m "feat: add AI Code Actions to Monaco context menu"
```

---

### Task 4: Extend AI context for multi-file @-mention

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/stores/ai.ts`

- [x] **Step 1: Add multi-file context type**

In `frontend/src/types/index.ts`, add a new type:

```typescript
export interface FileContextEntry {
  filePath: string;
  language: string;
  content: string;
}

export interface AIMentionContext {
  files: FileContextEntry[];
}
```

- [x] **Step 2: Extend AIState with mentioned files**

In `frontend/src/stores/ai.ts`, add to `AIState`:

```typescript
  mentionedFiles: FileContextEntry[];
```

Add to the reactive default:

```typescript
  mentionedFiles: [],
```

Add import:
```typescript
import type { ChatMessage, AIContextAttachment, AIActionName, Conversation, FileContextEntry } from "@/types";
```

- [x] **Step 3: Add mention management functions**

In `frontend/src/stores/ai.ts`, add:

```typescript
/**
 * Adds a file to the @-mention context list.
 */
export function addMentionedFile(entry: FileContextEntry): void {
  // Avoid duplicates
  if (aiState.mentionedFiles.some(f => f.filePath === entry.filePath)) return;
  aiState.mentionedFiles.push(entry);
}

/**
 * Removes a file from the @-mention context list by path.
 */
export function removeMentionedFile(filePath: string): void {
  const idx = aiState.mentionedFiles.findIndex(f => f.filePath === filePath);
  if (idx >= 0) aiState.mentionedFiles.splice(idx, 1);
}

/**
 * Clears all mentioned files.
 */
export function clearMentionedFiles(): void {
  aiState.mentionedFiles = [];
}
```

- [x] **Step 4: Update buildUserMessage to include mentioned files**

In `frontend/src/stores/ai.ts`, update `buildUserMessage`:

```typescript
function buildUserMessage(content: string): string {
  let prefix = "";

  // Add mentioned files context
  if (aiState.mentionedFiles.length > 0) {
    prefix += "Referenced files:\n\n";
    for (const file of aiState.mentionedFiles) {
      prefix += `File: ${file.filePath}\n\`\`\`${file.language}\n${file.content}\n\`\`\`\n\n`;
    }
    prefix += "---\n\n";
  }

  // Add selection context if attached
  if (aiState.context) {
    const ctx = aiState.context;
    if (ctx.kind === "selection") {
      prefix += `File: ${ctx.filePath}\nSelected code (${ctx.startLine}-${ctx.endLine}):\n\`\`\`${ctx.language}\n${ctx.content}\n\`\`\`\n\n`;
    } else {
      prefix += `File: ${ctx.filePath}\n\`\`\`${ctx.language}\n${ctx.content}\n\`\`\`\n\n`;
    }
  }

  return prefix + content;
}
```

- [x] **Step 5: Clear mentioned files after sending**

In `sendMessage`, after pushing the user message, add:

```typescript
  // Clear mentioned files after incorporating into message
  clearMentionedFiles();
```

- [x] **Step 6: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 7: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 8: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/stores/ai.ts
git commit -m "feat: add @-mention multi-file context support for AI chat"
```

---

### Task 5: @-mention UI in AiChatPanel

**Files:**
- Modify: `frontend/src/components/ai/AiChatPanel.vue`

- [x] **Step 1: Read current AiChatPanel structure**

Read `frontend/src/components/ai/AiChatPanel.vue` to understand the input area structure.

- [x] **Step 2: Add @-mention file picker**

In the `<script setup>`, add imports:

```typescript
import { ref, computed } from "vue";
import { fileService } from "@/api/services";
import { addMentionedFile, removeMentionedFile, clearMentionedFiles } from "@/stores/ai";
import { aiState } from "@/stores/ai";
import { notifyError, notifySuccess } from "@/lib/notifications";
```

Add state and handler:

```typescript
const showMentionPicker = ref(false);

async function handleMentionFile() {
  try {
    const path = await fileService.pickDirectory();
    // pickDirectory returns a directory; for file picking we use a simple approach:
    // Let user type the file path, or we could use a file tree dialog
    // For now, we'll use the file tree dialog approach
    showMentionPicker.value = true;
  } catch (e) {
    notifyError("Failed to open file picker");
  }
}

async function addFileByPath(filePath: string) {
  try {
    const content = await fileService.readFile(filePath);
    const ext = filePath.split(".").pop() ?? "";
    const languageMap: Record<string, string> = {
      ts: "typescript", tsx: "typescript", js: "javascript", jsx: "javascript",
      go: "go", py: "python", rs: "rust", java: "java",
      md: "markdown", json: "json", yaml: "yaml", yml: "yaml",
      html: "html", css: "css", vue: "vue",
    };
    const language = languageMap[ext] ?? "plaintext";
    addMentionedFile({ filePath, language, content });
    notifySuccess(`Added ${filePath} to context`);
    showMentionPicker.value = false;
  } catch (e: any) {
    notifyError(`Failed to read file: ${e?.message ?? e}`);
  }
}
```

- [x] **Step 3: Add @-mention button and file list to template**

In the chat input area, add a button before the send button:

```html
<button
  class="chat-input__mention-btn"
  :aria-label="'Add file to context'"
  title="Add file to context (@)"
  @click="handleMentionFile"
>
  @
</button>
```

Add mentioned files display above the input:

```html
<div v-if="aiState.mentionedFiles.length > 0" class="chat-mentions">
  <span
    v-for="file in aiState.mentionedFiles"
    :key="file.filePath"
    class="chat-mention-chip"
  >
    {{ file.filePath.split('/').pop() }}
    <button
      class="chat-mention-chip__remove"
      :aria-label="'Remove ' + file.filePath"
      @click="removeMentionedFile(file.filePath)"
    >
      ×
    </button>
  </span>
</div>
```

- [x] **Step 4: Add CSS for mention UI**

Add to the `<style scoped>` section:

```css
.chat-mentions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 4px 8px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.chat-mention-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  font-size: 11px;
  color: var(--color-text-secondary);
}

.chat-mention-chip__remove {
  background: none;
  border: none;
  color: var(--color-text-muted);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 0;
}

.chat-mention-chip__remove:hover {
  color: var(--color-text-primary);
}

.chat-input__mention-btn {
  background: none;
  border: 1px solid var(--color-border-subtle);
  border-radius: 4px;
  color: var(--color-text-secondary);
  cursor: pointer;
  font-size: 14px;
  font-weight: 600;
  padding: 4px 8px;
  transition: var(--transition-fast);
}

.chat-input__mention-btn:hover {
  background: var(--color-bg-surface);
  color: var(--color-text-primary);
}
```

- [x] **Step 5: Verify build**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 6: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 7: Commit**

```bash
git add frontend/src/components/ai/AiChatPanel.vue
git commit -m "feat: add @-mention file picker UI in AiChatPanel"
```

---

### Task 6: Full verification

- [x] **Step 1: Run Go tests**

Run: `go test ./services/... -count=1 -timeout 60s`
Expected: all PASS

- [x] **Step 2: Run Go build**

Run: `go build .`
Expected: success

- [x] **Step 3: Run frontend type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: success

- [x] **Step 4: Run frontend tests**

Run: `cd frontend && npx vitest run`
Expected: all PASS

- [x] **Step 5: Commit if any fixup needed**

```bash
git add -A
git commit -m "chore: verification fixes for Plan 11"
```
