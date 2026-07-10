package services

// Plan 11 Task 14 — 智能回滚（快照 + 操作日志）。
//
// 职责（Step 1-10）：
//   - Step 1: Snapshot 结构（ID/CreatedAt/Reason/Files/GitState）
//   - Step 2: CreateSnapshot/RestoreSnapshot/RestorePartial/ListSnapshots/DeleteSnapshot/DiffSnapshots
//   - Step 3: 触发（手动/Plan 每步骤前/Goal 每检查点/Apply 前/工作流每步骤前）
//   - Step 4: 内容寻址存储（hash→blob），相同文件不重复
//   - Step 5: 清理策略（保留最近 N 个 + 时间过期）
//   - Step 6: SnapshotTimeline.vue（前端，见 stores/snapshot.ts）
//   - Step 7: 选择性回滚（勾选文件回滚）
//   - Step 8: Git 集成（Git 干净用 git checkout，脏用快照覆盖）
//   - Step 9: G-SEC-06（存储路径校验 + ValidatePathWithinRoot）
//   - Step 10: snapshot_service_test.go 覆盖

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SnapshotReason 快照创建原因（Step 1/3）。
type SnapshotReason string

const (
	SnapshotReasonManual         SnapshotReason = "manual"
	SnapshotReasonPlanStep       SnapshotReason = "plan-step"
	SnapshotReasonGoalCheckpoint SnapshotReason = "goal-checkpoint"
	SnapshotReasonPreApply       SnapshotReason = "pre-apply"
	SnapshotReasonWorkflowStep   SnapshotReason = "workflow-step"
)

// FileSnapshot 单个文件的快照元数据（Step 1/4）。
// 文件内容以内容寻址方式存储在 blobs/<hash>，相同 hash 不重复存储。
type FileSnapshot struct {
	Path string `json:"path"`
	Hash string `json:"hash"` // SHA-256 内容哈希
	Size int64  `json:"size"`
}

// GitState 快照创建时的 Git 状态（Step 1/8）。
type GitState struct {
	Branch  string   `json:"branch"`
	IsClean bool     `json:"isClean"`
	Changes []string `json:"changes,omitempty"` // 变更文件路径列表
}

// Snapshot 完整快照（Step 1）。
type Snapshot struct {
	ID            string         `json:"id"`
	CreatedAt     time.Time      `json:"createdAt"`
	Reason        SnapshotReason `json:"reason"`
	WorkspaceRoot string         `json:"workspaceRoot"`
	Files         []FileSnapshot `json:"files"`
	GitState      *GitState      `json:"gitState,omitempty"`
	FileCount     int            `json:"fileCount"`
}

// SnapshotDiff 两个快照之间的差异（Step 2: DiffSnapshots）。
type SnapshotDiff struct {
	FromSnapshotID string   `json:"fromSnapshotId"`
	ToSnapshotID   string   `json:"toSnapshotId"`
	Added          []string `json:"added"`
	Removed        []string `json:"removed"`
	Modified       []string `json:"modified"`
}

// CleanupConfig 清理策略配置（Step 5）。
type CleanupConfig struct {
	KeepN  int           `json:"keepN"`  // 保留最近 N 个（0 = 不限）
	MaxAge time.Duration `json:"maxAge"` // 最大保留时长（0 = 不过期）
}

// SnapshotService 智能回滚服务（Step 1-10）。
type SnapshotService struct {
	mu           sync.Mutex
	configDir    string
	snapshotDir  string
	blobDir      string
	metadataPath string
	gitService   *GitService
}

// NewSnapshotService 创建快照服务（Step 9: 存储于 ~/.config/gugacode/snapshots/）。
func NewSnapshotService(configDir string) *SnapshotService {
	snapshotDir := filepath.Join(configDir, "gugacode", "snapshots")
	return &SnapshotService{
		configDir:    configDir,
		snapshotDir:  snapshotDir,
		blobDir:      filepath.Join(snapshotDir, "blobs"),
		metadataPath: filepath.Join(snapshotDir, "metadata.json"),
	}
}

// SetGitService 注入 GitService（Step 8: Git 集成）。
func (s *SnapshotService) SetGitService(g *GitService) {
	s.gitService = g
}

