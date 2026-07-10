// Plan 11 Task 13 — Diff 增强前端 store。
//
// 使用 lazy bindings + 可注入 backend 模式：
//   - 生产环境懒加载 Wails bindings
//   - 测试环境通过 setDiffBackend 注入 mock
//
// 职责（Step 1-12）：
//   - 计算单文件/多文件 diff（Step 1）
//   - 三方合并（Step 2）
//   - AI 审查标注 + 行内评论（Step 3-4）
//   - Apply / Reject（Step 6-7）
//   - PR 审查（Step 9）
//   - 导出 Markdown / unified diff / HTML（Step 10）
import { reactive } from "vue";
import { notifyError, notifySuccess } from "@/lib/notifications";
import type {
  AIComment,
  DiffExportFormat,
  DiffFileInput,
  FileDiff,
  FileReview,
  InlineComment,
  MultiFileDiff,
  ReviewPRResult,
  ThreeWayMergeResult,
} from "@/types";

// backend 接口镜像 services/diff_service.go 导出方法。
interface DiffBackend {
  computeFileDiff(path: string, oldContent: string, newContent: string): Promise<FileDiff>;
  computeMultiFileDiff(files: DiffFileInput[]): Promise<MultiFileDiff>;
  addAIComment(diff: MultiFileDiff, fileIdx: number, hunkIdx: number, comment: AIComment): Promise<void>;
  addInlineComment(
    diff: MultiFileDiff,
    fileIdx: number,
    hunkIdx: number,
    lineIdx: number,
    comment: InlineComment,
  ): Promise<void>;
  applyFile(fd: FileDiff): Promise<string>;
  applyAll(diff: MultiFileDiff): Promise<Record<string, string>>;
  rejectHunk(fd: FileDiff, hunkIdx: number): Promise<string>;
  rejectFile(fd: FileDiff): Promise<string>;
  rejectAll(diff: MultiFileDiff): Promise<Record<string, string>>;
  reviewPR(diff: MultiFileDiff, reviews: FileReview[]): Promise<ReviewPRResult>;
  exportMarkdown(diff: MultiFileDiff, reviews: FileReview[]): Promise<string>;
  exportUnifiedDiff(diff: MultiFileDiff): Promise<string>;
  exportHTML(diff: MultiFileDiff): Promise<string>;
  // ThreeWayMergeFile 是后端 ThreeWayMerge 的 service wrapper（Step 2）。
  threeWayMergeFile(base: string, ours: string, theirs: string): Promise<ThreeWayMergeResult>;
}

interface DiffState {
  /** 当前展示的多文件 diff。 */
  diff: MultiFileDiff | null;
  /** PR 审查结果。 */
  review: ReviewPRResult | null;
  /** 三方合并结果。 */
  mergeResult: ThreeWayMergeResult | null;
  /** 当前选中的文件索引（DiffViewer tab）。 */
  activeFileIdx: number;
  /** 折叠的 hunk key 集合（`fileIdx-hunkIdx`）。 */
  collapsedHunks: Set<string>;
  /** AI 审查模式是否开启（Step 8）。 */
  aiReviewMode: boolean;
  /** Artifact 预览模式是否开启（Step 11）。 */
  artifactPreview: boolean;
  loading: boolean;
  error: string | null;
}

export const diffState = reactive<DiffState>({
  diff: null,
  review: null,
  mergeResult: null,
  activeFileIdx: 0,
  collapsedHunks: new Set(),
  aiReviewMode: false,
  artifactPreview: false,
  loading: false,
  error: null,
});

let backend: DiffBackend | null = null;

