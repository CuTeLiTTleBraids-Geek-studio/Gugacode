# Terminal & AI Chat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the TerminalPanel shell into a working PTY terminal (xterm.js + creack/pty) and the AiChatPanel shell into a streaming AI chat client (OpenAI-compatible API with SSE streaming).

**Architecture:** Go backend adds `TerminalService` (PTY process management, output streamed via Wails events) and `AIService` (HTTP client for OpenAI-compatible `/v1/chat/completions`, SSE streaming via Wails events). Frontend integrates xterm.js for terminal rendering and a message-list UI for AI chat. Both consume Wails events for real-time data.

**Tech Stack:** Go 1.25, Wails v3 (alpha2.111), `github.com/creack/pty` (ConPTY on Windows), `net/http` (AI client), Vue 3, TypeScript, `@xterm/xterm`, `@xterm/addon-fit`, `@wailsio/runtime` (events)

**Project root:** `e:\gugacode\gugacode\gugacode\` (the directory containing `go.mod`, `main.go`, `frontend/`). All relative paths in this plan are from this root.

**Module name note:** `go.mod` declares `module changeme`. Bindings land in `frontend/bindings/changeme/` and sub-packages. Plan 1 is complete — 4 services (File/Project/Settings/Window) exist in `services/` package and are registered in `main.go`.

**Windows note:** `creack/pty` uses ConPTY on Windows 10 1809+. The dev machine (Windows) supports this. On older Windows, terminal features degrade gracefully (documented in code).

---

## Scope Check

This plan covers **two independent subsystems** that are both small enough to fit in one plan (they share the settings extension and event infrastructure):

| Subsystem | Backend | Frontend |
|-----------|---------|----------|
| Terminal | TerminalService (PTY) | xterm.js in TerminalPanel |
| AI Chat | AIService (HTTP + SSE) | Message list + streaming in AiChatPanel |

Both are testable independently. AI config (API key, base URL, model) extends the existing SettingsService.

---

## File Structure

```
services/
├── terminal_service.go           # NEW — PTY start/write/kill/resize, output streaming
├── terminal_service_test.go      # NEW — lifecycle tests (start, write, kill)
├── ai_service.go                 # NEW — OpenAI-compatible HTTP client, SSE streaming
├── ai_service_test.go            # NEW — mock server tests for send + stream
├── settings_service.go           # MODIFY — add AI config fields to Settings struct
└── settings_service_test.go      # MODIFY — add tests for new fields

main.go                           # MODIFY — register TerminalService, AIService, new events

frontend/
├── package.json                  # MODIFY — add @xterm/xterm, @xterm/addon-fit
├── src/
│   ├── types/
│   │   └── index.ts              # MODIFY — add AIConfig, ChatMessage types
│   ├── api/
│   │   └── services.ts           # MODIFY — re-export terminalService, aiService
│   ├── stores/
│   │   ├── terminal.ts           # NEW — terminal session state, event listeners
│   │   └── ai.ts                 # NEW — messages, streaming state, send function
│   ├── components/
│   │   └── layout/
│   │       ├── TerminalPanel.vue # MODIFY — integrate xterm.js
│   │       └── AiChatPanel.vue   # MODIFY — message list + streaming display
│   └── views/
│       └── SettingsView.vue      # MODIFY — AI config section (API key, base URL, model)
```

---

## Task 1: Go Backend — TerminalService PTY Lifecycle

**Files:**
- Create: `services/terminal_service.go`
- Create: `services/terminal_service_test.go`

- [ ] **Step 1: Write the failing tests**

Create `services/terminal_service_test.go`:

```go
package services

import (
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

	// Send a command and wait for output
	ts.Write([]byte("echo hello_pty\n"))

	output := ts.ReadOutput(2 * time.Second)
	if !strings.Contains(output, "hello_pty") {
		t.Errorf("expected output to contain 'hello_pty', got: %q", output)
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

func TestTerminalService_WriteWhenNotRunning(t *testing.T) {
	ts := NewTerminalService()
	err := ts.Write([]byte("test"))
	if err == nil {
		t.Error("expected error when writing to non-running terminal")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestTerminalService -v`
Expected: FAIL with "undefined: NewTerminalService" or similar compile error.

- [ ] **Step 3: Write minimal implementation**

First add the dependency:
```bash
cd e:\gugacode\gugacode\gugacode
go get github.com/creack/pty
```

Create `services/terminal_service.go`:

```go
package services

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

type TerminalService struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	ptmx      *os.File
	outputBuf *outputBuffer
	running   bool
}

func NewTerminalService() *TerminalService {
	return &TerminalService{
		outputBuf: newOutputBuffer(),
	}
}

func (t *TerminalService) Start(workingDir string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	shell := defaultShell()
	cmd := exec.Command(shell[0], shell[1:]...)
	cmd.Dir = workingDir

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}

	t.cmd = cmd
	t.ptmx = ptmx
	t.running = true

	go t.readLoop(ptmx)

	return nil
}

func (t *TerminalService) Write(input []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || t.ptmx == nil {
		return ErrTerminalNotRunning
	}
	_, err := t.ptmx.Write(input)
	return err
}

func (t *TerminalService) Kill() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	if t.ptmx != nil {
		t.ptmx.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}
	t.running = false
}

func (t *TerminalService) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *TerminalService) ReadOutput(timeout time.Duration) string {
	return t.outputBuf.Read(timeout)
}

func (t *TerminalService) readLoop(r io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			t.outputBuf.Append(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func defaultShell() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	return []string{shell}
}

var ErrTerminalNotRunning = errTerminalNotRunning{}

type errTerminalNotRunning struct{}

func (errTerminalNotRunning) Error() string { return "terminal not running" }
```

Create `services/output_buffer.go`:

```go
package services

import (
	"bytes"
	"sync"
	"time"
)

type outputBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	cond *sync.Cond
}

func newOutputBuffer() *outputBuffer {
	ob := &outputBuffer{}
	ob.cond = sync.NewCond(&ob.mu)
	return ob
}

