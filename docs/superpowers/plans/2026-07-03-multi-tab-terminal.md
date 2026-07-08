# Multi-Tab Terminal Implementation Plan (#19)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the terminal system to support multiple concurrent terminal sessions with tabbed UI. Users can create new terminals with the + button, switch between them, and close individual terminals. Each tab runs its own PTY session.

**Architecture:** Backend `TerminalService` manages a `map[string]*TerminalSession` where each session has its own PTY connection, output buffer, and readLoop goroutine. Sessions emit `terminal:output` events directly via the Wails app reference (eliminating the main.go poll loop). Frontend `terminalState` manages a map of sessions, each with its own xterm.js instance. `TerminalPanel.vue` renders a tab bar with +/close buttons and shows the active session's terminal.

**Tech Stack:** Go (ConPTY/pty), Wails v3 events, Vue 3 reactivity, xterm.js

---

## File Structure

- Modify: `services/terminal_service.go` — Task 1 (multi-session backend)
- Modify: `services/terminal_service_test.go` — Task 2 (update tests)
- Modify: `main.go` — Task 3 (wire app ref, remove poll loop)
- Modify: `frontend/bindings/gugacode/services/terminalservice.js` — Task 4 (new binding IDs)
- Modify: `frontend/src/api/services.ts` — Task 5 (new terminal API methods)
- Modify: `frontend/src/types/index.ts` — Task 5 (TerminalSession type)
- Modify: `frontend/src/stores/terminal.ts` — Task 6 (multi-session store)
- Modify: `frontend/src/stores/terminal.test.ts` — Task 7 (update tests)
- Modify: `frontend/src/components/layout/TerminalPanel.vue` — Task 8 (tabbed UI)

---

### Task 1: Refactor TerminalService for Multi-Session

**Files:**
- Modify: `services/terminal_service.go` (complete rewrite)

Refactor from single-session to multi-session. Add `TerminalSession` struct, session map, and `SetApp` for direct event emission.

- [ ] **Step 1: Rewrite terminal_service.go**

Overwrite `e:\gugacode\gugacode\gugacode\services\terminal_service.go` with:

