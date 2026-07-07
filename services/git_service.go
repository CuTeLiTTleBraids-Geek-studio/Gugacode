package services

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
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

// BranchRef represents a git branch reference.
type BranchRef struct {
	Name   string `json:"name"`
	IsHead bool   `json:"isHead"`
}

// ListBranches returns all local branches in the repository.
func (g *GitService) ListBranches(repoPath string) ([]BranchRef, error) {
	if err := g.validatePath(repoPath); err != nil {
		return nil, err
	}
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
	if err := g.validatePath(repoPath); err != nil {
		return err
	}
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}
	headRef, err := repo.Head()
	if err != nil {
		return err
	}
	refName := plumbing.NewBranchReferenceName(name)
	if err := repo.CreateBranch(&config.Branch{
		Name:   name,
		Remote: "origin",
		Merge:  refName,
	}); err != nil {
		return err
	}
	return repo.Storer.SetReference(plumbing.NewHashReference(refName, headRef.Hash()))
}

// CheckoutBranch switches the working tree to the named branch.
func (g *GitService) CheckoutBranch(repoPath string, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("branch name cannot be empty")
	}
	if err := g.validatePath(repoPath); err != nil {
		return err
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
	if err := g.validatePath(repoPath); err != nil {
		return err
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

// GitService exposes git operations to the frontend.
// N-67: when workspaceRoot is set via SetWorkspaceRoot, all repoPath/path
// arguments are validated to be within the workspace. This prevents the
// frontend from operating on git repos outside the open project.
type GitService struct {
	mu           sync.RWMutex
	workspaceRoot string
}

// SetWorkspaceRoot sets the directory within which git operations are allowed.
// Pass an empty string to disable sandboxing. The root is resolved to an
// absolute path and must be an existing directory.
func (g *GitService) SetWorkspaceRoot(root string) error {
	if root == "" {
		g.mu.Lock()
		g.workspaceRoot = ""
		g.mu.Unlock()
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
	g.mu.Lock()
	g.workspaceRoot = abs
	g.mu.Unlock()
	return nil
}

// validatePath returns nil if path is within the workspace root (or if no
// root is set). Uses the shared ValidatePathWithinRoot from pathsec.go which
// resolves symlinks on both the target and root.
func (g *GitService) validatePath(path string) error {
	g.mu.RLock()
	root := g.workspaceRoot
	g.mu.RUnlock()
	_, err := ValidatePathWithinRoot(root, path)
	return err
}

// statusToString converts a go-git status code to a human-readable label.
func statusToString(code git.StatusCode) string {
	switch code {
	case git.Untracked:
		return "Untracked"
	case git.Modified:
		return "Modified"
	case git.Added:
		return "Added"
	case git.Deleted:
		return "Deleted"
	case git.Renamed:
		return "Renamed"
	case git.Copied:
		return "Copied"
	case git.Unmodified:
		return "Unmodified"
	default:
		return "Modified"
	}
}

// GetStatus returns the list of changed files in the working tree at path.
func (g *GitService) GetStatus(path string) ([]GitFileChange, error) {
	if err := g.validatePath(path); err != nil {
		return nil, err
	}
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
		if code == git.Unmodified {
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
	if err := g.validatePath(path); err != nil {
		return BranchInfo{}, err
	}
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
	info.Ahead, info.Behind = countAheadBehind(repo, head.Hash(), ref.Hash())
	return info, nil
}

// countAheadBehind returns (ahead, behind) counts: commits reachable from head
// but not upstream, and vice versa. Uses the merge base as the divergence point.
func countAheadBehind(repo *git.Repository, head, upstream plumbing.Hash) (int, int) {
	headCommit, err := repo.CommitObject(head)
	if err != nil {
		return 0, 0
	}
	upstreamCommit, err := repo.CommitObject(upstream)
	if err != nil {
		return 0, 0
	}
	base, err := headCommit.MergeBase(upstreamCommit)
	var baseHash *plumbing.Hash
	if err == nil && len(base) > 0 {
		h := base[0].Hash
		baseHash = &h
	}
	return countReachable(repo, head, baseHash), countReachable(repo, upstream, baseHash)
}

// countReachable counts commits reachable from start, stopping at (excluding)
// the commit identified by stop when non-nil.
func countReachable(repo *git.Repository, start plumbing.Hash, stop *plumbing.Hash) int {
	count := 0
	visited := map[plumbing.Hash]bool{}
	queue := []plumbing.Hash{start}
	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]
		if visited[h] {
			continue
		}
		visited[h] = true
		if stop != nil && h == *stop {
			continue
		}
		count++
		c, err := repo.CommitObject(h)
		if err != nil {
			break
		}
		queue = append(queue, c.ParentHashes...)
	}
	return count
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

// Stage adds a file path to the git index.
func (g *GitService) Stage(path, filePath string) error {
	if err := g.validatePath(path); err != nil {
		return err
	}
	_, wt, err := openWorktree(path)
	if err != nil {
		return err
	}
	_, err = wt.Add(filePath)
	return err
}

// Unstage removes a file path from the git index (resets to HEAD).
func (g *GitService) Unstage(path, filePath string) error {
	if err := g.validatePath(path); err != nil {
		return err
	}
	repo, wt, err := openWorktree(path)
	if err != nil {
		return err
	}
	head, err := repo.Head()
	if err != nil {
		// No HEAD yet (no commits) — drop the entry from the index directly,
		// keeping the working-tree file in place so it becomes untracked again.
		idx, err := repo.Storer.Index()
		if err != nil {
			return err
		}
		if _, err := idx.Remove(filePath); err != nil && !errors.Is(err, index.ErrEntryNotFound) {
			return err
		}
		return repo.Storer.SetIndex(idx)
	}
	return wt.Reset(&git.ResetOptions{
		Mode:   git.MixedReset,
		Commit: head.Hash(),
		Files:  []string{filePath},
	})
}

// Commit creates a new commit with the currently staged changes.
func (g *GitService) Commit(path, message string) error {
	if err := g.validatePath(path); err != nil {
		return err
	}
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
		if s.Staging != git.Unmodified && s.Staging != git.Untracked {
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

// Push pushes local commits to the configured remote (origin by default).
// It uses the current branch and sets up tracking if needed.
func (g *GitService) Push(repoPath string) error {
	if err := g.validatePath(repoPath); err != nil {
		return err
	}
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}
	if !head.Name().IsBranch() {
		return errors.New("HEAD is not on a branch")
	}
	branchName := head.Name().Short()

	remote, err := repo.Remote("origin")
	if err != nil {
		return fmt.Errorf("no 'origin' remote configured: %w", err)
	}

	err = remote.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec("refs/heads/" + branchName + ":refs/heads/" + branchName)},
	})
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
		}
		return err
	}
	return nil
}