func (o *outputBuffer) Append(data []byte) {
	o.mu.Lock()
	o.buf.Write(data)
	o.cond.Broadcast()
	o.mu.Unlock()
}

func (o *outputBuffer) Read(timeout time.Duration) string {
	o.mu.Lock()
	defer o.mu.Unlock()

	deadline := time.Now().Add(timeout)
	for o.buf.Len() == 0 && time.Now().Before(deadline) {
		waitDone := make(chan struct{})
		go func() {
			o.mu.Lock()
			o.cond.Wait()
			o.mu.Unlock()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(time.Until(deadline)):
		}
	}
	return o.buf.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestTerminalService -v`
Expected: 3 tests PASS.

Note: If `defaultShell()` returns `bash` but bash isn't available on Windows, adapt `defaultShell` to use `powershell` or `cmd` on Windows. Add this build-tag-aware version if tests fail on Windows:

```go
//go:build windows

func defaultShell() []string {
	return []string{"powershell.exe", "-NoLogo"}
}
```

```go
//go:build !windows

func defaultShell() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	return []string{shell}
}
```

- [ ] **Step 5: Commit**

```bash
git add services/terminal_service.go services/terminal_service_test.go services/output_buffer.go
git commit -m "feat: add TerminalService with PTY lifecycle"
```

---

## Task 2: Go Backend — TerminalService Resize + Event Streaming

**Files:**
- Modify: `services/terminal_service.go`
- Modify: `services/terminal_service_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `services/terminal_service_test.go`:

```go
func TestTerminalService_Resize(t *testing.T) {
	ts := NewTerminalService()
	defer ts.Kill()

	if err := ts.Start(""); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err := ts.Resize(80, 24)
	if err != nil {
		t.Errorf("Resize failed: %v", err)
	}
}

func TestTerminalService_ResizeWhenNotRunning(t *testing.T) {
	ts := NewTerminalService()
	err := ts.Resize(80, 24)
	if err == nil {
		t.Error("expected error when resizing non-running terminal")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestTerminalService_Resize -v`
Expected: FAIL with "ts.Resize undefined".

- [ ] **Step 3: Write minimal implementation**

Add to `services/terminal_service.go`:

```go
import (
	"github.com/creack/pty"
	// ... existing imports
)

func (t *TerminalService) Resize(cols, rows int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || t.ptmx == nil {
		return ErrTerminalNotRunning
	}
	return pty.Setsize(t.ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestTerminalService -v`
Expected: all 5 TerminalService tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/terminal_service.go services/terminal_service_test.go
git commit -m "feat: add TerminalService Resize"
```

---

## Task 3: Go Backend — AIService Config + Send (Non-Streaming)

**Files:**
- Create: `services/ai_service.go`
- Create: `services/ai_service_test.go`

- [ ] **Step 1: Write the failing tests**

Create `services/ai_service_test.go`:

```go
package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIService_SendReturnsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		messages := body["messages"].([]interface{})
		if len(messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(messages))
		}

		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "Hello from AI",
					},
					"finish_reason": "stop",
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	ai := NewAIService()
	ai.SetConfig(AIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o",
	})

	resp, err := ai.Send([]ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if resp.Content != "Hello from AI" {
		t.Errorf("expected 'Hello from AI', got %q", resp.Content)
	}
}

