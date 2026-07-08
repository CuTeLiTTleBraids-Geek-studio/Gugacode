# AI Core Features 3 (Inline Completion + Conversation Rename + Diff View) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add inline AI code completion (Copilot-style ghost text), conversation rename support, and a Git diff viewer to reach P0 AI IDE baseline.

**Architecture:** Backend adds three new methods (`AIService.Complete`, `ConversationService.UpdateTitle`, `GitService.GetDiff`). Frontend wires them via a Monaco `InlineCompletionsProvider`, a store `renameConversation` function, and a `DiffView` component using `monaco-diff-editor`.

**Tech Stack:** Go 1.25 + Wails v3 + go-git v5.19.1 (backend); Vue 3 + Monaco 0.55 + TypeScript 5 (frontend).

---

## File Structure

### Backend (Go)
- Modify: `services/ai_service.go` — add `Complete` method + `CompletionRequest`/`CompletionResponse` types
- Modify: `services/ai_service_test.go` — add tests for `Complete`
- Modify: `services/conversation_service.go` — add `UpdateTitle` method
- Modify: `services/conversation_service_test.go` — add tests for `UpdateTitle`
- Modify: `services/git_service.go` — add `GetDiff` method
- Modify: `services/git_service_test.go` — add tests for `GetDiff`

### Frontend (TypeScript/Vue)
- Modify: `frontend/src/api/services.ts` — add `complete`, `updateTitle`, `getDiff` wrappers
- Modify: `frontend/src/types/index.ts` — add `CompletionRequest`, `CompletionResponse` types
- Modify: `frontend/src/stores/ai.ts` — add `renameConversation` function
- Modify: `frontend/src/components/editor/CodeEditor.vue` — register InlineCompletionsProvider
- Modify: `frontend/src/components/layout/AiChatPanel.vue` — add rename UI
- Create: `frontend/src/components/editor/DiffView.vue` — monaco-diff-editor component
- Modify: `frontend/src/components/layout/GitPanel.vue` — wire diff view button
- Create: `frontend/src/stores/inlineCompletion.ts` — debounce + completion state

---

## Task 1: AIService.Complete — Backend Inline Completion

**Files:**
- Modify: `services/ai_service.go`
- Test: `services/ai_service_test.go`

- [ ] **Step 1: Write the failing test**

Add to `services/ai_service_test.go`:

