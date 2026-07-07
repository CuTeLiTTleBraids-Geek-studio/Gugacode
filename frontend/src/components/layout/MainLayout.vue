<script setup lang="ts">
import { ref, computed } from "vue";
import { useRoute } from "vue-router";
import {
  appState,
  setPanelTab,
  toggleTerminal,
  toggleActivityBar,
  toggleStatusBar,
  saveSettings,
} from "@/stores/app";
import { saveFile } from "@/stores/editor";
import { clearMessages } from "@/stores/ai";
import { toggleInlineCompletion } from "@/stores/inlineCompletion";
import { registerShortcut, useKeyboard } from "@/composables/useKeyboard";
import { useDragResize } from "@/composables/useDragResize";
import { useI18n } from "@/lib/i18n";
import type { Command } from "@/types";
import ActivityBar from "./ActivityBar.vue";
import TitleBar from "./TitleBar.vue";
import SidePanel from "./SidePanel.vue";
import TerminalPanel from "./TerminalPanel.vue";
import StatusBar from "./StatusBar.vue";
import CommandPalette from "./CommandPalette.vue";
import QuickOpen from "./QuickOpen.vue";
import LayoutLeafView from "./LayoutLeafView.vue";
import LayoutSplitView from "./LayoutSplitView.vue";
import {
  layoutState,
  setActiveLeaf,
  saveLayoutToBackend,
} from "@/stores/layout";
import { layoutService } from "@/api/services";
import { listPluginCommands, executePluginCommand } from "@/lib/pluginRegistry";

useKeyboard();

const { t } = useI18n();
const route = useRoute();

// App.vue 把 <router-view> 内容作为 default slot 传给 MainLayout。
// /editor 路由由 LayoutLeafView 直接渲染 EditorView（不走 slot），其他
// 路由（/settings、/projects、/plugins）通过 slot 显示在 center 区域。
const isEditorRoute = computed(() => route.path === "/editor");

const paletteVisible = ref(false);
// Plan 55: Quick Open (Ctrl+P) fuzzy file finder.
const quickOpenVisible = ref(false);

const commands = computed<Command[]>(() => {
  // N-42: Reference paletteVisible so this computed re-evaluates when the
  // palette opens, picking up plugin commands registered since the last
  // open. (Full reactivity is N-57/Proposal Q; this is a pragmatic bridge.)
  void paletteVisible.value;
  const builtin: Command[] = [
    { id: "save", label: t("mainLayout.commandSaveFile"), shortcut: "Ctrl+S", action: () => saveFile() },
    { id: "toggle-ai", label: t("mainLayout.commandToggleAiChat"), action: () => setPanelTab("ai") },
    { id: "toggle-terminal", label: t("mainLayout.commandToggleTerminal"), action: () => toggleTerminal() },
    { id: "clear-chat", label: t("mainLayout.commandClearChat"), action: () => clearMessages() },
    { id: "toggle-sidebar", label: t("mainLayout.commandToggleSidebar"), action: () => { appState.sidebarCollapsed = !appState.sidebarCollapsed; } },
    { id: "toggle-minimap", label: t("mainLayout.commandToggleMinimap"), action: () => { appState.minimap = !appState.minimap; } },
    { id: "toggle-inline-completion", label: t("mainLayout.commandToggleInlineCompletion"), action: () => toggleInlineCompletion() },
    // N-20: layout toggles
    { id: "toggle-activity-bar", label: t("mainLayout.commandToggleActivityBar"), action: () => { toggleActivityBar(); saveSettings(); } },
    { id: "toggle-status-bar", label: t("mainLayout.commandToggleStatusBar"), action: () => { toggleStatusBar(); saveSettings(); } },
  ];
  // N-42: Merge plugin commands. Built-in IDs take priority — if a plugin
  // registers a command with the same ID as a built-in, the built-in wins
  // and the plugin command is filtered out to avoid confusion.
  const builtinIds = new Set(builtin.map((c) => c.id));
  const pluginCommands: Command[] = listPluginCommands()
    .filter((rc) => !builtinIds.has(rc.id))
    .map((rc) => ({
      id: rc.id,
      // Prefix the label with category (if any) so users can find plugin
      // commands by typing the category name. This also makes the source
      // visible in the palette without needing UI changes.
      label: rc.category ? `${rc.category}: ${rc.title}` : rc.title,
      shortcut: rc.keybinding,
      // Fire-and-forget the async handler. Errors are surfaced to the
      // console (and the Output panel via executePluginCommand's path).
      action: () => {
        executePluginCommand(rc.id).catch((e: unknown) => {
          console.error(`Plugin command "${rc.id}" failed:`, e);
        });
      },
    }));
  return [...builtin, ...pluginCommands];
});

registerShortcut({
  key: "p",
  ctrl: true,
  shift: true,
  label: t("mainLayout.commandPalette"),
  handler: () => {
    paletteVisible.value = true;
  },
});

// Plan 55: Ctrl+P opens Quick Open (fuzzy file finder).
registerShortcut({
  key: "p",
  ctrl: true,
  label: t("mainLayout.quickOpen"),
  handler: () => {
    quickOpenVisible.value = true;
  },
});

registerShortcut({
  key: "s",
  ctrl: true,
  label: t("mainLayout.commandSaveFile"),
  handler: () => {
    saveFile();
  },
});

// N-20: drag handles for resizable panels.
// Sidebar: dragging right increases width.
const sidebarDrag = useDragResize({
  direction: "horizontal",
  sign: "positive-increases",
  min: 140,
  max: 600,
  getStartSize: () => appState.sidebarWidth,
  onResize: (w) => { appState.sidebarWidth = w; },
  onCommit: () => saveSettings(),
});