// 懒加载 Wails bindings（services.DiffService）。
async function getBackend(): Promise<DiffBackend> {
  if (backend) return backend;
  // 使用字面量路径（无 @vite-ignore），让 vite 将 bindings 打包为 chunk。
  const mod = await import("../../bindings/gugacode/services/diffservice.js");
  // Bindings use enum DiffLineType; frontend uses string unions — cast at boundary.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const m = mod as any;
  backend = {
    computeFileDiff: (p, o, n) => m.ComputeFileDiff(p, o, n) as Promise<FileDiff>,
    computeMultiFileDiff: (f) => m.ComputeMultiFileDiff(f) as Promise<MultiFileDiff>,
    addAIComment: (d, fi, hi, c) => m.AddAIComment(d, fi, hi, c) as Promise<void>,
    addInlineComment: (d, fi, hi, li, c) => m.AddInlineComment(d, fi, hi, li, c) as Promise<void>,
    applyFile: (fd) => m.ApplyFile(fd) as Promise<string>,
    applyAll: (d) => m.ApplyAll(d) as Promise<Record<string, string>>,
    rejectHunk: (fd, hi) => m.RejectHunk(fd, hi) as Promise<string>,
    rejectFile: (fd) => m.RejectFile(fd) as Promise<string>,
    rejectAll: (d) => m.RejectAll(d) as Promise<Record<string, string>>,
    reviewPR: (d, r) => m.ReviewPR(d, r) as Promise<ReviewPRResult>,
    exportMarkdown: (d, r) => m.ExportMarkdown(d, r) as Promise<string>,
    exportUnifiedDiff: (d) => m.ExportUnifiedDiff(d) as Promise<string>,
    exportHTML: (d) => m.ExportHTML(d) as Promise<string>,
    threeWayMergeFile: (base, ours, theirs) =>
      m.ThreeWayMergeFile(base, ours, theirs) as Promise<ThreeWayMergeResult>,
  };
  return backend;
}

// 测试注入。
export function setDiffBackend(b: DiffBackend): void {
  backend = b;
}

// ---- Step 1: 计算 diff ----

export async function computeFileDiff(path: string, oldContent: string, newContent: string): Promise<void> {
  diffState.loading = true;
  diffState.error = null;
  try {
    const b = await getBackend();
    const fd = await b.computeFileDiff(path, oldContent, newContent);
    diffState.diff = {
      files: [fd],
      totalAdded: fd.addedLines,
      totalRemoved: fd.removedLines,
    };
    diffState.activeFileIdx = 0;
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
  } finally {
    diffState.loading = false;
  }
}

export async function computeMultiFileDiff(files: DiffFileInput[]): Promise<void> {
  diffState.loading = true;
  diffState.error = null;
  try {
    const b = await getBackend();
    diffState.diff = await b.computeMultiFileDiff(files);
    diffState.activeFileIdx = 0;
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
  } finally {
    diffState.loading = false;
  }
}

// ---- Step 2: 三方合并 ----

export async function threeWayMerge(base: string, ours: string, theirs: string): Promise<void> {
  try {
    const b = await getBackend();
    diffState.mergeResult = await b.threeWayMergeFile(base, ours, theirs);
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
  }
}

// ---- Step 3-4: AI 审查标注 + 行内评论 ----

export async function addAIComment(fileIdx: number, hunkIdx: number, comment: AIComment): Promise<boolean> {
  if (!diffState.diff) return false;
  try {
    const b = await getBackend();
    await b.addAIComment(diffState.diff, fileIdx, hunkIdx, comment);
    // 更新本地状态（AddAIComment 通过指针修改，但前端持有副本，需手动同步）
    if (fileIdx < diffState.diff.files.length) {
      const hunk = diffState.diff.files[fileIdx].hunks[hunkIdx];
      if (hunk) {
        hunk.aiComments = [...(hunk.aiComments ?? []), comment];
      }
    }
    return true;
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
    return false;
  }
}