```go
package services

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TerminalSession represents a single PTY terminal session.
type TerminalSession struct {
	id         string
	conn       io.ReadWriteCloser
	outputBuf  *outputBuffer
	running    bool
	workingDir string
	shell      string
}

// TerminalService manages multiple terminal sessions.
type TerminalService struct {
	mu       sync.Mutex
	sessions map[string]*TerminalSession
	rootDir  string
	app      *application.App
}

func NewTerminalService() *TerminalService {
	return &TerminalService{
		sessions: make(map[string]*TerminalSession),
	}
}

// SetApp links the Wails app for event emission.
func (t *TerminalService) SetApp(app *application.App) {
	t.mu.Lock()
	t.app = app
	t.mu.Unlock()
}

// SetWorkspaceRoot sets the directory within which terminal sessions are allowed.
func (t *TerminalService) SetWorkspaceRoot(root string) error {
	if root == "" {
		t.mu.Lock()
		t.rootDir = ""
		t.mu.Unlock()
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root is not a directory: %s", abs)
	}
	t.mu.Lock()
	t.rootDir = abs
	t.mu.Unlock()
	return nil
}

func (t *TerminalService) validateWorkingDir(workingDir string) error {
	if workingDir == "" {
		return nil
	}
	t.mu.Lock()
	root := t.rootDir
	t.mu.Unlock()
	if root == "" {
		return nil
	}
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return err
	}
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return fmt.Errorf("working directory %s is outside the workspace", workingDir)
	}
	return nil
}

// StartSession creates and starts a new terminal session with the given ID.
func (t *TerminalService) StartSession(id string, workingDir string, shell string) error {
	if id == "" {
		return fmt.Errorf("session ID cannot be empty")
	}
	if err := t.validateWorkingDir(workingDir); err != nil {
		return err
	}

	t.mu.Lock()
	if _, exists := t.sessions[id]; exists {
		t.mu.Unlock()
		return fmt.Errorf("session %s already exists", id)
	}
	t.mu.Unlock()

	if workingDir != "" {
		info, err := os.Stat(workingDir)
		if err != nil {
			return fmt.Errorf("invalid working directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("working directory is not a directory: %s", workingDir)
		}
	}

	resolvedShell := shell
	if resolvedShell == "" {
		resolvedShell = defaultShell()
	}

	conn, err := startPty(resolvedShell, workingDir)
	if err != nil {
		return err
	}

	session := &TerminalSession{
		id:         id,
		conn:       conn,
		outputBuf:  newOutputBuffer(),
		running:    true,
		workingDir: workingDir,
		shell:      resolvedShell,
	}

	t.mu.Lock()
	t.sessions[id] = session
	app := t.app
	t.mu.Unlock()

	go t.readLoop(session, app)

	return nil
}

// KillSession kills and removes a specific terminal session.
func (t *TerminalService) KillSession(id string) error {
	t.mu.Lock()
	session, exists := t.sessions[id]
	t.mu.Unlock()

	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	session.conn.Close()
	session.running = false

	t.mu.Lock()
	delete(t.sessions, id)
	t.mu.Unlock()

	return nil
}

// WriteSession writes input to a specific terminal session.
func (t *TerminalService) WriteSession(id string, input []byte) error {
	t.mu.Lock()
	session, exists := t.sessions[id]
	t.mu.Unlock()

	if !exists || !session.running {
		return ErrTerminalNotRunning
	}
	_, err := session.conn.Write(input)
	return err
}

// ResizeSession resizes a specific terminal session.
func (t *TerminalService) ResizeSession(id string, cols int, rows int) error {
	t.mu.Lock()
	session, exists := t.sessions[id]
	t.mu.Unlock()

	if !exists || !session.running {
		return ErrTerminalNotRunning
	}
	if r, ok := session.conn.(ptyResizer); ok {
		return r.Resize(cols, rows)
	}
	return nil
}

// IsSessionRunning checks if a specific session is running.
func (t *TerminalService) IsSessionRunning(id string) bool {
	t.mu.Lock()
	session, exists := t.sessions[id]
	t.mu.Unlock()
	return exists && session.running
}

// ListSessions returns the IDs of all active sessions.
func (t *TerminalService) ListSessions() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	ids := make([]string, 0, len(t.sessions))
	for id := range t.sessions {
		ids = append(ids, id)
	}
	return ids
}

// readLoop reads from the session's PTY and emits terminal:output events.
func (t *TerminalService) readLoop(session *TerminalSession, app *application.App) {
	buf := make([]byte, 4096)
	for {
		n, err := session.conn.Read(buf)
		if n > 0 {
			data := string(buf[:n])
			session.outputBuf.Append(buf[:n])
			if app != nil {
				app.Event.Emit("terminal:output", map[string]string{
					"sessionId": session.id,
					"data":      data,
				})
			}
		}
		if err != nil {
			session.running = false
			if app != nil {
				app.Event.Emit("terminal:output", map[string]string{
					"sessionId": session.id,
					"data":      "\r\n\x1b[90m[Process exited]\x1b[0m\r\n",
				})
			}
			return
		}
	}
}

// --- Backward-compatible single-session API (uses "default" session) ---

func (t *TerminalService) Start(workingDir string) error {
	return t.StartSession("default", workingDir, "")
}

func (t *TerminalService) Write(input []byte) error {
	return t.WriteSession("default", input)
}

func (t *TerminalService) Resize(cols, rows int) error {
	return t.ResizeSession("default", cols, rows)
}

func (t *TerminalService) Kill() {
	_ = t.KillSession("default")
}

func (t *TerminalService) IsRunning() bool {
	return t.IsSessionRunning("default")
}

func (t *TerminalService) ReadOutput(timeout time.Duration) string {
	t.mu.Lock()
	session, exists := t.sessions["default"]
	t.mu.Unlock()
	if !exists {
		return ""
	}
	return session.outputBuf.Read(timeout)
}

type ptyResizer interface {
	Resize(cols, rows int) error
}

var ErrTerminalNotRunning = errTerminalNotRunning{}

type errTerminalNotRunning struct{}

func (errTerminalNotRunning) Error() string { return "terminal not running" }
```

- [ ] **Step 2: Build**

Run: `cd e:\gugacode\gugacode\gugacode && go build .`
Expected: exit 0

---

### Task 2: Update Terminal Service Tests

**Files:**
- Modify: `services/terminal_service_test.go`

Update existing tests to use the new multi-session API.

- [ ] **Step 1: Rewrite test file**

Overwrite `e:\gugacode\gugacode\gugacode\services\terminal_service_test.go` with:

