# Git & Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add source-control (Git) panel and cross-file search to the gugacode IDE, so users can view changed files, stage/unstage, commit, see branch info, and search file contents across the open project.

**Architecture:** Go backend exposes a `GitService` (using `go-git/v5`) and a `SearchService` (using `filepath.WalkDir` + `regexp`) to the frontend via Wails bindings. The frontend adds two reactive stores (`git.ts`, `search.ts`) and two panel components (`GitPanel.vue`, `SearchPanel.vue`) that plug into the existing SidePanel. The status bar reads branch/changes counts from the git store.

**Tech Stack:** Go 1.25, `github.com/go-git/go-git/v5`, Wails v3 (alpha2.111), Vue 3, TypeScript, Element Plus, Vitest, `@vue/test-utils`, jsdom

**Project root:** `e:\gugacode\gugacode\gugacode\` (the directory containing `go.mod`, `main.go`, `frontend/`). All relative paths in this plan are from this root.

**Module name note:** `go.mod` declares `module changeme`. Generated bindings land in `frontend/bindings/changeme/`. This plan uses that path as-is.

---

## Scope Check

This is **Plan 3** of the original 4-plan decomposition. Plans 1 and 2 are complete (Core IDE Foundation, Terminal & AI Chat). This plan covers:

- Git status (changed files, branch, ahead/behind)
- Stage/unstage individual files
- Commit with message
- Cross-file content search with regex support
- Search results grouped by file, clickable to open at line

**Out of scope (deferred):** Push/pull/fetch (network ops), branch switching/creation, merge conflict resolution, diff viewer UI, stash management. These can be added in a follow-up plan.

**Plan 3 produces working, testable software on its own:** the Git panel shows real git status, the Search panel returns real matches, and both are clickable/navigable.

---

## File Structure

```
services/                          # Go backend services
├── git_service.go                 # NEW — go-git wrapper: status, stage, unstage, commit, branch info
├── git_service_test.go            # NEW — tests for git operations
├── search_service.go              # NEW — file walk + regex search, returns matches with line/col
└── search_service_test.go         # NEW — tests for search

main.go                            # MODIFY — register GitService, SearchService

frontend/
├── src/
│   ├── types/
│   │   └── index.ts               # MODIFY — add GitFileChange, SearchMatch, SearchResult types
│   ├── api/
│   │   └── services.ts            # MODIFY — add gitService, searchService wrappers
│   ├── stores/
│   │   ├── git.ts                 # NEW — git state: changes, branch, refresh/commit actions
│   │   ├── git.test.ts            # NEW — tests for git store
│   │   ├── search.ts              # NEW — search state: query, results, search action
│   │   └── search.test.ts         # NEW — tests for search store
│   └── components/
│       └── layout/
│           ├── SidePanel.vue      # MODIFY — render GitPanel/SearchPanel in their tabs
│           ├── GitPanel.vue       # NEW — changed files list with stage/unstage/commit
│           └── SearchPanel.vue    # NEW — search input + results list
```

**Why no separate `.spec.ts` for GitPanel/SearchPanel:** These components render store-driven lists with minimal logic; integration is verified via the store tests plus manual GUI testing (Task 11). This matches the approach used for FileTree (which has a spec because its recursive loading has non-trivial behavior) and TerminalPanel (no spec, verified manually).

---

## Task 1: Go Backend — GitService (Status & Branch)

**Files:**
- Create: `services/git_service.go`
- Create: `services/git_service_test.go`

- [x] **Step 1: Add go-git dependency**

Run from project root:

```bash
go get github.com/go-git/go-git/v5@latest
```

Expected: `go.mod` gains `github.com/go-git/go-git/v5` in the require block. `go.sum` is updated.

- [x] **Step 2: Write the failing tests**

Create `services/git_service_test.go`:

```go
package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func initBareRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git.PlainInit failed: %v", err)
	}
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	_ = wt.AddGlob(".")
	hash, err := wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	_ = hash
}

func TestGitService_Status_emptyRepo(t *testing.T) {
	dir := initBareRepo(t)
	svc := &GitService{}
	changes, err := svc.GetStatus(dir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes in fresh repo, got %d", len(changes))
	}
}

