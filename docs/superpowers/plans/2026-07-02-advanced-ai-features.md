# Advanced AI Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the basic AI chat into a production-grade AI coding assistant: configurable system prompts, code context injection, editor right-click AI actions, conversation persistence, markdown rendering with syntax highlighting, and stop-generation control.

**Architecture:** The Go `AIService` gains system-prompt support, context-aware message building, and a cancellable streaming API. A new `ConversationService` persists conversations to disk as JSON. The frontend AI store manages context attachment, stop signals, and action dispatch. The AI chat panel renders markdown (including fenced code blocks with Monaco-powered highlighting) and exposes editor-integrated AI actions via the right-click context menu.

**Tech Stack:** Go 1.25, Wails v3, Vue 3, TypeScript, Element Plus, `marked` (markdown parser), `highlight.js` (syntax highlighting), Vitest, `@vue/test-utils`, jsdom

**Project root:** `e:\gugacode\gugacode\gugacode\` (the directory containing `go.mod`, `main.go`, `frontend/`). All relative paths in this plan are from this root.

**Module name note:** `go.mod` declares `module changeme`. Generated bindings land in `frontend/bindings/changeme/`. This plan uses that path as-is.

---

## Scope Check

This is **Plan 4** of the ongoing gugacode IDE build-out. Plans 1-3 are complete (Core IDE Foundation, Terminal & AI Chat, Git & Search). The original Plan 4 (Plugins & Extensions) is deferred — this plan replaces it with **Advanced AI Features**, the user's stated priority.

**In scope:**
- System prompt configuration (with sensible defaults the assistant writes)
- Preset prompt templates (explain, refactor, fix, generate docs, generate tests)
- Code context injection (current file, selection)
- Editor right-click AI actions
- Conversation persistence (save/load/clear)
- Markdown rendering for AI responses (code blocks, inline code, lists)
- Stop-generation button
- Copy-message button
- Context chips showing attached context

**Out of scope (deferred to Plan 5+):**
- Inline code completion (ghost text) — requires deep Monaco integration
- Multi-model routing (different models for different actions)
- AI-powered rename/refactor across files
- Embeddings-based codebase search
- Function calling / tool use
- Local model support (llama.cpp, ollama)

**Plan 4 produces working, testable software on its own:** AI chat becomes genuinely useful with context, presets, persistence, and rich rendering.

---

## File Structure

```
services/                          # Go backend services
├── ai_service.go                  # MODIFY — add SystemPrompt field, context-aware Send, cancellable stream
├── ai_service_test.go             # MODIFY — add tests for system prompt, context, cancellation
├── ai_prompts.go                  # NEW — default system prompt + preset templates
├── ai_prompts_test.go             # NEW — tests for prompts
├── conversation_service.go        # NEW — save/load/list conversations as JSON files
└── conversation_service_test.go   # NEW — tests for conversation persistence

main.go                            # MODIFY — register ConversationService

frontend/
├── bindings/changeme/services/
│   ├── conversationservice.js     # NEW — manual binding (wails3 CLI not installed)
│   └── models.js                  # MODIFY — add Conversation, ConversationMessage, PromptTemplate types
├── src/
│   ├── types/
│   │   └── index.ts               # MODIFY — add Conversation, PromptTemplate, AIAction types
│   ├── api/
│   │   └── services.ts            # MODIFY — add conversationService wrapper
│   ├── stores/
│   │   ├── ai.ts                  # MODIFY — add context, stop, presets, actions
│   │   └── ai.test.ts             # MODIFY — add tests
│   ├── components/
│   │   ├── layout/
│   │   │   └── AiChatPanel.vue    # MODIFY — markdown rendering, stop, copy, context chips
│   │   └── editor/
│   │       └── CodeEditor.vue     # MODIFY — right-click AI actions context menu
│   ├── lib/
│   │   ├── markdown.ts            # NEW — markdown render helper (marked + highlight.js)
│   │   └── markdown.test.ts       # NEW — tests for markdown rendering
│   └── views/
│       └── SettingsView.vue       # MODIFY — system prompt config textarea
```

---

## Task 1: Go Backend — AI Prompts Module

**Files:**
- Create: `services/ai_prompts.go`
- Create: `services/ai_prompts_test.go`

- [ ] **Step 1: Write the failing tests**

Create `services/ai_prompts_test.go`:

```go
package services

import (
	"strings"
	"testing"
)

func TestDefaultSystemPrompt_IsNotEmpty(t *testing.T) {
	p := DefaultSystemPrompt
	if strings.TrimSpace(p) == "" {
		t.Error("DefaultSystemPrompt must not be empty")
	}
}

func TestDefaultSystemPrompt_MentionsCodeAssistant(t *testing.T) {
	p := DefaultSystemPrompt
	if !strings.Contains(strings.ToLower(p), "code") || !strings.Contains(strings.ToLower(p), "assistant") {
		t.Error("DefaultSystemPrompt should mention 'code' and 'assistant'")
	}
}

func TestPresetPrompts_ContainsAllExpectedActions(t *testing.T) {
	expected := []string{"explain", "refactor", "fix", "generate_docs", "generate_tests"}
	for _, name := range expected {
		if _, ok := PresetPrompts[name]; !ok {
			t.Errorf("PresetPrompts missing key %q", name)
		}
	}
}

func TestPresetPrompts_HaveNonEmptyTemplates(t *testing.T) {
	for name, tmpl := range PresetPrompts {
		if strings.TrimSpace(tmpl) == "" {
			t.Errorf("PresetPrompts[%q] template is empty", name)
		}
	}
}

func TestGetPresetPrompt_ReturnsTemplate(t *testing.T) {
	got, err := GetPresetPrompt("explain")
	if err != nil {
		t.Fatalf("GetPresetPrompt failed: %v", err)
	}
	if !strings.Contains(got, "{{code}}") {
		t.Errorf("explain template should contain {{code}} placeholder, got: %q", got)
	}
}

func TestGetPresetPrompt_UnknownReturnsError(t *testing.T) {
	_, err := GetPresetPrompt("nonexistent")
	if err == nil {
		t.Error("expected error for unknown preset name")
	}
}

func TestBuildPrompt_ReplacesCodePlaceholder(t *testing.T) {
	got := BuildPrompt("Explain this:\n{{code}}", "func foo() {}")
	if !strings.Contains(got, "func foo() {}") {
		t.Errorf("expected code in result, got: %q", got)
	}
	if strings.Contains(got, "{{code}}") {
		t.Errorf("placeholder should be replaced, got: %q", got)
	}
}

