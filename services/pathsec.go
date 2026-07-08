package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// pathsec.go — shared path security helpers (Proposal AA, N-91/N-92/N-67/N-112).
//
// This module centralizes path traversal defense logic that was previously
// duplicated (with varying completeness) across FileService.validatePath,
// PluginService.isPluginPathOutsideRoot, AgentService.validateCwd,
// TerminalService.validateWorkingDir, and RulesService.SaveRules.
//
// Two main classes of validation are provided:
//
//  1. Absolute path validation against a workspace root — resolves symlinks
//     on both the target and the root, then checks the relative path does
//     not escape via "..".
//
//  2. Relative name/id validation — rejects names containing path separators,
//     parent traversal (".."), absolute paths, and Windows volume-relative
//     forms. Used by services that join a user-supplied name/id to a fixed
//     directory (ConversationService, PresetService).

// ValidatePathWithinRoot returns nil if target resolves to a path inside root.
// If root is empty, any path is allowed (returns nil). The target is resolved
// to an absolute path, and symlinks on both target and root are evaluated so
// that a symlink inside the workspace pointing outside is rejected.
//
// For non-existent targets (e.g. a file about to be created), the parent
// directory's symlinks are resolved and the basename is re-joined.
//
// Returns the resolved absolute path on success.
func ValidatePathWithinRoot(root, target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	if root == "" {
		return abs, nil
	}
	absResolved, err := evalSymlinksAllowMissing(abs)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks for %s: %w", abs, err)
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		// If the root itself can't be resolved (e.g. temp dir already
		// cleaned in tests), fall back to the lexical root.
		rootResolved = root
	}
	rel, err := filepath.Rel(rootResolved, absResolved)
	if err != nil {
		return "", fmt.Errorf("compute relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s is outside the workspace root", target)
	}
	return abs, nil
}

// IsPathOutsideRoot reports whether absTarget escapes rootDir. Returns true
// if the target is outside the root, false if inside. An empty rootDir means
// "no restriction" (returns false). Symlinks are resolved on both paths.
//
// This is the boolean form of ValidatePathWithinRoot for callers that only
// need the escape verdict and not the resolved path.
func IsPathOutsideRoot(rootDir, absTarget string) bool {
	if rootDir == "" {
		return false
	}
	absResolved, err := evalSymlinksAllowMissing(absTarget)
	if err != nil {
		return true // can't resolve → treat as outside for safety
	}
	rootResolved, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		rootResolved = rootDir
	}
	rel, err := filepath.Rel(rootResolved, absResolved)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// IsRelativePathSafe reports whether relPath is a safe relative path that
// does not escape its base directory. It rejects:
//  1. Empty paths (callers should handle empty separately)
//  2. Unix-style absolute paths (leading "/")
//  3. Windows backslash-absolute paths (leading "\")
//  4. Windows drive paths and UNC paths (filepath.IsAbs)
//  5. Windows volume-relative form ("C:foo")
//  6. Parent traversal (".." or "../..." or "..\...")
//  7. Current-directory alias (".") — not a valid filename component
//
// This is the canonical implementation extracted from
// plugin_service.isPluginPathOutsideRoot. It does NOT touch the filesystem
// (pure lexical check) and is safe for validating user-supplied names/ids
// before joining them to a directory.
func IsRelativePathSafe(relPath string) bool {
	if relPath == "" {
		return false
	}
	// Reject Unix-style absolute paths that filepath.IsAbs misses on Windows.
	if strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, "\\") {
		return false
	}
	// filepath.IsAbs catches Windows drive paths ("C:\...") and UNC paths.
	if filepath.IsAbs(relPath) {
		return false
	}
	// Reject Windows volume-relative form ("C:foo" — not anchored to root).
	if len(relPath) >= 2 && relPath[1] == ':' {
		if len(relPath) == 2 || (relPath[2] != '/' && relPath[2] != '\\') {
			return false
		}
	}
	// Reject parent traversal and current-directory alias.
	cleaned := filepath.Clean(relPath)
	if cleaned == ".." || cleaned == "." {
		return false
	}
	if strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "..\\") {
		return false
	}
	return true
}

// SafeNameJoin joins baseDir, name, and ext into a path, after validating
// that name is a safe relative path component (no separators, no "..", no
// absolute paths). This is the helper for services that take a user-supplied
// id/name and append a file extension (e.g. ConversationService, PresetService).
//
// ext should include the leading dot (e.g. ".json"). If ext is empty, no
// extension is appended.
//
// Returns an error if name is empty, contains path separators, or would
// escape baseDir via traversal.
func SafeNameJoin(baseDir, name, ext string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if !IsRelativePathSafe(name) {
		return "", fmt.Errorf("invalid name %q: must be a simple filename without path separators or parent traversal", name)
	}
	// Additional check: reject any path separator in the name. Even though
	// IsRelativePathSafe rejects leading separators, a name like "sub/file"
	// would be "safe" by the traversal check but would create a subdirectory.
	// For id-based services we want a flat namespace.
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid name %q: path separators are not allowed", name)
	}
	return filepath.Join(baseDir, name+ext), nil
}

// ValidateNameForFlatDir validates that name is suitable for use as a
// filename in a flat directory (no subdirectories, no traversal). Returns
// nil if valid, an error otherwise.
//
// This is a lighter-weight check than SafeNameJoin for callers that don't
// need the joined path (e.g. delete operations that construct the path
// themselves).
func ValidateNameForFlatDir(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !IsRelativePathSafe(name) {
		return fmt.Errorf("invalid name %q: must be a simple filename without path separators or parent traversal", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid name %q: path separators are not allowed", name)
	}
	return nil
}

// evalSymlinksAllowMissing calls filepath.EvalSymlinks on path. If the path
// does not exist (e.g. a file about to be created), it resolves the parent
// directory's symlinks and rejoins with the basename. This prevents
// symlink-based traversal through not-yet-existing paths.
//
// This was previously defined in file_service.go; it is now shared via
// pathsec.go so all services can use it.
func evalSymlinksAllowMissing(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parentResolved, perr := filepath.EvalSymlinks(filepath.Dir(path))
	if perr != nil {
		parentResolved = filepath.Dir(path)
	}
	return filepath.Join(parentResolved, filepath.Base(path)), nil
}