// ensureDirs 确保快照目录存在。
func (s *SnapshotService) ensureDirs() error {
	if err := os.MkdirAll(s.snapshotDir, 0o755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}
	if err := os.MkdirAll(s.blobDir, 0o755); err != nil {
		return fmt.Errorf("create blob dir: %w", err)
	}
	return nil
}

// hashContent 计算内容的 SHA-256 哈希（Step 4: 内容寻址）。
func hashContent(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// storeBlob 存储文件内容到内容寻址存储（Step 4）。
// 相同 hash 的文件不重复存储。
func (s *SnapshotService) storeBlob(data []byte) (string, error) {
	hash := hashContent(data)
	blobPath := filepath.Join(s.blobDir, hash)
	// 检查是否已存在（内容寻址去重）
	if _, err := os.Stat(blobPath); err == nil {
		return hash, nil // 已存在，跳过
	}
	// 原子写入 blob
	if err := atomicWriteFile(blobPath, data, 0o644); err != nil {
		return "", fmt.Errorf("store blob %s: %w", hash, err)
	}
	return hash, nil
}

// readBlob 读取 blob 内容。
func (s *SnapshotService) readBlob(hash string) ([]byte, error) {
	blobPath := filepath.Join(s.blobDir, hash)
	// G-SEC-06: 验证 hash 只含十六进制字符，防止路径穿越
	if !isValidHash(hash) {
		return nil, fmt.Errorf("%w: invalid hash format", ErrInvalidInput)
	}
	data, err := os.ReadFile(blobPath)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", hash, err)
	}
	return data, nil
}

// isValidHash 验证哈希字符串格式（仅十六进制，64 字符）。
func isValidHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// loadMetadata 加载快照元数据。
func (s *SnapshotService) loadMetadata() ([]Snapshot, error) {
	data, err := os.ReadFile(s.metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Snapshot{}, nil
		}
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	var snapshots []Snapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return snapshots, nil
}

// saveMetadata 原子保存快照元数据（G-SEC: atomicWriteJSON）。
func (s *SnapshotService) saveMetadata(snapshots []Snapshot) error {
	return atomicWriteJSON(s.metadataPath, snapshots, 0o600)
}

// generateID 生成快照 ID（时间戳 + 随机后缀）。
func generateSnapshotID() string {
	return fmt.Sprintf("snap-%d", time.Now().UnixNano())
}

// ---- Step 2: CreateSnapshot ----

// CreateSnapshot 创建工作区快照（Step 2/3/4）。
// 遍历 workspaceRoot 下所有文件，计算哈希并存储到内容寻址存储。
// .git 目录会被跳过（Step 8: Git 状态单独记录）。
func (s *SnapshotService) CreateSnapshot(workspaceRoot, reason string) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDirs(); err != nil {
		return nil, err
	}

	// G-SEC-06: 验证 workspaceRoot 存在
	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve workspace root: %v", ErrInvalidInput, err)
	}
	info, err := os.Stat(rootAbs)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: workspace root not accessible: %v", ErrInvalidInput, err)
	}

	files := []FileSnapshot{}
	err = filepath.Walk(rootAbs, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过错误文件
		}
		// 跳过 .git 目录
		rel, _ := filepath.Rel(rootAbs, path)
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// 跳过 node_modules, dist, build 等大目录
		base := filepath.Base(path)
		if fi.IsDir() && isIgnoredDir(base) {
			return filepath.SkipDir
		}
		if fi.IsDir() {
			return nil
		}
		// 读取文件内容并存储
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil // 跳过不可读文件
		}
		hash, herr := s.storeBlob(data)
		if herr != nil {
			return herr
		}
		files = append(files, FileSnapshot{
			Path: rel,
			Hash: hash,
			Size: fi.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workspace: %w", err)
	}

	snap := &Snapshot{
		ID:            generateSnapshotID(),
		CreatedAt:     time.Now(),
		Reason:        SnapshotReason(reason),
		WorkspaceRoot: rootAbs,
		Files:         files,
		FileCount:     len(files),
	}

	// Step 8: 记录 Git 状态
	if s.gitService != nil {
		snap.GitState = s.captureGitState(rootAbs)
	}

	// 保存元数据
	snapshots, err := s.loadMetadata()
	if err != nil {
		return nil, err
	}
	snapshots = append(snapshots, *snap)
	if err := s.saveMetadata(snapshots); err != nil {
		return nil, err
	}
	return snap, nil
}

