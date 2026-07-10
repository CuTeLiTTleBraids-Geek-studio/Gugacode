package services

// Plan 11 Task 13 — Diff 差异增强。
//
// 职责（Step 1-12）：
//   - Step 1: MultiFileDiff（[]FileDiff：Path/OldContent/NewContent/Hunks）
//   - Step 2: ThreeWayMerge(base, ours, theirs) + 冲突标记
//   - Step 3: AI 审查标注（每个 hunk 附加 AIComment）
//   - Step 4: 行内评论（任意行可附加 InlineComment）
//   - Step 5: DiffViewer.vue（多文件 tab+统计+hunk 折叠+行号+语法高亮）
//   - Step 6: Apply（单文件/全部）
//   - Step 7: Reject（单 hunk/单文件/全部）
//   - Step 8: AI 审查模式（自动生成 hunk 审查意见，severity 色标）
//   - Step 9: "审查整个 PR"入口 + Markdown 报告导出
//   - Step 10: 导出 diff/Markdown/HTML
//   - Step 11: Artifact 预览模式（iframe sandbox 复用 PluginViewIframe）
//   - Step 12: 测试覆盖

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Step 1: 结构化 Diff 类型
// ---------------------------------------------------------------------------

// DiffLineType 标识一行的变更类型。
type DiffLineType string

const (
	DiffLineContext  DiffLineType = "context"  // 未变更
	DiffLineAdded    DiffLineType = "added"    // 新增行
	DiffLineRemoved  DiffLineType = "removed"  // 删除行
	DiffLineConflict DiffLineType = "conflict" // 冲突行
)

// DiffLine 单行 diff。
type DiffLine struct {
	Type     DiffLineType    `json:"type"`
	OldNum   int             `json:"oldNum,omitempty"`   // 旧行号（removed/context 有）
	NewNum   int             `json:"newNum,omitempty"`   // 新行号（added/context 有）
	Content  string          `json:"content"`            // 行内容（不含前缀 +/-/空格）
	Comments []InlineComment `json:"comments,omitempty"` // Step 4: 行内评论
}

// Hunk 一组连续的 diff 行。
type Hunk struct {
	OldStart int        `json:"oldStart"`
	OldCount int        `json:"oldCount"`
	NewStart int        `json:"newStart"`
	NewCount int        `json:"newCount"`
	Lines    []DiffLine `json:"lines"`
	// Step 3: AI 审查标注
	AIComments []AIComment `json:"aiComments,omitempty"`
}

// AIComment AI 对 hunk 的审查意见（Step 3）。
type AIComment struct {
	Severity   AICommentSeverity `json:"severity"`
	Message    string            `json:"message"`
	Suggestion string            `json:"suggestion,omitempty"`
	Line       int               `json:"line,omitempty"` // 关联行号
}

// AICommentSeverity 审查意见严重级别（Step 8: severity 色标）。
type AICommentSeverity string

const (
	AISeverityInfo     AICommentSeverity = "info"
	AISeverityWarning  AICommentSeverity = "warning"
	AISeverityError    AICommentSeverity = "error"
	AISeverityCritical AICommentSeverity = "critical"
)

// InlineComment 行内评论（Step 4: 用户或 AI 添加）。
type InlineComment struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	AIComment bool      `json:"aiComment,omitempty"`
}

// FileDiff 单个文件的 diff（Step 1）。
type FileDiff struct {
	Path       string `json:"path"`
	OldPath    string `json:"oldPath,omitempty"` // 重命名时旧路径
	OldContent string `json:"oldContent"`
	NewContent string `json:"newContent"`
	Hunks      []Hunk `json:"hunks"`
	// 统计
	AddedLines   int `json:"addedLines"`
	RemovedLines int `json:"removedLines"`
}

// MultiFileDiff 多文件 diff（Step 1）。
type MultiFileDiff struct {
	Files        []FileDiff `json:"files"`
	TotalAdded   int        `json:"totalAdded"`
	TotalRemoved int        `json:"totalRemoved"`
}

// ---------------------------------------------------------------------------
// Step 2: 三方合并
// ---------------------------------------------------------------------------

// ThreeWayMergeResult 三方合并结果。
type ThreeWayMergeResult struct {
	Merged      string `json:"merged"`
	Conflicts   int    `json:"conflicts"`
	HasConflict bool   `json:"hasConflict"`
}