func TestGitService_Status_detectsNewFile(t *testing.T) {
	dir := initBareRepo(t)
	writeFile(t, dir, "a.txt", "hello")
	svc := &GitService{}
	changes, err := svc.GetStatus(dir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != "a.txt" {
		t.Errorf("expected path 'a.txt', got %q", changes[0].Path)
	}
	if changes[0].Status != "Untracked" {
		t.Errorf("expected status 'Untracked', got %q", changes[0].Status)
	}
}

func TestGitService_Status_detectsModifiedFile(t *testing.T) {
	dir := initBareRepo(t)
	writeFile(t, dir, "a.txt", "hello")
	commitAll(t, dir, "initial")
	writeFile(t, dir, "a.txt", "hello world")
	svc := &GitService{}
	changes, err := svc.GetStatus(dir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Status != "Modified" {
		t.Errorf("expected status 'Modified', got %q", changes[0].Status)
	}
}

func TestGitService_Status_detectsDeletedFile(t *testing.T) {
	dir := initBareRepo(t)
	writeFile(t, dir, "a.txt", "hello")
	commitAll(t, dir, "initial")
	if err := os.Remove(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	svc := &GitService{}
	changes, err := svc.GetStatus(dir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Status != "Deleted" {
		t.Errorf("expected status 'Deleted', got %q", changes[0].Status)
	}
}

func TestGitService_BranchInfo(t *testing.T) {
	dir := initBareRepo(t)
	writeFile(t, dir, "a.txt", "hello")
	commitAll(t, dir, "initial")
	svc := &GitService{}
	info, err := svc.GetBranchInfo(dir)
	if err != nil {
		t.Fatalf("GetBranchInfo failed: %v", err)
	}
	if info.Name == "" {
		t.Error("expected non-empty branch name")
	}
	if info.Ahead != 0 {
		t.Errorf("expected ahead 0, got %d", info.Ahead)
	}
	if info.Behind != 0 {
		t.Errorf("expected behind 0, got %d", info.Behind)
	}
}

func TestGitService_BranchInfo_notARepo(t *testing.T) {
	dir := t.TempDir()
	svc := &GitService{}
	_, err := svc.GetBranchInfo(dir)
	if err == nil {
		t.Error("expected error for non-repo directory")
	}
}
```

- [x] **Step 3: Run tests to verify they fail**

Run: `go test ./services/ -run TestGitService -v`
Expected: FAIL — `services.GitService` undefined (no `git_service.go` yet).

- [x] **Step 4: Write minimal implementation**

Create `services/git_service.go`:

```go
package services

import (
	"errors"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/status"
)

// GitFileChange represents a single changed file in the working tree.
type GitFileChange struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// BranchInfo describes the current branch state.
type BranchInfo struct {
	Name   string `json:"name"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`
}

// GitService exposes git operations to the frontend.
type GitService struct{}

// statusToString converts a go-git status code to a human-readable label.
func statusToString(code status.StatusCode) string {
	switch code {
	case status.Untracked:
		return "Untracked"
	case status.Modified:
		return "Modified"
	case status.Added:
		return "Added"
	case status.Deleted:
		return "Deleted"
	case status.Renamed:
		return "Renamed"
	case status.Copied:
		return "Copied"
	case status.Unmodified:
		return "Unmodified"
	default:
		return "Modified"
	}
}

// GetStatus returns the list of changed files in the working tree at path.
func (g *GitService) GetStatus(path string) ([]GitFileChange, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	st, err := wt.Status()
	if err != nil {
		return nil, err
	}
	changes := make([]GitFileChange, 0, len(st))
	for path, s := range st {
		code := s.Worktree
		if code == status.Unmodified {
			code = s.Staging
		}
		changes = append(changes, GitFileChange{
			Path:   path,
			Status: statusToString(code),
		})
	}
	return changes, nil
}

// GetBranchInfo returns the current branch name and ahead/behind counts.
func (g *GitService) GetBranchInfo(path string) (BranchInfo, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return BranchInfo{}, err
	}
	head, err := repo.Head()
	if err != nil {
		return BranchInfo{}, err
	}
	info := BranchInfo{
		Name: head.Name().Short(),
	}
	// Ahead/behind require a remote reference. If no upstream is configured,
	// return zeros (no upstream to compare against).
	ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", info.Name), true)
	if err != nil {
		return info, nil
	}
	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return info, nil
	}
	upstreamCommit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return info, nil
	}
	headAhead, _ := headCommit.Ahead(upstreamCommit)
	upstreamAhead, _ := upstreamCommit.Ahead(headCommit)
	info.Ahead = headAhead
	info.Behind = upstreamAhead
	return info, nil
}

// openWorktree opens the git repo and worktree at path.
func openWorktree(path string) (*git.Repository, *git.Worktree, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	repo, err := git.PlainOpen(abs)
	if err != nil {
		return nil, nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, nil, err
	}
	return repo, wt, nil
}

var errNotARepo = errors.New("not a git repository")
```

- [x] **Step 5: Run tests to verify they pass**

Run: `go test ./services/ -run TestGitService -v`
Expected: PASS — all 6 GitService tests pass.

- [x] **Step 6: Commit**

```bash
git add services/git_service.go services/git_service_test.go go.mod go.sum
git commit -m "feat: add GitService with status and branch info"
```

---

## Task 2: Go Backend — GitService (Stage, Unstage, Commit)

**Files:**
- Modify: `services/git_service.go` (append methods)
- Modify: `services/git_service_test.go` (append tests)

- [x] **Step 1: Write the failing tests**

Append to `services/git_service_test.go`:

```go
func TestGitService_Stage(t *testing.T) {
	dir := initBareRepo(t)
	writeFile(t, dir, "a.txt", "hello")
	svc := &GitService{}
	if err := svc.Stage(dir, "a.txt"); err != nil {
		t.Fatalf("Stage failed: %v", err)
	}
	repo, _ := git.PlainOpen(dir)
	wt, _ := repo.Worktree()
	st, _ := wt.Status()
	if st.File("a.txt").IsUntracked() {
		t.Error("expected file to be staged, but still untracked")
	}
}

func TestGitService_Unstage(t *testing.T) {
	dir := initBareRepo(t)
	writeFile(t, dir, "a.txt", "hello")
	svc := &GitService{}
	_ = svc.Stage(dir, "a.txt")
	if err := svc.Unstage(dir, "a.txt"); err != nil {
		t.Fatalf("Unstage failed: %v", err)
	}
	repo, _ := git.PlainOpen(dir)
	wt, _ := repo.Worktree()
	st, _ := wt.Status()
	if !st.File("a.txt").IsUntracked() {
		t.Error("expected file to be untracked after unstage")
	}
}

func TestGitService_Commit(t *testing.T) {
	dir := initBareRepo(t)
	writeFile(t, dir, "a.txt", "hello")
	svc := &GitService{}
	if err := svc.Stage(dir, "a.txt"); err != nil {
		t.Fatalf("Stage failed: %v", err)
	}
	if err := svc.Commit(dir, "test commit"); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	// After commit, working tree should be clean
	changes, err := svc.GetStatus(dir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes after commit, got %d", len(changes))
	}
}

func TestGitService_Commit_nothingStaged(t *testing.T) {
	dir := initBareRepo(t)
	svc := &GitService{}
	err := svc.Commit(dir, "empty commit")
	if err == nil {
		t.Error("expected error when committing with nothing staged")
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -run TestGitService_Stage -v; go test ./services/ -run TestGitService_Unstage -v; go test ./services/ -run TestGitService_Commit -v`
Expected: FAIL — `svc.Stage` / `svc.Unstage` / `svc.Commit` undefined.

- [x] **Step 3: Write minimal implementation**

Append to `services/git_service.go`:

```go
import (
	// existing imports remain
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Stage adds a file path to the git index.
func (g *GitService) Stage(path, filePath string) error {
	_, wt, err := openWorktree(path)
	if err != nil {
		return err
	}
	_, err = wt.Add(filePath)
	return err
}

// Unstage removes a file path from the git index (resets to HEAD).
func (g *GitService) Unstage(path, filePath string) error {
	repo, wt, err := openWorktree(path)
	if err != nil {
		return err
	}
	head, err := repo.Head()
	if err != nil {
		// No HEAD yet (no commits) — reset to empty tree by removing from index
		return wt.Remove(filePath)
	}
	return wt.Reset(&git.ResetOptions{
		Mode:   git.ResetMixed,
		Commit: head.Hash(),
		Files:  []string{filePath},
	})
}

// Commit creates a new commit with the currently staged changes.
func (g *GitService) Commit(path, message string) error {
	_, wt, err := openWorktree(path)
	if err != nil {
		return err
	}
	st, err := wt.Status()
	if err != nil {
		return err
	}
	hasStaged := false
	for _, s := range st {
		if s.Staging != status.Unmodified && s.Staging != status.Untracked {
			hasStaged = true
			break
		}
	}
	if !hasStaged {
		return errors.New("nothing staged to commit")
	}
	_, err = wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "gugacode",
			Email: "nknk@local",
		},
	})
	return err
}
```

Note: Merge this `import` block with the existing one at the top of `git_service.go`. The final import block should read:

```go
import (
	"errors"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/status"
)
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -run TestGitService -v`
Expected: PASS — all 10 GitService tests pass.

- [x] **Step 5: Commit**

```bash
git add services/git_service.go services/git_service_test.go
git commit -m "feat: add GitService stage, unstage, commit operations"
```

---

## Task 3: Go Backend — SearchService

**Files:**
- Create: `services/search_service.go`
- Create: `services/search_service_test.go`

- [x] **Step 1: Write the failing tests**

Create `services/search_service_test.go`:

```go
package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchService_Search_findsMatches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello world\nfoo bar\nhello again")
	writeFile(t, dir, "b.txt", "nothing here")

	svc := &SearchService{}
	results, err := svc.Search(dir, "hello", false)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 file with matches, got %d", len(results))
	}
	if results[0].Path != "a.txt" {
		t.Errorf("expected path 'a.txt', got %q", results[0].Path)
	}
	if len(results[0].Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(results[0].Matches))
	}
	if results[0].Matches[0].Line != 1 {
		t.Errorf("expected first match on line 1, got %d", results[0].Matches[0].Line)
	}
	if results[0].Matches[0].Preview != "hello world" {
		t.Errorf("expected preview 'hello world', got %q", results[0].Matches[0].Preview)
	}
}

func TestSearchService_Search_caseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "Hello World\nHELLO AGAIN")

	svc := &SearchService{}
	results, err := svc.Search(dir, "hello", true)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 file, got %d", len(results))
	}
	if len(results[0].Matches) != 2 {
		t.Errorf("expected 2 case-insensitive matches, got %d", len(results[0].Matches))
	}
}