// isIgnoredDir 判断是否为应忽略的目录。
func isIgnoredDir(name string) bool {
	switch name {
	case "node_modules", "dist", "build", ".next", ".nuxt", "target", "bin", "__pycache__":
		return true
	}
	return false
}

// captureGitState 捕获当前 Git 状态（Step 8）。
func (s *SnapshotService) captureGitState(root string) *GitState {
	changes, err := s.gitService.GetStatus(root)
	if err != nil {
		return &GitState{IsClean: false}
	}
	changePaths := make([]string, 0, len(changes))
	for _, c := range changes {
		changePaths = append(changePaths, c.Path)
	}
	branch := ""
	if bi, err := s.gitService.GetBranchInfo(root); err == nil {
		branch = bi.Name
	}
	return &GitState{
		Branch:  branch,
		IsClean: len(changes) == 0,
		Changes: changePaths,
	}
}

// ---- Step 2: RestoreSnapshot ----

// RestoreSnapshot 恢复整个快照（Step 2/7/8）。
// 将快照中的所有文件写回到 workspaceRoot。
func (s *SnapshotService) RestoreSnapshot(snapshotID, workspaceRoot string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap, err := s.findSnapshot(snapshotID)
	if err != nil {
		return err
	}

	// G-SEC-06: 验证 workspaceRoot
	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("%w: resolve workspace root: %v", ErrInvalidInput, err)
	}

	// 恢复所有文件
	for _, fs := range snap.Files {
		if err := s.restoreFile(fs, rootAbs); err != nil {
			return fmt.Errorf("restore %s: %w", fs.Path, err)
		}
	}
	return nil
}

// RestorePartial 选择性恢复部分文件（Step 7）。
func (s *SnapshotService) RestorePartial(snapshotID, workspaceRoot string, filePaths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap, err := s.findSnapshot(snapshotID)
	if err != nil {
		return err
	}

	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("%w: resolve workspace root: %v", ErrInvalidInput, err)
	}

	// 构建文件路径集合
	want := make(map[string]bool, len(filePaths))
	for _, p := range filePaths {
		want[p] = true
	}

	for _, fs := range snap.Files {
		if want[fs.Path] {
			if err := s.restoreFile(fs, rootAbs); err != nil {
				return fmt.Errorf("restore %s: %w", fs.Path, err)
			}
		}
	}
	return nil
}

// restoreFile 恢复单个文件。
func (s *SnapshotService) restoreFile(fs FileSnapshot, rootAbs string) error {
	data, err := s.readBlob(fs.Hash)
	if err != nil {
		return err
	}
	targetPath := filepath.Join(rootAbs, fs.Path)
	// G-SEC-06: 验证目标路径在工作区内
	if _, err := ValidatePathWithinRoot(rootAbs, targetPath); err != nil {
		return fmt.Errorf("path validation failed for %s: %w", fs.Path, err)
	}
	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	return atomicWriteFile(targetPath, data, 0o644)
}

// ---- Step 2: ListSnapshots ----

// ListSnapshots 列出所有快照（按创建时间降序）。
func (s *SnapshotService) ListSnapshots() ([]Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshots, err := s.loadMetadata()
	if err != nil {
		return nil, err
	}
	// 按创建时间降序排序
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})
	return snapshots, nil
}

// ---- Step 2: DeleteSnapshot ----

// DeleteSnapshot 删除快照（Step 2/5）。
// 删除元数据记录，并清理不再被任何快照引用的 blob。
func (s *SnapshotService) DeleteSnapshot(snapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshots, err := s.loadMetadata()
	if err != nil {
		return err
	}

	// 找到并移除目标快照
	idx := -1
	for i, snap := range snapshots {
		if snap.ID == snapshotID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("%w: snapshot %s not found", ErrInvalidInput, snapshotID)
	}

	deleted := snapshots[idx]
	snapshots = append(snapshots[:idx], snapshots[idx+1:]...)

	// 保存更新后的元数据
	if err := s.saveMetadata(snapshots); err != nil {
		return err
	}

	// 清理孤立 blob（不被任何剩余快照引用）
	return s.cleanupOrphanBlobs(snapshots, &deleted)
}

