# Git & File Tree Enhancements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Git branch management (list/create/checkout/delete), file tree context menu (new/rename/delete/copy path), search & replace, and auto-save to close key P1 feature gaps.

**Architecture:** Backend Go services gain branch and replace methods (go-git + file I/O). Frontend gains branch selector dropdown in GitPanel, context menu in FileTree, replace input in SearchPanel, and auto-save timer in editor store. New Wails3 bindings use FNV-1a hash IDs.

**Tech Stack:** Go 1.25, go-git v5.19.1, Vue 3 + TypeScript, Element Plus, Wails v3

---

## File Structure

| File | Responsibility |
|---|---|
| `services/git_service.go` | Add ListBranches, CreateBranch, CheckoutBranch, DeleteBranch methods |
| `services/git_service_test.go` | Tests for branch methods |
| `services/search_service.go` | Add Replace method |
| `services/search_service_test.go` | Tests for Replace |
| `frontend/bindings/changeme/services/gitservice.js` | Add branch method bindings with FNV-1a IDs |
| `frontend/bindings/changeme/services/searchservice.js` | Add Replace binding |
| `frontend/src/types/index.ts` | Add BranchRef type |
| `frontend/src/api/services.ts` | Add branch + replace API wrappers |
| `frontend/src/stores/git.ts` | Add branch state + actions |
| `frontend/src/components/layout/GitPanel.vue` | Add branch selector dropdown |
| `frontend/src/components/explorer/FileTree.vue` | Add context menu |
| `frontend/src/stores/search.ts` | Add replace state + action |
| `frontend/src/components/layout/SearchPanel.vue` | Add replace input + replace button |
| `frontend/src/stores/editor.ts` | Add auto-save timer logic |
| `frontend/src/views/EditorView.vue` | Wire auto-save focus change |

---

### Task 1: GitService Branch Management Backend

**Files:**
- Modify: `services/git_service.go`
- Modify: `services/git_service_test.go`

- [ ] **Step 1: Write failing tests for branch methods**

Append to `services/git_service_test.go`:

```go
func TestGitService_ListBranches(t *testing.T) {
	repoPath := setupTestRepo(t)
	svc := &GitService{}

	branches, err := svc.ListBranches(repoPath)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(branches) == 0 {
		t.Fatal("expected at least one branch (default)")
	}
	found := false
	for _, b := range branches {
		if b.Name == "main" || b.Name == "master" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected main/master branch, got %v", branches)
	}
}

func TestGitService_CreateAndCheckoutBranch(t *testing.T) {
	repoPath := setupTestRepo(t)
	svc := &GitService{}

	// Create a new branch
	err := svc.CreateBranch(repoPath, "feature-1")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Verify it appears in list
	branches, _ := svc.ListBranches(repoPath)
	found := false
	for _, b := range branches {
		if b.Name == "feature-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("feature-1 not found after creation")
	}

	// Checkout the new branch
	err = svc.CheckoutBranch(repoPath, "feature-1")
	if err != nil {
		t.Fatalf("CheckoutBranch failed: %v", err)
	}

	// Verify current branch is feature-1
	info, err := svc.GetBranchInfo(repoPath)
	if err != nil {
		t.Fatalf("GetBranchInfo failed: %v", err)
	}
	if info.Name != "feature-1" {
		t.Fatalf("expected current branch 'feature-1', got '%s'", info.Name)
	}
}

func TestGitService_DeleteBranch(t *testing.T) {
	repoPath := setupTestRepo(t)
	svc := &GitService{}

	// Create and checkout a temp branch, then switch back to delete original
	_ = svc.CreateBranch(repoPath, "temp-branch")
	_ = svc.CheckoutBranch(repoPath, "temp-branch")

	// Create another branch from here to switch to before deleting
	_ = svc.CreateBranch(repoPath, "keeper")
	_ = svc.CheckoutBranch(repoPath, "keeper")

	// Now delete temp-branch (not checked out)
	err := svc.DeleteBranch(repoPath, "temp-branch")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// Verify temp-branch is gone
	branches, _ := svc.ListBranches(repoPath)
	for _, b := range branches {
		if b.Name == "temp-branch" {
			t.Fatal("temp-branch still exists after delete")
		}
	}
}

func TestGitService_DeleteCurrentBranch_Fails(t *testing.T) {
	repoPath := setupTestRepo(t)
	svc := &GitService{}

	// Create a branch and check it out
	_ = svc.CreateBranch(repoPath, "doomed")
	_ = svc.CheckoutBranch(repoPath, "doomed")

	// Trying to delete the currently checked-out branch should fail
	err := svc.DeleteBranch(repoPath, "doomed")
	if err == nil {
		t.Fatal("expected error deleting current branch, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -run TestGitService_ListBranches -v -count=1`
