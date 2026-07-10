<script setup lang="ts">
import { ref } from "vue";
import GeneralSection from "@/components/settings/GeneralSection.vue";
import EditorSection from "@/components/settings/EditorSection.vue";
import TerminalSection from "@/components/settings/TerminalSection.vue";
import ShortcutsSection from "@/components/settings/ShortcutsSection.vue";
import AppearanceSection from "@/components/settings/AppearanceSection.vue";
import ProfileSection from "@/components/settings/ProfileSection.vue";
import { useI18n } from "@/lib/i18n";

type SettingsSection = "general" | "editor" | "terminal" | "shortcuts" | "appearance" | "profiles";

const { t } = useI18n();
const activeSection = ref<SettingsSection>("general");

const primaryNavItems: { key: SettingsSection; labelKey: string }[] = [
  { key: "general", labelKey: "settings.general" },
  { key: "editor", labelKey: "settings.editor" },
  { key: "terminal", labelKey: "settings.terminal" },
  { key: "shortcuts", labelKey: "settings.shortcuts" },
  { key: "appearance", labelKey: "settings.appearance" },
  { key: "profiles", labelKey: "settings.profiles" },
];

function selectSection(key: SettingsSection) {
  activeSection.value = key;
}
</script>

<template>
  <div class="settings-view">
    <aside class="settings-nav">
      <ul class="settings-nav-list">
        <li
          v-for="item in primaryNavItems"
          :key="item.key"
          class="settings-nav-item"
        >
          <button
            type="button"
            class="settings-nav-btn"
            :class="{ 'is-active': activeSection === item.key }"
            :aria-label="t(item.labelKey)"
            :aria-current="activeSection === item.key ? 'page' : undefined"
            @click="selectSection(item.key)"
          >
            <span class="settings-nav-indicator" aria-hidden="true" />
            <span class="settings-nav-label">{{ t(item.labelKey) }}</span>
          </button>
        </li>
      </ul>
    </aside>

    <main class="settings-content">
      <GeneralSection v-show="activeSection === 'general'" />
      <EditorSection v-show="activeSection === 'editor'" />
      <TerminalSection v-show="activeSection === 'terminal'" />
      <ShortcutsSection v-show="activeSection === 'shortcuts'" />
      <AppearanceSection v-show="activeSection === 'appearance'" />
      <ProfileSection v-show="activeSection === 'profiles'" />
    </main>
  </div>
</template>

<style scoped>
.settings-view {
  display: flex;
  height: 100%;
  overflow: hidden;
}

.settings-nav {
  width: 200px;
  border-right: 1px solid var(--color-border-default);
  background: var(--color-bg-surface);
  padding: 16px 0;
  overflow-y: auto;
}

.settings-nav-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.settings-nav-item {
  margin: 2px 8px;
}

.settings-nav-btn {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  text-align: left;
  padding: 8px 16px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  font-family: var(--font-sans);
  font-size: 13px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  overflow: hidden;
  transition: background var(--transition-fast), color var(--transition-fast), transform 0.18s cubic-bezier(0.4, 0, 0.2, 1);
}

/* 左侧高亮指示条，宽度/opacity 过渡实现丝滑激活效果 */
.settings-nav-indicator {
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%) scaleY(0);
  width: 3px;
  height: 60%;
  border-radius: 0 3px 3px 0;
  background: var(--color-primary);
  opacity: 0;
  transition: transform 0.22s cubic-bezier(0.4, 0, 0.2, 1), opacity 0.22s ease;
}

.settings-nav-btn:hover {
  background: var(--color-sidebar-hover);
  color: var(--color-text-primary);
  transform: translateX(2px);
}

.settings-nav-btn:active {
  transform: translateX(0) scale(0.97);
}

.settings-nav-btn.is-active {
  background: var(--color-primary-container);
  color: var(--color-on-primary-container);
  font-weight: 500;
  transform: translateX(0);
}

.settings-nav-btn.is-active .settings-nav-indicator {
  transform: translateY(-50%) scaleY(1);
  opacity: 1;
}

/* prompt-6 Task 10: experimental settings group (no badge/recommendation). */
.settings-nav-group {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--color-border-default);
}

.settings-nav-group-label {
  margin: 0 16px 8px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--color-text-tertiary, var(--color-text-secondary));
  opacity: 0.85;
}

.settings-nav-btn--experimental {
  opacity: 0.9;
}

.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
}

.settings-content :deep(.settings-section) {
  max-width: 640px;
  /* 切换分区时淡入上移，实现丝滑过渡。
     v-show 从 display:none 变为 display:block 时 animation 自动触发。 */
  animation: settingsFadeInUp 0.28s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes settingsFadeInUp {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* 尊重用户的减少动效偏好 */
@media (prefers-reduced-motion: reduce) {
  .settings-content :deep(.settings-section) {
    animation: none;
  }
  .settings-nav-btn,
  .settings-nav-indicator {
    transition: none !important;
  }
}

.settings-content :deep(.section-title) {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 24px;
  color: var(--color-text-primary);
}

.settings-content :deep(.setting-row) {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
}

.settings-content :deep(.setting-label) {
  width: 180px;
  flex-shrink: 0;
  font-size: 13px;
  color: var(--color-text-secondary);
}

.settings-content :deep(.setting-control) {
  display: flex;
  align-items: center;
  gap: 8px;
}

.settings-content :deep(.slider-value) {
  font-size: 12px;
  color: var(--color-text-tertiary);
  margin-left: 8px;
}

.settings-content :deep(.prompt-actions) {
  display: flex;
  gap: 8px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.settings-content :deep(.prompt-hint) {
  display: block;
  margin-top: 6px;
  font-size: 12px;
  color: var(--color-text-tertiary);
}

.settings-content :deep(.color-swatches) {
  display: flex;
  gap: 8px;
}

.settings-content :deep(.color-swatch) {
  width: 28px;
  height: 28px;
  border-radius: var(--radius-full);
  border: 2px solid transparent;
  cursor: pointer;
  transition: border-color var(--transition-fast), transform var(--transition-fast);
}

.settings-content :deep(.color-swatch:hover) {
  transform: scale(1.1);
}

.settings-content :deep(.color-swatch.is-selected) {
  border-color: var(--color-text-primary);
}

.settings-content :deep(.shortcut-key) {
  display: inline-block;
  padding: 2px 8px;
  background: var(--color-bg-surface-container);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-xs);
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-text-primary);
}
</style>