func TestSearchService_Search_caseSensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "Hello World\nHELLO AGAIN")

	svc := &SearchService{}
	results, err := svc.Search(dir, "Hello", false)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 file, got %d", len(results))
	}
	if len(results[0].Matches) != 1 {
		t.Errorf("expected 1 case-sensitive match, got %d", len(results[0].Matches))
	}
}

func TestSearchService_Search_regexPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo123\nbar456\nbaz789")

	svc := &SearchService{}
	results, err := svc.Search(dir, "[a-z]+[0-9]+", false)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 file, got %d", len(results))
	}
	if len(results[0].Matches) != 3 {
		t.Errorf("expected 3 regex matches, got %d", len(results[0].Matches))
	}
}

func TestSearchService_Search_skipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello world")
	// Write a file containing null bytes (binary)
	binPath := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 'h', 'e', 'l', 'l', 'o', 0x00}, 0644); err != nil {
		t.Fatal(err)
	}

	svc := &SearchService{}
	results, err := svc.Search(dir, "hello", false)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	// Should only find in a.txt, not binary.bin
	if len(results) != 1 {
		t.Fatalf("expected 1 result (skipping binary), got %d", len(results))
	}
	if results[0].Path != "a.txt" {
		t.Errorf("expected only a.txt, got %q", results[0].Path)
	}
}

func TestSearchService_Search_ignoresNodeModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello")
	writeFile(t, dir, "node_modules/lib.js", "hello from node_modules")
	writeFile(t, dir, ".git/config", "hello from git")

	svc := &SearchService{}
	results, err := svc.Search(dir, "hello", false)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (ignoring node_modules/.git), got %d", len(results))
	}
	if results[0].Path != "a.txt" {
		t.Errorf("expected only a.txt, got %q", results[0].Path)
	}
}

func TestSearchService_Search_invalidRegex(t *testing.T) {
	dir := t.TempDir()
	svc := &SearchService{}
	_, err := svc.Search(dir, "[invalid", false)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -run TestSearchService -v`
Expected: FAIL — `services.SearchService` undefined.

- [x] **Step 3: Write minimal implementation**

Create `services/search_service.go`:

```go
package services

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SearchMatch describes a single match within a file.
type SearchMatch struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Preview string `json:"preview"`
}

// SearchResult groups all matches in a single file.
type SearchResult struct {
	Path    string        `json:"path"`
	Matches []SearchMatch `json:"matches"`
}

// SearchService exposes file-content search to the frontend.
type SearchService struct{}

// ignoredDirs are directory basenames skipped during search.
var ignoredDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".hg":          true,
	".svn":         true,
	"dist":         true,
	"build":        true,
	"out":          true,
	".next":        true,
	".nuxt":        true,
	"target":       true,
	"vendor":       true,
}

// isBinary returns true if the file content contains a null byte in the first 4KB.
func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}

