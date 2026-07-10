<script setup lang="ts">
import { ref, computed } from "vue";
import { fileService } from "@/api/services";
import { createSession } from "@/stores/terminal";
import { appState } from "@/stores/app";
import type { DirEntry } from "@/types";
import { CaretRight, Folder, Document } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { errorMessage as errorToString } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";

const { t } = useI18n();

/** prompt-5 Task J: window virtualization threshold for large directories. */
const VIRTUALIZE_THRESHOLD = 80;
const ROW_HEIGHT = 26;
const VIEWPORT_HEIGHT = 400;
const OVERSCAN = 8;

const props = withDefaults(defineProps<{
  path: string;
  name: string;
  depth?: number;
  isDir?: boolean;
}>(), {
  depth: 0,
  isDir: true,
});

const emit = defineEmits<{
  (e: "select", path: string): void;
}>();

const expanded = ref(false);
const loading = ref(false);
const loaded = ref(false);
const errorMessage = ref<string | null>(null);
const children = ref<DirEntry[]>([]);
const scrollTop = ref(0);

const useVirtual = computed(
  () => children.value.length >= VIRTUALIZE_THRESHOLD,
);

const virtualWindow = computed(() => {
  if (!useVirtual.value) {
    return {
      start: 0,
      end: children.value.length,
      offsetY: 0,
      totalH: children.value.length * ROW_HEIGHT,
    };
  }
  const total = children.value.length;
  const start = Math.max(0, Math.floor(scrollTop.value / ROW_HEIGHT) - OVERSCAN);
  const visible = Math.ceil(VIEWPORT_HEIGHT / ROW_HEIGHT) + OVERSCAN * 2;
  const end = Math.min(total, start + visible);
  return {
    start,
    end,
    offsetY: start * ROW_HEIGHT,
    totalH: total * ROW_HEIGHT,
  };
});

const visibleChildren = computed(() =>
  children.value.slice(virtualWindow.value.start, virtualWindow.value.end),
);

function onChildrenScroll(e: Event): void {
  const el = e.target as HTMLElement;
  scrollTop.value = el.scrollTop;
}

const contextMenuVisible = ref(false);
const contextMenuX = ref(0);
const contextMenuY = ref(0);

const isFolder = computed(() => props.depth === 0 || props.isDir);

async function toggle() {
  if (expanded.value) {
    expanded.value = false;
    return;
  }
  expanded.value = true;
  if (loaded.value || loading.value) {
    return;
  }
  loading.value = true;
  errorMessage.value = null;
  try {
    children.value = await fileService.listDirectory(props.path);
    loaded.value = true;
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
    console.error("Failed to list directory:", err);
  } finally {
    loading.value = false;
  }
}

function onRowClick() {
  if (isFolder.value) {
    toggle();
  } else {
    emit("select", props.path);
  }
}

function onContextMenu(e: MouseEvent) {
  e.preventDefault();
  contextMenuX.value = e.clientX;
  contextMenuY.value = e.clientY;
  contextMenuVisible.value = true;
}

function closeContextMenu() {
  contextMenuVisible.value = false;
}

async function handleNewFile() {
  closeContextMenu();
  if (!isFolder.value) return;
  try {
    const { value } = await ElMessageBox.prompt(t("fileTree.fileNamePrompt"), t("fileTree.newFile"), {
      confirmButtonText: t("fileTree.create"),
      cancelButtonText: t("common.cancel"),
    });
    if (!value) return;
    const newPath = props.path + "/" + value;
    await fileService.createFile(newPath);
    if (!expanded.value) expanded.value = true;
    await reloadChildren();
    emit("select", newPath);
  } catch (e: unknown) {
    ElMessage.error(t("fileTree.failedAction", { error: errorToString(e) }));
  }
}

async function handleNewFolder() {
  closeContextMenu();
  if (!isFolder.value) return;
  try {
    const { value } = await ElMessageBox.prompt(t("fileTree.folderNamePrompt"), t("fileTree.newFolder"), {
      confirmButtonText: t("fileTree.create"),
      cancelButtonText: t("common.cancel"),
    });
    if (!value) return;
    const newPath = props.path + "/" + value;
    await fileService.createDirectory(newPath);
    if (!expanded.value) expanded.value = true;
    await reloadChildren();
  } catch (e: unknown) {
    ElMessage.error(t("fileTree.failedAction", { error: errorToString(e) }));
  }
}

