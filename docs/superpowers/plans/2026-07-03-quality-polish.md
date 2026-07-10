# Quality & Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the final remaining quick-win bugs and quality issues from the prompt.md review: CSS token naming (B-5), terminal poll latency (H-2), output buffer grace period (H-1), preset icon mapping mismatches (Q-6), and file permission documentation (Q-8).

**Architecture:** Five independent single-file changes, each verifiable via existing tests or type-checking. No cross-file dependencies between tasks.

**Tech Stack:** Go 1.25, Vue 3 + TypeScript, Wails v3, Element Plus

---

## File Structure

- Modify: `frontend/src/views/WelcomeView.vue` — Task 1 (B-5 CSS tokens)
- Modify: `main.go` — Task 2 (H-2 terminal poll)
- Modify: `services/output_buffer.go` — Task 3 (H-1 grace period)
- Modify: `services/output_buffer_test.go` — Task 3 (add timing test)
- Modify: `services/ai_prompts.go` — Task 4 (Q-6 icon names)
- Modify: `frontend/src/components/editor/CodeEditor.vue` — Task 4 (Q-6 icon map)
- Modify: `services/file_service.go` — Task 5 (Q-8 permission docs)

---

### Task 1: Fix CSS Token Naming in WelcomeView (B-5)

**Files:**
- Modify: `frontend/src/views/WelcomeView.vue:191` and `:244`

The two remaining instances use `--font-family-sans` which is not defined in `main.css`. The correct token is `--font-sans`.

- [x] **Step 1: Replace both instances**

In `frontend/src/views/WelcomeView.vue`, replace all occurrences of `var(--font-family-sans)` with `var(--font-sans)`.

There are exactly 2 instances:
- Line 191: `font-family: var(--font-family-sans);`
- Line 244: `font-family: var(--font-family-sans);`

Both become:
```css
font-family: var(--font-sans);
```

- [x] **Step 2: Verify no remaining old tokens**

Run: `grep -rn "--font-family-sans\|--color-background\|--duration-fast\|--ease-out-expo\|--ease-in-out-quart" frontend/src/`
Expected: No output (all old token names eliminated)

- [x] **Step 3: Type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 2: Reduce Terminal Polling Latency (H-2)

**Files:**
- Modify: `main.go:82`

The terminal output poll loop uses `60 * time.Second` which means worst-case first-byte latency of 60 seconds. The `outputBuffer` has a `notify` channel that wakes the Read early when data arrives, so reducing the timeout is safe — it only affects the fallback case. Change to 5 seconds.

- [x] **Step 1: Change poll timeout**

In `main.go` line 82, change:
```go
output := terminalService.ReadOutput(60 * time.Second)
```
to:
```go
output := terminalService.ReadOutput(5 * time.Second)
```

- [x] **Step 2: Build**

Run: `go build .`
Expected: exit 0

---

### Task 3: Improve Output Buffer Read Grace Period (H-1)

**Files:**
- Modify: `services/output_buffer.go:43`
- Modify: `services/output_buffer_test.go` (add test)

The `Read` method has a 300ms silence period after data arrives (lines 43-52). This means after the first chunk, it waits 300ms for more data before returning. For interactive terminal output this adds noticeable latency. Reduce to 50ms — enough to batch rapid output, but not so long it feels laggy.

- [x] **Step 1: Read current implementation**

Current code in `services/output_buffer.go` lines 43-52:
```go
end := time.Now().Add(300 * time.Millisecond)
if end.After(deadline) {
    end = deadline
}
for time.Now().Before(end) {
    select {
    case <-o.notify:
    case <-time.After(time.Until(end)):
    }
}
```

- [x] **Step 2: Change 300ms to 50ms**

Replace `300 * time.Millisecond` with `50 * time.Millisecond`:
```go
end := time.Now().Add(50 * time.Millisecond)
if end.After(deadline) {
    end = deadline
}
for time.Now().Before(end) {
    select {
    case <-o.notify:
    case <-time.After(time.Until(end)):
    }
}
```

- [x] **Step 3: Run existing tests to verify no regression**

Run: `go test ./services/ -run TestOutputBuffer -v`
Expected: All 3 tests pass (TestOutputBuffer_ReadClearsBuffer, TestOutputBuffer_AppendAndRead, TestOutputBuffer_ReadEmpty)

- [x] **Step 4: Run full services test suite**

Run: `go test ./services/... `
Expected: ok changeme/services

---

### Task 4: Fix PresetMeta Icon Name Mismatches (Q-6)