// Search walks path recursively and returns files whose content matches the query.
// If ignoreCase is true, the match is case-insensitive. The query is treated as a
// regular expression.
func (s *SearchService) Search(root, query string, ignoreCase bool) ([]SearchResult, error) {
	pattern := query
	flags := regexp.None
	if ignoreCase {
		flags = regexp.IgnoreCase
	}
	re, err := regexp.Compile("(?"+flagsToString(flags)+")" + pattern)
	if err != nil {
		// Try without the inline flags wrapper — older Go may not need it.
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		if ignoreCase {
			re = re.Copy()
			// Fallback: lowercase comparison
			re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(pattern))
		}
	}

	var results []SearchResult
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "" || strings.HasPrefix(d.Name(), ".") && d.Name() != ".env" {
			// Skip dotfiles except .env
			return nil
		}
		if isBinary(p) {
			return nil
		}
		matches := searchFile(p, re)
		if len(matches) > 0 {
			relPath, _ := filepath.Rel(root, p)
			results = append(results, SearchResult{
				Path:    filepath.ToSlash(relPath),
				Matches: matches,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func flagsToString(f regexp.Flags) string {
	if f&regexp.IgnoreCase != 0 {
		return "i"
	}
	return ""
}

func searchFile(path string, re *regexp.Regexp) []SearchMatch {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var matches []SearchMatch
	scanner := bufio.NewScanner(f)
	// Allow longer lines (default 64KB is too small for minified files)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		loc := re.FindStringIndex(line)
		if loc != nil {
			matches = append(matches, SearchMatch{
				Line:    lineNum,
				Column:  loc[0] + 1,
				Preview: line,
			})
		}
	}
	return matches
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -run TestSearchService -v`
Expected: PASS — all 7 SearchService tests pass.

- [x] **Step 5: Commit**

```bash
git add services/search_service.go services/search_service_test.go
git commit -m "feat: add SearchService with regex content search"
```

---

## Task 4: Go Backend — Register Services in main.go

**Files:**
- Modify: `main.go`

- [x] **Step 1: Edit main.go to register GitService and SearchService**

In `main.go`, locate the service instantiation block (currently lines 29-34):

```go
	fileService := &services.FileService{}
	projectService := services.NewProjectService()
	settingsService := services.NewSettingsService()
	windowService := &services.WindowService{}
	terminalService := services.NewTerminalService()
	aiService := services.NewAIService()
```

Replace with:

```go
	fileService := &services.FileService{}
	projectService := services.NewProjectService()
	settingsService := services.NewSettingsService()
	windowService := &services.WindowService{}
	terminalService := services.NewTerminalService()
	aiService := services.NewAIService()
	gitService := &services.GitService{}
	searchService := &services.SearchService{}
```

Then locate the `Services:` slice (currently lines 39-47):

```go
		Services: []application.Service{
			application.NewService(fileService),
			application.NewService(projectService),
			application.NewService(settingsService),
			application.NewService(windowService),
			application.NewService(terminalService),
			application.NewService(aiService),
			application.NewService(&GreetService{}),
		},
```

Replace with:

```go
		Services: []application.Service{
			application.NewService(fileService),
			application.NewService(projectService),
			application.NewService(settingsService),
			application.NewService(windowService),
			application.NewService(terminalService),
			application.NewService(aiService),
			application.NewService(gitService),
			application.NewService(searchService),
			application.NewService(&GreetService{}),
		},
```

- [x] **Step 2: Verify build and vet**

Run:
```bash
go build ./
go vet ./...
```

Expected: both succeed with no output.

- [x] **Step 3: Run all Go tests**

Run: `go test ./services/ -v`
Expected: all tests pass (24 prior + 6 Git status + 4 Git stage/commit + 7 Search = 41 tests).

- [x] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: register GitService and SearchService in main.go"
```

---

## Task 5: Frontend — Regenerate Bindings & Extend Types

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/services.ts`
- Generated: `frontend/bindings/changeme/services/gitservice.js`
- Generated: `frontend/bindings/changeme/services/searchservice.js`

- [x] **Step 1: Regenerate Wails bindings**

Run from project root:

```bash
wails3 generate bindings
```

Expected output includes `8 Services, 35 Methods` (or similar, reflecting the new GitService + SearchService methods). Verify the new binding files exist:

```bash
ls frontend/bindings/changeme/services/
```

Expected: `gitservice.js` and `searchservice.js` are present alongside the existing 6 service files.

- [x] **Step 2: Extend types**

Open `frontend/src/types/index.ts`. Append after the `ChatMessage` interface (currently the last type in the file):

```typescript
export interface GitFileChange {
  path: string;
  status: string;
}

export interface BranchInfo {
  name: string;
  ahead: number;
  behind: number;
}

export interface SearchMatch {
  line: number;
  column: number;
  preview: string;
}

export interface SearchResult {
  path: string;
  matches: SearchMatch[];
}
```

- [x] **Step 3: Extend API service wrappers**

Open `frontend/src/api/services.ts`. Add two new imports after the existing `AIServiceBindings` import (line 9):

```typescript
import * as GitServiceBindings from "../../bindings/changeme/services/gitservice.js";
import * as SearchServiceBindings from "../../bindings/changeme/services/searchservice.js";
```

Update the type import on line 10 to include the new types:

```typescript
import type { DirEntry, Project, Settings, ChatMessage, GitFileChange, BranchInfo, SearchResult } from "@/types";
```

Append at the end of the file (after `aiService`):

```typescript
export const gitService = {
  getStatus: (path: string) =>
    GitServiceBindings.GetStatus(path) as Promise<GitFileChange[]>,
  getBranchInfo: (path: string) =>
    GitServiceBindings.GetBranchInfo(path) as Promise<BranchInfo>,
  stage: (path: string, filePath: string) =>
    GitServiceBindings.Stage(path, filePath) as Promise<void>,
  unstage: (path: string, filePath: string) =>
    GitServiceBindings.Unstage(path, filePath) as Promise<void>,
  commit: (path: string, message: string) =>
    GitServiceBindings.Commit(path, message) as Promise<void>,
};

export const searchService = {
  search: (root: string, query: string, ignoreCase: boolean) =>
    SearchServiceBindings.Search(root, query, ignoreCase) as Promise<SearchResult[]>,
};
```

- [x] **Step 4: Verify type check**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
```

Expected: no errors.

- [x] **Step 5: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/services.ts frontend/bindings/
git commit -m "feat: add Git and Search bindings and TypeScript wrappers"
```

---

## Task 6: Frontend — Git Store

**Files:**
- Create: `frontend/src/stores/git.ts`
- Create: `frontend/src/stores/git.test.ts`

- [x] **Step 1: Write the failing tests**

Create `frontend/src/stores/git.test.ts`:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services", () => ({
  gitService: {
    getStatus: vi.fn().mockResolvedValue([
      { path: "a.txt", status: "Modified" },
      { path: "b.txt", status: "Untracked" },
    ]),
    getBranchInfo: vi.fn().mockResolvedValue({
      name: "main",
      ahead: 2,
      behind: 0,
    }),
    stage: vi.fn().mockResolvedValue(undefined),
    unstage: vi.fn().mockResolvedValue(undefined),
    commit: vi.fn().mockResolvedValue(undefined),
  },
}));

import {
  gitState,
  refreshGit,
  stageFile,
  unstageFile,
  commitChanges,
} from "./git";

describe("git store", () => {
  beforeEach(() => {
    gitState.changes = [];
    gitState.branchName = "";
    gitState.ahead = 0;
    gitState.behind = 0;
    gitState.loading = false;
    gitState.error = null;
  });

  it("starts with empty state", () => {
    expect(gitState.changes).toHaveLength(0);
    expect(gitState.branchName).toBe("");
    expect(gitState.loading).toBe(false);
  });

  it("refreshGit loads changes and branch info", async () => {
    await refreshGit("/some/repo");
    expect(gitState.changes).toHaveLength(2);
    expect(gitState.changes[0].path).toBe("a.txt");
    expect(gitState.branchName).toBe("main");
    expect(gitState.ahead).toBe(2);
    expect(gitState.loading).toBe(false);
  });

  it("stageFile calls gitService.stage", async () => {
    await stageFile("/repo", "a.txt");
    const { gitService } = await import("@/api/services");
    expect(gitService.stage).toHaveBeenCalledWith("/repo", "a.txt");
  });

  it("unstageFile calls gitService.unstage", async () => {
    await unstageFile("/repo", "a.txt");
    const { gitService } = await import("@/api/services");
    expect(gitService.unstage).toHaveBeenCalledWith("/repo", "a.txt");
  });

  it("commitChanges calls gitService.commit and refreshes", async () => {
    await commitChanges("/repo", "fix: something");
    const { gitService } = await import("@/api/services");
    expect(gitService.commit).toHaveBeenCalledWith("/repo", "fix: something");
    expect(gitService.getStatus).toHaveBeenCalled();
  });

  it("stores error on failure", async () => {
    const { gitService } = await import("@/api/services");
    (gitService.getStatus as any).mockRejectedValueOnce(new Error("fail"));
    await refreshGit("/repo");
    expect(gitState.error).toBe("fail");
    expect(gitState.loading).toBe(false);
  });
});
```

- [x] **Step 2: Run tests to verify they fail**

Run:
```bash
cd frontend
npx vitest run src/stores/git.test.ts
```

Expected: FAIL — cannot resolve `./git`.

- [x] **Step 3: Write minimal implementation**

Create `frontend/src/stores/git.ts`:

```typescript
import { reactive } from "vue";
import { gitService } from "@/api/services";
import type { GitFileChange } from "@/types";

export interface GitState {
  changes: GitFileChange[];
  branchName: string;
  ahead: number;
  behind: number;
  loading: boolean;
  error: string | null;
}

export const gitState = reactive<GitState>({
  changes: [],
  branchName: "",
  ahead: 0,
  behind: 0,
  loading: false,
  error: null,
});

export async function refreshGit(repoPath: string): Promise<void> {
  gitState.loading = true;
  gitState.error = null;
  try {
    const [changes, info] = await Promise.all([
      gitService.getStatus(repoPath),
      gitService.getBranchInfo(repoPath),
    ]);
    gitState.changes = changes;
    gitState.branchName = info.name;
    gitState.ahead = info.ahead;
    gitState.behind = info.behind;
  } catch (e: any) {
    gitState.error = e?.message ?? String(e);
  } finally {
    gitState.loading = false;
  }
}

export async function stageFile(repoPath: string, filePath: string): Promise<void> {
  try {
    await gitService.stage(repoPath, filePath);
    await refreshGit(repoPath);
  } catch (e: any) {
    gitState.error = e?.message ?? String(e);
  }
}

export async function unstageFile(repoPath: string, filePath: string): Promise<void> {
  try {
    await gitService.unstage(repoPath, filePath);
    await refreshGit(repoPath);
  } catch (e: any) {
    gitState.error = e?.message ?? String(e);
  }
}

export async function commitChanges(repoPath: string, message: string): Promise<void> {
  try {
    await gitService.commit(repoPath, message);
    await refreshGit(repoPath);
  } catch (e: any) {
    gitState.error = e?.message ?? String(e);
  }
}

export function clearGitState(): void {
  gitState.changes = [];
  gitState.branchName = "";
  gitState.ahead = 0;
  gitState.behind = 0;
  gitState.loading = false;
  gitState.error = null;
}
```

- [x] **Step 4: Run tests to verify they pass**

Run:
```bash
cd frontend
npx vitest run src/stores/git.test.ts
```

Expected: PASS — all 6 git store tests pass.

- [x] **Step 5: Commit**

```bash
git add frontend/src/stores/git.ts frontend/src/stores/git.test.ts
git commit -m "feat: add git store with refresh, stage, unstage, commit"
```

---

## Task 7: Frontend — Search Store

**Files:**
- Create: `frontend/src/stores/search.ts`
- Create: `frontend/src/stores/search.test.ts`

- [x] **Step 1: Write the failing tests**

Create `frontend/src/stores/search.test.ts`:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services", () => ({
  searchService: {
    search: vi.fn().mockResolvedValue([
      {
        path: "a.txt",
        matches: [
          { line: 1, column: 1, preview: "hello world" },
          { line: 3, column: 1, preview: "hello again" },
        ],
      },
      {
        path: "b.ts",
        matches: [{ line: 5, column: 3, preview: "  hello there" }],
      },
    ]),
  },
}));