// Pull fetches and merges from the configured remote (origin).
func (g *GitService) Pull(repoPath string) error {
	if err := g.validatePath(repoPath); err != nil {
		return err
	}
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	if _, err := repo.Remote("origin"); err != nil {
		return fmt.Errorf("no 'origin' remote configured: %w", err)
	}

	err = wt.Pull(&git.PullOptions{
		RemoteName: "origin",
	})
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
		}
		return err
	}
	return nil
}

// GetDiff returns the unified diff for a single file.
// For staged files, diffs HEAD vs index. For unstaged changes, diffs index vs worktree.
// For untracked files, returns the full content as additions.
func (g *GitService) GetDiff(repoPath string, filePath string) (string, error) {
	if err := g.validatePath(repoPath); err != nil {
		return "", err
	}
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

	// If fully untracked, return file content as all-added
	if fileStatus.Staging == git.Untracked && fileStatus.Worktree == git.Untracked {
		return g.diffUntrackedFile(repoPath, filePath)
	}

	// For staged changes (Staging is Modified/Added/Deleted), diff HEAD vs index
	if fileStatus.Staging != git.Unmodified && fileStatus.Staging != git.Untracked {
		return g.diffStaged(repo, filePath)
	}

	// For unstaged changes, diff index vs worktree
	return g.diffWorktree(repo, filePath)
}