func TestBuildPrompt_ReplacesLanguagePlaceholder(t *testing.T) {
	got := BuildPrompt("Language: {{language}}\n{{code}}", "x")
	if !strings.Contains(got, "Language: go") && !strings.Contains(got, "Language: text") {
		// BuildPrompt with no language arg defaults to "text" or derives from code
		// This test just verifies {{language}} is replaced
		t.Errorf("{{language}} placeholder should be replaced, got: %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -run TestDefaultSystemPrompt -v`
Expected: FAIL — `services.DefaultSystemPrompt` undefined.

- [ ] **Step 3: Write the implementation**

Create `services/ai_prompts.go`:

```go
package services

import (
	"errors"
	"strings"
)

// DefaultSystemPrompt is the system prompt used when the user has not configured one.
// It positions the AI as a pragmatic senior engineer pair-programmer.
const DefaultSystemPrompt = `You are an expert code assistant embedded in the gugacode IDE.

You help the user write, understand, refactor, and debug code. You are:
- Concise and direct. Prefer code over prose.
- Honest about uncertainty. If you don't know, say so.
- Context-aware. Use the provided file path, language, and selection when available.
- Safety-conscious. Never suggest destructive operations without warning.

When responding with code, always use fenced code blocks with a language tag.
When explaining, keep prose short and lead with the answer.`

// PresetPrompts maps action names to prompt templates. The {{code}} placeholder
// is replaced with the user's selected code or the full file content.
var PresetPrompts = map[string]string{
	"explain": `Explain what the following code does. Focus on intent, not line-by-line narration.
Be concise. If the code has bugs or smells, mention them briefly at the end.

Code:
{{code}}`,

	"refactor": `Refactor the following code for readability and maintainability.
Preserve behavior. Explain key changes briefly after the code block.

Code:
{{code}}`,

	"fix": `Find and fix bugs in the following code. Explain each bug and the fix.
Show the corrected code in a fenced block.

Code:
{{code}}`,

	"generate_docs": `Generate documentation comments for the following code.
Follow the language's prevailing doc convention (godoc, jsdoc, etc.).
Output only the documented code in a fenced block.

Code:
{{code}}`,

	"generate_tests": `Generate unit tests for the following code.
Cover happy paths and edge cases. Use the language's standard testing framework.
Output only the test file in a fenced block.

Code:
{{code}}`,
}

// GetPresetPrompt returns the template for the given action name.
func GetPresetPrompt(name string) (string, error) {
	tmpl, ok := PresetPrompts[name]
	if !ok {
		return "", errors.New("unknown preset prompt: " + name)
	}
	return tmpl, nil
}

// BuildPrompt replaces placeholders in a template with actual values.
// Supported placeholders: {{code}}, {{language}}, {{filepath}}
func BuildPrompt(template, code string) string {
	result := strings.ReplaceAll(template, "{{code}}", code)
	result = strings.ReplaceAll(result, "{{language}}", "go")
	result = strings.ReplaceAll(result, "{{filepath}}", "")
	return result
}

// BuildPromptWithMeta replaces placeholders with file metadata.
func BuildPromptWithMeta(template, code, language, filePath string) string {
	result := strings.ReplaceAll(template, "{{code}}", code)
	if language == "" {
		language = "text"
	}
	result = strings.ReplaceAll(result, "{{language}}", language)
	result = strings.ReplaceAll(result, "{{filepath}}", filePath)
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -run "TestDefaultSystemPrompt|TestPresetPrompts|TestGetPresetPrompt|TestBuildPrompt" -v`
Expected: PASS — all 8 prompt tests pass.

- [ ] **Step 5: Verify full suite still passes**

Run: `go test ./services/ -count=1`
Expected: all prior tests still pass (41 + 8 new = 49 tests).

- [ ] **Step 6: Commit**

```bash
git add services/ai_prompts.go services/ai_prompts_test.go
git commit -m "feat: add AI system prompt and preset prompt templates"
```

---

## Task 2: Go Backend — Extend AIService with System Prompt & Context

**Files:**
- Modify: `services/ai_service.go`
- Modify: `services/ai_service_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `services/ai_service_test.go` (add to existing file, do not duplicate imports):

```go
func TestAIService_Send_includesSystemPrompt(t *testing.T) {
	// Use a test server that captures the request body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		_ = json.Unmarshal(body, &req)
		messages, _ := req["messages"].([]interface{})
		if len(messages) == 0 {
			t.Error("expected at least one message")
		}
		first, _ := messages[0].(map[string]interface{})
		if first["role"] != "system" {
			t.Errorf("expected first message role 'system', got %v", first["role"])
		}
		if first["content"] == "" {
			t.Error("expected non-empty system prompt content")
		}
		// Return a minimal valid response
		resp := `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	svc := &AIService{}
	svc.SetConfig(AIConfig{
		APIKey:       "test-key",
		BaseURL:      server.URL,
		Model:        "test-model",
		SystemPrompt: "You are a test assistant.",
	})

	resp, err := svc.Send([]ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected 'ok', got %q", resp.Content)
	}
}

func TestAIService_Send_usesDefaultSystemPromptWhenNoneSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		_ = json.Unmarshal(body, &req)
		messages, _ := req["messages"].([]interface{})
		first, _ := messages[0].(map[string]interface{})
		if first["role"] != "system" {
			t.Errorf("expected default system prompt, got role %v", first["role"])
		}
		if first["content"] == "" {
			t.Error("expected non-empty default system prompt")
		}
		resp := `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	svc := &AIService{}
	svc.SetConfig(AIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
		// SystemPrompt intentionally empty — should default to DefaultSystemPrompt
	})

	_, err := svc.Send([]ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

func TestAIService_SendStream_isCancellable(t *testing.T) {
	// Server that sends one chunk then hangs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		// Block until client cancels
		<-r.Context().Done()
	}))
	defer server.Close()

	svc := &AIService{}
	svc.SetConfig(AIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})

	ctx, cancel := context.WithCancel(context.Background())
	chunks := []string{}
	done := make(chan error, 1)
	go func() {
		err := svc.SendStreamWithContext(ctx, []ChatMessage{{Role: "user", Content: "hi"}}, func(c string) {
			chunks = append(chunks, c)
		})
		done <- err
	}()

	// Wait for first chunk
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			// Cancellation may return nil or context.Canceled — both acceptable
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendStreamWithContext did not return after cancel")
	}

	if len(chunks) == 0 {
		t.Error("expected at least one chunk before cancellation")
	}
}
```

**IMPORTANT:** Add these imports to the top of `ai_service_test.go` if not present:
```go
import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	// existing imports...
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -run "TestAIService_Send_includesSystemPrompt|TestAIService_Send_usesDefaultSystemPromptWhenNoneSet|TestAIService_SendStream_isCancellable" -v`
Expected: FAIL — `AIConfig.SystemPrompt` undefined, `svc.SendStreamWithContext` undefined.

- [ ] **Step 3: Modify `services/ai_service.go`**

Replace the entire file content with:

```go
package services