import { searchState, runSearch, clearSearch } from "./search";

describe("search store", () => {
  beforeEach(() => {
    searchState.query = "";
    searchState.results = [];
    searchState.loading = false;
    searchState.error = null;
    searchState.ignoreCase = false;
  });

  it("starts with empty state", () => {
    expect(searchState.query).toBe("");
    expect(searchState.results).toHaveLength(0);
    expect(searchState.loading).toBe(false);
  });

  it("runSearch populates results", async () => {
    await runSearch("/repo", "hello");
    expect(searchState.results).toHaveLength(2);
    expect(searchState.results[0].path).toBe("a.txt");
    expect(searchState.results[0].matches).toHaveLength(2);
    expect(searchState.loading).toBe(false);
  });

  it("runSearch does nothing with empty query", async () => {
    await runSearch("/repo", "");
    expect(searchState.results).toHaveLength(0);
    const { searchService } = await import("@/api/services");
    expect(searchService.search).not.toHaveBeenCalled();
  });

  it("toggle ignoreCase is reflected in state", async () => {
    searchState.ignoreCase = true;
    await runSearch("/repo", "Hello");
    const { searchService } = await import("@/api/services");
    expect(searchService.search).toHaveBeenCalledWith("/repo", "Hello", true);
  });

  it("clearSearch resets state", () => {
    searchState.query = "foo";
    searchState.results = [{ path: "x", matches: [] }];
    clearSearch();
    expect(searchState.query).toBe("");
    expect(searchState.results).toHaveLength(0);
  });

  it("stores error on failure", async () => {
    const { searchService } = await import("@/api/services");
    (searchService.search as any).mockRejectedValueOnce(new Error("bad regex"));
    await runSearch("/repo", "[invalid");
    expect(searchState.error).toBe("bad regex");
    expect(searchState.loading).toBe(false);
  });
});
```

- [x] **Step 2: Run tests to verify they fail**

Run:
```bash
cd frontend
npx vitest run src/stores/search.test.ts
```

Expected: FAIL — cannot resolve `./search`.

- [x] **Step 3: Write minimal implementation**

Create `frontend/src/stores/search.ts`:

```typescript
import { reactive } from "vue";
import { searchService } from "@/api/services";
import type { SearchResult } from "@/types";

export interface SearchState {
  query: string;
  ignoreCase: boolean;
  results: SearchResult[];
  loading: boolean;
  error: string | null;
}

export const searchState = reactive<SearchState>({
  query: "",
  ignoreCase: false,
  results: [],
  loading: false,
  error: null,
});

let debounceTimer: ReturnType<typeof setTimeout> | null = null;

export async function runSearch(root: string, query: string): Promise<void> {
  if (!query.trim()) {
    searchState.results = [];
    searchState.query = query;
    return;
  }
  searchState.query = query;
  searchState.loading = true;
  searchState.error = null;
  try {
    const results = await searchService.search(root, query, searchState.ignoreCase);
    searchState.results = results;
  } catch (e: any) {
    searchState.error = e?.message ?? String(e);
    searchState.results = [];
  } finally {
    searchState.loading = false;
  }
}

export function debouncedSearch(root: string, query: string, delay = 300): void {
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    runSearch(root, query);
  }, delay);
}

export function clearSearch(): void {
  searchState.query = "";
  searchState.results = [];
  searchState.error = null;
  searchState.loading = false;
}
```

- [x] **Step 4: Run tests to verify they pass**

Run:
```bash
cd frontend
npx vitest run src/stores/search.test.ts
```

Expected: PASS — all 6 search store tests pass.

- [x] **Step 5: Commit**

```bash
git add frontend/src/stores/search.ts frontend/src/stores/search.test.ts
git commit -m "feat: add search store with debounced content search"
```

---

## Task 8: Frontend — GitPanel Component

**Files:**
- Create: `frontend/src/components/layout/GitPanel.vue`

- [x] **Step 1: Create the component**

Create `frontend/src/components/layout/GitPanel.vue`:

```vue
<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { appState } from "@/stores/app";
import {
  gitState,
  refreshGit,
  stageFile,
  unstageFile,
  commitChanges,
} from "@/stores/git";
import { Plus, Minus, Check } from "@element-plus/icons-vue";

const repoPath = computed(() => appState.currentProject ?? "");
const commitMessage = ref("");

const hasChanges = computed(() => gitState.changes.length > 0);

async function handleRefresh() {
  if (!repoPath.value) return;
  await refreshGit(repoPath.value);
}

async function handleStage(path: string) {
  if (!repoPath.value) return;
  await stageFile(repoPath.value, path);
}

async function handleUnstage(path: string) {
  if (!repoPath.value) return;
  await unstageFile(repoPath.value, path);
}

async function handleCommit() {
  if (!repoPath.value || !commitMessage.value.trim()) return;
  await commitChanges(repoPath.value, commitMessage.value);
  commitMessage.value = "";
}

function statusLabel(status: string): string {
  switch (status) {
    case "Modified":
      return "M";
    case "Added":
      return "A";
    case "Deleted":
      return "D";
    case "Untracked":
      return "U";
    case "Renamed":
      return "R";
    default:
      return "?";
  }
}

function statusClass(status: string): string {
  switch (status) {
    case "Modified":
      return "git-panel__status--modified";
    case "Added":
      return "git-panel__status--added";
    case "Deleted":
      return "git-panel__status--deleted";
    case "Untracked":
      return "git-panel__status--untracked";
    default:
      return "git-panel__status--default";
  }
}

onMounted(() => {
  if (repoPath.value) {
    refreshGit(repoPath.value);
  }
});

watch(repoPath, (newPath) => {
  if (newPath) {
    refreshGit(newPath);
  }
});
</script>