async function handleRename() {
  closeContextMenu();
  try {
    const { value } = await ElMessageBox.prompt(t("fileTree.newNamePrompt"), t("fileTree.renameTitle"), {
      confirmButtonText: t("fileTree.rename"),
      cancelButtonText: t("common.cancel"),
      inputValue: props.name,
    });
    if (!value || value === props.name) return;
    const parentPath = props.path.substring(0, props.path.lastIndexOf("/"));
    const newPath = parentPath + "/" + value;
    await fileService.renamePath(props.path, newPath);
    emit("select", newPath);
  } catch (e: unknown) {
    ElMessage.error(t("fileTree.failedAction", { error: errorToString(e) }));
  }
}

async function handleDelete() {
  closeContextMenu();
  try {
    await ElMessageBox.confirm(
      t("fileTree.deleteConfirm", { name: props.name }),
      t("fileTree.confirmDeleteTitle"),
      { confirmButtonText: t("fileTree.delete"), cancelButtonText: t("common.cancel"), type: "warning" }
    );
    await fileService.deletePath(props.path);
  } catch (e: unknown) {
    if (e !== "cancel") {
      ElMessage.error(t("fileTree.failedAction", { error: errorToString(e) }));
    }
  }
}

async function handleCopyPath() {
  closeContextMenu();
  try {
    await navigator.clipboard.writeText(props.path);
    ElMessage.success(t("fileTree.pathCopied"));
  } catch {
    ElMessage.error(t("fileTree.failedCopyPath"));
  }
}

async function handleOpenInTerminal() {
  closeContextMenu();
  // For a folder, use its own path; for a file, use the parent directory.
  const targetDir = isFolder.value
    ? props.path
    : props.path.substring(0, props.path.lastIndexOf("/"));
  if (!targetDir) {
    ElMessage.error(t("fileTree.cannotResolveDir"));
    return;
  }
  // Reveal the bottom panel so the user sees the new terminal.
  appState.terminalVisible = true;
  try {
    const id = await createSession(targetDir);
    if (!id) {
      ElMessage.error(t("fileTree.failedOpenTerminal"));
    }
  } catch (e: unknown) {
    ElMessage.error(t("fileTree.failedOpenTerminalError", { error: errorToString(e) }));
  }
}

async function handleRevealInOS() {
  closeContextMenu();
  try {
    await fileService.revealInOS(props.path);
  } catch (e: unknown) {
    ElMessage.error(t("fileTree.failedReveal", { error: errorToString(e) }));
  }
}