Expected: FAIL with "ListBranches undefined" or compile error

- [ ] **Step 3: Implement BranchRef type and branch methods**

In `services/git_service.go`, add after the `BranchInfo` struct:

```go
// BranchRef represents a git branch reference.
type BranchRef struct {
	Name   string `json:"name"`
	IsHead bool   `json:"isHead"`
}

// ListBranches returns all local branches in the repository.
func (g *GitService) ListBranches(repoPath string) ([]BranchRef, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}
	headRef, err := repo.Head()
	if err != nil {
		return nil, err
	}
	headName := headRef.Name().Short()

	var branches []BranchRef
	iter, err := repo.Branches()
	if err != nil {
		return nil, err
	}
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, BranchRef{
			Name:   ref.Name().Short(),
			IsHead: ref.Name().Short() == headName,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return branches, nil
}

// CreateBranch creates a new branch at the current HEAD.
func (g *GitService) CreateBranch(repoPath string, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("branch name cannot be empty")
	}
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}
	headRef, err := repo.Head()
	if err != nil {
		return err
	}
	ref := plumbing.NewBranchReferenceName(name)
	_, err = repo.CreateBranch(&config.Branch{
		Name:   name,
		Hash:   headRef.Hash(),
		Remote: "origin",
		Merge:  ref,
	})
	return err
}

// CheckoutBranch switches the working tree to the named branch.
func (g *GitService) CheckoutBranch(repoPath string, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("branch name cannot be empty")
	}
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	return wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
	})
}

// DeleteBranch removes a local branch by name. Returns an error if the
// branch is currently checked out.
func (g *GitService) DeleteBranch(repoPath string, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("branch name cannot be empty")
	}
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}
	headRef, err := repo.Head()
	if err != nil {
		return err
	}
	if headRef.Name().Short() == name {
		return errors.New("cannot delete the currently checked-out branch")
	}
	return repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(name))
}
```

Add `"github.com/go-git/go-git/v5/config"` to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -run "TestGitService_(ListBranches|CreateAndCheckoutBranch|DeleteBranch|DeleteCurrentBranch)" -v -count=1`
Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add services/git_service.go services/git_service_test.go
git commit -m "feat: add Git branch management (list/create/checkout/delete)"
```

---

### Task 2: SearchService Replace Backend

**Files:**
- Modify: `services/search_service.go`
- Modify: `services/search_service_test.go`

- [ ] **Step 1: Write failing tests for Replace**

Append to `services/search_service_test.go`:

```go
func TestSearchService_ReplaceInFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	content := "hello world\nhello go\nbye world\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc := &SearchService{}
	result, err := svc.Replace(filePath, "hello", "hi", true)
	if err != nil {
		t.Fatalf("Replace failed: %v", err)
	}
	if result.Replacements != 2 {
		t.Errorf("expected 2 replacements, got %d", result.Replacements)
	}

	data, _ := os.ReadFile(filePath)
	expected := "hi world\nhi go\nbye world\n"
	if string(data) != expected {
		t.Errorf("file content mismatch:\n got: %q\nwant: %q", string(data), expected)
	}
}

func TestSearchService_Replace_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	content := "Hello HELLO hello\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc := &SearchService{}
	result, err := svc.Replace(filePath, "hello", "hi", false)
	if err != nil {
		t.Fatalf("Replace failed: %v", err)
	}
	if result.Replacements != 3 {
		t.Errorf("expected 3 case-insensitive replacements, got %d", result.Replacements)
	}
}

func TestSearchService_Replace_Regex(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	content := "foo123 bar456\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc := &SearchService{}
	result, err := svc.Replace(filePath, `foo(\d+)`, `baz$1`, true)
	if err != nil {
		t.Fatalf("Replace failed: %v", err)
	}
	if result.Replacements != 1 {
		t.Errorf("expected 1 regex replacement, got %d", result.Replacements)
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != "baz123 bar456\n" {
		t.Errorf("regex replace mismatch: %q", string(data))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/ -run TestSearchService_Replace -v -count=1`