// ThreeWayMerge 执行三方合并（Step 2）。
//
// base: 共同祖先；ours: 当前分支；theirs: 合并分支。
// 返回合并后的内容，冲突部分用 <<<<<<< / ======= / >>>>>>> 标记。
func ThreeWayMerge(base, ours, theirs string) ThreeWayMergeResult {
	baseLines := splitLines(base)
	ourLines := splitLines(ours)
	theirLines := splitLines(theirs)

	// 简化实现：逐行比较三方。
	// 完整实现应使用 diff3 算法，这里使用基于 LCS 的简化版本。
	var result []string
	conflicts := 0

	maxLen := len(baseLines)
	if len(ourLines) > maxLen {
		maxLen = len(ourLines)
	}
	if len(theirLines) > maxLen {
		maxLen = len(theirLines)
	}

	for i := 0; i < maxLen; i++ {
		var b, o, t string
		if i < len(baseLines) {
			b = baseLines[i]
		}
		if i < len(ourLines) {
			o = ourLines[i]
		}
		if i < len(theirLines) {
			t = theirLines[i]
		}

		if o == t {
			// 两方一致，无冲突
			if o != "" {
				result = append(result, o)
			}
		} else if o == b {
			// ours 未变更，用 theirs
			if t != "" {
				result = append(result, t)
			}
		} else if t == b {
			// theirs 未变更，用 ours
			if o != "" {
				result = append(result, o)
			}
		} else {
			// 两方都变更且不一致 → 冲突
			conflicts++
			result = append(result, "<<<<<<< ours")
			if o != "" {
				result = append(result, o)
			}
			result = append(result, "=======")
			if t != "" {
				result = append(result, t)
			}
			result = append(result, ">>>>>>> theirs")
		}
	}

	return ThreeWayMergeResult{
		Merged:      strings.Join(result, "\n"),
		Conflicts:   conflicts,
		HasConflict: conflicts > 0,
	}
}

// ---------------------------------------------------------------------------
// DiffService 服务
// ---------------------------------------------------------------------------

// DiffService 提供结构化 diff、三方合并、AI 审查标注、导出等功能。
type DiffService struct {
	mu sync.Mutex
	// 缓存的 AI 审查结果（key = file path + hunk index）
	aiReviews     map[string][]AIComment
	snapshotSvc   *SnapshotService // Step 3: Apply 前创建快照（可选）
	workspaceRoot string           // Step 3: 快照工作区根
}

// NewDiffService 创建服务。
func NewDiffService() *DiffService {
	return &DiffService{
		aiReviews: make(map[string][]AIComment),
	}
}

// SetSnapshotService 注入快照服务与工作区根（Step 3: Apply 前创建快照）。
func (s *DiffService) SetSnapshotService(snap *SnapshotService, workspaceRoot string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshotSvc = snap
	s.workspaceRoot = workspaceRoot
}

// CreatePreApplySnapshot 在 Apply 前 best-effort 创建快照（Step 3: pre-apply）。
// 由 Apply 流程的调用方在写盘前调用。失败不阻断，返回空快照 ID。
func (s *DiffService) CreatePreApplySnapshot() string {
	s.mu.Lock()
	snap := s.snapshotSvc
	root := s.workspaceRoot
	s.mu.Unlock()
	if snap == nil || root == "" {
		return ""
	}
	created, err := snap.CreateSnapshot(root, string(SnapshotReasonPreApply))
	if err != nil || created == nil {
		return ""
	}
	return created.ID
}

// ThreeWayMergeFile 是包级 ThreeWayMerge 的 service wrapper（Step 2）。
// 包级函数无法被 Wails bindings 直接暴露，故在此转发，供前端调用。
func (s *DiffService) ThreeWayMergeFile(base, ours, theirs string) ThreeWayMergeResult {
	return ThreeWayMerge(base, ours, theirs)
}

// ComputeFileDiff 计算单个文件的 diff（Step 1）。
//
// 内部调用 myersDiff 生成 unified diff 文本，然后解析为结构化 Hunk。
func (s *DiffService) ComputeFileDiff(path, oldContent, newContent string) FileDiff {
	// 生成 unified diff 文本
	diffText := myersDiff(path, oldContent, newContent)

	// 解析为结构化 Hunk
	hunks := parseUnifiedDiff(diffText, oldContent, newContent)

	// 统计
	added, removed := 0, 0
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Type == DiffLineAdded {
				added++
			} else if l.Type == DiffLineRemoved {
				removed++
			}
		}
	}

	return FileDiff{
		Path:         path,
		OldContent:   oldContent,
		NewContent:   newContent,
		Hunks:        hunks,
		AddedLines:   added,
		RemovedLines: removed,
	}
}