async function reloadChildren() {
  loaded.value = false;
  loading.value = true;
  try {
    children.value = await fileService.listDirectory(props.path);
    loaded.value = true;
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
}

const indent = { paddingLeft: `${props.depth * 12 + 8}px` };
</script>

<template>
  <div class="file-tree">
    <div
      class="file-tree__row"
      :style="indent"
      @click="onRowClick"
      @contextmenu="onContextMenu"
    >
      <button
        type="button"
        v-if="isFolder && depth > 0"
        class="file-tree__chevron"
        :class="{ 'file-tree__chevron--expanded': expanded }"
        @click.stop="toggle"
        :aria-label="t('fileTree.toggleFolder')"
      >
        <el-icon :size="12"><CaretRight /></el-icon>
      </button>
      <span v-else class="file-tree__chevron-placeholder" />

      <el-icon :size="14" class="file-tree__icon">
        <Folder v-if="isFolder" />
        <Document v-else />
      </el-icon>

      <span class="file-tree__name">{{ name }}</span>
    </div>

    <div v-if="expanded && loading" class="file-tree__loading">
      {{ t("fileTree.loading") }}
    </div>

    <div v-if="expanded && errorMessage" class="file-tree__error">
      {{ errorMessage }}
    </div>

    <!-- prompt-5 Task J: virtualize large directories to keep DOM bounded -->
    <div
      v-if="expanded && !loading && !errorMessage && useVirtual"
      class="file-tree__children file-tree__children--virtual"
      :style="{ maxHeight: VIEWPORT_HEIGHT + 'px' }"
      @scroll.passive="onChildrenScroll"
    >
      <div class="file-tree__virt-spacer" :style="{ height: virtualWindow.totalH + 'px' }">
        <div :style="{ transform: `translateY(${virtualWindow.offsetY}px)` }">
          <FileTree
            v-for="child in visibleChildren"
            :key="child.path"
            :path="child.path"
            :name="child.name"
            :is-dir="child.isDir"
            :depth="depth + 1"
            @select="emit('select', $event)"
          />
        </div>
      </div>
    </div>
    <div v-else-if="expanded && !loading && !errorMessage" class="file-tree__children">
      <FileTree
        v-for="child in children"
        :key="child.path"
        :path="child.path"
        :name="child.name"
        :is-dir="child.isDir"
        :depth="depth + 1"
        @select="emit('select', $event)"
      />
    </div>

    <Teleport to="body">
      <div
        v-if="contextMenuVisible"
        class="file-tree__context-menu"
        :style="{ left: contextMenuX + 'px', top: contextMenuY + 'px' }"
        @click="closeContextMenu"
        @contextmenu.prevent="closeContextMenu"
      >
        <button type="button" v-if="isFolder" class="ctx-item" @click="handleNewFile">{{ t("fileTree.newFile") }}</button>
        <button type="button" v-if="isFolder" class="ctx-item" @click="handleNewFolder">{{ t("fileTree.newFolder") }}</button>
        <button type="button" class="ctx-item" @click="handleRename">{{ t("fileTree.rename") }}</button>
        <button type="button" class="ctx-item ctx-item--danger" @click="handleDelete">{{ t("fileTree.delete") }}</button>
        <button type="button" class="ctx-item" @click="handleCopyPath">{{ t("fileTree.copyPath") }}</button>
        <button type="button" class="ctx-item" @click="handleOpenInTerminal">{{ t("fileTree.openInTerminal") }}</button>
        <button type="button" class="ctx-item" @click="handleRevealInOS">{{ t("fileTree.revealInExplorer") }}</button>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.file-tree__row {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm, 8px);
  transition: background-color var(--transition-fast, 150ms var(--ease-standard));
}

.file-tree__row:hover {
  background-color: var(--color-bg-surface-container-low);
}

.file-tree__chevron {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  border-radius: var(--radius-xs, 4px);
  transition: transform var(--transition-fast, 150ms var(--ease-standard));
}

.file-tree__chevron--expanded {
  transform: rotate(90deg);
}

.file-tree__chevron-placeholder {
  width: 16px;
  flex-shrink: 0;
}

.file-tree__icon {
  color: var(--color-text-tertiary);
  flex-shrink: 0;
}

.file-tree__children--virtual {
  overflow-y: auto;
  overflow-x: hidden;
}

.file-tree__virt-spacer {
  position: relative;
  width: 100%;
}

.file-tree__name {
  font-size: 12px;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-tree__loading {
  padding: 4px 12px;
  font-size: 11px;
  color: var(--color-text-tertiary);
}

.file-tree__error {
  padding: 4px 12px;
  font-size: 11px;
  color: var(--color-error, var(--color-text-tertiary));
}

.file-tree__children {
  /* children render with their own indentation */
}

.file-tree__context-menu {
  position: fixed;
  z-index: 9999;
  min-width: 140px;
  padding: 4px;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-sm);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

.ctx-item {
  display: block;
  width: 100%;
  padding: 6px 10px;
  font-size: 12px;
  font-family: var(--font-sans);
  color: var(--color-text-secondary);
  background: transparent;
  border: none;
  border-radius: var(--radius-xs);
  text-align: left;
  cursor: pointer;
}

.ctx-item:hover {
  background: var(--color-bg-surface-container-low);
  color: var(--color-text-primary);
}

.ctx-item--danger:hover {
  color: var(--color-error, #f87171);
}
</style>
