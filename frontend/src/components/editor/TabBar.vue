<script setup lang="ts">
import { editorState } from "@/stores/editor";
import { Close } from "@element-plus/icons-vue";
import { useI18n } from "@/lib/i18n";

const emit = defineEmits<{
  (e: "select", path: string): void;
  (e: "close", path: string): void;
}>();

const { t } = useI18n();

function handleSelect(path: string) {
  emit("select", path);
}

function handleClose(path: string) {
  emit("close", path);
}
</script>

<template>
  <div v-if="editorState.openFiles.length > 0" class="tab-bar">
    <div
      v-for="file in editorState.openFiles"
      :key="file.path"
      class="tab-bar__tab"
      :class="{ 'tab-bar__tab--active': file.path === editorState.activeFilePath }"
      @click="handleSelect(file.path)"
    >
      <span class="tab-bar__name">{{ file.name }}</span>
      <span v-if="file.isDirty" class="tab-bar__dirty" aria-hidden="true">●</span>
      <button
        type="button"
        class="tab-bar__close"
        :aria-label="t('tabBar.closeTabAria')"
        @click.stop="handleClose(file.path)"
      >
        <el-icon :size="12"><Close /></el-icon>
      </button>
    </div>
  </div>
</template>

<style scoped>
.tab-bar {
  display: flex;
  align-items: center;
  height: 36px;
  padding: 0 8px;
  background: var(--color-bg-surface-dim);
  border-bottom: 1px solid var(--color-border-subtle);
  overflow-x: auto;
  gap: 2px;
}

.tab-bar__tab {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 12px;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  transition: background-color var(--transition-fast),
              color var(--transition-fast);
}

.tab-bar__tab:hover {
  background: var(--color-bg-surface-container-low);
  color: var(--color-text-secondary);
}

.tab-bar__tab--active {
  background: var(--color-bg-base);
  color: var(--color-text-primary);
}

.tab-bar__tab--active:hover {
  background: var(--color-bg-base);
  color: var(--color-text-primary);
}

.tab-bar__name {
  font-weight: 400;
}

.tab-bar__tab--active .tab-bar__name {
  font-weight: 500;
}

.tab-bar__dirty {
  color: var(--color-primary);
  font-size: 10px;
  line-height: 1;
}

.tab-bar__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: none;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  transition: background-color var(--transition-fast),
              color var(--transition-fast);
}

.tab-bar__close:hover {
  background-color: color-mix(in srgb, var(--color-text-primary) 12%, transparent);
  color: var(--color-text-primary);
}

@media (prefers-reduced-motion: reduce) {
  .tab-bar__tab {
    transition: none;
  }
  .tab-bar__close {
    transition: none;
  }
}
</style>