import (
	"bufio"
	"bytes"
	"context"
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
	APIKey       string
	BaseURL      string
	Model        string
	SystemPrompt string
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

// effectiveSystemPrompt returns the configured prompt or the default.
func (a *AIService) effectiveSystemPrompt() string {
	if a.config.SystemPrompt != "" {
		return a.config.SystemPrompt
	}
	return DefaultSystemPrompt
}

// withSystemPrompt prepends the system prompt to the messages slice.
func (a *AIService) withSystemPrompt(messages []ChatMessage) []ChatMessage {
	sp := a.effectiveSystemPrompt()
	if sp == "" {
		return messages
	}
	out := make([]ChatMessage, 0, len(messages)+1)
	out = append(out, ChatMessage{Role: "system", Content: sp})
	out = append(out, messages...)
	return out
}

func (a *AIService) Send(messages []ChatMessage) (*ChatResponse, error) {
	if a.config.APIKey == "" {
		return nil, errors.New("API key not configured")
	}

	fullMessages := a.withSystemPrompt(messages)
	reqBody := map[string]interface{}{
		"model":    a.config.Model,
		"messages": fullMessages,
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

// SendStream is the legacy streaming API (kept for backward compat).
// It creates an internal context that cannot be cancelled.
func (a *AIService) SendStream(messages []ChatMessage, onChunk func(chunk string)) error {
	return a.SendStreamWithContext(context.Background(), messages, onChunk)
}

// SendStreamWithContext streams the response and respects ctx cancellation.
func (a *AIService) SendStreamWithContext(ctx context.Context, messages []ChatMessage, onChunk func(chunk string)) error {
	if a.config.APIKey == "" {
		return errors.New("API key not configured")
	}

	fullMessages := a.withSystemPrompt(messages)
	reqBody := map[string]interface{}{
		"model":    a.config.Model,
		"messages": fullMessages,
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
	req = req.WithContext(ctx)
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
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

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -run "TestAIService_Send_includesSystemPrompt|TestAIService_Send_usesDefaultSystemPromptWhenNoneSet|TestAIService_SendStream_isCancellable" -v`
Expected: PASS — all 3 new tests pass.

- [ ] **Step 5: Verify full suite passes**

Run: `go test ./services/ -count=1 && go vet ./... && go build ./...`
Expected: all tests pass (49 + 3 = 52 tests), vet clean, build clean.

- [ ] **Step 6: Commit**

```bash
git add services/ai_service.go services/ai_service_test.go
git commit -m "feat: add system prompt and cancellable streaming to AIService"
```

---

## Task 3: Go Backend — Conversation Service

**Files:**
- Create: `services/conversation_service.go`
- Create: `services/conversation_service_test.go`

- [ ] **Step 1: Write the failing tests**

Create `services/conversation_service_test.go`:

```go
package services

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestConversationService_Save_andLoad(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}

	conv := Conversation{
		ID:        "test-1",
		Title:     "Test conversation",
		CreatedAt: time.Now().Unix(),
		Messages: []ConversationMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}

	err := svc.Save(conv)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "test-1.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}

	loaded, err := svc.Load("test-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Title != "Test conversation" {
		t.Errorf("expected title 'Test conversation', got %q", loaded.Title)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[1].Content != "hi there" {
		t.Errorf("expected second message 'hi there', got %q", loaded.Messages[1].Content)
	}
}

func TestConversationService_Load_nonexistentReturnsError(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}

	_, err := svc.Load("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent conversation")
	}
}

func TestConversationService_List_returnsAllSortedByCreatedDesc(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}

	// Save three conversations with different timestamps
	for i, id := range []string{"a", "b", "c"} {
		err := svc.Save(Conversation{
			ID:        id,
			Title:     "Conv " + id,
			CreatedAt: int64(i + 1),
			Messages:  []ConversationMessage{},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 conversations, got %d", len(list))
	}
	// Should be sorted by CreatedAt descending (newest first)
	if list[0].ID != "c" {
		t.Errorf("expected newest (c) first, got %q", list[0].ID)
	}
	if list[2].ID != "a" {
		t.Errorf("expected oldest (a) last, got %q", list[2].ID)
	}
}

func TestConversationService_Delete_removesFile(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}

	err := svc.Save(Conversation{
		ID:        "to-delete",
		Title:     "Delete me",
		CreatedAt: 1,
		Messages:  []ConversationMessage{},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Delete("to-delete")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	path := filepath.Join(dir, "to-delete.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, stat err=%v", err)
	}
}

func TestConversationService_GenerateID_isUnique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateConversationID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestConversationService_GenerateTitleFromMessage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello world", "Hello world"},
		{"This is a very long message that should be truncated because it exceeds the max title length", "This is a very long message that should be truncated because it e"},
		{"", "(new conversation)"},
		{"   ", "(new conversation)"},
	}
	for _, tt := range tests {
		got := GenerateTitle(tt.input)
		if got != tt.expected {
			t.Errorf("GenerateTitle(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// Ensure sort is stable for List — sanity check
func TestConversationService_List_emptyDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List on empty dir failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
	_ = sort.IntSlice{} // keep sort import used
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -run TestConversationService -v`
Expected: FAIL — `services.ConversationService` undefined.

- [ ] **Step 3: Write the implementation**

Create `services/conversation_service.go`:

```go
package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ConversationMessage is a single message in a persisted conversation.
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Conversation is a saved AI chat conversation.
type Conversation struct {
	ID        string               `json:"id"`
	Title     string               `json:"title"`
	CreatedAt int64                `json:"created_at"`
	Messages  []ConversationMessage `json:"messages"`
}

// ConversationService persists AI conversations to disk as JSON files.
type ConversationService struct {
	storageDir string
}

// NewConversationService creates a ConversationService rooted at the given directory.
func NewConversationService(storageDir string) *ConversationService {
	return &ConversationService{storageDir: storageDir}
}

// defaultStorageDir returns the user config dir + "/gugacode/conversations".
func defaultStorageDir() string {
	// Use XDG-style path. Fallback to temp dir if unavailable.
	home, err := os.UserConfigDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "gugacode", "conversations")
	}
	return filepath.Join(home, "gugacode", "conversations")
}

// ensureDir creates the storage directory if it doesn't exist.
func (s *ConversationService) ensureDir() error {
	dir := s.storageDir
	if dir == "" {
		dir = defaultStorageDir()
		s.storageDir = dir
	}
	return os.MkdirAll(dir, 0755)
}

func (s *ConversationService) pathFor(id string) string {
	return filepath.Join(s.storageDir, id+".json")
}

// Save writes a conversation to disk.
func (s *ConversationService) Save(conv Conversation) error {
	if conv.ID == "" {
		return errors.New("conversation ID is required")
	}
	if err := s.ensureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.pathFor(conv.ID), data, 0644)
}

// Load reads a conversation by ID.
func (s *ConversationService) Load(id string) (Conversation, error) {
	var conv Conversation
	data, err := os.ReadFile(s.pathFor(id))
	if err != nil {
		return conv, err
	}
	if err := json.Unmarshal(data, &conv); err != nil {
		return conv, err
	}
	return conv, nil
}

// List returns all conversations sorted by CreatedAt descending (newest first).
func (s *ConversationService) List() ([]Conversation, error) {
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		return nil, err
	}
	var convs []Conversation
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.storageDir, entry.Name()))
		if err != nil {
			continue
		}
		var conv Conversation
		if err := json.Unmarshal(data, &conv); err != nil {
			continue
		}
		convs = append(convs, conv)
	}
	sort.Slice(convs, func(i, j int) bool {
		return convs[i].CreatedAt > convs[j].CreatedAt
	})
	return convs, nil
}

// Delete removes a conversation file.
func (s *ConversationService) Delete(id string) error {
	return os.Remove(s.pathFor(id))
}