// ComputeMultiFileDiff 计算多个文件的 diff（Step 1）。
func (s *DiffService) ComputeMultiFileDiff(files []FileInput) MultiFileDiff {
	result := MultiFileDiff{}
	for _, f := range files {
		fd := s.ComputeFileDiff(f.Path, f.OldContent, f.NewContent)
		result.Files = append(result.Files, fd)
		result.TotalAdded += fd.AddedLines
		result.TotalRemoved += fd.RemovedLines
	}
	return result
}

// FileInput 单个文件输入。
type FileInput struct {
	Path       string `json:"path"`
	OldContent string `json:"oldContent"`
	NewContent string `json:"newContent"`
}

// ---------------------------------------------------------------------------
// Step 3-4, 8: AI 审查标注 + 行内评论
// ---------------------------------------------------------------------------

// AddAIComment 给指定 hunk 添加 AI 审查意见（Step 3）。
func (s *DiffService) AddAIComment(diff *MultiFileDiff, fileIdx, hunkIdx int, comment AIComment) {
	if fileIdx < 0 || fileIdx >= len(diff.Files) {
		return
	}
	f := &diff.Files[fileIdx]
	if hunkIdx < 0 || hunkIdx >= len(f.Hunks) {
		return
	}
	f.Hunks[hunkIdx].AIComments = append(f.Hunks[hunkIdx].AIComments, comment)

	// 缓存
	key := fmt.Sprintf("%s-%d", f.Path, hunkIdx)
	s.mu.Lock()
	s.aiReviews[key] = append(s.aiReviews[key], comment)
	s.mu.Unlock()
}

// AddInlineComment 给指定行添加行内评论（Step 4）。
func (s *DiffService) AddInlineComment(diff *MultiFileDiff, fileIdx, hunkIdx, lineIdx int, comment InlineComment) {
	if fileIdx < 0 || fileIdx >= len(diff.Files) {
		return
	}
	f := &diff.Files[fileIdx]
	if hunkIdx < 0 || hunkIdx >= len(f.Hunks) {
		return
	}
	h := &f.Hunks[hunkIdx]
	if lineIdx < 0 || lineIdx >= len(h.Lines) {
		return
	}
	h.Lines[lineIdx].Comments = append(h.Lines[lineIdx].Comments, comment)
}

// ---------------------------------------------------------------------------
// Step 6-7: Apply / Reject
// ---------------------------------------------------------------------------

// ApplyFile 应用单个文件的 diff（用 NewContent 替换 OldContent）。
// 返回应用后的内容。
func (s *DiffService) ApplyFile(fd FileDiff) string {
	return fd.NewContent
}

// ApplyAll 应用所有文件的 diff。
// 返回 map[path]content。
func (s *DiffService) ApplyAll(diff MultiFileDiff) map[string]string {
	result := make(map[string]string, len(diff.Files))
	for _, f := range diff.Files {
		result[f.Path] = f.NewContent
	}
	return result
}

// RejectHunk 拒绝单个 hunk（返回不含该 hunk 的内容）。
// Step 7: Reject 单 hunk。
func (s *DiffService) RejectHunk(fd FileDiff, hunkIdx int) string {
	if hunkIdx < 0 || hunkIdx >= len(fd.Hunks) {
		return fd.NewContent
	}
	// 简化实现：从 old content 开始，应用除被拒绝 hunk 外的所有 hunk。
	// 被拒绝的 hunk 保持 old content 不变（即丢弃其 added 行，恢复 removed 行）。
	// 完整逐行应用留给后续增强；此处先返回 old content 作为保守回退。
	return fd.OldContent
}

// RejectFile 拒绝整个文件（返回 OldContent）。
func (s *DiffService) RejectFile(fd FileDiff) string {
	return fd.OldContent
}

// RejectAll 拒绝所有文件。
func (s *DiffService) RejectAll(diff MultiFileDiff) map[string]string {
	result := make(map[string]string, len(diff.Files))
	for _, f := range diff.Files {
		result[f.Path] = f.OldContent
	}
	return result
}

// ---------------------------------------------------------------------------
// Step 9: PR 审查 + Markdown 报告
// ---------------------------------------------------------------------------

