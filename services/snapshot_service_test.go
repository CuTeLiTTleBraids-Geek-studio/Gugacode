package services

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// newTestSnapshotService 创建测试用快照服务（临时目录）。
func newTestSnapshotService(t *testing.T) (*SnapshotService, string) {
	t.Helper()
	tmpDir := t.TempDir()
	svc := NewSnapshotService(tmpDir)
	return svc, tmpDir
}

// createTestWorkspace 创建测试工作区。
func createTestWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	// 创建几个测试文件
	files := map[string]string{
		"main.go":         "package main\n\nfunc main() {}\n",
		"README.md":       "# Test Project\n",
		"src/utils.go":    "package utils\n\nfunc Helper() {}\n",
		"src/const.go":    "package utils\n\nconst Version = \"1.0\"\n",
		".gitignore":      "node_modules/\n",
	}
	for path, content := range files {
		fullPath := filepath.Join(ws, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	// 创建 .git 目录（应被跳过）
	gitDir := filepath.Join(ws, ".git", "refs")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0o644); err != nil {
		t.Fatalf("write .git/refs/HEAD: %v", err)
	}
	// 创建 node_modules（应被跳过）
	nmDir := filepath.Join(ws, "node_modules", "somepkg")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "index.js"), []byte("module.exports = {};"), 0o644); err != nil {
		t.Fatalf("write node_modules: %v", err)
	}
	return ws
}

func TestSnapshot_CreateSnapshot(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	snap, err := svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if snap.ID == "" {
		t.Error("expected non-empty ID")
	}
	if snap.Reason != SnapshotReasonManual {
		t.Errorf("expected reason manual, got %s", snap.Reason)
	}
	// 应包含 5 个文件（排除 .git 和 node_modules）
	if snap.FileCount != 5 {
		t.Errorf("expected 5 files, got %d", snap.FileCount)
	}
	if len(snap.Files) != 5 {
		t.Errorf("expected 5 file entries, got %d", len(snap.Files))
	}
	// 每个文件应有 hash
	for _, fs := range snap.Files {
		if fs.Hash == "" {
			t.Errorf("file %s has empty hash", fs.Path)
		}
		if !isValidHash(fs.Hash) {
			t.Errorf("file %s has invalid hash: %s", fs.Path, fs.Hash)
		}
	}
}

func TestSnapshot_CreateSnapshot_SkipsGitAndNodeModules(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	snap, err := svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	// 验证 .git 文件不在快照中
	for _, fs := range snap.Files {
		if fs.Path == ".git/refs/HEAD" {
			t.Error(".git file should be excluded from snapshot")
		}
		if fs.Path == "node_modules/somepkg/index.js" {
			t.Error("node_modules file should be excluded from snapshot")
		}
	}
}

func TestSnapshot_ListSnapshots(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	snap1, _ := svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	time.Sleep(1 * time.Millisecond)
	snap2, _ := svc.CreateSnapshot(ws, string(SnapshotReasonPlanStep))

	list, err := svc.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(list))
	}
	// 按创建时间降序：snap2 应在前
	if list[0].ID != snap2.ID {
		t.Errorf("expected first snapshot %s, got %s", snap2.ID, list[0].ID)
	}
	if list[1].ID != snap1.ID {
		t.Errorf("expected second snapshot %s, got %s", snap1.ID, list[1].ID)
	}
}

func TestSnapshot_RestoreSnapshot(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	snap, err := svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// 修改文件
	mainPath := filepath.Join(ws, "main.go")
	if err := os.WriteFile(mainPath, []byte("modified content"), 0o644); err != nil {
		t.Fatalf("modify file: %v", err)
	}
	// 删除文件
	utilsPath := filepath.Join(ws, "src", "utils.go")
	_ = os.Remove(utilsPath)

	// 恢复快照
	if err := svc.RestoreSnapshot(snap.ID, ws); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	// 验证文件恢复
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read restored main.go: %v", err)
	}
	if string(data) != "package main\n\nfunc main() {}\n" {
		t.Errorf("main.go not restored correctly: %s", string(data))
	}
	// 验证删除的文件恢复
	if _, err := os.Stat(utilsPath); err != nil {
		t.Error("utils.go should be restored")
	}
}