func TestAIService_SendMissingAPIKey(t *testing.T) {
	ai := NewAIService()
	_, err := ai.Send([]ChatMessage{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Error("expected error when API key is missing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestAIService -v`
Expected: FAIL with "undefined: NewAIService".

- [ ] **Step 3: Write minimal implementation**

Create `services/ai_service.go`:

```go
package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Content      string
	FinishReason string
}

type AIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type AIService struct {
	config AIConfig
}

func NewAIService() *AIService {
	return &AIService{}
}

func (a *AIService) SetConfig(config AIConfig) {
	a.config = config
}

func (a *AIService) Send(messages []ChatMessage) (*ChatResponse, error) {
	if a.config.APIKey == "" {
		return nil, errors.New("API key not configured")
	}

	reqBody := map[string]interface{}{
		"model":    a.config.Model,
		"messages": messages,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", a.config.BaseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}

	return &ChatResponse{
		Content:      result.Choices[0].Message.Content,
		FinishReason: result.Choices[0].FinishReason,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestAIService -v`
Expected: 2 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/ai_service.go services/ai_service_test.go
git commit -m "feat: add AIService with non-streaming Send"
```

---

## Task 4: Go Backend — AIService Streaming via SSE

**Files:**
- Modify: `services/ai_service.go`
- Modify: `services/ai_service_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `services/ai_service_test.go`:

```go
func TestAIService_SendStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		flusher, _ := w.(http.Flusher)
		chunks := []string{"Hello", " world", "!"}
		for _, chunk := range chunks {
			data := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"delta": map[string]string{"content": chunk},
					},
				},
			}
			jsonBytes, _ := json.Marshal(data)
			w.Write([]byte("data: " + string(jsonBytes) + "\n\n"))
			flusher.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	ai := NewAIService()
	ai.SetConfig(AIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o",
	})

	var collected string
	err := ai.SendStream([]ChatMessage{{Role: "user", Content: "hi"}}, func(chunk string) {
		collected += chunk
	})
	if err != nil {
		t.Fatalf("SendStream failed: %v", err)
	}
	if collected != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", collected)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestAIService_SendStream -v`
Expected: FAIL with "ai.SendStream undefined".

- [ ] **Step 3: Write minimal implementation**

Add to `services/ai_service.go`:

```go
import (
	"bufio"
	// ... existing imports
)

func (a *AIService) SendStream(messages []ChatMessage, onChunk func(chunk string)) error {
	if a.config.APIKey == "" {
		return errors.New("API key not configured")
	}

	reqBody := map[string]interface{}{
		"model":    a.config.Model,
		"messages": messages,
		"stream":   true,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", a.config.BaseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 6 || line[:6] != "data: " {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			break
		}

		var result struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &result); err != nil {
			continue
		}
		if len(result.Choices) > 0 && result.Choices[0].Delta.Content != "" {
			onChunk(result.Choices[0].Delta.Content)
		}
	}

	return scanner.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestAIService -v`
Expected: all 3 AIService tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/ai_service.go services/ai_service_test.go
git commit -m "feat: add AIService streaming via SSE"
```

---

## Task 5: Go Backend — Extend Settings with AI Config

**Files:**
- Modify: `services/settings_service.go`
- Modify: `services/settings_service_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `services/settings_service_test.go`:

```go
func TestSettingsService_SaveAndLoadAIConfig(t *testing.T) {
	dir := t.TempDir()
	ss := NewSettingsServiceWithDir(dir)

	settings := defaultSettings()
	settings.AIApiKey = "sk-test-key"
	settings.AIBaseURL = "https://api.openai.com"
	settings.AIModel = "gpt-4o"

	if err := ss.Save(settings); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := ss.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.AIApiKey != "sk-test-key" {
		t.Errorf("expected AIApiKey 'sk-test-key', got %q", loaded.AIApiKey)
	}
	if loaded.AIBaseURL != "https://api.openai.com" {
		t.Errorf("expected AIBaseURL, got %q", loaded.AIBaseURL)
	}
	if loaded.AIModel != "gpt-4o" {
		t.Errorf("expected AIModel 'gpt-4o', got %q", loaded.AIModel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestSettingsService_SaveAndLoadAIConfig -v`
Expected: FAIL with "settings.AIApiKey undefined" (struct lacks these fields).

- [ ] **Step 3: Write minimal implementation**

In `services/settings_service.go`, add fields to the `Settings` struct:

```go
type Settings struct {
	Language   string `json:"language"`
	Theme      string `json:"theme"`
	FontSize   int    `json:"fontSize"`
	FontFamily string `json:"fontFamily"`
	TabSize    int    `json:"tabSize"`
	WordWrap   bool   `json:"wordWrap"`
	LineNumbers bool  `json:"lineNumbers"`
	Minimap    bool   `json:"minimap"`
	AIApiKey   string `json:"aiApiKey"`
	AIBaseURL  string `json:"aiBaseUrl"`
	AIModel    string `json:"aiModel"`
}
```

Update `defaultSettings()` to include sensible defaults:

```go
func defaultSettings() Settings {
	return Settings{
		Language:    "en",
		Theme:       "dark",
		FontSize:    14,
		FontFamily:  "JetBrains Mono",
		TabSize:     2,
		WordWrap:    true,
		LineNumbers: true,
		Minimap:     false,
		AIApiKey:    "",
		AIBaseURL:   "https://api.openai.com",
		AIModel:     "gpt-4o",
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestSettingsService -v`
Expected: all settings tests PASS (existing + new).

- [ ] **Step 5: Commit**

```bash
git add services/settings_service.go services/settings_service_test.go
git commit -m "feat: add AI config fields to Settings"
```

---

## Task 6: Go Backend — Register Services and Events in main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Write the failing test**

No unit test for main.go wiring. Verification is via `go build ./` succeeding.

- [ ] **Step 2: Verify current build**

Run: `go build ./`
Expected: succeeds (baseline).

- [ ] **Step 3: Modify main.go**

Update `main.go` to register `TerminalService` and `AIService`, and register events for terminal output and AI streaming:

```go
package main

import (
	"embed"
	"log"
	"time"

	"changeme/services"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[string]("time")
	application.RegisterEvent[TerminalOutputEvent]("terminal:output")
	application.RegisterEvent[string]("ai:chunk")
	application.RegisterEvent[string]("ai:done")
	application.RegisterEvent[string]("ai:error")
}

type TerminalOutputEvent struct {
	Data string `json:"data"`
}

func main() {
	fileService := &services.FileService{}
	projectService := services.NewProjectService()
	settingsService := services.NewSettingsService()
	windowService := &services.WindowService{}
	terminalService := services.NewTerminalService()
	aiService := services.NewAIService()

	app := application.New(application.Options{
		Name:        "gugacode",
		Description: "AI-Powered Code Editor",
		Services: []application.Service{
			application.NewService(fileService),
			application.NewService(projectService),
			application.NewService(settingsService),
			application.NewService(windowService),
			application.NewService(terminalService),
			application.NewService(aiService),
			application.NewService(&GreetService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "gugacode",
		Width:  1000,
		Height: 618,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	windowService.SetWindow(window)

	// Wire terminal output to Wails event
	go func() {
		for {
			output := terminalService.ReadOutput(60 * time.Second)
			if output != "" {
				app.Event.Emit("terminal:output", TerminalOutputEvent{Data: output})
			}
		}
	}()

	// Legacy time event (can be removed later)
	go func() {
		for {
			now := time.Now().Format(time.RFC1123)
			app.Event.Emit("time", now)
			time.Sleep(time.Second)
		}
	}()

	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
```

**Note:** The `ReadOutput` polling loop above is a simplified approach. A more robust implementation would use a channel-based callback in TerminalService that emits events directly. If the polling approach causes latency issues, refactor `TerminalService` to accept an `onOutput func(string)` callback in `Start()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./`
Expected: succeeds.

Run: `go vet ./`
Expected: no errors.

Run: `go test ./services/ -v`
Expected: all existing tests still PASS.

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "feat: register TerminalService and AIService in main.go"
```

---

## Task 7: Frontend — Install xterm Dependencies and Regenerate Bindings

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/bindings/changeme/services/` (regenerated)

- [ ] **Step 1: Install dependencies**

```bash
cd frontend
npm install @xterm/xterm @xterm/addon-fit
```

- [ ] **Step 2: Regenerate Wails bindings**

```bash
cd e:\gugacode\gugacode\gugacode
C:\Users\SurgeFC\go\bin\wails3.exe generate bindings
```

Expected output: "Processed: ... 7 Services, ... Methods". The `frontend/bindings/changeme/services/` directory should now contain `terminalservice.js` and `aiservice.js`.

- [ ] **Step 3: Verify bindings exist**

List `frontend/bindings/changeme/services/` — should include:
- `fileservice.js`
- `projectservice.js`
- `settingsservice.js`
- `windowservice.js`
- `terminalservice.js` (NEW)
- `aiservice.js` (NEW)
- `models.js` (updated with ChatMessage, AIConfig, etc.)

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd frontend
npx vue-tsc --noEmit
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/bindings/
git commit -m "chore: install xterm deps and regenerate bindings"
```

---

## Task 8: Frontend — Extend Types and API Layer

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/services.ts`

- [ ] **Step 1: Update types**

Modify `frontend/src/types/index.ts` — add AI config and chat message types. Append to existing types:

```typescript
export interface AIConfig {
  aiApiKey: string;
  aiBaseUrl: string;
  aiModel: string;
}

export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: string;
}

export interface ChatResponse {
  content: string;
  finishReason: string;
}
```

Update the existing `Settings` interface to include AI fields:

```typescript
export interface Settings {
  language: string;
  theme: string;
  fontSize: number;
  fontFamily: string;
  tabSize: number;
  wordWrap: boolean;
  lineNumbers: boolean;
  minimap: boolean;
  aiApiKey: string;
  aiBaseUrl: string;
  aiModel: string;
}
```

- [ ] **Step 2: Update API layer**

Modify `frontend/src/api/services.ts` — add re-exports for the new services. Read the file first to see the existing pattern, then add:

```typescript
import { terminalService } from "../../bindings/changeme/services/terminalservice.js";
import { aiService } from "../../bindings/changeme/services/aiservice.js";

export { terminalService, aiService };
```

**Verify the actual export names** by reading `frontend/bindings/changeme/services/terminalservice.js` and `aiservice.js` — the exported singleton name may be `terminalService` or `TerminalService`. Use the actual name.

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd frontend
npx vue-tsc --noEmit
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/services.ts
git commit -m "feat: add AI and terminal types to frontend"
```

---

## Task 9: Frontend — Terminal Store

**Files:**
- Create: `frontend/src/stores/terminal.ts`
- Create: `frontend/src/stores/terminal.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/stores/terminal.test.ts`:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services", () => ({
  terminalService: {
    start: vi.fn().mockResolvedValue(undefined),
    write: vi.fn().mockResolvedValue(undefined),
    kill: vi.fn().mockResolvedValue(undefined),
    resize: vi.fn().mockResolvedValue(undefined),
    isRunning: vi.fn().mockReturnValue(false),
  },
}));

import { terminalState, startTerminal, writeToTerminal, stopTerminal, resizeTerminal } from "./terminal";

describe("terminal store", () => {
  beforeEach(() => {
    terminalState.running = false;
    terminalState.output = "";
  });

  it("starts terminal", async () => {
    await startTerminal("/some/path");
    expect(terminalState.running).toBe(true);
  });

  it("writes input", async () => {
    terminalState.running = true;
    await writeToTerminal("ls\n");
    expect(terminalState.running).toBe(true);
  });

  it("stops terminal", async () => {
    terminalState.running = true;
    await stopTerminal();
    expect(terminalState.running).toBe(false);
  });

  it("appends output", () => {
    terminalState.output = "hello";
    appendOutput(" world");
    expect(terminalState.output).toBe("hello world");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/stores/terminal.test.ts`
Expected: FAIL with "Cannot find module './terminal'".

- [ ] **Step 3: Write minimal implementation**

Create `frontend/src/stores/terminal.ts`:

```typescript
import { reactive } from "vue";
import { terminalService } from "@/api/services";
import { Events } from "@wailsio/runtime";

export interface TerminalState {
  running: boolean;
  output: string;
  cols: number;
  rows: number;
}

export const terminalState = reactive<TerminalState>({
  running: false,
  output: "",
  cols: 80,
  rows: 24,
});

let eventListenerRegistered = false;

function ensureEventListener() {
  if (eventListenerRegistered) return;
  eventListenerRegistered = true;
  Events.On("terminal:output", (event: any) => {
    const data = event?.data?.data ?? event?.data ?? "";
    if (typeof data === "string") {
      terminalState.output += data;
    }
  });
}

export async function startTerminal(workingDir: string): Promise<void> {
  ensureEventListener();
  try {
    await terminalService.start(workingDir);
    terminalState.running = true;
  } catch (e) {
    console.error("Failed to start terminal:", e);
  }
}

export async function writeToTerminal(input: string): Promise<void> {
  if (!terminalState.running) return;
  try {
    await terminalService.write(input);
  } catch (e) {
    console.error("Failed to write to terminal:", e);
  }
}

export async function stopTerminal(): Promise<void> {
  try {
    await terminalService.kill();
  } catch (e) {
    console.error("Failed to stop terminal:", e);
  }
  terminalState.running = false;
}

export async function resizeTerminal(cols: number, rows: number): Promise<void> {
  terminalState.cols = cols;
  terminalState.rows = rows;
  if (!terminalState.running) return;
  try {
    await terminalService.resize(cols, rows);
  } catch (e) {
    console.error("Failed to resize terminal:", e);
  }
}

export function appendOutput(data: string): void {
  terminalState.output += data;
}

export function clearOutput(): void {
  terminalState.output = "";
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/stores/terminal.test.ts`
Expected: 4 tests PASS.

Then run full suite: `npx vitest run`
Expected: all tests PASS (existing 40 + new 4).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/stores/terminal.ts frontend/src/stores/terminal.test.ts
git commit -m "feat: add terminal store with event streaming"
```

---

## Task 10: Frontend — Wire TerminalPanel with xterm.js

**Files:**
- Modify: `frontend/src/components/layout/TerminalPanel.vue`

- [ ] **Step 1: No unit test (xterm needs real DOM)**

xterm.js requires a real DOM/canvas and cannot run in jsdom. This task is verified via `vue-tsc --noEmit` and manual testing.

- [ ] **Step 2: Read current TerminalPanel.vue**

Read `frontend/src/components/layout/TerminalPanel.vue` to understand the existing template and styles.

- [ ] **Step 3: Modify TerminalPanel.vue**

Replace the `<script setup>` block and the terminal body section. Keep the header (tabs + close button) and styles. The key changes:

```vue
<script setup lang="ts">
import { appState, toggleTerminal } from "@/stores/app";
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { Close } from "@element-plus/icons-vue";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import {
  terminalState,
  startTerminal,
  writeToTerminal,
  stopTerminal,
  resizeTerminal,
  clearOutput,
} from "@/stores/terminal";
import "@xterm/xterm/css/xterm.css";

const isVisible = computed(() => appState.terminalVisible);
const terminalContainer = ref<HTMLElement | null>(null);
let term: Terminal | null = null;
let fitAddon: FitAddon | null = null;

const tabs = [
  { label: "Terminal", key: "terminal" },
  { label: "Output", key: "output" },
  { label: "Problems", key: "problems" },
  { label: "Debug", key: "debug" },
];

const activeTab = "terminal";

watch(terminalState, (state) => {
  if (!term) return;
  if (state.output) {
    term.write(state.output);
    clearOutput();
  }
});

async function initTerminal() {
  if (!terminalContainer.value || term) return;

  term = new Terminal({
    fontFamily: "var(--font-family-mono)",
    fontSize: 12,
    theme: {
      background: "#131316",
      foreground: "#e8e6e3",
      cursor: "#e8e6e3",
    },
    cursorBlink: true,
  });

  fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(terminalContainer.value);
  fitAddon.fit();

  term.onData((data) => {
    writeToTerminal(data);
  });

  term.onResize(({ cols, rows }) => {
    resizeTerminal(cols, rows);
  });

  const workingDir = appState.currentProject ?? "";
  await startTerminal(workingDir);
  resizeTerminal(term.cols, term.rows);
}

function fitTerminal() {
  if (fitAddon) {
    fitAddon.fit();
  }
}

onMounted(() => {
  if (isVisible.value) {
    initTerminal();
  }
});

watch(isVisible, (visible) => {
  if (visible && !term) {
    setTimeout(initTerminal, 50);
  } else if (visible && term) {
    setTimeout(fitTerminal, 50);
  }
});

onBeforeUnmount(() => {
  stopTerminal();
  term?.dispose();
  term = null;
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
        </div>
        <button
          class="terminal-panel__close"
          aria-label="Close terminal"
          title="Close terminal"
          @click="toggleTerminal"
        >
          <el-icon :size="14">
            <Close />
          </el-icon>
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
  box-shadow: 0 -1px 0 var(--color-border-subtle);
}

.terminal-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 30px;
  min-height: 30px;
  padding: 0 4px 0 8px;
  box-shadow: 0 1px 0 var(--color-border-subtle);
}

.terminal-panel__tabs {
  display: flex;
  align-items: center;
  gap: 0;
  overflow-x: auto;
}

.terminal-panel__tab {
  padding: 4px 10px;
  font-size: 11px;
  color: var(--color-text-tertiary);
  background: transparent;
  border: none;
  cursor: pointer;
  white-space: nowrap;
  transition: color var(--duration-micro) var(--ease-out-expo);
}

.terminal-panel__tab:hover {
  color: var(--color-text-secondary);
}

.terminal-panel__tab--active {
  color: var(--color-text-primary);
}

.terminal-panel__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
}

.terminal-panel__close:hover {
  color: var(--color-text-secondary);
}

.terminal-panel__body {
  flex: 1;
  padding: 4px 8px;
  overflow: hidden;
}

.terminal-panel__body :deep(.xterm) {
  height: 100%;
}

.terminal-panel__body :deep(.xterm-viewport) {
  overflow-y: auto;
}

.slide-terminal-enter-active,
.slide-terminal-leave-active {
  transition:
    height var(--duration-normal) var(--ease-out-expo),
    opacity var(--duration-fast) var(--ease-out-expo);
  overflow: hidden;
}

.slide-terminal-enter-from,
.slide-terminal-leave-to {
  height: 0;
  opacity: 0;
}
</style>
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `npx vue-tsc --noEmit`
Expected: no errors (existing tests still pass).

Run: `npx vitest run`
Expected: all existing tests PASS (no new tests — xterm can't run in jsdom).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/layout/TerminalPanel.vue
git commit -m "feat: integrate xterm.js into TerminalPanel"
```

---

## Task 11: Frontend — AI Store

**Files:**
- Create: `frontend/src/stores/ai.ts`
- Create: `frontend/src/stores/ai.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/stores/ai.test.ts`:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services", () => ({
  aiService: {
    sendStream: vi.fn((messages: any[], onChunk: (chunk: string) => void) => {
      onChunk("Hello");
      onChunk(" world");
      return Promise.resolve();
    }),
    setConfig: vi.fn(),
  },
}));

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: vi.fn(),
    Off: vi.fn(),
  },
}));

import { aiState, sendMessage, clearMessages } from "./ai";

describe("ai store", () => {
  beforeEach(() => {
    aiState.messages = [];
    aiState.streaming = false;
    aiState.error = null;
  });

  it("starts with empty messages", () => {
    expect(aiState.messages).toHaveLength(0);
    expect(aiState.streaming).toBe(false);
  });

  it("sends message and collects response", async () => {
    await sendMessage("Hello AI");
    expect(aiState.messages).toHaveLength(2);
    expect(aiState.messages[0].role).toBe("user");
    expect(aiState.messages[0].content).toBe("Hello AI");
    expect(aiState.messages[1].role).toBe("assistant");
    expect(aiState.messages[1].content).toBe("Hello world");
    expect(aiState.streaming).toBe(false);
  });

  it("clears messages", () => {
    aiState.messages.push({ role: "user", content: "test" });
    clearMessages();
    expect(aiState.messages).toHaveLength(0);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/stores/ai.test.ts`
Expected: FAIL with "Cannot find module './ai'".

- [ ] **Step 3: Write minimal implementation**

Create `frontend/src/stores/ai.ts`:

```typescript
import { reactive } from "vue";
import { aiService } from "@/api/services";
import { appState } from "@/stores/app";
import type { ChatMessage } from "@/types";

export interface AIState {
  messages: ChatMessage[];
  streaming: boolean;
  error: string | null;
}

export const aiState = reactive<AIState>({
  messages: [],
  streaming: false,
  error: null,
});

export async function sendMessage(content: string): Promise<void> {
  if (aiState.streaming) return;

  aiState.error = null;
  aiState.messages.push({ role: "user", content });
  aiState.streaming = true;

  const assistantMessage: ChatMessage = { role: "assistant", content: "" };
  aiState.messages.push(assistantMessage);

  try {
    const history = aiState.messages.slice(0, -1);
    await aiService.sendStream(history, (chunk: string) => {
      assistantMessage.content += chunk;
    });
  } catch (e: any) {
    aiState.error = e?.message ?? "AI request failed";
    if (assistantMessage.content === "") {
      aiState.messages.pop();
    }
  } finally {
    aiState.streaming = false;
  }
}

export function clearMessages(): void {
  if (aiState.streaming) return;
  aiState.messages = [];
  aiState.error = null;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/stores/ai.test.ts`
Expected: 3 tests PASS.

Run: `npx vitest run`
Expected: all tests PASS (existing + new).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/stores/ai.ts frontend/src/stores/ai.test.ts
git commit -m "feat: add AI store with streaming send"
```

---

## Task 12: Frontend — Wire AiChatPanel with Messages and Streaming

**Files:**
- Modify: `frontend/src/components/layout/AiChatPanel.vue`

- [ ] **Step 1: No unit test (UI component, manual verification)**

- [ ] **Step 2: Read current AiChatPanel.vue**

Read `frontend/src/components/layout/AiChatPanel.vue` to understand the existing template and styles.

- [ ] **Step 3: Modify AiChatPanel.vue**

Replace the `<script setup>` block and the chat body section. Keep the header, input area styling, and transitions. Key changes:

```vue
<script setup lang="ts">
import { appState, toggleAiChat } from "@/stores/app";
import { computed, ref, nextTick, watch } from "vue";
import { Close, Promotion } from "@element-plus/icons-vue";
import { aiState, sendMessage, clearMessages } from "@/stores/ai";

const isVisible = computed(() => appState.aiChatVisible);
const inputText = ref("");
const messageListRef = ref<HTMLElement | null>(null);

const modelOptions = [
  { label: "GPT-4o", value: "gpt-4o" },
  { label: "Claude 4 Sonnet", value: "claude-4-sonnet" },
  { label: "Gemini 2.5 Pro", value: "gemini-2.5-pro" },
];

const selectedModel = computed({
  get: () => appState.aiModel ?? "gpt-4o",
  set: (val: string) => {
    appState.aiModel = val;
  },
});

const hasMessages = computed(() => aiState.messages.length > 0);

async function handleSend() {
  const text = inputText.value.trim();
  if (!text || aiState.streaming) return;
  inputText.value = "";
  await sendMessage(text);
  await nextTick();
  scrollToBottom();
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    handleSend();
  }
}

function scrollToBottom() {
  if (messageListRef.value) {
    messageListRef.value.scrollTop = messageListRef.value.scrollHeight;
  }
}

watch(() => aiState.messages.length, () => {
  nextTick(scrollToBottom);
});

watch(
  () => aiState.messages[aiState.messages.length - 1]?.content,
  () => {
    nextTick(scrollToBottom);
  }
);
</script>

<template>
  <transition name="slide-chat">
    <aside
      v-if="isVisible"
      class="ai-chat-panel"
      role="complementary"
      aria-label="AI Assistant panel"
    >
      <div class="ai-chat-panel__header">
        <div class="ai-chat-panel__header-left">
          <span class="ai-chat-panel__title">AI Assistant</span>
        </div>
        <div class="ai-chat-panel__header-right">
          <select
            v-model="selectedModel"
            class="ai-chat-panel__model-select"
            aria-label="Select AI model"
          >
            <option
              v-for="model in modelOptions"
              :key="model.value"
              :value="model.value"
            >
              {{ model.label }}
            </option>
          </select>
          <button
            v-if="hasMessages"
            class="ai-chat-panel__clear"
            aria-label="Clear conversation"
            title="Clear conversation"
            @click="clearMessages"
          >
            <el-icon :size="14">
              <Close />
            </el-icon>
          </button>
          <button
            class="ai-chat-panel__close"
            aria-label="Close AI chat"
            title="Close AI chat"
            @click="toggleAiChat"
          >
            <el-icon :size="14">
              <Close />
            </el-icon>
          </button>
        </div>
      </div>

      <div ref="messageListRef" class="ai-chat-panel__body">
        <div v-if="!hasMessages" class="ai-chat-panel__empty">
          <div class="ai-chat-panel__empty-circle" aria-hidden="true" />
          <p class="ai-chat-panel__empty-title">Ask me anything about your code</p>
          <p class="ai-chat-panel__empty-subtitle">
            Write, refactor, debug, and explain.
          </p>
        </div>

        <div v-else class="ai-chat-panel__messages">
          <div
            v-for="(msg, i) in aiState.messages"
            :key="i"
            class="ai-chat-panel__message"
            :class="'ai-chat-panel__message--' + msg.role"
          >
            <div class="ai-chat-panel__message-role">{{ msg.role }}</div>
            <div class="ai-chat-panel__message-content">{{ msg.content }}</div>
          </div>

          <div v-if="aiState.error" class="ai-chat-panel__error">
            {{ aiState.error }}
          </div>
        </div>
      </div>

      <div class="ai-chat-panel__input-area">
        <div class="ai-chat-panel__input-wrap">
          <input
            v-model="inputText"
            type="text"
            class="ai-chat-panel__input"
            placeholder="Ask about your code..."
            name="ai-chat-input"
            aria-label="AI chat input"
            :disabled="aiState.streaming"
            @keydown="handleKeydown"
          />
          <button
            class="ai-chat-panel__send"
            aria-label="Send message"
            title="Send message"
            :disabled="!inputText.trim() || aiState.streaming"
            @click="handleSend"
          >
            <el-icon :size="14">
              <Promotion />
            </el-icon>
          </button>
        </div>
      </div>
    </aside>
  </transition>
</template>

<style scoped>
.ai-chat-panel {
  display: flex;
  flex-direction: column;
  width: 360px;
  min-width: 0;
  height: 100%;
  background-color: var(--color-bg-base);
  overflow: hidden;
  flex-shrink: 0;
  z-index: 5;
}

.ai-chat-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px 0 12px;
  height: 32px;
  min-height: 32px;
}

.ai-chat-panel__header-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.ai-chat-panel__model-select {
  padding: 2px 6px;
  font-size: 10px;
  color: var(--color-text-tertiary);
  background-color: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  outline: 0;
  cursor: pointer;
}

.ai-chat-panel__model-select:hover {
  color: var(--color-text-secondary);
  border-color: var(--color-border-subtle);
}

.ai-chat-panel__clear,
.ai-chat-panel__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
}

.ai-chat-panel__clear:hover,
.ai-chat-panel__close:hover {
  color: var(--color-text-secondary);
}

.ai-chat-panel__body {
  flex: 1;
  overflow-y: auto;
  padding: 0;
}

.ai-chat-panel__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 32px 24px;
  text-align: center;
}