```go
package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTerminalService_StartAndRead(t *testing.T) {
	ts := NewTerminalService()
	defer ts.Kill()

	if err := ts.Start(""); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if ts.IsRunning() != true {
		t.Error("expected IsRunning() to be true after Start")
	}

	ts.Write([]byte("echo hello_pty\n"))

	output := ts.ReadOutput(2 * time.Second)
	if !strings.Contains(output, "hello_pty") {
		t.Errorf("expected output to contain 'hello_pty', got %q", output)
	}
}

func TestTerminalService_Kill(t *testing.T) {
	ts := NewTerminalService()
	if err := ts.Start(""); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	ts.Kill()

	if ts.IsRunning() != false {
		t.Error("expected IsRunning() to be false after Kill")
	}
}

func TestTerminalService_MultiSession(t *testing.T) {
	ts := NewTerminalService()
	defer ts.KillSession("s1")
	defer ts.KillSession("s2")

	if err := ts.StartSession("s1", "", ""); err != nil {
		t.Fatalf("StartSession s1 failed: %v", err)
	}
	if err := ts.StartSession("s2", "", ""); err != nil {
		t.Fatalf("StartSession s2 failed: %v", err)
	}

	sessions := ts.ListSessions()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}

	if !ts.IsSessionRunning("s1") || !ts.IsSessionRunning("s2") {
		t.Error("expected both sessions to be running")
	}

	// Kill one session
	if err := ts.KillSession("s1"); err != nil {
		t.Fatalf("KillSession s1 failed: %v", err)
	}

	if ts.IsSessionRunning("s1") {
		t.Error("expected s1 to not be running after kill")
	}
	if !ts.IsSessionRunning("s2") {
		t.Error("expected s2 to still be running")
	}

	sessions = ts.ListSessions()
	if len(sessions) != 1 {
		t.Errorf("expected 1 session after kill, got %d", len(sessions))
	}
}

func TestTerminalService_DuplicateSession(t *testing.T) {
	ts := NewTerminalService()
	defer ts.KillSession("dup")

	if err := ts.StartSession("dup", "", ""); err != nil {
		t.Fatalf("first StartSession failed: %v", err)
	}
	err := ts.StartSession("dup", "", "")
	if err == nil {
		t.Error("expected error starting duplicate session")
	}
}

func TestTerminalService_WorkspaceRoot(t *testing.T) {
	ts := NewTerminalService()
	tmpDir := t.TempDir()

	if err := ts.SetWorkspaceRoot(tmpDir); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}

	// Inside workspace — should succeed
	subDir := filepath.Join(tmpDir, "subdir")
	os.Mkdir(subDir, 0755)
	if err := ts.validateWorkingDir(subDir); err != nil {
		t.Errorf("expected subdir to be valid: %v", err)
	}

	// Outside workspace — should fail
	parent := filepath.Dir(tmpDir)
	if err := ts.validateWorkingDir(parent); err == nil {
		t.Error("expected parent dir to be rejected")
	}
}
```

- [ ] **Step 2: Run tests**

Run: `cd e:\gugacode\gugacode\gugacode && go test ./services/ -run TestTerminalService -v -count=1`
Expected: All tests pass

---

### Task 3: Wire App Reference and Remove Poll Loop

**Files:**
- Modify: `main.go`

Give TerminalService the app reference for direct event emission, and remove the old poll loop.

- [ ] **Step 1: Add SetApp call**

In `main.go`, after `aiService.SetApp(app)` (line 77), add:
```go
	terminalService.SetApp(app)
```

- [ ] **Step 2: Remove the terminal poll loop**

Remove the entire poll loop goroutine (lines 80-87):
```go
	// Wire terminal output to Wails event
	go func() {
		for {
			output := terminalService.ReadOutput(5 * time.Second)
			if output != "" {
				app.Event.Emit("terminal:output", TerminalOutputEvent{Data: output})
			}
		}
	}()
```

- [ ] **Step 3: Remove TerminalOutputEvent struct**

Remove the struct definition (lines 24-26):
```go
type TerminalOutputEvent struct {
	Data string `json:"data"`
}
```

And remove its registration in `init()` (line 18):
```go
	application.RegisterEvent[TerminalOutputEvent]("terminal:output")
```

Replace with:
```go
	application.RegisterEvent[map[string]string]("terminal:output")
```

- [ ] **Step 4: Remove unused time import if needed**

Check if `time` is still used. If not, remove `"time"` from imports.

- [ ] **Step 5: Build**

Run: `cd e:\gugacode\gugacode\gugacode && go build .`
Expected: exit 0

---

### Task 4: Add New Binding IDs

**Files:**
- Modify: `frontend/bindings/gugacode/services/terminalservice.js`

Add 5 new methods with FNV-1a binding IDs computed from `gugacode/services.TerminalService.{MethodName}`.

- [ ] **Step 1: Compute binding IDs**

