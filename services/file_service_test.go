package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileService_ListDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	svc := &FileService{}
	entries, err := svc.ListDirectory(dir)
	if err != nil {
		t.Fatalf("ListDirectory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[0].IsDir {
		t.Error("expected directory to sort first")
	}
	if entries[0].Name != "subdir" {
		t.Errorf("expected subdir first, got %s", entries[0].Name)
	}
}

func TestFileService_ReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	svc := &FileService{}
	content, err := svc.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", content)
	}
}

func TestFileService_WriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(dir); err != nil {
		t.Fatalf("SetWorkspaceRoot: %v", err)
	}
	err := svc.WriteFile(path, "written content")
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "written content" {
		t.Errorf("expected 'written content', got '%s'", string(data))
	}
}

func TestFileService_CreateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(dir); err != nil {
		t.Fatalf("SetWorkspaceRoot: %v", err)
	}
	err := svc.CreateFile(path)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Size() != 0 {
		t.Errorf("expected empty file, got size %d", info.Size())
	}
}

func TestFileService_CreateDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c")

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(dir); err != nil {
		t.Fatalf("SetWorkspaceRoot: %v", err)
	}
	err := svc.CreateDirectory(path)
	if err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}
	info, _ := os.Stat(path)
	if !info.IsDir() {
		t.Error("expected directory to exist")
	}
}