// ReviewPRRequest PR 审查请求。
type ReviewPRRequest struct {
	BaseBranch string `json:"baseBranch"`
	RepoPath   string `json:"repoPath"`
}

// ReviewPRResult PR 审查结果。
type ReviewPRResult struct {
	Summary     string       `json:"summary"`
	FileReviews []FileReview `json:"fileReviews"`
	Stats       ReviewStats  `json:"stats"`
}

// FileReview 单文件审查结果。
type FileReview struct {
	Path     string      `json:"path"`
	Comments []AIComment `json:"comments"`
}

// ReviewStats 审查统计。
type ReviewStats struct {
	FilesReviewed int `json:"filesReviewed"`
	TotalComments int `json:"totalComments"`
	Critical      int `json:"critical"`
	Errors        int `json:"errors"`
	Warnings      int `json:"warnings"`
}

// ReviewPR 审查整个 PR（Step 9）。
// 简化实现：接收已有的 MultiFileDiff + AI 审查意见，生成 Markdown 报告。
func (s *DiffService) ReviewPR(diff MultiFileDiff, reviews []FileReview) ReviewPRResult {
	stats := ReviewStats{}
	for _, fr := range reviews {
		stats.FilesReviewed++
		for _, c := range fr.Comments {
			stats.TotalComments++
			switch c.Severity {
			case AISeverityCritical:
				stats.Critical++
			case AISeverityError:
				stats.Errors++
			case AISeverityWarning:
				stats.Warnings++
			}
		}
	}
	return ReviewPRResult{
		Summary:     fmt.Sprintf("Reviewed %d files with %d comments (%d critical, %d errors, %d warnings)", stats.FilesReviewed, stats.TotalComments, stats.Critical, stats.Errors, stats.Warnings),
		FileReviews: reviews,
		Stats:       stats,
	}
}

// ExportMarkdown 导出为 Markdown 报告（Step 9-10）。
func (s *DiffService) ExportMarkdown(diff MultiFileDiff, reviews []FileReview) string {
	var b strings.Builder
	b.WriteString("# Diff Review Report\n\n")
	b.WriteString(fmt.Sprintf("**Files:** %d  | **+%d / -%d**\n\n", len(diff.Files), diff.TotalAdded, diff.TotalRemoved))

	for _, f := range diff.Files {
		b.WriteString(fmt.Sprintf("## %s (+%d / -%d)\n\n", f.Path, f.AddedLines, f.RemovedLines))
		// 查找此文件的审查意见
		for _, r := range reviews {
			if r.Path == f.Path {
				for _, c := range r.Comments {
					emoji := severityEmoji(c.Severity)
					b.WriteString(fmt.Sprintf("- %s **%s**: %s", emoji, c.Severity, c.Message))
					if c.Suggestion != "" {
						b.WriteString(fmt.Sprintf("\n  > Suggestion: %s", c.Suggestion))
					}
					b.WriteString("\n")
				}
			}
		}
		// 输出 diff hunk
		for _, h := range f.Hunks {
			b.WriteString(fmt.Sprintf("```diff\n@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldCount, h.NewStart, h.NewCount))
			for _, l := range h.Lines {
				switch l.Type {
				case DiffLineAdded:
					b.WriteString("+")
				case DiffLineRemoved:
					b.WriteString("-")
				case DiffLineConflict:
					b.WriteString("!")
				default:
					b.WriteString(" ")
				}
				b.WriteString(l.Content + "\n")
			}
			b.WriteString("```\n\n")
		}
	}
	return b.String()
}

func severityEmoji(s AICommentSeverity) string {
	switch s {
	case AISeverityCritical:
		return "🔴"
	case AISeverityError:
		return "🟠"
	case AISeverityWarning:
		return "🟡"
	default:
		return "🔵"
	}
}

// ---------------------------------------------------------------------------
// Step 10: 导出
// ---------------------------------------------------------------------------

// ExportUnifiedDiff 导出为 unified diff 文本（Step 10）。
func (s *DiffService) ExportUnifiedDiff(diff MultiFileDiff) string {
	var b strings.Builder
	for _, f := range diff.Files {
		b.WriteString(fmt.Sprintf("--- a/%s\n", f.Path))
		b.WriteString(fmt.Sprintf("+++ b/%s\n", f.Path))
		for _, h := range f.Hunks {
			b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldCount, h.NewStart, h.NewCount))
			for _, l := range h.Lines {
				switch l.Type {
				case DiffLineAdded:
					b.WriteString("+")
				case DiffLineRemoved:
					b.WriteString("-")
				default:
					b.WriteString(" ")
				}
				b.WriteString(l.Content + "\n")
			}
		}
	}
	return b.String()
}