```go
func TestAIService_Complete_ReturnsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"fmt.Println"}}]}`)
	}))
	defer srv.Close()

	svc := NewAIService()
	svc.SetConfig(AIConfig{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-4o"})

	resp, err := svc.Complete(CompletionRequest{
		Prefix:   "package main\n\nfunc main() {\n    ",
		Suffix:   "\n}",
		Language: "go",
		FilePath: "main.go",
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if resp.Text != "fmt.Println" {
		t.Errorf("expected 'fmt.Println', got %q", resp.Text)
	}
}

func TestAIService_Complete_NoAPIKey(t *testing.T) {
	svc := NewAIService()
	_, err := svc.Complete(CompletionRequest{Prefix: "x", Suffix: ""})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestAIService_Complete -v`
Expected: FAIL with "undefined: CompletionRequest"

- [ ] **Step 3: Write minimal implementation**

Add to `services/ai_service.go` (after the `Send` method):

```go
// CompletionRequest holds the context for an inline code completion request.
type CompletionRequest struct {
	Prefix   string `json:"prefix"`
	Suffix   string `json:"suffix"`
	Language string `json:"language"`
	FilePath string `json:"filePath"`
}

// CompletionResponse holds the AI-generated completion text.
type CompletionResponse struct {
	Text string `json:"text"`
}

// completeSystemPrompt returns the system prompt tailored for code completion.
func completeSystemPrompt(language string) string {
	return "You are an inline code completion assistant. " +
		"Complete the code at the cursor position. " +
		"Return ONLY the completion text (what should be inserted), with no markdown fences, no explanations, no leading/trailing newlines. " +
		"Keep completions concise (1-3 lines typically, up to ~10 lines for multi-line constructs). " +
		"Language: " + language + ". " +
		"Match the surrounding code style, indentation, and naming conventions."
}

// Complete sends a non-streaming completion request and returns the suggested text.
func (a *AIService) Complete(req CompletionRequest) (*CompletionResponse, error) {
	if a.config.APIKey == "" {
		return nil, errors.New("API key not configured")
	}

	userMsg := fmt.Sprintf("File: %s\nLanguage: %s\n\nCode before cursor:\n%s\n\nCode after cursor:\n%s\n\nComplete the code at the cursor:",
		req.FilePath, req.Language, req.Prefix, req.Suffix)

	messages := []ChatMessage{
		{Role: "system", Content: completeSystemPrompt(req.Language)},
		{Role: "user", Content: userMsg},
	}

	reqBody := map[string]interface{}{
		"model":       a.config.Model,
		"messages":    messages,
		"max_tokens":  256,
		"temperature": 0.2,
		"stream":      false,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", a.config.BaseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	resp, err := aiHTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return &CompletionResponse{Text: ""}, nil
	}

	return &CompletionResponse{Text: strings.TrimSpace(result.Choices[0].Message.Content)}, nil
}
```

Add `"strings"` to the import block if not already present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestAIService_Complete -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/ai_service.go services/ai_service_test.go
git commit -m "feat: add AIService.Complete for inline code completion"
```

---

## Task 2: ConversationService.UpdateTitle — Backend Rename

**Files:**
- Modify: `services/conversation_service.go`
- Test: `services/conversation_service_test.go`

- [ ] **Step 1: Write the failing test**

Add to `services/conversation_service_test.go`:

```go
func TestConversationService_UpdateTitle(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)

	conv := Conversation{
		ID:        "test-conv-1",
		Title:     "Old Title",
		CreatedAt: time.Now().UnixMilli(),
		Messages:  []ConversationMessage{{Role: "user", Content: "hello"}},
	}
	if err := svc.Save(conv); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := svc.UpdateTitle("test-conv-1", "New Title"); err != nil {
		t.Fatalf("UpdateTitle failed: %v", err)
	}

	loaded, err := svc.Load("test-conv-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Title != "New Title" {
		t.Errorf("expected 'New Title', got %q", loaded.Title)
	}
	// Messages should be preserved
	if len(loaded.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(loaded.Messages))
	}
}

func TestConversationService_UpdateTitle_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)
	err := svc.UpdateTitle("nonexistent", "New Title")
	if err == nil {
		t.Fatal("expected error for nonexistent conversation")
	}
}

func TestConversationService_UpdateTitle_EmptyTitle(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)
	conv := Conversation{ID: "c1", Title: "Old", CreatedAt: time.Now().UnixMilli()}
	svc.Save(conv)
	err := svc.UpdateTitle("c1", "")
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestConversationService_UpdateTitle -v`
Expected: FAIL with "undefined: UpdateTitle"

- [ ] **Step 3: Write minimal implementation**

Add to `services/conversation_service.go` (after the `Delete` method):

```go
// UpdateTitle renames an existing conversation. Returns an error if the
// conversation doesn't exist or the new title is empty.
func (p *ConversationService) UpdateTitle(id string, title string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("title cannot be empty")
	}
	conv, err := p.Load(id)
	if err != nil {
		return fmt.Errorf("failed to load conversation: %w", err)
	}
	conv.Title = strings.TrimSpace(title)
	return p.Save(conv)
}
```

Add `"errors"`, `"fmt"`, and `"strings"` to the import block if not already present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestConversationService_UpdateTitle -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/conversation_service.go services/conversation_service_test.go
git commit -m "feat: add ConversationService.UpdateTitle for renaming"
```

---

## Task 3: GitService.GetDiff — Backend Diff Generation

**Files:**
- Modify: `services/git_service.go`
- Test: `services/git_service_test.go`

- [ ] **Step 1: Write the failing test**

Add to `services/git_service_test.go`:

```go
func TestGitService_GetDiff_ModifiedFile(t *testing.T) {
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit failed: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}

	// Create and commit initial file
	initialContent := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := wt.Commit("initial", &git.CommitOptions{Author: testAuthor}); err != nil {
		t.Fatal(err)
	}

	// Modify the file
	modifiedContent := "package main\n\nfunc main() {\n    println(\"hello\")\n}\n"
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	svc := &GitService{}
	diff, err := svc.GetDiff(repoDir, "main.go")
	if err != nil {
		t.Fatalf("GetDiff failed: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "println") {
		t.Errorf("diff should contain 'println', got: %s", diff)
	}
	if !strings.Contains(diff, "+") && !strings.Contains(diff, "-") {
		t.Errorf("diff should contain +/- markers, got: %s", diff)
	}
}

func TestGitService_GetDiff_UntrackedFile(t *testing.T) {
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit failed: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}
	os.WriteFile(filepath.Join(repoDir, "new.go"), []byte("package main\n"), 0644)
	wt.Add("new.go")

	svc := &GitService{}
	diff, err := svc.GetDiff(repoDir, "new.go")
	if err != nil {
		t.Fatalf("GetDiff failed: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff for staged new file")
	}
}
```

Note: If `testAuthor` is not defined in the test file, add this near the top:
```go
var testAuthor = object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}
```

And ensure `"os"`, `"path/filepath"`, `"strings"`, `"time"`, `"github.com/go-git/go-git/v5"`, `"github.com/go-git/go-git/v5/plumbing/object"` are imported.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/ -run TestGitService_GetDiff -v`
Expected: FAIL with "undefined: GetDiff"

- [ ] **Step 3: Write minimal implementation**

Add to `services/git_service.go` (after the `Commit` method):

```go
// GetDiff returns the unified diff for a single file. If the file is staged,
// it shows the staged diff (HEAD → index); otherwise it shows the working
// tree diff (index → worktree). For untracked files, returns the full file
// content as additions.
func (g *GitService) GetDiff(repoPath string, filePath string) (string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	status, err := wt.Status()
	if err != nil {
		return "", err
	}

	fileStatus, ok := status[filePath]
	if !ok {
		return "", fmt.Errorf("file %s not found in git status", filePath)
	}

	// If untracked (??), return the file content as all-added
	if fileStatus.Staging == git.Untracked && fileStatus.Worktree == git.Untracked {
		absPath := filepath.Join(repoPath, filePath)
		data, err := os.ReadFile(absPath)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", filePath, filePath))
		buf.WriteString(fmt.Sprintf("new file mode 100644\n"))
		buf.WriteString(fmt.Sprintf("--- /dev/null\n"))
		buf.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))
		for _, line := range strings.Split(string(data), "\n") {
			buf.WriteString("+" + line + "\n")
		}
		return buf.String(), nil
	}

	// For staged changes: diff HEAD vs index
	if fileStatus.Staging != git.Unmodified && fileStatus.Staging != git.Untracked {
		headRef, err := repo.Head()
		if err != nil {
			return "", err
		}
		headCommit, err := repo.CommitObject(headRef.Hash())
		if err != nil {
			return "", err
		}
		headTree, err := headCommit.Tree()
		if err != nil {
			return "", err
		}
		index, err := repo.Storer.Index()
		if err != nil {
			return "", err
		}
		// Build a tree from the current index
		// Use go-git's tree diff
		return diffTrees(repo, headTree, index, filePath)
	}

	// For unstaged changes: diff index vs worktree
	return g.diffWorktree(repo, wt, filePath)
}

// diffWorktree compares the index version of a file against the working tree version.
func (g *GitService) diffWorktree(repo *git.Repository, wt *git.Worktree, filePath string) (string, error) {
	idx, err := repo.Storer.Index()
	if err != nil {
		return "", err
	}

	idxEntry, err := idx.Entry(filePath)
	if err != nil {
		// File is new in worktree (not in index)
		absPath := filepath.Join(wt.Filesystem.Root(), filePath)
		data, err := os.ReadFile(absPath)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", filePath, filePath))
		buf.WriteString("--- /dev/null\n")
		buf.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))
		for _, line := range strings.Split(string(data), "\n") {
			buf.WriteString("+" + line + "\n")
		}
		return buf.String(), nil
	}

	// Get the blob from the index
	obj, err := repo.BlobObject(idxEntry.Hash)
	if err != nil {
		return "", err
	}
	idxReader, err := obj.Reader()
	if err != nil {
		return "", err
	}
	defer idxReader.Close()
	idxData, err := io.ReadAll(idxReader)
	if err != nil {
		return "", err
	}

	// Read working tree version
	absPath := filepath.Join(wt.Filesystem.Root(), filePath)
	wtData, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	return unifiedDiff(filePath, string(idxData), string(wtData)), nil
}

// diffTrees generates a diff for a specific file between a tree and the index.
func diffTrees(repo *git.Repository, headTree *object.Tree, index *index.Index, filePath string) (string, error) {
	// Get HEAD version of the file
	headFile, err := headTree.File(filePath)
	if err != nil {
		// File is new (added in index)
		idxEntry, err := index.Entry(filePath)
		if err != nil {
			return "", err
		}
		blob, err := repo.BlobObject(idxEntry.Hash)
		if err != nil {
			return "", err
		}
		reader, err := blob.Reader()
		if err != nil {
			return "", err
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", filePath, filePath))
		buf.WriteString("new file mode 100644\n")
		buf.WriteString("--- /dev/null\n")
		buf.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))
		for _, line := range strings.Split(string(data), "\n") {
			buf.WriteString("+" + line + "\n")
		}
		return buf.String(), nil
	}

	headReader, err := headFile.Reader()
	if err != nil {
		return "", err
	}
	defer headReader.Close()
	headData, err := io.ReadAll(headReader)
	if err != nil {
		return "", err
	}

	// Get index version
	idxEntry, err := index.Entry(filePath)
	if err != nil {
		return "", err
	}
	idxBlob, err := repo.BlobObject(idxEntry.Hash)
	if err != nil {
		return "", err
	}
	idxReader, err := idxBlob.Reader()
	if err != nil {
		return "", err
	}
	defer idxReader.Close()
	idxData, err := io.ReadAll(idxReader)
	if err != nil {
		return "", err
	}

	return unifiedDiff(filePath, string(headData), string(idxData)), nil
}

// unifiedDiff produces a simple line-by-line unified diff (no context lines
// for simplicity; this is sufficient for display in a diff editor).
func unifiedDiff(filePath string, oldText string, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", filePath, filePath))
	buf.WriteString(fmt.Sprintf("--- a/%s\n", filePath))
	buf.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))

	// Simple line-by-line comparison (not a full Myers diff, but adequate for display)
	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		var oldLine, newLine string
		hasOld := i < len(oldLines)
		hasNew := i < len(newLines)
		if hasOld {
			oldLine = oldLines[i]
		}
		if hasNew {
			newLine = newLines[i]
		}
		if hasOld && hasNew && oldLine == newLine {
			buf.WriteString(" " + newLine + "\n")
		} else {
			if hasOld {
				buf.WriteString("-" + oldLine + "\n")
			}
			if hasNew {
				buf.WriteString("+" + newLine + "\n")
			}
		}
	}
	return buf.String()
}
```

Add these imports to `git_service.go` if not present: `"bytes"`, `"fmt"`, `"io"`, `"os"`, `"path/filepath"`, `"strings"`, `"github.com/go-git/go-git/v5"`, `"github.com/go-git/go-git/v5/plumbing/object"`, `"github.com/go-git/go-git/v5/utils/index"`.

Note: The `index` package import path may need adjustment based on go-git v5.19.1. Check the actual path with `go list github.com/go-git/go-git/v5/utils/index` — if unavailable, use `repo.Storer.Index()` which returns `*index.Index` from `github.com/go-git/go-git/v5/utils/index`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/ -run TestGitService_GetDiff -v`
Expected: PASS

If the import for `index` fails, adjust: the correct type is obtained from `repo.Storer.Index()` which returns `index.Index` — you may need to import `github.com/go-git/go-git/v5/utils/index` or access the entry via the index's `Entries` slice.

- [ ] **Step 5: Commit**

```bash
git add services/git_service.go services/git_service_test.go
git commit -m "feat: add GitService.GetDiff for file-level diffs"
```

---

## Task 4: Frontend Types & API Wrappers

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/services.ts`

- [ ] **Step 1: Add types**

Add to `frontend/src/types/index.ts`:

```typescript
/** Request payload for AI inline code completion. */
export interface CompletionRequest {
  prefix: string;
  suffix: string;
  language: string;
  filePath: string;
}

/** Response from AI inline code completion. */
export interface CompletionResponse {
  text: string;
}
```

- [ ] **Step 2: Add API wrappers**

In `frontend/src/api/services.ts`, add to the `aiService` object:

```typescript
export const aiService = {
  // ... existing methods ...
  complete: (req: CompletionRequest) =>
    aiServiceBinding.Complete(req).then((r: any) => ({ text: r.Text ?? "" })),
};
```

Add to the `conversationService` object:

```typescript
export const conversationService = {
  // ... existing methods ...
  updateTitle: (id: string, title: string) =>
    conversationServiceBinding.UpdateTitle(id, title),
};
```

Add to the `gitService` object:

```typescript
export const gitService = {
  // ... existing methods ...
  getDiff: (path: string, filePath: string) =>
    gitServiceBinding.GetDiff(path, filePath),
};
```

Ensure `CompletionRequest` is imported in the types import block at the top of `services.ts`.

Note: The Wails Vite plugin auto-generates the binding JS files (`aiservice.js`, `conversationservice.js`, `gitservice.js`) when the Go services are updated. After running the dev server or `wails3 generate bindings`, the new `Complete`, `UpdateTitle`, and `GetDiff` functions will appear in the binding files. If the binding files are NOT auto-generated in the test environment, manually add the binding entries (see Task 4b below).

- [ ] **Step 3: Verify types compile**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0 (may have errors if binding files not yet regenerated — if so, proceed to Task 4b)

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/services.ts
git commit -m "feat: add complete/updateTitle/getDiff API wrappers"
```

---

## Task 4b: Manual Binding File Updates (if auto-generation unavailable)

**Files:**
- Modify: `frontend/bindings/changeme/services/aiservice.js`
- Modify: `frontend/bindings/changeme/services/conversationservice.js`
- Modify: `frontend/bindings/changeme/services/gitservice.js`

Since the Wails Vite plugin may not regenerate bindings in all environments, manually add the new binding entries. Each binding ID is the FNV-1a hash of `changeme/services.TypeName.MethodName`.

- [ ] **Step 1: Compute binding IDs**

Run this Go snippet to get the IDs:

```go
package main

import (
	"fmt"
	"hash/fnv"
)

func bindingID(name string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(name))
	return h.Sum32()
}

func main() {
	fmt.Println("AIService.Complete:", bindingID("changeme/services.AIService.Complete"))
	fmt.Println("ConversationService.UpdateTitle:", bindingID("changeme/services.ConversationService.UpdateTitle"))
	fmt.Println("GitService.GetDiff:", bindingID("changeme/services.GitService.GetDiff"))
}
```

- [ ] **Step 2: Add Complete to aiservice.js**

Add the `Complete` export using the computed binding ID:

```javascript
export function Complete(req) {
  return $Call(bindingID, req);
}
```

Where `bindingID` is the number from Step 1.

- [ ] **Step 3: Add UpdateTitle to conversationservice.js**

```javascript
export function UpdateTitle(id, title) {
  return $Call(bindingID, id, title);
}
```

- [ ] **Step 4: Add GetDiff to gitservice.js**

```javascript
export function GetDiff(path, filePath) {
  return $Call(bindingID, path, filePath);
}
```

- [ ] **Step 5: Verify frontend compiles**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

---

## Task 5: Inline Completion Provider in CodeEditor.vue

**Files:**
- Modify: `frontend/src/components/editor/CodeEditor.vue`
- Create: `frontend/src/stores/inlineCompletion.ts`

- [ ] **Step 1: Create the inline completion store**

Create `frontend/src/stores/inlineCompletion.ts`:

```typescript
import { ref } from "vue";
import { aiService } from "@/api/services";

/** Tracks the last completion request timestamp for debounce/dedup. */
let lastRequestTime = 0;
let pendingController: AbortController | null = null;

/** Minimum milliseconds between completion requests. */
const DEBOUNCE_MS = 300;

/** Minimum prefix length before requesting a completion. */
const MIN_PREFIX_LENGTH = 10;

export const inlineCompletionEnabled = ref(true);

/**
 * Request an inline completion from the AI service.
 * Returns the completion text or empty string if no completion is available.
 * Debounces requests to avoid excessive API calls.
 */
export async function requestCompletion(
  prefix: string,
  suffix: string,
  language: string,
  filePath: string
): Promise<string> {
  if (!inlineCompletionEnabled.value) return "";
  if (prefix.length < MIN_PREFIX_LENGTH) return "";

  const now = Date.now();
  if (now - lastRequestTime < DEBOUNCE_MS) return "";
  lastRequestTime = now;

  // Cancel any pending request
  if (pendingController) {
    pendingController.abort();
  }
  pendingController = new AbortController();

  try {
    const response = await aiService.complete({ prefix, suffix, language, filePath });
    pendingController = null;
    return response?.text ?? "";
  } catch (e) {
    pendingController = null;
    // Silently fail — inline completion is best-effort
    return "";
  }
}

export function toggleInlineCompletion(): void {
  inlineCompletionEnabled.value = !inlineCompletionEnabled.value;
}
```

- [ ] **Step 2: Register the InlineCompletionsProvider in CodeEditor.vue**

In `frontend/src/components/editor/CodeEditor.vue`, modify the `handleMount` function to register the provider:

```typescript
import { requestCompletion } from "@/stores/inlineCompletion";

function handleMount(editor: monacoEditor.editor.IStandaloneCodeEditor) {
  editor.onDidChangeCursorPosition((e) => {
    emit("cursor-change", e.position.lineNumber, e.position.column);
  });
  registerContextMenuActions(editor);
  registerInlineCompletionProvider(editor);
}

function registerInlineCompletionProvider(editor: monacoEditor.editor.IStandaloneCodeEditor) {
  // Register for all languages — the AI service handles language context
  monacoEditor.languages.registerInlineCompletionsProvider({ pattern: "**" }, {
    provideInlineCompletions: async (model, position) => {
      const language = resolvedLanguage.value;
      const filePath = props.path;

      // Get prefix (everything before cursor, up to 2000 chars for context)
      const startLine = Math.max(1, position.lineNumber - 50);
      const prefix = model.getValueInRange({
        startLineNumber: startLine,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      });

      // Get suffix (everything after cursor, up to 1000 chars)
      const lineCount = model.getLineCount();
      const endLine = Math.min(lineCount, position.lineNumber + 30);
      const suffix = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: position.column,
        endLineNumber: endLine,
        endColumn: model.getLineMaxColumn(endLine),
      });

      const text = await requestCompletion(prefix, suffix, language, filePath);
      if (!text) {
        return { items: [] };
      }

      return {
        items: [
          {
            insertText: text,
            range: {
              startLineNumber: position.lineNumber,
              startColumn: position.column,
              endLineNumber: position.lineNumber,
              endColumn: position.column,
            },
          },
        ],
      };
    },
    handleItemDidAccept: () => {
      // Called when the user accepts a completion — no action needed
    },
  });
}
```

Add `monacoEditor` to the import from `@guolao/vue-monaco-editor` if not already present (it should be imported as the type namespace).

- [ ] **Step 3: Verify frontend compiles**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Run existing tests**

Run: `cd frontend && npx vitest run`
Expected: all existing tests pass (78+)

- [ ] **Step 5: Commit**

```bash
git add frontend/src/stores/inlineCompletion.ts frontend/src/components/editor/CodeEditor.vue
git commit -m "feat: add AI inline code completion (Copilot-style ghost text)"
```

---

## Task 6: Conversation Rename in AI Store & UI

**Files:**
- Modify: `frontend/src/stores/ai.ts`
- Modify: `frontend/src/components/layout/AiChatPanel.vue`

- [ ] **Step 1: Add renameConversation to ai.ts store**

In `frontend/src/stores/ai.ts`, add:

```typescript
/**
 * Rename the current conversation. Updates the backend and the local state
 * (if the conversation is loaded). Returns true on success.
 */