Expected: FAIL with "Replace undefined" or compile error

- [ ] **Step 3: Implement Replace method**

In `services/search_service.go`, add the import `"regexp"` (if not already present) and append:

```go
// ReplaceResult reports the outcome of a replace operation.
type ReplaceResult struct {
	Replacements int `json:"replacements"`
}

// Replace replaces all occurrences of pattern in the file at filePath with
// replacement. If caseSensitive is false, the match is case-insensitive.
// The pattern is treated as a regular expression. The replacement string
// supports capture group references (e.g., $1).
func (s *SearchService) Replace(filePath string, pattern string, replacement string, caseSensitive bool) (*ReplaceResult, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, errors.New("pattern cannot be empty")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}

	count := 0
	newContent := re.ReplaceAllStringFunc(string(data), func(match string) string {
		count++
		return re.ReplaceAllString(match, replacement)
	})

	if count > 0 {
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			return nil, err
		}
	}

	return &ReplaceResult{Replacements: count}, nil
}
```

Ensure `"os"` and `"fmt"` are imported (they likely already are).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/ -run TestSearchService_Replace -v -count=1`
Expected: All 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add services/search_service.go services/search_service_test.go
git commit -m "feat: add SearchService.Replace for regex-based find-and-replace"
```

---

### Task 3: Frontend Bindings for Branch + Replace

**Files:**
- Modify: `frontend/bindings/changeme/services/gitservice.js`
- Modify: `frontend/bindings/changeme/services/searchservice.js`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/services.ts`

- [ ] **Step 1: Compute FNV-1a binding IDs**

Create a temporary Go file `services/_compute_ids.go` and run it to compute the binding IDs for the new methods. The binding ID format is `changeme/services.TypeName.MethodName`.

Run this Go program (save as `services/_compute_ids.go`, run, then delete):

```go
package main

import (
	"fmt"
	"hash/fnv"
)