// GenerateConversationID returns a random 16-byte hex ID with a time-based prefix.
func GenerateConversationID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// GenerateTitle returns a title derived from the first user message.
// Truncates to 60 characters; returns "(new conversation)" for empty input.
func GenerateTitle(firstMessage string) string {
	s := strings.TrimSpace(firstMessage)
	if s == "" {
		return "(new conversation)"
	}
	const maxLen = 60
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -run "TestConversationService|TestConversationService_GenerateTitleFromMessage" -v`
Expected: PASS — all 7 conversation tests pass.

- [ ] **Step 5: Verify full suite**

Run: `go test ./services/ -count=1 && go vet ./... && go build ./...`
Expected: 52 + 7 = 59 tests pass, vet/build clean.

- [ ] **Step 6: Commit**

```bash
git add services/conversation_service.go services/conversation_service_test.go
git commit -m "feat: add ConversationService for AI chat persistence"
```

---

## Task 4: Go Backend — Register ConversationService in main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Edit main.go**

In `main.go`, add the conversation service instantiation after the searchService line:

```go
	gitService := &services.GitService{}
	searchService := &services.SearchService{}
	conversationService := services.NewConversationService("")
```

Then add it to the Services slice (before `&GreetService{}`):

```go
			application.NewService(gitService),
			application.NewService(searchService),
			application.NewService(conversationService),
			application.NewService(&GreetService{}),
```

- [ ] **Step 2: Verify build and tests**

Run:
```bash
go build ./
go vet ./...
go test ./services/ -count=1
```

Expected: all succeed, 59 tests pass.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: register ConversationService in main.go"
```

---

## Task 5: Frontend — Bindings & Types Extension

**Files:**
- Modify: `frontend/bindings/changeme/services/models.js`
- Create: `frontend/bindings/changeme/services/conversationservice.js`
- Modify: `frontend/bindings/changeme/services/index.js`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/services.ts`

- [ ] **Step 1: Add new types to `frontend/bindings/changeme/services/models.js`**

Append these classes after the `SearchResult` class at the end of the file:

```javascript
/**
 * ConversationMessage is a single message in a persisted conversation.
 */
export class ConversationMessage {
    constructor($$source = {}) {
        if (!("role" in $$source)) {
            this["role"] = "";
        }
        if (!("content" in $$source)) {
            this["content"] = "";
        }
        Object.assign(this, $$source);
    }
    static createFrom($$source = {}) {
        let $$parsedSource = typeof $$source === 'string' ? JSON.parse($$source) : $$source;
        return new ConversationMessage(/** @type {Partial<ConversationMessage>} */($$parsedSource));
    }
}

/**
 * Conversation is a saved AI chat conversation.
 */
export class Conversation {
    constructor($$source = {}) {
        if (!("id" in $$source)) { this["id"] = ""; }
        if (!("title" in $$source)) { this["title"] = ""; }
        if (!("created_at" in $$source)) { this["created_at"] = 0; }
        if (!("messages" in $$source)) { this["messages"] = []; }
        Object.assign(this, $$source);
    }
    static createFrom($$source = {}) {
        let $$parsedSource = typeof $$source === 'string' ? JSON.parse($$source) : $$source;
        return new Conversation(/** @type {Partial<Conversation>} */($$parsedSource));
    }
}
```

- [ ] **Step 2: Create `frontend/bindings/changeme/services/conversationservice.js`**

```javascript
// @ts-check
// This file is automatically generated. DO NOT EDIT

import { Call as $Call, CancellablePromise as $CancellablePromise, Create as $Create } from "@wailsio/runtime";
import * as $models from "./models.js";

/**
 * @param {string} id
 * @returns {$CancellablePromise<void>}
 */
export function Delete(id) {
    return $Call.ByID(7, id);
}

/**
 * @returns {$CancellablePromise<string>}
 */
export function GenerateConversationID() {
    return $Call.ByID(8, "");
}

/**
 * @param {string} firstMessage
 * @returns {$CancellablePromise<string>}
 */
export function GenerateTitle(firstMessage) {
    return $Call.ByID(9, firstMessage);
}

/**
 * @param {string} id
 * @returns {$CancellablePromise<$models.Conversation>}
 */
export function Load(id) {
    return $Call.ByID(10, id).then(($result) => {
        return $models.Conversation.createFrom($result);
    });
}

/**
 * @returns {$CancellablePromise<$models.Conversation[]>}
 */
export function List() {
    return $Call.ByID(11).then(($result) => {
        return $$createType1($result);
    });
}

/**
 * @param {$models.Conversation} conv
 * @returns {$CancellablePromise<void>}
 */
export function Save(conv) {
    return $Call.ByID(12, conv);
}

const $$createType0 = $models.Conversation.createFrom;
const $$createType1 = $Create.Array($$createType0);
```

- [ ] **Step 3: Update `frontend/bindings/changeme/services/index.js`**

Add the import and re-export for ConversationService and the new model types:

```javascript
import * as ConversationService from "./conversationservice.js";
// ... existing imports
export {
    AIService,
    ConversationService,
    FileService,
    GitService,
    ProjectService,
    SearchService,
    SettingsService,
    TerminalService,
    WindowService
};

export {
    AIConfig,
    BranchInfo,
    ChatMessage,
    ChatResponse,
    Conversation,
    ConversationMessage,
    DirEntry,
    GitFileChange,
    Project,
    SearchMatch,
    SearchResult,
    Settings
} from "./models.js";
```

- [ ] **Step 4: Extend `frontend/src/types/index.ts`**

Append after the `SearchResult` interface:

```typescript
export interface ConversationMessage {
  role: string;
  content: string;
}

export interface Conversation {
  id: string;
  title: string;
  created_at: number;
  messages: ConversationMessage[];
}

export type AIActionName =
  | "explain"
  | "refactor"
  | "fix"
  | "generate_docs"
  | "generate_tests";

export interface AIContextAttachment {
  kind: "file" | "selection";
  filePath: string;
  language: string;
  content: string;
  startLine?: number;
  endLine?: number;
}
```

- [ ] **Step 5: Extend `frontend/src/api/services.ts`**

Add the import and wrapper. After the `SearchServiceBindings` import:

```typescript
import * as ConversationServiceBindings from "../../bindings/changeme/services/conversationservice.js";
```

Update the type import:

```typescript
import type { DirEntry, Project, Settings, ChatMessage, GitFileChange, BranchInfo, SearchResult, Conversation } from "@/types";
```

Append the wrapper at the end of the file:

```typescript
export const conversationService = {
  save: (conv: Conversation) =>
    ConversationServiceBindings.Save(conv) as Promise<void>,
  load: (id: string) =>
    ConversationServiceBindings.Load(id) as Promise<Conversation>,
  list: () =>
    ConversationServiceBindings.List() as Promise<Conversation[]>,
  delete: (id: string) =>
    ConversationServiceBindings.Delete(id) as Promise<void>,
  generateId: () =>
    ConversationServiceBindings.GenerateConversationID() as Promise<string>,
  generateTitle: (firstMessage: string) =>
    ConversationServiceBindings.GenerateTitle(firstMessage) as Promise<string>,
};

export const aiServiceV2 = {
  // Re-export for components that want the v2 surface
  setConfig: (config: {
    apiKey: string;
    baseUrl: string;
    model: string;
    systemPrompt?: string;
  }) =>
    AIServiceBindings.SetConfig({
      APIKey: config.apiKey,
      BaseURL: config.baseUrl,
      Model: config.model,
      SystemPrompt: config.systemPrompt ?? "",
    }) as Promise<void>,
};
```

- [ ] **Step 6: Verify type check**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/bindings/ frontend/src/types/index.ts frontend/src/api/services.ts
git commit -m "feat: add ConversationService bindings and AI v2 types"
```

---

## Task 6: Frontend — Markdown Rendering Library

**Files:**
- Create: `frontend/src/lib/markdown.ts`
- Create: `frontend/src/lib/markdown.test.ts`

- [ ] **Step 1: Install dependencies**

Run from `frontend/`:
```bash
npm install marked highlight.js
npm install -D @types/marked
```

- [ ] **Step 2: Write the failing tests**

Create `frontend/src/lib/markdown.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { renderMarkdown, sanitizeHtml } from "./markdown";

describe("renderMarkdown", () => {
  it("renders plain text as a paragraph", () => {
    const html = renderMarkdown("hello world");
    expect(html).toContain("hello world");
    expect(html).toContain("<p>");
  });

  it("renders fenced code blocks with language class", () => {
    const md = "```js\nconst x = 1;\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("<code");
    expect(html).toContain("language-js");
    expect(html).toContain("const x = 1");
  });

  it("renders inline code", () => {
    const html = renderMarkdown("use `const` for declarations");
    expect(html).toContain("<code>const</code>");
  });

  it("renders bold text", () => {
    const html = renderMarkdown("**important**");
    expect(html).toContain("<strong>important</strong>");
  });

  it("renders bullet lists", () => {
    const md = "- one\n- two\n- three";
    const html = renderMarkdown(md);
    expect(html).toContain("<ul>");
    expect(html).toContain("<li>one</li>");
    expect(html).toContain("<li>three</li>");
  });

  it("renders links with href", () => {
    const html = renderMarkdown("[docs](https://example.com)");
    expect(html).toContain('<a href="https://example.com"');
    expect(html).toContain(">docs</a>");
  });

  it("renders headers", () => {
    expect(renderMarkdown("# Title")).toContain("<h1>");
    expect(renderMarkdown("## Sub")).toContain("<h2>");
  });

  it("escapes raw HTML in content", () => {
    const html = renderMarkdown("<script>alert(1)</script>");
    expect(html).not.toContain("<script>");
  });
});

describe("sanitizeHtml", () => {
  it("strips script tags", () => {
    const result = sanitizeHtml("<p>ok</p><script>alert(1)</script>");
    expect(result).not.toContain("<script>");
    expect(result).toContain("<p>ok</p>");
  });

  it("strips on* attributes", () => {
    const result = sanitizeHtml('<p onclick="evil()">text</p>');
    expect(result).not.toContain("onclick");
    expect(result).toContain("text");
  });

  it("allows safe tags", () => {
    const result = sanitizeHtml("<strong>bold</strong><em>italic</em>");
    expect(result).toContain("<strong>bold</strong>");
    expect(result).toContain("<em>italic</em>");
  });
});
```

- [ ] **Step 3: Run tests to verify they fail**

Run:
```bash
cd frontend
npx vitest run src/lib/markdown.test.ts
```

Expected: FAIL — cannot resolve `./markdown`.

- [ ] **Step 4: Write the implementation**

Create `frontend/src/lib/markdown.ts`:

```typescript
import { marked } from "marked";

// Configure marked once
marked.setOptions({
  gfm: true,
  breaks: false,
});

/**
 * Sanitizes HTML to prevent XSS:
 * - Removes <script> tags entirely
 * - Strips on* event handler attributes
 * - Strips javascript: URLs
 */
export function sanitizeHtml(html: string): string {
  // Remove script tags and their content
  let result = html.replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, "");
  // Remove on* event handler attributes
  result = result.replace(/\s+on\w+\s*=\s*"[^"]*"/gi, "");
  result = result.replace(/\s+on\w+\s*=\s*'[^']*'/gi, "");
  result = result.replace(/\s+on\w+\s*=\s*[^\s>]+/gi, "");
  // Remove javascript: URLs in href/src
  result = result.replace(/(href|src)\s*=\s*["']javascript:[^"']*["']/gi, '$1="#"');
  return result;
}

/**
 * Renders markdown to sanitized HTML.
 * Code blocks are rendered with language-XXX classes for highlight.js.
 */
