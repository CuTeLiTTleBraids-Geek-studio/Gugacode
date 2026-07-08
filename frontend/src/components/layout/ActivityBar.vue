<script setup lang="ts">
import { appState, setPanelTab, toggleSidebar } from "@/stores/app";
import type { PanelTab } from "@/stores/app";
import {
  FolderOpened,
  Search,
  SetUp,
  Connection,
  MagicStick,
  Setting,
} from "@element-plus/icons-vue";
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();
const router = useRouter();
const route = useRoute();

interface ActivityItem {
  icon: typeof FolderOpened;
  labelKey: string;
  tab: PanelTab | null;
  isBottom?: boolean;
}

const items: ActivityItem[] = [
  { icon: FolderOpened, labelKey: "activity.explorer", tab: "explorer" },
  { icon: Search, labelKey: "activity.search", tab: "search" },
  { icon: Connection, labelKey: "activity.sourceControl", tab: "git" },
  { icon: SetUp, labelKey: "activity.extensions", tab: "extensions" },
  { icon: MagicStick, labelKey: "activity.ai", tab: "ai" },
];

const settingsItem: ActivityItem = {
  icon: Setting,
  labelKey: "activity.settings",
  tab: null,
  isBottom: true,
};

const activeTab = computed(() => appState.panelTab);
const isAiActive = computed(() => appState.panelTab === "ai");
const isSettingsActive = computed(() => route.path === "/settings");

function handleClick(item: ActivityItem) {
  if (item.tab) {
    // VS Code 风格：点击当前 active tab 折叠/展开侧边栏；
    // 点击其他 tab 切换并确保侧边栏展开（解决关闭后无法呼出的问题）。
    if (activeTab.value === item.tab && !appState.sidebarCollapsed) {
      toggleSidebar();
    } else {
      setPanelTab(item.tab);
      if (appState.sidebarCollapsed) {
        toggleSidebar();
      }
    }
  } else if (item.isBottom) {
    // 已在 settings 页面时再次点击则返回 /editor；否则进入 settings。
    if (isSettingsActive.value) {
      router.push("/editor");
    } else {
      router.push("/settings");
    }
  }
}
</script>

<template>
  <aside
    class="activity-bar"
    role="toolbar"
    :aria-label="t('activityBar.toolbarAria')"
  >
    <div class="activity-bar__top">
      <button
        type="button"
        v-for="item in items"
        :key="item.labelKey"
        class="activity-bar__item"
        :class="{
          'activity-bar__item--active': activeTab === item.tab,
        }"
        :aria-label="t(item.labelKey)"
        :aria-pressed="activeTab === item.tab"
        :title="t(item.labelKey)"
        @click="handleClick(item)"
      >
        <el-icon :size="20">
          <component :is="item.icon" />
        </el-icon>
        <!-- AI indicator dot -->
        <span
          v-if="item.tab === 'ai' && isAiActive"
          class="activity-bar__dot"
          aria-hidden="true"
        />
      </button>
    </div>

    <div class="activity-bar__bottom">
      <button
        type="button"
        class="activity-bar__item"
        :class="{ 'activity-bar__item--active': isSettingsActive }"
        :aria-label="t(settingsItem.labelKey)"
        :aria-pressed="isSettingsActive"
        :title="t(settingsItem.labelKey)"
        @click="handleClick(settingsItem)"
      >
        <el-icon :size="20">
          <component :is="settingsItem.icon" />
        </el-icon>
      </button>
    </div>
  </aside>
</template>

<style scoped>
/* Apple 风格 ActivityBar：纯黑背景、与 titlebar 同色形成统一全局导航 */
.activity-bar {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: space-between;
  width: 52px;
  min-width: 52px;
  height: 100%;
  background-color: var(--color-activitybar-bg);
  padding: 8px 0;
  z-index: 10;
}

.activity-bar__top,
.activity-bar__bottom {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 0 6px;
}

.activity-bar__bottom {
  padding-top: 8px;
  margin-top: auto;
  /* Apple 风格：用发丝级透明线，而非明显边框 */
  border-top: 0.5px solid var(--chrome-border);
}

.activity-bar__item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border: none;
  /* Apple pill 容器：8px 圆角 */
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--chrome-text-secondary);
  cursor: pointer;
  transition:
    background-color var(--transition-fast),
    color var(--transition-fast),
    transform var(--transition-fast);
}

.activity-bar__item:hover {
  background-color: var(--chrome-hover-bg);
  color: var(--chrome-text-primary);
}

.activity-bar__item:active {
  /* Apple 微交互 */
  transform: scale(0.95);
}

/* Active 状态：使用 chrome-text-active（深/浅模式自适应） */
.activity-bar__item--active {
  color: var(--chrome-text-active);
  background-color: var(--chrome-active-bg);
}

.activity-bar__item--active::before {
  content: "";
  position: absolute;
  left: -6px;
  top: 50%;
  transform: translateY(-50%);
  width: 2px;
  height: 18px;
  border-radius: 2px;
  background: var(--chrome-text-active);
}

.activity-bar__item--active:hover {
  background-color: var(--chrome-active-bg);
  color: var(--chrome-text-active);
}

.activity-bar__dot {
  position: absolute;
  top: 7px;
  right: 7px;
  width: 6px;
  height: 6px;
  border-radius: var(--radius-full);
  background-color: var(--color-success);
  border: 1.5px solid var(--color-activitybar-bg);
  pointer-events: none;
}

.activity-bar__item:focus-visible {
  outline: 2px solid var(--color-primary-focus);
  outline-offset: -2px;
}

@media (prefers-reduced-motion: reduce) {
  .activity-bar__item {
    transition: none;
  }
  .activity-bar__item:active {
    transform: none;
  }
}
</style>