The FNV-1a 32-bit hash of `gugacode/services.TerminalService.{MethodName}`:
- StartSession → need to compute
- KillSession → need to compute
- WriteSession → need to compute
- ResizeSession → need to compute
- IsSessionRunning → need to compute
- ListSessions → need to compute

Run this Go program to compute the IDs:
```go
package main
import (
	"fmt"
	"hash/fnv"
)
func main() {
	methods := []string{"StartSession", "KillSession", "WriteSession", "ResizeSession", "IsSessionRunning", "ListSessions"}
	for _, m := range methods {
		key := "gugacode/services.TerminalService." + m
		h := fnv.New32a()
		h.Write([]byte(key))
		fmt.Printf("%s: %d\n", m, h.Sum32())
	}
}
```

- [ ] **Step 2: Add new methods to terminalservice.js**

Add these export functions to `frontend/bindings/gugacode/services/terminalservice.js` (before the existing functions or after — order doesn't matter):

```javascript
/**
 * Starts a new terminal session with the given ID, working directory, and shell.
 * @param {string} id
 * @param {string} workingDir
 * @param {string} shell
 * @returns {$CancellablePromise<void>}
 */
export function StartSession(id, workingDir, shell) {
    return $Call.ByID(NEW_ID_1, id, workingDir, shell);
}

/**
 * Kills a specific terminal session by ID.
 * @param {string} id
 * @returns {$CancellablePromise<void>}
 */
export function KillSession(id) {
    return $Call.ByID(NEW_ID_2, id);
}

/**
 * Writes input to a specific terminal session.
 * @param {string} id
 * @param {Uint8Array | string} input
 * @returns {$CancellablePromise<void>}
 */
export function WriteSession(id, input) {
    return $Call.ByID(NEW_ID_3, id, input);
}

/**
 * Resizes a specific terminal session.
 * @param {string} id
 * @param {number} cols
 * @param {number} rows
 * @returns {$CancellablePromise<void>}
 */
export function ResizeSession(id, cols, rows) {
    return $Call.ByID(NEW_ID_4, id, cols, rows);
}

/**
 * Checks if a specific session is running.
 * @param {string} id
 * @returns {$CancellablePromise<boolean>}
 */
export function IsSessionRunning(id) {
    return $Call.ByID(NEW_ID_5, id);
}

/**
 * Returns the IDs of all active terminal sessions.
 * @returns {$CancellablePromise<string[]>}
 */
export function ListSessions() {
    return $Call.ByID(NEW_ID_6);
}
```

Replace `NEW_ID_1` through `NEW_ID_6` with the computed values from Step 1.

---

### Task 5: Update Frontend API and Types

**Files:**
- Modify: `frontend/src/api/services.ts`
- Modify: `frontend/src/types/index.ts`

- [ ] **Step 1: Add terminal session methods to services.ts**

In `frontend/src/api/services.ts`, find the `terminalService` export and add these methods:

```typescript
export const terminalService = {
  start: (workingDir: string) => TerminalServiceBindings.Start(workingDir),
  write: (input: string) => TerminalServiceBindings.Write(input),
  kill: () => TerminalServiceBindings.Kill(),
  resize: (cols: number, rows: number) => TerminalServiceBindings.Resize(cols, rows),
  isRunning: () => TerminalServiceBindings.IsRunning(),
  startSession: (id: string, workingDir: string, shell: string) =>
    TerminalServiceBindings.StartSession(id, workingDir, shell),
  killSession: (id: string) => TerminalServiceBindings.KillSession(id),
  writeSession: (id: string, input: string) =>
    TerminalServiceBindings.WriteSession(id, input),
  resizeSession: (id: string, cols: number, rows: number) =>
    TerminalServiceBindings.ResizeSession(id, cols, rows),
  isSessionRunning: (id: string) => TerminalServiceBindings.IsSessionRunning(id),
  listSessions: () => TerminalServiceBindings.ListSessions(),
};
```

- [ ] **Step 2: Add TerminalSessionInfo type**

In `frontend/src/types/index.ts`, add:

```typescript
export interface TerminalSessionInfo {
  id: string;
  title: string;
  active: boolean;
}
```

---

### Task 6: Rewrite Terminal Store for Multi-Session

**Files:**
- Modify: `frontend/src/stores/terminal.ts` (complete rewrite)

- [ ] **Step 1: Rewrite terminal.ts**

Overwrite `e:\gugacode\gugacode\gugacode\frontend\src\stores\terminal.ts` with:

```typescript
import { reactive } from "vue";
import { terminalService } from "@/api/services";
import { Events } from "@wailsio/runtime";

export interface TerminalSessionState {
  id: string;
  output: string;
  running: boolean;
  cols: number;
  rows: number;
}

export interface TerminalStoreState {
  sessions: Record<string, TerminalSessionState>;
  sessionOrder: string[];
  activeSessionId: string | null;
}

export const terminalState = reactive<TerminalStoreState>({
  sessions: {},
  sessionOrder: [],
  activeSessionId: null,
});

let eventListenerRegistered = false;

function ensureEventListener() {
  if (eventListenerRegistered) return;
  eventListenerRegistered = true;
  Events.On("terminal:output", (event: any) => {
    const sessionId = event?.data?.sessionId ?? event?.sessionId ?? "";
    const data = event?.data?.data ?? event?.data ?? "";
    if (sessionId && typeof data === "string") {
      const session = terminalState.sessions[sessionId];
      if (session) {
        session.output += data;
      }
    }
  });
}

function generateSessionId(): string {
  return "term-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 6);
}

export async function createSession(workingDir: string, shell: string = ""): Promise<string> {
  ensureEventListener();
  const id = generateSessionId();
  try {
    await terminalService.startSession(id, workingDir, shell);
    terminalState.sessions[id] = {
      id,
      output: "",
      running: true,
      cols: 80,
      rows: 24,
    };
    terminalState.sessionOrder.push(id);
    terminalState.activeSessionId = id;
    return id;
  } catch (e) {
    console.error("Failed to create terminal session:", e);
    return "";
  }
}

export async function writeToSession(sessionId: string, input: string): Promise<void> {
  const session = terminalState.sessions[sessionId];
  if (!session || !session.running) return;
  try {
    await terminalService.writeSession(sessionId, input);
  } catch (e) {
    console.error("Failed to write to terminal:", e);
  }
}

export async function killSession(sessionId: string): Promise<void> {
  try {
    await terminalService.killSession(sessionId);
  } catch (e) {
    console.error("Failed to kill terminal:", e);
  }
  delete terminalState.sessions[sessionId];
  terminalState.sessionOrder = terminalState.sessionOrder.filter((id) => id !== sessionId);
  if (terminalState.activeSessionId === sessionId) {
    terminalState.activeSessionId = terminalState.sessionOrder[0] ?? null;
  }
}

export async function resizeSession(sessionId: string, cols: number, rows: number): Promise<void> {
  const session = terminalState.sessions[sessionId];
  if (!session) return;
  session.cols = cols;
  session.rows = rows;
  if (!session.running) return;
  try {
    await terminalService.resizeSession(sessionId, cols, rows);
  } catch (e) {
    console.error("Failed to resize terminal:", e);
  }
}

export function setActiveSession(sessionId: string): void {
  terminalState.activeSessionId = sessionId;
}

export function getActiveSession(): TerminalSessionState | null {
  if (!terminalState.activeSessionId) return null;
  return terminalState.sessions[terminalState.activeSessionId] ?? null;
}

export function clearSessionOutput(sessionId: string): void {
  const session = terminalState.sessions[sessionId];
  if (session) {
    session.output = "";
  }
}

// --- Backward-compatible single-session API ---

export async function startTerminal(workingDir: string): Promise<void> {
  await createSession(workingDir);
}

export async function writeToTerminal(input: string): Promise<void> {
  if (terminalState.activeSessionId) {
    await writeToSession(terminalState.activeSessionId, input);
  }
}

export async function stopTerminal(): Promise<void> {
  if (terminalState.activeSessionId) {
    await killSession(terminalState.activeSessionId);
  }
}

export async function resizeTerminal(cols: number, rows: number): Promise<void> {
  if (terminalState.activeSessionId) {
    await resizeSession(terminalState.activeSessionId, cols, rows);
  }
}

export function appendOutput(data: string): void {
  if (terminalState.activeSessionId) {
    const session = terminalState.sessions[terminalState.activeSessionId];
    if (session) {
      session.output += data;
    }
  }
}

export function clearOutput(): void {
  if (terminalState.activeSessionId) {
    clearSessionOutput(terminalState.activeSessionId);
  }
}
```

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 7: Update Terminal Tests

**Files:**
- Modify: `frontend/src/stores/terminal.test.ts`

- [ ] **Step 1: Rewrite test file**

Overwrite `e:\gugacode\gugacode\gugacode\frontend\src\stores\terminal.test.ts` with:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: vi.fn(),
  },
}));

