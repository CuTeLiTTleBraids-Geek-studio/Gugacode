<script setup lang="ts">
import { computed, onMounted, watch } from "vue";
import { appState, toggleSidebar } from "@/stores/app";
import { Close } from "@element-plus/icons-vue";
import FileTree from "@/components/explorer/FileTree.vue";
import GitPanel from "@/components/layout/GitPanel.vue";
import SearchPanel from "@/components/layout/SearchPanel.vue";
import AiChatPanel from "@/components/layout/AiChatPanel.vue";
import { openFileFromPath } from "@/stores/editor";
import { gitState, refreshGit } from "@/stores/git";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();

const isCollapsed = computed(() => appState.sidebarCollapsed);
const currentTab = computed(() => appState.panelTab);
const panelTitle = computed(() => {
  switch (currentTab.value) {
    case "search":
      return t("activity.search");
    case "git":
      return t("activity.sourceControl");
    case "extensions":
      return t("activity.extensions");
    case "ai":
      return t("activity.ai");
    default:
      return t("activity.explorer");
  }
});
const projectPath = computed(() => appState.currentProject);
const projectName = computed(() => appState.projectName ?? t("sidePanel.defaultProjectName"));
// N-20: bind width to appState so the drag handle can resize the sidebar.
const panelWidthPx = computed(() =>
  isCollapsed.value ? "0px" : `${appState.sidebarWidth}px`,
);

function handleFileSelect(path: string) {
  openFileFromPath(path);
}

const emptyMessage = computed(() => {
  if (currentTab.value === "extensions") return t("sidePanel.noExtensions");
  if (currentTab.value === "ai") return t("sidePanel.aiReady");
  if (projectPath.value) return panelTitle.value;
  return t("sidePanel.openProjectToStart");
});

// Sync git branch name to appState for StatusBar
watch(
  () => gitState.branchName,
  (name) => {
    if (name) appState.branchName = name;
  },
);

onMounted(() => {
  if (projectPath.value && currentTab.value === "git") {
    refreshGit(projectPath.value);
  }
});

watch(
  [currentTab, projectPath],
  ([tab, path]) => {
    if (tab === "git" && path) {
      refreshGit(path as string);
    }
  },
);
</script>

<template>
  <aside
    class="side-panel"
    :class="{ 'side-panel--collapsed': isCollapsed }"
    :style="{ width: panelWidthPx }"
    role="complementary"
    :aria-label="t('sidePanel.panelAria', { title: panelTitle })"
  >
    <div class="side-panel__content">
      <!-- Panel header -->
      <div class="side-panel__header">
        <span class="side-panel__title">{{ panelTitle }}</span>
        <button
          type="button"
          class="side-panel__close"
          :aria-label="t('sidePanel.closePanelAria')"
          :title="t('sidePanel.closePanelTitle')"
          @click="toggleSidebar"
        >
          <el-icon :size="14">
            <Close />
          </el-icon>
        </button>
      </div>

      <!-- Panel body -->
      <div
        class="side-panel__body"
        :class="{ 'side-panel__body--chat': currentTab === 'ai' }"
      >
        <Transition name="side-panel-fade" mode="out-in">
          <!-- Explorer: file tree -->
          <div v-if="currentTab === 'explorer' && projectPath" key="explorer" class="side-panel__explorer">
            <div class="side-panel__project-header">{{ projectName }}</div>
            <FileTree :path="projectPath" :name="projectName" :depth="0" @select="handleFileSelect" />
          </div>

          <!-- Search panel -->
          <SearchPanel v-else-if="currentTab === 'search' && projectPath" key="search" />

          <!-- Git panel -->
          <GitPanel v-else-if="currentTab === 'git' && projectPath" key="git" />

          <!-- AI chat panel (embedded，占用侧边栏空间，不挡住代码) -->
          <AiChatPanel v-else-if="currentTab === 'ai'" key="ai" embedded />

          <!-- Empty state for other tabs -->
          <div v-else key="empty" class="side-panel__empty">
            <div class="side-panel__empty-line" aria-hidden="true" />
            <p class="side-panel__empty-text">
              {{ emptyMessage }}
            </p>
          </div>
        </Transition>
      </div>
    </div>
  </aside>
</template>

<style scoped>
/* Apple SidePanel：Parchment 背景、发丝级边框、无阴影 */
.side-panel {
  min-width: 0;
  height: 100%;
  background: var(--color-sidebar-bg);
  overflow: hidden;
  flex-shrink: 0;
  z-index: 5;
  transition: width var(--transition-normal);
  /* Apple hairline 分割：色块本身就是分割，仅极弱边框 */
  border-right: 0.5px solid var(--color-border-subtle);
}

.side-panel--collapsed {
  width: 0;
  border-right: none;
}

.side-panel__content {
  display: flex;
  flex-direction: column;
  width: 100%;
  min-width: 140px;
  height: 100%;
  opacity: 1;
  transition: opacity var(--transition-fast);
}

.side-panel--collapsed .side-panel__content {
  opacity: 0;
  pointer-events: none;
}

/* Apple sub-nav 风格 header：52px 高、tagline 字体 */
.side-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  height: 44px;
  min-height: 44px;
}

.side-panel__title {
  /* Apple tagline 21px / 600 / 0.231px tracking */
  font-size: 14px;
  font-weight: 600;
  letter-spacing: -0.224px;
  color: var(--color-text-primary);
}

.side-panel__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  transition:
    color var(--transition-fast),
    background-color var(--transition-fast),
    transform var(--transition-fast);
}

.side-panel__close:hover {
  color: var(--color-text-secondary);
  background-color: var(--color-border-subtle);
}

.side-panel__close:active {
  transform: scale(0.95);
}

.side-panel__close:focus-visible {
  outline: 2px solid var(--color-primary-focus);
  outline-offset: -2px;
}

.side-panel__body {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}

.side-panel__body--chat {
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.side-panel__explorer {
  padding: 0 4px;
}

.side-panel__project-header {
  padding: 6px 12px 6px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: -0.12px;
  color: var(--color-text-secondary);
}

.side-panel__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 32px 16px;
  text-align: center;
}

.side-panel__empty-line {
  width: 32px;
  height: 1px;
  background-color: var(--color-hairline);
  margin-bottom: 12px;
}

.side-panel__empty-text {
  font-size: 14px;
  color: var(--color-text-tertiary);
  line-height: 1.43;
  letter-spacing: -0.224px;
}

@media (prefers-reduced-motion: reduce) {
  .side-panel,
  .side-panel__content,
  .side-panel__close {
    transition: none;
  }
  .side-panel__close:active {
    transform: none;
  }
}

/* 侧边栏 tab 内容切换的丝滑过渡动画。
   out-in 模式：旧内容先淡出，新内容再淡入上移。 */
.side-panel-fade-enter-active {
  transition: opacity 0.2s cubic-bezier(0.4, 0, 0.2, 1),
              transform 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.side-panel-fade-leave-active {
  transition: opacity 0.14s ease-out;
}

.side-panel-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.side-panel-fade-leave-to {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .side-panel-fade-enter-active,
  .side-panel-fade-leave-active {
    transition: none;
  }
  .side-panel-fade-enter-from,
  .side-panel-fade-leave-to {
    opacity: 1;
    transform: none;
  }
}
</style>