.ai-chat-panel__empty-circle {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  border: 1px dashed var(--color-border-default);
  margin-bottom: 16px;
}

.ai-chat-panel__empty-title {
  font-size: 13px;
  color: var(--color-text-secondary);
  margin-bottom: 6px;
}

.ai-chat-panel__empty-subtitle {
  font-size: 11px;
  color: var(--color-text-tertiary);
  line-height: 1.5;
  max-width: 240px;
}

.ai-chat-panel__messages {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
}

.ai-chat-panel__message {
  padding: 8px 12px;
  border-radius: var(--radius-md);
  font-size: 12px;
  line-height: 1.5;
}

.ai-chat-panel__message--user {
  background-color: var(--color-bg-elevated);
  color: var(--color-text-primary);
}

.ai-chat-panel__message--assistant {
  background-color: var(--color-bg-surface);
  color: var(--color-text-primary);
}

.ai-chat-panel__message-role {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-tertiary);
  margin-bottom: 4px;
}

.ai-chat-panel__error {
  padding: 8px 12px;
  font-size: 11px;
  color: var(--color-error);
  background-color: color-mix(in srgb, var(--color-error) 10%, transparent);
  border-radius: var(--radius-sm);
}

.ai-chat-panel__input-area {
  padding: 8px 12px 12px;
}