**Files:**
- Modify: `services/ai_prompts.go:160` and `:166`
- Modify: `frontend/src/components/editor/CodeEditor.vue:30`

The backend `PresetMetas` uses icon strings that don't all match the frontend `presetIconMap` keys. Specifically:
- Backend `"el-icon-refresh"` (refactor) — frontend map has `"el-icon-refresh-left"` → mismatch
- Backend `"el-icon-warning"` (fix) — frontend map has no `"el-icon-warning"` key → missing

Fix: align backend icon names to match the frontend map keys, and add the missing mapping.

- [x] **Step 1: Fix backend icon name for refactor**

In `services/ai_prompts.go` line 160, change:
```go
Icon: "el-icon-refresh",
```
to:
```go
Icon: "el-icon-refresh-left",
```

- [x] **Step 2: Fix backend icon name for fix**

In `services/ai_prompts.go` line 166, change:
```go
Icon: "el-icon-warning",
```
to:
```go
Icon: "el-icon-warning",
```
(This name is fine — we'll add the frontend mapping for it in Step 3.)

- [x] **Step 3: Add missing frontend icon mapping**

In `frontend/src/components/editor/CodeEditor.vue`, add `Warning` to the icon imports and add the mapping.

Update the import (line 6-15) to include `Warning`:
```typescript
import {
  InfoFilled,
  Refresh,
  MagicStick,
  Document,
  CircleCheck,
  Cpu,
  View,
  Lock,
  Edit,
  Warning,
} from "@element-plus/icons-vue";
```

Add to `presetIconMap` (after line 37):
```typescript
  "el-icon-warning": Warning,
```

The full map becomes:
```typescript
const presetIconMap: Record<string, any> = {
  "el-icon-info": InfoFilled,
  "el-icon-refresh-left": Refresh,
  "el-icon-magic-stick": MagicStick,
  "el-icon-document": Document,
  "el-icon-circle-check": CircleCheck,
  "el-icon-cpu": Cpu,
  "el-icon-view": View,
  "el-icon-lock": Lock,
  "el-icon-edit": Edit,
  "el-icon-warning": Warning,
};
```

- [x] **Step 4: Run Go tests**

Run: `go test ./services/ -run TestPreset -v`
Expected: PASS

- [x] **Step 5: Type-check frontend**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 5: Document File Permission Policy (Q-8)

**Files:**
- Modify: `services/file_service.go` (add comment near permission constants)

The file operations use fixed permissions (0644 for files, 0755 for directories). This is acceptable but should be documented as an intentional design decision.

- [x] **Step 1: Add documentation comment**

In `services/file_service.go`, add a comment block after the imports (after line 12, before the `DirEntry` struct):

```go
// File permission policy:
// Files are created with mode 0644 (owner read/write, group/others read-only).
// Directories are created with mode 0755 (owner rwx, group/others rx).
// These fixed modes are used instead of respecting umask to ensure
// consistent behavior across platforms (Windows ignores Unix permission bits,
// macOS/Linux honor them). Users who need different permissions can chmod
// after creation via the terminal.
```

- [x] **Step 2: Build**

Run: `go build .`
Expected: exit 0

---

### Task 6: Full Verification

- [x] **Step 1: Run Go tests**

Run: `go test ./services/...`
Expected: ok changeme/services

- [x] **Step 2: Run Go build**

Run: `go build .`
Expected: exit 0

- [x] **Step 3: Run frontend type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [x] **Step 4: Run frontend tests**

Run: `cd frontend && npx vitest run`
Expected: All tests pass

- [x] **Step 5: Verify no old CSS tokens remain**

Run: `grep -rn "--font-family-sans\|--color-background\|--duration-fast\|--ease-out-expo\|--ease-in-out-quart" frontend/src/`
Expected: No output

- [x] **Step 6: Verify terminal poll is 5s**

Run: `grep "ReadOutput" main.go`
Expected: `output := terminalService.ReadOutput(5 * time.Second)`

- [x] **Step 7: Verify output buffer grace is 50ms**

Run: `grep "Millisecond" services/output_buffer.go`
Expected: `end := time.Now().Add(50 * time.Millisecond)`

- [x] **Step 8: Verify icon name alignment**

Run: `grep "el-icon-refresh" services/ai_prompts.go`
Expected: `Icon: "el-icon-refresh-left",`

Run: `grep "el-icon-warning" frontend/src/components/editor/CodeEditor.vue`
Expected: `"el-icon-warning": Warning,`