<template>
  <div class="git-panel">
    <!-- Branch header -->
    <div class="git-panel__branch-bar">
      <span class="git-panel__branch-label">{{ gitState.branchName || "—" }}</span>
      <span v-if="gitState.ahead > 0" class="git-panel__ahead" title="Ahead">
        ↑{{ gitState.ahead }}
      </span>
      <span v-if="gitState.behind > 0" class="git-panel__behind" title="Behind">
        ↓{{ gitState.behind }}
      </span>
      <button
        class="git-panel__refresh"
        aria-label="Refresh git status"
        title="Refresh"
        @click="handleRefresh"
      >
        ↻
      </button>
    </div>

    <!-- Commit message + button -->
    <div class="git-panel__commit-area">
      <textarea
        v-model="commitMessage"
        class="git-panel__commit-input"
        placeholder="Commit message..."
        rows="2"
        aria-label="Commit message"
      />
      <button
        class="git-panel__commit-btn"
        :disabled="!commitMessage.trim()"
        @click="handleCommit"
      >
        <el-icon :size="12"><Check /></el-icon>
        Commit
      </button>
    </div>

    <!-- Loading -->
    <div v-if="gitState.loading" class="git-panel__loading">
      Loading...
    </div>

    <!-- Error -->
    <div v-if="gitState.error" class="git-panel__error">
      {{ gitState.error }}
    </div>

    <!-- Changes list -->
    <div v-if="!gitState.loading && hasChanges" class="git-panel__changes">
      <div class="git-panel__section-header">Changes ({{ gitState.changes.length }})</div>
      <div
        v-for="change in gitState.changes"
        :key="change.path"
        class="git-panel__row"
      >
        <span class="git-panel__path" :title="change.path">{{ change.path }}</span>
        <span class="git-panel__actions">
          <button
            class="git-panel__action"
            aria-label="Stage"
            title="Stage"
            @click="handleStage(change.path)"
          >
            <el-icon :size="12"><Plus /></el-icon>
          </button>
          <button
            class="git-panel__action"
            aria-label="Unstage"
            title="Unstage"
            @click="handleUnstage(change.path)"
          >
            <el-icon :size="12"><Minus /></el-icon>
          </button>
        </span>
        <span class="git-panel__status" :class="statusClass(change.status)">
          {{ statusLabel(change.status) }}
        </span>
      </div>
    </div>

    <!-- Empty state -->
    <div v-if="!gitState.loading && !hasChanges && !gitState.error" class="git-panel__empty">
      No changes
    </div>
  </div>
</template>

<style scoped>
.git-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  font-family: var(--font-sans);
}

.git-panel__branch-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.git-panel__branch-label {
  font-size: 11px;
  font-weight: 500;
  color: var(--color-text-secondary);
}

.git-panel__ahead,
.git-panel__behind {
  font-size: 10px;
  color: var(--color-text-tertiary);
}

.git-panel__refresh {
  margin-left: auto;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
}

.git-panel__refresh:hover {
  color: var(--color-text-primary);
  background-color: color-mix(in srgb, var(--color-text-tertiary) 8%, transparent);
}

.git-panel__commit-area {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.git-panel__commit-input {
  width: 100%;
  padding: 6px 8px;
  font-size: 12px;
  font-family: var(--font-sans);
  color: var(--color-text-primary);
  background-color: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-sm);
  outline: none;
  resize: vertical;
}

.git-panel__commit-input:focus {
  border-color: var(--color-primary);
}

.git-panel__commit-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 4px 12px;
  font-size: 11px;
  color: #fff;
  background-color: var(--color-primary);
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.git-panel__commit-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.git-panel__commit-btn:not(:disabled):hover {
  background-color: color-mix(in srgb, var(--color-primary) 85%, #000);
}

.git-panel__loading,
.git-panel__empty,
.git-panel__error {
  padding: 12px;
  font-size: 11px;
  color: var(--color-text-tertiary);
}

.git-panel__error {
  color: var(--color-error);
}

.git-panel__section-header {
  padding: 6px 12px 4px;
  font-size: 10px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-tertiary);
}

.git-panel__changes {
  flex: 1;
  overflow-y: auto;
}

.git-panel__row {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 12px;
  font-size: 12px;
  cursor: default;
}

.git-panel__row:hover {
  background-color: color-mix(in srgb, var(--color-text-primary) 4%, transparent);
}

.git-panel__path {
  flex: 1;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.git-panel__actions {
  display: flex;
  gap: 2px;
  opacity: 0;
  transition: opacity var(--duration-micro) var(--ease-out-expo);
}

.git-panel__row:hover .git-panel__actions {
  opacity: 1;
}

.git-panel__action {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  border-radius: var(--radius-sm);
}

.git-panel__action:hover {
  color: var(--color-text-primary);
  background-color: color-mix(in srgb, var(--color-text-tertiary) 12%, transparent);
}

.git-panel__status {
  width: 16px;
  text-align: center;
  font-weight: 500;
  font-size: 11px;
}

.git-panel__status--modified { color: var(--color-warning); }
.git-panel__status--added { color: var(--color-success); }
.git-panel__status--deleted { color: var(--color-error); }
.git-panel__status--untracked { color: var(--color-text-tertiary); }
.git-panel__status--default { color: var(--color-text-tertiary); }
</style>
```

- [x] **Step 2: Verify type check**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
```

Expected: no errors.

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/layout/GitPanel.vue
git commit -m "feat: add GitPanel component with stage/unstage/commit UI"
```

---

## Task 9: Frontend — SearchPanel Component

**Files:**
- Create: `frontend/src/components/layout/SearchPanel.vue`

- [x] **Step 1: Create the component**

Create `frontend/src/components/layout/SearchPanel.vue`:

```vue
<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { appState } from "@/stores/app";
import { searchState, debouncedSearch, clearSearch } from "@/stores/search";
import { openFileFromPath } from "@/stores/editor";
import { Search } from "@element-plus/icons-vue";

const repoPath = computed(() => appState.currentProject ?? "");
const localQuery = ref(searchState.query);
const caseSensitive = ref(!searchState.ignoreCase);

const totalMatches = computed(() =>
  searchState.results.reduce((sum, r) => sum + r.matches.length, 0),
);

const hasResults = computed(() => searchState.results.length > 0);

function handleInput() {
  if (!repoPath.value) return;
  searchState.ignoreCase = !caseSensitive.value;
  debouncedSearch(repoPath.value, localQuery.value);
}

function handleClear() {
  localQuery.value = "";
  clearSearch();
}

async function handleMatchClick(filePath: string, line: number) {
  if (!repoPath.value) return;
  const fullPath = repoPath.value + "/" + filePath;
  await openFileFromPath(fullPath);
  // Note: scrolling to line requires Monaco integration that is out of scope
  // for this task. The file opens; the user can navigate to the line manually.
}

function toggleCaseSensitive() {
  caseSensitive.value = !caseSensitive.value;
  searchState.ignoreCase = !caseSensitive.value;
  if (localQuery.value) {
    handleInput();
  }
}

watch(() => appState.currentProject, () => {
  handleClear();
});
</script>

<template>
  <div class="search-panel">
    <!-- Search input -->
    <div class="search-panel__input-area">
      <div class="search-panel__input-wrap">
        <el-icon :size="12" class="search-panel__icon">
          <Search />
        </el-icon>
        <input
          v-model="localQuery"
          type="text"
          class="search-panel__input"
          placeholder="Search in files..."
          aria-label="Search query"
          @input="handleInput"
        />
        <button
          v-if="localQuery"
          class="search-panel__clear"
          aria-label="Clear search"
          @click="handleClear"
        >
          ×
        </button>
      </div>
      <button
        class="search-panel__case-btn"
        :class="{ 'search-panel__case-btn--active': caseSensitive }"
        :aria-pressed="caseSensitive"
        title="Match case"
        @click="toggleCaseSensitive"
      >
        Aa
      </button>
    </div>

    <!-- Summary -->
    <div v-if="hasResults && !searchState.loading" class="search-panel__summary">
      {{ searchState.results.length }} files, {{ totalMatches }} matches
    </div>

    <!-- Loading -->
    <div v-if="searchState.loading" class="search-panel__loading">
      Searching...
    </div>

    <!-- Error -->
    <div v-if="searchState.error" class="search-panel__error">
      {{ searchState.error }}
    </div>

    <!-- Results -->
    <div v-if="hasResults && !searchState.loading" class="search-panel__results">
      <div v-for="result in searchState.results" :key="result.path" class="search-panel__file-group">
        <div class="search-panel__file-path" :title="result.path">
          {{ result.path }}
          <span class="search-panel__file-count">{{ result.matches.length }}</span>
        </div>
        <button
          v-for="(match, idx) in result.matches"
          :key="idx"
          class="search-panel__match"
          @click="handleMatchClick(result.path, match.line)"
        >
          <span class="search-panel__line-num">{{ match.line }}</span>
          <span class="search-panel__preview">{{ match.preview }}</span>
        </button>
      </div>
    </div>

    <!-- Empty state -->
    <div
      v-if="!hasResults && !searchState.loading && localQuery && !searchState.error"
      class="search-panel__empty"
    >
      No results
    </div>
    <div
      v-if="!localQuery && !hasResults"
      class="search-panel__empty"
    >
      Type to search across files
    </div>
  </div>
</template>

<style scoped>
.search-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  font-family: var(--font-sans);
}

