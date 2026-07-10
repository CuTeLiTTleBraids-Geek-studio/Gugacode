<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { appState } from "@/stores/app";
import {
  gitState,
  branchState,
  loadBranches,
  createBranch,
  checkoutBranch,
  refreshGit,
  stageFile,
  unstageFile,
  commitChanges,
  pushChanges,
  pullChanges,
  conflictState,
  rebaseState,
  loadConflicts,
  resolveConflictAsOurs,
  resolveConflictAsTheirs,
  markConflictResolved,
  startRebase,
  abortRebase,
  continueRebase,
  checkRebaseStatus,
  generateGitignore,
  clearConflictState,
} from "@/stores/git";
import { openFileFromPath } from "@/stores/editor";
import { ArrowDown, Plus, Minus, Check, Top, Bottom, Aim, Close } from "@element-plus/icons-vue";
import DiffView from "@/components/editor/DiffView.vue";
import MarkdownContent from "@/components/common/MarkdownContent.vue";
import {
  reviewState,
  hasReview,
  runReview,
  clearReview,
} from "@/stores/review";
import { renderMarkdownWithApplyButtons } from "@/lib/markdown";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import type { MergeConflict } from "@/types";

const { t } = useI18n();

const repoPath = computed(() => appState.currentProject ?? "");
const commitMessage = ref("");

const currentBranchName = computed(() => {
  const head = branchState.branches.find((b) => b.isHead);
  return head?.name ?? gitState.branchName ?? "—";
});

const diffVisible = ref(false);
const diffFilePath = ref("");

function viewDiff(filePath: string) {
  diffFilePath.value = filePath;
  diffVisible.value = true;
}

const hasChanges = computed(() => gitState.changes.length > 0);
const hasConflicts = computed(() => conflictState.conflicts.length > 0);
const isRebaseInProgress = computed(() => rebaseState.inProgress);

async function handleRefresh() {
  if (!repoPath.value) return;
  await refreshGit(repoPath.value);
  await checkRebaseStatus();
  if (isRebaseInProgress.value) {
    await loadConflicts();
  }
}

async function handleStage(path: string) {
  if (!repoPath.value) return;
  await stageFile(repoPath.value, path);
}

async function handleUnstage(path: string) {
  if (!repoPath.value) return;
  await unstageFile(repoPath.value, path);
}

async function handleCommit() {
  if (!repoPath.value || !commitMessage.value.trim()) return;
  await commitChanges(repoPath.value, commitMessage.value);
  commitMessage.value = "";
}

async function handlePush() {
  if (!repoPath.value) return;
  try {
    await pushChanges(repoPath.value);
    ElMessage.success(t("git.pushed"));
  } catch (e: unknown) {
    ElMessage.error(t("git.pushFailed", { error: errorMessage(e) }));
  }
}

async function handlePull() {
  if (!repoPath.value) return;
  try {
    await pullChanges(repoPath.value);
    ElMessage.success(t("git.pulled"));
  } catch (e: unknown) {
    ElMessage.error(t("git.pullFailed", { error: errorMessage(e) }));
  }
}

async function handleBranchCommand(name: string) {
  if (!repoPath.value) return;
  if (name === "__new__") {
    try {
      const { value } = await ElMessageBox.prompt(t("git.branchNamePrompt"), t("git.createBranchTitle"), {
        confirmButtonText: t("git.create"),
        cancelButtonText: t("common.cancel"),
        inputPattern: /^[A-Za-z0-9._\-/]+$/,
        inputErrorMessage: t("git.invalidBranchName"),
      });
      if (value) {
        await createBranch(repoPath.value, value);
        await checkoutBranch(repoPath.value, value);
        ElMessage.success(t("git.createdAndSwitched", { name: value }));
      }
    } catch {
      // user cancelled
    }
  } else {
    try {
      await checkoutBranch(repoPath.value, name);
      ElMessage.success(t("git.switched", { name }));
    } catch (e: unknown) {
      ElMessage.error(t("git.switchFailed", { error: errorMessage(e) }));
    }
  }
}