func TestFileService_DeletePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gone.txt")
	os.WriteFile(path, []byte("x"), 0644)

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(dir); err != nil {
		t.Fatalf("SetWorkspaceRoot: %v", err)
	}
	err := svc.DeletePath(path)
	if err != nil {
		t.Fatalf("DeletePath failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestFileService_RenamePath(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	os.WriteFile(oldPath, []byte("x"), 0644)

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(dir); err != nil {
		t.Fatalf("SetWorkspaceRoot: %v", err)
	}
	err := svc.RenamePath(oldPath, newPath)
	if err != nil {
		t.Fatalf("RenamePath failed: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Error("expected new file to exist")
	}
}

// --- Path sandboxing tests ---

func TestFileService_NoWorkspace_RejectsWrite(t *testing.T) {
	// prompt-6 Task 4 / BUG-M5: empty root must refuse mutations.
	dir := t.TempDir()
	path := filepath.Join(dir, "free.txt")
	svc := &FileService{}
	if err := svc.WriteFile(path, "data"); err == nil {
		t.Fatal("WriteFile should fail without workspace root")
	}
	if err := svc.CreateFile(path); err == nil {
		t.Fatal("CreateFile should fail without workspace root")
	}
	if err := svc.DeletePath(path); err == nil {
		t.Fatal("DeletePath should fail without workspace root")
	}
	// Read remains allowed (no sandbox when root empty) for open-file UX
	// outside a project; create a real file via os then read.
	if werr := os.WriteFile(path, []byte("x"), 0644); werr != nil {
		t.Fatalf("seed file: %v", werr)
	}
	if _, err := svc.ReadFile(path); err != nil {
		t.Fatalf("ReadFile should still work without workspace: %v", err)
	}
}

func TestFileService_WorkspaceAllowsInsidePath(t *testing.T) {
	workspace := t.TempDir()
	innerFile := filepath.Join(workspace, "inside.txt")

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	if err := svc.WriteFile(innerFile, "data"); err != nil {
		t.Fatalf("WriteFile inside workspace should succeed: %v", err)
	}
}

func TestFileService_WorkspaceRejectsOutsidePath(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.txt")

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	if err := svc.WriteFile(outsideFile, "data"); err == nil {
		t.Error("WriteFile outside workspace should fail")
	}
	if _, err := svc.ReadFile(outsideFile); err == nil {
		t.Error("ReadFile outside workspace should fail")
	}
	if err := svc.CreateFile(outsideFile); err == nil {
		t.Error("CreateFile outside workspace should fail")
	}
	if err := svc.DeletePath(outsideFile); err == nil {
		t.Error("DeletePath outside workspace should fail")
	}
}

func TestFileService_WorkspaceRejectsTraversalPath(t *testing.T) {
	workspace := t.TempDir()
	// Create a subdirectory and try to traverse up
	os.Mkdir(filepath.Join(workspace, "subdir"), 0755)
	traversalPath := filepath.Join(workspace, "subdir", "..", "..", "etc", "passwd")

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	if _, err := svc.ReadFile(traversalPath); err == nil {
		t.Error("ReadFile with traversal path should fail")
	}
}

func TestFileService_SetWorkspaceRootInvalidPath(t *testing.T) {
	svc := &FileService{}
	if err := svc.SetWorkspaceRoot("/nonexistent/path/xyz"); err == nil {
		t.Error("SetWorkspaceRoot with non-existent path should fail")
	}
}

func TestFileService_SetWorkspaceRootEmptyRejectsWrite(t *testing.T) {
	// prompt-6 Task 4: clearing workspace root re-enables the empty-root write ban.
	workspace := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "free.txt")

	svc := &FileService{}
	svc.SetWorkspaceRoot(workspace)
	svc.SetWorkspaceRoot("")
	if err := svc.WriteFile(outsideFile, "data"); err == nil {
		t.Error("WriteFile should fail after clearing workspace root")
	}
}

func TestFileService_RenamePathBothMustBeInside(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	insideFile := filepath.Join(workspace, "inside.txt")
	outsideFile := filepath.Join(outside, "outside.txt")
	os.WriteFile(insideFile, []byte("x"), 0644)

	svc := &FileService{}
	svc.SetWorkspaceRoot(workspace)
	// Rename from inside to outside should fail
	if err := svc.RenamePath(insideFile, outsideFile); err == nil {
		t.Error("RenamePath from inside to outside should fail")
	}
}

func TestFileService_ListDirectory_RespectsSandbox(t *testing.T) {
	fs := NewFileService()
	root := t.TempDir()
	fs.SetWorkspaceRoot(root)

	os.WriteFile(filepath.Join(root, "inside.txt"), []byte("hello"), 0644)

	// List inside workspace — should work
	entries, err := fs.ListDirectory(root)
	if err != nil {
		t.Fatalf("ListDirectory inside workspace failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	// List outside workspace — should fail
	outside := t.TempDir()
	_, err = fs.ListDirectory(outside)
	if err == nil {
		t.Fatal("expected error for listing outside workspace")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("error should mention 'outside', got: %v", err)
	}
}

func TestFileService_ListDirectory_NoRootAllowsAny(t *testing.T) {
	fs := NewFileService()
	// No workspace root set — any directory should be allowed
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("x"), 0644)
	entries, err := fs.ListDirectory(dir)
	if err != nil {
		t.Fatalf("expected no error when no workspace root set, got: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

// --- Plan 55: ListAllFiles tests ---

func TestFileService_ListAllFiles_BasicWalk(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "c.ts"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "d.py"), []byte("x"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	expected := []string{"a.txt", "b.go", "sub/c.ts", "sub/deep/d.py"}
	if len(files) != len(expected) {
		t.Fatalf("expected %d files, got %d: %v", len(expected), len(files), files)
	}
	for i, f := range files {
		if f != expected[i] {
			t.Errorf("file[%d] = %q, want %q", i, f, expected[i])
		}
	}
	// Result should be sorted.
	for i := 1; i < len(files); i++ {
		if files[i-1] > files[i] {
			t.Errorf("result is not sorted: %q > %q", files[i-1], files[i])
		}
	}
}

func TestFileService_ListAllFiles_UsesForwardSlashes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src", "util"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "util", "helper.go"), []byte("x"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0] != "src/util/helper.go" {
		t.Errorf("expected forward slashes, got %q", files[0])
	}
}

func TestFileService_ListAllFiles_SkipsHiddenFilesAndDirs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	// .hidden and .git/* should be skipped. .gitignore is also hidden, so skipped.
	if len(files) != 1 {
		t.Fatalf("expected 1 visible file, got %d: %v", len(files), files)
	}
	if files[0] != "visible.txt" {
		t.Errorf("expected visible.txt, got %q", files[0])
	}
}

func TestFileService_ListAllFiles_SkipsIgnoreDirs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.go"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "dist"), 0755)
	os.WriteFile(filepath.Join(dir, "dist", "bundle.js"), []byte("x"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (keep.go), got %d: %v", len(files), files)
	}
	if files[0] != "keep.go" {
		t.Errorf("expected keep.go, got %q", files[0])
	}
}

func TestFileService_ListAllFiles_RespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nbuild/\n/temp\n"), 0644)
	os.WriteFile(filepath.Join(dir, "keep.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "debug.log"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "build"), 0755)
	os.WriteFile(filepath.Join(dir, "build", "out.js"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "temp"), 0755)
	os.WriteFile(filepath.Join(dir, "temp", "tmp.txt"), []byte("x"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	// .gitignore is hidden so skipped by the hidden rule. *.log, build/,
	// /temp should be skipped by gitignore. Only keep.go remains.
	if len(files) != 1 {
		t.Fatalf("expected 1 file (keep.go), got %d: %v", len(files), files)
	}
	if files[0] != "keep.go" {
		t.Errorf("expected keep.go, got %q", files[0])
	}
}

func TestFileService_ListAllFiles_GitignoreWildcardSegment(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.min.js\n"), 0644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "vendor.min.js"), []byte("x"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (app.js), got %d: %v", len(files), files)
	}
	if files[0] != "app.js" {
		t.Errorf("expected app.js, got %q", files[0])
	}
}

func TestFileService_ListAllFiles_GitignoreNegation(t *testing.T) {
	dir := t.TempDir()
	// *.log is ignored, but important.log is re-included with !.
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n!important.log\n"), 0644)
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "debug.log"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "important.log"), []byte("x"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	want := map[string]bool{"app.go": true, "important.log": true}
	if len(files) != len(want) {
		t.Fatalf("expected %d files, got %d: %v", len(want), len(files), files)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q", f)
		}
	}
}

func TestFileService_ListAllFiles_GitignoreAnchoredPattern(t *testing.T) {
	dir := t.TempDir()
	// /gen is anchored — only matches a top-level gen dir, not nested ones.
	// ("gen" is NOT in the hardcoded quickOpenIgnoreDirs list, so only the
	// gitignore anchored pattern controls skipping here.)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/gen/\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "gen"), 0755)
	os.WriteFile(filepath.Join(dir, "gen", "out.js"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "src", "gen"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "gen", "keep.js"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("x"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	want := map[string]bool{"root.txt": true, "src/gen/keep.js": true}
	if len(files) != len(want) {
		t.Fatalf("expected %d files, got %d: %v", len(want), len(files), files)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q (anchored /gen/ should only skip top-level gen)", f)
		}
	}
}

func TestFileService_ListAllFiles_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d: %v", len(files), files)
	}
}

func TestFileService_ListAllFiles_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("x"), 0644)
	svc := &FileService{}
	if _, err := svc.ListAllFiles(file); err == nil {
		t.Error("expected error when ListAllFiles is called on a file, got nil")
	}
}