func fnv1a(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func main() {
	methods := []string{
		"changeme/services.GitService.ListBranches",
		"changeme/services.GitService.CreateBranch",
		"changeme/services.GitService.CheckoutBranch",
		"changeme/services.GitService.DeleteBranch",
		"changeme/services.SearchService.Replace",
	}
	for _, m := range methods {
		fmt.Printf("%s => %d\n", m, fnv1a(m))
	}
}
```

Run: `go run services/_compute_ids.go`
Record the 5 IDs. Delete the temp file afterward.

- [ ] **Step 2: Add branch method bindings to gitservice.js**

In `frontend/bindings/changeme/services/gitservice.js`, add (using the computed IDs — placeholder values shown, REPLACE with actual computed values):

```javascript
export function ListBranches(path) {
  return $Call.ByID(COMPUTED_ID_LISTBRANCHES, path);
}

export function CreateBranch(path, name) {
  return $Call.ByID(COMPUTED_ID_CREATEBRANCH, path, name);
}

export function CheckoutBranch(path, name) {
  return $Call.ByID(COMPUTED_ID_CHECKOUTBRANCH, path, name);
}

export function DeleteBranch(path, name) {
  return $Call.ByID(COMPUTED_ID_DELETEBRANCH, path, name);
}
```

Replace `COMPUTED_ID_*` with the actual uint32 values from Step 1.

- [ ] **Step 3: Add Replace binding to searchservice.js**

In `frontend/bindings/changeme/services/searchservice.js`, add:

```javascript
export function Replace(filePath, pattern, replacement, caseSensitive) {
  return $Call.ByID(COMPUTED_ID_REPLACE, filePath, pattern, replacement, caseSensitive);
}
```

Replace `COMPUTED_ID_REPLACE` with the actual value from Step 1.

- [ ] **Step 4: Add BranchRef and ReplaceResult types**

In `frontend/src/types/index.ts`, add:

```typescript
export interface BranchRef {
  name: string;
  isHead: boolean;
}

export interface ReplaceResult {
  replacements: number;
}
```

- [ ] **Step 5: Add API wrappers in services.ts**

In `frontend/src/api/services.ts`:

1. Add `BranchRef, ReplaceResult` to the type import line (line 13).
2. Add to `gitService` object (after `getDiff`):

```typescript
  listBranches: (path: string) =>
    GitServiceBindings.ListBranches(path) as Promise<BranchRef[]>,
  createBranch: (path: string, name: string) =>
    GitServiceBindings.CreateBranch(path, name) as Promise<void>,
  checkoutBranch: (path: string, name: string) =>
    GitServiceBindings.CheckoutBranch(path, name) as Promise<void>,
  deleteBranch: (path: string, name: string) =>
    GitServiceBindings.DeleteBranch(path, name) as Promise<void>,
```

3. Add to `searchService` object:

```typescript
  replace: (filePath: string, pattern: string, replacement: string, caseSensitive: boolean) =>
    SearchServiceBindings.Replace(filePath, pattern, replacement, caseSensitive) as Promise<ReplaceResult>,
```

- [ ] **Step 6: Delete temp file and verify build**

Delete `services/_compute_ids.go`.

Run: `go build .`
Expected: success (exit 0)

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 7: Commit**

```bash
git add frontend/bindings/changeme/services/gitservice.js frontend/bindings/changeme/services/searchservice.js frontend/src/types/index.ts frontend/src/api/services.ts
git commit -m "feat: add frontend bindings for Git branch + Search replace"
```

---

### Task 4: GitPanel Branch Selector UI

**Files:**
- Modify: `frontend/src/stores/git.ts`
- Modify: `frontend/src/components/layout/GitPanel.vue`

- [ ] **Step 1: Add branch state and actions to git store**

In `frontend/src/stores/git.ts`, add:

```typescript
import { gitService } from "@/api/services";
import type { BranchRef } from "@/types";

export const branchState = reactive({
  branches: [] as BranchRef[],
  loadingBranches: false,
});

export async function loadBranches(repoPath: string) {
  if (!repoPath) return;
  branchState.loadingBranches = true;
  try {
    branchState.branches = await gitService.listBranches(repoPath);
  } catch (e) {
    console.error("Failed to load branches:", e);
    branchState.branches = [];
  } finally {
    branchState.loadingBranches = false;
  }
}

export async function createBranch(repoPath: string, name: string) {
  await gitService.createBranch(repoPath, name);
  await loadBranches(repoPath);
  await refreshStatus(repoPath);
}

export async function checkoutBranch(repoPath: string, name: string) {
  await gitService.checkoutBranch(repoPath, name);
  await loadBranches(repoPath);
  await refreshStatus(repoPath);
}

export async function deleteBranch(repoPath: string, name: string) {
  await gitService.deleteBranch(repoPath, name);
  await loadBranches(repoPath);
}
```

(Adjust imports as needed — `refreshStatus` should already exist in the store. If not, call whatever function loads `gitState.changes`.)

- [ ] **Step 2: Add branch selector dropdown to GitPanel.vue**

In `frontend/src/components/layout/GitPanel.vue`:

1. Add imports:

```typescript
import { ElMessage, ElMessageBox } from "element-plus";
import { branchState, loadBranches, createBranch, checkoutBranch, deleteBranch } from "@/stores/git";
```

2. Add branch selector UI in the header section (after the panel title, before the changes list):

```html
<div class="git-panel__branch-bar">
  <el-dropdown trigger="click" @command="handleBranchCommand">
    <span class="git-panel__branch-current">
      <el-icon :size="12"><ArrowDown /></el-icon>
      {{ currentBranchName }}
    </span>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item
          v-for="b in branchState.branches"
          :key="b.name"
          :command="b.name"
          :disabled="b.isHead"
        >
          {{ b.name }}{{ b.isHead ? " (current)" : "" }}
        </el-dropdown-item>
        <el-dropdown-item divided command="__new__">
          <el-icon><Plus /></el-icon> New Branch...
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</div>
```

3. Add computed + handlers in `<script setup>`:

```typescript
import { ArrowDown, Plus } from "@element-plus/icons-vue";

const currentBranchName = computed(() => {
  const head = branchState.branches.find((b) => b.isHead);
  return head?.name ?? gitState.branchName ?? "—";
});

const repoPath = computed(() => appState.currentProject ?? "");

watch(repoPath, (path) => {
  if (path) loadBranches(path);
}, { immediate: true });

async function handleBranchCommand(name: string) {
  if (!repoPath.value) return;
  if (name === "__new__") {
    try {
      const { value } = await ElMessageBox.prompt("Branch name", "Create Branch", {
        confirmButtonText: "Create",
        cancelButtonText: "Cancel",
        inputPattern: /^[A-Za-z0-9._\-/]+$/,
        inputErrorMessage: "Invalid branch name",
      });
      if (value) {
        await createBranch(repoPath.value, value);
        await checkoutBranch(repoPath.value, value);
        ElMessage.success(`Created and switched to '${value}'`);
      }
    } catch (e) {
      // user cancelled
    }
  } else {
    try {
      await checkoutBranch(repoPath.value, name);
      ElMessage.success(`Switched to '${name}'`);
    } catch (e: any) {
      ElMessage.error(`Failed to switch: ${e?.message ?? e}`);
    }
  }
}
```

4. Add styles:

```css
.git-panel__branch-bar {
  padding: 4px 12px;
  border-bottom: 1px solid var(--color-border-subtle);
}
.git-panel__branch-current {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  transition: background-color var(--transition-fast);
}
.git-panel__branch-current:hover {
  background-color: var(--color-bg-surface-container-low);
}
```

- [ ] **Step 3: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add frontend/src/stores/git.ts frontend/src/components/layout/GitPanel.vue
git commit -m "feat: add Git branch selector dropdown with create/checkout"
```

---

### Task 5: FileTree Context Menu

**Files:**
- Modify: `frontend/src/components/explorer/FileTree.vue`

- [ ] **Step 1: Add context menu state and handlers**

In `frontend/src/components/explorer/FileTree.vue`, add to `<script setup>`:

```typescript
import { ElMessage, ElMessageBox } from "element-plus";
import { fileService } from "@/api/services";

const contextMenuVisible = ref(false);
const contextMenuX = ref(0);
const contextMenuY = ref(0);

function onContextMenu(e: MouseEvent) {
  e.preventDefault();
  contextMenuX.value = e.clientX;
  contextMenuY.value = e.clientY;
  contextMenuVisible.value = true;
}

function closeContextMenu() {
  contextMenuVisible.value = false;
}

async function handleNewFile() {
  closeContextMenu();
  if (!isFolder.value) return;
  try {
    const { value } = await ElMessageBox.prompt("File name", "New File", {
      confirmButtonText: "Create",
      cancelButtonText: "Cancel",
    });
    if (!value) return;
    const newPath = props.path + "/" + value;
    await fileService.createFile(newPath);
    if (!expanded.value) expanded.value = true;
    await reloadChildren();
    emit("select", newPath);
  } catch (e: any) {
    ElMessage.error(`Failed: ${e?.message ?? e}`);
  }
}

async function handleNewFolder() {
  closeContextMenu();
  if (!isFolder.value) return;
  try {
    const { value } = await ElMessageBox.prompt("Folder name", "New Folder", {
      confirmButtonText: "Create",
      cancelButtonText: "Cancel",
    });
    if (!value) return;
    const newPath = props.path + "/" + value;
    await fileService.createDirectory(newPath);
    if (!expanded.value) expanded.value = true;
    await reloadChildren();
  } catch (e: any) {
    ElMessage.error(`Failed: ${e?.message ?? e}`);
  }
}

async function handleRename() {
  closeContextMenu();
  try {
    const { value } = await ElMessageBox.prompt("New name", "Rename", {
      confirmButtonText: "Rename",
      cancelButtonText: "Cancel",
      inputValue: props.name,
    });
    if (!value || value === props.name) return;
    const parentPath = props.path.substring(0, props.path.lastIndexOf("/"));
    const newPath = parentPath + "/" + value;
    await fileService.renamePath(props.path, newPath);
    emit("select", newPath);
  } catch (e: any) {
    ElMessage.error(`Failed: ${e?.message ?? e}`);
  }
}

async function handleDelete() {
  closeContextMenu();
  try {
    await ElMessageBox.confirm(
      `Delete '${props.name}'? This cannot be undone.`,
      "Confirm Delete",
      { confirmButtonText: "Delete", cancelButtonText: "Cancel", type: "warning" }
    );
    await fileService.deletePath(props.path);
  } catch (e: any) {
    if (e !== "cancel") {
      ElMessage.error(`Failed: ${e?.message ?? e}`);
    }
  }
}

async function handleCopyPath() {
  closeContextMenu();
  try {
    await navigator.clipboard.writeText(props.path);
    ElMessage.success("Path copied");
  } catch {
    ElMessage.error("Failed to copy path");
  }
}

async function reloadChildren() {
  loaded.value = false;
  loading.value = true;
  try {
    children.value = await fileService.listDirectory(props.path);
    loaded.value = true;
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
}
```

- [ ] **Step 2: Add context menu trigger to the file row**

In the template, add `@contextmenu="onContextMenu"` to the `.file-tree__row` div:

```html
<div
  class="file-tree__row"
  :style="indent"
  @click="onRowClick"
  @contextmenu="onContextMenu"
>
```

- [ ] **Step 3: Add context menu component to template**

Append before the closing `</div>` of `.file-tree`:

```html
<Teleport to="body">
  <div
    v-if="contextMenuVisible"
    class="file-tree__context-menu"
    :style="{ left: contextMenuX + 'px', top: contextMenuY + 'px' }"
    @click="closeContextMenu"
    @contextmenu.prevent="closeContextMenu"
  >
    <button v-if="isFolder" class="ctx-item" @click="handleNewFile">New File</button>
    <button v-if="isFolder" class="ctx-item" @click="handleNewFolder">New Folder</button>
    <button class="ctx-item" @click="handleRename">Rename</button>
    <button class="ctx-item ctx-item--danger" @click="handleDelete">Delete</button>
    <button class="ctx-item" @click="handleCopyPath">Copy Path</button>
  </div>
</Teleport>
```

- [ ] **Step 4: Add context menu styles**

```css
.file-tree__context-menu {
  position: fixed;
  z-index: 9999;
  min-width: 140px;
  padding: 4px;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-sm);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
.ctx-item {
  display: block;
  width: 100%;
  padding: 6px 10px;
  font-size: 12px;
  font-family: var(--font-sans);
  color: var(--color-text-secondary);
  background: transparent;
  border: none;
  border-radius: var(--radius-xs);
  text-align: left;
  cursor: pointer;
}
.ctx-item:hover {
  background: var(--color-bg-surface-container-low);
  color: var(--color-text-primary);
}
.ctx-item--danger:hover {
  color: var(--color-error, #f87171);
}
```

- [ ] **Step 5: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/explorer/FileTree.vue
git commit -m "feat: add file tree context menu (new file/folder, rename, delete, copy path)"
```

---

### Task 6: SearchPanel Replace UI

**Files:**
- Modify: `frontend/src/stores/search.ts`
- Modify: `frontend/src/components/layout/SearchPanel.vue`

- [ ] **Step 1: Add replace action to search store**

In `frontend/src/stores/search.ts`, add:

```typescript
import { searchService } from "@/api/services";

export async function replaceInFile(filePath: string, pattern: string, replacement: string, caseSensitive: boolean) {
  const fullPath = repoPath + "/" + filePath;
  return searchService.replace(fullPath, pattern, replacement, caseSensitive);
}

export async function replaceAll(pattern: string, replacement: string, caseSensitive: boolean) {
  let total = 0;
  for (const result of searchState.results) {
    const r = await replaceInFile(result.file, pattern, replacement, caseSensitive);
    total += r.replacements;
  }
  return total;
}
```

(If `repoPath` is not a module-level variable in search.ts, pass it as a parameter instead. Adjust to fit the existing store structure.)

- [ ] **Step 2: Add replace UI to SearchPanel.vue**

In `frontend/src/components/layout/SearchPanel.vue`:

1. Add imports:

```typescript
import { replaceAll } from "@/stores/search";
import { ElMessage } from "element-plus";
```

2. Add state:

```typescript
const showReplace = ref(false);
const replaceText = ref("");
const replacing = ref(false);
```

3. Add handler:

```typescript
async function handleReplaceAll() {
  if (!localQuery.value) return;
  replacing.value = true;
  try {
    const caseSensitive = !searchState.ignoreCase;
    const total = await replaceAll(localQuery.value, replaceText.value, caseSensitive);
    ElMessage.success(`${total} replacement(s) made`);
    // Re-run search to refresh results
    handleInput();
  } catch (e: any) {
    ElMessage.error(`Replace failed: ${e?.message ?? e}`);
  } finally {
    replacing.value = false;
  }
}
```

4. Add toggle button + replace input in the template (after the search input area):

```html
<button
  class="search-panel__toggle-replace"
  :class="{ active: showReplace }"
  @click="showReplace = !showReplace"
  aria-label="Toggle replace"
  title="Toggle Replace"
>
  <el-icon :size="12"><Switch /></el-icon>
</button>

<div v-if="showReplace" class="search-panel__replace-area">
  <input
    v-model="replaceText"
    class="search-panel__replace-input"
    placeholder="Replace..."
    @keydown.enter="handleReplaceAll"
  />
  <button
    class="search-panel__replace-btn"
    :disabled="replacing || !hasResults"
    @click="handleReplaceAll"
  >
    {{ replacing ? "..." : "Replace All" }}
  </button>
</div>
```

5. Add `Switch` icon import:

```typescript
import { Search, Switch } from "@element-plus/icons-vue";
```

6. Add styles:

```css
.search-panel__toggle-replace {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
}
.search-panel__toggle-replace:hover,
.search-panel__toggle-replace.active {
  color: var(--color-text-primary);
  background: var(--color-bg-surface-container-low);
}
.search-panel__replace-area {
  display: flex;
  gap: 4px;
  padding: 0 8px 6px;
}
.search-panel__replace-input {
  flex: 1;
  height: 24px;
  padding: 0 8px;
  font-size: 12px;
  color: var(--color-text-primary);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-xs);
}
.search-panel__replace-btn {
  padding: 0 10px;
  height: 24px;
  font-size: 11px;
  color: var(--color-text-primary);
  background: var(--color-primary);
  border: none;
  border-radius: var(--radius-xs);
  cursor: pointer;
}
.search-panel__replace-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
```

- [ ] **Step 3: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add frontend/src/stores/search.ts frontend/src/components/layout/SearchPanel.vue
git commit -m "feat: add search & replace UI with regex support"
```