export async function renameConversation(id: string, newTitle: string): Promise<boolean> {
  const trimmed = newTitle.trim();
  if (!trimmed) return false;
  try {
    await conversationService.updateTitle(id, trimmed);
    // If the renamed conversation is the current one, update the loaded messages
    if (aiState.currentConversationId === id) {
      // We don't store the title in aiState, but the conversation list UI
      // should refresh. The caller can reload the conversation list.
    }
    return true;
  } catch (e) {
    notifyError(`Failed to rename conversation: ${e instanceof Error ? e.message : String(e)}`);
    return false;
  }
}
```

Ensure `conversationService` and `notifyError` are imported.

- [ ] **Step 2: Add rename UI to AiChatPanel.vue**

In `frontend/src/components/layout/AiChatPanel.vue`, add a rename button and dialog. Add these refs and functions to the `<script setup>`:

```typescript
import { renameConversation, aiState } from "@/stores/ai";
import { ElMessageBox } from "element-plus";
import { Edit } from "@element-plus/icons-vue";

async function handleRenameConversation() {
  const convId = aiState.currentConversationId;
  if (!convId) {
    notifyWarning("No active conversation to rename");
    return;
  }
  try {
    const { value } = await ElMessageBox.prompt("Enter new conversation title:", "Rename Conversation", {
      confirmButtonText: "Rename",
      cancelButtonText: "Cancel",
      inputPattern: /.+/,
      inputErrorMessage: "Title cannot be empty",
    });
    if (value) {
      await renameConversation(convId, value);
      notifySuccess("Conversation renamed");
    }
  } catch {
    // User cancelled — no action needed
  }
}
```

Add a rename button in the template (near the "Clear" or "New Chat" button):

```html
<el-button
  :icon="Edit"
  size="small"
  aria-label="Rename conversation"
  @click="handleRenameConversation"