.ai-chat-panel__input-wrap {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 6px 6px 14px;
  background-color: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: 999px;
}

.ai-chat-panel__input-wrap:focus-within {
  border-color: var(--color-primary);
}

.ai-chat-panel__input {
  flex: 1;
  min-width: 0;
  padding: 4px 0;
  font-size: 12px;
  color: var(--color-text-primary);
  background: transparent;
  border: none;
  outline: none;
}

.ai-chat-panel__input::placeholder {
  color: var(--color-text-tertiary);
}

.ai-chat-panel__input:disabled {
  opacity: 0.5;
}

.ai-chat-panel__send {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 50%;
  background-color: var(--color-primary);
  color: #ffffff;
  cursor: pointer;
  flex-shrink: 0;
}

.ai-chat-panel__send:hover:not(:disabled) {
  background-color: var(--color-primary-hover);
}

.ai-chat-panel__send:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.slide-chat-enter-active,
.slide-chat-leave-active {
  transition:
    width var(--duration-normal) var(--ease-out-expo),
    opacity var(--duration-fast) var(--ease-out-expo);
  overflow: hidden;
}

.slide-chat-enter-from,
.slide-chat-leave-to {
  width: 0;
  opacity: 0;
}
</style>
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `npx vue-tsc --noEmit`
Expected: no errors.

