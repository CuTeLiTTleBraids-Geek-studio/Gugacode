package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var testAuthor = object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}

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
	if st.IsUntracked("a.txt") {
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
	if !st.IsUntracked("a.txt") {
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

	initialContent := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("main.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("initial", &git.CommitOptions{Author: &testAuthor}); err != nil {
		t.Fatal(err)
	}

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

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := initBareRepo(t)
	writeFile(t, dir, "README.md", "initial`n")
	commitAll(t, dir, "initial commit")
	return dir
}

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

	err := svc.CreateBranch(repoPath, "feature-1")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

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

	err = svc.CheckoutBranch(repoPath, "feature-1")
	if err != nil {
		t.Fatalf("CheckoutBranch failed: %v", err)
	}

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

	_ = svc.CreateBranch(repoPath, "temp-branch")
	_ = svc.CheckoutBranch(repoPath, "temp-branch")
	_ = svc.CreateBranch(repoPath, "keeper")
	_ = svc.CheckoutBranch(repoPath, "keeper")

	err := svc.DeleteBranch(repoPath, "temp-branch")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

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

	_ = svc.CreateBranch(repoPath, "doomed")
	_ = svc.CheckoutBranch(repoPath, "doomed")

	err := svc.DeleteBranch(repoPath, "doomed")
	if err == nil {
		t.Fatal("expected error deleting current branch, got nil")
	}
}

func TestGitService_GetFullDiff_emptyRepo(t *testing.T) {
	dir := initBareRepo(t)
	svc := &GitService{}
	diff, err := svc.GetFullDiff(dir)
	if err != nil {
		t.Fatalf("GetFullDiff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for repo with no changes, got: %q", diff)
	}
}

func TestGitService_GetFullDiff_multipleChanges(t *testing.T) {
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit failed: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}

	// Commit an initial file.
	writeFile(t, repoDir, "a.txt", "initial a\n")
	writeFile(t, repoDir, "b.txt", "initial b\n")
	if _, err := wt.Add("a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("initial", &git.CommitOptions{Author: &testAuthor}); err != nil {
		t.Fatal(err)
	}

	// Modify a.txt, add new c.txt, leave b.txt unchanged.
	writeFile(t, repoDir, "a.txt", "modified a\n")
	writeFile(t, repoDir, "c.txt", "new file c\n")

	svc := &GitService{}
	diff, err := svc.GetFullDiff(repoDir)
	if err != nil {
		t.Fatalf("GetFullDiff failed: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff with 2 changed files")
	}
	// Should contain headers for a.txt and c.txt
	if !strings.Contains(diff, "=== a.txt ===") {
		t.Errorf("diff should contain '=== a.txt ===' header, got: %s", diff)
	}
	if !strings.Contains(diff, "=== c.txt ===") {
		t.Errorf("diff should contain '=== c.txt ===' header, got: %s", diff)
	}
	// Should NOT contain b.txt (unchanged)
	if strings.Contains(diff, "=== b.txt ===") {
		t.Errorf("diff should not contain unchanged file b.txt")
	}
	// Should contain the modified content
	if !strings.Contains(diff, "modified a") {
		t.Errorf("diff should contain 'modified a'")
	}
	if !strings.Contains(diff, "new file c") {
		t.Errorf("diff should contain 'new file c'")
	}
}

func TestGitService_GetFullDiff_notARepo(t *testing.T) {
	dir := t.TempDir()
	svc := &GitService{}
	_, err := svc.GetFullDiff(dir)
	if err == nil {
		t.Fatal("expected error for non-repo directory, got nil")
	}
}

