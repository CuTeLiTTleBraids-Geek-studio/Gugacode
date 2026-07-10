<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Events } from "@wailsio/runtime";
import { Document, FolderOpened, Top } from "@element-plus/icons-vue";
import MessageList from "@/components/ai-assistant/MessageList.vue";
import InputComposer from "@/components/ai-assistant/InputComposer.vue";
import SnapshotTimeline from "@/components/ai-assistant/SnapshotTimeline.vue";
import AiWorkspaceSidebar from "@/components/ai-window/AiWorkspaceSidebar.vue";
import AiTerminalDock from "@/components/ai-window/AiTerminalDock.vue";
import AiSkillsView from "@/components/ai-window/AiSkillsView.vue";
import AiAutomationView from "@/components/ai-window/AiAutomationView.vue";
import AiSettingsView from "@/components/ai-window/AiSettingsView.vue";
import { aiState, loadConversation, clearMessages, addContextChip } from "@/stores/ai";
import { aiAssistantState } from "@/stores/aiAssistant";
import { appState, saveSettings } from "@/stores/app";
import {
  aiWindowState,
  applyAIWindowTheme,
  getTerminalMaxWidth,
  setAISidebarWidth,
  setAITerminalWidth,
} from "@/stores/aiWindow";
import { conversationService, windowService, projectService } from "@/api/services";
import { useI18n } from "@/lib/i18n";
import { notifyError, notifySuccess, notifyWarning } from "@/lib/notifications";
import type { Project } from "@/types";
import { agentMcpTools, refreshAgentMcpTools } from "@/stores/mcp";
import { skillsList, loadSkills } from "@/stores/skills";
import { setSnapshotWorkspaceRoot, listSnapshots } from "@/stores/snapshot";

const { t } = useI18n();

type ComposerExpose = { handleAttach?: () => void };
const composer = ref<ComposerExpose | null>(null);
const editingTitle = ref(false);
const titleDraft = ref("");
const alwaysOnTop = ref(true);
const lastSelectionPath = ref("");
const viewportWidth = ref(window.innerWidth);

const projects = ref<Project[]>([]);
const selectedWorkspace = ref(appState.currentProject || "");
const selectedMcp = ref<string[]>([]);
const selectedSkills = ref<string[]>([]);
const showWorkspaceMenu = ref(false);
const showMcpMenu = ref(false);
const showSkillsMenu = ref(false);
const showModelMenu = ref(false);

const conversationTitle = computed(() => {
  if (aiState.currentConversationTitle) return aiState.currentConversationTitle;
  if (aiState.currentConversationId) return t("aiWindow.untitledConversation");
  return t("aiWindow.newConversation");
});

const modeLabel = computed(() => {
  const mode = aiAssistantState.mode;
  if (mode === "plan") return t("aiAssistant.modePlan");
  if (mode === "goal") return t("aiAssistant.modeGoal");
  if (mode === "agent") return t("aiAssistant.modeAgent");
  return t("aiAssistant.modeChat");
});

const modelOptions = computed(() => appState.aiProviderConfigs.map((cfg) => ({
  label: `${cfg.provider || cfg.name || "AI"}: ${cfg.model || "—"}`,
  value: cfg.model || "",
  configId: cfg.id,
})));

const currentModelLabel = computed(() => appState.aiModel || t("aiAssistant.noModel"));
const terminalMaxWidth = computed(() => getTerminalMaxWidth(
  Math.max(620, viewportWidth.value - aiWindowState.sidebarWidth),
));

function closeMenus(): void {
  showWorkspaceMenu.value = false;
  showMcpMenu.value = false;
  showSkillsMenu.value = false;
  showModelMenu.value = false;
}

async function handleSelectConversation(id: string): Promise<void> {
  aiWindowState.activeView = "assistant";
  if (!id) {
    aiState.currentConversationId = null;
    aiState.currentConversationTitle = null;
    clearMessages();
    return;
  }
  aiState.currentConversationId = id;
  await loadConversation(id);
}

function startEditTitle(): void {
  titleDraft.value = conversationTitle.value;
  editingTitle.value = true;
}