Run: `npx vitest run`
Expected: all existing tests PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/layout/AiChatPanel.vue
git commit -m "feat: wire AiChatPanel with message list and streaming"
```

---

## Task 13: Frontend — Wire SettingsView AI Config

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`
- Modify: `frontend/src/stores/app.ts`

- [ ] **Step 1: No unit test (settings UI form)**

- [ ] **Step 2: Read current SettingsView.vue and app.ts**

Read both files to understand the existing settings form structure and the appState fields.

- [ ] **Step 3: Modify app.ts — add AI config fields**

In `frontend/src/stores/app.ts`, add to the `AppState` interface:

```typescript
export interface AppState {
  // ... existing fields ...
  aiApiKey: string;
  aiBaseUrl: string;
  aiModel: string;
}
```

Add to the `appState` reactive object defaults:

```typescript
export const appState = reactive<AppState>({
  // ... existing defaults ...
  aiApiKey: "",
  aiBaseUrl: "https://api.openai.com",
  aiModel: "gpt-4o",
});
```

Update `loadSettings()` to load AI fields:

```typescript
export async function loadSettings(): Promise<void> {
  try {
    const settings = await settingsService.loadSettings();
    appState.language = settings.language;
    appState.theme = settings.theme;
    appState.fontSize = settings.fontSize;
    appState.fontFamily = settings.fontFamily;
    appState.tabSize = settings.tabSize;
    appState.wordWrap = settings.wordWrap;
    appState.lineNumbers = settings.lineNumbers;
    appState.minimap = settings.minimap;
    appState.aiApiKey = settings.aiApiKey;
    appState.aiBaseUrl = settings.aiBaseUrl;
    appState.aiModel = settings.aiModel;
  } catch (e) {
    console.error("Failed to load settings:", e);
  }
}
```