// --- G-FEAT-04: Rebase controls ---

async function handleRebaseCommand(cmd: string) {
  if (cmd === "__start__") {
    await handleStartRebase();
  }
}

async function handleStartRebase() {
  if (!repoPath.value) return;
  try {
    const { value } = await ElMessageBox.prompt(t("git.rebaseBranchPrompt"), t("git.rebaseTitle"), {
      confirmButtonText: t("git.startRebase"),
      cancelButtonText: t("common.cancel"),
      inputPattern: /^[A-Za-z0-9._\-/]+$/,
      inputErrorMessage: t("git.invalidBranchName"),
    });
    if (value) {
      await startRebase(value);
      if (isRebaseInProgress.value) {
        ElMessage.warning(t("git.rebaseInProgress"));
      } else {
        ElMessage.success(t("git.rebaseStarted", { branch: value }));
      }
      await refreshGit(repoPath.value);
    }
  } catch {
    // user cancelled or rebase failed
    if (rebaseState.error) {
      ElMessage.error(t("git.rebaseFailed", { error: rebaseState.error }));
    }
  }
}

async function handleAbortRebase() {
  try {
    await abortRebase();
    ElMessage.success(t("git.rebaseAborted"));
    if (repoPath.value) await refreshGit(repoPath.value);
  } catch (e: unknown) {
    ElMessage.error(t("git.rebaseFailed", { error: errorMessage(e) }));
  }
}

async function handleContinueRebase() {
  try {
    await continueRebase();
    ElMessage.success(t("git.rebaseContinued"));
    if (repoPath.value) await refreshGit(repoPath.value);
  } catch (e: unknown) {
    ElMessage.error(t("git.rebaseFailed", { error: errorMessage(e) }));
  }
}

// --- G-FEAT-04: Conflict resolution ---

const resolvingFile = ref<string | null>(null);

async function handleAcceptOurs(conflict: MergeConflict) {
  if (!repoPath.value) return;
  resolvingFile.value = conflict.file;
  try {
    await resolveConflictAsOurs(repoPath.value, conflict);
    ElMessage.success(t("git.conflictResolved", { file: conflict.file }));
  } catch (e: unknown) {
    ElMessage.error(t("git.conflictResolveFailed", { error: errorMessage(e) }));
  } finally {
    resolvingFile.value = null;
  }
}

async function handleAcceptTheirs(conflict: MergeConflict) {
  if (!repoPath.value) return;
  resolvingFile.value = conflict.file;
  try {
    await resolveConflictAsTheirs(repoPath.value, conflict);
    ElMessage.success(t("git.conflictResolved", { file: conflict.file }));
  } catch (e: unknown) {
    ElMessage.error(t("git.conflictResolveFailed", { error: errorMessage(e) }));
  } finally {
    resolvingFile.value = null;
  }
}

async function handleOpenEditor(conflict: MergeConflict) {
  if (!repoPath.value) return;
  const fullPath = repoPath.value + "/" + conflict.file;
  await openFileFromPath(fullPath);
}

async function handleMarkResolved(file: string) {
  if (!repoPath.value) return;
  resolvingFile.value = file;
  try {
    await markConflictResolved(repoPath.value, file);
    ElMessage.success(t("git.conflictResolved", { file }));
  } catch (e: unknown) {
    ElMessage.error(t("git.conflictResolveFailed", { error: errorMessage(e) }));
  } finally {
    resolvingFile.value = null;
  }
}

// --- G-FEAT-04: .gitignore generation ---