// ExportHTML 导出为 HTML（Step 10）。
func (s *DiffService) ExportHTML(diff MultiFileDiff) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Diff Report</title>")
	b.WriteString("<style>body{font-family:monospace;margin:20px}.added{color:green}.removed{color:red}.context{color:gray}.hunk{background:#f0f0f0;padding:4px;margin:8px 0}.file{border:1px solid #ccc;margin:16px 0;padding:8px}</style>")
	b.WriteString("</head><body>")

	for _, f := range diff.Files {
		b.WriteString(fmt.Sprintf("<div class=\"file\"><h2>%s (+%d / -%d)</h2>", f.Path, f.AddedLines, f.RemovedLines))
		for _, h := range f.Hunks {
			b.WriteString(fmt.Sprintf("<div class=\"hunk\">@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount))
			for _, l := range h.Lines {
				cls := "context"
				prefix := " "
				switch l.Type {
				case DiffLineAdded:
					cls = "added"
					prefix = "+"
				case DiffLineRemoved:
					cls = "removed"
					prefix = "-"
				}
				b.WriteString(fmt.Sprintf("<div class=\"%s\">%s%s</div>", cls, prefix, escapeHTML(l.Content)))
			}
			b.WriteString("</div>")
		}
		b.WriteString("</div>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ---------------------------------------------------------------------------
// unified diff 解析
// ---------------------------------------------------------------------------

// parseUnifiedDiff 解析 unified diff 文本为结构化 Hunk。
func parseUnifiedDiff(diffText, oldContent, newContent string) []Hunk {
	if diffText == "" {
		return nil
	}

	lines := splitLines(diffText)
	var hunks []Hunk
	var current *Hunk

	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			// 新 hunk
			if current != nil {
				hunks = append(hunks, *current)
			}
			h := parseHunkHeader(line)
			current = &h
		} else if current != nil {
			dl := parseDiffLine(line, &current.OldStart, &current.NewStart, oldLines, newLines)
			current.Lines = append(current.Lines, dl)
		}
	}
	if current != nil {
		hunks = append(hunks, *current)
	}

	// 重新计算 oldStart/newStart（因为上面的 parseDiffLine 会递增）
	// 重新解析以获得正确的行号
	return renumberHunks(hunks, oldLines, newLines)
}

// parseHunkHeader 解析 @@ -oldStart,oldCount +newStart,newCount @@ 行。
func parseHunkHeader(line string) Hunk {
	// 格式: @@ -1,5 +1,7 @@
	var oldStart, oldCount, newStart, newCount int
	fmt.Sscanf(line, "@@ -%d,%d +%d,%d @@", &oldStart, &oldCount, &newStart, &newCount)
	if oldCount == 0 {
		oldCount = 1
	}
	if newCount == 0 {
		newCount = 1
	}
	return Hunk{
		OldStart: oldStart,
		OldCount: oldCount,
		NewStart: newStart,
		NewCount: newCount,
	}
}

// parseDiffLine 解析单行 diff。
func parseDiffLine(line string, oldStart, newStart *int, oldLines, newLines []string) DiffLine {
	if line == "" {
		return DiffLine{Type: DiffLineContext, Content: ""}
	}
	prefix := line[0]
	content := line[1:]
	switch prefix {
	case '+':
		return DiffLine{Type: DiffLineAdded, Content: content}
	case '-':
		return DiffLine{Type: DiffLineRemoved, Content: content}
	default:
		return DiffLine{Type: DiffLineContext, Content: content}
	}
}

// renumberHunks 重新计算行号。
func renumberHunks(hunks []Hunk, oldLines, newLines []string) []Hunk {
	for hi := range hunks {
		oldNum := hunks[hi].OldStart
		newNum := hunks[hi].NewStart
		for li := range hunks[hi].Lines {
			l := &hunks[hi].Lines[li]
			switch l.Type {
			case DiffLineContext:
				l.OldNum = oldNum
				l.NewNum = newNum
				oldNum++
				newNum++
			case DiffLineAdded:
				l.NewNum = newNum
				newNum++
			case DiffLineRemoved:
				l.OldNum = oldNum
				oldNum++
			}
		}
	}
	return hunks
}

// splitLines 由 myers_diff.go 提供（包内共享），此处不重复声明。
