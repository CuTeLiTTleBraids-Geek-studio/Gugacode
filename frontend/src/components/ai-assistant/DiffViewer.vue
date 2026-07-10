<script setup lang="ts">
// Plan 11 Task 13 — DiffViewer.vue
// Step 5: 多文件 tab + 统计概览 + hunk 折叠 + 行号 + 语法高亮
// Step 8: AI 审查模式（自动生成 hunk 审查意见，severity 色标）
// Step 11: Artifact 预览模式（iframe sandbox="allow-scripts"）
import { computed, ref } from "vue";
import hljs from "highlight.js/lib/common";
import {
  diffState,
  setActiveFile,
  toggleHunk,
  setAIReviewMode,
  setArtifactPreview,
  applyFile,
  applyAll,
  rejectFile,
  rejectHunk,
} from "@/stores/diff";
import { useI18n } from "@/lib/i18n";
import { detectLanguage } from "@/lib/language";
import { sanitizeHtml } from "@/lib/markdown";
import MarkdownContent from "@/components/common/MarkdownContent.vue";
import type { AICommentSeverity, DiffLine, FileDiff } from "@/types";

const { t } = useI18n();

const emit = defineEmits<{
  (e: "export", format: "markdown" | "unified" | "html"): void;
}>();

// ---- Step 5: 多文件 tab ----

const activeFile = computed<FileDiff | null>(() => {
  if (!diffState.diff || diffState.activeFileIdx >= diffState.diff.files.length) return null;
  return diffState.diff.files[diffState.activeFileIdx];
});

const stats = computed(() => {
  if (!diffState.diff) return { added: 0, removed: 0, files: 0 };
  return {
    added: diffState.diff.totalAdded,
    removed: diffState.diff.totalRemoved,
    files: diffState.diff.files.length,
  };
});

function selectFile(idx: number): void {
  setActiveFile(idx);
}

// ---- Step 5: hunk 折叠 ----

function isHunkCollapsed(fileIdx: number, hunkIdx: number): boolean {
  return diffState.collapsedHunks.has(`${fileIdx}-${hunkIdx}`);
}

// ---- Step 5: 行号 + 语法高亮 ----

function lineClass(line: DiffLine): string {
  switch (line.type) {
    case "added":
      return "diff-line--added";
    case "removed":
      return "diff-line--removed";
    case "conflict":
      return "diff-line--conflict";
    default:
      return "diff-line--context";
  }
}

function linePrefix(line: DiffLine): string {
  switch (line.type) {
    case "added":
      return "+";
    case "removed":
      return "-";
    case "conflict":
      return "!";
    default:
      return " ";
  }
}

