package services

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// File permission policy:
// Files are created with mode 0644 (owner read/write, group/others read-only).
// Directories are created with mode 0755 (owner rwx, group/others rx).
// These fixed modes are used instead of respecting umask to ensure
// consistent behavior across platforms (Windows ignores Unix permission bits,
// macOS/Linux honor them). Users who need different permissions can chmod
// after creation via the terminal.

// DirEntry represents a single file or folder returned by ListDirectory.
type DirEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"isDir"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

// FileService exposes file-system operations to the frontend.
// When a workspace root is set via SetWorkspaceRoot, all file operations
// (including ListDirectory) are sandboxed to that directory.
type FileService struct {
	mu      sync.Mutex
	rootDir string
	app     *application.App
}

// NewFileService creates a new FileService with no workspace root set.
func NewFileService() *FileService {
	return &FileService{}
}

// SetApp links the application instance so FileService can emit events
// (e.g. "file:saved" after WriteFile). Called from main.go after the app
// is created. When not set, event emission is skipped (Proposal B).
func (f *FileService) SetApp(app *application.App) {
	f.app = app
}

// SetWorkspaceRoot sets the directory within which file operations are allowed.
// Pass an empty string to disable sandboxing.
func (f *FileService) SetWorkspaceRoot(root string) error {
	if root == "" {
		f.mu.Lock()
		f.rootDir = ""
		f.mu.Unlock()
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
	f.mu.Lock()
	f.rootDir = abs
	f.mu.Unlock()
	return nil
}

// validatePath returns the absolute path if it's within the workspace root,
// or an error if it's outside. If no workspace root is set, any path is allowed.
//
// N-56: filepath.Abs only performs lexical cleaning — it does NOT resolve
// symlinks. A symlink placed inside the workspace that points outside
// (e.g., ./link -> ../../../etc/passwd) would pass the lexical prefix
// check. We therefore call filepath.EvalSymlinks on both the target and
// the root before the prefix comparison. For paths that don't yet exist
// (e.g., CreateFile targets), we resolve the parent directory's symlinks
// and rejoin with the basename.
func (f *FileService) validatePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	root := f.rootDir
	f.mu.Unlock()
	if root == "" {
		return abs, nil
	}
	absResolved, err := evalSymlinksAllowMissing(abs)
	if err != nil {
		return "", err
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		// If the root itself can't be resolved, fall back to lexical.
		rootResolved = root
	}
	rel, err := filepath.Rel(rootResolved, absResolved)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("path %s is outside the workspace", path)
	}
	return abs, nil
}