async function handleGitignoreCommand(projectType: string) {
  try {
    await generateGitignore(projectType);
    ElMessage.success(t("git.gitignoreCreated"));
    if (repoPath.value) await refreshGit(repoPath.value);
  } catch (e: unknown) {
    const msg = errorMessage(e);
    if (msg.includes("already exists")) {
      ElMessage.warning(t("git.gitignoreExists"));
    } else {
      ElMessage.error(t("git.gitignoreFailed", { error: msg }));
    }
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case "Modified":
      return "M";
    case "Added":
      return "A";
    case "Deleted":
      return "D";
    case "Untracked":
      return "U";
    case "Renamed":
      return "R";
    default:
      return "?";
  }
}

// --- AI Code Review (#27) ---
const reviewModalVisible = ref(false);

function openReviewModal() {
  reviewModalVisible.value = true;
  // Auto-run review on first open if no result exists
  if (!hasReview.value && !reviewState.loading && !reviewState.error && repoPath.value) {
    void runReview(repoPath.value);
  }
}

function closeReviewModal() {
  reviewModalVisible.value = false;
}

async function handleRunReview() {
  if (!repoPath.value) return;
  clearReview();
  await runReview(repoPath.value);
}

function renderReviewContent(content: string): string {
  return renderMarkdownWithApplyButtons(content);
}

function formatReviewTime(ts: number | null): string {
  if (!ts) return "";
  return new Date(ts).toLocaleString();
}

function statusClass(status: string): string {
  switch (status) {
    case "Modified":
      return "git-panel__status--modified";
    case "Added":
      return "git-panel__status--added";
    case "Deleted":
      return "git-panel__status--deleted";
    case "Untracked":
      return "git-panel__status--untracked";
    default:
      return "git-panel__status--default";
  }
}

onMounted(() => {
  if (repoPath.value) {
    refreshGit(repoPath.value);
    loadBranches(repoPath.value);
    checkRebaseStatus().then(() => {
      if (isRebaseInProgress.value) {
        loadConflicts();
      }
    });
  }
});

watch(repoPath, (newPath) => {
  if (newPath) {
    refreshGit(newPath);
    loadBranches(newPath);
    clearConflictState();
    checkRebaseStatus().then(() => {
      if (isRebaseInProgress.value) {
        loadConflicts();
      }
    });
  } else {
    clearConflictState();
  }
});
</script>