async function commitTitle(): Promise<void> {
  editingTitle.value = false;
  const next = titleDraft.value.trim();
  if (!next || !aiState.currentConversationId) return;
  try {
    await conversationService.updateTitle(aiState.currentConversationId, next);
    aiState.currentConversationTitle = next;
  } catch (error) {
    notifyError(error instanceof Error ? error.message : String(error));
  }
}

function resizeSidebar(width: number): void {
  setAISidebarWidth(width);
}

function persistSidebar(width: number): void {
  appState.aiSidebarWidth = setAISidebarWidth(width);
  saveSettings();
}

function resizeTerminal(width: number): void {
  setAITerminalWidth(Math.min(width, terminalMaxWidth.value));
}

function persistTerminal(width: number): void {
  appState.aiTerminalWidth = setAITerminalWidth(Math.min(width, terminalMaxWidth.value));
  saveSettings();
}

function toggleTerminal(): void {
  aiWindowState.terminalVisible = !aiWindowState.terminalVisible;
  if (aiWindowState.terminalVisible) appState.terminalVisible = true;
}

function closeTerminal(): void {
  aiWindowState.terminalVisible = false;
}

function selectWorkspace(path: string): void {
  selectedWorkspace.value = path;
  appState.currentProject = path;
  const project = projects.value.find((item) => item.path === path);
  if (project) appState.projectName = project.name;
  setSnapshotWorkspaceRoot(path);
  closeMenus();
}

function toggleMcp(namespace: string): void {
  const index = selectedMcp.value.indexOf(namespace);
  if (index >= 0) selectedMcp.value.splice(index, 1);
  else selectedMcp.value.push(namespace);
}

function toggleSkill(id: string): void {
  const index = selectedSkills.value.indexOf(id);
  if (index >= 0) selectedSkills.value.splice(index, 1);
  else selectedSkills.value.push(id);
}

function selectModel(model: string, configId: string): void {
  if (model) appState.aiModel = model;
  if (configId) appState.activeAIConfigId = configId;
  saveSettings();
  closeMenus();
}

async function openInExplorer(): Promise<void> {
  const path = selectedWorkspace.value || appState.currentProject;
  if (!path) return notifyWarning(t("aiWindow.noWorkspace"));
  try { await windowService.openPathInExplorer(path); }
  catch (error) { notifyError(error instanceof Error ? error.message : String(error)); }
}

async function openInVSCode(): Promise<void> {
  const path = selectedWorkspace.value || appState.currentProject;
  if (!path) return notifyWarning(t("aiWindow.noWorkspace"));
  try { await windowService.openPathInVSCode(path); }
  catch (error) { notifyError(error instanceof Error ? error.message : String(error)); }
}

async function toggleAlwaysOnTop(): Promise<void> {
  alwaysOnTop.value = !alwaysOnTop.value;
  try { await windowService.setAIAlwaysOnTop(alwaysOnTop.value); }
  catch (error) { notifyError(error instanceof Error ? error.message : String(error)); }
}

function onSelectionEvent(data: unknown): void {
  const payload = (Array.isArray(data) ? data[0] : data) as
    | { code?: string; language?: string; filePath?: string }
    | undefined;
  if (!payload?.code) return;
  const path = payload.filePath || "selection";
  if (payload.filePath && payload.filePath !== "untitled") lastSelectionPath.value = payload.filePath;
  addContextChip({
    id: `sel-${Date.now()}`,
    kind: "codeblock",
    label: path.split(/[/\\]/).pop() || path,
    content: payload.code,
    language: payload.language || "text",
  });
  aiWindowState.activeView = "assistant";
  notifySuccess(t("aiWindow.selectionReceived"));
}

function handleMessageClick(event: MouseEvent): void {
  const target = event.target as HTMLElement | null;
  if (!target?.classList.contains("code-block-apply-btn")) return;
  const code = target.closest(".code-block-wrap")?.querySelector("pre")?.textContent ?? "";
  const filePath = lastSelectionPath.value || appState.currentFilePath || "";
  if (!filePath) return notifyError(t("aiWindow.noActiveFile"), t("aiWindow.applyTitle"));
  void Events.Emit("ai:apply-to-editor", { code, filePath, language: "" });
  notifySuccess(t("aiWindow.applySent"));
}

