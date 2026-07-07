<script setup lang="ts">
import { appState, toggleTerminal } from "@/stores/app";
import { editorState, activeFile } from "@/stores/editor";
import { toggleInlineCompletion } from "@/stores/inlineCompletion";
import { computed } from "vue";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();

const branchName = computed(() => appState.branchName || "—");
const errors = computed(() => appState.errors);
const warnings = computed(() => appState.warnings);
const cursorLine = computed(() => appState.cursorLine);
const cursorColumn = computed(() => appState.cursorColumn);
const encoding = computed(() => appState.encoding);
const languageMode = computed(() => activeFile.value?.language ?? appState.languageMode);
const hasProblems = computed(() => errors.value > 0 || warnings.value > 0);
const hasOpenFile = computed(() => editorState.openFiles.length > 0);
const inlineCompletionLabel = computed(() =>
  appState.inlineCompletionEnabled ? t("statusBar.aiCompletionOn") : t("statusBar.aiCompletionOff"),
);
</script>

<template>
  <footer
    class="statusbar"
    role="status"
    :aria-label="t('statusBar.statusBar')"
  >
    <!-- Left side -->
    <div class="statusbar__left">
      <span
        class="statusbar__item statusbar__item--branch"
        role="status"
        :aria-label="t('statusBar.currentBranchAria', { name: branchName })"
        :title="t('statusBar.currentBranch')"
      >
        <span class="statusbar__branch-symbol" aria-hidden="true">&#x2387;</span>
        <span>{{ branchName }}</span>
      </span>

      <span
        v-if="hasProblems"
        class="statusbar__item"
        role="status"
        :aria-label="t('statusBar.problemsAria', { errors, warnings })"
        :title="t('statusBar.errorsAndWarnings')"
      >
        <span v-if="errors > 0" class="statusbar__problem statusbar__problem--error" aria-hidden="true">
          <span class="statusbar__dot statusbar__dot--error" />
          {{ errors }}
        </span>
        <span v-if="warnings > 0" class="statusbar__problem statusbar__problem--warning" aria-hidden="true">
          <span class="statusbar__dot statusbar__dot--warning" />
          {{ warnings }}
        </span>
      </span>
    </div>

    <!-- Right side -->
    <div class="statusbar__right">
      <button
        v-if="hasOpenFile"
        type="button"
        class="statusbar__item"
        :aria-label="t('statusBar.lineColumnAria', { line: cursorLine, column: cursorColumn })"
      >
        {{ t("statusBar.lineColumn", { line: cursorLine, column: cursorColumn }) }}
      </button>
      <button
        v-if="hasOpenFile"
        type="button"
        class="statusbar__item"
        :aria-label="t('statusBar.encodingAria', { encoding })"
      >
        {{ encoding }}
      </button>
      <button
        type="button"
        class="statusbar__item"
        :aria-label="t('statusBar.languageAria', { language: languageMode })"
      >
        {{ languageMode }}
      </button>
      <button
        type="button"
        class="statusbar__item"
        :class="{ 'statusbar__item--active': appState.terminalVisible }"
        :aria-label="t('statusBar.toggleTerminalAria', { state: appState.terminalVisible ? t('statusBar.on') : t('statusBar.off') })"
        :title="t('statusBar.toggleTerminal')"
        @click="toggleTerminal"
      >
        <span
          class="statusbar__dot"
          :class="appState.terminalVisible ? 'statusbar__dot--success' : 'statusbar__dot--muted'"
          aria-hidden="true"
        />
        {{ t("statusBar.terminal") }}
      </button>
      <button
        type="button"
        class="statusbar__item"
        :class="{ 'statusbar__item--active': appState.inlineCompletionEnabled }"
        :aria-label="t('statusBar.toggleHint', { label: inlineCompletionLabel })"
        :title="inlineCompletionLabel"
        @click="toggleInlineCompletion"
      >
        <span
          class="statusbar__dot"
          :class="appState.inlineCompletionEnabled ? 'statusbar__dot--success' : 'statusbar__dot--muted'"
          aria-hidden="true"
        />
        {{ t("statusBar.aiLabel") }}
      </button>
    </div>
  </footer>
</template>

<style scoped>
/* Apple 风格 StatusBar：与全局导航同色（纯黑），形成视觉框架 */
.statusbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 28px;
  min-height: 28px;
  padding: 0 12px;
  background-color: var(--color-statusbar-bg);
  /* Apple 风格：无装饰边框，色块本身就是分割 */
  border-top: none;
  z-index: 15;
  user-select: none;
}

.statusbar__left,
.statusbar__right {
  display: flex;
  align-items: center;
  gap: 4px;
}

.statusbar__item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  /* Apple nav-link 12px / 400 / -0.12px */
  font-size: 12px;
  font-family: var(--font-sans);
  font-weight: 400;
  letter-spacing: -0.12px;
  color: var(--chrome-text-secondary);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  cursor: default;
  white-space: nowrap;
  line-height: 1;
  transition: background-color var(--transition-fast),
              color var(--transition-fast),
              transform var(--transition-fast);
}

.statusbar__item--branch {
  cursor: pointer;
  color: var(--chrome-text-primary);
}

.statusbar__item--branch:hover {
  background-color: var(--chrome-hover-bg);
  color: var(--chrome-text-primary);
}

.statusbar__item--branch:active {
  transform: scale(0.95);
}

/* Branch symbol — chrome-text-active（深/浅模式自适应） */
.statusbar__branch-symbol {
  font-size: 13px;
  line-height: 1;
  color: var(--chrome-text-active);
}

.statusbar__problem {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.statusbar__problem--error {
  color: var(--color-error);
}

.statusbar__problem--warning {
  color: var(--color-warning);
}

.statusbar__dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
}

.statusbar__dot--error {
  background-color: var(--color-error);
}

.statusbar__dot--warning {
  background-color: var(--color-warning);
}

.statusbar__dot--success {
  background-color: var(--color-success);
}

.statusbar__dot--muted {
  background-color: var(--chrome-text-muted);
}

.statusbar__item--active {
  cursor: pointer;
}

.statusbar__item--active:hover {
  background-color: var(--chrome-hover-bg);
  color: var(--chrome-text-primary);
}

.statusbar__item--active:active {
  transform: scale(0.95);
}

button.statusbar__item:focus-visible {
  outline: 2px solid var(--color-primary-focus);
  outline-offset: -2px;
}
</style>