// N-67: GitService workspace sandbox — when SetWorkspaceRoot is set, all
// operations must reject paths outside the workspace. This prevents the
// frontend from operating on git repos outside the open project.
func TestGitService_N67_SetWorkspaceRoot_RejectsOutsidePath(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	// Initialize a git repo OUTSIDE the workspace.
	repoDir := initBareRepo(t)
	// Move it outside — actually, initBareRepo already created it in a temp
	// dir. We just need a dir that's outside the workspace. Let's use the
	// parent of the workspace.
	_ = outside
	_ = repoDir

	svc := &GitService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// Create a repo outside the workspace.
	outsideRepo := t.TempDir()
	_, err := git.PlainInit(outsideRepo, false)
	if err != nil {
		t.Fatalf("git.PlainInit failed: %v", err)
	}
	// All GitService operations on the outside repo should be rejected.
	t.Run("GetStatus", func(t *testing.T) {
		_, err := svc.GetStatus(outsideRepo)
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("GetBranchInfo", func(t *testing.T) {
		_, err := svc.GetBranchInfo(outsideRepo)
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("ListBranches", func(t *testing.T) {
		_, err := svc.ListBranches(outsideRepo)
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("Stage", func(t *testing.T) {
		err := svc.Stage(outsideRepo, "file.txt")
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("Commit", func(t *testing.T) {
		err := svc.Commit(outsideRepo, "msg")
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("Push", func(t *testing.T) {
		err := svc.Push(outsideRepo)
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("Pull", func(t *testing.T) {
		err := svc.Pull(outsideRepo)
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("GetFullDiff", func(t *testing.T) {
		_, err := svc.GetFullDiff(outsideRepo)
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
}

// N-67: when SetWorkspaceRoot is set, operations on paths INSIDE the
// workspace should still work.
func TestGitService_N67_SetWorkspaceRoot_AllowsInsidePath(t *testing.T) {
	workspace := t.TempDir()
	// Init a git repo inside the workspace.
	repoDir := filepath.Join(workspace, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	_, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("git.PlainInit failed: %v", err)
	}
	svc := &GitService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// GetStatus on the inside repo should work.
	_, err = svc.GetStatus(repoDir)
	if err != nil {
		t.Errorf("expected success for path inside workspace, got: %v", err)
	}
}

// N-67 / Proposal AJ: additional methods that call validatePath but were
// not covered by TestGitService_N67_SetWorkspaceRoot_RejectsOutsidePath.
// Each subtest verifies the method rejects a repo path outside the workspace.
func TestGitService_N67_SetWorkspaceRoot_RejectsOutsidePath_AdditionalMethods(t *testing.T) {
	workspace := t.TempDir()
	svc := &GitService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// Create a valid git repo outside the workspace.
	outsideRepo := t.TempDir()
	if _, err := git.PlainInit(outsideRepo, false); err != nil {
		t.Fatalf("git.PlainInit failed: %v", err)
	}
	// Stage a file so later operations have something to work with.
	writeFile(t, outsideRepo, "file.txt", "content")
	repo, err := git.PlainOpen(outsideRepo)
	if err != nil {
		t.Fatalf("PlainOpen failed: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}
	_ = wt.AddGlob(".")
	_, _ = wt.Commit("init", &git.CommitOptions{Author: &testAuthor})

	t.Run("CreateBranch", func(t *testing.T) {
		if err := svc.CreateBranch(outsideRepo, "feature"); err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("CheckoutBranch", func(t *testing.T) {
		if err := svc.CheckoutBranch(outsideRepo, "main"); err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("DeleteBranch", func(t *testing.T) {
		if err := svc.DeleteBranch(outsideRepo, "feature"); err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("Unstage", func(t *testing.T) {
		if err := svc.Unstage(outsideRepo, "file.txt"); err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
	t.Run("GetDiff", func(t *testing.T) {
		if _, err := svc.GetDiff(outsideRepo, "file.txt"); err == nil {
			t.Error("expected error for path outside workspace")
		}
	})
}

// N-67 / Proposal AJ: when no workspace root is set (legacy mode), any path is allowed.
func TestGitService_N67_NoWorkspaceRoot_AllowsAnyPath(t *testing.T) {
	repoDir := initBareRepo(t)
	svc := &GitService{} // no SetWorkspaceRoot call
	_, err := svc.GetStatus(repoDir)
	if err != nil {
		t.Errorf("expected success without workspace root, got: %v", err)
	}
}

// N-67: SetWorkspaceRoot with empty string disables sandboxing.
func TestGitService_N67_EmptyWorkspaceRoot_DisablesSandbox(t *testing.T) {
	workspace := t.TempDir()
	svc := &GitService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatal(err)
	}
	// Disable sandbox.
	if err := svc.SetWorkspaceRoot(""); err != nil {
		t.Fatal(err)
	}
	// Now any path should be allowed.
	repoDir := initBareRepo(t)
	_, err := svc.GetStatus(repoDir)
	if err != nil {
		t.Errorf("expected success after disabling sandbox, got: %v", err)
	}
}