let unsubSelection: (() => void) | null = null;
let systemThemeQuery: MediaQueryList | null = null;

function applyCurrentAITheme(): void {
  applyAIWindowTheme(aiWindowState.theme);
}

function handleViewportResize(): void {
  viewportWidth.value = window.innerWidth;
  if (aiWindowState.terminalWidth > terminalMaxWidth.value) {
    setAITerminalWidth(terminalMaxWidth.value);
  }
}

onMounted(async () => {
  aiWindowState.theme = appState.aiWindowTheme;
  setAISidebarWidth(appState.aiSidebarWidth);
  setAITerminalWidth(appState.aiTerminalWidth);
  applyCurrentAITheme();
  window.addEventListener("resize", handleViewportResize);

  systemThemeQuery = typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-color-scheme: light)")
    : null;
  systemThemeQuery?.addEventListener?.("change", applyCurrentAITheme);

  try { alwaysOnTop.value = await windowService.isAIAlwaysOnTop(); }
  catch { alwaysOnTop.value = true; }
  try { projects.value = await projectService.getRecentProjects(); }
  catch { projects.value = []; }

  if (selectedWorkspace.value) setSnapshotWorkspaceRoot(selectedWorkspace.value);
  void refreshAgentMcpTools();
  void loadSkills();
  void listSnapshots();

  try {
    const off = Events.On("ai:selection", (event: unknown) => {
      const data = event && typeof event === "object" && "data" in event
        ? (event as { data: unknown }).data
        : event;
      onSelectionEvent(data);
    });
    unsubSelection = typeof off === "function" ? off : null;
  } catch {
    unsubSelection = null;
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", handleViewportResize);
  systemThemeQuery?.removeEventListener?.("change", applyCurrentAITheme);
  unsubSelection?.();
});

watch(() => aiWindowState.theme, applyCurrentAITheme);
watch(
  () => [appState.theme, appState.designLanguage],
  () => applyCurrentAITheme(),
);
watch(selectedWorkspace, (root) => { if (root) setSnapshotWorkspaceRoot(root); });
</script>