function highlightLine(content: string, filePath: string): string {
  const lang = detectLanguage(filePath);
  try {
    let html: string;
    if (lang && hljs.getLanguage(lang)) {
      html = hljs.highlight(content, { language: lang }).value;
    } else {
      html = hljs.highlightAuto(content).value;
    }
    // G-SEC-11: 经 DOMPurify 净化后再渲染（highlight.js 输出仅含 <span>，净化是纵深防御）。
    return sanitizeHtml(html);
  } catch {
    return escapeHtml(content);
  }
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

// ---- Step 8: AI 审查模式（severity 色标）----

function severityClass(sev: AICommentSeverity): string {
  return `ai-comment--${sev}`;
}

function severityIcon(sev: AICommentSeverity): string {
  switch (sev) {
    case "critical":
      return "🔴";
    case "error":
      return "❌";
    case "warning":
      return "⚠️";
    default:
      return "ℹ️";
  }
}

function toggleAIReview(): void {
  setAIReviewMode(!diffState.aiReviewMode);
}

// ---- Step 11: Artifact 预览模式（iframe sandbox）----

function toggleArtifactPreview(): void {
  setArtifactPreview(!diffState.artifactPreview);
}

const artifactSrcDoc = computed(() => {
  if (!activeFile.value) return "";
  // Artifact 预览仅对 HTML 类文件有意义；其他文件显示源码。
  const path = activeFile.value.path;
  if (!/\.(html?|svg)$/i.test(path)) return "";
  return activeFile.value.newContent;
});

const isArtifactable = computed(() => {
  if (!activeFile.value) return false;
  return /\.(html?|svg)$/i.test(activeFile.value.path);
});

// ---- Step 6-7: Apply / Reject ----

async function handleApplyFile(): Promise<void> {
  await applyFile(diffState.activeFileIdx);
}

async function handleApplyAll(): Promise<void> {
  await applyAll();
}

async function handleRejectFile(): Promise<void> {
  await rejectFile(diffState.activeFileIdx);
}

async function handleRejectHunk(hunkIdx: number): Promise<void> {
  await rejectHunk(diffState.activeFileIdx, hunkIdx);
}

function handleExport(format: "markdown" | "unified" | "html"): void {
  emit("export", format);
}

// 导出菜单可见性
const exportMenuVisible = ref(false);
</script>

<template>
  <div class="diff-viewer">
    <!-- Step 5: 统计概览 -->
    <div class="diff-viewer__stats">
      <span class="diff-stat diff-stat--added">+{{ stats.added }}</span>
      <span class="diff-stat diff-stat--removed">−{{ stats.removed }}</span>
      <span class="diff-stat diff-stat--files">{{ stats.files }} {{ t("diffViewer.files") }}</span>
    </div>

    <!-- Step 5: 多文件 tab -->
    <div v-if="diffState.diff && diffState.diff.files.length > 1" class="diff-viewer__tabs">
      <button
        v-for="(file, idx) in diffState.diff.files"
        :key="file.path"
        :class="['diff-tab', { 'diff-tab--active': idx === diffState.activeFileIdx }]"
        @click="selectFile(idx)"
      >
        <span class="diff-tab__name">{{ file.path }}</span>
        <span class="diff-tab__count">
          <span class="diff-tab__added">+{{ file.addedLines }}</span>
          <span class="diff-tab__removed">−{{ file.removedLines }}</span>
        </span>
      </button>
    </div>

    <!-- 工具栏 -->
    <div class="diff-viewer__toolbar">
      <label class="diff-toolbar__toggle">
        <input type="checkbox" :checked="diffState.aiReviewMode" @change="toggleAIReview" />
        <span>{{ t("diffViewer.aiReviewMode") }}</span>
      </label>
      <label v-if="isArtifactable" class="diff-toolbar__toggle">
        <input type="checkbox" :checked="diffState.artifactPreview" @change="toggleArtifactPreview" />
        <span>{{ t("diffViewer.artifactPreview") }}</span>
      </label>
      <div class="diff-toolbar__spacer" />
      <button class="diff-toolbar__btn" :disabled="!activeFile" @click="handleApplyFile">
        {{ t("diffViewer.applyFile") }}
      </button>
      <button class="diff-toolbar__btn" :disabled="!activeFile" @click="handleRejectFile">
        {{ t("diffViewer.rejectFile") }}
      </button>
      <button class="diff-toolbar__btn diff-toolbar__btn--primary" @click="handleApplyAll">
        {{ t("diffViewer.applyAll") }}
      </button>
      <div class="diff-toolbar__export">
        <button class="diff-toolbar__btn" @click="exportMenuVisible = !exportMenuVisible">
          {{ t("diffViewer.export") }} ▾
        </button>
        <div v-if="exportMenuVisible" class="diff-export__menu">
          <button @click="handleExport('markdown'); exportMenuVisible = false">Markdown</button>
          <button @click="handleExport('unified'); exportMenuVisible = false">Unified Diff</button>
          <button @click="handleExport('html'); exportMenuVisible = false">HTML</button>
        </div>
      </div>
    </div>

    <!-- Step 11: Artifact 预览模式 -->
    <div v-if="diffState.artifactPreview && isArtifactable && artifactSrcDoc" class="diff-viewer__artifact">
      <!-- G-SEC-11: iframe sandbox="allow-scripts" 防止同源访问 -->
      <iframe
        :srcdoc="artifactSrcDoc"
        sandbox="allow-scripts"
        class="diff-artifact__iframe"
        :title="t('diffViewer.artifactTitle')"
      />
    </div>

    <!-- Step 5: diff 内容 -->
    <div v-else-if="activeFile" class="diff-viewer__content">
      <div v-for="(hunk, hunkIdx) in activeFile.hunks" :key="hunkIdx" class="diff-hunk">
        <!-- hunk 头（可折叠） -->
        <button class="diff-hunk__header" @click="toggleHunk(diffState.activeFileIdx, hunkIdx)">
          <span class="diff-hunk__toggle">{{ isHunkCollapsed(diffState.activeFileIdx, hunkIdx) ? "▶" : "▼" }}</span>
          <span class="diff-hunk__range">@@ -{{ hunk.oldStart }},{{ hunk.oldCount }} +{{ hunk.newStart }},{{ hunk.newCount }} @@</span>
          <span class="diff-hunk__actions">
            <button
              class="diff-hunk__reject"
              :title="t('diffViewer.rejectHunk')"
              @click.stop="handleRejectHunk(hunkIdx)"
            >✕</button>
          </span>
        </button>

        <!-- hunk 行 -->
        <div v-show="!isHunkCollapsed(diffState.activeFileIdx, hunkIdx)" class="diff-hunk__body">
          <table class="diff-table">
            <tbody>
              <tr v-for="(line, lineIdx) in hunk.lines" :key="lineIdx" :class="lineClass(line)">
                <td class="diff-table__num diff-table__num--old">{{ line.oldNum || "" }}</td>
                <td class="diff-table__num diff-table__num--new">{{ line.newNum || "" }}</td>
                <td class="diff-table__prefix">{{ linePrefix(line) }}</td>
                <td class="diff-table__content">
                  <MarkdownContent :html="highlightLine(line.content, activeFile.path)" class="diff-table__code" />
                </td>
              </tr>
            </tbody>
          </table>

          <!-- Step 8: AI 审查意见（severity 色标） -->
          <div v-if="diffState.aiReviewMode && hunk.aiComments && hunk.aiComments.length" class="diff-hunk__ai-comments">
            <div
              v-for="(comment, cIdx) in hunk.aiComments"
              :key="cIdx"
              :class="['ai-comment', severityClass(comment.severity)]"
            >
              <span class="ai-comment__icon">{{ severityIcon(comment.severity) }}</span>
              <span class="ai-comment__severity">{{ comment.severity }}</span>
              <span class="ai-comment__message">{{ comment.message }}</span>
              <span v-if="comment.suggestion" class="ai-comment__suggestion">
                {{ t("diffViewer.suggestion") }}: {{ comment.suggestion }}
              </span>
            </div>
          </div>

          <!-- Step 4: 行内评论 -->
          <template v-for="(line, lineIdx) in hunk.lines" :key="`c-${lineIdx}`">
            <div v-if="line.comments && line.comments.length" class="diff-line__comments">
              <div v-for="(c, ci) in line.comments" :key="ci" class="inline-comment">
                <span class="inline-comment__author">{{ c.author }}</span>
                <span class="inline-comment__body">{{ c.body }}</span>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else class="diff-viewer__empty">
      {{ t("diffViewer.empty") }}
    </div>
  </div>
</template>

<style scoped>
.diff-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  font-family: var(--gugacode-font-mono, "Cascadia Code", "Fira Code", monospace);
  font-size: 13px;
}