>
  Rename
</el-button>
```

Ensure `notifyWarning`, `notifySuccess` are imported from `@/lib/notifications`.

- [ ] **Step 3: Verify frontend compiles**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add frontend/src/stores/ai.ts frontend/src/components/layout/AiChatPanel.vue
git commit -m "feat: add conversation rename with prompt dialog"
```

---

## Task 7: DiffView Component & GitPanel Integration

**Files:**
- Create: `frontend/src/components/editor/DiffView.vue`
- Modify: `frontend/src/components/layout/GitPanel.vue`

- [ ] **Step 1: Create DiffView.vue**

Create `frontend/src/components/editor/DiffView.vue`:

```vue
<script setup lang="ts">
import { ref, watch, computed } from "vue";
import { VueMonacoEditor } from "@guolao/vue-monaco-editor";
import type * as monacoEditor from "monaco-editor";
import { gitService } from "@/api/services";
import { appState } from "@/stores/app";
import { detectLanguage } from "@/lib/language";
import { getMonacoThemeName } from "@/lib/monaco-themes";
import { Close } from "@element-plus/icons-vue";
import { notifyError } from "@/lib/notifications";

const props = defineProps<{
  repoPath: string;
  filePath: string;
  visible: boolean;
}>();

const emit = defineEmits<{
  (e: "close"): void;
}>();

const originalContent = ref("");
const modifiedContent = ref("");
const loading = ref(false);
const diffInfo = ref<string>("");

const monacoTheme = computed(() => getMonacoThemeName(appState.accentTheme));
const language = computed(() => detectLanguage(props.filePath));

const diffOptions = computed<monacoEditor.editor.IDiffEditorConstructionOptions>(() => ({
  readOnly: true,
  renderSideBySide: true,
  minimap: { enabled: false },
  fontSize: appState.fontSize,
  fontFamily: appState.fontFamily,
  lineHeight: 20,
  scrollBeyondLastLine: false,
  automaticLayout: true,
}));

async function loadDiff() {
  if (!props.filePath || !props.repoPath) return;
  loading.value = true;
  try {
    const diffText = await gitService.getDiff(props.repoPath, props.filePath);
    diffInfo.value = diffText;

    // Parse the diff to extract original and modified content
    // For a simple approach, we'll load the file from disk as "modified"
    // and use the git diff to reconstruct the "original"
    // A simpler approach: use the diff text directly in a read-only editor
    // But for a proper diff view, we need both versions.

    // Load current (modified) file content from disk
    // The original content would need to come from git show HEAD:filepath
    // For now, we'll display the raw diff text in a Monaco editor
    modifiedContent.value = diffText;
    originalContent.value = "";
  } catch (e) {
    notifyError(`Failed to load diff: ${e instanceof Error ? e.message : String(e)}`);
  } finally {
    loading.value = false;
  }
}

watch(() => [props.visible, props.filePath], ([vis, _path]) => {
  if (vis) loadDiff();
}, { immediate: true });

function handleClose() {
  emit("close");
}
</script>

<template>
  <div v-if="visible" class="diff-view">
    <div class="diff-view__header">
      <span class="diff-view__title">Diff: {{ filePath }}</span>
      <el-button
        :icon="Close"
        size="small"
        aria-label="Close diff view"
        @click="handleClose"
      >
        Close
      </el-button>
    </div>
    <div v-if="loading" class="diff-view__loading">Loading diff...</div>
    <div v-else class="diff-view__editor">
      <VueMonacoEditor
        :value="modifiedContent"
        :language="language"
        :theme="monacoTheme"
        :options="{
          readOnly: true,
          fontSize: appState.fontSize,
          fontFamily: appState.fontFamily,
          minimap: { enabled: false },
          lineNumbers: 'on',
          scrollBeyondLastLine: false,
        }"
        height="100%"
      />
    </div>
  </div>
</template>

<style scoped>
.diff-view {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--color-bg-base);
  z-index: 10;
  display: flex;
  flex-direction: column;
}

.diff-view__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  border-bottom: 1px solid var(--color-border-subtle);
  background-color: var(--color-bg-surface);
}

.diff-view__title {
  font-size: 12px;
  color: var(--color-text-primary);
  font-family: var(--font-mono);
}

.diff-view__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--color-text-tertiary);
  font-size: 12px;
}

.diff-view__editor {
  flex: 1;
  min-height: 0;
}
</style>
```