<template>
  <div class="ai-window" @click.self="closeMenus">
    <AiWorkspaceSidebar
      :active-view="aiWindowState.activeView"
      :width="aiWindowState.sidebarWidth"
      :terminal-visible="aiWindowState.terminalVisible"
      @select-view="aiWindowState.activeView = $event"
      @select-conversation="handleSelectConversation"
      @toggle-terminal="toggleTerminal"
      @resize="resizeSidebar"
      @resize-commit="persistSidebar"
    />

    <main class="ai-window__main">
      <header class="ai-window__top">
        <div class="ai-window__heading">
          <div v-if="!editingTitle" class="ai-window__title" @dblclick="startEditTitle">
            {{ aiWindowState.activeView === "assistant" ? conversationTitle : t(`aiWorkspace.${aiWindowState.activeView}`) }}
          </div>
          <input
            v-else
            v-model="titleDraft"
            class="ai-window__title-input"
            autofocus
            @keydown.enter="commitTitle"
            @keydown.esc="editingTitle = false"
            @blur="commitTitle"
          />
          <span v-if="aiWindowState.activeView === 'assistant'" class="ai-window__mode-badge">{{ modeLabel }}</span>
        </div>
        <div class="ai-window__top-actions">
          <button type="button" :title="t('aiWindow.actExplorer')" @click="openInExplorer"><el-icon><FolderOpened /></el-icon></button>
          <button type="button" :title="t('aiWindow.actVSCode')" @click="openInVSCode"><el-icon><Document /></el-icon></button>
          <button type="button" :class="{ 'is-active': alwaysOnTop }" :title="t('aiWindow.alwaysOnTop')" @click="toggleAlwaysOnTop"><el-icon><Top /></el-icon></button>
        </div>
      </header>

      <section class="ai-window__workspace-row">
        <div class="ai-window__workspace-main">
          <section v-show="aiWindowState.activeView === 'assistant'" class="ai-window__assistant">
            <div class="ai-window__messages" @click="handleMessageClick"><MessageList /></div>
            <footer class="ai-window__footer">
              <div class="ai-window__toolbar">
                <button type="button" class="ai-window__tool" :title="t('aiAssistant.attach')" @click="composer?.handleAttach?.()">📎</button>
                <div class="ai-window__dropdown">
                  <button type="button" class="ai-window__tool" @click.stop="showWorkspaceMenu = !showWorkspaceMenu; showMcpMenu = false; showSkillsMenu = false; showModelMenu = false">
                    {{ t("aiWindow.workspace") }} <span v-if="selectedWorkspace" class="ai-window__badge">1</span> ▾
                  </button>
                  <ul v-if="showWorkspaceMenu" class="ai-window__menu">
                    <li v-for="project in projects" :key="project.path" :class="{ 'is-selected': project.path === selectedWorkspace }" @click="selectWorkspace(project.path)">{{ project.name }}</li>
                    <li v-if="!projects.length" class="ai-window__menu-empty">{{ t("aiWindow.noProjects") }}</li>
                  </ul>
                </div>
                <div class="ai-window__dropdown">
                  <button type="button" class="ai-window__tool" @click.stop="showMcpMenu = !showMcpMenu; showWorkspaceMenu = false; showSkillsMenu = false; showModelMenu = false">MCP <span v-if="selectedMcp.length" class="ai-window__badge">{{ selectedMcp.length }}</span> ▾</button>
                  <ul v-if="showMcpMenu" class="ai-window__menu">
                    <li v-for="tool in agentMcpTools" :key="tool.namespace" @click.stop="toggleMcp(tool.namespace)"><input type="checkbox" :checked="selectedMcp.includes(tool.namespace)" readonly />{{ tool.namespace }}</li>
                    <li v-if="!agentMcpTools.length" class="ai-window__menu-empty">{{ t("aiAssistant.noMcpTools") }}</li>
                  </ul>
                </div>
                <div class="ai-window__dropdown">
                  <button type="button" class="ai-window__tool" @click.stop="showSkillsMenu = !showSkillsMenu; showWorkspaceMenu = false; showMcpMenu = false; showModelMenu = false">Skills <span v-if="selectedSkills.length" class="ai-window__badge">{{ selectedSkills.length }}</span> ▾</button>
                  <ul v-if="showSkillsMenu" class="ai-window__menu">
                    <li v-for="skill in skillsList" :key="skill.id" @click.stop="toggleSkill(skill.id)"><input type="checkbox" :checked="selectedSkills.includes(skill.id)" readonly />{{ skill.name }}</li>
                    <li v-if="!skillsList.length" class="ai-window__menu-empty">{{ t("aiAssistant.noSkillsAvailable") }}</li>
                  </ul>
                </div>
                <div class="ai-window__dropdown ai-window__dropdown--model">
                  <button type="button" class="ai-window__tool" @click.stop="showModelMenu = !showModelMenu; showWorkspaceMenu = false; showMcpMenu = false; showSkillsMenu = false">{{ currentModelLabel }} ▾</button>
                  <ul v-if="showModelMenu" class="ai-window__menu">
                    <li v-for="model in modelOptions" :key="model.configId + model.value" :class="{ 'is-selected': model.value === appState.aiModel }" @click="selectModel(model.value, model.configId)">{{ model.label }}</li>
                    <li v-if="!modelOptions.length" class="ai-window__menu-empty">{{ t("aiAssistant.noModel") }}</li>
                  </ul>
                </div>
              </div>
              <InputComposer ref="composer" />
            </footer>
          </section>

          <AiSkillsView v-show="aiWindowState.activeView === 'skills'" />
          <AiAutomationView v-show="aiWindowState.activeView === 'automation'" />
          <AiSettingsView v-show="aiWindowState.activeView === 'settings'" />
          <section v-show="aiWindowState.activeView === 'rollback'" class="ai-window__feature-page"><SnapshotTimeline /></section>
        </div>

        <AiTerminalDock
          :visible="aiWindowState.terminalVisible"
          :width="aiWindowState.terminalWidth"
          :max-width="terminalMaxWidth"
          @close="closeTerminal"
          @resize="resizeTerminal"
          @resize-commit="persistTerminal"
        />
      </section>
    </main>
  </div>
</template>