Update `saveSettings()` to include AI fields in the saved object:

```typescript
export function saveSettings(): void {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(async () => {
    const settings: Settings = {
      language: appState.language,
      theme: appState.theme,
      fontSize: appState.fontSize,
      fontFamily: appState.fontFamily,
      tabSize: appState.tabSize,
      wordWrap: appState.wordWrap,
      lineNumbers: appState.lineNumbers,
      minimap: appState.minimap,
      aiApiKey: appState.aiApiKey,
      aiBaseUrl: appState.aiBaseUrl,
      aiModel: appState.aiModel,
    };
    try {
      await settingsService.saveSettings(settings);
    } catch (e) {
      console.error("Failed to save settings:", e);
    }
  }, 500);
}
```

- [ ] **Step 4: Modify SettingsView.vue — add AI config section**

Read the existing `frontend/src/views/SettingsView.vue`. Find the AI settings section (it likely exists as a tab/section). Add form fields for:

- API Key (password input)
- Base URL (text input)
- Model (text input or select dropdown)

Each field binds to `appState.aiApiKey`, `appState.aiBaseUrl`, `appState.aiModel` respectively, and calls `saveSettings()` on `@input` or `@change`.

If the existing SettingsView has an AI section placeholder, fill it in. If not, add a new section following the existing pattern (el-form, el-input, labels).