- [ ] **Step 2: Wire DiffView into GitPanel.vue**

In `frontend/src/components/layout/GitPanel.vue`, add a "View Diff" button for each changed file. Add to the `<script setup>`:

```typescript
import DiffView from "@/components/editor/DiffView.vue";

const diffVisible = ref(false);
const diffFilePath = ref("");

function viewDiff(filePath: string) {
  diffFilePath.value = filePath;
  diffVisible.value = true;
}
```

Add a diff button next to each file in the changes list. In the template, for each file change row, add:

```html
<el-button
  size="small"
  link
  aria-label="View diff"
  @click="viewDiff(change.path)"
>
  Diff
</el-button>
```

Add the DiffView component at the end of the template:

```html
<DiffView
  :repo-path="appState.currentProject ?? ''"
  :file-path="diffFilePath"
  :visible="diffVisible"
  @close="diffVisible = false"
/>
```

Ensure `ref` is imported from vue.

- [ ] **Step 3: Verify frontend compiles**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Run tests**

Run: `cd frontend && npx vitest run`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/editor/DiffView.vue frontend/src/components/layout/GitPanel.vue
git commit -m "feat: add Git diff view with file-level diff display"
```

---

## Task 8: Full Verification

- [ ] **Step 1: Run all Go tests**

Run: `go test ./services/... -count=1 -timeout 60s`
Expected: all PASS

- [ ] **Step 2: Run go build**

Run: `go build .`
Expected: exit 0

- [ ] **Step 3: Run vue-tsc**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Run vitest**

Run: `cd frontend && npx vitest run`
Expected: all tests pass

- [ ] **Step 5: Manual GUI verification notes**

The following should be manually verified in the running app:
1. **Inline completion**: Open a code file, type code, wait ~300ms — ghost text should appear. Tab to accept, Esc to dismiss.
2. **Conversation rename**: Start an AI chat, send a message, click "Rename" button — a dialog should appear, enter new title, confirm.
3. **Diff view**: Open Git panel, click "Diff" on a changed file — diff view should overlay showing the changes.