<template>
  <div class="git-panel">
    <!-- Branch header -->
    <div class="git-panel__branch-bar">
      <el-dropdown trigger="click" @command="handleBranchCommand">
        <span class="git-panel__branch-current">
          <el-icon :size="12"><ArrowDown /></el-icon>
          {{ currentBranchName }}
        </span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item
              v-for="b in branchState.branches"
              :key="b.name"
              :command="b.name"
              :disabled="b.isHead"
            >
              {{ b.name }}{{ b.isHead ? t('git.current') : "" }}
            </el-dropdown-item>
            <el-dropdown-item divided command="__new__">
              <el-icon><Plus /></el-icon> {{ t('git.newBranch') }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <span v-if="gitState.ahead > 0" class="git-panel__ahead" :title="t('git.ahead')">
        ↑{{ gitState.ahead }}
      </span>
      <span v-if="gitState.behind > 0" class="git-panel__behind" :title="t('git.behind')">
        ↓{{ gitState.behind }}
      </span>
      <button
        type="button"
        class="git-panel__action-btn"
        :aria-label="t('git.pullAria')"
        :title="t('git.pullTitle')"
        @click="handlePull"
      >
        <el-icon :size="13"><Bottom /></el-icon>
      </button>
      <button
        type="button"
        class="git-panel__action-btn"
        :aria-label="t('git.pushAria')"
        :title="t('git.pushTitle')"
        @click="handlePush"
      >
        <el-icon :size="13"><Top /></el-icon>
      </button>
      <button
        type="button"
        class="git-panel__refresh"
        :aria-label="t('git.refreshAria')"
        :title="t('git.refreshTitle')"
        @click="handleRefresh"
      >
        ↻
      </button>
      <!-- G-FEAT-04: Rebase controls -->
      <el-dropdown trigger="click" @command="handleRebaseCommand" v-if="!isRebaseInProgress">
        <button
          type="button"
          class="git-panel__action-btn"
          :title="t('git.rebaseTitle')"
          @click.stop
        >
          <el-icon :size="13"><ArrowDown /></el-icon>
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="__start__">{{ t('git.startRebase') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <template v-else>
        <button
          type="button"
          class="git-panel__action-btn git-panel__action-btn--warning"
          :title="t('git.abortRebase')"
          :disabled="rebaseState.loading"
          @click="handleAbortRebase"
        >
          ✕
        </button>
        <button
          type="button"
          class="git-panel__action-btn git-panel__action-btn--success"
          :title="t('git.continueRebase')"
          :disabled="rebaseState.loading"
          @click="handleContinueRebase"
        >
          ✓
        </button>
      </template>
      <!-- G-FEAT-04: .gitignore generation -->
      <el-dropdown trigger="click" @command="handleGitignoreCommand">
        <button
          type="button"
          class="git-panel__action-btn"
          :title="t('git.gitignoreTitle')"
          @click.stop
        >
          .gitignore
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="go">{{ t('git.gitignoreTypeGo') }}</el-dropdown-item>
            <el-dropdown-item command="typescript">{{ t('git.gitignoreTypeTypeScript') }}</el-dropdown-item>
            <el-dropdown-item command="javascript">{{ t('git.gitignoreTypeJavaScript') }}</el-dropdown-item>
            <el-dropdown-item command="general">{{ t('git.gitignoreTypeGeneral') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <button
        type="button"
        class="git-panel__review-btn"
        :aria-label="t('git.reviewAria')"
        :title="t('git.reviewTitle')"
        @click="openReviewModal"
      >
        <el-icon :size="13"><Aim /></el-icon>
        <span>{{ t('git.review') }}</span>
      </button>
    </div>

    <!-- G-FEAT-04: Rebase in progress banner -->
    <div v-if="isRebaseInProgress" class="git-panel__rebase-banner">
      <span class="git-panel__rebase-indicator" />
      <span class="git-panel__rebase-text">{{ t('git.rebaseInProgress') }}</span>
    </div>

    <!-- G-FEAT-04: Merge conflict resolver -->
    <div v-if="hasConflicts" class="git-panel__conflicts">
      <div class="git-panel__section-header git-panel__conflicts-header">
        {{ t('git.conflicts', { count: conflictState.conflicts.length }) }}
      </div>
      <div
        v-for="conflict in conflictState.conflicts"
        :key="conflict.file"
        class="git-panel__conflict-row"
      >
        <span class="git-panel__conflict-path" :title="conflict.file">{{ conflict.file }}</span>
        <span class="git-panel__conflict-actions">
          <button
            type="button"
            class="git-panel__conflict-btn git-panel__conflict-btn--ours"
            :disabled="resolvingFile === conflict.file"
            @click="handleAcceptOurs(conflict)"
          >
            {{ t('git.acceptOurs') }}
          </button>
          <button
            type="button"
            class="git-panel__conflict-btn git-panel__conflict-btn--theirs"
            :disabled="resolvingFile === conflict.file"
            @click="handleAcceptTheirs(conflict)"
          >
            {{ t('git.acceptTheirs') }}
          </button>
          <button
            type="button"
            class="git-panel__conflict-btn"
            :disabled="resolvingFile === conflict.file"
            @click="handleOpenEditor(conflict)"
          >
            {{ t('git.openEditor') }}
          </button>
          <button
            type="button"
            class="git-panel__conflict-btn git-panel__conflict-btn--resolved"
            :disabled="resolvingFile === conflict.file"
            @click="handleMarkResolved(conflict.file)"
          >
            {{ t('git.markResolved') }}
          </button>
        </span>
      </div>
    </div>

    <!-- Commit message + button -->
    <div class="git-panel__commit-area">
      <textarea
        v-model="commitMessage"
        class="git-panel__commit-input"
        :placeholder="t('git.commitMessagePlaceholder')"
        rows="2"
        :aria-label="t('git.commitMessageAria')"
      />
      <button
        type="button"
        class="git-panel__commit-btn"
        :disabled="!commitMessage.trim()"
        @click="handleCommit"
      >
        <el-icon :size="12"><Check /></el-icon>
        {{ t('git.commit') }}
      </button>
    </div>

    <!-- Loading -->
    <div v-if="gitState.loading" class="git-panel__loading">
      {{ t('common.loading') }}
    </div>

    <!-- Error -->
    <div v-if="gitState.error" class="git-panel__error">
      {{ gitState.error }}
    </div>

    <!-- Changes list -->
    <div v-if="!gitState.loading && hasChanges" class="git-panel__changes">
      <div class="git-panel__section-header">{{ t('git.changesCount', { count: gitState.changes.length }) }}</div>
      <div
        v-for="change in gitState.changes"
        :key="change.path"
        class="git-panel__row"
      >
        <span class="git-panel__path" :title="change.path">{{ change.path }}</span>
        <span class="git-panel__actions">
          <button
            type="button"
            class="git-panel__action"
            :aria-label="t('git.stage')"
            :title="t('git.stage')"
            @click="handleStage(change.path)"
          >
            <el-icon :size="12"><Plus /></el-icon>
          </button>
          <button
            type="button"
            class="git-panel__action"
            :aria-label="t('git.unstage')"
            :title="t('git.unstage')"
            @click="handleUnstage(change.path)"
          >
            <el-icon :size="12"><Minus /></el-icon>
          </button>
          <button
            type="button"
            class="git-panel__action"
            :aria-label="t('git.viewDiffAria')"
            :title="t('git.diff')"
            @click="viewDiff(change.path)"
          >
            {{ t('git.diff') }}
          </button>
        </span>
        <span class="git-panel__status" :class="statusClass(change.status)">
          {{ statusLabel(change.status) }}
        </span>
      </div>
    </div>

    <!-- Empty state -->
    <div v-if="!gitState.loading && !hasChanges && !gitState.error" class="git-panel__empty">
      {{ t('git.noChanges') }}
    </div>

    <DiffView
      :repo-path="repoPath"
      :file-path="diffFilePath"
      :visible="diffVisible"
      @close="diffVisible = false"
    />

    <!-- AI Code Review modal (#27) -->
    <transition name="fade">
      <div
        v-if="reviewModalVisible"
        class="review-modal-overlay"
        role="dialog"
        aria-modal="true"
        :aria-label="t('git.reviewAria')"
        @click.self="closeReviewModal"
      >
        <div class="review-modal">
          <div class="review-modal__header">
            <div class="review-modal__header-left">
              <el-icon :size="14"><Aim /></el-icon>
              <span class="review-modal__title">{{ t('git.reviewTitle') }}</span>
              <span v-if="reviewState.reviewedFiles.length > 0" class="review-modal__file-count">
                {{ t('git.reviewedFilesCount', { count: reviewState.reviewedFiles.length }) }}
              </span>
            </div>
            <div class="review-modal__header-right">
              <button
                type="button"
                class="review-modal__rerun"
                :disabled="reviewState.loading || !repoPath"
                :title="t('git.rerunTitle')"
                @click="handleRunReview"
              >
                {{ reviewState.loading ? t('git.reviewing') : t('git.rerun') }}
              </button>
              <button
                type="button"
                class="review-modal__close"
                :aria-label="t('git.closeReviewAria')"
                @click="closeReviewModal"
              >
                <el-icon :size="14"><Close /></el-icon>
              </button>
            </div>
          </div>
          <div class="review-modal__body">
            <!-- Loading -->
            <div v-if="reviewState.loading" class="review-modal__loading">
              <div class="review-modal__spinner" />
              <p>{{ t('git.analyzingChanges') }}</p>
            </div>

            <!-- Error -->
            <div v-else-if="reviewState.error" class="review-modal__error">
              <p>{{ reviewState.error }}</p>
              <button
                type="button"
                v-if="repoPath"
                class="review-modal__retry"
                @click="handleRunReview"
              >{{ t('common.retry') }}</button>
            </div>

            <!-- Result -->
            <div v-else-if="hasReview" class="review-modal__result">
              <div v-if="reviewState.reviewedFiles.length > 0" class="review-modal__files">
                <span class="review-modal__files-label">{{ t('git.reviewed') }}</span>
                <span
                  v-for="f in reviewState.reviewedFiles"
                  :key="f"
                  class="review-modal__file-chip"
                  :title="f"
                >{{ f.split('/').pop() }}</span>
              </div>
              <MarkdownContent
                class="review-modal__content markdown-body"
                :html="renderReviewContent(reviewState.result!)"
              />
              <div v-if="reviewState.reviewedAt" class="review-modal__timestamp">
                {{ t('git.reviewedAt', { time: formatReviewTime(reviewState.reviewedAt) }) }}
              </div>
            </div>

            <!-- Empty (no review run yet) -->
            <div v-else class="review-modal__empty">
              <p>{{ t('git.noReviewYet') }}</p>
            </div>
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.git-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  font-family: var(--font-sans);
}

.git-panel__branch-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.git-panel__branch-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--color-text-secondary);
}

.git-panel__branch-current {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  transition: background-color var(--transition-fast);
}

.git-panel__branch-current:hover {
  background-color: var(--color-bg-surface-container-low);
}

.git-panel__ahead,
.git-panel__behind {
  font-size: 10px;
  color: var(--color-text-tertiary);
}

.git-panel__action-btn {
  margin-left: auto;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  line-height: 1;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
  transition: background-color var(--transition-fast);
}

.git-panel__action-btn + .git-panel__action-btn,
.git-panel__action-btn + .git-panel__refresh,
.git-panel__refresh + .git-panel__action-btn {
  margin-left: 0;
}

.git-panel__action-btn:hover {
  color: var(--color-text-primary);
  background-color: var(--color-bg-surface-container-low);
}

.git-panel__refresh {
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
  transition: background-color var(--transition-fast);
}

.git-panel__refresh:hover {
  color: var(--color-text-primary);
  background-color: var(--color-bg-surface-container-low);
}

.git-panel__commit-area {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.git-panel__commit-input {
  width: 100%;
  padding: 8px 10px;
  font-size: 12px;
  font-family: var(--font-sans);
  color: var(--color-text-primary);
  background-color: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-md);
  outline: none;
  resize: vertical;
  transition: border-color var(--transition-fast);
}

.git-panel__commit-input:focus {
  border-color: var(--color-primary);
}

.git-panel__commit-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 6px 16px;
  font-size: 12px;
  font-weight: 500;
  color: #fff;
  background-color: var(--color-primary);
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.git-panel__commit-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.git-panel__commit-btn:not(:disabled):hover {
  background-color: color-mix(in srgb, var(--color-primary) 85%, #000);
}

.git-panel__loading,
.git-panel__empty,
.git-panel__error {
  padding: 12px;
  font-size: 11px;
  color: var(--color-text-tertiary);
}

.git-panel__error {
  color: var(--color-error);
}

.git-panel__section-header {
  padding: 6px 16px 4px;
  font-size: 10px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--color-text-tertiary);
}

.git-panel__changes {
  flex: 1;
  overflow-y: auto;
}

.git-panel__row {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px 16px;
  height: 26px;
  font-size: 12px;
  cursor: default;
  border-radius: var(--radius-sm);
  transition: background-color var(--transition-fast);
}

.git-panel__row:hover {
  background: var(--color-bg-surface-container-low);
}

.git-panel__path {
  flex: 1;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.git-panel__actions {
  display: flex;
  gap: 2px;
  opacity: 0;
  transition: opacity var(--transition-fast);
}

.git-panel__row:hover .git-panel__actions {
  opacity: 1;
}

.git-panel__action {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  border-radius: var(--radius-xs);
  transition: color var(--transition-fast), background-color var(--transition-fast);
}

.git-panel__action:hover {
  color: var(--color-text-primary);
  background-color: var(--color-bg-surface-container-low);
}

.git-panel__status {
  width: 16px;
  text-align: center;
  font-weight: 500;
  font-size: 11px;
}

.git-panel__status--modified { color: var(--color-warning); }
.git-panel__status--added { color: var(--color-success); }
.git-panel__status--deleted { color: var(--color-error); }
.git-panel__status--untracked { color: var(--color-text-disabled); }
.git-panel__status--default { color: var(--color-text-tertiary); }

/* G-FEAT-04: Rebase button variants */
.git-panel__action-btn--warning {
  color: var(--color-error, #ef4444);
}
.git-panel__action-btn--warning:hover {
  background-color: color-mix(in srgb, var(--color-error, #ef4444) 12%, transparent);
}
.git-panel__action-btn--success {
  color: var(--color-success, #22c55e);
}
.git-panel__action-btn--success:hover {
  background-color: color-mix(in srgb, var(--color-success, #22c55e) 12%, transparent);
}

/* G-FEAT-04: Rebase in progress banner */
.git-panel__rebase-banner {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 16px;
  background-color: color-mix(in srgb, var(--color-warning, #f59e0b) 10%, transparent);
  border-bottom: 1px solid var(--color-border-subtle);
}

.git-panel__rebase-indicator {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: var(--color-warning, #f59e0b);
  animation: git-pulse 1.4s ease-in-out infinite;
}

@keyframes git-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

@media (prefers-reduced-motion: reduce) {
  .git-panel__rebase-indicator { animation: none; }
}

.git-panel__rebase-text {
  font-size: 11px;
  color: var(--color-warning, #f59e0b);
  font-weight: 500;
}

/* G-FEAT-04: Merge conflict resolver */
.git-panel__conflicts {
  border-bottom: 1px solid var(--color-border-subtle);
}

.git-panel__conflicts-header {
  color: var(--color-error, #ef4444);
}

.git-panel__conflict-row {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 16px;
  font-size: 12px;
  border-bottom: 1px solid var(--color-border-subtle, transparent);
}

.git-panel__conflict-row:last-child {
  border-bottom: none;
}

.git-panel__conflict-path {
  flex: 1;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: var(--font-mono);
  font-size: 11px;
}

.git-panel__conflict-actions {
  display: flex;
  gap: 3px;
  flex-shrink: 0;
}

.git-panel__conflict-btn {
  padding: 2px 6px;
  font-size: 10px;
  font-family: var(--font-sans);
  color: var(--color-text-tertiary);
  background: var(--color-bg-surface-container-low);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-xs);
  cursor: pointer;
  transition: all var(--transition-fast);
  white-space: nowrap;
}

.git-panel__conflict-btn:hover:not(:disabled) {
  color: var(--color-text-primary);
  background: var(--color-bg-surface-container);
}

.git-panel__conflict-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.git-panel__conflict-btn--ours:hover:not(:disabled) {
  color: var(--color-primary);
  border-color: var(--color-primary);
}

.git-panel__conflict-btn--theirs:hover:not(:disabled) {
  color: var(--color-primary);
  border-color: var(--color-primary);
}

.git-panel__conflict-btn--resolved:hover:not(:disabled) {
  color: var(--color-success, #22c55e);
  border-color: var(--color-success, #22c55e);
}

/* AI Code Review button */
.git-panel__review-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 500;
  color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--color-primary) 30%, transparent);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background-color var(--transition-fast), color var(--transition-fast);
}

.git-panel__review-btn:hover {
  background: color-mix(in srgb, var(--color-primary) 16%, transparent);
}

/* AI Code Review modal */
.review-modal-overlay {
  position: fixed;
  inset: 0;
  background-color: color-mix(in srgb, var(--color-bg-base) 75%, transparent);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2000;
  padding: 24px;
}

.review-modal {
  display: flex;
  flex-direction: column;
  width: min(720px, 95vw);
  height: min(640px, 88vh);
  background-color: var(--color-bg-surface);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-3, 0 12px 32px rgba(0, 0, 0, 0.4));
  overflow: hidden;
}

.review-modal__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid var(--color-border-subtle);
  background-color: var(--color-bg-elevated);
}

.review-modal__header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--color-primary);
}

.review-modal__title {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-primary);
}

.review-modal__file-count {
  font-size: 10px;
  color: var(--color-text-tertiary);
  padding: 1px 6px;
  background: var(--color-bg-surface);
  border-radius: 8px;
}

.review-modal__header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.review-modal__rerun {
  padding: 4px 12px;
  font-size: 11px;
  font-family: var(--font-sans);
  color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--color-primary) 30%, transparent);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.review-modal__rerun:hover:not(:disabled) {
  background: color-mix(in srgb, var(--color-primary) 16%, transparent);
}

.review-modal__rerun:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.review-modal__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  transition: color var(--transition-fast), background-color var(--transition-fast);
}

.review-modal__close:hover {
  color: var(--color-text-primary);
  background-color: color-mix(in srgb, var(--color-text-tertiary) 12%, transparent);
}

.review-modal__body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
}

.review-modal__loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 16px;
  color: var(--color-text-tertiary);
  font-size: 12px;
}

.review-modal__spinner {
  width: 32px;
  height: 32px;
  border: 2px solid var(--color-border-subtle);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: review-spin 0.8s linear infinite;
}

@keyframes review-spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .review-modal__spinner { animation: none; }
}

.review-modal__error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 12px;
  color: var(--color-error);
  font-size: 12px;
  text-align: center;
}

.review-modal__retry {
  padding: 6px 14px;
  font-size: 12px;
  color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  border: 1px solid var(--color-primary);
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.review-modal__retry:hover {
  background: var(--color-primary);
  color: #fff;
}

.review-modal__result {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.review-modal__files {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.review-modal__files-label {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-tertiary);
}

.review-modal__file-chip {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  font-size: 10px;
  font-family: var(--font-mono);
  color: var(--color-text-secondary);
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-xs);
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.review-modal__content {
  font-size: 13px;
  line-height: 1.6;
  color: var(--color-text-primary);
}

.review-modal__content :deep(pre) {
  margin: 8px 0;
  padding: 12px 16px;
  background-color: var(--hljs-bg, var(--color-bg-base));
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  overflow-x: auto;
  font-size: 13px;
  line-height: 1.5;
}

.review-modal__content :deep(code) {
  font-family: var(--font-mono);
  font-size: 13px;
}

.review-modal__content :deep(code.hljs) {
  background: transparent;
  padding: 0;
  font-weight: 500;
}

.review-modal__content :deep(p) {
  margin: 6px 0;
}

.review-modal__content :deep(ul),
.review-modal__content :deep(ol) {
  margin: 6px 0;
  padding-left: 20px;
}

.review-modal__content :deep(h1),
.review-modal__content :deep(h2),
.review-modal__content :deep(h3) {
  margin: 12px 0 6px;
  font-size: 14px;
  font-weight: 600;
}

.review-modal__timestamp {
  padding-top: 8px;
  border-top: 1px solid var(--color-border-subtle);
  font-size: 10px;
  color: var(--color-text-tertiary);
}

.review-modal__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--color-text-tertiary);
  font-size: 12px;
  text-align: center;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity var(--transition-fast);
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .fade-enter-active,
  .fade-leave-active {
    transition: none;
  }
}
</style>
