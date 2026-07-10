<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { appState } from "@/stores/app";
import { searchState, debouncedSearch, clearSearch, replaceAll } from "@/stores/search";
import { openFileFromPath } from "@/stores/editor";
import { ElMessage } from "element-plus";
import { Search, Switch } from "@element-plus/icons-vue";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();

const repoPath = computed(() => appState.currentProject ?? "");
const localQuery = ref(searchState.query);
const caseSensitive = ref(!searchState.ignoreCase);
const showReplace = ref(false);
const replaceText = ref("");
const replacing = ref(false);

const totalMatches = computed(() =>
  searchState.results.reduce((sum, r) => sum + r.matches.length, 0),
);

const hasResults = computed(() => searchState.results.length > 0);

function handleInput() {
  if (!repoPath.value) return;
  searchState.ignoreCase = !caseSensitive.value;
  debouncedSearch(repoPath.value, localQuery.value);
}

function handleClear() {
  localQuery.value = "";
  clearSearch();
}

async function handleMatchClick(filePath: string, _line: number) {
  if (!repoPath.value) return;
  const fullPath = repoPath.value + "/" + filePath;
  await openFileFromPath(fullPath);
  // Note: scrolling to line requires Monaco integration that is out of scope
  // for this task. The file opens; the user can navigate to the line manually.
}

function toggleCaseSensitive() {
  caseSensitive.value = !caseSensitive.value;
  searchState.ignoreCase = !caseSensitive.value;
  if (localQuery.value) {
    handleInput();
  }
}

async function handleReplaceAll() {
  if (!localQuery.value || !repoPath.value) return;
  replacing.value = true;
  try {
    const total = await replaceAll(repoPath.value, localQuery.value, replaceText.value, caseSensitive.value);
    ElMessage.success(t("search.replacementsMade", { count: total }));
    handleInput();
  } catch (e: unknown) {
    ElMessage.error(t("search.replaceFailed", { error: errorMessage(e) }));
  } finally {
    replacing.value = false;
  }
}

watch(() => appState.currentProject, () => {
  localQuery.value = "";
  clearSearch();
});
</script>

<template>
  <div class="search-panel">
    <!-- Search input -->
    <div class="search-panel__input-area">
      <div class="search-panel__input-wrap">
        <el-icon :size="12" class="search-panel__icon">
          <Search />
        </el-icon>
        <input
          v-model="localQuery"
          type="text"
          class="search-panel__input"
          :placeholder="t('search.placeholder')"
          :aria-label="t('search.queryAria')"
          @input="handleInput"
        />
        <button
          type="button"
          v-if="localQuery"
          class="search-panel__clear"
          :aria-label="t('search.clearAria')"
          @click="handleClear"
        >
          ×
        </button>
      </div>
      <button
        type="button"
        class="search-panel__case-btn"
        :class="{ 'search-panel__case-btn--active': caseSensitive }"
        :aria-pressed="caseSensitive"
        :title="t('search.matchCase')"
        @click="toggleCaseSensitive"
      >
        Aa
      </button>
      <button
        type="button"
        class="search-panel__toggle-replace"
        :class="{ active: showReplace }"
        @click="showReplace = !showReplace"
        :aria-label="t('search.toggleReplaceAria')"
        :title="t('search.toggleReplaceTitle')"
      >
        <el-icon :size="12"><Switch /></el-icon>
      </button>
    </div>

    <div v-if="showReplace" class="search-panel__replace-area">
      <input
        v-model="replaceText"
        class="search-panel__replace-input"
        :placeholder="t('search.replacePlaceholder')"
        @keydown.enter="handleReplaceAll"
      />
      <button
        type="button"
        class="search-panel__replace-btn"
        :disabled="replacing || !hasResults"
        @click="handleReplaceAll"
      >
        {{ replacing ? t('search.replaceInProgress') : t('search.replaceAll') }}
      </button>
    </div>

    <!-- Summary -->
    <div v-if="hasResults && !searchState.loading" class="search-panel__summary">
      {{ t('search.summary', { files: searchState.results.length, matches: totalMatches }) }}
    </div>

    <!-- Loading -->
    <div v-if="searchState.loading" class="search-panel__loading">
      {{ t('search.searching') }}
    </div>

    <!-- Error -->
    <div v-if="searchState.error" class="search-panel__error">
      {{ searchState.error }}
    </div>

    <!-- Results -->
    <div v-if="hasResults && !searchState.loading" class="search-panel__results">
      <div v-for="result in searchState.results" :key="result.path" class="search-panel__file-group">
        <div class="search-panel__file-path" :title="result.path">
          {{ result.path }}
          <span class="search-panel__file-count">{{ result.matches.length }}</span>
        </div>
        <button
          type="button"
          v-for="(match, idx) in result.matches"
          :key="idx"
          class="search-panel__match"
          @click="handleMatchClick(result.path, match.line)"
        >
          <span class="search-panel__line-num">{{ match.line }}</span>
          <span class="search-panel__preview">{{ match.preview }}</span>
        </button>
      </div>
    </div>

    <!-- Empty state -->
    <div
      v-if="!hasResults && !searchState.loading && localQuery && !searchState.error"
      class="search-panel__empty"
    >
      {{ t('search.noResults') }}
    </div>
    <div
      v-if="!localQuery && !hasResults"
      class="search-panel__empty"
    >
      {{ t('search.typeToSearch') }}
    </div>
  </div>