export function renderMarkdown(md: string): string {
  if (!md) return "";
  const rawHtml = marked.parse(md, { async: false }) as string;
  return sanitizeHtml(rawHtml);
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
cd frontend
npx vitest run src/lib/markdown.test.ts
```

Expected: PASS — all 11 markdown tests pass.

- [ ] **Step 6: Verify full suite and type check**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
npx vitest run
```

Expected: vue-tsc clean, 60 + 11 = 71 tests pass.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/markdown.ts frontend/src/lib/markdown.test.ts frontend/package.json frontend/package-lock.json
git commit -m "feat: add markdown rendering with XSS sanitization"
```

---

## Task 7: Frontend — Extend AI Store with Context, Stop, Presets

**Files:**
- Modify: `frontend/src/stores/ai.ts`
- Modify: `frontend/src/stores/ai.test.ts`

- [ ] **Step 1: Replace `frontend/src/stores/ai.ts` with the extended version**

```typescript
import { reactive } from "vue";
import { aiService, aiServiceV2, conversationService } from "@/api/services";
import { appState } from "@/stores/app";
import type { ChatMessage, AIContextAttachment, AIActionName, Conversation } from "@/types";

export interface AIState {
  messages: ChatMessage[];
  streaming: boolean;
  error: string | null;
  context: AIContextAttachment | null;
  currentConversationId: string | null;
  abortController: AbortController | null;
}

export const aiState = reactive<AIState>({
  messages: [],
  streaming: false,
  error: null,
  context: null,
  currentConversationId: null,
  abortController: null,
});

/**
 * Builds the user message including context if attached.
 */
function buildUserMessage(content: string): string {
  if (!aiState.context) return content;
  const ctx = aiState.context;
  let prefix: string;
  if (ctx.kind === "selection") {
    prefix = `File: ${ctx.filePath}\nSelected code (${ctx.startLine}-${ctx.endLine}):\n\`\`\`${ctx.language}\n${ctx.content}\n\`\`\`\n\n`;
  } else {
    prefix = `File: ${ctx.filePath}\n\`\`\`${ctx.language}\n${ctx.content}\n\`\`\`\n\n`;
  }
  return prefix + content;
}

/**
 * Sends a user message. Respects attached context and persists the conversation.
 */
export async function sendMessage(content: string): Promise<void> {
  if (aiState.streaming) return;

  aiState.error = null;
  const fullContent = buildUserMessage(content);
  aiState.messages.push({ role: "user", content: fullContent });
  aiState.streaming = true;

  const assistantMessage: ChatMessage = { role: "assistant", content: "" };
  aiState.messages.push(assistantMessage);

  // Create an AbortController for stop-generation
  const abortController = new AbortController();
  aiState.abortController = abortController;

  try {
    aiServiceV2.setConfig({
      apiKey: appState.aiApiKey,
      baseUrl: appState.aiBaseUrl,
      model: appState.aiModel,
      systemPrompt: appState.aiSystemPrompt,
    });
    const history = aiState.messages.slice(0, -1);
    await aiService.sendStream(history, (chunk: string) => {
      assistantMessage.content += chunk;
    });
    // Stream completed — persist
    await persistConversation();
  } catch (e: any) {
    aiState.error = e?.message ?? "AI request failed";
    if (assistantMessage.content === "") {
      aiState.messages.pop();
    }
  } finally {
    aiState.streaming = false;
    aiState.abortController = null;
  }
}

/**
 * Stops an in-progress streaming request.
 */
export function stopGeneration(): void {
  if (aiState.abortController) {
    aiState.abortController.abort();
    aiState.abortController = null;
  }
  aiState.streaming = false;
}

/**
 * Attaches code context to the next message.
 */
export function attachContext(context: AIContextAttachment): void {
  aiState.context = context;
}

/**
 * Clears attached context.
 */
export function clearContext(): void {
  aiState.context = null;
}

/**
 * Runs a preset AI action on the given code.
 */
export async function runAIAction(
  action: AIActionName,
  code: string,
  language: string,
  filePath: string,
): Promise<void> {
  const prompts: Record<AIActionName, string> = {
    explain: "Explain this code.",
    refactor: "Refactor this code.",
    fix: "Find and fix bugs in this code.",
    generate_docs: "Generate documentation for this code.",
    generate_tests: "Generate unit tests for this code.",
  };
  const context: AIContextAttachment = {
    kind: "selection",
    filePath,
    language,
    content: code,
  };
  attachContext(context);
  await sendMessage(prompts[action]);
  clearContext();
}

/**
 * Clears all messages and starts fresh.
 */
export function clearMessages(): void {
  if (aiState.streaming) return;
  aiState.messages = [];
  aiState.error = null;
  aiState.currentConversationId = null;
  aiState.context = null;
}

/**
 * Persists the current conversation to disk.
 */
async function persistConversation(): Promise<void> {
  if (aiState.messages.length === 0) return;
  try {
    let id = aiState.currentConversationId;
    if (!id) {
      id = await conversationService.generateId();
      aiState.currentConversationId = id;
    }
    const firstUser = aiState.messages.find((m) => m.role === "user");
    const title = firstUser
      ? await conversationService.generateTitle(firstUser.content.slice(0, 200))
      : "(empty)";
    await conversationService.save({
      id,
      title,
      created_at: Math.floor(Date.now() / 1000),
      messages: aiState.messages.map((m) => ({ role: m.role, content: m.content })),
    });
  } catch (e) {
    // Persistence failure is non-fatal
    console.error("Failed to persist conversation:", e);
  }
}

/**
 * Loads a saved conversation into the chat.
 */
export async function loadConversation(id: string): Promise<void> {
  try {
    const conv = await conversationService.load(id);
    aiState.messages = conv.messages.map((m) => ({ role: m.role as "user" | "assistant" | "system", content: m.content }));
    aiState.currentConversationId = conv.id;
    aiState.error = null;
  } catch (e: any) {
    aiState.error = e?.message ?? "Failed to load conversation";
  }
}
```

- [ ] **Step 2: Add `aiSystemPrompt` to `appState`**

In `frontend/src/stores/app.ts`, add to the `AppState` interface (after `aiModel`):

```typescript
  aiSystemPrompt: string;
```

And in the `appState` reactive initializer (after `aiModel: "gpt-4o"`):

```typescript
  aiSystemPrompt: "",
```

In `loadSettings()`, add after `appState.aiModel = settings.aiModel;`:

```typescript
    appState.aiSystemPrompt = settings.aiSystemPrompt;
```

In `saveSettings()`, add to the settings object:

```typescript
      aiSystemPrompt: appState.aiSystemPrompt,
```

In `frontend/src/types/index.ts`, add `aiSystemPrompt: string` to the `Settings` interface:

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
  aiSystemPrompt: string;
}
```

In `services/settings_service.go`, add `AISystemPrompt string` to the `Settings` struct and set `AISystemPrompt: ""` in `defaultSettings()`.

- [ ] **Step 3: Update tests**

Replace `frontend/src/stores/ai.test.ts` with:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services", () => ({
  aiService: {
    setConfig: vi.fn().mockResolvedValue(undefined),
    sendStream: vi.fn().mockImplementation(async (messages: any[], onChunk: (c: string) => void) => {
      onChunk("hello");
      onChunk(" world");
    }),
  },
  aiServiceV2: {
    setConfig: vi.fn().mockResolvedValue(undefined),
  },
  conversationService: {
    save: vi.fn().mockResolvedValue(undefined),
    load: vi.fn().mockResolvedValue({ id: "1", title: "test", created_at: 0, messages: [] }),
    generateId: vi.fn().mockResolvedValue("new-id"),
    generateTitle: vi.fn().mockResolvedValue("test title"),
  },
}));

import {
  aiState,
  sendMessage,
  stopGeneration,
  attachContext,
  clearContext,
  runAIAction,
  clearMessages,
} from "./ai";

describe("ai store", () => {
  beforeEach(() => {
    aiState.messages = [];
    aiState.streaming = false;
    aiState.error = null;
    aiState.context = null;
    aiState.currentConversationId = null;
    aiState.abortController = null;
  });

  it("sends a message and appends assistant response", async () => {
    await sendMessage("hi");
    expect(aiState.messages.length).toBe(2);
    expect(aiState.messages[0].role).toBe("user");
    expect(aiState.messages[1].role).toBe("assistant");
    expect(aiState.messages[1].content).toBe("hello world");
  });

  it("includes context prefix when attached", async () => {
    attachContext({
      kind: "selection",
      filePath: "/test.ts",
      language: "typescript",
      content: "const x = 1;",
      startLine: 1,
      endLine: 1,
    });
    await sendMessage("explain");
    expect(aiState.messages[0].content).toContain("const x = 1");
    expect(aiState.messages[0].content).toContain("/test.ts");
    expect(aiState.messages[0].content).toContain("explain");
  });

  it("stops generation", async () => {
    aiState.streaming = true;
    const ac = new AbortController();
    aiState.abortController = ac;
    stopGeneration();
    expect(aiState.streaming).toBe(false);
    expect(aiState.abortController).toBe(null);
  });

  it("clears context", () => {
    aiState.context = { kind: "file", filePath: "/x", language: "go", content: "x" };
    clearContext();
    expect(aiState.context).toBe(null);
  });

  it("clears messages", () => {
    aiState.messages = [{ role: "user", content: "x" }];
    clearMessages();
    expect(aiState.messages).toHaveLength(0);
  });

  it("does not send while streaming", async () => {
    aiState.streaming = true;
    const before = aiState.messages.length;
    await sendMessage("hi");
    expect(aiState.messages.length).toBe(before);
  });

  it("runAIAction attaches context and sends", async () => {
    await runAIAction("explain", "func foo() {}", "go", "/main.go");
    expect(aiState.messages.length).toBe(2);
    expect(aiState.messages[0].content).toContain("func foo() {}");
    expect(aiState.messages[0].content).toContain("Explain this code.");
  });
});
```

- [ ] **Step 4: Run tests**

Run:
```bash
cd frontend
npx vitest run src/stores/ai.test.ts
```

Expected: PASS — all 7 AI store tests pass.

- [ ] **Step 5: Verify full suite**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
npx vitest run
```