func TestFileService_ListAllFiles_RespectsSandbox(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "free.txt"), []byte("x"), 0644)

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	if _, err := svc.ListAllFiles(outside); err == nil {
		t.Error("expected error for ListAllFiles outside workspace")
	}
}

func TestFileService_ListAllFiles_NestedGitignoreNotLoaded(t *testing.T) {
	// Only the root .gitignore is loaded — nested .gitignore files in
	// subdirectories are NOT loaded (documented limitation). This test
	// documents that behavior so it doesn't silently change.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	// Nested .gitignore that would ignore *.go — but it should NOT be applied
	// because we only load the root .gitignore.
	os.WriteFile(filepath.Join(dir, "sub", ".gitignore"), []byte("*.go\n"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "keep.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "debug.log"), []byte("x"), 0644)

	svc := &FileService{}
	files, err := svc.ListAllFiles(dir)
	if err != nil {
		t.Fatalf("ListAllFiles failed: %v", err)
	}
	// *.log is ignored by root .gitignore. *.go in sub/.gitignore is NOT applied.
	want := map[string]bool{"sub/keep.go": true}
	if len(files) != len(want) {
		t.Fatalf("expected %d files, got %d: %v", len(want), len(files), files)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q", f)
		}
	}
}

func TestMatchSegment(t *testing.T) {
	cases := []struct {
		seg, pattern string
		want         bool
	}{
		{"foo", "foo", true},
		{"foo", "bar", false},
		{"foo.js", "*.js", true},
		{"foo.ts", "*.js", false},
		{"vendor.min.js", "*.min.js", true},
		{"a.b.c", "a.*.c", true},
		{"a.b.c", "a.*.d", false},
		{"", "*", true},
		{"abc", "abc*", true},
		{"abc", "*abc", true},
		{"xabcx", "*abc*", true},
		{"xabcy", "*abcd*", false},
		{"foo", "foo*", true},
		{"foobar", "foo*", true},
		{"barfoo", "*foo", true},
	}
	for _, c := range cases {
		got := matchSegment(c.seg, c.pattern)
		if got != c.want {
			t.Errorf("matchSegment(%q, %q) = %v, want %v", c.seg, c.pattern, got, c.want)
		}
	}
}

func TestLoadGitignorePatterns_EmptyAndComments(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# comment\n\n   \n*.log\n"), 0644)
	patterns := loadGitignorePatterns(dir)
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].segments[0] != "*.log" {
		t.Errorf("expected *.log, got %q", patterns[0].segments[0])
	}
}

func TestLoadGitignorePatterns_NoFile(t *testing.T) {
	dir := t.TempDir()
	patterns := loadGitignorePatterns(dir)
	if patterns != nil {
		t.Errorf("expected nil when .gitignore is absent, got %v", patterns)
	}
}