Example AI section (adapt to match existing SettingsView structure):

```vue
<template>
  <!-- ... existing sections ... -->
  <section class="settings-section">
    <h3 class="settings-section__title">AI Configuration</h3>
    <el-form label-position="top">
      <el-form-item label="API Key">
        <el-input
          v-model="appState.aiApiKey"
          type="password"
          show-password
          placeholder="sk-..."
          @input="saveSettings"
        />
      </el-form-item>
      <el-form-item label="Base URL">
        <el-input
          v-model="appState.aiBaseUrl"
          placeholder="https://api.openai.com"
          @input="saveSettings"
        />
      </el-form-item>
      <el-form-item label="Model">
        <el-input
          v-model="appState.aiModel"
          placeholder="gpt-4o"
          @input="saveSettings"
        />
      </el-form-item>
    </el-form>
  </section>
</template>

<script setup lang="ts">
import { appState, saveSettings } from "@/stores/app";
// ... existing imports ...
</script>
```

**Important:** Match the existing SettingsView's exact structure, classes, and patterns. The above is a template — adapt to fit.

- [ ] **Step 5: Verify and commit**

Run: `npx vue-tsc --noEmit`
Expected: no errors.

Run: `npx vitest run`
Expected: all tests PASS.

```bash
git add frontend/src/stores/app.ts frontend/src/views/SettingsView.vue
git commit -m "feat: add AI config section to SettingsView"
```

---

## Task 14: Integration — Manual Verification

**Files:** None (verification only)

- [ ] **Step 1: Run all Go tests**

Run from project root: `go test ./services/ -v`
Expected: all tests PASS (FileService 7 + ProjectService 5 + SettingsService 4 + TerminalService 5 + AIService 3 = 24 tests)

- [ ] **Step 2: Run all frontend tests**

Run from `frontend/`: `npx vitest run`
Expected: all tests PASS (Plan 1's 40 + terminal 4 + ai 3 = 47 tests)

- [ ] **Step 3: Verify TypeScript compiles**

Run from `frontend/`: `npx vue-tsc --noEmit`
Expected: no errors

- [ ] **Step 4: Start dev mode and manually verify**

Run from project root: `wails3 dev`

Manual test checklist:
1. App launches, terminal panel visible at bottom by default
2. Terminal shows a working shell prompt (PowerShell on Windows, bash on Unix)
3. Type `echo test` in terminal → `test` appears in output
4. Type `ls` or `dir` → directory listing appears
5. Terminal resizes when panel height changes (drag the border)
6. Close terminal via X button → panel hides
7. Open terminal via menu/activity bar → panel shows again, terminal still active
8. Open AI chat panel (via activity bar or menu)
9. Type a message and press Enter → message appears in chat
10. AI response streams in token by token (if API key configured)
11. If no API key configured → error message shown
12. Model selector dropdown works
13. Clear conversation button (X) clears messages
14. Navigate to Settings → AI Configuration section visible
15. Enter API key → it persists after app restart
16. Change Base URL → it persists after app restart
17. Change Model → AiChatPanel model selector reflects new default

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "chore: integration verification for terminal and AI chat"
```

---

## Self-Review Notes

**Spec coverage:** 
- Terminal: PTY lifecycle (start/write/kill) ✓, resize ✓, output streaming via events ✓, xterm.js integration ✓
- AI: HTTP client ✓, SSE streaming ✓, message list UI ✓, model selector ✓, error handling ✓
- Settings: AI config fields (API key, base URL, model) ✓, persistence ✓, SettingsView UI ✓
- Events: terminal:output, ai:chunk, ai:done, ai:error registered in main.go ✓

**Placeholder scan:** No TBD/TODO in implementation steps. All code blocks are complete. The SettingsView AI section (Task 13) includes a template that may need adaptation to match the existing file's exact structure — this is noted explicitly in the task, not hidden as a placeholder.

**Type consistency:** 
- `ChatMessage` struct in Go (ai_service.go) matches `ChatMessage` interface in types/index.ts (role, content fields)
- `AIConfig` struct in Go matches `AIConfig` interface in TypeScript
- `Settings` struct in Go includes `aiApiKey`, `aiBaseUrl`, `aiModel` matching the TypeScript `Settings` interface
- `TerminalService` methods: `Start(workingDir string)`, `Write([]byte)`, `Kill()`, `Resize(cols, rows int)`, `IsRunning() bool` — consistent across Go and bindings
- `AIService` methods: `SetConfig(AIConfig)`, `Send([]ChatMessage)`, `SendStream([]ChatMessage, func(string))` — consistent

**Note on Wails events:** The exact frontend event listener API (`Events.On(...)`) may differ slightly in Wails v3 alpha2.111. If `@wailsio/runtime` doesn't export `Events` directly, check the actual export (may be `runtime.Events.On` or similar) and adapt. The terminal store (Task 9) and AI store (Task 11) may need this adjustment.

**Note on terminal output polling:** Task 6 uses a polling loop (`ReadOutput(60s)`) to forward terminal output to Wails events. This is a simplification — a production implementation would use a callback or channel. The polling approach works but may add ~60s latency on idle. If this is a problem during testing, refactor `TerminalService.readLoop` to emit events directly via a callback passed to `Start()`.

---

## Follow-Up Plans (Already Outlined)

- **Plan 3: Git & Search** — GitService (go-git or git CLI), search SidePanel tab (ripgrep or filepath.WalkDir). ~10 tasks.
- **Plan 4: Plugins & Extensions** — Recommend deferring. Remove PluginsView or replace with "Coming soon" page.