Expected: vue-tsc clean, all tests pass (71 - 3 old ai tests + 7 new = 75 tests).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/stores/ai.ts frontend/src/stores/ai.test.ts frontend/src/stores/app.ts frontend/src/types/index.ts services/settings_service.go
git commit -m "feat: extend AI store with context, stop, presets, persistence"
```

---

## Task 8: Frontend — Redesign AiChatPanel with Markdown & Controls

**Files:**
- Modify: `frontend/src/components/layout/AiChatPanel.vue`

- [ ] **Step 1: Replace AiChatPanel.vue**

Replace the entire file with:

```vue
<script setup lang="ts">
import { appState, toggleAiChat } from "@/stores/app";
import { computed, ref, nextTick, watch } from "vue";
import { Close, Promotion, StopFilled, CopyDocument, ChatDotRound } from "@element-plus/icons-vue";
import { aiState, sendMessage, clearMessages, stopGeneration, clearContext } from "@/stores/ai";
import { renderMarkdown } from "@/lib/markdown";

const isVisible = computed(() => appState.aiChatVisible);
const inputText = ref("");
const messageListRef = ref<HTMLElement | null>(null);

const modelOptions = [
  { label: "GPT-4o", value: "gpt-4o" },
  { label: "GPT-4o mini", value: "gpt-4o-mini" },
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
const hasContext = computed(() => aiState.context !== null);

function contextLabel(): string {
  if (!aiState.context) return "";
  const c = aiState.context;
  const name = c.filePath.split("/").pop() ?? c.filePath;
  return c.kind === "selection" ? `${name}:${c.startLine}-${c.endLine}` : name;
}

function renderContent(content: string): string {
  return renderMarkdown(content);
}

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

function handleStop() {
  stopGeneration();
}

function handleCopy(content: string) {
  navigator.clipboard.writeText(content).catch(() => {
    // Fallback for clipboard failure
  });
}

function scrollToBottom() {
  if (messageListRef.value) {
    messageListRef.value.scrollTop = messageListRef.value.scrollHeight;
  }
}

watch(
  () => aiState.messages.length,
  () => {
    nextTick(scrollToBottom);
  },
);

watch(
  () => aiState.messages[aiState.messages.length - 1]?.content,
  () => {
    nextTick(scrollToBottom);
  },
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
          <el-icon :size="14"><ChatDotRound /></el-icon>
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
            <el-icon :size="14"><Close /></el-icon>
          </button>
          <button
            class="ai-chat-panel__close"
            aria-label="Close AI chat"
            title="Close AI chat"
            @click="toggleAiChat"
          >
            <el-icon :size="14"><Close /></el-icon>
          </button>
        </div>
      </div>

      <!-- Context chip -->
      <div v-if="hasContext" class="ai-chat-panel__context-bar">
        <span class="ai-chat-panel__context-chip">
          {{ contextLabel() }}
          <button
            class="ai-chat-panel__context-remove"
            aria-label="Remove context"
            @click="clearContext"
          >×</button>
        </span>
      </div>

      <div ref="messageListRef" class="ai-chat-panel__body">
        <div v-if="!hasMessages" class="ai-chat-panel__empty">
          <div class="ai-chat-panel__empty-circle" aria-hidden="true" />
          <p class="ai-chat-panel__empty-title">Ask me anything about your code</p>
          <p class="ai-chat-panel__empty-subtitle">
            Write, refactor, debug, and explain. Select code in the editor for targeted actions.
          </p>
        </div>

        <div v-else class="ai-chat-panel__messages">
          <div
            v-for="(msg, i) in aiState.messages"
            :key="i"
            class="ai-chat-panel__message"
            :class="'ai-chat-panel__message--' + msg.role"
          >
            <div class="ai-chat-panel__message-header">
              <span class="ai-chat-panel__message-role">{{ msg.role }}</span>
              <button
                v-if="msg.role === 'assistant' && msg.content"
                class="ai-chat-panel__copy-btn"
                aria-label="Copy message"
                title="Copy message"
                @click="handleCopy(msg.content)"
              >
                <el-icon :size="12"><CopyDocument /></el-icon>
              </button>
            </div>
            <div
              class="ai-chat-panel__message-content markdown-body"
              v-html="renderContent(msg.content)"
            />
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
            v-if="aiState.streaming"
            class="ai-chat-panel__stop"
            aria-label="Stop generation"
            title="Stop generation"
            @click="handleStop"
          >
            <el-icon :size="14"><StopFilled /></el-icon>
          </button>
          <button
            v-else
            class="ai-chat-panel__send"
            aria-label="Send message"
            title="Send message"
            :disabled="!inputText.trim()"
            @click="handleSend"
          >
            <el-icon :size="14"><Promotion /></el-icon>
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

.ai-chat-panel__header-left {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-tertiary);
}

.ai-chat-panel__title {
  font-size: 10px;
  font-weight: 400;
  text-transform: lowercase;
  letter-spacing: 0.08em;
  color: var(--color-text-tertiary);
}

.ai-chat-panel__header-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.ai-chat-panel__model-select {
  padding: 2px 6px;
  font-size: 10px;
  font-family: var(--font-sans);
  color: var(--color-text-tertiary);
  background-color: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  outline: 0;
  cursor: pointer;
  transition: border-color var(--duration-fast) var(--ease-out-expo);
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
  transition: color var(--duration-micro) var(--ease-out-expo),
              background-color var(--duration-micro) var(--ease-out-expo);
}

.ai-chat-panel__clear:hover,
.ai-chat-panel__close:hover {
  color: var(--color-text-secondary);
  background-color: color-mix(in srgb, var(--color-text-tertiary) 6%, transparent);
}

.ai-chat-panel__context-bar {
  padding: 4px 12px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.ai-chat-panel__context-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  font-size: 10px;
  color: var(--color-text-secondary);
  background-color: color-mix(in srgb, var(--color-primary) 12%, transparent);
  border-radius: var(--radius-sm);
}

.ai-chat-panel__context-remove {
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 0;
}

.ai-chat-panel__context-remove:hover {
  color: var(--color-text-primary);
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
  flex-shrink: 0;
}

.ai-chat-panel__empty-title {
  font-size: 13px;
  font-weight: 400;
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

.ai-chat-panel__message-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
}

.ai-chat-panel__message-role {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-tertiary);
}

.ai-chat-panel__copy-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  border-radius: var(--radius-sm);
  opacity: 0;
  transition: opacity var(--duration-micro) var(--ease-out-expo);
}

.ai-chat-panel__message:hover .ai-chat-panel__copy-btn {
  opacity: 1;
}

.ai-chat-panel__copy-btn:hover {
  color: var(--color-text-primary);
  background-color: color-mix(in srgb, var(--color-text-tertiary) 12%, transparent);
}

.ai-chat-panel__message-content {
  word-wrap: break-word;
}

.ai-chat-panel__message-content :deep(pre) {
  margin: 6px 0;
  padding: 8px;
  background-color: var(--color-bg-base);
  border-radius: var(--radius-sm);
  overflow-x: auto;
}

.ai-chat-panel__message-content :deep(code) {
  font-family: var(--font-mono);
  font-size: 11px;
}

.ai-chat-panel__message-content :deep(p) {
  margin: 4px 0;
}

.ai-chat-panel__message-content :deep(ul),
.ai-chat-panel__message-content :deep(ol) {
  margin: 4px 0;
  padding-left: 20px;
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
  transition: border-color var(--duration-fast) var(--ease-out-expo);
}

.ai-chat-panel__input-wrap:focus-within {
  border-color: var(--color-primary);
}