// --- N-56: Symlink path traversal tests ---
//
// On Windows, creating symlinks requires either administrator privileges
// or Developer Mode enabled. We attempt to create one and skip the test
// if the OS refuses — this keeps the suite portable.

// trySymlinkOrFail skips the test if the OS refuses to create a symlink
// at linkPath pointing at target. Returns true if the test should
// continue.
//
// N-56 note: on Windows, os.Symlink may return nil even when the user
// lacks the SeCreateSymbolicLinkPrivilege (or Developer Mode) — the
// symlink is silently NOT created. We therefore verify with Lstat
// after creation and skip the test if the link doesn't actually exist.
func trySymlinkOrFail(t *testing.T, linkPath, target string) bool {
	t.Helper()
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skipf("symlink creation failed (likely missing privileges on Windows): %v", err)
		return false
	}
	if _, err := os.Lstat(linkPath); err != nil {
		t.Skipf("symlink was not actually created (likely missing privileges on Windows): %v", err)
		return false
	}
	return true
}

func TestFileService_N56_RejectsSymlinkEscapingWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	// outside/secret.txt
	outsideFile := filepath.Join(outside, "secret.txt")
	os.WriteFile(outsideFile, []byte("top-secret"), 0644)
	// workspace/link -> outside/secret.txt
	linkPath := filepath.Join(workspace, "link")
	if !trySymlinkOrFail(t, linkPath, outsideFile) {
		return
	}

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// ReadFile via the symlink should be rejected — the symlink resolves
	// to a path outside the workspace.
	if _, err := svc.ReadFile(linkPath); err == nil {
		t.Error("ReadFile via symlink escaping workspace should fail (N-56)")
	}
	// WriteFile via the symlink should also be rejected.
	if err := svc.WriteFile(linkPath, "tampered"); err == nil {
		t.Error("WriteFile via symlink escaping workspace should fail (N-56)")
	}
}

func TestFileService_N56_AllowsSymlinkInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	// workspace/real.txt
	realFile := filepath.Join(workspace, "real.txt")
	os.WriteFile(realFile, []byte("ok"), 0644)
	// workspace/link -> workspace/real.txt
	linkPath := filepath.Join(workspace, "link")
	if !trySymlinkOrFail(t, linkPath, realFile) {
		return
	}

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// Reading through a symlink that resolves INSIDE the workspace is fine.
	data, err := svc.ReadFile(linkPath)
	if err != nil {
		t.Fatalf("ReadFile via symlink inside workspace should succeed: %v", err)
	}
	if data != "ok" {
		t.Errorf("expected 'ok', got %q", data)
	}
}

func TestFileService_N56_RejectsSymlinkDirEscapingWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	// outside/secret.txt
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0644)
	// workspace/links -> outside  (symlink to a directory)
	linkDir := filepath.Join(workspace, "links")
	if !trySymlinkOrFail(t, linkDir, outside) {
		return
	}

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// Listing through the symlinked directory should be rejected.
	if _, err := svc.ListDirectory(linkDir); err == nil {
		t.Error("ListDirectory via symlinked dir escaping workspace should fail (N-56)")
	}
}

func TestFileService_N56_RejectsTraversalThroughSymlinkedSubdir(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	// outside/secret.txt
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0644)
	// workspace/sub/escape -> outside  (symlink to a directory inside subdir)
	os.MkdirAll(filepath.Join(workspace, "sub"), 0755)
	escapeLink := filepath.Join(workspace, "sub", "escape")
	if !trySymlinkOrFail(t, escapeLink, outside) {
		return
	}

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// Accessing through the symlinked subdir should be rejected.
	target := filepath.Join(workspace, "sub", "escape", "secret.txt")
	if _, err := svc.ReadFile(target); err == nil {
		t.Error("ReadFile via symlinked subdir escaping workspace should fail (N-56)")
	}
}

func TestFileService_N56_CreateFileThroughSymlinkedParentRejected(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	// workspace/links -> outside  (symlink to outside dir)
	linkDir := filepath.Join(workspace, "links")
	if !trySymlinkOrFail(t, linkDir, outside) {
		return
	}

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// Creating a file through the symlinked parent dir would write to
	// outside/evil.txt — must be rejected even though the file doesn't
	// exist yet (evalSymlinksAllowMissing resolves the parent).
	target := filepath.Join(linkDir, "evil.txt")
	if err := svc.CreateFile(target); err == nil {
		t.Error("CreateFile through symlinked parent escaping workspace should fail (N-56)")
	}
}

// --- N-56: Plugin service symlink tests ---