func TestSnapshot_RestorePartial(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	snap, err := svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// 修改两个文件
	mainPath := filepath.Join(ws, "main.go")
	readmePath := filepath.Join(ws, "README.md")
	os.WriteFile(mainPath, []byte("modified main"), 0o644)
	os.WriteFile(readmePath, []byte("modified readme"), 0o644)

	// 只恢复 main.go
	if err := svc.RestorePartial(snap.ID, ws, []string{"main.go"}); err != nil {
		t.Fatalf("RestorePartial: %v", err)
	}

	// main.go 应恢复
	mainData, _ := os.ReadFile(mainPath)
	if string(mainData) != "package main\n\nfunc main() {}\n" {
		t.Error("main.go should be restored")
	}
	// README.md 应保持修改
	readmeData, _ := os.ReadFile(readmePath)
	if string(readmeData) != "modified readme" {
		t.Error("README.md should remain modified")
	}
}

func TestSnapshot_DeleteSnapshot(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	snap1, _ := svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	snap2, _ := svc.CreateSnapshot(ws, string(SnapshotReasonPlanStep))

	// 删除 snap1
	if err := svc.DeleteSnapshot(snap1.ID); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	list, _ := svc.ListSnapshots()
	if len(list) != 1 {
		t.Fatalf("expected 1 snapshot after delete, got %d", len(list))
	}
	if list[0].ID != snap2.ID {
		t.Errorf("expected remaining snapshot %s, got %s", snap2.ID, list[0].ID)
	}
}

func TestSnapshot_DeleteSnapshot_NotFound(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	err := svc.DeleteSnapshot("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent snapshot")
	}
}