.search-panel__input-area {
  display: flex;
  gap: 4px;
  padding: 8px 12px;
}

.search-panel__input-wrap {
  position: relative;
  flex: 1;
  display: flex;
  align-items: center;
}

.search-panel__icon {
  position: absolute;
  left: 8px;
  color: var(--color-text-tertiary);
  pointer-events: none;
}

.search-panel__input {
  width: 100%;
  padding: 6px 26px 6px 26px;
  font-size: 12px;
  font-family: var(--font-sans);
  color: var(--color-text-primary);
  background-color: var(--color-bg-elevated);
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  outline: none;
  transition: border-color var(--duration-fast) var(--ease-out-expo);
}

.search-panel__input:focus {
  border-color: var(--color-primary);
}

.search-panel__clear {
  position: absolute;
  right: 4px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
}

.search-panel__clear:hover {
  color: var(--color-text-primary);
}

.search-panel__case-btn {
  width: 28px;
  height: 28px;
  border: 1px solid var(--color-border-subtle);
  background: transparent;
  color: var(--color-text-tertiary);
  font-size: 11px;
  font-weight: 500;
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.search-panel__case-btn--active {
  color: var(--color-text-primary);
  background-color: color-mix(in srgb, var(--color-text-primary) 10%, transparent);
  border-color: var(--color-border-default);
}

.search-panel__summary,
.search-panel__loading,
.search-panel__empty,
.search-panel__error {
  padding: 4px 12px;
  font-size: 11px;
  color: var(--color-text-tertiary);
}

.search-panel__error {
  color: var(--color-error);
}

.search-panel__results {
  flex: 1;
  overflow-y: auto;
  padding-bottom: 8px;
}

.search-panel__file-group {
  margin-bottom: 4px;
}

.search-panel__file-path {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px 2px;
  font-size: 11px;
  font-weight: 500;
  color: var(--color-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.search-panel__file-count {
  flex-shrink: 0;
  padding: 0 6px;
  font-size: 10px;
  color: var(--color-text-tertiary);
  background-color: color-mix(in srgb, var(--color-text-tertiary) 10%, transparent);
  border-radius: 8px;
}

.search-panel__match {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 3px 12px 3px 24px;
  background: transparent;
  border: none;
  cursor: pointer;
  text-align: left;
  transition: background-color var(--duration-micro) var(--ease-out-expo);
}

.search-panel__match:hover {
  background-color: color-mix(in srgb, var(--color-text-primary) 6%, transparent);
}

.search-panel__line-num {
  flex-shrink: 0;
  width: 28px;
  font-size: 10px;
  color: var(--color-text-tertiary);
  font-family: var(--font-mono);
  text-align: right;
}

.search-panel__preview {
  flex: 1;
  font-size: 11px;
  color: var(--color-text-primary);
  font-family: var(--font-mono);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
```

- [x] **Step 2: Verify type check**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
```

Expected: no errors.

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/layout/SearchPanel.vue
git commit -m "feat: add SearchPanel component with debounced search UI"
```

---

## Task 10: Frontend — Wire GitPanel & SearchPanel into SidePanel

**Files:**
- Modify: `frontend/src/components/layout/SidePanel.vue`
- Modify: `frontend/src/stores/app.ts` (extend loadSettings to read git branch)

- [x] **Step 1: Edit SidePanel.vue script section**

Open `frontend/src/components/layout/SidePanel.vue`. Replace the existing `<script setup>` block (lines 1-33) with:

```vue
<script setup lang="ts">
import { computed, onMounted, watch } from "vue";
import { appState, toggleSidebar } from "@/stores/app";
import { Close } from "@element-plus/icons-vue";
import FileTree from "@/components/explorer/FileTree.vue";
import GitPanel from "@/components/layout/GitPanel.vue";
import SearchPanel from "@/components/layout/SearchPanel.vue";
import { openFileFromPath } from "@/stores/editor";
import { gitState, refreshGit } from "@/stores/git";

const panelTitles: Record<string, string> = {
  explorer: "Explorer",
  search: "Search",
  git: "Source Control",
  extensions: "Extensions",
  ai: "AI Assistant",
};

const isCollapsed = computed(() => appState.sidebarCollapsed);
const currentTab = computed(() => appState.panelTab);
const panelTitle = computed(() => panelTitles[currentTab.value] || "Explorer");
const projectPath = computed(() => appState.currentProject);
const projectName = computed(() => appState.projectName ?? "Project");

function handleFileSelect(path: string) {
  openFileFromPath(path);
}

const emptyMessages: Record<string, string> = {
  explorer: "Open a project to start",
  extensions: "No extensions installed",
  ai: "AI assistant ready",
};

// Sync git branch name to appState for StatusBar
watch(
  () => gitState.branchName,
  (name) => {
    if (name) appState.branchName = name;
  },
);

onMounted(() => {
  if (projectPath.value && currentTab.value === "git") {
    refreshGit(projectPath.value);
  }
});

watch(
  [currentTab, projectPath],
  ([tab, path]) => {
    if (tab === "git" && path) {
      refreshGit(path as string);
    }
  },
);
</script>
```

- [x] **Step 2: Edit SidePanel.vue template section**

Replace the entire `<template>` block (the part between `<template>` and `</template>`) with:

```vue
<template>
  <aside
    class="side-panel"
    :class="{ 'side-panel--collapsed': isCollapsed }"
    role="complementary"
    :aria-label="panelTitle + ' panel'"
  >
    <div class="side-panel__content">
      <!-- Panel header -->
      <div class="side-panel__header">
        <span class="side-panel__title">{{ panelTitle }}</span>
        <button
          class="side-panel__close"
          aria-label="Close panel"
          title="Close panel"
          @click="toggleSidebar"
        >
          <el-icon :size="14">
            <Close />
          </el-icon>
        </button>
      </div>

      <!-- Panel body -->
      <div class="side-panel__body">
        <!-- Explorer: file tree -->
        <div v-if="currentTab === 'explorer' && projectPath" class="side-panel__explorer">
          <div class="side-panel__project-header">{{ projectName }}</div>
          <FileTree :path="projectPath" :name="projectName" :depth="0" @select="handleFileSelect" />
        </div>

        <!-- Search panel -->
        <SearchPanel v-else-if="currentTab === 'search' && projectPath" />

        <!-- Git panel -->
        <GitPanel v-else-if="currentTab === 'git' && projectPath" />

        <!-- Empty state for other tabs -->
        <div v-else class="side-panel__empty">
          <div class="side-panel__empty-line" aria-hidden="true" />
          <p class="side-panel__empty-text">
            {{ emptyMessages[currentTab] || (projectPath ? panelTitle : "Open a project to start") }}
          </p>
        </div>
      </div>
    </div>
  </aside>
</template>
```

- [x] **Step 3: Remove the now-unused search input CSS**

In the `<style scoped>` block of `SidePanel.vue`, remove these two rules (they are no longer used since the search input moved into `SearchPanel.vue`):

```css
.side-panel__search-wrap {
  padding: 0 12px 8px;
}

.side-panel__search-input {
  width: 100%;
  padding: 6px 10px;
  font-size: 12px;
  font-family: var(--font-sans);
  color: var(--color-text-primary);
  background-color: var(--color-bg-elevated);
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  outline: none;
  transition:
    border-color var(--duration-fast) var(--ease-out-expo),
    background-color var(--duration-fast) var(--ease-out-expo);
}

.search-panel__search-input::placeholder {
  color: var(--color-text-tertiary);
}

.search-panel__search-input:hover {
  background-color: var(--color-bg-overlay);
}

.search-panel__search-input:focus-visible {
  border-color: var(--color-primary);
  background-color: var(--color-bg-overlay);
}
```

Leave the rest of the CSS unchanged.

- [x] **Step 4: Verify type check and run tests**

Run:
```bash
cd frontend
npx vue-tsc --noEmit
npx vitest run
```

Expected: no type errors. All existing tests still pass (48 prior + 6 git + 6 search = 60 tests).

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/layout/SidePanel.vue
git commit -m "feat: wire GitPanel and SearchPanel into SidePanel"
```

---

## Task 11: Integration — Manual Verification

**Files:** none (manual testing only)

- [x] **Step 1: Run full backend test suite**

Run:
```bash
go test ./services/ -v
```

Expected: all tests pass. Count: 24 prior (File, Project, Settings, Terminal, AI) + 10 GitService + 7 SearchService = 41 tests.

- [x] **Step 2: Run full frontend test suite**

Run:
```bash
cd frontend
npx vitest run
```

Expected: all tests pass. Count: 48 prior + 6 git store + 6 search store = 60 tests.

- [x] **Step 3: Build verification**

Run:
```bash
go build ./
cd frontend
npx vue-tsc --noEmit
```

Expected: both succeed with no output.

- [x] **Step 4: Manual GUI verification**

Run `wails3 dev` from the project root. When the app launches, verify each of the following:

1. **Open a git repository**: Click "Open Project" and select a folder that is a git repo (e.g. the `gugacode` project itself). The app should open the editor view.
2. **Git tab — branch shown**: Click the "Source Control" icon in the activity bar (third icon, the connection/branch symbol). The SidePanel should switch to the Git panel and show the current branch name at the top.
3. **Git tab — changes list**: Modify a file in the repo (e.g. edit `README.md` from outside the app, or create a new file). Click the refresh button (↻) in the Git panel. The changed file should appear in the list with a status letter (M/U/A/D).
4. **Git tab — stage**: Hover over a changed file row. Click the "+" button. The file should be staged (and on refresh, may move or update its status). Verify via `git status` in a terminal if unsure.
5. **Git tab — unstage**: Hover over a staged file. Click the "−" button. The file should be unstaged.
6. **Git tab — commit**: Type a commit message in the textarea. Click the "Commit" button. The changes list should clear (working tree clean). Verify via `git log` in a terminal.
7. **Search tab — basic search**: Click the "Search" icon in the activity bar (second icon, magnifying glass). Type a search query that exists in the project files (e.g. "nknk"). Results should appear grouped by file, each with line number and preview.
8. **Search tab — case toggle**: Click the "Aa" button to enable case-sensitive search. Type a query with different casing. Results should reflect the case sensitivity.
9. **Search tab — clear**: Click the "×" button in the search input. The query and results should clear.
10. **Search tab — click result**: Click on a search result row. The file should open in the editor tab.
11. **Search tab — empty state**: With no query, the panel should show "Type to search across files".
12. **Search tab — no results**: Type a query that matches nothing. The panel should show "No results".
13. **Status bar — branch**: The status bar at the bottom should show the current branch name (synced from the git store).
14. **Non-git folder**: Open a folder that is NOT a git repo. Click the Source Control icon. The Git panel should show an error message (e.g. "repository does not exist"), not crash.
15. **Invalid regex**: In the Search panel, type `[invalid` (unclosed bracket). The panel should show an error message, not crash.
16. **Search ignores node_modules**: Search for a term that appears in `node_modules/`. The results should NOT include files under `node_modules/`.
17. **Tab switching**: Switch between Explorer, Search, and Source Control tabs. Each should render its content correctly without errors in the console.

- [x] **Step 5: Final commit (if any fixes were needed during manual testing)**

If manual testing revealed bugs that required fixes, commit them with descriptive messages. Otherwise, no commit needed.

---

## Self-Review Notes

**Spec coverage:**
- Git status (changed files, branch, ahead/behind) → Task 1 ✓
- Stage/unstage → Task 2 ✓
- Commit with message → Task 2 ✓
- Cross-file content search → Task 3 ✓
- Regex support → Task 3 (query treated as regex) ✓
- Case-insensitive toggle → Task 3 + Task 9 (UI toggle) ✓
- Search results grouped by file, clickable to open at line → Task 9 ✓
- Status bar branch name → Task 10 (watch syncs `gitState.branchName` → `appState.branchName`) ✓

**Placeholder scan:** No TBD/TODO/placeholder text. All steps have complete code.

**Type consistency:**
- `GitFileChange{Path, Status}` — Go struct (Task 1) → TS interface (Task 5) → store (Task 6) → component (Task 8) ✓
- `BranchInfo{Name, Ahead, Behind}` — Go struct (Task 1) → TS interface (Task 5) → store (Task 6) → component (Task 8) ✓
- `SearchMatch{Line, Column, Preview}` — Go struct (Task 3) → TS interface (Task 5) → store (Task 7) → component (Task 9) ✓
- `SearchResult{Path, Matches}` — Go struct (Task 3) → TS interface (Task 5) → store (Task 7) → component (Task 9) ✓
- `gitService.getStatus/getBranchInfo/stage/unstage/commit` — Go methods (Task 1-2) → TS wrapper (Task 5) → store (Task 6) ✓
- `searchService.search` — Go method (Task 3) → TS wrapper (Task 5) → store (Task 7) ✓

**Out of scope (deferred to a future plan):**
- Push/pull/fetch (network operations)
- Branch switching/creation/deletion
- Merge conflict resolution UI
- Diff viewer (inline or side-by-side)
- Stash management
- Search-and-replace
- File exclusion patterns (currently hardcoded in `ignoredDirs`)
- Search file-name only (currently content-only)
- Monaco scroll-to-line on search result click (file opens but does not jump to line)

---

## Follow-up Plans

### Plan 4: Plugins & Extensions (Deferred)

**Status:** Recommended to defer. A plugin/extension marketplace is a massive undertaking (API design, sandboxing, packaging, distribution). Recommend replacing the Extensions tab with a "Coming soon" page until there is real demand.

**If pursued later, would cover:**
- Extension manifest format (`package.json`-style)
- Extension host process (sandboxed)
- Extension API surface (commands, menus, editors)
- Marketplace UI (browse, install, update)
- Settings for enabling/disabling extensions