---

### Task 7: Auto-Save Implementation

**Files:**
- Modify: `frontend/src/stores/editor.ts`
- Modify: `frontend/src/views/EditorView.vue`

- [ ] **Step 1: Add auto-save timer to editor store**

In `frontend/src/stores/editor.ts`, add:

```typescript
import { watch } from "vue";
import { appState } from "./app";

let autoSaveTimer: ReturnType<typeof setTimeout> | null = null;

export function setupAutoSave() {
  watch(
    () => editorState.activeFile?.content,
    (newContent, oldContent) => {
      if (!appState.autoSave || !editorState.activeFile || newContent === oldContent) return;
      if (autoSaveTimer) clearTimeout(autoSaveTimer);
      const delay = parseInt(appState.autoSaveDelay, 10) || 1000;
      autoSaveTimer = setTimeout(() => {
        saveFile();
      }, delay);
    }
  );
}

export function saveOnFocusChange() {
  if (appState.autoSave && editorState.activeFile?.isDirty) {
    saveFile();
  }
}
```

(Adjust `editorState.activeFile` references to match the actual editor store structure. The key is: watch content changes, debounce by `autoSaveDelay`, then call `saveFile()`.)

- [ ] **Step 2: Wire auto-save in EditorView**

In `frontend/src/views/EditorView.vue`:

1. Import and call `setupAutoSave`:

```typescript
import { setupAutoSave, saveOnFocusChange } from "@/stores/editor";

onMounted(() => {
  setupAutoSave();
  window.addEventListener("blur", saveOnFocusChange);
});

onBeforeUnmount(() => {
  window.removeEventListener("blur", saveOnFocusChange);
});
```

2. Ensure `onMounted` and `onBeforeUnmount` are imported from "vue".

- [ ] **Step 3: Verify type check**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add frontend/src/stores/editor.ts frontend/src/views/EditorView.vue
git commit -m "feat: implement auto-save (afterDelay + onFocusChange modes)"
```

---

### Task 8: Full Verification

**Files:** None (verification only)

- [ ] **Step 1: Run Go tests**

Run: `go test ./services/... -count=1 -timeout 60s`
Expected: `ok changeme/services` (exit 0)

- [ ] **Step 2: Run Go build**

Run: `go build .`
Expected: exit 0

- [ ] **Step 3: Run vue-tsc**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Run vitest**

Run: `cd frontend && npx vitest run`
Expected: All tests pass (9+ files, 82+ tests)

- [ ] **Step 5: Final commit (if any remaining changes)**

```bash
git add -A
git commit -m "chore: Plan 14 verification complete"
```