func TestPluginService_N56_RejectsSymlinkEscapingPluginDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping plugin symlink test in short mode")
	}
	tmp := t.TempDir()
	// Plugins are discovered at <projectRoot>/.nknk/plugins/<name>/plugin.json
	projectDir := filepath.Join(tmp, ".nknk", "plugins", "myplugin")
	os.MkdirAll(projectDir, 0755)
	// Write a minimal plugin.json manifest.
	manifest := []byte(`{"name":"myplugin","version":"1.0.0","main":"main.js"}`)
	os.WriteFile(filepath.Join(projectDir, "plugin.json"), manifest, 0644)

	// outside/secret.js
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.js")
	os.WriteFile(outsideFile, []byte("evil"), 0644)

	// projectDir/link.js -> outside/secret.js
	linkPath := filepath.Join(projectDir, "link.js")
	if !trySymlinkOrFail(t, linkPath, outsideFile) {
		return
	}

	svc := &PluginService{}
	_, err := svc.ReadPluginFile("myplugin", "link.js", tmp)
	if err == nil {
		t.Error("ReadPluginFile via symlink escaping plugin dir should fail (N-56)")
	}
}

func TestPluginService_N56_AllowsSymlinkInsidePluginDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping plugin symlink test in short mode")
	}
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, ".nknk", "plugins", "myplugin")
	os.MkdirAll(projectDir, 0755)
	manifest := []byte(`{"name":"myplugin","version":"1.0.0","main":"main.js"}`)
	os.WriteFile(filepath.Join(projectDir, "plugin.json"), manifest, 0644)

	// projectDir/real.js (real file inside the plugin dir)
	realFile := filepath.Join(projectDir, "real.js")
	os.WriteFile(realFile, []byte("ok"), 0644)
	// projectDir/link.js -> projectDir/real.js (symlink inside plugin dir)
	linkPath := filepath.Join(projectDir, "link.js")
	if !trySymlinkOrFail(t, linkPath, realFile) {
		return
	}

	svc := &PluginService{}
	data, err := svc.ReadPluginFile("myplugin", "link.js", tmp)
	if err != nil {
		t.Fatalf("ReadPluginFile via symlink inside plugin dir should succeed: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("expected 'ok', got %q", string(data))
	}
}

// --- N-56: evalSymlinksAllowMissing unit tests (no symlink privileges required) ---

func TestEvalSymlinksAllowMissing_ExistingPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "exists.txt")
	os.WriteFile(file, []byte("x"), 0644)
	// For an existing path, the helper should behave like EvalSymlinks.
	got, err := evalSymlinksAllowMissing(file)
	if err != nil {
		t.Fatalf("evalSymlinksAllowMissing failed: %v", err)
	}
	expected, _ := filepath.EvalSymlinks(file)
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestEvalSymlinksAllowMissing_NonExistentFileWithExistentParent(t *testing.T) {
	dir := t.TempDir()
	// Non-existent file under an existing parent directory.
	file := filepath.Join(dir, "newfile.txt")
	got, err := evalSymlinksAllowMissing(file)
	if err != nil {
		t.Fatalf("evalSymlinksAllowMissing failed: %v", err)
	}
	// Should resolve the parent and rejoin with the basename.
	expectedParent, _ := filepath.EvalSymlinks(dir)
	expected := filepath.Join(expectedParent, "newfile.txt")
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestEvalSymlinksAllowMissing_NonExistentParent(t *testing.T) {
	dir := t.TempDir()
	// Both the file and its parent don't exist.
	file := filepath.Join(dir, "missing-subdir", "newfile.txt")
	got, err := evalSymlinksAllowMissing(file)
	if err != nil {
		t.Fatalf("evalSymlinksAllowMissing failed: %v", err)
	}
	// Should fall back to lexical resolution (parent missing → no
	// symlinks to follow).
	expected := filepath.Join(dir, "missing-subdir", "newfile.txt")
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestFileService_N56_RejectsTraversalPath_Lexical(t *testing.T) {
	// This test does NOT require symlink privileges — it verifies the
	// existing lexical traversal check still works (defense in depth).
	workspace := t.TempDir()
	os.Mkdir(filepath.Join(workspace, "subdir"), 0755)
	traversalPath := filepath.Join(workspace, "subdir", "..", "..", "outside.txt")

	svc := &FileService{}
	if err := svc.SetWorkspaceRoot(workspace); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	if _, err := svc.ReadFile(traversalPath); err == nil {
		t.Error("ReadFile with lexical traversal should fail")
	}
	if err := svc.WriteFile(traversalPath, "data"); err == nil {
		t.Error("WriteFile with lexical traversal should fail")
	}
}