vi.mock("@/api/services", () => ({
  terminalService: {
    start: vi.fn().mockResolvedValue(undefined),
    write: vi.fn().mockResolvedValue(undefined),
    kill: vi.fn().mockResolvedValue(undefined),
    resize: vi.fn().mockResolvedValue(undefined),
    isRunning: vi.fn().mockReturnValue(false),
    startSession: vi.fn().mockResolvedValue(undefined),
    killSession: vi.fn().mockResolvedValue(undefined),
    writeSession: vi.fn().mockResolvedValue(undefined),
    resizeSession: vi.fn().mockResolvedValue(undefined),
    isSessionRunning: vi.fn().mockReturnValue(false),
    listSessions: vi.fn().mockReturnValue([]),
  },
}));

import {
  terminalState,
  createSession,
  writeToSession,
  killSession,
  resizeSession,
  setActiveSession,
  clearSessionOutput,
  startTerminal,
  stopTerminal,
  appendOutput,
} from "./terminal";

describe("terminal store (multi-session)", () => {
  beforeEach(() => {
    // Reset state
    Object.keys(terminalState.sessions).forEach((id) => delete terminalState.sessions[id]);
    terminalState.sessionOrder = [];
    terminalState.activeSessionId = null;
  });

  it("creates a session", async () => {
    const id = await createSession("/some/path");
    expect(id).toBeTruthy();
    expect(terminalState.sessions[id]).toBeDefined();
    expect(terminalState.sessions[id].running).toBe(true);
    expect(terminalState.activeSessionId).toBe(id);
  });

  it("writes to a session", async () => {
    const id = await createSession("/path");
    await writeToSession(id, "ls\n");
    // No throw = pass
  });

  it("kills a session", async () => {
    const id = await createSession("/path");
    await killSession(id);
    expect(terminalState.sessions[id]).toBeUndefined();
    expect(terminalState.activeSessionId).toBeNull();
  });

  it("switches active session", async () => {
    const id1 = await createSession("/path1");
    const id2 = await createSession("/path2");
    expect(terminalState.activeSessionId).toBe(id2);
    setActiveSession(id1);
    expect(terminalState.activeSessionId).toBe(id1);
  });

  it("resizes a session", async () => {
    const id = await createSession("/path");
    await resizeSession(id, 120, 40);
    expect(terminalState.sessions[id].cols).toBe(120);
    expect(terminalState.sessions[id].rows).toBe(40);
  });

  it("clears session output", async () => {
    const id = await createSession("/path");
    terminalState.sessions[id].output = "hello";
    clearSessionOutput(id);
    expect(terminalState.sessions[id].output).toBe("");
  });

  it("maintains session order", async () => {
    const id1 = await createSession("/path1");
    const id2 = await createSession("/path2");
    const id3 = await createSession("/path3");
    expect(terminalState.sessionOrder).toEqual([id1, id2, id3]);
  });

  it("backward-compat: startTerminal creates a session", async () => {
    await startTerminal("/path");
    expect(terminalState.activeSessionId).toBeTruthy();
  });

  it("backward-compat: appendOutput adds to active session", async () => {
    await startTerminal("/path");
    appendOutput("hello");
    const activeId = terminalState.activeSessionId!;
    expect(terminalState.sessions[activeId].output).toBe("hello");
  });

  it("backward-compat: stopTerminal kills active session", async () => {
    await startTerminal("/path");
    const id = terminalState.activeSessionId!;
    await stopTerminal();
    expect(terminalState.sessions[id]).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run tests**

Run: `cd frontend && npx vitest run src/stores/terminal.test.ts`
Expected: All tests pass

---

### Task 8: Update TerminalPanel for Multi-Tab UI

**Files:**
- Modify: `frontend/src/components/layout/TerminalPanel.vue` (complete rewrite)

The panel needs:
- A tab bar showing all sessions with + and close buttons
- Multiple xterm.js instances (one per session, lazily initialized)
- Switching between tabs shows the correct terminal

- [ ] **Step 1: Rewrite TerminalPanel.vue**

Overwrite `e:\gugacode\gugacode\gugacode\frontend\src\components\layout\TerminalPanel.vue` with:

```vue
<script setup lang="ts">
import { appState, toggleTerminal } from "@/stores/app";
import { computed, onMounted, onBeforeUnmount, ref, watch, nextTick } from "vue";
import { Close, Plus } from "@element-plus/icons-vue";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import {
  terminalState,
  createSession,
  writeToSession,
  killSession,
  resizeSession,
  clearSessionOutput,
} from "@/stores/terminal";
import "@xterm/xterm/css/xterm.css";

const isVisible = computed(() => appState.terminalVisible);
const terminalContainer = ref<HTMLElement | null>(null);

// Map of session ID -> { term, fitAddon }
const terminals: Record<string, { term: Terminal; fitAddon: FitAddon }> = {};
let currentSessionId: string | null = null;

const sessionList = computed(() =>
  terminalState.sessionOrder.map((id) => ({
    id,
    label: id === "default" ? "Terminal" : `Terminal ${terminalState.sessionOrder.indexOf(id) + 1}`,
  }))
);

watch(
  () => terminalState.activeSessionId,
  (newId) => {
    if (newId) {
      nextTick(() => switchToSession(newId));
    }
  }
);

// Watch each session's output
watch(
  () => terminalState.sessions,
  (sessions) => {
    for (const [id, session] of Object.entries(sessions)) {
      const entry = terminals[id];
      if (entry && session.output) {
        entry.term.write(session.output);
        clearSessionOutput(id);
      }
    }
  },
  { deep: true }
);

async function initTerminalForSession(sessionId: string) {
  if (terminals[sessionId]) return;

  if (!terminalContainer.value) return;

  const term = new Terminal({
    fontFamily: "JetBrains Mono, Consolas, 'Courier New', monospace",
    fontSize: 12,
    theme: {
      background: "#131316",
      foreground: "#e8e6e3",
      cursor: "#e8e6e3",
    },
    cursorBlink: true,
  });

  const fitAddon = new FitAddon();
  term.loadAddon(fitAddon);

  // Create a div for this terminal
  const div = document.createElement("div");
  div.style.display = "none";
  div.style.height = "100%";
  terminalContainer.value.appendChild(div);
  term.open(div);
  fitAddon.fit();

  term.onData((data) => {
    writeToSession(sessionId, data);
  });

  term.onResize(({ cols, rows }) => {
    resizeSession(sessionId, cols, rows);
  });

  terminals[sessionId] = { term, fitAddon };
}

function switchToSession(sessionId: string) {
  if (currentSessionId === sessionId) return;

  // Hide current
  if (currentSessionId && terminals[currentSessionId]) {
    const container = terminals[currentSessionId].term.element?.parentElement;
    if (container) container.style.display = "none";
  }

  // Show new
  if (terminals[sessionId]) {
    const container = terminals[sessionId].term.element?.parentElement;
    if (container) container.style.display = "block";
    terminals[sessionId].fitAddon.fit();
  }

  currentSessionId = sessionId;
}

async function handleNewTerminal() {
  const workingDir = appState.currentProject ?? "";
  await createSession(workingDir, "");
}

async function handleCloseTerminal(sessionId: string) {
  // Dispose xterm instance
  if (terminals[sessionId]) {
    const container = terminals[sessionId].term.element?.parentElement;
    if (container) container.remove();
    terminals[sessionId].term.dispose();
    delete terminals[sessionId];
  }

  await killSession(sessionId);

  // Switch to another session if any
  if (terminalState.activeSessionId) {
    nextTick(() => switchToSession(terminalState.activeSessionId!));
  }
}

function handleSelectTab(sessionId: string) {
  terminalState.activeSessionId = sessionId;
}

function fitTerminal() {
  if (currentSessionId && terminals[currentSessionId]) {
    terminals[currentSessionId].fitAddon.fit();
  }
}

onMounted(async () => {
  if (isVisible.value) {
    await initFirstSession();
  }
});

async function initFirstSession() {
  if (terminalState.sessionOrder.length === 0) {
    const workingDir = appState.currentProject ?? "";
    await createSession(workingDir, "");
  }
  if (terminalState.activeSessionId) {
    await initTerminalForSession(terminalState.activeSessionId);
    switchToSession(terminalState.activeSessionId);
  }
}

watch(isVisible, async (visible) => {
  if (visible) {
    if (terminalState.sessionOrder.length === 0) {
      await initFirstSession();
    } else if (terminalState.activeSessionId) {
      await initTerminalForSession(terminalState.activeSessionId);
      switchToSession(terminalState.activeSessionId);
    }
    setTimeout(fitTerminal, 50);
  }
});

onBeforeUnmount(() => {
  for (const id of Object.keys(terminals)) {
    terminals[id].term.dispose();
  }
});
</script>

<template>
  <transition name="slide-terminal">
    <div
      v-if="isVisible"
      class="terminal-panel"
      role="region"
      aria-label="Terminal panel"
    >
      <div class="terminal-panel__header">
        <div class="terminal-panel__tabs" role="tablist" aria-label="Terminal tabs">
          <button
            v-for="s in sessionList"
            :key="s.id"
            class="terminal-panel__tab"
            :class="{ 'terminal-panel__tab--active': terminalState.activeSessionId === s.id }"
            role="tab"
            :aria-selected="terminalState.activeSessionId === s.id"
            :aria-label="s.label + ' tab'"
            @click="handleSelectTab(s.id)"
          >
            {{ s.label }}
            <span
              class="terminal-panel__tab-close"
              role="button"
              aria-label="Close terminal"
              @click.stop="handleCloseTerminal(s.id)"
            >
              <el-icon :size="11"><Close /></el-icon>
            </span>
          </button>
          <button
            class="terminal-panel__new"
            aria-label="New terminal"
            title="New terminal"
            @click="handleNewTerminal"
          >
            <el-icon :size="14"><Plus /></el-icon>
          </button>
        </div>
        <button
          class="terminal-panel__close"
          aria-label="Close panel"
          title="Close panel"
          @click="toggleTerminal"
        >
          <el-icon :size="14"><Close /></el-icon>
        </button>
      </div>

      <div ref="terminalContainer" class="terminal-panel__body" />
    </div>
  </transition>
</template>

<style scoped>
.terminal-panel {
  display: flex;
  flex-direction: column;
  height: 220px;
  min-height: 0;
  background-color: var(--color-terminal-bg);
  overflow: hidden;
  border-top: 1px solid var(--color-border-subtle);
}

.terminal-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 34px;
  min-height: 34px;
  padding: 0 4px 0 8px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.terminal-panel__tabs {
  display: flex;
  align-items: center;
  gap: 0;
  overflow-x: auto;
}

.terminal-panel__tab {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  font-size: 11px;
  font-family: var(--font-sans);
  color: var(--color-text-tertiary);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  white-space: nowrap;
  transition: color var(--transition-fast), background-color var(--transition-fast);
}

.terminal-panel__tab:hover {
  color: var(--color-text-secondary);
  background-color: var(--color-bg-surface-container-low);
}

.terminal-panel__tab--active {
  color: var(--color-text-primary);
  background-color: var(--color-bg-surface-container-low);
}

.terminal-panel__tab-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: var(--radius-xs);
  opacity: 0.6;
  transition: opacity var(--transition-fast), background-color var(--transition-fast);
}

.terminal-panel__tab-close:hover {
  opacity: 1;
  background-color: var(--color-bg-surface-container-high);
}

.terminal-panel__new {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  flex-shrink: 0;
  transition: color var(--transition-fast), background-color var(--transition-fast);
}

.terminal-panel__new:hover {
  color: var(--color-text-primary);
  background-color: var(--color-bg-surface-container-low);
}

.terminal-panel__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  flex-shrink: 0;
  transition: color var(--transition-fast), background-color var(--transition-fast);
}

.terminal-panel__close:hover {
  color: var(--color-text-secondary);
  background-color: color-mix(in srgb, var(--color-text-tertiary) 6%, transparent);
}

.terminal-panel__body {
  flex: 1;
  padding: 4px 8px;
  overflow: hidden;
  position: relative;
}

.terminal-panel__body :deep(.xterm) {
  height: 100%;
}

.terminal-panel__body :deep(.xterm-viewport) {
  overflow-y: auto;
}

.slide-terminal-enter-active,
.slide-terminal-leave-active {
  transition: height var(--transition-normal), opacity var(--transition-fast);
  overflow: hidden;
}

.slide-terminal-enter-from,
.slide-terminal-leave-to {
  height: 0;
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .terminal-panel__tab,
  .terminal-panel__close,
  .terminal-panel__new {
    transition: none;
  }
  .slide-terminal-enter-active,
  .slide-terminal-leave-active {
    transition: none;
  }
}
</style>
```

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

### Task 9: Full Verification

- [ ] **Step 1: Go tests**

Run: `cd e:\gugacode\gugacode\gugacode && go test ./services/... -count=1`
Expected: ok gugacode/services

- [ ] **Step 2: Go build**

Run: `cd e:\gugacode\gugacode\gugacode && go build .`
Expected: exit 0

- [ ] **Step 3: Frontend type-check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Frontend tests**

Run: `cd frontend && npx vitest run`
Expected: All tests pass (existing 87 + updated terminal tests)

- [ ] **Step 5: Verify multi-session methods exist in bindings**

Run: `grep "StartSession\|KillSession\|WriteSession\|ResizeSession\|IsSessionRunning\|ListSessions" frontend/bindings/gugacode/services/terminalservice.js`
Expected: 6 matches

- [ ] **Step 6: Verify poll loop removed from main.go**

Run: `grep "ReadOutput" main.go`
Expected: No matches (poll loop removed)