func TestSnapshot_DiffSnapshots(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	// 第一个快照
	snap1, _ := svc.CreateSnapshot(ws, string(SnapshotReasonManual))

	// 修改文件
	os.WriteFile(filepath.Join(ws, "main.go"), []byte("modified"), 0o644)
	// 新增文件
	os.WriteFile(filepath.Join(ws, "new.go"), []byte("new file"), 0o644)
	// 删除文件
	os.Remove(filepath.Join(ws, "README.md"))

	// 第二个快照
	snap2, _ := svc.CreateSnapshot(ws, string(SnapshotReasonManual))

	diff, err := svc.DiffSnapshots(snap1.ID, snap2.ID)
	if err != nil {
		t.Fatalf("DiffSnapshots: %v", err)
	}
	if len(diff.Added) != 1 || diff.Added[0] != "new.go" {
		t.Errorf("expected added [new.go], got %v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0] != "README.md" {
		t.Errorf("expected removed [README.md], got %v", diff.Removed)
	}
	if len(diff.Modified) != 1 || diff.Modified[0] != "main.go" {
		t.Errorf("expected modified [main.go], got %v", diff.Modified)
	}
}

func TestSnapshot_DiffSnapshots_NoChange(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	snap1, _ := svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	snap2, _ := svc.CreateSnapshot(ws, string(SnapshotReasonManual))

	diff, err := svc.DiffSnapshots(snap1.ID, snap2.ID)
	if err != nil {
		t.Fatalf("DiffSnapshots: %v", err)
	}
	if len(diff.Added) != 0 || len(diff.Removed) != 0 || len(diff.Modified) != 0 {
		t.Errorf("expected no changes, got added=%v removed=%v modified=%v",
			diff.Added, diff.Removed, diff.Modified)
	}
}

func TestSnapshot_ContentAddressableDedup(t *testing.T) {
	// Step 4: 相同内容不应重复存储
	svc, tmpDir := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	// 创建两个快照（相同内容）
	svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	svc.CreateSnapshot(ws, string(SnapshotReasonManual))

	// 检查 blob 目录中的文件数
	blobDir := filepath.Join(tmpDir, "gugacode", "snapshots", "blobs")
	entries, err := os.ReadDir(blobDir)
	if err != nil {
		t.Fatalf("read blob dir: %v", err)
	}
	// 应该只有 5 个 blob（5 个不同文件），不是 10 个
	if len(entries) != 5 {
		t.Errorf("expected 5 blobs (deduplicated), got %d", len(entries))
	}
}

func TestSnapshot_CleanupSnapshots_KeepN(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	// 创建 5 个快照
	for i := 0; i < 5; i++ {
		_, err := svc.CreateSnapshot(ws, string(SnapshotReasonManual))
		if err != nil {
			t.Fatalf("CreateSnapshot %d: %v", i, err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	// 保留最近 2 个
	deleted, err := svc.CleanupSnapshots(2, 0)
	if err != nil {
		t.Fatalf("CleanupSnapshots: %v", err)
	}
	if deleted != 3 {
		t.Errorf("expected 3 deleted, got %d", deleted)
	}
	list, _ := svc.ListSnapshots()
	if len(list) != 2 {
		t.Errorf("expected 2 remaining, got %d", len(list))
	}
}

func TestSnapshot_CleanupSnapshots_MaxAge(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	// 创建快照（第一个会被判定为过期）
	svc.CreateSnapshot(ws, string(SnapshotReasonManual))
	// sleep 足够长时间确保第一个快照超过 maxAge，
	// 同时 maxAge 足够大确保第二个快照不会因 IO 延迟被误判过期
	time.Sleep(200 * time.Millisecond)
	svc.CreateSnapshot(ws, string(SnapshotReasonManual))

	// 清理超过 100ms 的（应该删除第一个，保留第二个）
	deleted, err := svc.CleanupSnapshots(0, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("CleanupSnapshots: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
	list, _ := svc.ListSnapshots()
	if len(list) != 1 {
		t.Errorf("expected 1 remaining snapshot, got %d", len(list))
	}
}

func TestSnapshot_HashValidation(t *testing.T) {
	// isValidHash 验证
	validHash := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	if !isValidHash(validHash) {
		t.Error("valid hash rejected")
	}
	// 太短
	if isValidHash("abc123") {
		t.Error("short hash should be rejected")
	}
	// 含非十六进制字符
	if isValidHash("g1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2") {
		t.Error("hash with non-hex chars should be rejected")
	}
}

func TestSnapshot_GetSnapshot(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	snap, _ := svc.CreateSnapshot(ws, string(SnapshotReasonManual))

	found, err := svc.GetSnapshot(snap.ID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if found.ID != snap.ID {
		t.Errorf("expected ID %s, got %s", snap.ID, found.ID)
	}
}

func TestSnapshot_GetSnapshot_NotFound(t *testing.T) {
	svc, _ := newTestSnapshotService(t)
	_, err := svc.GetSnapshot("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent snapshot")
	}
}

func TestSnapshot_IsIgnoredDir(t *testing.T) {
	ignoredDirs := []string{"node_modules", "dist", "build", ".next", "target", "__pycache__"}
	for _, d := range ignoredDirs {
		if !isIgnoredDir(d) {
			t.Errorf("expected %s to be ignored", d)
		}
	}
	if isIgnoredDir("src") {
		t.Error("src should not be ignored")
	}
}

func TestSnapshot_FilePermission0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission test skipped on Windows")
	}
	svc, tmpDir := newTestSnapshotService(t)
	ws := createTestWorkspace(t)

	svc.CreateSnapshot(ws, string(SnapshotReasonManual))

	metaPath := filepath.Join(tmpDir, "gugacode", "snapshots", "metadata.json")
	info, err := os.Stat(metaPath)
	if err != nil {
		t.Fatalf("stat metadata: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected metadata file permission 0600, got %o", perm)
	}
}