<style scoped>
.ai-window { display: flex; width: 100vw; height: 100vh; overflow: hidden; color: var(--color-text-primary); background: var(--color-bg-base); font-family: var(--font-sans); }
.ai-window__main { display: flex; flex: 1; min-width: 0; flex-direction: column; overflow: hidden; }
.ai-window__top { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 52px; padding: 0 14px 0 18px; border-bottom: 1px solid var(--color-border-default); background: var(--color-bg-surface); }
.ai-window__heading { display: flex; min-width: 0; align-items: center; gap: 9px; }
.ai-window__title { overflow: hidden; color: var(--color-text-primary); font-size: 14px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; cursor: text; }
.ai-window__title-input { min-width: 240px; padding: 5px 8px; border: 1px solid var(--color-primary); border-radius: var(--radius-sm); color: var(--color-text-primary); background: var(--color-bg-elevated); outline: none; }
.ai-window__mode-badge { padding: 3px 8px; border-radius: var(--radius-pill); color: var(--color-text-secondary); background: var(--color-bg-surface-container); font-size: 10px; }
.ai-window__top-actions { display: flex; gap: 4px; }
.ai-window__top-actions button { display: grid; width: 32px; height: 32px; place-items: center; border: 0; border-radius: var(--radius-sm); color: var(--color-text-secondary); background: transparent; cursor: pointer; }
.ai-window__top-actions button:hover, .ai-window__top-actions button.is-active { color: var(--color-primary); background: var(--chrome-hover-bg); }
.ai-window__workspace-row { display: flex; flex: 1; min-height: 0; overflow: hidden; }
.ai-window__workspace-main { position: relative; flex: 1; min-width: 0; overflow: hidden; }
.ai-window__assistant { display: flex; height: 100%; flex-direction: column; overflow: hidden; }
.ai-window__messages { display: flex; flex: 1; min-height: 0; flex-direction: column; overflow: hidden; background-image: var(--personalization-chat-bg, none); background-position: center; background-size: cover; }
.ai-window__footer { flex: 0 0 auto; border-top: 1px solid var(--color-border-default); background: var(--color-bg-surface); }
.ai-window__toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; padding: 8px 12px 0; }
.ai-window__tool { display: inline-flex; max-width: 180px; align-items: center; gap: 4px; overflow: hidden; padding: 5px 9px; border: 1px solid var(--color-border-default); border-radius: var(--radius-pill); color: var(--color-text-secondary); background: var(--color-bg-surface-container-low); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }
.ai-window__tool:hover { color: var(--color-text-primary); border-color: var(--color-primary); }
.ai-window__badge { display: inline-grid; min-width: 16px; height: 16px; padding: 0 4px; place-items: center; border-radius: var(--radius-pill); color: var(--color-on-primary); background: var(--color-primary); font-size: 9px; }
.ai-window__dropdown { position: relative; }
.ai-window__menu { position: absolute; z-index: 50; bottom: calc(100% + 5px); left: 0; min-width: 190px; max-height: 240px; overflow: auto; padding: 4px; border: 1px solid var(--color-border-default); border-radius: var(--radius-sm); background: var(--color-bg-elevated); box-shadow: var(--shadow-floating); list-style: none; }
.ai-window__menu li { display: flex; align-items: center; gap: 6px; padding: 7px 9px; border-radius: 6px; font-size: 11px; cursor: pointer; }
.ai-window__menu li:hover, .ai-window__menu li.is-selected { background: var(--chrome-active-bg); }
.ai-window__menu-empty { color: var(--color-text-tertiary); cursor: default !important; }
.ai-window__feature-page { height: 100%; overflow: auto; padding: 24px; }
.ai-window__messages :deep(.ai-msg) { max-width: 92%; padding: 16px; border-radius: var(--radius-lg); }
.ai-window__messages :deep(.ai-msg--user) { align-self: flex-end; color: var(--color-on-primary); background: var(--color-primary); }
.ai-window__messages :deep(.ai-msg--assistant) { align-self: flex-start; border: 1px solid var(--color-border-subtle); background: var(--color-bg-elevated); }
@media (prefers-reduced-motion: reduce) { .ai-window *, .ai-window *::before, .ai-window *::after { scroll-behavior: auto !important; animation-duration: .01ms !important; transition-duration: .01ms !important; } }
</style>