// diffUntrackedFile returns the full file content as a diff with all lines added.
func (g *GitService) diffUntrackedFile(repoPath, filePath string) (string, error) {
	absPath := filepath.Join(repoPath, filePath)
	data, err := os.ReadFile(absPath)
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

// diffStaged diffs the HEAD version vs the index version of a file.
func (g *GitService) diffStaged(repo *git.Repository, filePath string) (string, error) {
	// Get HEAD version
	headData, err := g.getFileFromHead(repo, filePath)
	if err != nil {
		// File is new in index (no HEAD version)
		idxData, err2 := g.getFileFromIndex(repo, filePath)
		if err2 != nil {
			return "", err2
		}
		return g.formatNewFileDiff(filePath, idxData), nil
	}

	// Get index version
	idxData, err := g.getFileFromIndex(repo, filePath)
	if err != nil {
		return "", err
	}

	return myersDiff(filePath, headData, idxData), nil
}

// diffWorktree diffs the index version vs the working tree version of a file.
func (g *GitService) diffWorktree(repo *git.Repository, filePath string) (string, error) {
	idxData, err := g.getFileFromIndex(repo, filePath)
	if err != nil {
		// File not in index — this case is handled by diffUntrackedFile in GetDiff;
		// return the error if we somehow reach here.
		return "", err
	}

	// Read worktree version
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	absPath := filepath.Join(wt.Filesystem.Root(), filePath)
	wtData, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	return myersDiff(filePath, idxData, string(wtData)), nil
}

// getFileFromHead reads the file content from the HEAD commit's tree.
func (g *GitService) getFileFromHead(repo *git.Repository, filePath string) (string, error) {
	headRef, err := repo.Head()
	if err != nil {
		return "", err
	}
	commit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return "", err
	}
	tree, err := commit.Tree()
	if err != nil {
		return "", err
	}
	file, err := tree.File(filePath)
	if err != nil {
		return "", err
	}
	reader, err := file.Reader()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// getFileFromIndex reads the file content from the git index.
func (g *GitService) getFileFromIndex(repo *git.Repository, filePath string) (string, error) {
	idx, err := repo.Storer.Index()
	if err != nil {
		return "", err
	}
	entry, err := idx.Entry(filePath)
	if err != nil {
		return "", err
	}
	blob, err := repo.BlobObject(entry.Hash)
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
	return string(data), nil
}

// formatNewFileDiff returns a diff for a newly added file.
func (g *GitService) formatNewFileDiff(filePath string, content string) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", filePath, filePath))
	buf.WriteString("new file mode 100644\n")
	buf.WriteString("--- /dev/null\n")
	buf.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))
	for _, line := range strings.Split(content, "\n") {
		buf.WriteString("+" + line + "\n")
	}
	return buf.String()
}

// GetFullDiff returns the combined diff of all changed files (staged + unstaged
// + untracked) in the working tree. Each file's diff is preceded by a header
// line of the form "=== filePath ===" for easy parsing. Returns an empty string
// when there are no changes. Used by the AI code review feature (#27).
func (g *GitService) GetFullDiff(repoPath string) (string, error) {
	changes, err := g.GetStatus(repoPath)
	if err != nil {
		return "", err
	}
	if len(changes) == 0 {
		return "", nil
	}
	var buf bytes.Buffer
	for _, c := range changes {
		d, err := g.GetDiff(repoPath, c.Path)
		if err != nil {
			// Skip files that fail to diff (e.g. binary, deleted) but continue.
			continue
		}
		if d == "" {
			continue
		}
		buf.WriteString(fmt.Sprintf("=== %s ===\n", c.Path))
		buf.WriteString(d)
		if !strings.HasSuffix(d, "\n") {
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}
	return buf.String(), nil
}

// Note: unifiedDiff was replaced by myersDiff (Plan 60 / N-27) which
// implements the Myers O(ND) diff algorithm for cleaner diffs.