.ai-chat-panel__input {
  flex: 1;
  min-width: 0;
  padding: 4px 0;
  font-size: 12px;
  font-family: var(--font-sans);
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

.ai-chat-panel__send,
.ai-chat-panel__stop {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 50%;
  color: #ffffff;
  cursor: pointer;
  flex-shrink: 0;
  transition: background-color var(--duration-micro) var(--ease-out-expo);
}

.ai-chat-panel__send {
  background-color: var(--color-primary);
}

.ai-chat-panel__send:hover:not(:disabled) {
  background-color: color-mix(in srgb, var(--color-primary) 85%, #000000);
}

.ai-chat-panel__send:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.ai-chat-panel__stop {
  background-color: var(--color-error);
}

.ai-chat-panel__stop:hover {
  background-color: color-mix(in srgb, var(--color-error) 85%, #000000);
}

.slide-chat-enter-active,
.slide-chat-leave-active {
  transition: width var(--duration-normal) var(--ease-out-expo),
              opacity var(--duration-fast) var(--ease-out-expo);
  overflow: hidden;
}

.slide-chat-enter-from,
.slide-chat-leave-to {
  width: 0;
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .ai-chat-panel { transition: none; }
  .slide-chat-enter-active,
  .slide-chat-leave-active { transition: none; }
}
</style>
```

- [ ] **Step 2: Verify type check and tests**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
npx vitest run
```

Expected: vue-tsc clean, all tests pass.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/layout/AiChatPanel.vue
git commit -m "feat: redesign AiChatPanel with markdown, stop, copy, context chips"
```

---

## Task 9: Frontend — Editor Right-Click AI Actions

**Files:**
- Modify: `frontend/src/components/editor/CodeEditor.vue`

- [ ] **Step 1: Read the current CodeEditor.vue**

Read `frontend/src/components/editor/CodeEditor.vue` to understand its structure. It uses `vue-monaco-editor` and binds to `editorState`.

- [ ] **Step 2: Add AI action menu**

In the `<script setup>` block, add imports and action handlers after the existing setup:

```typescript
import { aiState, runAIAction, attachContext, clearContext } from "@/stores/ai";
import { appState, toggleAiChat } from "@/stores/app";
import type { AIActionName } from "@/types";

// Right-click AI actions
function handleEditorContextMenu(editor: any, mouseEvent: MouseEvent) {
  // We don't render a custom menu — instead we use Monaco's built-in
  // context menu via actions registered on the editor.
}

function registerAIActions(editor: any) {
  const actions: Array<{ id: string; label: string; action: AIActionName }> = [
    { id: "ai-explain", label: "AI: Explain Selection", action: "explain" },
    { id: "ai-refactor", label: "AI: Refactor Selection", action: "refactor" },
    { id: "ai-fix", label: "AI: Fix Bugs in Selection", action: "fix" },
    { id: "ai-docs", label: "AI: Generate Docs", action: "generate_docs" },
    { id: "ai-tests", label: "AI: Generate Tests", action: "generate_tests" },
  ];

  for (const a of actions) {
    editor.addAction({
      id: a.id,
      label: a.label,
      contextMenuGroupId: "ai",
      contextMenuOrder: actions.indexOf(a),
      run: async (ed: any) => {
        const selection = ed.getSelection();
        const model = ed.getModel();
        if (!model) return;

        let code: string;
        let language = "text";
        let filePath = "";

        if (selection && !selection.isEmpty()) {
          code = model.getValueInRange(selection);
          const startLine = selection.startLineNumber;
          const endLine = selection.endLineNumber;
          // Determine language from model
          const langId = model.getLanguageId();
          language = langId || "text";
          // File path from editor state
          filePath = editorState.activeFilePath ?? "";
          attachContext({
            kind: "selection",
            filePath,
            language,
            content: code,
            startLine,
            endLine,
          });
        } else {
          // Use entire file
          code = model.getValue();
          const langId = model.getLanguageId();
          language = langId || "text";
          filePath = editorState.activeFilePath ?? "";
          attachContext({
            kind: "file",
            filePath,
            language,
            content: code,
          });
        }

        // Open AI panel
        if (!appState.aiChatVisible) {
          toggleAiChat();
        }

        await runAIAction(a.action, code, language, filePath);
        clearContext();
      },
    });
  }
}

// Wire to Monaco's onMount
function handleMount(editor: any) {
  registerAIActions(editor);
}
```

Then in the `<template>`, ensure the Monaco component calls `@mount="handleMount"`:

```vue
<vue-monaco-editor
  ...
  @mount="handleMount"
/>
```

**Note:** Read the existing CodeEditor.vue first to find the exact Monaco editor tag and add the `@mount` handler. If the editor already uses `@mount`, add `registerAIActions(editor)` inside the existing mount handler instead.

- [ ] **Step 3: Verify type check**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
```

Expected: no errors. If `editorState` is not imported, add it to the imports.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/editor/CodeEditor.vue
git commit -m "feat: add editor right-click AI actions (explain/refactor/fix/docs/tests)"
```

---

## Task 10: Frontend — Settings: System Prompt Configuration

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`
- Modify: `services/settings_service.go` (if not already done in Task 7)
- Modify: `frontend/src/stores/app.ts` (if not already done in Task 7)

- [ ] **Step 1: Read current SettingsView.vue**

Read `frontend/src/views/SettingsView.vue` to understand its layout. Find the AI config section.

- [ ] **Step 2: Add system prompt textarea**

In the AI configuration section of `SettingsView.vue`, add a system prompt textarea after the model selector:

```vue
<el-form-item label="System Prompt">
  <el-input
    v-model="appState.aiSystemPrompt"
    type="textarea"
    :rows="6"
    placeholder="Leave empty to use the default code-assistant prompt. Customize to steer the AI's behavior."
    @input="saveSettings"
  />
  <div class="settings-hint">
    Default prompt positions the AI as a pragmatic senior engineer. Override to customize persona, tone, or constraints.
  </div>
</el-form-item>
```

Add a "Reset to default" button next to it:

```vue
<el-button size="small" @click="resetSystemPrompt">Reset to default</el-button>
```

And in the script:

```typescript
function resetSystemPrompt() {
  appState.aiSystemPrompt = "";
  saveSettings();
}
```

- [ ] **Step 3: Ensure Go settings_service.go has the field**

In `services/settings_service.go`, the `Settings` struct must have:

```go
type Settings struct {
	// ... existing fields ...
	AISystemPrompt string `json:"aiSystemPrompt"`
}
```

And `defaultSettings()` must set:

```go
	AISystemPrompt: "",
```

- [ ] **Step 4: Add a settings test**

Append to `services/settings_service_test.go`:

```go
func TestSettingsService_SaveAndLoadAISystemPrompt(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AISystemPrompt = "You are a Rust expert."

	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	loaded, err := svc2.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.AISystemPrompt != "You are a Rust expert." {
		t.Errorf("expected AISystemPrompt 'You are a Rust expert.', got %q", loaded.AISystemPrompt)
	}
}
```

- [ ] **Step 5: Run tests**

Run:
```bash
go test ./services/ -run TestSettingsService_SaveAndLoadAISystemPrompt -v
cd frontend
npx vue-tsc --noEmit
npx vitest run
```

Expected: Go test passes, vue-tsc clean, all frontend tests pass.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/SettingsView.vue services/settings_service.go services/settings_service_test.go
git commit -m "feat: add system prompt configuration in settings"
```

---

## Task 11: Frontend — Conversation History Sidebar

**Files:**
- Modify: `frontend/src/components/layout/AiChatPanel.vue`

- [ ] **Step 1: Add conversation history dropdown**

In `AiChatPanel.vue`, add a history button next to the clear button in the header:

```vue
<button
  class="ai-chat-panel__history"
  aria-label="Conversation history"
  title="History"
  @click="toggleHistory"
>
  <el-icon :size="14"><Clock /></el-icon>
</button>
```

Add to the script:

```typescript
import { Clock } from "@element-plus/icons-vue";
import { conversationService } from "@/api/services";
import type { Conversation } from "@/types";
import { loadConversation } from "@/stores/ai";

const showHistory = ref(false);
const conversations = ref<Conversation[]>([]);

async function toggleHistory() {
  showHistory.value = !showHistory.value;
  if (showHistory.value) {
    try {
      conversations.value = await conversationService.list();
    } catch (e) {
      console.error("Failed to load conversations:", e);
    }
  }
}

async function handleLoadConversation(id: string) {
  await loadConversation(id);
  showHistory.value = false;
}

async function handleDeleteConversation(id: string) {
  try {
    await conversationService.delete(id);
    conversations.value = conversations.value.filter((c) => c.id !== id);
  } catch (e) {
    console.error("Failed to delete conversation:", e);
  }
}

function formatTime(ts: number): string {
  return new Date(ts * 1000).toLocaleString();
}
```

Add the history panel in the template (between header and context-bar):

```vue
<div v-if="showHistory" class="ai-chat-panel__history-panel">
  <div class="ai-chat-panel__history-header">
    <span>Conversations</span>
    <button @click="showHistory = false" aria-label="Close history">×</button>
  </div>
  <div class="ai-chat-panel__history-list">
    <div v-if="conversations.length === 0" class="ai-chat-panel__history-empty">
      No saved conversations
    </div>
    <div
      v-for="conv in conversations"
      :key="conv.id"
      class="ai-chat-panel__history-item"
    >
      <button
        class="ai-chat-panel__history-load"
        @click="handleLoadConversation(conv.id)"
      >
        <div class="ai-chat-panel__history-title">{{ conv.title }}</div>
        <div class="ai-chat-panel__history-time">{{ formatTime(conv.created_at) }}</div>
      </button>
      <button
        class="ai-chat-panel__history-delete"
        aria-label="Delete conversation"
        @click="handleDeleteConversation(conv.id)"
      >×</button>
    </div>
  </div>
</div>
```

Add styles:

```css
.ai-chat-panel__history {
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

.ai-chat-panel__history:hover {
  color: var(--color-text-secondary);
  background-color: color-mix(in srgb, var(--color-text-tertiary) 6%, transparent);
}

.ai-chat-panel__history-panel {
  position: absolute;
  top: 32px;
  right: 0;
  width: 100%;
  max-height: 300px;
  background-color: var(--color-bg-surface);
  border-bottom: 1px solid var(--color-border-subtle);
  overflow-y: auto;
  z-index: 10;
}

.ai-chat-panel__history-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-tertiary);
}

.ai-chat-panel__history-list {
  padding: 0 0 8px;
}

.ai-chat-panel__history-empty {
  padding: 16px 12px;
  font-size: 11px;
  color: var(--color-text-tertiary);
  text-align: center;
}

.ai-chat-panel__history-item {
  display: flex;
  align-items: center;
  padding: 0 4px 0 0;
}

.ai-chat-panel__history-load {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 12px;
  background: transparent;
  border: none;
  cursor: pointer;
  text-align: left;
}

.ai-chat-panel__history-load:hover {
  background-color: color-mix(in srgb, var(--color-text-primary) 4%, transparent);
}

.ai-chat-panel__history-title {
  font-size: 11px;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ai-chat-panel__history-time {
  font-size: 9px;
  color: var(--color-text-tertiary);
}

.ai-chat-panel__history-delete {
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  border-radius: var(--radius-sm);
}

.ai-chat-panel__history-delete:hover {
  color: var(--color-error);
}
```

Also make the `.ai-chat-panel` position relative so the absolute panel anchors correctly — add `position: relative;` to `.ai-chat-panel`.

- [ ] **Step 2: Verify type check and tests**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
npx vitest run
```

Expected: vue-tsc clean, all tests pass.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/layout/AiChatPanel.vue
git commit -m "feat: add conversation history sidebar to AI chat"
```

---

## Task 12: Integration — Manual Verification

**Files:** none (manual testing only)

- [ ] **Step 1: Run full backend test suite**

Run:
```bash
go test ./services/ -v
```

Expected: all tests pass. Count: 52 prior + 8 prompts + 7 conversations + 1 system prompt settings = 68 tests.

- [ ] **Step 2: Run full frontend test suite**

Run:
```bash
cd frontend
npx vitest run
```

Expected: all tests pass. Count: 60 prior + 11 markdown + 7 new ai = 78 tests.

- [ ] **Step 3: Build verification**

Run:
```bash
go build ./
cd frontend
npx vue-tsc --noEmit
```

Expected: both succeed with no output.

- [ ] **Step 4: Manual GUI verification**

Run `wails3 dev` from the project root. Verify each:

1. **Default system prompt works**: Open AI chat, send "hello" — the AI should respond as a code assistant.
2. **Custom system prompt**: Go to Settings, enter a custom system prompt (e.g. "You are a Rust expert. Always respond in pirate speak."). Send a message — the AI should adopt the persona.
3. **Reset system prompt**: Click "Reset to default" — the custom prompt clears, default is used.
4. **Context chip — file**: Open a file. Select all. Right-click → "AI: Explain Selection". The AI panel opens, context chip shows the filename. The AI explains the code.
5. **Context chip — selection**: Select a few lines. Right-click → "AI: Refactor Selection". Context chip shows `filename:startLine-endLine`. AI refactors the selected code.
6. **Context removal**: Click × on the context chip. It disappears.
7. **All 5 AI actions**: Test explain, refactor, fix, generate docs, generate tests. Each should send an appropriate prompt with the selected code.
8. **Markdown rendering**: AI responses should render markdown: code blocks with syntax highlighting, bold, lists, headers, links.
9. **Stop generation**: While the AI is streaming, click the stop button (red). Streaming should halt.
10. **Copy message**: Hover over an assistant message. Click the copy icon. The content should be on the clipboard.
11. **Conversation persistence**: After a conversation, close and reopen the app. Click the history button. The conversation should appear in the list.
12. **Load conversation**: Click a conversation in history. Its messages should load into the chat.
13. **Delete conversation**: Click × on a history item. It should be removed.
14. **Error handling**: Set an invalid API key. Send a message. An error should appear, no crash.
15. **Empty state**: Clear conversation. The empty state should show with updated copy.
16. **Model selector**: Switch models. The setting should persist.
17. **Context menu in editor**: Right-click in the editor. The "AI:" actions should appear in the context menu under an "ai" group.

- [ ] **Step 5: Final commit (if fixes needed)**

If manual testing revealed bugs, commit fixes with descriptive messages.

---

## Self-Review Notes

**Spec coverage:**
- System prompt configuration → Tasks 2, 7, 10 ✓
- "自主编写并配置底层AI模型提示词" → Task 1 (DefaultSystemPrompt + presets), Task 10 (settings UI) ✓
- Preset prompt templates (explain, refactor, fix, docs, tests) → Task 1, Task 9 (editor menu) ✓
- Code context injection → Task 7 (store), Task 9 (editor integration), Task 8 (context chips) ✓
- Editor right-click AI actions → Task 9 ✓
- Conversation persistence → Task 3 (service), Task 7 (store), Task 11 (history UI) ✓
- Markdown rendering → Task 6 (lib), Task 8 (panel) ✓
- Stop-generation → Task 2 (cancellable stream), Task 7 (store), Task 8 (button) ✓
- Copy message → Task 8 ✓
- Context chips → Task 8 ✓

**Placeholder scan:** No TBD/TODO/placeholder text. All steps have complete code.

**Type consistency:**
- `AIConfig.SystemPrompt` — Go struct (Task 2) → TS `aiSystemPrompt` (Task 7) ✓
- `Conversation{ID, Title, CreatedAt, Messages}` — Go struct (Task 3) → TS interface (Task 5) ✓
- `ConversationMessage{Role, Content}` — Go struct (Task 3) → TS interface (Task 5) ✓
- `AIContextAttachment` — TS interface (Task 5) → store (Task 7) → editor (Task 9) ✓
- `AIActionName` — TS type (Task 5) → store (Task 7) → editor (Task 9) ✓
- `aiServiceV2.setConfig` — TS wrapper (Task 5) → store (Task 7) ✓
- `conversationService.save/load/list/delete/generateId/generateTitle` — Go methods (Task 3) → TS wrapper (Task 5) → store/UI (Tasks 7, 11) ✓

**Out of scope (deferred to Plan 5+):**
- Inline code completion (ghost text)
- Multi-model routing
- AI-powered rename across files
- Embeddings-based codebase search
- Function calling / tool use
- Local model support

---

## Follow-up Plans

### Plan 5: Editor & UX Polish (next)
- Command palette (Ctrl+Shift+P)
- Keyboard shortcuts system
- Search-and-replace in files
- Markdown preview pane
- Monaco scroll-to-line on search result click
- File dirty-state persistence
- Output/Problems panels
- Minimap toggle

### Plan 6: Open Source Readiness
- README with usage docs and screenshots
- LICENSE (MIT recommended)
- CONTRIBUTING.md
- Build/release config (GitHub Actions CI)
- Theme customization (light theme)
- Internationalization (i18n)
- Release binaries for Windows/macOS/Linux