// ListDirectory returns the immediate children of path, directories first.
func (f *FileService) ListDirectory(path string) ([]DirEntry, error) {
	abs, err := f.validatePath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	result := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result = append(result, DirEntry{
			Name:     entry.Name(),
			Path:     filepath.Join(abs, entry.Name()),
			IsDir:    entry.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime().UnixMilli(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// ReadFile reads and returns the full text content of a file.
func (f *FileService) ReadFile(path string) (string, error) {
	abs, err := f.validatePath(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile writes text content to a file, creating or truncating it.
// After a successful write, emits a "file:saved" event with the absolute
// file path so workflow triggers (Proposal B) can match it.
func (f *FileService) WriteFile(path string, content string) error {
	abs, err := f.validatePath(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		return err
	}
	// Emit file:saved for workflow triggers (Proposal B). Skip when no
	// app is wired (e.g. in unit tests).
	if f.app != nil {
		f.app.Event.Emit("file:saved", abs)
	}
	return nil
}

// CreateFile creates an empty file.
func (f *FileService) CreateFile(path string) error {
	abs, err := f.validatePath(path)
	if err != nil {
		return err
	}
	file, err := os.Create(abs)
	if err != nil {
		return err
	}
	return file.Close()
}

// CreateDirectory creates a directory and any necessary parents.
func (f *FileService) CreateDirectory(path string) error {
	abs, err := f.validatePath(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0755)
}

// DeletePath removes a file or directory recursively.
func (f *FileService) DeletePath(path string) error {
	abs, err := f.validatePath(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(abs)
}

// RenamePath moves or renames a file or directory.
func (f *FileService) RenamePath(oldPath, newPath string) error {
	oldAbs, err := f.validatePath(oldPath)
	if err != nil {
		return err
	}
	newAbs, err := f.validatePath(newPath)
	if err != nil {
		return err
	}
	return os.Rename(oldAbs, newAbs)
}

// PickDirectory opens a native directory-selection dialog and returns the chosen path.
// Returns an empty string if the user cancels.
func (f *FileService) PickDirectory() (string, error) {
	dialog := application.Get().Dialog.OpenFile()
	dialog.SetTitle("Open Folder")
	dialog.CanChooseFiles(false)
	dialog.CanChooseDirectories(true)
	return dialog.PromptForSingleSelection()
}

// quickOpenIgnoreDirs is a hardcoded list of directories that are virtually
// always noise for Quick Open. These are skipped in addition to any patterns
// found in .gitignore. Hidden directories (starting with ".") are also skipped.
var quickOpenIgnoreDirs = map[string]bool{
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"out":          true,
	"target":       true,
	"vendor":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"env":          true,
	".idea":        true,
	".vscode":      true,
	".next":        true,
	".nuxt":        true,
	".svelte-kit":  true,
	".gradle":      true,
	"bin":          true,
	"obj":          true,
	"coverage":     true,
	".cache":       true,
}

// maxQuickOpenFiles caps the number of files returned by ListAllFiles to
// prevent excessive memory use on very large repositories. 10000 is plenty
// for Quick Open — anything beyond that is unlikely to be useful in a
// fuzzy finder and would hurt responsiveness.
const maxQuickOpenFiles = 10000

// ListAllFiles walks the directory tree rooted at rootPath and returns the
// relative paths of all files, using forward slashes for cross-platform
// consistency. It skips:
//   - directories listed in quickOpenIgnoreDirs
//   - hidden directories and files (starting with ".")
//   - patterns listed in the root .gitignore (simple matching: exact name,
//     leading "/" for root-anchored, trailing "/" for dir-only, and "*"
//     wildcards within a single path segment)
//
// The result is sorted lexicographically. If rootPath is not within the
// workspace root (when sandboxing is active), an error is returned.
func (f *FileService) ListAllFiles(rootPath string) ([]string, error) {
	abs, err := f.validatePath(rootPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", abs)
	}

	// Load .gitignore patterns from the root directory.
	patterns := loadGitignorePatterns(abs)

	var result []string
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		name := d.Name()
		// Skip the root itself.
		if path == abs {
			return nil
		}
		// Skip hidden entries (starting with ".").
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip ignored directories.
		if d.IsDir() && quickOpenIgnoreDirs[name] {
			return filepath.SkipDir
		}
		// Compute the relative path with forward slashes.
		rel, rerr := filepath.Rel(abs, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		// Check .gitignore patterns.
		if matchGitignore(rel, d.IsDir(), patterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			result = append(result, rel)
			if len(result) >= maxQuickOpenFiles {
				return errStopWalk
			}
		}
		return nil
	})
	// errStopWalk is a sentinel used to cap the result size; treat it as success.
	if err != nil && err != errStopWalk {
		return nil, err
	}
	sort.Strings(result)
	return result, nil
}

// errStopWalk is a sentinel error used to stop filepath.WalkDir early once
// the result cap (maxQuickOpenFiles) is reached.
var errStopWalk = fmt.Errorf("stop walk: max file count reached")

// gitignorePattern represents a single parsed .gitignore line.
type gitignorePattern struct {
	negate    bool   // "!" prefix
	dirOnly   bool   // trailing "/"
	anchored  bool   // leading "/" — pattern is relative to the .gitignore dir
	segments  []string // path segments, each may contain "*" wildcards
}

// loadGitignorePatterns reads .gitignore from dir (if present) and parses
// it into a list of patterns. Each line is parsed as follows:
//   - empty lines and lines starting with "#" are skipped
//   - leading "!" sets negate=true
//   - leading "/" anchors the pattern to the .gitignore directory
//   - trailing "/" marks the pattern as dir-only
//   - the rest is split on "/" into segments, each kept as-is (wildcards
//     are handled by matchSegment at match time)
func loadGitignorePatterns(dir string) []gitignorePattern {
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil
	}
	var patterns []gitignorePattern
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := gitignorePattern{}
		if strings.HasPrefix(line, "!") {
			p.negate = true
			line = line[1:]
		}
		if strings.HasPrefix(line, "/") {
			p.anchored = true
			line = strings.TrimPrefix(line, "/")
		}
		if strings.HasSuffix(line, "/") {
			p.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		p.segments = strings.Split(line, "/")
		patterns = append(patterns, p)
	}
	return patterns
}

// matchGitignore returns true if relPath should be ignored according to the
// given patterns. relPath is a forward-slash-relative path. isDir indicates
// whether the entry is a directory (used for dir-only patterns).
func matchGitignore(relPath string, isDir bool, patterns []gitignorePattern) bool {
	ignored := false
	for _, p := range patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if !matchPath(relPath, p) {
			continue
		}
		if p.negate {
			ignored = false
		} else {
			ignored = true
		}
	}
	return ignored
}

// matchPath checks whether relPath matches a single gitignore pattern.
// For non-anchored patterns, the pattern matches if ANY path suffix matches
// (mirroring gitignore's "match in any directory" rule). For anchored
// patterns, the full relative path must match.
func matchPath(relPath string, p gitignorePattern) bool {
	segments := strings.Split(relPath, "/")
	if p.anchored {
		return matchSegments(segments, p.segments)
	}
	// Non-anchored: try matching at every suffix.
	for i := 0; i < len(segments); i++ {
		if matchSegments(segments[i:], p.segments) {
			return true
		}
	}
	return false
}

// matchSegments checks whether the leading portion of pathSegs matches all
// of the pattern segments. Each segment is matched with matchSegment which
// supports "*" as a wildcard for any run of characters within one segment.
func matchSegments(pathSegs, patternSegs []string) bool {
	if len(patternSegs) > len(pathSegs) {
		return false
	}
	for i, ps := range patternSegs {
		if !matchSegment(pathSegs[i], ps) {
			return false
		}
	}
	// If the pattern has fewer segments than the path, it only matches if
	// the pattern's last segment is a directory match (i.e. we matched a
	// prefix directory). For gitignore, a pattern "foo/bar" matches the
	// path "foo/bar/baz" because "foo/bar" is a directory. We approximate
	// this by accepting when there are leftover path segments — the
	// caller already filtered by isDir for dir-only patterns.
	return true
}

// matchSegment matches a single path segment against a pattern segment,
// supporting "*" wildcards (zero or more characters). This is a simple
// iterative matcher — no support for "**", "?", or character classes.
func matchSegment(seg, pattern string) bool {
	// Fast path: no wildcard.
	if !strings.Contains(pattern, "*") {
		return seg == pattern
	}
	// Iterative glob match with "*" expansion.
	si, pi := 0, 0
	starIdx, matchIdx := -1, 0
	for si < len(seg) {
		if pi < len(pattern) && (pattern[pi] == seg[si]) {
			si++
			pi++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			starIdx = pi
			matchIdx = si
			pi++
		} else if starIdx != -1 {
			pi = starIdx + 1
			matchIdx++
			si = matchIdx
		} else {
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// RevealInOS opens the host operating system's file explorer and selects
// the given path. On Windows it uses `explorer.exe /select,`; on macOS it
// uses `open -R`; on Linux it uses `xdg-open` on the parent directory
// (no universal "select" flag exists across Linux file managers).
//
// N-105: the previous implementation called cmd.Start() without a paired
// cmd.Wait(), leaving zombie processes on Unix until the parent (the IDE)
// exited. We now start the command and reap it in a goroutine so the
// caller still returns immediately (the explorer launch is non-blocking
// from the user's perspective) but no zombie lingers.
func (f *FileService) RevealInOS(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// explorer.exe /select, works with both files and directories.
		cmd = exec.Command("explorer.exe", "/select,", abs)
	case "darwin":
		// `open -R` reveals a file in Finder; for a directory, just open it.
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			cmd = exec.Command("open", abs)
		} else {
			cmd = exec.Command("open", "-R", abs)
		}
	default: // linux and other unix-like
		// xdg-open opens the parent directory; selecting the file is not
		// universally supported across file managers.
		dir := abs
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			dir = filepath.Dir(abs)
		}
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the child process asynchronously so it doesn't become a
	// zombie. The error (if any) is logged but not surfaced — by the
	// time Wait returns the caller has long moved on, and a non-zero
	// exit from the file manager is not actionable.
	go func() {
		if werr := cmd.Wait(); werr != nil {
			slog.Debug("reveal command exited non-zero", "cmd", cmd.Args, "err", werr)
		}
	}()
	return nil
}