// Terminal: dragging up increases height (positive-decreases).
const terminalDrag = useDragResize({
  direction: "vertical",
  sign: "positive-decreases",
  min: 80,
  max: 600,
  getStartSize: () => appState.terminalHeight,
  onResize: (h) => { appState.terminalHeight = h; },
  onCommit: () => saveSettings(),
});

// Show sidebar drag handle only when sidebar is expanded.
const sidebarHandleVisible = computed(
  () => !appState.sidebarCollapsed,
);
// Show terminal drag handle only when terminal is visible.
const terminalHandleVisible = computed(() => appState.terminalVisible);

function handleRunCommand(cmd: Command) {
  cmd.action();
  paletteVisible.value = false;
}

// N-30: Layout tree activation handler. When a leaf is clicked, set it
// active and persist the layout (best-effort).
function handleLeafActivate(leafId: string) {
  setActiveLeaf(leafId);
  // Persist layout changes (debounced via saveSettings pattern is not
  // needed here — saveLayoutToBackend is already best-effort).
  void saveLayoutToBackend(layoutService.saveLayout);
}

// N-53/Proposal P: A drag handle resize ended — persist the updated sizes.
function handleResizeEnd() {
  void saveLayoutToBackend(layoutService.saveLayout);
}

// N-30 / Proposal H: Reset the layout to default — both in-memory and
// in the backend (removes layout.json, then persists the fresh default).
function handleResetLayout() {
  // Imported here to avoid circular dependency at module load.
  import("@/stores/layout").then(({ resetLayoutFromBackend }) => {
    void resetLayoutFromBackend(
      layoutService.resetLayout,
      layoutService.saveLayout,
    );
  });
}
</script>

<template>
  <div class="main-layout">
    <TitleBar />

    <div class="main-layout__body">
      <ActivityBar v-if="appState.activityBarVisible" />

      <SidePanel />

      <!-- Sidebar resize handle (N-20, N-54 a11y) -->
      <div
        v-if="sidebarHandleVisible"
        class="drag-handle drag-handle--horizontal"
        role="separator"
        tabindex="0"
        aria-orientation="vertical"
        :aria-valuenow="sidebarDrag.getCurrentValue()"
        :aria-valuemin="sidebarDrag.ariaMin"
        :aria-valuemax="sidebarDrag.ariaMax"
        :aria-label="t('layout.sidebarResizeHandle')"
        @pointerdown="sidebarDrag.onPointerDown"
        @keydown="sidebarDrag.onKeyDown"
      />

      <div class="main-layout__center">
        <!-- N-30: Layout tree renders the editor area. The root can be a
             leaf (single view) or a split (multiple views). -->
        <template v-if="isEditorRoute">
          <LayoutSplitView
            v-if="layoutState.tree.root.type === 'split'"
            :node="layoutState.tree.root"
            :active-leaf-id="layoutState.tree.activeLeafId"
            @activate="handleLeafActivate"
            @resizeend="handleResizeEnd"
          />
          <LayoutLeafView
            v-else
            :leaf="layoutState.tree.root"
            :active="layoutState.tree.root.id === layoutState.tree.activeLeafId"
            @activate="handleLeafActivate"
          />
        </template>
        <!-- 非 /editor 路由（/settings、/projects、/plugins）通过 slot
             渲染 App.vue 传给 MainLayout 的 <router-view> 内容。 -->
        <div v-else class="main-layout__route-view">
          <slot />
        </div>

        <!-- Terminal resize handle (N-20, N-54 a11y) -->
        <div
          v-if="terminalHandleVisible"
          class="drag-handle drag-handle--vertical"
          role="separator"
          tabindex="0"
          aria-orientation="horizontal"
          :aria-valuenow="terminalDrag.getCurrentValue()"
          :aria-valuemin="terminalDrag.ariaMin"
          :aria-valuemax="terminalDrag.ariaMax"
          :aria-label="t('layout.terminalResizeHandle')"
          @pointerdown="terminalDrag.onPointerDown"
          @keydown="terminalDrag.onKeyDown"
        />

        <TerminalPanel />
      </div>
    </div>

    <StatusBar v-if="appState.statusBarVisible" />

    <CommandPalette
      :visible="paletteVisible"
      :commands="commands"
      @close="paletteVisible = false"
      @run="handleRunCommand"
    />

    <!-- Plan 55: Quick Open (Ctrl+P) fuzzy file finder -->
    <QuickOpen
      :visible="quickOpenVisible"
      @close="quickOpenVisible = false"
    />
  </div>
</template>

<style scoped>
.main-layout {
  display: flex;
  flex-direction: column;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
  background-color: var(--color-bg-base);
  font-family: var(--font-sans);
}

.main-layout__body {
  display: flex;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.main-layout__center {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

/* 非 /editor 路由视图（settings/projects/plugins）通过 slot 渲染，
   需要 flex: 1 + min-height: 0 才能正确填充 center 区域并支持内部滚动。 */
.main-layout__route-view {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.main-layout__route-view > :deep(*) {
  flex: 1;
  min-height: 0;
}

.main-layout__editor {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 0;
  overflow: auto;
}

.main-layout__empty-text {
  font-size: 13px;
  color: var(--color-text-tertiary);
  user-select: none;
}

/* N-20: drag handles for resizable panels */
.drag-handle {
  flex-shrink: 0;
  background-color: var(--color-border-subtle);
  transition: background-color var(--transition-fast);
  z-index: 10;
}

.drag-handle:hover {
  background-color: var(--color-primary, #4285f4);
}

.drag-handle--horizontal {
  width: 4px;
  cursor: col-resize;
  height: 100%;
}

.drag-handle--vertical {
  height: 4px;
  cursor: row-resize;
  width: 100%;
}
</style>