</template>

<style scoped>
.search-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  font-family: var(--font-sans);
}

.search-panel__input-area {
  display: flex;
  gap: 4px;
  padding: 8px 12px;
}

.search-panel__input-wrap {
  position: relative;
  flex: 1;
  display: flex;
  align-items: center;
}

.search-panel__icon {
  position: absolute;
  left: 8px;
  color: var(--color-text-tertiary);
  pointer-events: none;
}

.search-panel__input {
  width: 100%;
  padding: 8px 28px 8px 30px;
  font-size: 12px;
  font-family: var(--font-sans);
  color: var(--color-text-primary);
  background-color: var(--color-bg-elevated);
  border: 1px solid transparent;
  border-radius: var(--radius-md, 12px);
  outline: none;
  transition: border-color var(--transition-fast, 150ms var(--ease-standard));
}

.search-panel__input:focus {
  border-color: var(--color-primary);
}

.search-panel__clear {
  position: absolute;
  right: 4px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
  padding: 2px 4px;
  border-radius: var(--radius-sm, 8px);
  transition: color var(--transition-fast, 150ms var(--ease-standard));
}

.search-panel__clear:hover {
  color: var(--color-text-primary);
}

.search-panel__case-btn {
  width: 32px;
  height: 32px;
  border: 1px solid var(--color-border-subtle);
  background: transparent;
  color: var(--color-text-tertiary);
  font-size: 11px;
  font-weight: 500;
  border-radius: var(--radius-sm, 8px);
  cursor: pointer;
  transition: background-color var(--transition-fast, 150ms var(--ease-standard)),
              border-color var(--transition-fast, 150ms var(--ease-standard)),
              color var(--transition-fast, 150ms var(--ease-standard));
}

.search-panel__case-btn--active {
  background: var(--color-primary-container);
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.search-panel__toggle-replace {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
}

.search-panel__toggle-replace:hover,
.search-panel__toggle-replace.active {
  color: var(--color-text-primary);
  background: var(--color-bg-surface-container-low);
}

.search-panel__replace-area {
  display: flex;
  gap: 4px;
  padding: 0 8px 6px;
}

.search-panel__replace-input {
  flex: 1;
  height: 24px;
  padding: 0 8px;
  font-size: 12px;
  color: var(--color-text-primary);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-xs);
}

.search-panel__replace-btn {
  padding: 0 10px;
  height: 24px;
  font-size: 11px;
  color: var(--color-text-primary);
  background: var(--color-primary);
  border: none;
  border-radius: var(--radius-xs);
  cursor: pointer;
}

.search-panel__replace-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.search-panel__summary,
.search-panel__loading,
.search-panel__empty,
.search-panel__error {
  padding: 4px 12px;
  font-size: 11px;
  color: var(--color-text-tertiary);
}

.search-panel__error {
  color: var(--color-error);
}

.search-panel__results {
  flex: 1;
  overflow-y: auto;
  padding-bottom: 8px;
}

.search-panel__file-group {
  margin-bottom: 4px;
}

.search-panel__file-path {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px 2px;
  font-size: 11px;
  font-weight: 500;
  color: var(--color-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.search-panel__file-count {
  flex-shrink: 0;
  padding: 1px 7px;
  font-size: 10px;
  color: var(--color-text-tertiary);
  background-color: var(--color-bg-surface-container);
  border-radius: var(--radius-sm, 8px);
}

.search-panel__match {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 4px 12px 4px 28px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm, 8px);
  cursor: pointer;
  text-align: left;
  transition: background-color var(--transition-fast, 150ms var(--ease-standard));
}

.search-panel__match:hover {
  background-color: var(--color-bg-surface-container-low);
}

.search-panel__line-num {
  flex-shrink: 0;
  width: 28px;
  font-size: 10px;
  color: var(--color-text-disabled);
  font-family: var(--font-mono);
  text-align: right;
}

.search-panel__preview {
  flex: 1;
  font-size: 11px;
  color: var(--color-text-primary);
  font-family: var(--font-mono);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>