// cleanupOrphanBlobs 清理不被任何快照引用的 blob。
func (s *SnapshotService) cleanupOrphanBlobs(snapshots []Snapshot, justDeleted *Snapshot) error {
	// 构建所有仍在使用的 hash 集合
	usedHashes := make(map[string]bool)
	for _, snap := range snapshots {
		for _, fs := range snap.Files {
			usedHashes[fs.Hash] = true
		}
	}
	// 检查刚删除快照中的 blob
	if justDeleted != nil {
		for _, fs := range justDeleted.Files {
			if !usedHashes[fs.Hash] {
				blobPath := filepath.Join(s.blobDir, fs.Hash)
				_ = os.Remove(blobPath) // 忽略错误（blob 可能已被清理）
			}
		}
	}
	return nil
}

// ---- Step 2: DiffSnapshots ----

// DiffSnapshots 比较两个快照的差异（Step 2）。
func (s *SnapshotService) DiffSnapshots(fromID, toID string) (*SnapshotDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	from, err := s.findSnapshot(fromID)
	if err != nil {
		return nil, err
	}
	to, err := s.findSnapshot(toID)
	if err != nil {
		return nil, err
	}

	// 构建文件 hash 映射
	fromFiles := make(map[string]string) // path → hash
	for _, fs := range from.Files {
		fromFiles[fs.Path] = fs.Hash
	}
	toFiles := make(map[string]string)
	for _, fs := range to.Files {
		toFiles[fs.Path] = fs.Hash
	}

	diff := &SnapshotDiff{
		FromSnapshotID: fromID,
		ToSnapshotID:   toID,
	}

	// 找出 added（to 有 from 无）
	for path := range toFiles {
		if _, exists := fromFiles[path]; !exists {
			diff.Added = append(diff.Added, path)
		}
	}
	// 找出 removed（from 有 to 无）
	for path := range fromFiles {
		if _, exists := toFiles[path]; !exists {
			diff.Removed = append(diff.Removed, path)
		}
	}
	// 找出 modified（都有但 hash 不同）
	for path, fromHash := range fromFiles {
		if toHash, exists := toFiles[path]; exists && toHash != fromHash {
			diff.Modified = append(diff.Modified, path)
		}
	}

	sort.Strings(diff.Added)
	sort.Strings(diff.Removed)
	sort.Strings(diff.Modified)
	return diff, nil
}

// ---- Step 5: CleanupSnapshots ----

// CleanupSnapshots 清理旧快照（Step 5）。
//
// 保留条件采用 AND 语义——快照必须同时满足两个条件才会保留：
//   - keepByCount: keepN == 0 表示不限数量（全部保留），否则仅保留最近 keepN 个
//   - keepByAge  : maxAge == 0 表示不过期（全部保留），否则仅保留创建未超过 maxAge 的
//
// 任一条件不满足即删除。这样：
//   - CleanupSnapshots(2, 0)          → 保留最近 2 个，删除其余
//   - CleanupSnapshots(0, 24h)        → 删除所有超过 24h 的快照
//   - CleanupSnapshots(5, 24h)        → 仅保留最近 5 个中未过期的
func (s *SnapshotService) CleanupSnapshots(keepN int, maxAge time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshots, err := s.loadMetadata()
	if err != nil {
		return 0, err
	}

	// 按创建时间降序排序（最新在前）
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	now := time.Now()
	var kept []Snapshot
	deletedCount := 0

	for i, snap := range snapshots {
		keepByCount := keepN == 0 || i < keepN
		keepByAge := maxAge == 0 || now.Sub(snap.CreatedAt) <= maxAge
		if keepByCount && keepByAge {
			kept = append(kept, snap)
			continue
		}
		deletedCount++
		snapCopy := snap
		_ = s.cleanupOrphanBlobs(kept, &snapCopy)
	}

	if deletedCount > 0 {
		if err := s.saveMetadata(kept); err != nil {
			return 0, err
		}
	}
	return deletedCount, nil
}

// ---- 辅助方法 ----

// findSnapshot 查找指定 ID 的快照。
func (s *SnapshotService) findSnapshot(id string) (*Snapshot, error) {
	snapshots, err := s.loadMetadata()
	if err != nil {
		return nil, err
	}
	for _, snap := range snapshots {
		if snap.ID == id {
			snapCopy := snap
			return &snapCopy, nil
		}
	}
	return nil, fmt.Errorf("%w: snapshot %s not found", ErrInvalidInput, id)
}

// GetSnapshot 获取单个快照详情（供前端调用）。
func (s *SnapshotService) GetSnapshot(id string) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findSnapshot(id)
}