export async function addInlineComment(
  fileIdx: number,
  hunkIdx: number,
  lineIdx: number,
  comment: InlineComment,
): Promise<boolean> {
  if (!diffState.diff) return false;
  try {
    const b = await getBackend();
    await b.addInlineComment(diffState.diff, fileIdx, hunkIdx, lineIdx, comment);
    if (fileIdx < diffState.diff.files.length) {
      const line = diffState.diff.files[fileIdx].hunks[hunkIdx]?.lines[lineIdx];
      if (line) {
        line.comments = [...(line.comments ?? []), comment];
      }
    }
    return true;
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
    return false;
  }
}

// ---- Step 6-7: Apply / Reject ----

export async function applyFile(fileIdx: number): Promise<string | null> {
  if (!diffState.diff || fileIdx >= diffState.diff.files.length) return null;
  try {
    const b = await getBackend();
    const content = await b.applyFile(diffState.diff.files[fileIdx]);
    notifySuccess(`Applied ${diffState.diff.files[fileIdx].path}`);
    return content;
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
    return null;
  }
}

export async function applyAll(): Promise<Record<string, string> | null> {
  if (!diffState.diff) return null;
  try {
    const b = await getBackend();
    const result = await b.applyAll(diffState.diff);
    notifySuccess(`Applied ${Object.keys(result).length} file(s)`);
    return result;
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
    return null;
  }
}

export async function rejectHunk(fileIdx: number, hunkIdx: number): Promise<string | null> {
  if (!diffState.diff || fileIdx >= diffState.diff.files.length) return null;
  try {
    const b = await getBackend();
    return await b.rejectHunk(diffState.diff.files[fileIdx], hunkIdx);
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
    return null;
  }
}

export async function rejectFile(fileIdx: number): Promise<string | null> {
  if (!diffState.diff || fileIdx >= diffState.diff.files.length) return null;
  try {
    const b = await getBackend();
    return await b.rejectFile(diffState.diff.files[fileIdx]);
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
    return null;
  }
}

export async function rejectAll(): Promise<Record<string, string> | null> {
  if (!diffState.diff) return null;
  try {
    const b = await getBackend();
    return await b.rejectAll(diffState.diff);
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
    return null;
  }
}

// ---- Step 9: PR 审查 ----

export async function reviewPR(reviews: FileReview[]): Promise<void> {
  if (!diffState.diff) return;
  try {
    const b = await getBackend();
    diffState.review = await b.reviewPR(diffState.diff, reviews);
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
  }
}

// ---- Step 10: 导出 ----

export async function exportDiff(format: DiffExportFormat, reviews: FileReview[]): Promise<string | null> {
  if (!diffState.diff) return null;
  try {
    const b = await getBackend();
    switch (format) {
      case "markdown":
        return await b.exportMarkdown(diffState.diff, reviews);
      case "unified":
        return await b.exportUnifiedDiff(diffState.diff);
      case "html":
        return await b.exportHTML(diffState.diff);
      default:
        return null;
    }
  } catch (e: unknown) {
    diffState.error = e instanceof Error ? e.message : String(e);
    notifyError(diffState.error);
    return null;
  }
}

// ---- UI 辅助 ----

export function setActiveFile(idx: number): void {
  diffState.activeFileIdx = idx;
}

export function toggleHunk(fileIdx: number, hunkIdx: number): void {
  const key = `${fileIdx}-${hunkIdx}`;
  if (diffState.collapsedHunks.has(key)) {
    diffState.collapsedHunks.delete(key);
  } else {
    diffState.collapsedHunks.add(key);
  }
}

export function setAIReviewMode(on: boolean): void {
  diffState.aiReviewMode = on;
}

export function setArtifactPreview(on: boolean): void {
  diffState.artifactPreview = on;
}

export function resetDiffStore(): void {
  diffState.diff = null;
  diffState.review = null;
  diffState.mergeResult = null;
  diffState.activeFileIdx = 0;
  diffState.collapsedHunks.clear();
  diffState.aiReviewMode = false;
  diffState.artifactPreview = false;
  diffState.loading = false;
  diffState.error = null;
}