/* Step 5: 统计概览 */
.diff-viewer__stats {
  display: flex;
  gap: 12px;
  padding: 6px 12px;
  background: var(--el-bg-color-page, #f5f5f5);
  border-bottom: 1px solid var(--el-border-color, #dcdfe6);
}
.diff-stat--added { color: #52c41a; font-weight: 600; }
.diff-stat--removed { color: #f5222d; font-weight: 600; }
.diff-stat--files { color: var(--el-text-color-secondary, #909399); }

/* Step 5: 多文件 tab */
.diff-viewer__tabs {
  display: flex;
  gap: 2px;
  padding: 0 8px;
  background: var(--el-bg-color, #fff);
  border-bottom: 1px solid var(--el-border-color, #dcdfe6);
  overflow-x: auto;
}
.diff-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: none;
  background: transparent;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  white-space: nowrap;
  color: var(--el-text-color-regular, #606266);
}
.diff-tab--active {
  border-bottom-color: var(--el-color-primary, #409eff);
  color: var(--el-color-primary, #409eff);
}
.diff-tab__name { font-size: 12px; }
.diff-tab__count { display: flex; gap: 4px; font-size: 11px; }
.diff-tab__added { color: #52c41a; }
.diff-tab__removed { color: #f5222d; }

/* 工具栏 */
.diff-viewer__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--el-border-color, #dcdfe6);
}
.diff-toolbar__toggle {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  cursor: pointer;
  color: var(--el-text-color-regular, #606266);
}
.diff-toolbar__spacer { flex: 1; }
.diff-toolbar__btn {
  padding: 4px 10px;
  font-size: 12px;
  border: 1px solid var(--el-border-color, #dcdfe6);
  background: var(--el-bg-color, #fff);
  border-radius: 3px;
  cursor: pointer;
  color: var(--el-text-color-regular, #606266);
}
.diff-toolbar__btn:hover { border-color: var(--el-color-primary, #409eff); }
.diff-toolbar__btn:disabled { opacity: 0.5; cursor: not-allowed; }
.diff-toolbar__btn--primary {
  background: var(--el-color-primary, #409eff);
  color: #fff;
  border-color: var(--el-color-primary, #409eff);
}
.diff-toolbar__export { position: relative; }
.diff-export__menu {
  position: absolute;
  right: 0;
  top: 100%;
  display: flex;
  flex-direction: column;
  background: var(--el-bg-color, #fff);
  border: 1px solid var(--el-border-color, #dcdfe6);
  border-radius: 3px;
  z-index: 10;
  min-width: 120px;
}
.diff-export__menu button {
  padding: 6px 12px;
  border: none;
  background: transparent;
  text-align: left;
  cursor: pointer;
  font-size: 12px;
}
.diff-export__menu button:hover { background: var(--el-fill-color-light, #f5f7fa); }

/* Step 11: Artifact 预览 */
.diff-viewer__artifact { flex: 1; overflow: hidden; }
.diff-artifact__iframe { width: 100%; height: 100%; border: none; }

/* diff 内容 */
.diff-viewer__content { flex: 1; overflow: auto; }
.diff-hunk { border-bottom: 1px solid var(--el-border-color-light, #e4e7ed); }
.diff-hunk__header {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 4px 12px;
  border: none;
  background: var(--el-fill-color-light, #f5f7fa);
  cursor: pointer;
  text-align: left;
  font-family: inherit;
  font-size: 12px;
  color: var(--el-text-color-secondary, #909399);
}
.diff-hunk__toggle { width: 12px; }
.diff-hunk__range { color: var(--el-color-primary, #409eff); }
.diff-hunk__actions { margin-left: auto; }
.diff-hunk__reject {
  border: none;
  background: transparent;
  color: #f5222d;
  cursor: pointer;
  font-size: 12px;
  padding: 0 4px;
}
.diff-hunk__body { font-size: 13px; }

.diff-table { width: 100%; border-collapse: collapse; }
.diff-table__num {
  width: 50px;
  min-width: 50px;
  padding: 0 8px;
  text-align: right;
  color: var(--el-text-color-placeholder, #c0c4cc);
  user-select: none;
  font-size: 12px;
  vertical-align: top;
}
.diff-table__prefix {
  width: 20px;
  min-width: 20px;
  text-align: center;
  user-select: none;
  vertical-align: top;
}
.diff-table__content { vertical-align: top; }
.diff-table__code { font-family: inherit; white-space: pre-wrap; word-break: break-all; }
.diff-table__code :deep(code) { font-family: inherit; }

/* 行类型样式 */
.diff-line--added { background: rgba(82, 196, 26, 0.08); }
.diff-line--added .diff-table__prefix { color: #52c41a; }
.diff-line--removed { background: rgba(245, 34, 45, 0.08); }
.diff-line--removed .diff-table__prefix { color: #f5222d; }
.diff-line--conflict { background: rgba(255, 165, 0, 0.12); }
.diff-line--conflict .diff-table__prefix { color: #fa8c16; }
.diff-line--context .diff-table__prefix { color: var(--el-text-color-placeholder, #c0c4cc); }

/* Step 8: AI 审查意见 severity 色标 */
.diff-hunk__ai-comments {
  padding: 4px 12px 8px 48px;
  border-top: 1px dashed var(--el-border-color, #dcdfe6);
}
.ai-comment {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 3px 6px;
  margin: 2px 0;
  border-radius: 3px;
  font-size: 12px;
}
.ai-comment__icon { flex-shrink: 0; }
.ai-comment__severity {
  font-weight: 600;
  text-transform: uppercase;
  font-size: 10px;
  flex-shrink: 0;
}
.ai-comment__message { color: var(--el-text-color-regular, #606266); }
.ai-comment__suggestion {
  margin-left: 8px;
  color: var(--el-text-color-secondary, #909399);
  font-style: italic;
}
.ai-comment--critical { background: rgba(245, 34, 45, 0.12); border-left: 3px solid #f5222d; }
.ai-comment--error { background: rgba(245, 34, 45, 0.08); border-left: 3px solid #ff7875; }
.ai-comment--warning { background: rgba(250, 140, 22, 0.1); border-left: 3px solid #fa8c16; }
.ai-comment--info { background: rgba(64, 158, 255, 0.08); border-left: 3px solid #409eff; }

/* Step 4: 行内评论 */
.diff-line__comments { padding: 0 12px 4px 48px; }
.inline-comment {
  display: flex;
  gap: 6px;
  padding: 2px 6px;
  font-size: 12px;
  color: var(--el-text-color-regular, #606266);
  background: var(--el-fill-color, #f5f7fa);
  border-radius: 3px;
  margin: 2px 0;
}
.inline-comment__author { font-weight: 600; color: var(--el-color-primary, #409eff); }

/* 空状态 */
.diff-viewer__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: var(--el-text-color-placeholder, #c0c4cc);
}
</style>
